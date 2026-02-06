package devspace

import (
	"testing"
)

func TestBroker_SubscribePublish(t *testing.T) {
	b := NewBroker()
	ch := make(chan *BrokerMessage, 2)
	unsub := b.Subscribe("ds1", "agent1", "bugs", ch)
	defer unsub()

	b.Publish("ds1", "bugs", "agent2", &BrokerMessage{
		DevSpaceID: "ds1",
		AgentID:    "agent2",
		Topic:      "bugs",
		Payload:    []byte("hello"),
	})

	msg := <-ch
	if msg.AgentID != "agent2" || string(msg.Payload) != "hello" {
		t.Errorf("got AgentID=%q Payload=%q", msg.AgentID, msg.Payload)
	}
	if msg.Sequence == 0 {
		t.Error("expected non-zero sequence")
	}
}

func TestBroker_PublishExcludesSender(t *testing.T) {
	b := NewBroker()
	ch := make(chan *BrokerMessage, 1)
	unsub := b.Subscribe("ds1", "agent1", "", ch)
	defer unsub()

	b.Publish("ds1", "", "agent1", &BrokerMessage{AgentID: "agent1", Payload: []byte("self")})
	select {
	case <-ch:
		t.Error("sender should not receive own message")
	default:
	}
}

func TestBroker_TopicFilter(t *testing.T) {
	b := NewBroker()
	allCh := make(chan *BrokerMessage, 1)
	bugsCh := make(chan *BrokerMessage, 1)
	unsubAll := b.Subscribe("ds1", "a1", "", allCh)
	unsubBugs := b.Subscribe("ds1", "a2", "bugs", bugsCh)
	defer unsubAll()
	defer unsubBugs()

	b.Publish("ds1", "bugs", "a3", &BrokerMessage{AgentID: "a3", Topic: "bugs", Payload: []byte("x")})
	if msg := <-allCh; msg == nil || string(msg.Payload) != "x" {
		t.Errorf("subscriber with empty topic should receive: %v", msg)
	}
	if msg := <-bugsCh; msg == nil || string(msg.Payload) != "x" {
		t.Errorf("subscriber with topic bugs should receive: %v", msg)
	}

	b.Publish("ds1", "other", "a3", &BrokerMessage{AgentID: "a3", Topic: "other", Payload: []byte("y")})
	if msg := <-allCh; msg == nil || string(msg.Payload) != "y" {
		t.Errorf("empty-topic subscriber should receive other: %v", msg)
	}
	select {
	case <-bugsCh:
		t.Error("bugs subscriber should not receive other topic")
	default:
	}
}
