# UNTS — Universal Hash Test Specification (Hash Verification Module)

**Alias:** Nash Verification module  
**Parent:** [FRAMEWORK_GOVERNANCE.md](./FRAMEWORK_GOVERNANCE.md)  
**Status:** Spec (implementation not started)  
**Date:** 2026-01-22

---

## Purpose

Maintain a **current and historical record** of hash verification for all MDEMG files protected by hash verification across frameworks: **UPTS**, **UATS**, **UBTS**, **USTS**, **UOTS**, **UAMS**, and **UDTS**. Provide a single registry, API/gRPC surface for monitoring, observability, and manipulation (including revert to previous hash) so the UNTS framework functions correctly in production.

---

## Scope

- **In scope:** Any file or artifact in the MDEMG application that is protected by hash verification, including:
  - **Manifest:** `docs/specs/manifest.sha256` (spec docs and other tracked files)
  - **UDTS:** `config.proto_sha256` in `docs/api/api-spec/udts/specs/*.udts.json` (proto files)
  - **UATS / UBTS / USTS / UOTS / UAMS:** Future hash fields in their specs (when adopted)
  - **UPTS:** Parser/spec hashes where applicable
- **Out of scope:** General file integrity outside the above frameworks (e.g. arbitrary binaries).

---

## Data Model

### Verified file record

| Field | Type | Description |
|-------|------|-------------|
| `path` | string | Repository-relative path (e.g. `api/proto/devspace.proto`, `docs/specs/space-transfer.md`) |
| `framework` | string | One of: `manifest`, `udts`, `uats`, `ubts`, `usts`, `uots`, `uams`, `upts` |
| `current_hash` | string | SHA-256 hex (64 chars) — current verified hash |
| `status` | enum | `verified` \| `mismatch` \| `unknown` \| `reverted` |
| `updated_at` | string | ISO8601 — last status/hash update |
| `history` | array | Last **3** hash values: `[{ "hash", "updated_at", "source" }]` (newest first) |
| `source_ref` | string | Where this hash is enforced (e.g. `docs/specs/manifest.sha256`, or spec id `devspace_connect.udts.json`) |

### History entry

| Field | Type | Description |
|-------|------|-------------|
| `hash` | string | SHA-256 hex |
| `updated_at` | string | ISO8601 |
| `source` | string | `"manifest"` \| `"spec"` \| `"revert"` \| `"manual"` |

---

## Functionality

### Core

1. **Current hash-verified files with status and updated date**  
   Registry of all tracked paths; for each: `current_hash`, `status`, `updated_at`. Status is computed by comparing on-disk file hash to the stored expected hash (from manifest or spec).

2. **Last 3 hash values with ability to revert to previous hash**  
   For each verified file, retain up to 3 historical `(hash, updated_at, source)` entries. **Revert:** set current expected hash (in manifest or in the referencing spec) back to a chosen previous hash and append a history entry with `source: "revert"`.

3. **Verify-now**  
   Recompute file hash on disk, compare to expected hash, update `status` and `updated_at`; optionally push current hash into history if it changed.

### Production requirements

- **Persistence:** Registry stored in a durable store (e.g. `docs/specs/unts-registry.json` or DB table). Survives restarts.
- **Audit:** All revert and manual updates logged (who/what/when) for observability.
- **Idempotent updates:** Updating a spec’s `proto_sha256` or manifest entry updates the UNTS record and history without duplicating logic.

---

## API / gRPC Surface

Service name (suggested): `mdemg.unts.v1.HashVerification` or `mdemg.unts.v1.NashVerification`.

### Endpoints (RPCs)

| RPC | Request | Response | Description |
|-----|---------|----------|-------------|
| **ListVerifiedFiles** | `ListVerifiedFilesRequest` (optional: `framework` filter) | `ListVerifiedFilesResponse` (repeated `VerifiedFileRecord`) | List all tracked files with current hash, status, updated_at. |
| **GetFileStatus** | `GetFileStatusRequest` (path, optional framework) | `GetFileStatusResponse` (record + computed status) | Single file: current hash, status, updated_at. |
| **GetHashHistory** | `GetHashHistoryRequest` (path) | `GetHashHistoryResponse` (current + last 3 history entries) | Full history for revert UI/API. |
| **RevertToPreviousHash** | `RevertToPreviousHashRequest` (path, target_hash or history_index) | `RevertToPreviousHashResponse` (new current_hash, updated_at) | Set expected hash to a previous value; update manifest or spec and registry. |
| **UpdateHash** | `UpdateHashRequest` (path, new_hash, source) | `UpdateHashResponse` (ok, updated_at) | Manual/CI update of expected hash; append to history. |
| **VerifyNow** | `VerifyNowRequest` (path or empty for all) | `VerifyNowResponse` (results: path -> status, mismatch_detail) | Recompute hashes, compare, update status. |
| **RegisterTrackedFile** | `RegisterTrackedFileRequest` (path, framework, initial_hash, source_ref) | `RegisterTrackedFileResponse` (ok) | Add a new path to the registry (e.g. new UDTS spec). |

### Monitoring and observability

