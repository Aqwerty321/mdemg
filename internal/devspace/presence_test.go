package devspace

import (
	"context"
	"testing"
	"time"

	pb "mdemg/api/devspacepb"
)

func TestCatalog_UpdateHeartbeat(t *testing.T) {
	cat, err := NewCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Register agent first
	cat.RegisterAgent("team1", "agent1", map[string]string{"name": "Test Agent"})

	// Update heartbeat
	status := map[string]string{"load": "0.5", "task": "indexing"}
	queueSize := cat.UpdateHeartbeat("team1", "agent1", status)

	if queueSize != 0 {
		t.Errorf("expected queue size 0, got %d", queueSize)
	}

	// Verify heartbeat was recorded
	presence := cat.GetPresence("team1", "agent1")
	if len(presence) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(presence))
	}

	if presence[0].Status != "online" {
		t.Errorf("expected status 'online', got %q", presence[0].Status)
	}
	if presence[0].LastStatus["load"] != "0.5" {
		t.Errorf("expected last_status[load] '0.5', got %q", presence[0].LastStatus["load"])
	}
}

func TestCatalog_UpdateHeartbeat_UnregisteredAgent(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	// Try to update heartbeat for unregistered agent
	queueSize := cat.UpdateHeartbeat("team1", "unknown", nil)
	if queueSize != 0 {
		t.Errorf("expected queue size 0 for unknown agent, got %d", queueSize)
	}
}

func TestCatalog_GetPresence(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	// Register two agents
	cat.RegisterAgent("team1", "agent1", map[string]string{"name": "Agent 1"})
	cat.RegisterAgent("team1", "agent2", map[string]string{"name": "Agent 2"})

	// Update heartbeat for agent1 only
	cat.UpdateHeartbeat("team1", "agent1", nil)

	// Get all presence
	presence := cat.GetPresence("team1", "")
	if len(presence) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(presence))
	}

	// Get specific agent
	presence = cat.GetPresence("team1", "agent1")
	if len(presence) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(presence))
	}
	if presence[0].AgentID != "agent1" {
		t.Errorf("expected agent1, got %q", presence[0].AgentID)
	}
}

func TestCatalog_GetPresence_EmptyDevSpace(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	presence := cat.GetPresence("nonexistent", "")
	if len(presence) != 0 {
		t.Errorf("expected 0 agents, got %d", len(presence))
	}
}

func TestCatalog_PresenceStatus(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	cat.RegisterAgent("team1", "agent1", nil)

	// No heartbeat yet - should be unknown
	presence := cat.GetPresence("team1", "agent1")
	if presence[0].Status != "unknown" {
		t.Errorf("expected 'unknown' status, got %q", presence[0].Status)
	}

	// Update heartbeat - should be online
	cat.UpdateHeartbeat("team1", "agent1", nil)
	presence = cat.GetPresence("team1", "agent1")
	if presence[0].Status != "online" {
		t.Errorf("expected 'online' status, got %q", presence[0].Status)
	}
}

func TestCatalog_SetQueueConfig(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	cat.RegisterAgent("team1", "agent1", nil)

	// Set queue config
	err := cat.SetQueueConfig("team1", "agent1", 50)
	if err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}

	// Set for unknown agent
	err = cat.SetQueueConfig("team1", "unknown", 50)
	if err == nil {
		t.Error("expected error for unknown agent")
	}

	// Set for unknown devspace
	err = cat.SetQueueConfig("unknown", "agent1", 50)
	if err == nil {
		t.Error("expected error for unknown devspace")
	}
}

func TestCatalog_QueueMessage(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	cat.RegisterAgent("team1", "agent1", nil)
	cat.SetQueueConfig("team1", "agent1", 10)

	// Queue a message
	msg := &pb.AgentMessage{
		DevSpaceId: "team1",
		AgentId:    "sender",
		Payload:    []byte("hello"),
	}
	queued := cat.QueueMessage("team1", "agent1", msg)
	if !queued {
		t.Error("expected message to be queued")
	}

	// Check queue size
	presence := cat.GetPresence("team1", "agent1")
	if presence[0].QueuedMessages != 1 {
		t.Errorf("expected 1 queued message, got %d", presence[0].QueuedMessages)
	}
}

