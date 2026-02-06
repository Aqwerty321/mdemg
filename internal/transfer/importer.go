package transfer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	pb "mdemg/api/transferpb"
)

// Importer writes SpaceChunks into Neo4j.
type Importer struct {
	driver       neo4j.DriverWithContext
	conflictMode pb.ConflictMode
}

// NewImporter creates a new Importer with the specified conflict handling mode.
func NewImporter(driver neo4j.DriverWithContext, mode pb.ConflictMode) *Importer {
	return &Importer{driver: driver, conflictMode: mode}
}

// ImportResult tracks statistics from the import operation.
type ImportResult struct {
	NodesCreated        int32
	NodesSkipped        int32
	NodesOverwritten    int32
	EdgesCreated        int32
	EdgesSkipped        int32
	ObservationsCreated int32
	SymbolsCreated      int32
	Warnings            []string
	Duration            time.Duration
}

// ToProto converts ImportResult to a protobuf ImportResponse.
func (r *ImportResult) ToProto() *pb.ImportResponse {
	return &pb.ImportResponse{
		Success: true,
		Stats: &pb.ImportStats{
			NodesCreated:        r.NodesCreated,
			NodesSkipped:        r.NodesSkipped,
			NodesOverwritten:    r.NodesOverwritten,
			EdgesCreated:        r.EdgesCreated,
			EdgesSkipped:        r.EdgesSkipped,
			ObservationsCreated: r.ObservationsCreated,
			SymbolsCreated:      r.SymbolsCreated,
			DurationMs:          r.Duration.Milliseconds(),
		},
		Warnings: r.Warnings,
	}
}

// Import processes a sequence of SpaceChunks and writes them to Neo4j.
func (imp *Importer) Import(ctx context.Context, chunks []*pb.SpaceChunk) (*ImportResult, error) {
	start := time.Now()
	result := &ImportResult{}

	// Validate schema version from metadata chunk
	for _, chunk := range chunks {
		if chunk.ChunkType == pb.ChunkType_CHUNK_TYPE_METADATA {
			if err := imp.validateSchema(ctx, chunk); err != nil {
				return nil, err
			}
			break
		}
	}

	// Ensure TapRoot exists for the space
	if len(chunks) > 0 {
		spaceID := chunks[0].SpaceId
		if err := imp.ensureTapRoot(ctx, spaceID); err != nil {
			return nil, fmt.Errorf("ensure taproot: %w", err)
		}
	}

	// Process chunks in order
	for _, chunk := range chunks {
		switch chunk.ChunkType {
		case pb.ChunkType_CHUNK_TYPE_NODES:
			if chunk.Nodes != nil {
				stats, err := imp.importNodes(ctx, chunk.Nodes.Nodes)
				if err != nil {
					return nil, fmt.Errorf("import nodes (chunk %d): %w", chunk.Sequence, err)
				}
				result.NodesCreated += stats.created
				result.NodesSkipped += stats.skipped
				result.NodesOverwritten += stats.overwritten
				log.Printf("Imported node chunk %d: %d created, %d skipped", chunk.Sequence, stats.created, stats.skipped)
			}

		case pb.ChunkType_CHUNK_TYPE_EDGES:
			if chunk.Edges != nil {
				stats, err := imp.importEdges(ctx, chunk.Edges.Edges)
				if err != nil {
					return nil, fmt.Errorf("import edges (chunk %d): %w", chunk.Sequence, err)
				}
				result.EdgesCreated += stats.created
				result.EdgesSkipped += stats.skipped
				log.Printf("Imported edge chunk %d: %d created, %d skipped", chunk.Sequence, stats.created, stats.skipped)
			}

		case pb.ChunkType_CHUNK_TYPE_OBSERVATIONS:
			if chunk.Observations != nil {
				count, err := imp.importObservations(ctx, chunk.Observations.Observations)
				if err != nil {
					return nil, fmt.Errorf("import observations (chunk %d): %w", chunk.Sequence, err)
				}
				result.ObservationsCreated += int32(count)
			}

		case pb.ChunkType_CHUNK_TYPE_SYMBOLS:
			if chunk.Symbols != nil {
				count, err := imp.importSymbols(ctx, chunk.Symbols.Symbols)
				if err != nil {
					return nil, fmt.Errorf("import symbols (chunk %d): %w", chunk.Sequence, err)
				}
				result.SymbolsCreated += int32(count)
			}

		case pb.ChunkType_CHUNK_TYPE_METADATA, pb.ChunkType_CHUNK_TYPE_SUMMARY:
			// Informational only
			continue
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

func (imp *Importer) validateSchema(ctx context.Context, chunk *pb.SpaceChunk) error {
	if chunk.Metadata == nil {
		return nil
	}

	localVersion, err := imp.getLocalSchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("check local schema: %w", err)
	}

	exportVersion := int(chunk.Metadata.SchemaVersion)
	if exportVersion > localVersion {
		return fmt.Errorf("export schema version %d is newer than local schema version %d — apply migrations first", exportVersion, localVersion)
	}

	return nil
}

func (imp *Importer) getLocalSchemaVersion(ctx context.Context) (int, error) {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, `MATCH (s:SchemaMeta {key: "schema_version"}) RETURN s.value AS version`, nil)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			return res.Record().Values[0], nil
		}
		return int64(0), nil
	})
	if err != nil {
		return 0, err
	}
	return int(result.(int64)), nil
}

