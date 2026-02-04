package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"mdemg/internal/api"
	"mdemg/internal/config"
	"mdemg/internal/db"
	"mdemg/internal/plugins"
)

func main() {
	// Load .env file if present (silently ignore if not found)
	_ = godotenv.Load()

	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	driver, err := db.NewDriver(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer driver.Close(context.Background())

	// Readiness check: schema version
	if err := db.AssertSchemaVersion(context.Background(), driver, cfg.RequiredSchemaVersion); err != nil {
		log.Fatalf("schema version check failed: %v", err)
	}

	// Initialize Plugin Manager if enabled
	var pluginMgr *plugins.Manager
	if cfg.PluginsEnabled {
		pluginMgr = plugins.NewManager(cfg.PluginsDir, cfg.PluginSocketDir, cfg.MdemgVersion)
		if err := pluginMgr.Start(); err != nil {
			log.Printf("warning: failed to start plugin manager: %v", err)
			// Continue without plugins - this is not fatal
		} else {
			log.Printf("plugin manager started (dir=%s)", cfg.PluginsDir)
		}
	}

	srv := api.NewServer(cfg, driver, pluginMgr)

	// Start periodic conversation memory consolidation (every 5 minutes)
	srv.StartPeriodicConsolidation("mdemg-dev", 5*time.Minute)

	// Start scheduled sync if configured (Phase 9.2)
	if cfg.SyncIntervalMinutes > 0 {
		srv.StartScheduledSync(time.Duration(cfg.SyncIntervalMinutes) * time.Minute)
	}

	h := &http.Server{
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(cfg.HTTPReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(cfg.HTTPWriteTimeout) * time.Second,
	}

	// Dynamic port allocation: try preferred port, then scan range
	listener, err := listenWithFallback(cfg)
	if err != nil {
		log.Fatalf("failed to bind: %v", err)
	}

	// Extract actual port and write port file for client discovery
	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	portFile, _ := filepath.Abs(cfg.PortFilePath)
	if err := writePortFile(portFile, portStr); err != nil {
		log.Printf("warning: failed to write port file %s: %v", portFile, err)
	} else {
		log.Printf("port file written: %s", portFile)
	}

	// Set up graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("MDEMG server started on http://localhost:%s", portStr)
		if err := h.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-shutdown
	log.Println("shutdown signal received")

	// Remove port file
	os.Remove(portFile)

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Stop plugin manager first
	if pluginMgr != nil {
		log.Println("stopping plugin manager...")
		if err := pluginMgr.Stop(); err != nil {
			log.Printf("error stopping plugin manager: %v", err)
		}
	}

	// Shutdown HTTP server
	log.Println("shutting down HTTP server...")
	if err := h.Shutdown(ctx); err != nil {
		log.Printf("error shutting down HTTP server: %v", err)
		os.Exit(1)
	}

	log.Println("shutdown complete")
}

// listenWithFallback tries the preferred address first, then scans the port range.
func listenWithFallback(cfg config.Config) (net.Listener, error) {
	// Try preferred address first
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err == nil {
		return ln, nil
	}

	// Only fallback if the error is "address already in use"
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return nil, fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}
	if !errors.Is(opErr.Err, syscall.EADDRINUSE) {
		return nil, fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}

	log.Printf("preferred address %s in use, scanning range %d-%d",
		cfg.ListenAddr, cfg.PortRangeStart, cfg.PortRangeEnd)

	for port := cfg.PortRangeStart; port <= cfg.PortRangeEnd; port++ {
		addr := fmt.Sprintf(":%d", port)
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
	}

	return nil, fmt.Errorf("no available port in range %d-%d", cfg.PortRangeStart, cfg.PortRangeEnd)
}

// writePortFile writes the port number atomically (write tmp, then rename).
func writePortFile(path, port string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(port+"\n"), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
