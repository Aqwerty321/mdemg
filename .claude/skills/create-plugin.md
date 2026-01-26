# Skill: create-plugin

## Metadata

- **Name**: create-plugin
- **Description**: Create MDEMG sidecar plugins using the SDK
- **Triggers**: When user asks to extend MDEMG, add data source, customize retrieval, create background tasks, or mentions creating a plugin/module

## Overview

This skill guides the creation of MDEMG plugins (modules) - standalone executables that extend MDEMG functionality via gRPC over Unix domain sockets. Plugins run as separate processes, enabling fault isolation and hot reloading.

## Decision Framework

### When to Create an INGESTION Plugin

Create an INGESTION plugin when the user wants to:
- Add a new data source (Slack, Notion, Linear, Confluence, etc.)
- Parse custom file formats
- Sync content from external APIs
- Ingest webhooks or event streams
- Import from databases or data warehouses

**Key indicators in user requests:**
- "ingest my [X] data"
- "sync from [service]"
- "import [files/data]"
- "connect to [API/service]"
- "parse [format] files"

### When to Create a REASONING Plugin

Create a REASONING plugin when the user wants to:
- Improve retrieval quality for specific domains
- Boost certain types of results (recency, keywords, entities)
- Filter results based on custom criteria
- Re-rank results using ML models or heuristics
- Add context-aware scoring

**Key indicators in user requests:**
- "boost results that [condition]"
- "prioritize [type] of content"
- "filter out [type] from results"
- "re-rank based on [criteria]"
- "retrieval quality is poor for [domain]"
- "results should favor [criteria]"

### When to Create an APE Plugin

Create an APE (Active Participant Engine) plugin when the user wants to:
- Run scheduled maintenance tasks
- Perform background processing after events
- Clean up stale data periodically
- Generate summaries or insights automatically
- Detect patterns across observations

**Key indicators in user requests:**
- "run every [time period]"
- "after each session, [do something]"
- "nightly cleanup"
- "periodic [task]"
- "automatically [action] when [event]"
- "scheduled [task]"

---

## Step-by-Step Workflow

### Phase 1: Requirements Gathering

1. **Identify the need** from user request
2. **Determine plugin type** using the decision framework above
3. **Clarify requirements:**
   - For INGESTION: What data source? Authentication needed? Incremental sync required?
   - For REASONING: What scoring logic? What inputs affect ranking?
   - For APE: What schedule/triggers? What actions to perform?

### Phase 2: Design

4. **Design the plugin:**
   - Choose a descriptive `id` (lowercase, hyphens allowed, e.g., `notion-ingestion`, `keyword-booster`)
   - Define capabilities (sources, content types, triggers)
   - Identify configuration needs (API keys, thresholds, options)
   - Plan the core logic

### Phase 3: Implementation

5. **Create directory structure:**
   ```bash
   mkdir -p plugins/{plugin-id}
   ```

6. **Write manifest.json** (see templates below)

7. **Implement main.go:**
   - Set up gRPC server on Unix socket
   - Register required services (Lifecycle + type-specific service)
   - Implement all required RPCs
   - Handle graceful shutdown

### Phase 4: Build and Test

8. **Build the binary:**
   ```bash
   cd plugins/{plugin-id}
   go build -o {plugin-id} .
   ```

9. **Test standalone:**
   ```bash
   ./{plugin-id} --socket /tmp/test-{plugin-id}.sock
   # In another terminal, verify socket exists:
   ls -la /tmp/test-{plugin-id}.sock
   ```

### Phase 5: Deploy

10. **Deploy to plugins/ directory:**
    - Ensure binary is executable: `chmod +x {plugin-id}`
    - Verify manifest.json is valid JSON
    - MDEMG auto-discovers on startup

11. **Verify integration:**
    - Check module status: `curl http://localhost:8080/api/v1/modules`
    - Review logs for handshake success
    - Test the specific functionality

---

## Code Templates

### manifest.json Template

```json
{
  "id": "{{PLUGIN_ID}}",
  "name": "{{PLUGIN_NAME}}",
  "version": "1.0.0",
  "type": "{{INGESTION|REASONING|APE}}",
  "binary": "{{PLUGIN_ID}}",
  "capabilities": {
    {{#if INGESTION}}
    "ingestion_sources": ["{{scheme}}://"],
    "content_types": ["{{mime/type}}"]
    {{/if}}
    {{#if REASONING}}
    "pattern_detectors": ["{{detector_name}}"]
    {{/if}}
    {{#if APE}}
    "event_triggers": ["{{event_name}}"]
    {{/if}}
  },
  "health_check_interval_ms": 5000,
  "startup_timeout_ms": 10000,
  "config": {
    "{{key}}": "{{value}}"
  }
}
```

