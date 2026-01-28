package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/api"
	"mdemg/internal/config"
	"mdemg/internal/db"
)

func main() {
	spaceID := flag.String("space-id", "", "Delete only this space ID (required unless --all is set)")
	deleteAll := flag.Bool("all", false, "Delete all non-protected spaces (requires confirmation)")
	skipConfirm := flag.Bool("yes", false, "Skip confirmation prompt")
	flag.Parse()

	if *spaceID == "" && !*deleteAll {
		fmt.Fprintln(os.Stderr, "Usage: reset-db --space-id <space> [--yes]")
		fmt.Fprintln(os.Stderr, "       reset-db --all [--yes]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Protected spaces (never deleted):")
		for id := range api.ProtectedSpaces {
			fmt.Fprintf(os.Stderr, "  - %s\n", id)
		}
		os.Exit(1)
	}

	// Reject attempts to delete protected spaces
	if *spaceID != "" && api.ProtectedSpaces[*spaceID] {
		fmt.Fprintf(os.Stderr, "ERROR: space '%s' is protected and cannot be deleted.\n", *spaceID)
		os.Exit(1)
	}

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

	// Confirmation prompt
	if !*skipConfirm {
		target := *spaceID
		if *deleteAll {
			target = "ALL non-protected spaces"
		}
		fmt.Printf("⚠ This will DELETE all nodes in: %s\n", target)
		fmt.Printf("Protected spaces (safe): %v\n", protectedSpaceList())
		fmt.Print("Type 'yes' to confirm: ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		if strings.TrimSpace(scanner.Text()) != "yes" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
	}

	if *spaceID != "" {
		fmt.Printf("[STATUS %s] Starting delete for space: %s\n", ts(), *spaceID)
	} else {
		fmt.Printf("[STATUS %s] Starting database clear (all non-protected)...\n", ts())
	}

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
	fmt.Printf("[STATUS %s] Initial node count: %d\n", ts(), initialCount)

	if initialCount == 0 {
		fmt.Printf("[STATUS %s] Database is already empty\n", ts())
		return
	}

	// Print protected spaces
	psList := protectedSpaceList()
	if len(psList) > 0 {
		fmt.Printf("[STATUS %s] PROTECTED SPACES (will NOT be deleted): %v\n", ts(), psList)
	}

	// Delete in batches to avoid transaction limits
	batchSize := 10000
	totalDeleted := int64(0)

	for {
		sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})

		var cypher string
		if *spaceID != "" {
			// Targeted: delete only from specified space
			cypher = fmt.Sprintf(`
				MATCH (n)
				WHERE n.space_id = '%s'
				WITH n LIMIT %d
				DETACH DELETE n
				RETURN count(*) as deleted
			`, *spaceID, batchSize)
		} else {
			// All mode: exclude protected spaces
			if len(psList) > 0 {
				exclusions := make([]string, len(psList))
				for i, sid := range psList {
					exclusions[i] = fmt.Sprintf("n.space_id <> '%s'", sid)
				}
				whereClause := strings.Join(exclusions, " AND ")
				cypher = fmt.Sprintf(`
					MATCH (n)
					WHERE (%s) OR n.space_id IS NULL
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
		fmt.Printf("[STATUS %s] Deleted batch: %d (total: %d)\n", ts(), deleted, totalDeleted)

		if deleted == 0 {
			break
		}
	}

	// Verify
	fmt.Printf("[STATUS %s] Verifying...\n", ts())
	sess = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	result, err = sess.Run(ctx, "MATCH (n) RETURN count(n) as nodes", nil)
	if err != nil {
		log.Fatal(err)
	}

	if result.Next(ctx) {
		count := result.Record().Values[0]
		fmt.Printf("[STATUS %s] Remaining nodes: %v\n", ts(), count)
		if count.(int64) == 0 {
			fmt.Printf("[STATUS %s] Database cleared successfully!\n", ts())
		} else {
			fmt.Println("[WARNING] Some nodes may remain - check for constraints or protected spaces")
		}
	}
}

func ts() string {
	return time.Now().Format("15:04:05")
}

func protectedSpaceList() []string {
	var list []string
	for id := range api.ProtectedSpaces {
		list = append(list, id)
	}
	return list
}
