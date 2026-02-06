# space-transfer

CLI for exporting and importing MDEMG space graphs as `.mdemg` files. Use this to share mature `space_id` data between developer environments (e.g. export from your Neo4j, send the file, import into a teammate’s Neo4j).

## Requirements

- `NEO4J_URI`, `NEO4J_USER`, `NEO4J_PASS` environment variables set for the Neo4j instance you are reading from (export/list/info) or writing to (import).

## Build

```bash
go build -o space-transfer ./cmd/space-transfer/
```

## Subcommands

### export

Export a space to a `.mdemg` file.

```bash
space-transfer export -space-id <space_id> [-output <path>] [options]
```

- **-space-id** (required): Space to export.
- **-output**: Output file path (default: `<space-id>.mdemg`).
- **-profile**: Export profile: `full` (default) | `codebase` | `cms` | `learned` | `metadata`.
  - **full**: All nodes, edges, observations, symbols, embeddings.
  - **codebase**: Nodes, edges, symbols, learned edges; no observations (code structure + learning).
  - **cms**: Observations, nodes, edges; no symbols (conversation memory).
  - **learned**: Only CO_ACTIVATED_WITH edges + all nodes (learning graph only).
  - **metadata**: Only metadata + summary chunks (counts, schema; no entity data).
- **-repo**: Git repo path; if set, export fails unless working tree is clean and branch is up to date with `origin/main`. Use with shared codebases.
- **-skip-git-check**: Skip pre-export git check even when `-repo` is set.
- **-chunk-size**: Nodes per chunk (default: 500).
- **-no-embeddings**: Omit embedding vectors (smaller file).
- **-no-observations**: Omit observation nodes.
- **-no-symbols**: Omit symbol nodes.
- **-no-learned-edges**: Omit CO_ACTIVATED_WITH edges.
- **-min-layer**, **-max-layer**: Layer range (0 = no limit).

### import

Import a `.mdemg` file into the current Neo4j.

```bash
space-transfer import -input <path> [-conflict skip|overwrite|error]
```

- **-input** (required): Path to `.mdemg` file.
- **-conflict**: On node ID collision: `skip` (default), `overwrite`, or `error`.

Schema version of the export must be ≤ local Neo4j schema version; run migrations first if needed.

### list

List all spaces in Neo4j (by node count).

```bash
space-transfer list
```

### info

Show detailed metadata for one space (for pre-import validation).

```bash
space-transfer info -space-id <space_id>
```

### serve

Run the SpaceTransfer gRPC server so remote clients can pull spaces. Optionally enable the DevSpace hub for agent registration and out-of-band export distribution (Phase 2–3).

```bash
space-transfer serve [-port 50051] [-enable-devspace] [-devspace-data-dir .devspace/data]
```

- **-port**: Listen port (default: 50051). Requires NEO4J_URI, NEO4J_USER, NEO4J_PASS.
- **-enable-devspace**: Register the DevSpace service (RegisterAgent, ListExports, PublishExport, PullExport, Connect). Same port as SpaceTransfer.
- **-devspace-data-dir**: Directory for published export files when DevSpace is enabled (default: `.devspace/data`).

### pull

Pull a space from a remote gRPC server and save it as a `.mdemg` file.

```bash
space-transfer pull -remote <host:port> -space-id <space_id> [-output <path>]
```

- **-remote** (required): Remote gRPC address (e.g. `localhost:50051`).
- **-space-id** (required): Space to export from the remote server.
- **-output**: Output file path (default: `<space-id>.mdemg`). Does not require Neo4j env vars.

## Examples

```bash
# Export space "demo" to demo.mdemg
space-transfer export -space-id demo -output demo.mdemg

# Import into another Neo4j (skip existing nodes)
space-transfer import -input demo.mdemg -conflict skip

# List spaces
space-transfer list

# Inspect a space before importing
space-transfer info -space-id demo

# Run gRPC server (on machine with Neo4j); with DevSpace hub for agent messaging
space-transfer serve -port 50051 -enable-devspace

# Pull a space from a remote server to a file
space-transfer pull -remote localhost:50051 -space-id demo -output demo.mdemg
```

## File format

`.mdemg` files are JSON: a header (`format`, `version`) plus an array of chunks (metadata, nodes, edges, observations, symbols, summary). The format is defined by `api/proto/space-transfer.proto` and serialized via `internal/transfer/format.go`.