### INGESTION Plugin Template

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	pb "mdemg/api/modulepb"
)

const (
	moduleID      = "{{PLUGIN_ID}}"
	moduleVersion = "1.0.0"
)

var requestCounter uint64

func main() {
	socketPath := flag.String("socket", "", "Unix socket path")
	flag.Parse()

	if *socketPath == "" {
		log.Fatal("--socket is required")
	}

	os.Remove(*socketPath)

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	server := grpc.NewServer()
	module := &IngestionModule{startTime: time.Now()}
	pb.RegisterModuleLifecycleServer(server, module)
	pb.RegisterIngestionModuleServer(server, module)

	log.Printf("%s: listening on %s", moduleID, *socketPath)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Printf("%s: shutting down", moduleID)
		server.GracefulStop()
	}()

	if err := server.Serve(listener); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

type IngestionModule struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedIngestionModuleServer
	startTime time.Time
	config    map[string]string
}

// ============ Lifecycle RPCs (REQUIRED) ============

func (m *IngestionModule) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)
	m.config = req.Config

	// TODO: Validate required configuration
	// if apiKey := os.Getenv(m.config["api_key_env"]); apiKey == "" {
	//     return &pb.HandshakeResponse{Ready: false, Error: "API key not set"}, nil
	// }

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_INGESTION,
		Capabilities:  []string{"{{scheme}}://", "{{content_type}}"},
		Ready:         true,
	}, nil
}

func (m *IngestionModule) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ok",
		Metrics: map[string]string{
			"uptime":           time.Since(m.startTime).String(),
			"requests_handled": fmt.Sprintf("%d", atomic.LoadUint64(&requestCounter)),
		},
	}, nil
}

func (m *IngestionModule) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{Success: true, Message: "goodbye"}, nil
}

// ============ Ingestion RPCs (REQUIRED) ============

func (m *IngestionModule) Matches(ctx context.Context, req *pb.MatchRequest) (*pb.MatchResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Implement matching logic
	matches := strings.HasPrefix(req.SourceUri, "{{scheme}}://") ||
		req.ContentType == "{{content_type}}"

	confidence := float32(0.0)
	reason := "not a supported source"
	if matches {
		confidence = 1.0
		reason = "matches {{scheme}}:// or {{content_type}}"
	}

	return &pb.MatchResponse{
		Matches:    matches,
		Confidence: confidence,
		Reason:     reason,
	}, nil
}

func (m *IngestionModule) Parse(ctx context.Context, req *pb.ParseRequest) (*pb.ParseResponse, error) {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Implement parsing logic - convert raw content to observations
	obs := &pb.Observation{
		NodeId:      fmt.Sprintf("%s-%d", moduleID, time.Now().UnixNano()),
		Path:        req.SourceUri,
		Name:        "Parsed Item", // TODO: Extract meaningful name
		Content:     string(req.Content),
		ContentType: req.ContentType,
		Tags:        []string{"{{tag}}"},
		Metadata:    map[string]string{"source_uri": req.SourceUri},
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      moduleID,
	}

	return &pb.ParseResponse{
		Observations: []*pb.Observation{obs},
		Metadata: map[string]string{
			"parsed_at": time.Now().Format(time.RFC3339),
		},
	}, nil
}

func (m *IngestionModule) Sync(req *pb.SyncRequest, stream pb.IngestionModule_SyncServer) error {
	atomic.AddUint64(&requestCounter, 1)

	// TODO: Implement incremental sync
	// 1. Use req.Cursor to resume from last position (empty = full sync)
	// 2. Fetch items from external source in batches
	// 3. Convert to observations and send via stream
	// 4. Return cursor for next sync

	// Example single observation:
	obs := &pb.Observation{
		NodeId:      fmt.Sprintf("%s-sync-%d", moduleID, time.Now().UnixNano()),
		Path:        "{{scheme}}://sync",
		Name:        "Synced Item",
		Content:     "synced content",
		ContentType: "text/plain",
		Tags:        []string{"{{tag}}", "sync"},
		Timestamp:   time.Now().Format(time.RFC3339),
		Source:      moduleID,
	}

	return stream.Send(&pb.SyncResponse{
		Observations: []*pb.Observation{obs},
		Cursor:       "cursor-1",
		HasMore:      false,
		Stats: &pb.SyncStats{
			ItemsProcessed: 1,
			ItemsCreated:   1,
		},
	})
}
```

### REASONING Plugin Template

```go
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	pb "mdemg/api/modulepb"
)

