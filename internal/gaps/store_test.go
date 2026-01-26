package gaps

import (
	"testing"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func TestRecordToGap_NilRecord(t *testing.T) {
	gap := recordToGap(nil)
	if gap != nil {
		t.Error("expected nil gap for nil record")
	}
}

func TestToInt64(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int64
	}{
		{"int64", int64(42), 42},
		{"int", int(42), 42},
		{"float64", float64(42.0), 42},
		{"string", "42", 0},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toInt64(tt.input)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected float64
	}{
		{"float64", float64(3.14), 3.14},
		{"float32", float32(3.14), float64(float32(3.14))},
		{"int64", int64(42), 42.0},
		{"int", int(42), 42.0},
		{"string", "3.14", 0},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toFloat64(tt.input)
			if result != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, result)
			}
		})
	}
}

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected []string
	}{
		{"string slice", []string{"a", "b"}, []string{"a", "b"}},
		{"any slice", []any{"a", "b"}, []string{"a", "b"}},
		{"nil", nil, nil},
		{"empty string slice", []string{}, []string{}},
		{"empty any slice", []any{}, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toStringSlice(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected len %d, got %d", len(tt.expected), len(result))
				return
			}
			for i, s := range tt.expected {
				if result[i] != s {
					t.Errorf("element %d: expected %q, got %q", i, s, result[i])
				}
			}
		})
	}
}

func TestToTime(t *testing.T) {
	now := time.Now()
	nowStr := now.Format(time.RFC3339)

	tests := []struct {
		name  string
		input any
	}{
		{"time.Time", now},
		{"string RFC3339", nowStr},
		{"neo4j.LocalDateTime", neo4j.LocalDateTimeOf(now)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toTime(tt.input)
			// Just check that it's not zero
			if result.IsZero() && tt.input != nil {
				t.Error("expected non-zero time")
			}
		})
	}
}

func TestNilIfEmpty(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected any
	}{
		{"empty string", "", nil},
		{"non-empty string", "hello", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nilIfEmpty(tt.input)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestStore_Initialization tests that NewStore doesn't panic
func TestStore_Initialization(t *testing.T) {
	// NewStore should not panic with nil driver
	store := NewStore(nil)
	if store == nil {
		t.Error("NewStore returned nil")
	}
}

// Note: Full store tests require a real Neo4j connection and are typically
// done in integration tests. These unit tests focus on helper functions.
