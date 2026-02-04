package retrieval

import (
	"regexp"
	"strings"
	"time"
)

// maxScanChars limits date extraction to the header/metadata area of content.
const maxScanChars = 500

// dateMinYear rejects dates before this year as implausible for content timestamps.
const dateMinYear = 2000

// changelogDateRe matches changelog version date patterns: "## [1.2.3] - 2026-01-15"
var changelogDateRe = regexp.MustCompile(`(?m)^##\s+\[[\d.]+\]\s*[-–—]\s*(\d{4}-\d{2}-\d{2})`)

// versionDateRe matches version release date patterns: "Version 1.2.3 (2026-01-15)"
var versionDateRe = regexp.MustCompile(`(?i)(?:version|release|v)\s*[\d.]+\s*\((\d{4}-\d{2}-\d{2})\)`)

// metadataDateRe matches metadata date fields: "Date: 2026-01-15", "Created: 2026-01-15"
var metadataDateRe = regexp.MustCompile(`(?im)^(?:date|created|modified|updated|last[- ]modified|last[- ]updated)\s*:\s*(\d{4}-\d{2}-\d{2})`)

// isoDateRe matches standalone ISO dates: "2026-01-15"
var isoDateRe = regexp.MustCompile(`\b(\d{4}-\d{2}-\d{2})\b`)

// naturalDateRe matches natural dates: "January 15, 2026"
var naturalDateRe = regexp.MustCompile(`(?i)\b(` + monthPattern + `)\s+(\d{1,2}),?\s+(\d{4})\b`)

// ExtractCanonicalTime attempts to extract the most relevant date from content text.
// Returns zero time if no date found. Only scans first 500 chars (header/metadata area).
// Priority: changelog patterns > version date > metadata patterns > ISO dates > natural dates.
func ExtractCanonicalTime(content string, tags []string) time.Time {
	if content == "" {
		return time.Time{}
	}

	// Limit scan area to header/metadata region
	scanArea := content
	if len(scanArea) > maxScanChars {
		scanArea = scanArea[:maxScanChars]
	}

	// Priority 1: Changelog version date (scan full content for this pattern)
	if t := extractChangelogDate(content); !t.IsZero() {
		return t
	}

	// Priority 2: Version release date
	if t := extractVersionDate(scanArea); !t.IsZero() {
		return t
	}

	// Priority 3: Metadata date fields
	if t := extractMetadataDate(scanArea); !t.IsZero() {
		return t
	}

	// Priority 4: ISO 8601 date in header area
	if t := extractISODate(scanArea); !t.IsZero() {
		return t
	}

	// Priority 5: Natural date
	if t := extractNaturalDate(scanArea); !t.IsZero() {
		return t
	}

	return time.Time{}
}

// extractChangelogDate finds changelog version dates like "## [2.1.0] - 2026-01-15".
// Scans full content since changelogs may have headers before the first entry,
// but only returns the first (most recent) match.
func extractChangelogDate(content string) time.Time {
	// Only scan first 500 chars for changelog pattern too
	scanArea := content
	if len(scanArea) > maxScanChars {
		scanArea = scanArea[:maxScanChars]
	}
	m := changelogDateRe.FindStringSubmatch(scanArea)
	if m == nil {
		return time.Time{}
	}
	return parseAndValidate(m[1])
}

// extractVersionDate finds version release dates like "Version 1.0 (2025-12-01)".
func extractVersionDate(content string) time.Time {
	m := versionDateRe.FindStringSubmatch(content)
	if m == nil {
		return time.Time{}
	}
	return parseAndValidate(m[1])
}

// extractMetadataDate finds metadata date fields like "Date: 2026-01-15".
func extractMetadataDate(content string) time.Time {
	m := metadataDateRe.FindStringSubmatch(content)
	if m == nil {
		return time.Time{}
	}
	return parseAndValidate(m[1])
}

// extractISODate finds the first ISO 8601 date (YYYY-MM-DD) in the header area.
func extractISODate(content string) time.Time {
	m := isoDateRe.FindStringSubmatch(content)
	if m == nil {
		return time.Time{}
	}
	return parseAndValidate(m[1])
}

// extractNaturalDate finds natural language dates like "January 15, 2026".
func extractNaturalDate(content string) time.Time {
	m := naturalDateRe.FindStringSubmatch(content)
	if m == nil {
		return time.Time{}
	}

	month := parseMonth(strings.ToLower(m[1]))
	if month == 0 {
		return time.Time{}
	}

	day := 0
	for _, c := range m[2] {
		day = day*10 + int(c-'0')
	}
	year := 0
	for _, c := range m[3] {
		year = year*10 + int(c-'0')
	}

	if day < 1 || day > 31 {
		return time.Time{}
	}

	t := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if !isValidDate(t) {
		return time.Time{}
	}
	return t
}

// parseAndValidate parses an ISO date string and validates it within acceptable range.
func parseAndValidate(dateStr string) time.Time {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}
	}
	if !isValidDate(t) {
		return time.Time{}
	}
	return t
}

// isValidDate checks that a date is within acceptable range:
// - Not before year 2000
// - Not more than 1 year in the future
func isValidDate(t time.Time) bool {
	if t.Year() < dateMinYear {
		return false
	}
	maxFuture := time.Now().AddDate(1, 0, 0)
	if t.After(maxFuture) {
		return false
	}
	return true
}