const (
	moduleID      = "{{PLUGIN_ID}}"
	moduleVersion = "1.0.0"
)

func main() {
	socketPath := flag.String("socket", "", "Unix socket path")
	flag.Parse()

	if *socketPath == "" {
		log.Fatal("--socket is required")
	}

	os.Remove(*socketPath)

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	log.Printf("%s: listening on %s", moduleID, *socketPath)

	grpcServer := grpc.NewServer()
	s := &ReasoningServer{
		startTime:   time.Now(),
		boostFactor: 0.2, // TODO: Configure
	}

	pb.RegisterModuleLifecycleServer(grpcServer, s)
	pb.RegisterReasoningModuleServer(grpcServer, s)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		log.Printf("%s: shutting down", moduleID)
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

type ReasoningServer struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedReasoningModuleServer

	mu              sync.Mutex
	startTime       time.Time
	requestsHandled int64
	boostFactor     float64
}

// ============ Lifecycle RPCs (REQUIRED) ============

func (s *ReasoningServer) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	// Parse configuration
	if factor, ok := req.Config["boost_factor"]; ok {
		if f, err := strconv.ParseFloat(factor, 64); err == nil {
			s.boostFactor = f
		}
	}

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_REASONING,
		Capabilities:  []string{"{{capability}}"},
		Ready:         true,
	}, nil
}

func (s *ReasoningServer) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	s.mu.Lock()
	requests := s.requestsHandled
	s.mu.Unlock()

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ready",
		Metrics: map[string]string{
			"uptime":           time.Since(s.startTime).String(),
			"requests_handled": strconv.FormatInt(requests, 10),
			"boost_factor":     strconv.FormatFloat(s.boostFactor, 'f', 2, 64),
		},
	}, nil
}

func (s *ReasoningServer) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{Success: true, Message: "goodbye"}, nil
}

// ============ Reasoning RPC (REQUIRED) ============

func (s *ReasoningServer) Process(ctx context.Context, req *pb.ProcessRequest) (*pb.ProcessResponse, error) {
	s.mu.Lock()
	s.requestsHandled++
	s.mu.Unlock()

	if len(req.Candidates) == 0 {
		return &pb.ProcessResponse{Results: req.Candidates}, nil
	}

	log.Printf("%s: processing %d candidates for: %s",
		moduleID, len(req.Candidates), truncate(req.QueryText, 50))

	// TODO: Implement your scoring/ranking logic
	// Example: keyword boost
	keywords := extractKeywords(req.QueryText)

	type scored struct {
		candidate *pb.RetrievalCandidate
		boost     float64
	}

	results := make([]scored, len(req.Candidates))
	for i, c := range req.Candidates {
		boost := calculateBoost(c.Name, c.Summary, keywords) * s.boostFactor
		c.Score = c.Score + float32(boost)
		results[i] = scored{candidate: c, boost: boost}
	}

	// Sort by score (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].candidate.Score > results[j].candidate.Score
	})

	// Build output
	output := make([]*pb.RetrievalCandidate, len(results))
	for i, r := range results {
		output[i] = r.candidate
	}

	// Apply top_k limit
	if req.TopK > 0 && int(req.TopK) < len(output) {
		output = output[:req.TopK]
	}

	return &pb.ProcessResponse{
		Results: output,
		Metadata: map[string]string{
			"keywords":     strings.Join(keywords, ","),
			"boost_factor": strconv.FormatFloat(s.boostFactor, 'f', 2, 64),
		},
	}, nil
}

// Helper functions - customize as needed

func extractKeywords(query string) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true,
		"of": true, "at": true, "by": true, "for": true, "with": true,
		"to": true, "from": true, "in": true, "out": true, "on": true,
		"and": true, "or": true, "but": true, "if": true, "what": true,
		"this": true, "that": true, "how": true, "why": true, "where": true,
	}

	words := strings.Fields(strings.ToLower(query))
	var keywords []string
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:\"'()[]{}/-")
		if len(word) >= 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}
	return keywords
}

func calculateBoost(name, summary string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	combined := strings.ToLower(name + " " + summary)
	matchCount := 0

	for _, kw := range keywords {
		if strings.Contains(combined, kw) {
			matchCount++
		}
	}

	return float64(matchCount) / float64(len(keywords))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
```

### APE Plugin Template

```go
package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	pb "mdemg/api/modulepb"
)

const (
	moduleID      = "{{PLUGIN_ID}}"
	moduleVersion = "1.0.0"
)

