// Command extract-symbols extracts code symbols from a codebase using tree-sitter
// and stores them in Neo4j for evidence-locked retrieval.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"mdemg/internal/symbols"
)

var (
	codebasePath = flag.String("path", "", "Path to codebase to extract symbols from")
	spaceID      = flag.String("space-id", "codebase", "MDEMG space ID")
	neo4jURI     = flag.String("neo4j-uri", "bolt://localhost:7687", "Neo4j URI")
	neo4jUser    = flag.String("neo4j-user", "neo4j", "Neo4j username")
	neo4jPass    = flag.String("neo4j-pass", "testpassword", "Neo4j password")
	workers      = flag.Int("workers", 8, "Number of parallel workers")
	dryRun       = flag.Bool("dry-run", false, "Print what would be extracted without storing")
	verbose      = flag.Bool("verbose", false, "Verbose output")
	excludeDirs  = flag.String("exclude", ".git,node_modules,vendor,test,__tests__,.vscode", "Comma-separated directories to exclude")
)

func main() {
	flag.Parse()

	if *codebasePath == "" {
		log.Fatal("--path is required")
	}

	excludeSet := make(map[string]bool)
	for _, dir := range strings.Split(*excludeDirs, ",") {
		excludeSet[strings.TrimSpace(dir)] = true
	}

	log.Printf("=== Symbol Extraction ===")
	log.Printf("Path: %s", *codebasePath)
	log.Printf("Space ID: %s", *spaceID)
	log.Printf("Neo4j: %s", *neo4jURI)
	log.Printf("Workers: %d", *workers)

	// Initialize symbol service (tree-sitter parser)
	cfg := symbols.ParserConfig{
		IncludeDocComments: true,
		EvaluateConstants:  true,
		IncludePrivate:     true, // Include class static fields
	}
	svc, err := symbols.NewService(cfg)
	if err != nil {
		log.Fatalf("Failed to create symbol service: %v", err)
	}
	defer svc.Close()

	ctx := context.Background()

	// Initialize symbol store (only if not dry-run)
	var store *symbols.Store
	var driver neo4j.DriverWithContext
	if !*dryRun {
		driver, err = neo4j.NewDriverWithContext(*neo4jURI, neo4j.BasicAuth(*neo4jUser, *neo4jPass, ""))
		if err != nil {
			log.Fatalf("Failed to create Neo4j driver: %v", err)
		}
		defer driver.Close(ctx)
		store = symbols.NewStore(driver)
	}

	// Collect files to process
	var files []string
	err = filepath.Walk(*codebasePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if excludeSet[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process supported files
		ext := filepath.Ext(path)
		lang := symbols.LanguageFromExtension(ext)
		if lang != symbols.LangUnknown {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to walk codebase: %v", err)
	}

	log.Printf("Found %d files to process", len(files))

	// Process files with worker pool
	fileChan := make(chan string, len(files))
	var wg sync.WaitGroup

	var totalSymbols int64
	var totalFiles int64
	var errors int64
	startTime := time.Now()

	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for path := range fileChan {
				result, err := svc.ParseFile(ctx, path)
				if err != nil {
					atomic.AddInt64(&errors, 1)
					if *verbose {
						log.Printf("[Worker %d] Error parsing %s: %v", workerID, path, err)
					}
					continue
				}

				if len(result.Symbols) == 0 {
					continue
				}

				atomic.AddInt64(&totalFiles, 1)
				atomic.AddInt64(&totalSymbols, int64(len(result.Symbols)))

				relPath, _ := filepath.Rel(*codebasePath, path)
				relPath = "/" + relPath // Make it absolute-like

				if *dryRun || *verbose {
					for _, sym := range result.Symbols {
						log.Printf("[Worker %d] %s: %s (%s) = %s", workerID, relPath, sym.Name, sym.Type, sym.Value)
					}
				}

				if !*dryRun && store != nil {
					// Convert to SymbolRecords
					var records []symbols.SymbolRecord
					for _, sym := range result.Symbols {
						record := symbols.SymbolRecord{
							SpaceID:        *spaceID,
							SymbolID:       symbols.GenerateSymbolID(*spaceID, relPath, sym.Name, sym.LineNumber),
							Name:           sym.Name,
							SymbolType:     string(sym.Type),
							Value:          sym.Value,
							RawValue:       sym.RawValue,
							FilePath:       relPath,
							LineNumber:     sym.LineNumber,
							EndLine:        sym.EndLine,
							Exported:       sym.Exported,
							DocComment:     sym.DocComment,
							Signature:      sym.Signature,
							Parent:         sym.Parent,
							Language:       string(sym.Language),
							TypeAnnotation: sym.TypeAnnotation,
						}
						records = append(records, record)
					}

					if err := store.SaveSymbols(ctx, *spaceID, records); err != nil {
						atomic.AddInt64(&errors, 1)
						if *verbose {
							log.Printf("[Worker %d] Failed to store symbols for %s: %v", workerID, relPath, err)
						}
					}
				}

				// Progress log every 100 files
				current := atomic.LoadInt64(&totalFiles)
				if current%100 == 0 {
					elapsed := time.Since(startTime)
					rate := float64(current) / elapsed.Seconds()
					log.Printf("Progress: %d files, %d symbols (%.1f files/s)",
						current, atomic.LoadInt64(&totalSymbols), rate)
				}
			}
		}(w)
	}

	// Send files to workers
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	wg.Wait()

	elapsed := time.Since(startTime)
	log.Printf("=== Extraction Complete ===")
	log.Printf("Files: %d, Symbols: %d, Errors: %d", totalFiles, totalSymbols, errors)
	log.Printf("Time: %v, Rate: %.1f files/sec", elapsed, float64(totalFiles)/elapsed.Seconds())
}