- **Health:** `HashVerificationService` can expose a standard health check (e.g. registry loadable, last verify timestamp).
- **Metrics (UOTS-aligned):** Count of verified vs mismatch vs unknown; last successful verify timestamp; revert count (for dashboards).
- **Structured logging:** Log revert and UpdateHash with path, old/new hash, actor (e.g. "CI", "admin").

---

## Messages (sketch)

```protobuf
message VerifiedFileRecord {
  string path = 1;
  string framework = 2;
  string current_hash = 3;
  string status = 4;       // verified | mismatch | unknown | reverted
  string updated_at = 5;   // ISO8601
  repeated HashHistoryEntry history = 6;
  string source_ref = 7;
}

message HashHistoryEntry {
  string hash = 1;
  string updated_at = 2;
  string source = 3;       // manifest | spec | revert | manual
}

message ListVerifiedFilesRequest {
  string framework = 1;   // optional filter
}

message ListVerifiedFilesResponse {
  repeated VerifiedFileRecord files = 1;
}

message GetFileStatusRequest {
  string path = 1;
  string framework = 2;
}

message GetFileStatusResponse {
  VerifiedFileRecord record = 1;
}

message GetHashHistoryRequest {
  string path = 1;
}

message GetHashHistoryResponse {
  string current_hash = 1;
  string updated_at = 2;
  repeated HashHistoryEntry history = 3;
}

message RevertToPreviousHashRequest {
  string path = 1;
  oneof target {
    string target_hash = 2;
    int32 history_index = 3;   // 0 = most recent previous
  }
}

message RevertToPreviousHashResponse {
  string new_current_hash = 1;
  string updated_at = 2;
  bool ok = 3;
  string error = 4;
}

message UpdateHashRequest {
  string path = 1;
  string new_hash = 2;
  string source = 3;       // manual | ci | spec
  string source_ref = 4;
}

message VerifyNowRequest {
  string path = 1;         // empty = all
  string framework = 2;
}

message VerifyNowResponse {
  repeated FileVerifyResult results = 1;
}

message FileVerifyResult {
  string path = 1;
  string status = 2;
  string expected_hash = 3;
  string actual_hash = 4;
  string updated_at = 5;
}

message RegisterTrackedFileRequest {
  string path = 1;
  string framework = 2;
  string initial_hash = 3;
  string source_ref = 4;
}
```

---

## Registry storage

- **Option A (file-based):** `docs/specs/unts-registry.json` (or `docs/api/api-spec/unts/registry.json`) — JSON array of records; versioned in git. Good for transparency and CI.
- **Option B (DB):** Table(s) in existing store (e.g. Neo4j or PostgreSQL) keyed by path + framework. Good for high write rate and querying.
- **MVP:** Option A; sync from existing sources (manifest.sha256, UDTS spec `proto_sha256` fields) at startup or via VerifyNow, and write back on Revert/UpdateHash to both registry and manifest/spec files.

---

## Integration with existing frameworks

- **manifest.sha256:** UNTS treats each line as a verified file (path + current hash). ListVerifiedFiles includes them with `framework: "manifest"`. Revert updates the line in `manifest.sha256` and the registry.
- **UDTS:** Scan `docs/api/api-spec/udts/specs/*.udts.json` for `config.proto_sha256`; each spec references a proto path (convention: one proto per service). UNTS records proto path + hash with `framework: "udts"`, `source_ref: spec filename`. Revert updates the spec JSON and registry.
- **UATS / UBTS / USTS / UOTS / UAMS / UPTS:** When those frameworks define hash fields, add scanners and `source_ref` conventions so UNTS can list, verify, and revert them.

---

## Implementation order

1. **Spec and schema** — This doc; optional JSON schema for `unts-registry.json`.
2. **Registry format and loader** — Define `unts-registry.json` schema; loader in `internal/unts/` (or `internal/hashverify/`).
3. **Scanners** — Ingest from `manifest.sha256` and UDTS specs into registry (or compute on the fly for read-only).
4. **Core logic** — VerifyNow (recompute, compare), UpdateHash, RevertToPreviousHash (write back to manifest/spec + update registry and history).
5. **gRPC service** — `api/proto/unts.proto` (or `hash-verification.proto`); implement in `internal/unts/server.go`; register in a server binary (e.g. `space-transfer` with `-enable-unts` or dedicated `cmd/unts-server`).
6. **UDTS for UNTS** — Add UDTS specs for ListVerifiedFiles, GetFileStatus, GetHashHistory, VerifyNow; runner in `tests/udts/`.
7. **Observability** — Health, metrics (if UOTS is present), structured logging.

---

## Acceptance

- [ ] Registry (file or DB) holds current hash, status, updated_at, and last 3 history entries per tracked file.
- [ ] ListVerifiedFiles and GetFileStatus return correct data; GetHashHistory returns current + last 3.
- [ ] RevertToPreviousHash updates manifest or spec and registry; history reflects revert.
- [ ] VerifyNow recomputes hashes and updates status.
- [ ] gRPC (or REST) API available for monitoring and manipulation; UDTS coverage for UNTS RPCs.
- [ ] Documented for production: deployment, backup of registry, audit log.
