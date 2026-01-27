package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/api"
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

	// Get initial count
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	result, err := sess.Run(ctx, "MATCH (n) RETURN count(n) as nodes", nil)
	if err != nil {
		log.Fatal(err)
	}
	initialCount := int64(0)
	if result.Next(ctx) {
		initialCount = result.Record().Values[0].(int64)
	}
	sess.Close(ctx)
	fmt.Printf("[STATUS %s] Initial node count: %d\n", time.Now().Format("15:04:05"), initialCount)

	if initialCount == 0 {
		fmt.Println("[STATUS " + time.Now().Format("15:04:05") + "] Database is already empty")
		return
	}

	// Build list of protected space IDs for exclusion
	var protectedSpaceList []string
	for spaceID := range api.ProtectedSpaces {
		protectedSpaceList = append(protectedSpaceList, spaceID)
	}
	if len(protectedSpaceList) > 0 {
		fmt.Printf("[STATUS %s] PROTECTED SPACES (will NOT be deleted): %v\n",
			time.Now().Format("15:04:05"), protectedSpaceList)
	}

	// Delete in batches to avoid transaction limits
	batchSize := 10000
	totalDeleted := int64(0)

	for {
		sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})

		// Use CALL {} IN TRANSACTIONS for large deletes (Neo4j 4.4+)
		// Fallback to simple batch delete
		// EXCLUDE nodes from protected spaces
		var cypher string
		if len(protectedSpaceList) > 0 {
			// Build exclusion clause
			exclusions := make([]string, len(protectedSpaceList))
			for i, spaceID := range protectedSpaceList {
				exclusions[i] = fmt.Sprintf("n.space_id <> '%s'", spaceID)
			}
			whereClause := strings.Join(exclusions, " AND ")
			cypher = fmt.Sprintf(`
				MATCH (n)
				WHERE %s OR n.space_id IS NULL
				WITH n LIMIT %d
				DETACH DELETE n
				RETURN count(*) as deleted
			`, whereClause, batchSize)
		} else {
			cypher = fmt.Sprintf(`
				MATCH (n)
				WITH n LIMIT %d
				DETACH DELETE n
				RETURN count(*) as deleted
			`, batchSize)
		}

		result, err := sess.Run(ctx, cypher, nil)
		if err != nil {
			sess.Close(ctx)
			log.Fatal(err)
		}

		deleted := int64(0)
		if result.Next(ctx) {
			deleted = result.Record().Values[0].(int64)
		}
		sess.Close(ctx)

		totalDeleted += deleted
		fmt.Printf("[STATUS %s] Deleted batch: %d (total: %d)\n", time.Now().Format("15:04:05"), deleted, totalDeleted)

		if deleted == 0 {
			break
		}
	}

	// Verify
	fmt.Println("[STATUS " + time.Now().Format("15:04:05") + "] Verifying...")
	sess = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err = sess.Run(ctx, "MATCH (n) RETURN count(n) as nodes", nil)
	if err != nil {
		log.Fatal(err)
	}

	if result.Next(ctx) {
		count := result.Record().Values[0]
		fmt.Printf("[STATUS %s] Remaining nodes: %v\n", time.Now().Format("15:04:05"), count)
		if count.(int64) == 0 {
			fmt.Println("[STATUS " + time.Now().Format("15:04:05") + "] Database cleared successfully!")
		} else {
			fmt.Println("[WARNING] Some nodes may remain - check for constraints")
		}
	}
}
