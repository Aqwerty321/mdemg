# Feature Spec: Space Transfer via gRPC

**Phase**: N/A (standalone tooling)
**Status**: Draft
**Author**: Agent (Cursor)
**Date**: 2026-02-06

---

## Overview

Enable sharing of mature MDEMG space_id graphs between developer environments. A developer who has built up a rich memory space (ingested code, learned edges, hidden layer concepts, conversation history) can export it and distribute it to teammates, so they start with full context instead of from zero.

**Transfer mechanism:** gRPC streaming — the same transport pattern MDEMG already uses for plugins.

**Data flow:**
```
Developer A (mature space)                     Developer B (new setup)
┌──────────────┐                               ┌──────────────┐
│  Neo4j       │   gRPC stream (or file)       │  Neo4j       │
│  space_id:   │  ────────────────────────►     │  space_id:   │
│  whk-wms     │   nodes + edges + embeddings  │  whk-wms     │
└──────────────┘                               └──────────────┘
```

## Requirements

### Functional Requirements
1. FR-1: Export all graph data for a given `space_id` (nodes, edges, embeddings, observations, symbols, hidden layer concepts)
2. FR-2: Import exported data into a target Neo4j, creating nodes/edges/indexes
3. FR-3: gRPC streaming for remote transfer (server-to-server or server-to-CLI)
4. FR-4: File-based export/import for offline sharing (`.mdemg` JSON format)
5. FR-5: Schema version validation — refuse import if target schema is incompatible
6. FR-6: Conflict handling — skip, overwrite, or error on existing nodes
7. FR-7: Progress reporting during export/import (node counts, edge counts, ETA)

### Non-Functional Requirements
1. NFR-1: Export 100k nodes in < 5 minutes (batch Cypher reads)
2. NFR-2: Streaming chunks of 500 nodes to avoid OOM
3. NFR-3: Embeddings transferred as float32 arrays (no re-embedding needed)
4. NFR-4: Zero interference with other agent's work (no changes to server.go, config.go, handlers.go)

## Architecture

### Components (all new files — zero conflict)

```
api/proto/space-transfer.proto      ← gRPC service definition
api/transferpb/                     ← Generated Go code
internal/transfer/                  ← Core export/import logic
  ├── exporter.go                   ← Read from Neo4j, produce chunks
  ├── importer.go                   ← Write chunks to Neo4j
  ├── format.go                     ← File serialization (.mdemg JSON)
  └── validate.go                   ← Schema version checks
cmd/space-transfer/                 ← CLI tool
  └── main.go                       ← Subcommands: export, import, serve, pull
```

### gRPC Service Design

```protobuf
service SpaceTransfer {
  // Export streams all data for a space from server to client
  rpc Export(ExportRequest) returns (stream SpaceChunk);
  
  // Import receives streamed chunks and writes to Neo4j
  rpc Import(stream SpaceChunk) returns (ImportResponse);
  
  // ListSpaces returns available spaces with metadata
  rpc ListSpaces(ListSpacesRequest) returns (ListSpacesResponse);
}
```

### CLI Subcommands

```bash
# Export to file (local Neo4j → file)
space-transfer export --space whk-wms --out ./spaces/whk-wms.mdemg

# Import from file (file → local Neo4j)
space-transfer import --file ./spaces/whk-wms.mdemg --conflict skip

# Serve gRPC endpoint for remote pulls
space-transfer serve --port 50051

# Pull from remote gRPC server
space-transfer pull --remote 192.168.1.100:50051 --space whk-wms
```

## Data Model

### Export Chunk Structure

Each chunk contains a batch of one entity type:

```go
type SpaceChunk struct {
    ChunkType    string        // "nodes", "edges", "observations", "symbols", "metadata"
    SpaceID      string
    SchemaVersion int
    Sequence     int           // Chunk sequence number
    TotalChunks  int           // -1 if unknown (streaming)
    Nodes        []NodeData    // Present when ChunkType == "nodes"
    Edges        []EdgeData    // Present when ChunkType == "edges"
    Observations []ObsData     // Present when ChunkType == "observations"  
    Symbols      []SymbolData  // Present when ChunkType == "symbols"
}
```

