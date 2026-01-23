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

const moduleID = "reflection-module"
const moduleVersion = "1.0.0"

type server struct {
	pb.UnimplementedModuleLifecycleServer
	pb.UnimplementedAPEModuleServer

	mu              sync.Mutex
	startTime       time.Time
	executionsTotal int64
	lastExecution   time.Time
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

	// Create gRPC server
	grpcServer := grpc.NewServer()
	s := &server{
		startTime: time.Now(),
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
		Capabilities:  []string{"reflection", "session_summary"},
		Ready:         true,
	}, nil
}

// HealthCheck implements ModuleLifecycle.HealthCheck
func (s *server) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	s.mu.Lock()
	executions := s.executionsTotal
	lastExec := s.lastExecution
	s.mu.Unlock()

	uptime := time.Since(s.startTime).String()

	metrics := map[string]string{
		"uptime":           uptime,
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
		CronExpression:     "0 * * * *", // Every hour
		EventTriggers:      []string{"session_end", "consolidate"},
		MinIntervalSeconds: 300, // Minimum 5 minutes between runs
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

	// Simulate some work
	// In a real implementation, this would:
	// 1. Query MDEMG for recent observations
	// 2. Analyze patterns and generate reflections
	// 3. Create summary nodes or update existing ones

	var message string
	var stats *pb.ExecuteStats

	switch req.Trigger {
	case "event:session_end":
		message = "Session reflection completed - analyzed recent activity"
		stats = &pb.ExecuteStats{
			NodesCreated: 0, // Would create reflection nodes
			NodesUpdated: 0,
			EdgesCreated: 0,
			EdgesUpdated: 0,
			DurationMs:   time.Since(start).Milliseconds(),
		}

	case "event:consolidate":
		message = "Post-consolidation reflection completed"
		stats = &pb.ExecuteStats{
			NodesCreated: 0,
			NodesUpdated: 0,
			EdgesCreated: 0,
			EdgesUpdated: 0,
			DurationMs:   time.Since(start).Milliseconds(),
		}

	case "schedule":
		message = "Scheduled reflection completed - periodic health check"
		stats = &pb.ExecuteStats{
			NodesCreated: 0,
			NodesUpdated: 0,
			EdgesCreated: 0,
			EdgesUpdated: 0,
			DurationMs:   time.Since(start).Milliseconds(),
		}

	default:
		message = "Generic execution completed"
		stats = &pb.ExecuteStats{
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	log.Printf("%s: task %s completed in %v", moduleID, req.TaskId, time.Since(start))

	return &pb.ExecuteResponse{
		Success: true,
		Message: message,
		Stats:   stats,
	}, nil
}
