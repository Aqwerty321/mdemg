package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Fetcher handles HTTP requests for web scraping.
type Fetcher struct {
	client           *http.Client
	userAgent        string
	respectRobotsTxt bool
	timeout          time.Duration

	robotsCache   map[string]bool // domain -> allowed
	robotsCacheMu sync.RWMutex
}

// NewFetcher creates a new HTTP fetcher.
func NewFetcher(client *http.Client, userAgent string, respectRobotsTxt bool, timeout time.Duration) *Fetcher {
	return &Fetcher{
		client:           client,
		userAgent:        userAgent,
		respectRobotsTxt: respectRobotsTxt,
		timeout:          timeout,
		robotsCache:      make(map[string]bool),
	}
}

// Fetch retrieves a URL and returns the body bytes and content type.
func (f *Fetcher) Fetch(ctx context.Context, url string, auth *ScrapeAuth) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	// Apply authentication
	if auth != nil {
		switch auth.Type {
		case "cookie":
			for k, v := range auth.Credentials {
				req.AddCookie(&http.Cookie{Name: k, Value: v})
			}
		case "header":
			for k, v := range auth.Credentials {
				req.Header.Set(k, v)
			}
		case "basic":
			user := auth.Credentials["username"]
			pass := auth.Credentials["password"]
			req.SetBasicAuth(user, pass)
		}
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	// Limit read to 10MB to prevent memory issues
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	return body, contentType, nil
}

// CheckRobotsTxt checks if a URL is allowed by robots.txt.
// Returns true if scraping is allowed.
func (f *Fetcher) CheckRobotsTxt(ctx context.Context, url string) bool {
	if !f.respectRobotsTxt {
		return true
	}

	// Extract domain
	domain := extractDomain(url)
	if domain == "" {
		return true
	}

	// Check cache
	f.robotsCacheMu.RLock()
	if allowed, ok := f.robotsCache[domain]; ok {
		f.robotsCacheMu.RUnlock()
		return allowed
	}
	f.robotsCacheMu.RUnlock()

	// Fetch robots.txt
	robotsURL := fmt.Sprintf("%s/robots.txt", domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		f.cacheRobots(domain, true)
		return true
	}
	req.Header.Set("User-Agent", f.userAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		f.cacheRobots(domain, true)
		return true // Default allow if robots.txt unavailable
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		f.cacheRobots(domain, true)
		return true
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	content := string(body)

	// Simple robots.txt parsing: check for Disallow: /
	allowed := !containsDisallowAll(content, f.userAgent)
	f.cacheRobots(domain, allowed)
	return allowed
}

func (f *Fetcher) cacheRobots(domain string, allowed bool) {
	f.robotsCacheMu.Lock()
	f.robotsCache[domain] = allowed
	f.robotsCacheMu.Unlock()
}

// extractDomain extracts the scheme+host from a URL.
func extractDomain(rawURL string) string {
	idx := strings.Index(rawURL, "://")
	if idx == -1 {
		return ""
	}
	rest := rawURL[idx+3:]
	slashIdx := strings.Index(rest, "/")
	if slashIdx == -1 {
		return rawURL
	}
	return rawURL[:idx+3+slashIdx]
}

// containsDisallowAll checks if robots.txt blocks the given user agent from /.
func containsDisallowAll(content, userAgent string) bool {
	lines := strings.Split(content, "\n")
	inRelevantBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}

		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "user-agent:") {
			agent := strings.TrimSpace(line[len("user-agent:"):])
			inRelevantBlock = agent == "*" || strings.EqualFold(agent, userAgent)
		}

		if inRelevantBlock && strings.HasPrefix(lower, "disallow:") {
			path := strings.TrimSpace(line[len("disallow:"):])
			if path == "/" {
				return true
			}
		}
	}
	return false
}
