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
	agentID       string
	metadata      map[string]string
	seenAt        time.Time
	lastHeartbeat time.Time
	lastStatus    map[string]string // Status from last heartbeat
	maxQueueSize  int               // Max offline queue size (-1 = unlimited, 0 = disabled)
	queuedMsgs    []*QueuedMessage  // Offline message queue
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

// =============================================================================
// Phase 37: Heartbeat / Presence
// =============================================================================

// Presence thresholds
const (
	OnlineThreshold  = 30 * time.Second  // Online if heartbeat within 30s
	AwayThreshold    = 5 * time.Minute   // Away if heartbeat within 5min
	DefaultQueueSize = 100               // Default max queued messages
)

// QueuedMessage represents a message waiting for an offline agent.
type QueuedMessage struct {
	Message   *pb.AgentMessage
	QueuedAt  time.Time
	ExpiresAt time.Time
}

// UpdateHeartbeat updates the last heartbeat time for an agent.
func (c *Catalog) UpdateHeartbeat(devSpaceID, agentID string, status map[string]string) (queueSize int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.agents[devSpaceID] == nil {
		return 0
	}
	agent, ok := c.agents[devSpaceID][agentID]
	if !ok {
		return 0
	}

	agent.lastHeartbeat = time.Now().UTC()
	agent.lastStatus = status
	c.agents[devSpaceID][agentID] = agent

	return len(agent.queuedMsgs)
}

// AgentPresenceInfo holds presence information for an agent.
type AgentPresenceInfo struct {
	AgentID               string
	Status                string // "online", "away", "offline", "unknown"
	LastHeartbeat         time.Time
	SecondsSinceHeartbeat int
	Metadata              map[string]string
	LastStatus            map[string]string
	QueuedMessages        int
}

// GetPresence returns the presence status of agents in a DevSpace.
func (c *Catalog) GetPresence(devSpaceID, agentID string) []AgentPresenceInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	agents := c.agents[devSpaceID]
	if agents == nil {
		return []AgentPresenceInfo{}
	}

	now := time.Now().UTC()
	result := make([]AgentPresenceInfo, 0)

	for id, agent := range agents {
		if agentID != "" && id != agentID {
			continue
		}

		var status string
		var secsSince int
		if agent.lastHeartbeat.IsZero() {
			status = "unknown"
			secsSince = -1
		} else {
			elapsed := now.Sub(agent.lastHeartbeat)
			secsSince = int(elapsed.Seconds())
			if elapsed <= OnlineThreshold {
				status = "online"
			} else if elapsed <= AwayThreshold {
				status = "away"
			} else {
				status = "offline"
			}
		}

		result = append(result, AgentPresenceInfo{
			AgentID:               id,
			Status:                status,
			LastHeartbeat:         agent.lastHeartbeat,
			SecondsSinceHeartbeat: secsSince,
			Metadata:              agent.metadata,
			LastStatus:            agent.lastStatus,
			QueuedMessages:        len(agent.queuedMsgs),
		})
	}

	return result
}

// SetQueueConfig sets the max queue size for an agent's offline message queue.
func (c *Catalog) SetQueueConfig(devSpaceID, agentID string, maxSize int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.agents[devSpaceID] == nil {
		return fmt.Errorf("agent not found: %s", agentID)
	}
	agent, ok := c.agents[devSpaceID][agentID]
	if !ok {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	agent.maxQueueSize = maxSize
	c.agents[devSpaceID][agentID] = agent
	return nil
}

// QueueMessage adds a message to an offline agent's queue.
// Returns true if queued, false if queue is full or disabled.
func (c *Catalog) QueueMessage(devSpaceID, agentID string, msg *pb.AgentMessage) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.agents[devSpaceID] == nil {
		return false
	}
	agent, ok := c.agents[devSpaceID][agentID]
	if !ok {
		return false
	}

	// Check if queueing is disabled
	if agent.maxQueueSize == 0 {
		return false
	}

	// Check if queue is full (if not unlimited)
	if agent.maxQueueSize > 0 && len(agent.queuedMsgs) >= agent.maxQueueSize {
		return false
	}

	agent.queuedMsgs = append(agent.queuedMsgs, &QueuedMessage{
		Message:   msg,
		QueuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour), // Messages expire after 24h
	})
	c.agents[devSpaceID][agentID] = agent
	return true
}

// DrainQueue returns and clears all queued messages for an agent.
func (c *Catalog) DrainQueue(devSpaceID, agentID string) []*pb.AgentMessage {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.agents[devSpaceID] == nil {
		return nil
	}
	agent, ok := c.agents[devSpaceID][agentID]
	if !ok {
		return nil
	}

	now := time.Now().UTC()
	result := make([]*pb.AgentMessage, 0, len(agent.queuedMsgs))
	for _, qm := range agent.queuedMsgs {
		if qm.ExpiresAt.After(now) {
			result = append(result, qm.Message)
		}
	}

	// Clear the queue
	agent.queuedMsgs = nil
	c.agents[devSpaceID][agentID] = agent

	return result
}

// IsAgentOnline returns true if the agent has sent a heartbeat recently.
func (c *Catalog) IsAgentOnline(devSpaceID, agentID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.agents[devSpaceID] == nil {
		return false
	}
	agent, ok := c.agents[devSpaceID][agentID]
	if !ok {
		return false
	}

	if agent.lastHeartbeat.IsZero() {
		return false
	}

	return time.Since(agent.lastHeartbeat) <= OnlineThreshold
}