func TestCatalog_QueueMessage_Disabled(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	cat.RegisterAgent("team1", "agent1", nil)
	cat.SetQueueConfig("team1", "agent1", 0) // Disable queue

	msg := &pb.AgentMessage{Payload: []byte("hello")}
	queued := cat.QueueMessage("team1", "agent1", msg)
	if queued {
		t.Error("expected message NOT to be queued when disabled")
	}
}

func TestCatalog_QueueMessage_Full(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	cat.RegisterAgent("team1", "agent1", nil)
	cat.SetQueueConfig("team1", "agent1", 2) // Max 2 messages

	msg := &pb.AgentMessage{Payload: []byte("hello")}

	// Queue 2 messages
	cat.QueueMessage("team1", "agent1", msg)
	cat.QueueMessage("team1", "agent1", msg)

	// Third should fail
	queued := cat.QueueMessage("team1", "agent1", msg)
	if queued {
		t.Error("expected message NOT to be queued when full")
	}
}

func TestCatalog_QueueMessage_Unlimited(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	cat.RegisterAgent("team1", "agent1", nil)
	cat.SetQueueConfig("team1", "agent1", -1) // Unlimited

	msg := &pb.AgentMessage{Payload: []byte("hello")}

	// Queue many messages
	for i := 0; i < 1000; i++ {
		queued := cat.QueueMessage("team1", "agent1", msg)
		if !queued {
			t.Errorf("expected message %d to be queued", i)
			break
		}
	}
}

func TestCatalog_DrainQueue(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	cat.RegisterAgent("team1", "agent1", nil)
	cat.SetQueueConfig("team1", "agent1", 10)

	// Queue messages
	for i := 0; i < 3; i++ {
		cat.QueueMessage("team1", "agent1", &pb.AgentMessage{
			Payload: []byte("msg"),
		})
	}

	// Drain queue
	msgs := cat.DrainQueue("team1", "agent1")
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}

	// Queue should be empty now
	presence := cat.GetPresence("team1", "agent1")
	if presence[0].QueuedMessages != 0 {
		t.Errorf("expected 0 queued messages after drain, got %d", presence[0].QueuedMessages)
	}
}

func TestCatalog_DrainQueue_UnknownAgent(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	msgs := cat.DrainQueue("team1", "unknown")
	if msgs != nil {
		t.Error("expected nil for unknown agent")
	}
}

func TestCatalog_IsAgentOnline(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	cat.RegisterAgent("team1", "agent1", nil)

	// No heartbeat yet
	if cat.IsAgentOnline("team1", "agent1") {
		t.Error("expected agent to be offline before heartbeat")
	}

	// After heartbeat
	cat.UpdateHeartbeat("team1", "agent1", nil)
	if !cat.IsAgentOnline("team1", "agent1") {
		t.Error("expected agent to be online after heartbeat")
	}

	// Unknown agent
	if cat.IsAgentOnline("team1", "unknown") {
		t.Error("expected unknown agent to be offline")
	}
}

func TestServer_Heartbeat(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	srv := NewServer(cat, nil)

	// Register agent first
	cat.RegisterAgent("team1", "agent1", nil)

	ctx := context.Background()

	// Send heartbeat
	resp, err := srv.Heartbeat(ctx, &pb.HeartbeatRequest{
		DevSpaceId: "team1",
		AgentId:    "agent1",
		Status:     map[string]string{"task": "testing"},
	})
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}
	if resp.ServerTime == "" {
		t.Error("expected server_time to be set")
	}
}

func TestServer_Heartbeat_InvalidArgs(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	srv := NewServer(cat, nil)

	ctx := context.Background()

	// Missing dev_space_id
	_, err := srv.Heartbeat(ctx, &pb.HeartbeatRequest{AgentId: "agent1"})
	if err == nil {
		t.Error("expected error for missing dev_space_id")
	}

	// Missing agent_id
	_, err = srv.Heartbeat(ctx, &pb.HeartbeatRequest{DevSpaceId: "team1"})
	if err == nil {
		t.Error("expected error for missing agent_id")
	}
}

