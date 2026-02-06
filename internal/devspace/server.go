package devspace

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "mdemg/api/devspacepb"
)

// Server implements pb.DevSpaceServer.
type Server struct {
	pb.UnimplementedDevSpaceServer
	catalog *Catalog
	broker  *Broker
}

// NewServer returns a DevSpace gRPC server. If broker is nil, Connect will return Unimplemented.
func NewServer(catalog *Catalog, broker *Broker) *Server {
	return &Server{catalog: catalog, broker: broker}
}

// RegisterAgent registers an agent in a DevSpace (idempotent).
func (s *Server) RegisterAgent(ctx context.Context, req *pb.RegisterAgentRequest) (*pb.RegisterAgentResponse, error) {
	if req.DevSpaceId == "" || req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "dev_space_id and agent_id required")
	}
	meta := make(map[string]string)
	for k, v := range req.Metadata {
		meta[k] = v
	}
	s.catalog.RegisterAgent(req.DevSpaceId, req.AgentId, meta)
	return &pb.RegisterAgentResponse{Ok: true}, nil
}

// DeregisterAgent removes an agent from a DevSpace.
func (s *Server) DeregisterAgent(ctx context.Context, req *pb.DeregisterAgentRequest) (*pb.DeregisterAgentResponse, error) {
	if req.DevSpaceId == "" || req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "dev_space_id and agent_id required")
	}
	s.catalog.DeregisterAgent(req.DevSpaceId, req.AgentId)
	return &pb.DeregisterAgentResponse{Ok: true}, nil
}

// ListExports returns the catalog of published exports.
func (s *Server) ListExports(ctx context.Context, req *pb.ListExportsRequest) (*pb.ListExportsResponse, error) {
	if req.DevSpaceId == "" {
		return nil, status.Error(codes.InvalidArgument, "dev_space_id required")
	}
	exports := s.catalog.ListExports(req.DevSpaceId)
	resp := &pb.ListExportsResponse{Exports: exports}
	// Ensure exports is never nil
	if resp.Exports == nil {
		resp.Exports = []*pb.ExportEntry{}
	}
	return resp, nil
}

// PublishExport receives a stream: first message is header, rest are file bytes.
func (s *Server) PublishExport(stream pb.DevSpace_PublishExportServer) error {
	var outPath string
	var outFile *os.File
	var exportID string

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			if outFile != nil {
				outFile.Close()
				os.Remove(outPath)
			}
			return err
		}
		switch p := chunk.Payload.(type) {
		case *pb.PublishExportChunk_Header:
			header := p.Header
			if header.DevSpaceId == "" || header.SpaceId == "" || header.PublishedByAgentId == "" {
				_ = stream.SendAndClose(&pb.PublishExportResponse{Ok: false, Error: "header: dev_space_id, space_id, published_by_agent_id required"})
				return nil
			}
			exportID, outPath = s.catalog.ReserveExport(header.DevSpaceId, header.SpaceId, header.PublishedByAgentId, header.Label)
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				_ = stream.SendAndClose(&pb.PublishExportResponse{Ok: false, Error: err.Error()})
				return nil
			}
			f, err := os.Create(outPath)
			if err != nil {
				_ = stream.SendAndClose(&pb.PublishExportResponse{Ok: false, Error: err.Error()})
				return nil
			}
			outFile = f
		case *pb.PublishExportChunk_Data:
			if outFile == nil {
				_ = stream.SendAndClose(&pb.PublishExportResponse{Ok: false, Error: "missing header"})
				return nil
			}
			if _, err := outFile.Write(p.Data); err != nil {
				outFile.Close()
				os.Remove(outPath)
				_ = stream.SendAndClose(&pb.PublishExportResponse{Ok: false, Error: err.Error()})
				return nil
			}
		}
	}

	if outFile != nil {
		if err := outFile.Close(); err != nil {
			os.Remove(outPath)
			_ = stream.SendAndClose(&pb.PublishExportResponse{Ok: false, Error: err.Error()})
			return nil
		}
	}
	return stream.SendAndClose(&pb.PublishExportResponse{Ok: true, ExportId: exportID})
}

const pullChunkSize = 64 * 1024 // 64 KiB

// PullExport streams a published export file to the caller.
func (s *Server) PullExport(req *pb.PullExportRequest, stream pb.DevSpace_PullExportServer) error {
	if req.DevSpaceId == "" || req.ExportId == "" {
		return status.Error(codes.InvalidArgument, "dev_space_id and export_id required")
	}
	filePath, err := s.catalog.GetExport(req.DevSpaceId, req.ExportId)
	if err != nil {
		return status.Error(codes.NotFound, err.Error())
	}
	f, err := os.Open(filePath)
	if err != nil {
		return status.Errorf(codes.Internal, "open export file: %v", err)
	}
	defer f.Close()
	buf := make([]byte, pullChunkSize)
	seq := int64(0)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if err := stream.Send(&pb.PullExportChunk{Data: buf[:n], Sequence: seq}); err != nil {
				return err
			}
			seq++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "read export file: %v", err)
		}
	}
	return nil
}

// =============================================================================
// Phase 37: Heartbeat / Presence
// =============================================================================

