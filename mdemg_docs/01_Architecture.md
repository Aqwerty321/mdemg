# Architecture (Neo4j + Vector Indexes)

> **See also:** [VISION.md](/Users/reh3376/mdemg/VISION.md) for the philosophical foundation and design principles.

## Core Philosophy

MDEMG is a **cognitive substrate** for AI coding agents where higher-level concepts **emerge automatically** from accumulated observations. Unlike static knowledge bases:

- **Edges remain stable** - Once a relationship is learned, it persists
- **Node positions are fluid** - Concepts can move between layers as understanding deepens
- **Paths adapt** - The route to reach a concept changes as organization evolves
- **No manual maintenance** - Reorganization happens automatically through Hebbian learning

## Multi-Dimensional Layered Graph

```
Layer N   [Principles / Axioms]           ← Most abstract
    ↑     Emerges from patterns in Layer N-1
Layer 3   [Concepts / Abstractions]
    ↑     Emerges from patterns in Layer 2
Layer 2   [Patterns / Regularities]
    ↑     Emerges from patterns in Layer 1
Layer 1   [Observations / Events]         ← Most concrete
    ↑
[Raw Input: code, decisions, conversations]
```

**Layer constraints:**
- **Minimum**: 1 (raw observations only)
- **Maximum**: Unconstrained (hardware-limited only)
- **Growth**: Dynamic - layers emerge as data density warrants

## High-level components
1. **Neo4j**: property graph store for nodes/edges + weights + provenance.
2. **Neo4j Vector Index**: fast nearest-neighbor search over embeddings.
3. **Embedding generation**:
   - Option A: Neo4j **GenAI plugin** (`genai.vector.encode`, `encodeBatch`) to generate/store embeddings directly.
   - Option B: external embedding service (Python/Go) writes vectors to Neo4j.
4. **Memory Service (API)** (Go recommended for latency):
   - ingestion endpoint
   - retrieval endpoint
   - maintenance endpoints (decay, consolidation)
5. **Offline Jobs** (Python optional):
   - summarization refresh
   - consolidation clustering
   - health checks / anomaly detection

## Online flow (retrieve)
1. Query comes in (text + policy context).
2. Create query embedding (GenAI plugin or external).
3. Vector search: top N candidates via `db.index.vector.queryNodes`.
4. Graph expansion: 1–D hops from candidates using typed edges.
5. Activation pass: compute transient activation scores.
6. Rank and return:
   - top K nodes
   - explanation graph (why each node was selected)
7. Learning updates:
   - strengthen `CO_ACTIVATED_WITH` edges among selected nodes
   - bump evidence counts / timestamps

## Online flow (ingest)
1. Observation arrives with timestamp/source/content/tags.
2. Resolve target node(s) or create new.
3. Append observation event (immutable log).
4. Update node summary (small rolling summarization).
5. Generate/store embedding for node (or observation chunk).
6. Create/adjust edges:
   - semantic links (nearest neighbors)
   - temporal adjacency (recent event chain)
   - containment/path links
7. Optionally enqueue for consolidation evaluation.

## Offline flow (maintenance)
- **Decay job**: exponentially decay weights + prune weak edges.
- **Consolidation job**:
  - detect stable clusters at layer k
  - create abstraction node at layer k+1
  - add `ABSTRACTS_TO` edges
  - compress redundant lateral edges

## Key decision: where activation “lives”
Activation values are **transient**. Do NOT permanently write per-query activation to nodes unless:
- you store debug snapshots in a dedicated label (recommended for debugging only)
- or you want a short-lived cache with TTL semantics (hard in vanilla Neo4j)

Recommended:
- compute activation in the Memory Service runtime,
- write only *learning deltas* back to the graph.

## Integration Modes

MDEMG operates as a **full active participant** in the development workflow:

### 1. Background Service
- Always running, similar to claude-mem
- API available for agent queries
- Continuous learning from observations

### 2. Event-Driven Hooks
- Git commits trigger memory updates
- File saves capture context
- Session events (start/end) trigger reflection

### 3. Proactive Surfacing

| Mode | Behavior |
|------|----------|
| **Context Suggestions** | When working on code, surface related patterns/decisions |
| **Periodic Reflection** | Synthesize insights at session start/end |
| **Anomaly Detection** | Alert when current work contradicts stored knowledge |
| **Conflict Resolution** | Identify when new info conflicts with existing beliefs |

### 4. Agent Consulting Service

A higher-order capability where MDEMG acts as an **SME (Subject Matter Expert)** for coding agents:

- **Context provision**: "Based on this codebase's patterns..."
- **Process guidance**: "The typical workflow for this type of change is..."
- **Concept synthesis**: "This relates to the higher-level principle of..."
- **Risk awareness**: "Previous attempts at this approach encountered..."

## What MDEMG Stores

| Category | Examples |
|----------|----------|
| **Code Patterns** | Solutions, idioms, reusable structures |
| **Architectural Decisions** | Why things are built a certain way |
| **Process Knowledge** | How to do things, workflows, procedures |
| **Project Context** | Domain-specific terminology, constraints |
| **Error Patterns** | What went wrong and how it was fixed |
| **User Preferences** | Coding style, tool preferences, conventions |
| **Cross-Project Learnings** | Insights that transfer between projects |

## Technical Invariants (DO NOT VIOLATE)

- **Vector index = recall** (fast candidate generation)
- **Graph = reasoning** (typed edges with evidence)
- **Runtime = activation physics** (computed in-memory, NOT persisted)
- **DB writes = learning deltas only** (bounded, no per-request activation writes)