func TestServer_GetPresence(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	srv := NewServer(cat, nil)

	cat.RegisterAgent("team1", "agent1", map[string]string{"name": "Agent 1"})
	cat.UpdateHeartbeat("team1", "agent1", nil)

	ctx := context.Background()

	resp, err := srv.GetPresence(ctx, &pb.GetPresenceRequest{
		DevSpaceId: "team1",
	})
	if err != nil {
		t.Fatalf("GetPresence: %v", err)
	}
	if len(resp.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(resp.Agents))
	}
	if resp.Agents[0].Status != pb.PresenceStatus_PRESENCE_ONLINE {
		t.Errorf("expected ONLINE status, got %v", resp.Agents[0].Status)
	}
}

func TestServer_GetPresence_InvalidArgs(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	srv := NewServer(cat, nil)

	ctx := context.Background()

	_, err := srv.GetPresence(ctx, &pb.GetPresenceRequest{})
	if err == nil {
		t.Error("expected error for missing dev_space_id")
	}
}

func TestServer_SetQueueConfig(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	srv := NewServer(cat, nil)

	cat.RegisterAgent("team1", "agent1", nil)

	ctx := context.Background()

	resp, err := srv.SetQueueConfig(ctx, &pb.SetQueueConfigRequest{
		DevSpaceId:   "team1",
		AgentId:      "agent1",
		MaxQueueSize: 50,
	})
	if err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}
	if !resp.Ok {
		t.Errorf("expected ok=true, got error: %s", resp.Error)
	}
}

func TestServer_SetQueueConfig_InvalidArgs(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	srv := NewServer(cat, nil)

	ctx := context.Background()

	// Missing dev_space_id
	_, err := srv.SetQueueConfig(ctx, &pb.SetQueueConfigRequest{AgentId: "agent1"})
	if err == nil {
		t.Error("expected error for missing dev_space_id")
	}

	// Missing agent_id
	_, err = srv.SetQueueConfig(ctx, &pb.SetQueueConfigRequest{DevSpaceId: "team1"})
	if err == nil {
		t.Error("expected error for missing agent_id")
	}
}

func TestServer_SetQueueConfig_UnknownAgent(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	srv := NewServer(cat, nil)

	ctx := context.Background()

	resp, err := srv.SetQueueConfig(ctx, &pb.SetQueueConfigRequest{
		DevSpaceId:   "team1",
		AgentId:      "unknown",
		MaxQueueSize: 50,
	})
	if err != nil {
		t.Fatalf("SetQueueConfig: %v", err)
	}
	if resp.Ok {
		t.Error("expected ok=false for unknown agent")
	}
}

func TestPresenceThresholds(t *testing.T) {
	// Verify threshold constants are sensible
	if OnlineThreshold <= 0 {
		t.Error("OnlineThreshold should be positive")
	}
	if AwayThreshold <= OnlineThreshold {
		t.Error("AwayThreshold should be greater than OnlineThreshold")
	}
	if DefaultQueueSize <= 0 {
		t.Error("DefaultQueueSize should be positive")
	}
}

// TestPresenceStatusTransitions tests that presence status changes correctly over time.
// Note: This is a unit test that verifies the calculation logic, not actual time passage.
func TestPresenceStatusTransitions(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	cat.RegisterAgent("team1", "agent1", nil)

	// Simulate heartbeat with specific time by directly modifying agent
	cat.mu.Lock()
	agent := cat.agents["team1"]["agent1"]

	// Test online (heartbeat 10 seconds ago)
	agent.lastHeartbeat = time.Now().UTC().Add(-10 * time.Second)
	cat.agents["team1"]["agent1"] = agent
	cat.mu.Unlock()

	presence := cat.GetPresence("team1", "agent1")
	if presence[0].Status != "online" {
		t.Errorf("expected 'online' for 10s ago, got %q", presence[0].Status)
	}

	// Test away (heartbeat 2 minutes ago)
	cat.mu.Lock()
	agent.lastHeartbeat = time.Now().UTC().Add(-2 * time.Minute)
	cat.agents["team1"]["agent1"] = agent
	cat.mu.Unlock()

	presence = cat.GetPresence("team1", "agent1")
	if presence[0].Status != "away" {
		t.Errorf("expected 'away' for 2m ago, got %q", presence[0].Status)
	}

	// Test offline (heartbeat 10 minutes ago)
	cat.mu.Lock()
	agent.lastHeartbeat = time.Now().UTC().Add(-10 * time.Minute)
	cat.agents["team1"]["agent1"] = agent
	cat.mu.Unlock()

	presence = cat.GetPresence("team1", "agent1")
	if presence[0].Status != "offline" {
		t.Errorf("expected 'offline' for 10m ago, got %q", presence[0].Status)
	}
}