### What Gets Exported Per Space

| Entity | Neo4j Label | Key Properties |
|--------|-------------|----------------|
| Memory nodes | `:MemoryNode` | All properties including embedding (1536-dim) |
| Observations | `:Observation` | All properties including embedding |
| Symbol nodes | `:SymbolNode` | All properties including embedding |
| TapRoot | `:TapRoot` | Singleton per space |
| Hidden concepts | `:MemoryNode` (layer >= 1) | Included with memory nodes |
| CO_ACTIVATED_WITH edges | Learned edges | weight, evidence_count, timestamps |
| ASSOCIATED_WITH edges | Semantic edges | weight, initial_weight |
| GENERALIZES edges | Layer hierarchy | weight |
| ABSTRACTS_TO edges | Concept links | weight |
| HAS_OBSERVATION edges | Node→Obs links | All properties |
| HAS_SYMBOL edges | Node→Symbol links | All properties |
| TEMPORALLY_ADJACENT edges | Temporal links | weight |
| Capability gaps | `:CapabilityGap` | If present in space |
| Interview prompts | `:InterviewPrompt` | If present in space |

### What Does NOT Get Exported

- `:SchemaMeta` and `:Migration` nodes (target has its own)
- Indexes and constraints (created by migrations on target)
- Cross-space edges (if any exist)

## Conflict Resolution

| Mode | Behavior |
|------|----------|
| `skip` (default) | If node_id already exists in target, skip it |
| `overwrite` | Replace existing node with imported data |
| `error` | Abort import if any collision detected |
| `merge` | Future: merge properties intelligently |

## Test Plan

### Unit Tests
- [ ] Export produces correct chunk count for known graph
- [ ] Import creates expected nodes/edges in Neo4j
- [ ] Schema version mismatch rejects import
- [ ] Conflict modes (skip, overwrite, error) behave correctly
- [ ] Empty space export produces valid empty file
- [ ] Embeddings survive round-trip (float32 precision)

### Integration Tests
- [ ] Export → file → Import round-trip preserves all data
- [ ] gRPC Export → Import streaming between two processes
- [ ] Large space (10k+ nodes) completes within timeout
- [ ] Import into non-empty space with skip mode

## Acceptance Criteria

- [ ] AC-1: `space-transfer export --space X --out file.mdemg` produces valid export
- [ ] AC-2: `space-transfer import --file file.mdemg` populates target Neo4j
- [ ] AC-3: Exported space is queryable via `/v1/memory/retrieve` on target
- [ ] AC-4: gRPC streaming works for remote transfer
- [ ] AC-5: Schema version checked before import
- [ ] AC-6: Progress reported during export/import
- [ ] AC-7: `go vet ./...` reports no issues
- [ ] AC-8: All existing tests pass (`go test ./...`)

## Dependencies

- Depends on: Neo4j migrations (V0001-V0011) applied on both source and target
- Depends on: `google.golang.org/grpc` (already in go.mod)
- Depends on: `google.golang.org/protobuf` (already in go.mod)
- Blocks: None (additive feature)

## Files Changed

### New Files
- `api/proto/space-transfer.proto` — gRPC service definition
- `api/transferpb/space-transfer.pb.go` — Generated protobuf code
- `api/transferpb/space-transfer_grpc.pb.go` — Generated gRPC code
- `internal/transfer/exporter.go` — Neo4j → chunks
- `internal/transfer/importer.go` — Chunks → Neo4j
- `internal/transfer/format.go` — File I/O (.mdemg JSON)
- `internal/transfer/validate.go` — Schema checks
- `cmd/space-transfer/main.go` — CLI entrypoint

### Modified Files
- None (zero conflict with other agent)
