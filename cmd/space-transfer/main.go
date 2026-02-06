// space-transfer is a CLI for exporting and importing MDEMG space graphs
// between Neo4j instances (file-based .mdemg format). Use for sharing
// mature space_id data across developer environments.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	pb "mdemg/api/transferpb"
	devspacepb "mdemg/api/devspacepb"
	"mdemg/internal/devspace"
	"mdemg/internal/transfer"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	toolName = "space-transfer"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	sub := strings.ToLower(os.Args[1])
	args := os.Args[2:]

	ctx := context.Background()

	switch sub {
	case "export":
		runExport(ctx, args)
	case "import":
		runImport(ctx, args)
	case "list":
		runList(ctx, args)
	case "info":
		runInfo(ctx, args)
	case "serve":
		runServe(ctx, args)
	case "pull":
		runPull(ctx, args)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand %q\n", sub)
		printUsage()
		os.Exit(1)
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func newDriver() (neo4j.DriverWithContext, error) {
	uri := getEnv("NEO4J_URI", "")
	user := getEnv("NEO4J_USER", "")
	pass := getEnv("NEO4J_PASS", "")
	if uri == "" || user == "" || pass == "" {
		return nil, fmt.Errorf("NEO4J_URI, NEO4J_USER, NEO4J_PASS are required")
	}
	return neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, pass, ""))
}

func runExport(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	spaceID := fs.String("space-id", "", "Space ID to export (required)")
	output := fs.String("output", "", "Output .mdemg file path (default: <space-id>.mdemg)")
	profile := fs.String("profile", "full", "Export profile: full | codebase | cms | learned | metadata")
	repoDir := fs.String("repo", "", "Git repo path; if set, export fails unless repo is clean and up to date with origin/main")
	skipGitCheck := fs.Bool("skip-git-check", false, "Skip pre-export git check even when -repo is set")
	chunkSize := fs.Int("chunk-size", 500, "Nodes per chunk")
	noEmbeddings := fs.Bool("no-embeddings", false, "Exclude embedding vectors to reduce size")
	noObservations := fs.Bool("no-observations", false, "Exclude observations")
	noSymbols := fs.Bool("no-symbols", false, "Exclude symbol nodes")
	noLearnedEdges := fs.Bool("no-learned-edges", false, "Exclude CO_ACTIVATED_WITH edges")
	minLayer := fs.Int("min-layer", 0, "Minimum layer to export (0 = all)")
	maxLayer := fs.Int("max-layer", 0, "Maximum layer to export (0 = all)")
	_ = fs.Parse(args)

	if *spaceID == "" {
		fmt.Fprintln(os.Stderr, "Error: -space-id is required")
		fs.Usage()
		os.Exit(1)
	}
	outPath := *output
	if outPath == "" {
		outPath = *spaceID + ".mdemg"
	}
	if !strings.HasSuffix(outPath, ".mdemg") {
		outPath = outPath + ".mdemg"
	}

	if *repoDir != "" && !*skipGitCheck {
		if err := preExportGitCheck(*repoDir); err != nil {
			log.Fatalf("Pre-export git check failed: %v", err)
		}
	}

	driver, err := newDriver()
	if err != nil {
		log.Fatalf("Neo4j config: %v", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatalf("Neo4j connect: %v", err)
	}

	cfg, err := transfer.ExportConfigForProfile(*spaceID, *profile)
	if err != nil {
		log.Fatalf("Profile: %v", err)
	}
	cfg.ChunkSize = *chunkSize
	cfg.MinLayer = *minLayer
	cfg.MaxLayer = *maxLayer
	if *noEmbeddings {
		cfg.IncludeEmbeddings = false
	}
	if *noObservations {
		cfg.IncludeObservations = false
	}
	if *noSymbols {
		cfg.IncludeSymbols = false
	}
	if *noLearnedEdges {
		cfg.IncludeLearnedEdges = false
		if cfg.OnlyLearnedEdges {
			cfg.OnlyLearnedEdges = false
		}
	}
	cfg.ProgressFunc = func(phase string, done, total int64) {
		if total > 0 {
			fmt.Fprintf(os.Stderr, "  %s: %d/%d\n", phase, done, total)
		} else {
			fmt.Fprintf(os.Stderr, "  %s: %d\n", phase, done)
		}
	}
	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, cfg)
	if err != nil {
		log.Fatalf("Export failed: %v", err)
	}
	if err := transfer.WriteFile(outPath, result); err != nil {
		log.Fatalf("Write file failed: %v", err)
	}
	fmt.Printf("Exported %s to %s (%d chunks)\n", *spaceID, outPath, len(result.Chunks))
}

