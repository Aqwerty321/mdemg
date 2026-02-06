// Package devspace: in-memory message broker for Phase 3 inter-agent messaging.
// Routes messages by dev_space_id and optional topic; does not depend on proto.

package devspace

import (
	"sync"
	"sync/atomic"
)

// BrokerMessage is the in-memory message type used by the broker.
// When Phase 3 proto is added, convert to/from pb.AgentMessage at the server boundary.
type BrokerMessage struct {
	DevSpaceID  string
	AgentID     string
	Topic       string
	PayloadType string
	Payload     []byte
	Sequence    int64
}

// subEntry holds a subscriber's topic and outbound channel.
type subEntry struct {
	topic string
	ch   chan<- *BrokerMessage
}

// Broker routes messages to connected agents in the same DevSpace (and optional topic).
type Broker struct {
	mu          sync.RWMutex
	subs        map[string]map[string]subEntry // dev_space_id -> agent_id -> entry
	sequenceGen atomic.Int64
}

// NewBroker returns a new in-memory broker.
func NewBroker() *Broker {
	return &Broker{
		subs: make(map[string]map[string]subEntry),
	}
}

// Subscribe registers an agent's outbound channel for a DevSpace (and optional topic).
// Empty topic means receive all messages in the DevSpace; non-empty matches Publish topic.
// The returned unsubscribe function must be called when the agent disconnects.
func (b *Broker) Subscribe(devSpaceID, agentID, topic string, outCh chan<- *BrokerMessage) (unsubscribe func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs[devSpaceID] == nil {
		b.subs[devSpaceID] = make(map[string]subEntry)
	}
	b.subs[devSpaceID][agentID] = subEntry{topic: topic, ch: outCh}
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if m := b.subs[devSpaceID]; m != nil {
			delete(m, agentID)
			if len(m) == 0 {
				delete(b.subs, devSpaceID)
			}
		}
	}
}

// Publish sends a message to all subscribed agents in the same DevSpace.
// If topic is non-empty, only subscribers subscribed to "" or this topic receive it.
// Excludes the sender. Non-blocking: skips if channel full (MVP).
func (b *Broker) Publish(devSpaceID, topic, fromAgentID string, msg *BrokerMessage) {
	b.mu.RLock()
	agents := b.subs[devSpaceID]
	if agents == nil {
		b.mu.RUnlock()
		return
	}
	var targets []chan<- *BrokerMessage
	for id, e := range agents {
		if id == fromAgentID {
			continue
		}
		if topic == "" || e.topic == "" || e.topic == topic {
			targets = append(targets, e.ch)
		}
	}
	b.mu.RUnlock()

	msg.Sequence = b.sequenceGen.Add(1)
	for _, ch := range targets {
		select {
		case ch <- msg:
		default:
			// MVP: drop if buffer full; optional queue later
		}
	}
}

// NextSequence returns the next sequence number for ordering (e.g. when building AgentMessage).
func (b *Broker) NextSequence() int64 {
	return b.sequenceGen.Add(1)
}