func (imp *Importer) ensureTapRoot(ctx context.Context, spaceID string) error {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `MERGE (t:TapRoot {space_id: $spaceId}) ON CREATE SET t.created_at = datetime()`,
			map[string]any{"spaceId": spaceID})
		return nil, err
	})
	return err
}

type batchStats struct {
	created     int32
	skipped     int32
	overwritten int32
}

func (imp *Importer) importNodes(ctx context.Context, nodes []*pb.NodeData) (batchStats, error) {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	var stats batchStats

	for _, nd := range nodes {
		result, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			// Check if node exists
			existRes, err := tx.Run(ctx,
				`MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId}) RETURN count(n) AS cnt`,
				map[string]any{"spaceId": nd.SpaceId, "nodeId": nd.NodeId})
			if err != nil {
				return nil, err
			}
			exists := false
			if existRes.Next(ctx) {
				cnt, _ := existRes.Record().Get("cnt")
				exists = cnt.(int64) > 0
			}

			if exists {
				switch imp.conflictMode {
				case pb.ConflictMode_CONFLICT_SKIP:
					return "skipped", nil
				case pb.ConflictMode_CONFLICT_ERROR:
					return nil, fmt.Errorf("node %s already exists (conflict_mode=error)", nd.NodeId)
				case pb.ConflictMode_CONFLICT_OVERWRITE:
					// Fall through to create/merge
				}
			}

			// Build property map
			props := map[string]any{
				"space_id":  nd.SpaceId,
				"node_id":   nd.NodeId,
				"path":      nd.Path,
				"name":      nd.Name,
				"layer":     int64(nd.Layer),
				"role_type": nd.RoleType,
			}

			if nd.Version > 0 {
				props["version"] = int64(nd.Version)
			}
			if nd.Description != "" {
				props["description"] = nd.Description
			}
			if nd.Summary != "" {
				props["summary"] = nd.Summary
			}
			if nd.Confidence > 0 {
				props["confidence"] = nd.Confidence
			}
			if nd.Sensitivity != "" {
				props["sensitivity"] = nd.Sensitivity
			}
			if len(nd.Tags) > 0 {
				props["tags"] = nd.Tags
			}
			if nd.UserId != "" {
				props["user_id"] = nd.UserId
			}
			if nd.Visibility != "" {
				props["visibility"] = nd.Visibility
			}
			if nd.Volatile {
				props["volatile"] = true
			}
			if nd.Content != "" {
				props["content"] = nd.Content
			}
			if nd.ObsType != "" {
				props["obs_type"] = nd.ObsType
			}
			if nd.SurpriseScore > 0 {
				props["surprise_score"] = nd.SurpriseScore
			}
			if nd.SessionId != "" {
				props["session_id"] = nd.SessionId
			}
			if nd.AgentId != "" {
				props["agent_id"] = nd.AgentId
			}
			if nd.MemberCount > 0 {
				props["member_count"] = int64(nd.MemberCount)
			}
			if nd.DominantObsType != "" {
				props["dominant_obs_type"] = nd.DominantObsType
			}
			if nd.AvgSurpriseScore > 0 {
				props["avg_surprise_score"] = nd.AvgSurpriseScore
			}
			if len(nd.Keywords) > 0 {
				props["keywords"] = nd.Keywords
			}
			if nd.SessionCount > 0 {
				props["session_count"] = int64(nd.SessionCount)
			}
			if nd.AggregationCount > 0 {
				props["aggregation_count"] = int64(nd.AggregationCount)
			}
			if nd.StabilityScore > 0 {
				props["stability_score"] = nd.StabilityScore
			}
			if nd.CreatedAt != "" {
				props["created_at"] = nd.CreatedAt
			}
			if nd.UpdatedAt != "" {
				props["updated_at"] = nd.UpdatedAt
			}

			// Handle embeddings (convert float32 → float64 for Neo4j)
			if len(nd.Embedding) > 0 {
				emb := make([]float64, len(nd.Embedding))
				for i, v := range nd.Embedding {
					emb[i] = float64(v)
				}
				props["embedding"] = emb
			}
			if len(nd.MessagePassEmbedding) > 0 {
				emb := make([]float64, len(nd.MessagePassEmbedding))
				for i, v := range nd.MessagePassEmbedding {
					emb[i] = float64(v)
				}
				props["message_pass_embedding"] = emb
			}

			// Build dynamic SET clause from props
			setClauses := make([]string, 0, len(props))
			for k := range props {
				setClauses = append(setClauses, fmt.Sprintf("n.%s = $props.%s", k, k))
			}

			cypher := fmt.Sprintf(`MERGE (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
ON CREATE SET %s
ON MATCH SET %s`, strings.Join(setClauses, ", "), strings.Join(setClauses, ", "))

			_, err = tx.Run(ctx, cypher, map[string]any{
				"spaceId": nd.SpaceId,
				"nodeId":  nd.NodeId,
				"props":   props,
			})
			if err != nil {
				return nil, err
			}

			if exists && imp.conflictMode == pb.ConflictMode_CONFLICT_OVERWRITE {
				return "overwritten", nil
			}
			return "created", nil
		})

		if err != nil {
			return stats, err
		}

		switch result.(string) {
		case "created":
			stats.created++
		case "skipped":
			stats.skipped++
		case "overwritten":
			stats.overwritten++
		}
	}

	return stats, nil
}

