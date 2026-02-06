package devspace

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	pb "mdemg/api/devspacepb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestDevSpaceServerInProcess verifies DevSpace RPCs (RegisterAgent, ListExports, Connect)
// without requiring UDTS_TARGET or Neo4j. Confirms proper response and function at a minimum.
func TestDevSpaceServerInProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Catalog with temp dir; broker for Connect
	catalog, err := NewCatalog(t.TempDir())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	broker := NewBroker()
	srv := NewServer(catalog, broker)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	pb.RegisterDevSpaceServer(grpcServer, srv)
	go func() { _ = grpcServer.Serve(lis) }()
	defer grpcServer.GracefulStop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewDevSpaceClient(conn)

	// RegisterAgent: must return Ok
	t.Run("RegisterAgent", func(t *testing.T) {
		resp, err := client.RegisterAgent(ctx, &pb.RegisterAgentRequest{
			DevSpaceId: "test-space",
			AgentId:    "test-agent",
		})
		if err != nil {
			t.Fatalf("RegisterAgent: %v", err)
		}
		if !resp.Ok {
			t.Errorf("RegisterAgent: expected ok=true, got %v", resp.Ok)
		}
	})

	// ListExports: must return OK; exports may be nil or empty for new space (proto3)
	t.Run("ListExports", func(t *testing.T) {
		resp, err := client.ListExports(ctx, &pb.ListExportsRequest{DevSpaceId: "test-space"})
		if err != nil {
			t.Fatalf("ListExports: %v", err)
		}
		if resp != nil && len(resp.Exports) != 0 {
			t.Errorf("ListExports: expected 0 exports for new space, got %d", len(resp.Exports))
		}
	})

	// Connect: open stream, send one message with dev_space_id and agent_id, close send, recv until EOF
	t.Run("Connect", func(t *testing.T) {
		stream, err := client.Connect(ctx)
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		if err := stream.Send(&pb.AgentMessage{
			DevSpaceId: "test-space",
			AgentId:    "test-agent",
		}); err != nil {
			t.Fatalf("Connect Send: %v", err)
		}
		if err := stream.CloseSend(); err != nil {
			t.Fatalf("Connect CloseSend: %v", err)
		}
		for {
			_, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Connect Recv: %v", err)
			}
		}
	})

	// Connect validation: missing dev_space_id or agent_id must fail with InvalidArgument
	t.Run("Connect_invalid_first_message", func(t *testing.T) {
		stream, err := client.Connect(ctx)
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		if err := stream.Send(&pb.AgentMessage{DevSpaceId: "x", AgentId: ""}); err != nil {
			t.Fatalf("Send: %v", err)
		}
		_ = stream.CloseSend()
		_, err = stream.Recv()
		if err == nil {
			t.Error("expected error for missing agent_id")
		}
	})
}

// TestDevSpaceTwoAgentMessaging verifies that two agents in the same DevSpace can exchange messages via the hub.
func TestDevSpaceTwoAgentMessaging(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	catalog, err := NewCatalog(t.TempDir())
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	broker := NewBroker()
	srv := NewServer(catalog, broker)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	pb.RegisterDevSpaceServer(grpcServer, srv)
	go func() { _ = grpcServer.Serve(lis) }()
	defer grpcServer.GracefulStop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewDevSpaceClient(conn)

	// Agent A: connect and subscribe (no payload in first message)
	streamA, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Agent A Connect: %v", err)
	}
	if err := streamA.Send(&pb.AgentMessage{DevSpaceId: "exchange-space", AgentId: "agent-a"}); err != nil {
		t.Fatalf("Agent A first send: %v", err)
	}

	// Agent B: connect and subscribe
	streamB, err := client.Connect(ctx)
	if err != nil {
		t.Fatalf("Agent B Connect: %v", err)
	}
	if err := streamB.Send(&pb.AgentMessage{DevSpaceId: "exchange-space", AgentId: "agent-b"}); err != nil {
		t.Fatalf("Agent B first send: %v", err)
	}

	// Start B's Recv in background so server can deliver (avoids flow-control deadlock)
	recvDone := make(chan *pb.AgentMessage, 1)
	go func() {
		msg, err := streamB.Recv()
		if err != nil {
			return
		}
		recvDone <- msg
	}()

	// Small delay to ensure both agents are subscribed before sending
	time.Sleep(100 * time.Millisecond)

	// Agent A sends a message (will be delivered to B, not back to A)
	payload := []byte("hello from A")
	if err := streamA.Send(&pb.AgentMessage{
		DevSpaceId: "exchange-space",
		AgentId:    "agent-a",
		Topic:      "test",
		Payload:    payload,
	}); err != nil {
		t.Fatalf("Agent A send payload: %v", err)
	}

	// Agent B must receive the message (from A)
	// Use 5s timeout to handle CI environments with resource contention
	var received *pb.AgentMessage
	select {
	case received = <-recvDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Agent B did not receive message within 5s")
	}
	if received.AgentId != "agent-a" || string(received.Payload) != string(payload) {
		t.Errorf("Agent B received wrong message: AgentId=%q Payload=%q", received.AgentId, received.Payload)
	}

	// Close both streams and drain
	_ = streamA.CloseSend()
	_ = streamB.CloseSend()
	for {
		_, err := streamA.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Agent A Recv: %v", err)
		}
	}
	for {
		_, err := streamB.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Agent B Recv: %v", err)
		}
	}
}
