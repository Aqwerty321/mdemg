package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"

	pb "mdemg/api/modulepb"
)

const moduleID = "context-cooler"
const moduleVersion = "1.0.0"

// Default configuration
const (
	defaultMDEMGEndpoint  = "http://localhost:9999"
	defaultCronExpression = "*/30 * * * *" // Every 30 minutes
	defaultMinInterval    = 300            // 5 minutes
)

type server struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedAPEModuleServer

	mu              sync.Mutex
	startTime       time.Time
	executionsTotal int64
	lastExecution   time.Time
	lastGraduated   int64
	lastTombstoned  int64
	httpClient      *http.Client
	mdemgEndpoint   string
}

// GraduateRequest is the request body for /v1/conversation/graduate
type GraduateRequest struct {
	SpaceID string `json:"space_id"`
}

// GraduateSummary matches the response from /v1/conversation/graduate
type GraduateSummary struct {
	SpaceID           string    `json:"space_id"`
	Timestamp         time.Time `json:"timestamp"`
	Graduated         int       `json:"graduated"`
	Tombstoned        int       `json:"tombstoned"`
	RemainingVolatile int       `json:"remaining_volatile"`
	DecayApplied      int       `json:"decay_applied"`
}

func main() {
	socketPath := flag.String("socket", "", "Unix socket path")
	flag.Parse()

	if *socketPath == "" {
		log.Fatal("--socket flag is required")
	}

	// Remove stale socket
	os.Remove(*socketPath)

	// Create Unix socket listener
	listener, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on socket: %v", err)
	}
	defer listener.Close()
	defer os.Remove(*socketPath)

	log.Printf("%s: listening on %s", moduleID, *socketPath)

	// Get MDEMG endpoint from environment or use default
	mdemgEndpoint := os.Getenv("MDEMG_ENDPOINT")
	if mdemgEndpoint == "" {
		mdemgEndpoint = defaultMDEMGEndpoint
	}

	// Create gRPC server
	grpcServer := grpc.NewServer()
	s := &server{
		startTime:     time.Now(),
		mdemgEndpoint: mdemgEndpoint,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	pb.RegisterModuleLifecycleServer(grpcServer, s)
	pb.RegisterAPEModuleServer(grpcServer, s)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-sigChan
		log.Printf("%s: received shutdown signal", moduleID)
		grpcServer.GracefulStop()
	}()

	// Start serving
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

// Handshake implements ModuleLifecycle.Handshake
func (s *server) Handshake(ctx context.Context, req *pb.HandshakeRequest) (*pb.HandshakeResponse, error) {
	log.Printf("%s: handshake from MDEMG %s", moduleID, req.MdemgVersion)

	return &pb.HandshakeResponse{
		ModuleId:      moduleID,
		ModuleVersion: moduleVersion,
		ModuleType:    pb.ModuleType_MODULE_TYPE_APE,
		Capabilities:  []string{"graduation", "volatile_memory_management", "tombstoning"},
		Ready:         true,
	}, nil
}

// HealthCheck implements ModuleLifecycle.HealthCheck
func (s *server) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	s.mu.Lock()
	executions := s.executionsTotal
	lastExec := s.lastExecution
	graduated := s.lastGraduated
	tombstoned := s.lastTombstoned
	s.mu.Unlock()

	uptime := time.Since(s.startTime).String()

	metrics := map[string]string{
		"uptime":                uptime,
		"executions_total":      strconv.FormatInt(executions, 10),
		"last_graduated_count":  strconv.FormatInt(graduated, 10),
		"last_tombstoned_count": strconv.FormatInt(tombstoned, 10),
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

// Shutdown implements ModuleLifecycle.Shutdown
func (s *server) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	log.Printf("%s: shutdown requested (reason: %s)", moduleID, req.Reason)
	return &pb.ShutdownResponse{
		Success: true,
		Message: "shutting down gracefully",
	}, nil
}

// GetSchedule implements APEModule.GetSchedule
func (s *server) GetSchedule(ctx context.Context, req *pb.GetScheduleRequest) (*pb.GetScheduleResponse, error) {
	return &pb.GetScheduleResponse{
		CronExpression:     defaultCronExpression,
		EventTriggers:      []string{"session_end", "consolidate"},
		MinIntervalSeconds: defaultMinInterval,
	}, nil
}

// Execute implements APEModule.Execute
func (s *server) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	start := time.Now()

	s.mu.Lock()
	s.executionsTotal++
	s.lastExecution = start
	execNum := s.executionsTotal
	s.mu.Unlock()

	log.Printf("%s: executing task %s (trigger=%s, execution #%d)",
		moduleID, req.TaskId, req.Trigger, execNum)

	// Get space_id from context, default to mdemg-dev (Claude's conversation memory)
	spaceID := req.Context["space_id"]
	if spaceID == "" {
		spaceID = "mdemg-dev"
	}

	// Call the graduation endpoint
	summary, err := s.processGraduation(ctx, spaceID)
	if err != nil {
		log.Printf("%s: graduation failed for space %s: %v", moduleID, spaceID, err)
		return &pb.ExecuteResponse{
			Success: false,
			Error:   fmt.Sprintf("graduation failed: %v", err),
			Stats: &pb.ExecuteStats{
				DurationMs: time.Since(start).Milliseconds(),
			},
		}, nil
	}

	// Update metrics
	s.mu.Lock()
	s.lastGraduated = int64(summary.Graduated)
	s.lastTombstoned = int64(summary.Tombstoned)
	s.mu.Unlock()

	var message string
	switch req.Trigger {
	case "event:session_end":
		message = fmt.Sprintf("Session end graduation: %d graduated, %d tombstoned, %d volatile remaining",
			summary.Graduated, summary.Tombstoned, summary.RemainingVolatile)
	case "event:consolidate":
		message = fmt.Sprintf("Post-consolidation graduation: %d graduated, %d tombstoned, %d volatile remaining",
			summary.Graduated, summary.Tombstoned, summary.RemainingVolatile)
	case "schedule":
		message = fmt.Sprintf("Scheduled graduation: %d graduated, %d tombstoned, %d volatile remaining",
			summary.Graduated, summary.Tombstoned, summary.RemainingVolatile)
	default:
		message = fmt.Sprintf("Graduation complete: %d graduated, %d tombstoned, %d volatile remaining",
			summary.Graduated, summary.Tombstoned, summary.RemainingVolatile)
	}

	log.Printf("%s: task %s completed in %v - %s", moduleID, req.TaskId, time.Since(start), message)

	return &pb.ExecuteResponse{
		Success: true,
		Message: message,
		Stats: &pb.ExecuteStats{
			NodesCreated: 0,
			NodesUpdated: int32(summary.Graduated + summary.Tombstoned),
			EdgesCreated: 0,
			EdgesUpdated: 0,
			DurationMs:   time.Since(start).Milliseconds(),
		},
	}, nil
}

// processGraduation calls the MDEMG graduation endpoint
func (s *server) processGraduation(ctx context.Context, spaceID string) (*GraduateSummary, error) {
	url := fmt.Sprintf("%s/v1/conversation/graduate", s.mdemgEndpoint)

	reqBody := GraduateRequest{SpaceID: spaceID}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("graduation failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var summary GraduateSummary
	if err := json.NewDecoder(resp.Body).Decode(&summary); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &summary, nil
}