// TestServer_GetPresence_AllStatusTypes tests that the server correctly maps all status types to proto enums.
func TestServer_GetPresence_AllStatusTypes(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	srv := NewServer(cat, nil)
	ctx := context.Background()

	// Register agents for each status type
	cat.RegisterAgent("team1", "online-agent", nil)
	cat.RegisterAgent("team1", "away-agent", nil)
	cat.RegisterAgent("team1", "offline-agent", nil)
	cat.RegisterAgent("team1", "unknown-agent", nil)

	// Set different heartbeat times
	cat.mu.Lock()
	// Online: heartbeat 5 seconds ago
	onlineAgent := cat.agents["team1"]["online-agent"]
	onlineAgent.lastHeartbeat = time.Now().UTC().Add(-5 * time.Second)
	cat.agents["team1"]["online-agent"] = onlineAgent

	// Away: heartbeat 2 minutes ago
	awayAgent := cat.agents["team1"]["away-agent"]
	awayAgent.lastHeartbeat = time.Now().UTC().Add(-2 * time.Minute)
	cat.agents["team1"]["away-agent"] = awayAgent

	// Offline: heartbeat 10 minutes ago
	offlineAgent := cat.agents["team1"]["offline-agent"]
	offlineAgent.lastHeartbeat = time.Now().UTC().Add(-10 * time.Minute)
	cat.agents["team1"]["offline-agent"] = offlineAgent

	// Unknown: no heartbeat (zero time)
	// (already zero by default)
	cat.mu.Unlock()

	// Test online agent
	resp, err := srv.GetPresence(ctx, &pb.GetPresenceRequest{
		DevSpaceId: "team1",
		AgentId:    "online-agent",
	})
	if err != nil {
		t.Fatalf("GetPresence online: %v", err)
	}
	if resp.Agents[0].Status != pb.PresenceStatus_PRESENCE_ONLINE {
		t.Errorf("expected PRESENCE_ONLINE, got %v", resp.Agents[0].Status)
	}
	if resp.Agents[0].LastHeartbeat == "" {
		t.Error("expected LastHeartbeat to be set for online agent")
	}

	// Test away agent
	resp, err = srv.GetPresence(ctx, &pb.GetPresenceRequest{
		DevSpaceId: "team1",
		AgentId:    "away-agent",
	})
	if err != nil {
		t.Fatalf("GetPresence away: %v", err)
	}
	if resp.Agents[0].Status != pb.PresenceStatus_PRESENCE_AWAY {
		t.Errorf("expected PRESENCE_AWAY, got %v", resp.Agents[0].Status)
	}

	// Test offline agent
	resp, err = srv.GetPresence(ctx, &pb.GetPresenceRequest{
		DevSpaceId: "team1",
		AgentId:    "offline-agent",
	})
	if err != nil {
		t.Fatalf("GetPresence offline: %v", err)
	}
	if resp.Agents[0].Status != pb.PresenceStatus_PRESENCE_OFFLINE {
		t.Errorf("expected PRESENCE_OFFLINE, got %v", resp.Agents[0].Status)
	}

	// Test unknown agent (no heartbeat)
	resp, err = srv.GetPresence(ctx, &pb.GetPresenceRequest{
		DevSpaceId: "team1",
		AgentId:    "unknown-agent",
	})
	if err != nil {
		t.Fatalf("GetPresence unknown: %v", err)
	}
	if resp.Agents[0].Status != pb.PresenceStatus_PRESENCE_UNKNOWN {
		t.Errorf("expected PRESENCE_UNKNOWN, got %v", resp.Agents[0].Status)
	}
	if resp.Agents[0].LastHeartbeat != "" {
		t.Error("expected LastHeartbeat to be empty for unknown agent")
	}
}