func (imp *Importer) importEdges(ctx context.Context, edges []*pb.EdgeData) (batchStats, error) {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	var stats batchStats

	for _, ed := range edges {
		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			// Use MERGE to avoid duplicates; APOC not required
			cypher := fmt.Sprintf(`
MATCH (a:MemoryNode {space_id: $spaceId, node_id: $fromId})
MATCH (b:MemoryNode {space_id: $spaceId, node_id: $toId})
MERGE (a)-[r:%s]->(b)
ON CREATE SET r.weight = $weight, r.created_at = datetime(), r.space_id = $spaceId
ON MATCH SET r.weight = CASE WHEN r.weight < $weight THEN $weight ELSE r.weight END`, ed.RelType)

			params := map[string]any{
				"spaceId": ed.SpaceId,
				"fromId":  ed.FromNodeId,
				"toId":    ed.ToNodeId,
				"weight":  ed.Weight,
			}

			_, err := tx.Run(ctx, cypher, params)
			return nil, err
		})
		if err != nil {
			// Log warning but continue — missing nodes are expected if partial export
			log.Printf("WARN: edge %s->%s (%s): %v", ed.FromNodeId, ed.ToNodeId, ed.RelType, err)
			stats.skipped++
			continue
		}
		stats.created++
	}

	return stats, nil
}

