package devspace

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	pb "mdemg/api/devspacepb"
)

// Catalog is an in-memory DevSpace catalog with export files on disk.
type Catalog struct {
	mu       sync.RWMutex
	dataDir  string
	agents   map[string]map[string]agentInfo   // dev_space_id -> agent_id -> info
	exports  map[string]map[string]exportEntry // dev_space_id -> export_id -> entry
}

type agentInfo struct {
	agentID  string
	metadata map[string]string
	seenAt   time.Time
}

type exportEntry struct {
	ExportID           string
	SpaceID            string
	PublishedAt        time.Time
	PublishedByAgentID string
	Label              string
	FilePath           string
}

// NewCatalog creates a catalog that stores export files under dataDir.
func NewCatalog(dataDir string) (*Catalog, error) {
	if dataDir == "" {
		dataDir = ".devspace/data"
	}
	return &Catalog{
		dataDir: dataDir,
		agents:  make(map[string]map[string]agentInfo),
		exports: make(map[string]map[string]exportEntry),
	}, nil
}

// RegisterAgent adds or updates an agent in a DevSpace (idempotent).
func (c *Catalog) RegisterAgent(devSpaceID, agentID string, metadata map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agents[devSpaceID] == nil {
		c.agents[devSpaceID] = make(map[string]agentInfo)
	}
	c.agents[devSpaceID][agentID] = agentInfo{
		agentID:  agentID,
		metadata: metadata,
		seenAt:   time.Now().UTC(),
	}
}

// DeregisterAgent removes an agent from a DevSpace.
func (c *Catalog) DeregisterAgent(devSpaceID, agentID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if m := c.agents[devSpaceID]; m != nil {
		delete(m, agentID)
	}
}

// ListExports returns all exports for a DevSpace.
func (c *Catalog) ListExports(devSpaceID string) []*pb.ExportEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := c.exports[devSpaceID]
	if m == nil {
		return []*pb.ExportEntry{} // Return empty slice instead of nil
	}
	out := make([]*pb.ExportEntry, 0, len(m))
	for _, e := range m {
		out = append(out, &pb.ExportEntry{
			ExportId:             e.ExportID,
			SpaceId:              e.SpaceID,
			PublishedAt:          e.PublishedAt.Format(time.RFC3339),
			PublishedByAgentId:   e.PublishedByAgentID,
			Label:                e.Label,
		})
	}
	return out
}

// ReserveExport allocates an export_id and returns the path to write the .mdemg file.
// Caller writes the file, then the entry is already stored (no separate confirm).
func (c *Catalog) ReserveExport(devSpaceID, spaceID, publishedByAgentID, label string) (exportID, filePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.exports[devSpaceID] == nil {
		c.exports[devSpaceID] = make(map[string]exportEntry)
	}
	exportID = uuid.Must(uuid.NewRandom()).String()[:8]
	filePath = filepath.Join(c.dataDir, devSpaceID, exportID+".mdemg")
	c.exports[devSpaceID][exportID] = exportEntry{
		ExportID:           exportID,
		SpaceID:            spaceID,
		PublishedAt:        time.Now().UTC(),
		PublishedByAgentID: publishedByAgentID,
		Label:              label,
		FilePath:           filePath,
	}
	return exportID, filePath
}

// GetExport returns the file path for an export, or error if not found.
func (c *Catalog) GetExport(devSpaceID, exportID string) (filePath string, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := c.exports[devSpaceID]
	if m == nil {
		return "", fmt.Errorf("export not found: %s", exportID)
	}
	e, ok := m[exportID]
	if !ok {
		return "", fmt.Errorf("export not found: %s", exportID)
	}
	return e.FilePath, nil
}