func runImport(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	input := fs.String("input", "", "Input .mdemg file path (required)")
	conflict := fs.String("conflict", "skip", "On node collision: skip | overwrite | error")
	_ = fs.Parse(args)

	if *input == "" {
		fmt.Fprintln(os.Stderr, "Error: -input is required")
		fs.Usage()
		os.Exit(1)
	}

	var mode pb.ConflictMode
	switch *conflict {
	case "skip":
		mode = pb.ConflictMode_CONFLICT_SKIP
	case "overwrite":
		mode = pb.ConflictMode_CONFLICT_OVERWRITE
	case "error":
		mode = pb.ConflictMode_CONFLICT_ERROR
	default:
		log.Fatalf("Invalid -conflict %q; use skip, overwrite, or error", *conflict)
	}

	chunks, err := transfer.ReadFile(*input)
	if err != nil {
		log.Fatalf("Read file: %v", err)
	}

	driver, err := newDriver()
	if err != nil {
		log.Fatalf("Neo4j config: %v", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatalf("Neo4j connect: %v", err)
	}

	if err := transfer.ValidateImport(ctx, driver, chunks); err != nil {
		log.Fatalf("Import validation failed: %v", err)
	}

	imp := transfer.NewImporter(driver, mode)
	imp.ProgressFunc = func(phase string, done, total int64) {
		if total > 0 {
			fmt.Fprintf(os.Stderr, "  %s: %d/%d\n", phase, done, total)
		} else {
			fmt.Fprintf(os.Stderr, "  %s: %d\n", phase, done)
		}
	}
	result, err := imp.Import(ctx, chunks)
	if err != nil {
		log.Fatalf("Import failed: %v", err)
	}

	for _, w := range result.Warnings {
		fmt.Fprintln(os.Stderr, "Warning:", w)
	}
	fmt.Printf("Import complete: nodes created=%d skipped=%d overwritten=%d edges=%d obs=%d symbols=%d (duration %v)\n",
		result.NodesCreated, result.NodesSkipped, result.NodesOverwritten,
		result.EdgesCreated, result.ObservationsCreated, result.SymbolsCreated, result.Duration)
}

func runList(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	_ = fs.Parse(args)

	driver, err := newDriver()
	if err != nil {
		log.Fatalf("Neo4j config: %v", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatalf("Neo4j connect: %v", err)
	}

	spaces, err := transfer.ListSpaces(ctx, driver)
	if err != nil {
		log.Fatalf("List spaces: %v", err)
	}
	if len(spaces) == 0 {
		fmt.Println("No spaces found.")
		return
	}
	fmt.Println("Space ID          | Nodes   | Max layer")
	fmt.Println("------------------+---------+----------")
	for _, s := range spaces {
		fmt.Printf("%-17s | %7d | %d\n", s.SpaceId, s.NodeCount, s.MaxLayer)
	}
}

func runInfo(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("info", flag.ExitOnError)
	spaceID := fs.String("space-id", "", "Space ID (required)")
	_ = fs.Parse(args)

	if *spaceID == "" {
		fmt.Fprintln(os.Stderr, "Error: -space-id is required")
		fs.Usage()
		os.Exit(1)
	}

	driver, err := newDriver()
	if err != nil {
		log.Fatalf("Neo4j config: %v", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatalf("Neo4j connect: %v", err)
	}

	info, err := transfer.GetSpaceInfo(ctx, driver, *spaceID)
	if err != nil {
		log.Fatalf("Space info: %v", err)
	}
	sum := info.Summary
	fmt.Printf("Space ID:    %s\n", sum.SpaceId)
	fmt.Printf("Nodes:       %d\n", sum.NodeCount)
	fmt.Printf("Edges:       %d\n", sum.EdgeCount)
	fmt.Printf("Observations: %d\n", sum.ObservationCount)
	fmt.Printf("Symbols:     %d\n", sum.SymbolCount)
	fmt.Printf("Max layer:   %d\n", sum.MaxLayer)
	fmt.Printf("Schema:      %d\n", info.SchemaVersion)
	fmt.Printf("Embed dims:  %d\n", info.EmbeddingDimensions)
	if sum.LastUpdated != "" {
		fmt.Printf("Last updated: %s\n", sum.LastUpdated)
	}
	if len(info.EdgeTypes) > 0 {
		fmt.Printf("Edge types:  %s\n", strings.Join(info.EdgeTypes, ", "))
	}
}

func runServe(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 50051, "gRPC listen port")
	enableDevSpace := fs.Bool("enable-devspace", false, "Enable DevSpace hub (RegisterAgent, ListExports, PublishExport, PullExport)")
	devSpaceDataDir := fs.String("devspace-data-dir", ".devspace/data", "Directory for DevSpace export files (used when -enable-devspace)")
	_ = fs.Parse(args)

	driver, err := newDriver()
	if err != nil {
		log.Fatalf("Neo4j config: %v", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		log.Fatalf("Neo4j connect: %v", err)
	}

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(*port))
	if err != nil {
		log.Fatalf("Listen: %v", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	pb.RegisterSpaceTransferServer(grpcServer, transfer.NewGRPCServer(driver))
	if *enableDevSpace {
		catalog, err := devspace.NewCatalog(*devSpaceDataDir)
		if err != nil {
			log.Fatalf("DevSpace catalog: %v", err)
		}
		devspacepb.RegisterDevSpaceServer(grpcServer, devspace.NewServer(catalog, devspace.NewBroker()))
		log.Printf("DevSpace hub enabled (data dir: %s)", *devSpaceDataDir)
	}
	log.Printf("SpaceTransfer gRPC listening on :%d", *port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Serve: %v", err)
	}
}

func runPull(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("pull", flag.ExitOnError)
	remote := fs.String("remote", "", "Remote gRPC address (host:port, required)")
	spaceID := fs.String("space-id", "", "Space ID to pull (required)")
	output := fs.String("output", "", "Output .mdemg file path (default: <space-id>.mdemg)")
	_ = fs.Parse(args)

	if *remote == "" || *spaceID == "" {
		fmt.Fprintln(os.Stderr, "Error: -remote and -space-id are required")
		fs.Usage()
		os.Exit(1)
	}
	outPath := *output
	if outPath == "" {
		outPath = *spaceID + ".mdemg"
	}
	if !strings.HasSuffix(outPath, ".mdemg") {
		outPath = outPath + ".mdemg"
	}

	conn, err := grpc.NewClient(*remote, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Dial %s: %v", *remote, err)
	}
	defer conn.Close()

	client := pb.NewSpaceTransferClient(conn)
	stream, err := client.Export(ctx, &pb.ExportRequest{SpaceId: *spaceID})
	if err != nil {
		log.Fatalf("Export: %v", err)
	}

	var chunks []*pb.SpaceChunk
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Recv: %v", err)
		}
		chunks = append(chunks, chunk)
	}

	result := &transfer.ExportResult{Chunks: chunks}
	if err := transfer.WriteFile(outPath, result); err != nil {
		log.Fatalf("Write file: %v", err)
	}
	fmt.Printf("Pulled %s from %s to %s (%d chunks)\n", *spaceID, *remote, outPath, len(chunks))
}

// preExportGitCheck ensures the repo at dir has a clean working tree and is
// not behind origin/main. Used when sharing spaces from a shared codebase.
func preExportGitCheck(dir string) error {
	// Working tree clean
	out, err := exec.CommandContext(context.Background(), "git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if len(out) > 0 {
		return fmt.Errorf("working tree not clean in %s (commit or stash changes before export)", dir)
	}
	// Fetch origin so we can compare with origin/main
	_ = exec.CommandContext(context.Background(), "git", "-C", dir, "fetch", "origin", "main").Run()
	// Not behind origin/main
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-list", "HEAD..origin/main", "--count")
	revOut, err := cmd.Output()
	if err != nil {
		// origin/main may not exist (e.g. no remote or branch not pushed)
		return fmt.Errorf("cannot compare with origin/main: %w", err)
	}
	count := strings.TrimSpace(string(revOut))
	if count != "" && count != "0" {
		return fmt.Errorf("branch is behind origin/main by %s commit(s); pull before export", count)
	}
	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: %s <subcommand> [flags]

Export/import MDEMG space graphs as .mdemg files or via gRPC. Requires NEO4J_URI, NEO4J_USER, NEO4J_PASS for Neo4j operations.

Subcommands:
  export    Export a space to a .mdemg file
  import    Import a .mdemg file into Neo4j
  list      List all spaces (by node count)
  info      Show detailed info for one space
  serve     Run gRPC server for remote pull (default port 50051)
  pull      Pull a space from a remote gRPC server to a .mdemg file

Examples:
  %s export -space-id demo -output demo.mdemg
  %s import -input demo.mdemg -conflict skip
  %s list
  %s info -space-id demo
  %s serve -port 50051 [-enable-devspace] [-devspace-data-dir .devspace/data]
  %s pull -remote localhost:50051 -space-id demo -output demo.mdemg
`, toolName, toolName, toolName, toolName, toolName, toolName, toolName)
}
