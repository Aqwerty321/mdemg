package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/config"
	"mdemg/internal/db"
)

func main() {
	// Load env
	_ = godotenv.Load()

	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	driver, err := db.NewDriver(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer driver.Close(ctx)

	fmt.Println("[STATUS " + time.Now().Format("15:04:05") + "] Starting database clear...")

	// Create session and delete all nodes
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	result, err := sess.Run(ctx, "MATCH (n) DETACH DELETE n RETURN count(*) as deleted", nil)
	if err != nil {
		log.Fatal(err)
	}

	if result.Next(ctx) {
		deleted := result.Record().Values[0]
		fmt.Printf("[STATUS %s] Database cleared - deleted %v nodes\n", time.Now().Format("15:04:05"), deleted)
	}

	// Verify
	fmt.Println("[STATUS " + time.Now().Format("15:04:05") + "] Verifying...")
	result, err = sess.Run(ctx, "MATCH (n) RETURN count(n) as nodes", nil)
	if err != nil {
		log.Fatal(err)
	}

	if result.Next(ctx) {
		count := result.Record().Values[0]
		fmt.Printf("[STATUS %s] Remaining nodes: %v\n", time.Now().Format("15:04:05"), count)
	}
}
