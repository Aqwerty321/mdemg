# Operations Runbook (Neo4j + MDEMG Service)

This runbook sets the **procedural policies** required to operate the system without turning your memory graph into an expensive chaos machine.

Design mantra (operationalized):
- vector index = recall
- graph = reasoning
- runtime = activation physics
- DB writes = only learning deltas

---

## 1) Service-level SLOs (start conservative)
### Retrieval
- **p95 latency**: < 250 ms (warm cache)
- **candidate_k**: default 200 (cap <= 1000)
- **hop_depth**: default 2 (cap <= 3)
- **max_total_edges_fetched**: default 20k (cap <= 50k)

### Ingestion
- **p95 ingestion latency** (excluding embedding compute): < 150 ms
- **embedding throughput**: budget separately; do not let embedding stall ingestion.

### Learning writeback
- **max edges updated per request**: hard cap (default 200)
- **writeback failures**: < 0.1% of requests

---

## 2) Deployment topology
- Neo4j: dedicated host(s), persistent storage, page cache sized for graph hot set.
- Service: stateless Go instances behind a load balancer.
- Optional offline jobs: decay/consolidation run as cron jobs or a separate worker deployment.

### Configuration must be explicit
All tuning knobs must be config-driven (env/config file), not compiled constants.

---

## 3) Backups and restore
Neo4j supports different backup methods depending on edition and deployment. The operational requirement is simple:
- **You must be able to restore to a consistent point-in-time state**.

### 3.1 Policy
- **RPO** (data loss): 24h maximum (start here; tighten later)
- **RTO** (restore time): 2h maximum (start here; tighten later)
- Keep at least **7 daily** backups, **4 weekly**, **6 monthly**.

### 3.2 Community edition (file-based)
Use `neo4j-admin database dump/load`.

Backup:
```bash
neo4j-admin database dump neo4j --to-path=/backups/neo4j/$(date +%F)
```

Restore:
```bash
neo4j-admin database load neo4j --from-path=/backups/neo4j/2026-01-15 --overwrite-destination=true
```

Operational notes:
- Stop Neo4j for restores (or restore to a new DB name, then swap).
- Validate by running `/readyz` (schema version check) and a smoke retrieval.

### 3.3 Enterprise / cluster
Use Neo4j backup tooling appropriate for the cluster deployment. The policy remains: automate + verify restores regularly.

### 3.4 Backup verification (non-negotiable)
Weekly:
1. Restore backup to a staging environment.
2. Run migrations (should be no-ops if schema already correct).
3. Run a smoke suite:
   - vector index exists
   - retrieval returns results
   - learning writeback succeeds

---

## 4) Schema migrations in production
### 4.1 Rules
- Migrations are **append-only** (no editing old versions).
- Each migration is **idempotent**.
- Service startup checks schema version and refuses to serve if it is below `REQUIRED_SCHEMA_VERSION`.

### 4.2 Rollout procedure
1. Apply migrations to staging.
2. Run regression suite (golden retrieval scoring test vectors).
3. Apply migrations to production.
4. Roll service instances (or let them restart) so they pass schema checks.

---

## 5) Rollover, retention, pruning
### 5.1 Observations retention
Observations are append-only logs; they can grow without bound.

Policy:
- Keep full text observations for **N days** (e.g., 90).
- After N days, either:
  - archive externally (object storage), and keep only a pointer + summary in Neo4j, or
  - keep the observation but drop large payload fields.

### 5.2 Edge pruning
Run daily:
- decay weights
- prune edges below threshold if weak AND low evidence AND old

This is mandatory to prevent hub/clique blowups.

### 5.3 Consolidation rollover
Run weekly (or daily once stable):
- detect stable clusters
- create abstraction nodes (layer k+1)
- thin redundant lateral edges among members

---

## 6) Failure modes and alarms
Your goal is to detect “emergence going pathological” early.

### 6.1 DB health
Alarm on:
- Neo4j down / not accepting connections
- page cache thrash (if visible)
- query latency spikes for vector queries

### 6.2 Retrieval health
Alarm on sustained:
- p95 latency breach
- candidate_k/hop_depth above safe bounds (config drift)
- `max_total_edges_fetched` frequently hit (means expansions too big)

### 6.3 Graph pathology alarms
Alarm on:
- **hub explosion**: max degree or degree p99 climbs rapidly
- **clique spam**: CO_ACTIVATED_WITH edge count grows superlinearly
- **over-decay**: mean activation and recall@K collapse over time

### 6.4 Learning writeback alarms
Alarm on:
- writeback error rate
- edges updated per request at cap for sustained period (means learning is trying to explode)

---

## 7) Incident playbooks
### 7.1 Hub explosion
Symptoms:
- one node dominates results regardless of query
Actions:
1. Increase hub penalty `φ`.
2. Tighten expansion caps (neighbors per node, edges fetched).
3. Prune weak edges around the hub (keep pinned/structural).
4. Consider splitting the hub node into more specific nodes (tombstone old, create new, `MERGED_INTO`).

### 7.2 Clique spam (CO_ACTIVATED_WITH density blowup)
Actions:
1. Raise activation threshold for learning.
2. Reduce `eta` and increase `mu` regularization.
3. Enforce per-node cap: only keep top-N coactivation neighbors by weight.
4. Prune low-evidence coactivation edges.

### 7.3 “Forgetting everything”
Actions:
1. Reduce decay rates.
2. Pin critical nodes/edges.
3. Increase baseline importance (recency and confidence weights) temporarily.

---

## 8) Observability checklist
Track these time series:
- node/edge counts by label/type
- degree distribution (p50/p90/p99/max)
- edge weight distribution by relationship type
- vector query latency + recall sizes
- expansion edges fetched per request
- learning edges updated per request
- consolidation outputs (abstractions created per period)

---

## 9) Security and policy enforcement
- Store `sensitivity` on nodes/observations.
- Enforce sensitivity filtering **server-side** during retrieval and before returning context.
- Log policy decisions (deny/allow counts) but avoid logging raw sensitive content.

---

## 10) Maintenance cadence (suggested)
- Hourly: lightweight health checks + anomaly detection
- Daily: decay + pruning
- Weekly: consolidation + abstraction generation
- Monthly: restore drill + capacity review
