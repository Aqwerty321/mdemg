package plugins

import (
	"testing"
)

func TestHasSubscription(t *testing.T) {
	tests := []struct {
		name          string
		subscriptions []string
		event         string
		want          bool
	}{
		{
			name:          "exact match",
			subscriptions: []string{"source_changed"},
			event:         "source_changed",
			want:          true,
		},
		{
			name:          "no match",
			subscriptions: []string{"source_changed"},
			event:         "ingest_complete",
			want:          false,
		},
		{
			name:          "wildcard match",
			subscriptions: []string{"*"},
			event:         "anything",
			want:          true,
		},
		{
			name:          "empty subscriptions",
			subscriptions: nil,
			event:         "source_changed",
			want:          false,
		},
		{
			name:          "multiple subscriptions",
			subscriptions: []string{"source_changed", "ingest_complete"},
			event:         "ingest_complete",
			want:          true,
		},
		{
			name:          "multiple subscriptions no match",
			subscriptions: []string{"source_changed", "ingest_complete"},
			event:         "config_changed",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasSubscription(tt.subscriptions, tt.event)
			if got != tt.want {
				t.Errorf("hasSubscription(%v, %q) = %v, want %v",
					tt.subscriptions, tt.event, got, tt.want)
			}
		})
	}
}

func TestNewEventDispatcher_NilManager(t *testing.T) {
	d := NewEventDispatcher(nil)
	if d == nil {
		t.Fatal("NewEventDispatcher(nil) returned nil")
	}
	// Should not panic with nil manager
	d.DispatchEvent("source_changed", map[string]string{"space_id": "test"})
}

func TestNewEventDispatcher_WithManager(t *testing.T) {
	mgr := NewManager("", "", "test")
	d := NewEventDispatcher(mgr)
	if d == nil {
		t.Fatal("NewEventDispatcher returned nil")
	}
	if d.pluginMgr != mgr {
		t.Error("pluginMgr not set correctly")
	}
}

func TestDispatchEvent_NoModules(t *testing.T) {
	mgr := NewManager("", "", "test")
	d := NewEventDispatcher(mgr)
	// Should not panic when no modules are loaded
	d.DispatchEvent("source_changed", map[string]string{"space_id": "test"})
}
