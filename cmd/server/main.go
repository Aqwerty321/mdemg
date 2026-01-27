package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	h := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(cfg.HTTPReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(cfg.HTTPWriteTimeout) * time.Second,
	}

	// Set up graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("listening on %s", cfg.ListenAddr)
		if err := h.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-shutdown
	log.Println("shutdown signal received")

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