func main() {
	socketPath := flag.String("socket", "", "Unix socket path")
	flag.Parse()

	if *socketPath == "" {
		log.Fatal("--socket is required")
	}

	os.Remove(*socketPath)

	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	log.Printf("%s: listening on %s", moduleID, *socketPath)

	grpcServer := grpc.NewServer()
	s := &APEServer{startTime: time.Now()}

	pb.RegisterModuleLifecycleServer(grpcServer, s)
	pb.RegisterAPEModuleServer(grpcServer, s)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		log.Printf("%s: shutting down", moduleID)
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

type APEServer struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedAPEModuleServer

	mu              sync.Mutex
	startTime       time.Time
	executionsTotal int64
	lastExecution   time.Time
}

// ============ Lifecycle RPCs (REQUIRED) ============

func (s *APEServer) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_APE,
		Capabilities:  []string{"{{capability}}"},
		Ready:         true,
	}, nil
}

func (s *APEServer) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	s.mu.Lock()
	executions := s.executionsTotal
	lastExec := s.lastExecution
	s.mu.Unlock()

	metrics := map[string]string{
		"uptime":           time.Since(s.startTime).String(),
		"executions_total": strconv.FormatInt(executions, 10),
	}
	if !lastExec.IsZero() {
		metrics["last_execution"] = lastExec.Format(time.RFC3339)
	}

	return &pb.HealthCheckResponse{
		Healthy: true,
		Status:  "ready",
		Metrics: metrics,
	}, nil
}

func (s *APEServer) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{Success: true, Message: "goodbye"}, nil
}

// ============ APE RPCs (REQUIRED) ============

func (s *APEServer) GetSchedule(ctx context.Context, req *pb.GetScheduleRequest) (*pb.GetScheduleResponse, error) {
	return &pb.GetScheduleResponse{
		// TODO: Configure schedule
		CronExpression:     "{{cron_expression}}", // e.g., "0 * * * *" = hourly
		EventTriggers:      []string{"{{event}}"}, // e.g., "session_end", "ingest"
		MinIntervalSeconds: 300,                   // Don't run more often than 5 min
	}, nil
}

func (s *APEServer) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	start := time.Now()

	s.mu.Lock()
	s.executionsTotal++
	s.lastExecution = start
	execNum := s.executionsTotal
	s.mu.Unlock()

	log.Printf("%s: executing task %s (trigger=%s, execution #%d)",
		moduleID, req.TaskId, req.Trigger, execNum)

	// TODO: Implement your task logic based on trigger type
	var message string
	var stats *pb.ExecuteStats

	switch req.Trigger {
	case "schedule":
		// Periodic scheduled task
		message, stats = s.handleScheduled(ctx, req)

	case "event:session_end":
		// After session ends
		message, stats = s.handleSessionEnd(ctx, req)

	case "event:ingest":
		// After content ingestion
		message, stats = s.handleIngest(ctx, req)

	case "event:consolidate":
		// After consolidation
		message, stats = s.handleConsolidate(ctx, req)

	default:
		message = "Unknown trigger, no action taken"
		stats = &pb.ExecuteStats{}
	}

	stats.DurationMs = time.Since(start).Milliseconds()

	log.Printf("%s: task %s completed in %v", moduleID, req.TaskId, time.Since(start))

	return &pb.ExecuteResponse{
		Success: true,
		Message: message,
		Stats:   stats,
	}, nil
}

// Task handlers - implement your logic

func (s *APEServer) handleScheduled(ctx context.Context, req *pb.ExecuteRequest) (string, *pb.ExecuteStats) {
	// TODO: Implement periodic maintenance logic
	return "Scheduled task completed", &pb.ExecuteStats{
		NodesCreated: 0,
		NodesUpdated: 0,
		EdgesCreated: 0,
	}
}

func (s *APEServer) handleSessionEnd(ctx context.Context, req *pb.ExecuteRequest) (string, *pb.ExecuteStats) {
	// TODO: Implement session reflection/summarization
	return "Session end processing completed", &pb.ExecuteStats{}
}

func (s *APEServer) handleIngest(ctx context.Context, req *pb.ExecuteRequest) (string, *pb.ExecuteStats) {
	// TODO: Implement post-ingest processing
	return "Post-ingest processing completed", &pb.ExecuteStats{}
}

