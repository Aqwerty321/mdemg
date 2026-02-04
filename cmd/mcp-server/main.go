// MDEMG MCP Server - Provides memory tools for coding agents
// Connects to the MDEMG HTTP API (endpoint resolved via .mdemg.port or MDEMG_ENDPOINT env)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"mdemg/internal/config"
)

const (
	defaultMDEMGEndpoint = "http://localhost:9999"
	defaultSpaceID       = "ide-agent"
)

var mdemgEndpoint string

func main() {
	// Resolve endpoint via priority chain: MDEMG_ENDPOINT env > .mdemg.port file > LISTEN_ADDR > default
	mdemgEndpoint = config.ResolveEndpoint(defaultMDEMGEndpoint)

	// Create MCP server
	s := server.NewMCPServer(
		"MDEMG Memory",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register tools
	registerMemoryStoreToool(s)
	registerMemoryRecallTool(s)
	registerMemoryAssociateTool(s)
	registerMemoryReflectTool(s)
	registerMemoryStatusTool(s)
	registerMemorySymbolsTool(s)
	registerMemoryIngestTriggerTool(s)
	registerMemoryIngestStatusTool(s)
	registerMemoryIngestCancelTool(s)
	registerMemoryIngestJobsTool(s)

	// Start server (stdio mode for Cursor integration)
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

// memory_store - Store an observation about code, decisions, patterns, or learnings
func registerMemoryStoreToool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_store",
		mcp.WithDescription(`Store a memory observation for later recall. Use this to remember:
- Code patterns and idioms you discover
- Decisions made and their rationale
- Problems solved and their solutions
- User preferences and conventions
- Project-specific knowledge

The memory system will automatically generate embeddings and link related concepts.`),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("The content to remember. Be specific and include context.")),
		mcp.WithString("name",
			mcp.Description("Short name/title for this memory (optional, auto-generated if not provided)")),
		mcp.WithString("path",
			mcp.Description("Hierarchical path like /project/component/topic (optional)")),
		mcp.WithString("tags",
			mcp.Description("Comma-separated tags for categorization (e.g., 'golang,error-handling,pattern')")),
		mcp.WithString("source",
			mcp.Description("Source of this observation (e.g., 'code-review', 'debugging', 'user-request')")),
	)

	s.AddTool(tool, memoryStoreHandler)
}

func memoryStoreHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments

	content, _ := args["content"].(string)
	if content == "" {
		return newToolResultError("content is required"), nil
	}

	name, _ := args["name"].(string)
	path, _ := args["path"].(string)
	tagsStr, _ := args["tags"].(string)
	source, _ := args["source"].(string)

	if source == "" {
		source = "agent-observation"
	}

	// Parse tags
	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	// Build ingest request
	ingestReq := map[string]any{
		"space_id":  defaultSpaceID,
		"content":   content,
		"source":    source,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if name != "" {
		ingestReq["name"] = name
	}
	if path != "" {
		ingestReq["path"] = path
	}
	if len(tags) > 0 {
		ingestReq["tags"] = tags
	}

	// Call MDEMG API
	resp, err := callMDEMG("/v1/memory/ingest", ingestReq)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to store memory: %v", err)), nil
	}

	nodeID, _ := resp["node_id"].(string)
	embDims, _ := resp["embedding_dims"].(float64)

	result := fmt.Sprintf("Memory stored successfully.\nNode ID: %s\nEmbedding dimensions: %d\nPath: %s",
		nodeID, int(embDims), path)

	return mcp.NewToolResultText(result), nil
}

// memory_recall - Retrieve relevant memories based on the current context
func registerMemoryRecallTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_recall",
		mcp.WithDescription(`Recall relevant memories based on a query. Use this to:
- Find previous solutions to similar problems
- Remember decisions made about a topic
- Retrieve learned patterns and idioms
- Get context about a project or component

Returns memories ranked by relevance, with semantic similarity and activation scores.`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("What you're looking for - describe the context or problem")),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of memories to return (default: 10)")),
	)

	s.AddTool(tool, memoryRecallHandler)
}

func memoryRecallHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments

	query, _ := args["query"].(string)
	if query == "" {
		return newToolResultError("query is required"), nil
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	// Call MDEMG API
	retrieveReq := map[string]any{
		"space_id":   defaultSpaceID,
		"query_text": query,
		"top_k":      limit,
	}

	resp, err := callMDEMG("/v1/memory/retrieve", retrieveReq)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to recall memories: %v", err)), nil
	}

	results, _ := resp["results"].([]any)
	if len(results) == 0 {
		return mcp.NewToolResultText("No relevant memories found."), nil
	}

	// Format results
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d relevant memories:\n\n", len(results)))

	for i, r := range results {
		result, _ := r.(map[string]any)
		name, _ := result["name"].(string)
		path, _ := result["path"].(string)
		summary, _ := result["summary"].(string)
		score, _ := result["score"].(float64)
		vectorSim, _ := result["vector_sim"].(float64)

		sb.WriteString(fmt.Sprintf("## %d. %s\n", i+1, name))
		if path != "" {
			sb.WriteString(fmt.Sprintf("   Path: %s\n", path))
		}
		sb.WriteString(fmt.Sprintf("   Relevance: %.1f%% (semantic: %.1f%%)\n", score*100, vectorSim*100))
		if summary != "" {
			sb.WriteString(fmt.Sprintf("   Summary: %s\n", summary))
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// memory_associate - Explicitly link two concepts or memories
func registerMemoryAssociateTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_associate",
		mcp.WithDescription(`Create an explicit association between two concepts or memories.
Use this when you discover that two things are related in a way the system might not
automatically detect (e.g., a workaround relates to a specific bug, or a pattern
applies to a particular domain).`),
		mcp.WithString("source_query",
			mcp.Required(),
			mcp.Description("Query to find the source memory/concept")),
		mcp.WithString("target_query",
			mcp.Required(),
			mcp.Description("Query to find the target memory/concept")),
		mcp.WithString("relationship",
			mcp.Description("Type of relationship: 'related', 'causes', 'enables', 'similar' (default: 'related')")),
		mcp.WithString("reason",
			mcp.Description("Why these concepts are associated")),
	)

	s.AddTool(tool, memoryAssociateHandler)
}

func memoryAssociateHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments

	sourceQuery, _ := args["source_query"].(string)
	targetQuery, _ := args["target_query"].(string)

	if sourceQuery == "" || targetQuery == "" {
		return newToolResultError("both source_query and target_query are required"), nil
	}

	relationship, _ := args["relationship"].(string)
	if relationship == "" {
		relationship = "related"
	}
	reason, _ := args["reason"].(string)

	// First, find both memories
	sourceResp, err := callMDEMG("/v1/memory/retrieve", map[string]any{
		"space_id":   defaultSpaceID,
		"query_text": sourceQuery,
		"top_k":      1,
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to find source: %v", err)), nil
	}

	targetResp, err := callMDEMG("/v1/memory/retrieve", map[string]any{
		"space_id":   defaultSpaceID,
		"query_text": targetQuery,
		"top_k":      1,
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to find target: %v", err)), nil
	}

	sourceResults, _ := sourceResp["results"].([]any)
	targetResults, _ := targetResp["results"].([]any)

	if len(sourceResults) == 0 {
		return newToolResultError("Could not find a memory matching source_query"), nil
	}
	if len(targetResults) == 0 {
		return newToolResultError("Could not find a memory matching target_query"), nil
	}

	sourceNode, _ := sourceResults[0].(map[string]any)
	targetNode, _ := targetResults[0].(map[string]any)

	sourceName, _ := sourceNode["name"].(string)
	targetName, _ := targetNode["name"].(string)

	// For now, store the association as a new observation linking the concepts
	// In the future, this would create a direct edge in the graph
	associationContent := fmt.Sprintf("Association: '%s' %s '%s'", sourceName, relationship, targetName)
	if reason != "" {
		associationContent += fmt.Sprintf("\nReason: %s", reason)
	}

	_, err = callMDEMG("/v1/memory/ingest", map[string]any{
		"space_id":  defaultSpaceID,
		"content":   associationContent,
		"source":    "explicit-association",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"name":      fmt.Sprintf("Link: %s → %s", sourceName, targetName),
		"tags":      []string{"association", relationship},
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to store association: %v", err)), nil
	}

	result := fmt.Sprintf("Association created:\n  '%s' --[%s]--> '%s'", sourceName, relationship, targetName)
	return mcp.NewToolResultText(result), nil
}

// memory_reflect - Summarize what is known about a topic
func registerMemoryReflectTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_reflect",
		mcp.WithDescription(`Reflect on what is known about a topic, triggering a broader search
and potentially identifying patterns or abstractions. Use this for:
- Understanding the full context around a concept
- Finding patterns across multiple related memories
- Preparing to make a decision by reviewing relevant knowledge`),
		mcp.WithString("topic",
			mcp.Required(),
			mcp.Description("The topic to reflect on")),
		mcp.WithNumber("depth",
			mcp.Description("How deeply to explore (1-3, default: 2)")),
	)

	s.AddTool(tool, memoryReflectHandler)
}

func memoryReflectHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments

	topic, _ := args["topic"].(string)
	if topic == "" {
		return newToolResultError("topic is required"), nil
	}

	depth := 2
	if d, ok := args["depth"].(float64); ok && d >= 1 && d <= 3 {
		depth = int(d)
	}

	// Retrieve with more candidates for deeper reflection
	candidateK := 50 * depth
	topK := 10 * depth

	resp, err := callMDEMG("/v1/memory/retrieve", map[string]any{
		"space_id":    defaultSpaceID,
		"query_text":  topic,
		"candidate_k": candidateK,
		"top_k":       topK,
		"hop_depth":   depth,
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to reflect: %v", err)), nil
	}

	results, _ := resp["results"].([]any)
	debug, _ := resp["debug"].(map[string]any)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Reflection on: %s\n\n", topic))

	if len(results) == 0 {
		sb.WriteString("No memories found on this topic yet.\n")
		return mcp.NewToolResultText(sb.String()), nil
	}

	// Group by relevance tiers
	var highRelevance, medRelevance, lowRelevance []map[string]any
	for _, r := range results {
		result, _ := r.(map[string]any)
		score, _ := result["score"].(float64)
		if score >= 0.7 {
			highRelevance = append(highRelevance, result)
		} else if score >= 0.4 {
			medRelevance = append(medRelevance, result)
		} else {
			lowRelevance = append(lowRelevance, result)
		}
	}

	sb.WriteString(fmt.Sprintf("Found %d relevant memories (depth: %d hops)\n\n", len(results), depth))

	if len(highRelevance) > 0 {
		sb.WriteString("## Highly Relevant\n")
		for _, r := range highRelevance {
			name, _ := r["name"].(string)
			path, _ := r["path"].(string)
			score, _ := r["score"].(float64)
			sb.WriteString(fmt.Sprintf("- **%s** (%.0f%%) %s\n", name, score*100, path))
		}
		sb.WriteString("\n")
	}

	if len(medRelevance) > 0 {
		sb.WriteString("## Related\n")
		for _, r := range medRelevance {
			name, _ := r["name"].(string)
			score, _ := r["score"].(float64)
			sb.WriteString(fmt.Sprintf("- %s (%.0f%%)\n", name, score*100))
		}
		sb.WriteString("\n")
	}

	if len(lowRelevance) > 0 {
		sb.WriteString("## Tangentially Related\n")
		for _, r := range lowRelevance {
			name, _ := r["name"].(string)
			score, _ := r["score"].(float64)
			sb.WriteString(fmt.Sprintf("- %s (%.0f%%)\n", name, score*100))
		}
		sb.WriteString("\n")
	}

	// Add debug info
	if debug != nil {
		edgesFetched, _ := debug["edges_fetched"].(float64)
		if edgesFetched > 0 {
			sb.WriteString(fmt.Sprintf("*Graph traversal explored %d edges*\n", int(edgesFetched)))
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// memory_status - Check the memory system status
func registerMemoryStatusTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_status",
		mcp.WithDescription("Check if the MDEMG memory system is running and get its status."),
	)

	s.AddTool(tool, memoryStatusHandler)
}

func memoryStatusHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(mdemgEndpoint + "/readyz")
	if err != nil {
		return newToolResultError(fmt.Sprintf("MDEMG service not reachable at %s: %v", mdemgEndpoint, err)), nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var status map[string]any
	json.Unmarshal(body, &status)

	var sb strings.Builder
	sb.WriteString("MDEMG Memory System Status\n\n")
	sb.WriteString(fmt.Sprintf("Endpoint: %s\n", mdemgEndpoint))
	sb.WriteString(fmt.Sprintf("Status: %v\n", status["status"]))

	if provider, ok := status["embedding_provider"].(string); ok {
		sb.WriteString(fmt.Sprintf("Embedding Provider: %s\n", provider))
	}
	if dims, ok := status["embedding_dimensions"].(float64); ok {
		sb.WriteString(fmt.Sprintf("Embedding Dimensions: %d\n", int(dims)))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// memory_symbols - Search for code symbols (functions, classes, constants)
func registerMemorySymbolsTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_symbols",
		mcp.WithDescription(`Search for code symbols in the memory graph. Use this to:
- Find function definitions and their locations
- Search for classes, interfaces, and types
- Locate constants and variables
- Discover exported vs unexported symbols

Returns symbols with their file locations, types, and metadata.`),
		mcp.WithString("name",
			mcp.Description("Symbol name pattern to search for (partial match supported)")),
		mcp.WithString("type",
			mcp.Description("Symbol type to filter by: function, class, method, constant, variable, interface, type")),
		mcp.WithString("file",
			mcp.Description("Filter symbols by file path (partial match)")),
		mcp.WithString("exported",
			mcp.Description("Filter by exported status: 'true' for exported only, 'false' for unexported only")),
		mcp.WithString("q",
			mcp.Description("Fulltext search query across all symbol metadata")),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of symbols to return (default: 20)")),
	)

	s.AddTool(tool, memorySymbolsHandler)
}

func memorySymbolsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments

	// Build query parameters
	params := make(map[string]string)
	params["space_id"] = defaultSpaceID

	if name, ok := args["name"].(string); ok && name != "" {
		params["name"] = name
	}
	if symbolType, ok := args["type"].(string); ok && symbolType != "" {
		params["type"] = symbolType
	}
	if file, ok := args["file"].(string); ok && file != "" {
		params["file"] = file
	}
	if exported, ok := args["exported"].(string); ok && exported != "" {
		params["exported"] = exported
	}
	if q, ok := args["q"].(string); ok && q != "" {
		params["q"] = q
	}

	limit := 20
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	params["limit"] = fmt.Sprintf("%d", limit)

	// Call MDEMG API (GET request with query params)
	resp, err := callMDEMGGet("/v1/memory/symbols", params)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to search symbols: %v", err)), nil
	}

	symbols, _ := resp["symbols"].([]any)
	total, _ := resp["total"].(float64)

	if len(symbols) == 0 {
		return mcp.NewToolResultText("No symbols found matching the criteria."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d symbols (showing %d):\n\n", int(total), len(symbols)))

	for i, s := range symbols {
		sym, _ := s.(map[string]any)
		name, _ := sym["name"].(string)
		symType, _ := sym["type"].(string)
		file, _ := sym["file"].(string)
		line, _ := sym["line"].(float64)
		exported, _ := sym["exported"].(bool)

		exportedStr := ""
		if exported {
			exportedStr = " (exported)"
		}

		sb.WriteString(fmt.Sprintf("%d. **%s** [%s]%s\n", i+1, name, symType, exportedStr))
		if file != "" {
			if line > 0 {
				sb.WriteString(fmt.Sprintf("   Location: %s:%d\n", file, int(line)))
			} else {
				sb.WriteString(fmt.Sprintf("   File: %s\n", file))
			}
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// memory_ingest_trigger - Trigger a background ingestion job
func registerMemoryIngestTriggerTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_ingest_trigger",
		mcp.WithDescription(`Trigger a background ingestion job to import code from a directory.
The job runs asynchronously. Use memory_ingest_status to check progress.
Use this when you want to import or re-import a codebase into memory.`),
		mcp.WithString("source_path",
			mcp.Required(),
			mcp.Description("Path to the source directory to ingest")),
		mcp.WithString("mode",
			mcp.Description("Ingestion mode: 'full' (re-ingest everything) or 'incremental' (only changed files). Default: 'full'")),
		mcp.WithString("include_pattern",
			mcp.Description("Glob pattern for files to include (e.g., '**/*.go')")),
		mcp.WithString("exclude_pattern",
			mcp.Description("Glob pattern for files to exclude (e.g., '**/vendor/**')")),
	)

	s.AddTool(tool, memoryIngestTriggerHandler)
}

func memoryIngestTriggerHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments

	sourcePath, _ := args["source_path"].(string)
	if sourcePath == "" {
		return newToolResultError("source_path is required"), nil
	}

	mode, _ := args["mode"].(string)
	if mode == "" {
		mode = "full"
	}

	ingestReq := map[string]any{
		"space_id":    defaultSpaceID,
		"source_path": sourcePath,
		"mode":        mode,
	}

	if include, ok := args["include_pattern"].(string); ok && include != "" {
		ingestReq["include_pattern"] = include
	}
	if exclude, ok := args["exclude_pattern"].(string); ok && exclude != "" {
		ingestReq["exclude_pattern"] = exclude
	}

	resp, err := callMDEMG("/v1/memory/ingest/trigger", ingestReq)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to trigger ingestion: %v", err)), nil
	}

	jobID, _ := resp["job_id"].(string)
	status, _ := resp["status"].(string)

	result := fmt.Sprintf("Ingestion job started.\nJob ID: %s\nStatus: %s\n\nUse memory_ingest_status with job_id '%s' to check progress.", jobID, status, jobID)
	return mcp.NewToolResultText(result), nil
}

