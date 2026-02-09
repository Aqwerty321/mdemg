package models

import (
	"fmt"
	"strconv"
	"time"
)

const (
	TimestampFormatRFC3339  = "rfc3339"
	TimestampFormatUnix     = "unix"
	TimestampFormatUnixMs   = "unix_ms"
	TimestampFormatDateOnly = "date_only"
)

// ParseTimestamp parses a timestamp string according to the given format enum.
// An empty format defaults to rfc3339.
func ParseTimestamp(value, format string) (time.Time, error) {
	if format == "" {
		format = TimestampFormatRFC3339
	}

	switch format {
	case TimestampFormatRFC3339:
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("timestamp '%s' is not valid rfc3339 format. Expected: 2006-01-02T15:04:05Z07:00", value)
		}
		return t, nil

	case TimestampFormatUnix:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("timestamp '%s' is not valid unix format. Expected: integer seconds since epoch", value)
		}
		return time.Unix(n, 0).UTC(), nil

	case TimestampFormatUnixMs:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("timestamp '%s' is not valid unix_ms format. Expected: integer milliseconds since epoch", value)
		}
		return time.UnixMilli(n).UTC(), nil

	case TimestampFormatDateOnly:
		t, err := time.Parse("2006-01-02", value)
		if err != nil {
			return time.Time{}, fmt.Errorf("timestamp '%s' is not valid date_only format. Expected: 2006-01-02", value)
		}
		return t, nil

	default:
		return time.Time{}, fmt.Errorf("unknown timestamp_format '%s'", format)
	}
}

// NormalizeTimestamp parses a timestamp according to the given format and returns
// the RFC3339 normalized string. This is the main entry point for handlers.
func NormalizeTimestamp(value, format string) (string, error) {
	t, err := ParseTimestamp(value, format)
	if err != nil {
		return "", err
	}
	return t.UTC().Format(time.RFC3339), nil
}