func (imp *Importer) importObservations(ctx context.Context, obs []*pb.ObservationData) (int, error) {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	count := 0
	for _, od := range obs {
		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			props := map[string]any{
				"space_id":  od.SpaceId,
				"obs_id":    od.ObsId,
				"content":   od.Content,
				"source":    od.Source,
				"timestamp": od.Timestamp,
			}
			if od.CreatedAt != "" {
				props["created_at"] = od.CreatedAt
			}
			if len(od.Embedding) > 0 {
				emb := make([]float64, len(od.Embedding))
				for i, v := range od.Embedding {
					emb[i] = float64(v)
				}
				props["embedding"] = emb
			}

			_, err := tx.Run(ctx, `MERGE (o:Observation {space_id: $spaceId, obs_id: $obsId})
ON CREATE SET o += $props`, map[string]any{
				"spaceId": od.SpaceId,
				"obsId":   od.ObsId,
				"props":   props,
			})
			if err != nil {
				return nil, err
			}

			// Link to parent node if specified
			if od.ParentNodeId != "" {
				_, err = tx.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
MATCH (o:Observation {space_id: $spaceId, obs_id: $obsId})
MERGE (n)-[:HAS_OBSERVATION]->(o)`, map[string]any{
					"spaceId": od.SpaceId,
					"nodeId":  od.ParentNodeId,
					"obsId":   od.ObsId,
				})
				if err != nil {
					log.Printf("WARN: could not link observation %s to node %s: %v", od.ObsId, od.ParentNodeId, err)
				}
			}

			return nil, nil
		})
		if err != nil {
			log.Printf("WARN: observation %s: %v", od.ObsId, err)
			continue
		}
		count++
	}

	return count, nil
}

func (imp *Importer) importSymbols(ctx context.Context, syms []*pb.SymbolData) (int, error) {
	sess := imp.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	count := 0
	for _, sd := range syms {
		_, err := sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			props := map[string]any{
				"space_id":    sd.SpaceId,
				"symbol_id":   sd.SymbolId,
				"name":        sd.Name,
				"symbol_type": sd.SymbolType,
				"file_path":   sd.FilePath,
				"line":        int64(sd.Line),
				"exported":    sd.Exported,
			}
			if sd.LineEnd > 0 {
				props["line_end"] = int64(sd.LineEnd)
			}
			if sd.Parent != "" {
				props["parent"] = sd.Parent
			}
			if sd.Signature != "" {
				props["signature"] = sd.Signature
			}
			if sd.Value != "" {
				props["value"] = sd.Value
			}
			if sd.DocComment != "" {
				props["doc_comment"] = sd.DocComment
			}
			if sd.Language != "" {
				props["language"] = sd.Language
			}
			if len(sd.Embedding) > 0 {
				emb := make([]float64, len(sd.Embedding))
				for i, v := range sd.Embedding {
					emb[i] = float64(v)
				}
				props["embedding"] = emb
			}

			_, err := tx.Run(ctx, `MERGE (s:SymbolNode {space_id: $spaceId, symbol_id: $symbolId})
ON CREATE SET s += $props`, map[string]any{
				"spaceId":  sd.SpaceId,
				"symbolId": sd.SymbolId,
				"props":    props,
			})
			if err != nil {
				return nil, err
			}

			// Link to parent memory node
			if sd.ParentNodeId != "" {
				_, err = tx.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
MATCH (s:SymbolNode {space_id: $spaceId, symbol_id: $symbolId})
MERGE (n)-[:HAS_SYMBOL]->(s)`, map[string]any{
					"spaceId":  sd.SpaceId,
					"nodeId":   sd.ParentNodeId,
					"symbolId": sd.SymbolId,
				})
				if err != nil {
					log.Printf("WARN: could not link symbol %s to node %s: %v", sd.SymbolId, sd.ParentNodeId, err)
				}
			}

			return nil, nil
		})
		if err != nil {
			log.Printf("WARN: symbol %s: %v", sd.SymbolId, err)
			continue
		}
		count++
	}

	return count, nil
}