// memory_ingest_status - Check the status of an ingestion job
func registerMemoryIngestStatusTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_ingest_status",
		mcp.WithDescription(`Check the status and progress of an ingestion job.
Returns the current status, progress percentage, and any errors.`),
		mcp.WithString("job_id",
			mcp.Required(),
			mcp.Description("The job ID returned by memory_ingest_trigger")),
	)

	s.AddTool(tool, memoryIngestStatusHandler)
}

func memoryIngestStatusHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments

	jobID, _ := args["job_id"].(string)
	if jobID == "" {
		return newToolResultError("job_id is required"), nil
	}

	resp, err := callMDEMGGet("/v1/memory/ingest/status/"+jobID, nil)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get job status: %v", err)), nil
	}

	status, _ := resp["status"].(string)
	progress, _ := resp["progress"].(map[string]any)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Job ID: %s\n", jobID))
	sb.WriteString(fmt.Sprintf("Status: %s\n", status))

	if progress != nil {
		current, _ := progress["current"].(float64)
		total, _ := progress["total"].(float64)
		percentage, _ := progress["percentage"].(float64)
		rate, _ := progress["rate"].(float64)

		if total > 0 {
			sb.WriteString(fmt.Sprintf("Progress: %d/%d (%.1f%%)\n", int(current), int(total), percentage))
		}
		if rate > 0 {
			sb.WriteString(fmt.Sprintf("Rate: %.1f items/sec\n", rate))
		}
	}

	if errMsg, ok := resp["error"].(string); ok && errMsg != "" {
		sb.WriteString(fmt.Sprintf("\nError: %s\n", errMsg))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// memory_ingest_cancel - Cancel a running ingestion job
func registerMemoryIngestCancelTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_ingest_cancel",
		mcp.WithDescription(`Cancel a running ingestion job. The job will be stopped as soon as possible.`),
		mcp.WithString("job_id",
			mcp.Required(),
			mcp.Description("The job ID to cancel")),
	)

	s.AddTool(tool, memoryIngestCancelHandler)
}

func memoryIngestCancelHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments

	jobID, _ := args["job_id"].(string)
	if jobID == "" {
		return newToolResultError("job_id is required"), nil
	}

	resp, err := callMDEMG("/v1/memory/ingest/cancel/"+jobID, map[string]any{})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to cancel job: %v", err)), nil
	}

	status, _ := resp["status"].(string)
	result := fmt.Sprintf("Job %s cancellation requested.\nNew status: %s", jobID, status)
	return mcp.NewToolResultText(result), nil
}

// memory_ingest_jobs - List all ingestion jobs
func registerMemoryIngestJobsTool(s *server.MCPServer) {
	tool := mcp.NewTool("memory_ingest_jobs",
		mcp.WithDescription(`List all ingestion jobs with their current status.
Shows pending, running, completed, and failed jobs.`),
	)

	s.AddTool(tool, memoryIngestJobsHandler)
}

func memoryIngestJobsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := callMDEMGGet("/v1/memory/ingest/jobs", nil)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to list jobs: %v", err)), nil
	}

	jobs, _ := resp["jobs"].([]any)
	if len(jobs) == 0 {
		return mcp.NewToolResultText("No ingestion jobs found."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Ingestion Jobs (%d):\n\n", len(jobs)))

	for _, j := range jobs {
		job, _ := j.(map[string]any)
		jobID, _ := job["job_id"].(string)
		status, _ := job["status"].(string)
		jobType, _ := job["type"].(string)
		createdAt, _ := job["created_at"].(string)

		sb.WriteString(fmt.Sprintf("- **%s** [%s]\n", jobID, status))
		sb.WriteString(fmt.Sprintf("  Type: %s\n", jobType))
		sb.WriteString(fmt.Sprintf("  Created: %s\n", createdAt))

		if progress, ok := job["progress"].(map[string]any); ok {
			if pct, ok := progress["percentage"].(float64); ok {
				sb.WriteString(fmt.Sprintf("  Progress: %.1f%%\n", pct))
			}
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// Helper function to create error result
func newToolResultError(errMsg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(errMsg)},
		IsError: true,
	}
}

// Helper function to call MDEMG API
func callMDEMG(path string, body map[string]any) (map[string]any, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", mdemgEndpoint+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if errMsg, ok := result["error"].(string); ok {
		return nil, fmt.Errorf("MDEMG error: %s", errMsg)
	}

	return result, nil
}

// Helper function to call MDEMG API with GET request
func callMDEMGGet(path string, params map[string]string) (map[string]any, error) {
	req, err := http.NewRequest("GET", mdemgEndpoint+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if params != nil {
		q := req.URL.Query()
		for k, v := range params {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if errMsg, ok := result["error"].(string); ok {
		return nil, fmt.Errorf("MDEMG error: %s", errMsg)
	}

	return result, nil
}