// TestServer_GetPresence_EmptyDevSpace tests GetPresence with no registered agents.
func TestServer_GetPresence_EmptyDevSpace(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	srv := NewServer(cat, nil)
	ctx := context.Background()

	resp, err := srv.GetPresence(ctx, &pb.GetPresenceRequest{
		DevSpaceId: "empty-team",
	})
	if err != nil {
		t.Fatalf("GetPresence: %v", err)
	}
	if len(resp.Agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(resp.Agents))
	}
}

// TestServer_GetPresence_WithMetadataAndStatus tests that metadata and status are correctly returned.
func TestServer_GetPresence_WithMetadataAndStatus(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	srv := NewServer(cat, nil)
	ctx := context.Background()

	// Register agent with metadata
	cat.RegisterAgent("team1", "agent1", map[string]string{
		"name":    "Test Agent",
		"version": "1.0.0",
	})

	// Send heartbeat with status
	srv.Heartbeat(ctx, &pb.HeartbeatRequest{
		DevSpaceId: "team1",
		AgentId:    "agent1",
		Status: map[string]string{
			"task": "indexing",
			"load": "0.75",
		},
	})

	// Get presence
	resp, err := srv.GetPresence(ctx, &pb.GetPresenceRequest{
		DevSpaceId: "team1",
		AgentId:    "agent1",
	})
	if err != nil {
		t.Fatalf("GetPresence: %v", err)
	}

	agent := resp.Agents[0]

	// Check metadata
	if agent.Metadata["name"] != "Test Agent" {
		t.Errorf("expected metadata[name] 'Test Agent', got %q", agent.Metadata["name"])
	}
	if agent.Metadata["version"] != "1.0.0" {
		t.Errorf("expected metadata[version] '1.0.0', got %q", agent.Metadata["version"])
	}

	// Check last status from heartbeat
	if agent.LastStatus["task"] != "indexing" {
		t.Errorf("expected last_status[task] 'indexing', got %q", agent.LastStatus["task"])
	}
	if agent.LastStatus["load"] != "0.75" {
		t.Errorf("expected last_status[load] '0.75', got %q", agent.LastStatus["load"])
	}

	// Check seconds since heartbeat is reasonable
	if agent.SecondsSinceHeartbeat < 0 || agent.SecondsSinceHeartbeat > 5 {
		t.Errorf("expected seconds_since_heartbeat 0-5, got %d", agent.SecondsSinceHeartbeat)
	}
}

// TestServer_GetPresence_WithQueuedMessages tests that queued message count is returned.
func TestServer_GetPresence_WithQueuedMessages(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())
	srv := NewServer(cat, nil)
	ctx := context.Background()

	// Register agent and configure queue
	cat.RegisterAgent("team1", "agent1", nil)
	cat.SetQueueConfig("team1", "agent1", 100)

	// Queue some messages
	for i := 0; i < 5; i++ {
		cat.QueueMessage("team1", "agent1", &pb.AgentMessage{
			Payload: []byte("test"),
		})
	}

	// Get presence
	resp, err := srv.GetPresence(ctx, &pb.GetPresenceRequest{
		DevSpaceId: "team1",
		AgentId:    "agent1",
	})
	if err != nil {
		t.Fatalf("GetPresence: %v", err)
	}

	if resp.Agents[0].QueuedMessages != 5 {
		t.Errorf("expected 5 queued messages, got %d", resp.Agents[0].QueuedMessages)
	}
}

// TestCatalog_UpdateHeartbeat_UnknownDevSpace tests heartbeat for unknown devspace.
func TestCatalog_UpdateHeartbeat_UnknownDevSpace(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	// Try to update heartbeat for unknown devspace
	queueSize := cat.UpdateHeartbeat("unknown-devspace", "agent1", nil)
	if queueSize != 0 {
		t.Errorf("expected queue size 0 for unknown devspace, got %d", queueSize)
	}
}

// TestCatalog_QueueMessage_UnknownDevSpace tests queueing for unknown devspace.
func TestCatalog_QueueMessage_UnknownDevSpace(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	queued := cat.QueueMessage("unknown-devspace", "agent1", &pb.AgentMessage{})
	if queued {
		t.Error("expected message NOT to be queued for unknown devspace")
	}
}

