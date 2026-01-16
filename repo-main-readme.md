# Multi-Dimensional Emergent Memory Graph (MDEMG) — Neo4j Implementation Docs

System for long term memory with emergent activation layering
- A **multi-layer** node taxonomy (L0..Ln) for concreteness → abstraction
- **Typed, weighted relationships** with evidence and decay
- **Vector embeddings + Neo4j vector indexes** for semantic recall
- **Spreading activation + Hebbian updates** to encourage emergent associations
- **Consolidation/pruning** to prevent “graph cancer”

## What you should build first (MVP)
1. **Schema + constraints** (labels, relationship types, property conventions)
2. **Ingestion**: append-only observations + embedding creation + basic edges
3. **Retrieval**: vector candidate recall + 1–2 hop expansion + scoring
4. **Learning loop**: co-activation edge strengthening + decay job
5. **Consolidation**: create abstraction nodes in higher layers from stable clusters

## File map
- `01_Architecture.md` — services + flow (online retrieval vs offline maintenance)
- `02_Graph_Schema.md` — labels, rel types, properties, constraints, conventions
- `03_Vector_Embeddings_and_Indexes.md` — Neo4j vector index + GenAI plugin patterns
- `04_Activation_and_Learning.md` — spreading activation, Hebbian learning, decay
- `05_Ingestion_Pipeline.md` — upsert patterns, edge updates, observation events
- `06_Retrieval_API_and_Scoring.md` — candidate generation, scoring, explanations
- `07_Consolidation_and_Pruning.md` — abstraction promotion + pruning rules
- `08_Config_and_Tuning.md` — the knobs (rates, thresholds, scoring weights)
- `09_Testing_and_Simulation.md` — synthetic graphs + regression tests
- `10_Operational_Notes.md` — deploy, performance, security, backups

## Design mantra
Emergence comes from **local rules with limits**:
- activation spreads, but decays
- weights strengthen, but regularize
- graph grows, but prunes and compresses

decay metrics are required: decay + consolidation.
