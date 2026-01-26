// Package cache provides a distributed caching service with Redis backend.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrCacheMiss indicates a cache miss.
var ErrCacheMiss = errors.New("cache miss")

// Config holds cache service configuration.
type Config struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	DefaultTTL    time.Duration
	MaxRetries    int
	PoolSize      int
}

// Service provides caching operations with Redis.
type Service struct {
	client     *redis.Client
	defaultTTL time.Duration
	mu         sync.RWMutex
	stats      Stats
}

// Stats tracks cache statistics.
type Stats struct {
	Hits       int64
	Misses     int64
	Sets       int64
	Deletes    int64
	Errors     int64
	LastError  error
	LastUpdate time.Time
}

// NewService creates a new cache service.
func NewService(cfg Config) (*Service, error) {
	if cfg.DefaultTTL == 0 {
		cfg.DefaultTTL = time.Hour
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.PoolSize == 0 {
		cfg.PoolSize = 10
	}

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		MaxRetries:   cfg.MaxRetries,
		PoolSize:     cfg.PoolSize,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &Service{
		client:     client,
		defaultTTL: cfg.DefaultTTL,
	}, nil
}

// Get retrieves a value from cache.
func (s *Service) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			s.recordMiss()
			return ErrCacheMiss
		}
		s.recordError(err)
		return fmt.Errorf("cache get: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		s.recordError(err)
		return fmt.Errorf("cache unmarshal: %w", err)
	}

	s.recordHit()
	return nil
}

// Set stores a value in cache with default TTL.
func (s *Service) Set(ctx context.Context, key string, value interface{}) error {
	return s.SetWithTTL(ctx, key, value, s.defaultTTL)
}

// SetWithTTL stores a value in cache with custom TTL.
func (s *Service) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		s.recordError(err)
		return fmt.Errorf("cache marshal: %w", err)
	}

	if err := s.client.Set(ctx, key, data, ttl).Err(); err != nil {
		s.recordError(err)
		return fmt.Errorf("cache set: %w", err)
	}

	s.recordSet()
	return nil
}

// Delete removes a key from cache.
func (s *Service) Delete(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, key).Err(); err != nil {
		s.recordError(err)
		return fmt.Errorf("cache delete: %w", err)
	}

	s.recordDelete()
	return nil
}

// DeletePattern removes all keys matching a pattern.
func (s *Service) DeletePattern(ctx context.Context, pattern string) (int64, error) {
	var deleted int64

	iter := s.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		if err := s.client.Del(ctx, key).Err(); err != nil {
			continue
		}
		deleted++
	}

	if err := iter.Err(); err != nil {
		s.recordError(err)
		return deleted, fmt.Errorf("cache delete pattern: %w", err)
	}

	return deleted, nil
}

// Exists checks if a key exists in cache.
func (s *Service) Exists(ctx context.Context, key string) (bool, error) {
	count, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		s.recordError(err)
		return false, fmt.Errorf("cache exists: %w", err)
	}
	return count > 0, nil
}

// GetOrSet retrieves a value from cache, or calls loader and stores result.
func (s *Service) GetOrSet(ctx context.Context, key string, dest interface{}, loader func() (interface{}, error)) error {
	// Try to get from cache first
	err := s.Get(ctx, key, dest)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrCacheMiss) {
		return err
	}

	// Load fresh data
	value, err := loader()
	if err != nil {
		return fmt.Errorf("loader: %w", err)
	}

	// Store in cache
	if err := s.Set(ctx, key, value); err != nil {
		// Log but don't fail - we have the data
		s.recordError(err)
	}

	// Copy value to destination
	data, _ := json.Marshal(value)
	return json.Unmarshal(data, dest)
}

// Invalidate invalidates cache entries by prefix.
func (s *Service) Invalidate(ctx context.Context, prefix string) error {
	_, err := s.DeletePattern(ctx, prefix+"*")
	return err
}

// Stats returns current cache statistics.
func (s *Service) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// Close closes the Redis connection.
func (s *Service) Close() error {
	return s.client.Close()
}

func (s *Service) recordHit() {
	s.mu.Lock()
	s.stats.Hits++
	s.stats.LastUpdate = time.Now()
	s.mu.Unlock()
}

func (s *Service) recordMiss() {
	s.mu.Lock()
	s.stats.Misses++
	s.stats.LastUpdate = time.Now()
	s.mu.Unlock()
}

func (s *Service) recordSet() {
	s.mu.Lock()
	s.stats.Sets++
	s.stats.LastUpdate = time.Now()
	s.mu.Unlock()
}

func (s *Service) recordDelete() {
	s.mu.Lock()
	s.stats.Deletes++
	s.stats.LastUpdate = time.Now()
	s.mu.Unlock()
}

func (s *Service) recordError(err error) {
	s.mu.Lock()
	s.stats.Errors++
	s.stats.LastError = err
	s.stats.LastUpdate = time.Now()
	s.mu.Unlock()
}

// CacheKey builds a cache key from components.
func CacheKey(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	key := parts[0]
	for _, part := range parts[1:] {
		key += ":" + part
	}
	return key
}