func (s *APEServer) handleConsolidate(ctx context.Context, req *pb.ExecuteRequest) (string, *pb.ExecuteStats) {
	// TODO: Implement post-consolidation processing
	return "Post-consolidation processing completed", &pb.ExecuteStats{}
}
```

---

## Validation Checklist

Before considering the plugin complete, verify:

### manifest.json Validation
- [ ] `id` is lowercase with hyphens only, unique
- [ ] `type` is exactly `INGESTION`, `REASONING`, or `APE`
- [ ] `binary` matches the actual binary filename
- [ ] JSON is valid (no trailing commas, proper quoting)
- [ ] `capabilities` appropriate for the type

### Build Validation
- [ ] `go build` succeeds without errors
- [ ] Binary is created with correct name
- [ ] Binary is executable (`chmod +x`)

### Runtime Validation
- [ ] Plugin starts with `--socket /tmp/test.sock`
- [ ] Socket file is created at specified path
- [ ] No startup errors in logs

### Integration Validation
- [ ] Handshake succeeds (`Ready: true`)
- [ ] Health checks return `Healthy: true`
- [ ] Type-specific RPCs work correctly:
  - INGESTION: `Matches`, `Parse`, `Sync`
  - REASONING: `Process`
  - APE: `GetSchedule`, `Execute`
- [ ] Graceful shutdown works (responds to SIGTERM)

### Testing Commands
```bash
# Build
cd plugins/{{plugin-id}}
go build -o {{plugin-id}} .

# Test standalone
./{{plugin-id}} --socket /tmp/test-{{plugin-id}}.sock &
ls -la /tmp/test-{{plugin-id}}.sock

# Test with grpcurl (if available)
grpcurl -plaintext -unix /tmp/test-{{plugin-id}}.sock \
    mdemg.module.v1.ModuleLifecycle/HealthCheck

# Cleanup
kill %1
rm /tmp/test-{{plugin-id}}.sock

# Check module status after MDEMG restart
curl http://localhost:8080/api/v1/modules
```

---

## Example Invocations

### Example 1: "Create a plugin to ingest my Notion pages"

**Type**: INGESTION

**Implementation steps**:
1. Create `plugins/notion-ingestion/`
2. manifest.json with `type: "INGESTION"`, `ingestion_sources: ["notion://"]`
3. Implement `Matches` to detect `notion://` URIs
4. Implement `Parse` to convert Notion page JSON to observations
5. Implement `Sync` to use Notion API with pagination
6. Configure API key via `config.api_key_env`

### Example 2: "Create a plugin that boosts results containing specific keywords"

**Type**: REASONING

**Implementation steps**:
1. Create `plugins/keyword-booster/`
2. manifest.json with `type: "REASONING"`, `pattern_detectors: ["keyword_boost"]`
3. Implement `Process` to:
   - Extract keywords from query
   - Calculate match score for each candidate
   - Boost scores by configured factor
   - Re-sort by new scores
4. Configure boost factor and optional keyword list in config

### Example 3: "Create a plugin that runs nightly cleanup of old observations"

**Type**: APE

**Implementation steps**:
1. Create `plugins/nightly-cleanup/`
2. manifest.json with `type: "APE"`, `event_triggers: []` (schedule only)
3. Implement `GetSchedule` to return `"0 2 * * *"` (2 AM daily)
4. Implement `Execute` to:
   - Query for observations older than threshold
   - Delete or archive stale nodes
   - Report stats on nodes removed
5. Configure retention period in config

### Example 4: "Create a plugin to sync from our internal API"

**Type**: INGESTION

**Implementation steps**:
1. Create `plugins/internal-api-sync/`
2. manifest.json with custom `ingestion_sources: ["internal-api://"]`
3. Implement `Sync` with:
   - Cursor-based pagination
   - API authentication from env var
   - Rate limiting
   - Error handling with retries
4. Map API response fields to Observation fields

### Example 5: "Improve retrieval for code-related questions"

**Type**: REASONING

**Implementation steps**:
1. Create `plugins/code-reranker/`
2. Implement `Process` to:
   - Detect code-related queries (keywords: function, class, method, etc.)
   - Boost candidates with `content_type` containing "code"
   - Boost candidates matching file extensions (.py, .go, .js)
   - Apply language-specific keyword matching

---

## Troubleshooting Reference

### "binary not found"
- Verify binary exists in plugin directory
- Verify `manifest.binary` matches filename exactly
- Run `chmod +x` on the binary

### "handshake failed: connection refused"
- Increase `startup_timeout_ms` in manifest
- Check plugin logs for startup errors
- Test plugin manually: `./plugin --socket /tmp/test.sock`

### "health check failed"
- HealthCheck must return within 2 seconds
- Avoid blocking operations in HealthCheck
- Check for deadlocks

### "module not ready"
- Check handshake response error message
- Verify required configuration is present
- Check environment variables for API keys

### Module keeps restarting
- Add panic recovery in main()
- Check stderr for panic messages
- MDEMG retries 3 times with backoff (2s, 4s, 6s)

---

## Reference

Full SDK documentation: `/Users/reh3376/mdemg/docs/SDK_PLUGIN_GUIDE.md`