// TestCatalog_QueueMessage_UnknownAgent tests queueing for unknown agent.
func TestCatalog_QueueMessage_UnknownAgent(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	// Register devspace but not the agent
	cat.RegisterAgent("team1", "other-agent", nil)

	queued := cat.QueueMessage("team1", "unknown-agent", &pb.AgentMessage{})
	if queued {
		t.Error("expected message NOT to be queued for unknown agent")
	}
}

// TestCatalog_DrainQueue_UnknownDevSpace tests draining queue for unknown devspace.
func TestCatalog_DrainQueue_UnknownDevSpace(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	msgs := cat.DrainQueue("unknown-devspace", "agent1")
	if msgs != nil {
		t.Error("expected nil for unknown devspace")
	}
}

// TestCatalog_IsAgentOnline_UnknownDevSpace tests online check for unknown devspace.
func TestCatalog_IsAgentOnline_UnknownDevSpace(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	if cat.IsAgentOnline("unknown-devspace", "agent1") {
		t.Error("expected agent to be offline for unknown devspace")
	}
}

// TestCatalog_DrainQueue_ExpiredMessages tests that expired messages are not returned.
func TestCatalog_DrainQueue_ExpiredMessages(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	cat.RegisterAgent("team1", "agent1", nil)
	cat.SetQueueConfig("team1", "agent1", 100)

	// Manually add an expired message
	cat.mu.Lock()
	agent := cat.agents["team1"]["agent1"]
	agent.queuedMsgs = append(agent.queuedMsgs, &QueuedMessage{
		Message:   &pb.AgentMessage{Payload: []byte("expired")},
		QueuedAt:  time.Now().UTC().Add(-25 * time.Hour),
		ExpiresAt: time.Now().UTC().Add(-1 * time.Hour), // Already expired
	})
	agent.queuedMsgs = append(agent.queuedMsgs, &QueuedMessage{
		Message:   &pb.AgentMessage{Payload: []byte("valid")},
		QueuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour), // Still valid
	})
	cat.agents["team1"]["agent1"] = agent
	cat.mu.Unlock()

	// Drain should only return valid message
	msgs := cat.DrainQueue("team1", "agent1")
	if len(msgs) != 1 {
		t.Errorf("expected 1 valid message, got %d", len(msgs))
	}
	if string(msgs[0].Payload) != "valid" {
		t.Errorf("expected 'valid' message, got %q", string(msgs[0].Payload))
	}
}

// TestCatalog_DrainQueue_AgentNotFound tests draining for existing devspace but unknown agent.
func TestCatalog_DrainQueue_AgentNotFound(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	// Register one agent to create the devspace
	cat.RegisterAgent("team1", "agent1", nil)

	// Try to drain queue for different agent in same devspace
	msgs := cat.DrainQueue("team1", "unknown-agent")
	if msgs != nil {
		t.Error("expected nil for unknown agent in existing devspace")
	}
}

// TestCatalog_UpdateHeartbeat_WithQueuedMessages tests heartbeat returns queue size.
func TestCatalog_UpdateHeartbeat_WithQueuedMessages(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	cat.RegisterAgent("team1", "agent1", nil)
	cat.SetQueueConfig("team1", "agent1", 100)

	// Queue some messages
	for i := 0; i < 3; i++ {
		cat.QueueMessage("team1", "agent1", &pb.AgentMessage{Payload: []byte("test")})
	}

	// Heartbeat should return queue size
	queueSize := cat.UpdateHeartbeat("team1", "agent1", nil)
	if queueSize != 3 {
		t.Errorf("expected queue size 3, got %d", queueSize)
	}
}

// TestCatalog_UpdateHeartbeat_AgentNotFoundInExistingDevSpace tests heartbeat for
// unknown agent in an existing devspace (devspace exists, agent doesn't).
func TestCatalog_UpdateHeartbeat_AgentNotFoundInExistingDevSpace(t *testing.T) {
	cat, _ := NewCatalog(t.TempDir())

	// Register one agent to create the devspace
	cat.RegisterAgent("team1", "existing-agent", nil)

	// Try to update heartbeat for a different agent that doesn't exist in this devspace
	queueSize := cat.UpdateHeartbeat("team1", "nonexistent-agent", nil)
	if queueSize != 0 {
		t.Errorf("expected queue size 0 for nonexistent agent in existing devspace, got %d", queueSize)
	}
}
