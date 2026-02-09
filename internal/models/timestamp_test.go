package models

import (
	"testing"
	"time"
)

func TestParseTimestamp_RFC3339(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Time
		wantErr bool
	}{
		{"valid UTC", "2026-02-09T10:30:00Z", time.Date(2026, 2, 9, 10, 30, 0, 0, time.UTC), false},
		{"valid with offset", "2026-02-09T10:30:00+05:00", time.Date(2026, 2, 9, 10, 30, 0, 0, time.FixedZone("", 5*3600)), false},
		{"valid with nanos", "2026-02-09T10:30:00.123456789Z", time.Date(2026, 2, 9, 10, 30, 0, 123456789, time.UTC), false},
		{"invalid", "not-a-date", time.Time{}, true},
		{"date only (wrong format)", "2026-02-09", time.Time{}, true},
		{"unix seconds (wrong format)", "1739054400", time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimestamp(tt.value, TimestampFormatRFC3339)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTimestamp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.want) {
				t.Errorf("ParseTimestamp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTimestamp_Unix(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Time
		wantErr bool
	}{
		{"valid", "1739054400", time.Unix(1739054400, 0).UTC(), false},
		{"zero epoch", "0", time.Unix(0, 0).UTC(), false},
		{"negative", "-86400", time.Unix(-86400, 0).UTC(), false},
		{"not a number", "abc", time.Time{}, true},
		{"float", "1739054400.5", time.Time{}, true},
		{"rfc3339 string", "2026-02-09T10:30:00Z", time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimestamp(tt.value, TimestampFormatUnix)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTimestamp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.want) {
				t.Errorf("ParseTimestamp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTimestamp_UnixMs(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Time
		wantErr bool
	}{
		{"valid", "1739054400000", time.UnixMilli(1739054400000).UTC(), false},
		{"zero", "0", time.UnixMilli(0).UTC(), false},
		{"not a number", "abc", time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimestamp(tt.value, TimestampFormatUnixMs)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTimestamp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.want) {
				t.Errorf("ParseTimestamp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTimestamp_DateOnly(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    time.Time
		wantErr bool
	}{
		{"valid", "2026-02-09", time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC), false},
		{"valid leap year", "2024-02-29", time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), false},
		{"invalid format", "02/09/2026", time.Time{}, true},
		{"rfc3339 string", "2026-02-09T10:30:00Z", time.Time{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimestamp(tt.value, TimestampFormatDateOnly)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTimestamp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.want) {
				t.Errorf("ParseTimestamp() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTimestamp_DefaultFormat(t *testing.T) {
	// Empty format should default to rfc3339
	got, err := ParseTimestamp("2026-02-09T10:30:00Z", "")
	if err != nil {
		t.Fatalf("ParseTimestamp() with empty format error = %v", err)
	}
	want := time.Date(2026, 2, 9, 10, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("ParseTimestamp() = %v, want %v", got, want)
	}
}

func TestParseTimestamp_UnknownFormat(t *testing.T) {
	_, err := ParseTimestamp("2026-02-09T10:30:00Z", "epoch_ns")
	if err == nil {
		t.Error("ParseTimestamp() expected error for unknown format")
	}
}

func TestNormalizeTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		format  string
		want    string
		wantErr bool
	}{
		{"rfc3339 passthrough", "2026-02-09T10:30:00Z", "rfc3339", "2026-02-09T10:30:00Z", false},
		{"rfc3339 with offset normalizes to UTC", "2026-02-09T10:30:00+05:00", "rfc3339", "2026-02-09T05:30:00Z", false},
		{"unix to rfc3339", "1739054400", "unix", "2025-02-08T22:40:00Z", false},
		{"unix_ms to rfc3339", "1739054400000", "unix_ms", "2025-02-08T22:40:00Z", false},
		{"date_only to rfc3339", "2026-02-09", "date_only", "2026-02-09T00:00:00Z", false},
		{"empty format defaults to rfc3339", "2026-02-09T10:30:00Z", "", "2026-02-09T10:30:00Z", false},
		{"invalid rfc3339", "not-a-date", "rfc3339", "", true},
		{"invalid unix", "abc", "unix", "", true},
		{"unknown format", "2026-02-09T10:30:00Z", "epoch_ns", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeTimestamp(tt.value, tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeTimestamp() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("NormalizeTimestamp() = %v, want %v", got, tt.want)
			}
		})
	}
}
