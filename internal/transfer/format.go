package transfer

import (
	"encoding/json"
	"fmt"
	"os"

	pb "mdemg/api/transferpb"
	"google.golang.org/protobuf/encoding/protojson"
)

// MdemgFileHeader is the top-level structure of an .mdemg export file.
type MdemgFileHeader struct {
	Format  string `json:"format"`  // "mdemg-space-transfer"
	Version string `json:"version"` // "1.0.0"
}

// MdemgFile is the complete .mdemg file structure (JSON).
type MdemgFile struct {
	Header MdemgFileHeader    `json:"header"`
	Chunks []*json.RawMessage `json:"chunks"`
}

// WriteFile writes an ExportResult to a .mdemg JSON file.
func WriteFile(path string, result *ExportResult) error {
	marshaler := protojson.MarshalOptions{
		EmitDefaultValues: false,
		Indent:            "",
		UseProtoNames:     true,
	}

	file := MdemgFile{
		Header: MdemgFileHeader{
			Format:  "mdemg-space-transfer",
			Version: "1.0.0",
		},
		Chunks: make([]*json.RawMessage, 0, len(result.Chunks)),
	}

	for _, chunk := range result.Chunks {
		b, err := marshaler.Marshal(chunk)
		if err != nil {
			return fmt.Errorf("marshal chunk %d: %w", chunk.Sequence, err)
		}
		raw := json.RawMessage(b)
		file.Chunks = append(file.Chunks, &raw)
	}

	out, err := json.Marshal(file)
	if err != nil {
		return fmt.Errorf("marshal file: %w", err)
	}

	return os.WriteFile(path, out, 0644)
}

// ReadFile reads an .mdemg JSON file and returns SpaceChunks.
func ReadFile(path string) ([]*pb.SpaceChunk, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var file MdemgFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse file: %w", err)
	}

	if file.Header.Format != "mdemg-space-transfer" {
		return nil, fmt.Errorf("invalid file format: %q (expected mdemg-space-transfer)", file.Header.Format)
	}

	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}

	chunks := make([]*pb.SpaceChunk, 0, len(file.Chunks))
	for i, raw := range file.Chunks {
		chunk := &pb.SpaceChunk{}
		if err := unmarshaler.Unmarshal([]byte(*raw), chunk); err != nil {
			return nil, fmt.Errorf("unmarshal chunk %d: %w", i, err)
		}
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}
