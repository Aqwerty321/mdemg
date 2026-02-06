# Framework Governance (UPTS-Type Specs)

**Purpose:** Standard frameworks for testing, security, observability, auth, hash verification, and gRPC contracts across the Development Space Collaboration and related services.

---

## Framework Summary

| Acronym | Name | Use when |
|---------|------|----------|
| **UNTS** | Universal Hash Test Specification (Hash Verification / Nash Verification module) | Hash verification for all framework-protected files (manifest, UDTS, UATS, UBTS, USTS, UOTS, UAMS, UPTS) |
| **UDTS** | Universal DevSpace Test Specification | gRPC services (contract/integration tests) |
| **UBTS** | Universal Benchmark Test Specification | Benchmarking (throughput, latency, load) |
| **USTS** | Universal Security Test Specification | Security functionality (hardening, vuln checks) |
| **UOTS** | Universal Observability Test Specification | Observability (metrics, tracing, logging) |
| **UAMS** | Universal Auth Management Specification | Authentication and authorization |

---

## UNTS — Hash Verification (Nash Verification module)

- **Scope:** Current and historical record of hash verification for **all** MDEMG files protected by hash verification across UPTS, UATS, UBTS, USTS, UOTS, UAMS, and UDTS.
- **Functionality:** Current hash-verified files with status and updated date; last 3 hash values per file with ability to **revert to previous hash**; VerifyNow; API/gRPC for monitoring, observability, and manipulation.
- **Location:** Spec [unts-hash-verification.md](./unts-hash-verification.md); registry (e.g. `docs/specs/unts-registry.json`); implementation to be under `internal/unts/` (or `internal/hashverify/`); optional gRPC in `api/proto/unts.proto`.
- **Reference:** [UNTS Hash Verification spec](./unts-hash-verification.md).

---

## UDTS — gRPC Services

- **Scope:** All gRPC APIs (Space Transfer, DevSpace, future services).
- **Location:** Specs in `docs/api/api-spec/udts/specs/`, schema in `docs/api/api-spec/udts/schema/`, runner in `tests/udts/`.
- **Usage:** One or more `.udts.json` specs per RPC; `config.proto_sha256` for proto stability; run with `UDTS_TARGET=host:port`.
- **Reference:** [UDTS README](../api/api-spec/udts/README.md).

---

## UBTS — Benchmarking

- **Scope:** Performance and load tests (e.g. export/import throughput, retrieval latency, Connect message rate).
- **Use when:** Adding or validating benchmarks, regression thresholds, or load profiles.
- **Location:** To be established under `docs/api/api-spec/ubts/` (or equivalent) with specs and runner.
- **Reference:** Create UBTS when first benchmark work is required.

---

## USTS — Security

- **Scope:** Security hardening, vulnerability checks, and security-related behavior (TLS, auth boundaries, injection).
- **Use when:** Implementing or changing security functionality (e.g. TLS for gRPC, API keys, rate limits).
- **Location:** To be established under `docs/api/api-spec/usts/` with specs and runner.
- **Reference:** Create USTS when first security test work is required.

---

## UOTS — Observability

- **Scope:** Metrics, distributed tracing, and structured logging (e.g. Prometheus, OpenTelemetry, log levels).
- **Use when:** Implementing or changing observability (metrics endpoints, trace propagation, log formats).
- **Location:** To be established under `docs/api/api-spec/uots/` with specs and runner.
- **Reference:** Create UOTS when first observability test work is required.

---

## UAMS — Auth

- **Scope:** Authentication and authorization (tokens, API keys, roles, DevSpace membership checks).
- **Use when:** Adding or changing auth (e.g. Phase 2b DevSpace auth, Phase 8 presence/auth).
- **Location:** To be established under `docs/api/api-spec/uams/` with specs and runner.
- **Reference:** Create UAMS when first auth-specific test work is required.

---

## Application in Development Space Collaboration

- **UNTS:** Apply to all hash-protected artifacts (manifest.sha256, UDTS proto_sha256, and future UATS/UBTS/USTS/UOTS/UAMS/UPTS hashes). Implement Hash Verification module for production monitoring and revert capability.
- **Phases 1–2:** UDTS only (gRPC contract tests).
- **Phase 3+:** UDTS for all new gRPC methods; add UBTS/USTS/UOTS/UAMS as each capability is introduced (benchmarks, security, observability, auth).
