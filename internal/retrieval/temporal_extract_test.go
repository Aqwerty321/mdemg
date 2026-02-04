package retrieval

import (
	"strings"
	"testing"
	"time"
)

func TestExtractCanonicalTime_Changelog(t *testing.T) {
	content := "# Changelog\n\n## [2.1.0] - 2026-01-15\n\n### Added\n- New feature"
	result := ExtractCanonicalTime(content, nil)
	expected := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestExtractCanonicalTime_VersionParens(t *testing.T) {
	content := "Version 1.0 (2025-12-01)\n\nRelease notes..."
	result := ExtractCanonicalTime(content, nil)
	expected := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestExtractCanonicalTime_MetadataDate(t *testing.T) {
	content := "Date: 2025-11-20\nAuthor: John\n\nContent here..."
	result := ExtractCanonicalTime(content, nil)
	expected := time.Date(2025, 11, 20, 0, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestExtractCanonicalTime_ISODate(t *testing.T) {
	content := "# Documentation\n2025-06-15\nSome content follows"
	result := ExtractCanonicalTime(content, nil)
	expected := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestExtractCanonicalTime_NaturalDate(t *testing.T) {
	content := "January 15, 2026\nMeeting notes from the standup"
	result := ExtractCanonicalTime(content, nil)
	expected := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestExtractCanonicalTime_NoDate(t *testing.T) {
	content := "func HandleAuth() {\n\treturn nil\n}"
	result := ExtractCanonicalTime(content, nil)
	if !result.IsZero() {
		t.Errorf("expected zero time for dateless content, got %v", result)
	}
}

func TestExtractCanonicalTime_FutureDateRejected(t *testing.T) {
	content := "Date: 2099-01-01\nFar future content"
	result := ExtractCanonicalTime(content, nil)
	if !result.IsZero() {
		t.Errorf("expected zero time for far-future date, got %v", result)
	}
}

func TestExtractCanonicalTime_AncientDateRejected(t *testing.T) {
	content := "Date: 1990-01-01\nAncient content"
	result := ExtractCanonicalTime(content, nil)
	if !result.IsZero() {
		t.Errorf("expected zero time for pre-2000 date, got %v", result)
	}
}

func TestExtractCanonicalTime_DateBeyond500Chars(t *testing.T) {
	// Build content with 600+ characters of filler before the date
	filler := strings.Repeat("x", 600)
	content := filler + "\nDate: 2025-06-15\n"
	result := ExtractCanonicalTime(content, nil)
	if !result.IsZero() {
		t.Errorf("expected zero time when date is beyond 500 char scan area, got %v", result)
	}
}

func TestExtractCanonicalTime_PriorityOrder(t *testing.T) {
	// Content has both a changelog date and an ISO date; changelog should win
	content := "## [1.0.0] - 2026-01-15\n\n2025-06-01\nOther content"
	result := ExtractCanonicalTime(content, nil)
	expected := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("expected changelog date %v to win over ISO date, got %v", expected, result)
	}
}

func TestExtractCanonicalTime_EmptyContent(t *testing.T) {
	result := ExtractCanonicalTime("", nil)
	if !result.IsZero() {
		t.Errorf("expected zero time for empty content, got %v", result)
	}
}

func TestExtractCanonicalTime_MetadataVariants(t *testing.T) {
	tests := []struct {
		name    string
		content string
		year    int
		month   time.Month
		day     int
	}{
		{"Created", "Created: 2025-03-10\nContent", 2025, time.March, 10},
		{"Modified", "Modified: 2025-04-20\nContent", 2025, time.April, 20},
		{"Updated", "Updated: 2025-05-30\nContent", 2025, time.May, 30},
		{"Last-Modified", "Last-Modified: 2025-06-15\nContent", 2025, time.June, 15},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractCanonicalTime(tc.content, nil)
			expected := time.Date(tc.year, tc.month, tc.day, 0, 0, 0, 0, time.UTC)
			if !result.Equal(expected) {
				t.Errorf("expected %v, got %v", expected, result)
			}
		})
	}
}
