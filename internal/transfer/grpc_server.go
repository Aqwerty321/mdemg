package transfer

import (
	"context"
	"io"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "mdemg/api/transferpb"
)

// grpcServer implements pb.SpaceTransferServer for the SpaceTransfer gRPC service.
type grpcServer struct {
	pb.UnimplementedSpaceTransferServer
	driver neo4j.DriverWithContext
}

// NewGRPCServer returns a SpaceTransfer gRPC server that uses the given Neo4j driver.
func NewGRPCServer(driver neo4j.DriverWithContext) pb.SpaceTransferServer {
	return &grpcServer{driver: driver}
}

// Export streams all chunks for the requested space to the client.
func (s *grpcServer) Export(req *pb.ExportRequest, stream grpc.ServerStreamingServer[pb.SpaceChunk]) error {
	if req == nil || req.SpaceId == "" {
		return status.Error(codes.InvalidArgument, "space_id is required")
	}
	cfg := ExportFromRequest(req)
	ex := NewExporter(s.driver)
	result, err := ex.Export(stream.Context(), cfg)
	if err != nil {
		return status.Errorf(codes.Internal, "export: %v", err)
	}
	for _, ch := range result.Chunks {
		if err := stream.Send(ch); err != nil {
			return err
		}
	}
	return nil
}

// Import receives streamed chunks from the client and writes them to Neo4j.
func (s *grpcServer) Import(stream grpc.ClientStreamingServer[pb.SpaceChunk, pb.ImportResponse]) error {
	var chunks []*pb.SpaceChunk
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		chunks = append(chunks, chunk)
	}
	if len(chunks) == 0 {
		return status.Error(codes.InvalidArgument, "no chunks received")
	}
	ctx := stream.Context()
	if err := ValidateImport(ctx, s.driver, chunks); err != nil {
		_ = stream.SendAndClose(&pb.ImportResponse{
			Success: false,
			Error:   err.Error(),
		})
		return nil
	}
	// Use conflict mode from first chunk that sets it; default skip (0).
	mode := pb.ConflictMode_CONFLICT_SKIP
	for _, c := range chunks {
		if c.GetConflictMode() != pb.ConflictMode_CONFLICT_SKIP {
			mode = c.GetConflictMode()
			break
		}
	}
	imp := NewImporter(s.driver, mode)
	result, err := imp.Import(ctx, chunks)
	if err != nil {
		_ = stream.SendAndClose(&pb.ImportResponse{
			Success: false,
			Error:   err.Error(),
		})
		return nil
	}
	return stream.SendAndClose(result.ToProto())
}

// ListSpaces returns all spaces with summary metadata.
func (s *grpcServer) ListSpaces(ctx context.Context, req *pb.ListSpacesRequest) (*pb.ListSpacesResponse, error) {
	spaces, err := ListSpaces(ctx, s.driver)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list spaces: %v", err)
	}
	return &pb.ListSpacesResponse{Spaces: spaces}, nil
}

// SpaceInfo returns detailed metadata for one space.
func (s *grpcServer) SpaceInfo(ctx context.Context, req *pb.SpaceInfoRequest) (*pb.SpaceInfoResponse, error) {
	if req == nil || req.SpaceId == "" {
		return nil, status.Error(codes.InvalidArgument, "space_id is required")
	}
	info, err := GetSpaceInfo(ctx, s.driver, req.SpaceId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "space info: %v", err)
	}
	return info, nil
}