// Heartbeat updates the agent's last heartbeat time.
func (s *Server) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	if req.DevSpaceId == "" || req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "dev_space_id and agent_id required")
	}

	statusMap := make(map[string]string)
	for k, v := range req.Status {
		statusMap[k] = v
	}

	queueSize := s.catalog.UpdateHeartbeat(req.DevSpaceId, req.AgentId, statusMap)

	return &pb.HeartbeatResponse{
		Ok:         true,
		ServerTime: time.Now().UTC().Format(time.RFC3339),
		QueueSize:  int32(queueSize),
	}, nil
}

// GetPresence returns the presence status of agents in a DevSpace.
func (s *Server) GetPresence(ctx context.Context, req *pb.GetPresenceRequest) (*pb.GetPresenceResponse, error) {
	if req.DevSpaceId == "" {
		return nil, status.Error(codes.InvalidArgument, "dev_space_id required")
	}

	presenceList := s.catalog.GetPresence(req.DevSpaceId, req.AgentId)

	agents := make([]*pb.AgentPresence, len(presenceList))
	for i, p := range presenceList {
		var protoStatus pb.PresenceStatus
		switch p.Status {
		case "online":
			protoStatus = pb.PresenceStatus_PRESENCE_ONLINE
		case "away":
			protoStatus = pb.PresenceStatus_PRESENCE_AWAY
		case "offline":
			protoStatus = pb.PresenceStatus_PRESENCE_OFFLINE
		default:
			protoStatus = pb.PresenceStatus_PRESENCE_UNKNOWN
		}

		var lastHeartbeat string
		if !p.LastHeartbeat.IsZero() {
			lastHeartbeat = p.LastHeartbeat.Format(time.RFC3339)
		}

		agents[i] = &pb.AgentPresence{
			AgentId:                p.AgentID,
			Status:                 protoStatus,
			LastHeartbeat:          lastHeartbeat,
			SecondsSinceHeartbeat:  int32(p.SecondsSinceHeartbeat),
			Metadata:               p.Metadata,
			LastStatus:             p.LastStatus,
			QueuedMessages:         int32(p.QueuedMessages),
		}
	}

	return &pb.GetPresenceResponse{Agents: agents}, nil
}

// SetQueueConfig configures the offline message queue for an agent.
func (s *Server) SetQueueConfig(ctx context.Context, req *pb.SetQueueConfigRequest) (*pb.SetQueueConfigResponse, error) {
	if req.DevSpaceId == "" || req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "dev_space_id and agent_id required")
	}

	if err := s.catalog.SetQueueConfig(req.DevSpaceId, req.AgentId, int(req.MaxQueueSize)); err != nil {
		return &pb.SetQueueConfigResponse{
			Ok:    false,
			Error: err.Error(),
		}, nil
	}

	return &pb.SetQueueConfigResponse{Ok: true}, nil
}

// Connect (Phase 3) handles bidirectional inter-agent messaging via the broker.
func (s *Server) Connect(stream pb.DevSpace_ConnectServer) error {
	if s.broker == nil {
		return status.Error(codes.Unimplemented, "messaging not enabled")
	}
	// First message must identify the agent (dev_space_id, agent_id)
	first, err := stream.Recv()
	if err == io.EOF {
		return status.Error(codes.InvalidArgument, "connect requires at least one message with dev_space_id and agent_id")
	}
	if err != nil {
		return err
	}
	devSpaceID := first.DevSpaceId
	agentID := first.AgentId
	topic := first.Topic
	if devSpaceID == "" || agentID == "" {
		return status.Error(codes.InvalidArgument, "dev_space_id and agent_id required in first message")
	}
	outCh := make(chan *BrokerMessage, 64)
	unsub := s.broker.Subscribe(devSpaceID, agentID, topic, outCh)
	defer unsub()

	// If first message had payload, publish it
	if len(first.Payload) > 0 {
		s.broker.Publish(devSpaceID, topic, agentID, &BrokerMessage{
			DevSpaceID:  devSpaceID,
			AgentID:     agentID,
			Topic:       topic,
			PayloadType: first.PayloadType,
			Payload:     first.Payload,
			Sequence:    s.broker.NextSequence(),
		})
	}

	// Goroutine: receive from client and publish to broker; close outCh when done so send loop exits
	go func() {
		defer close(outCh)
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}
			if msg.DevSpaceId != devSpaceID || msg.AgentId != agentID {
				continue
			}
			s.broker.Publish(devSpaceID, msg.Topic, agentID, &BrokerMessage{
				DevSpaceID:  msg.DevSpaceId,
				AgentID:     msg.AgentId,
				Topic:       msg.Topic,
				PayloadType: msg.PayloadType,
				Payload:     msg.Payload,
				Sequence:    s.broker.NextSequence(),
			})
		}
	}()

	// Send broker messages to client
	for m := range outCh {
		if err := stream.Send(&pb.AgentMessage{
			DevSpaceId:  m.DevSpaceID,
			AgentId:     m.AgentID,
			Topic:       m.Topic,
			PayloadType: m.PayloadType,
			Payload:     m.Payload,
			Sequence:    m.Sequence,
		}); err != nil {
			return err
		}
	}
	return nil
}
