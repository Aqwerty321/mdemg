# Constraint Nodes

Phase 45.5 automatically detects and promotes constraint-tagged observations to first-class constraint nodes in the knowledge graph. This enables structured tracking of requirements, prohibitions, recommendations, and deadlines extracted from natural language.

## How It Works

### Auto-Detection

The `ConstraintDetector` scans observation content for natural language patterns indicating constraints:

| Type | Example Patterns | Confidence |
|------|-----------------|------------|
| `must` | "must", "always", "required", "mandatory" | 0.65-0.80 |
| `must_not` | "never", "must not", "forbidden", "prohibited" | 0.55-0.85 |
| `should` | "should", "prefer", "recommended", "best practice" | 0.50-0.65 |
| `should_not` | "should not", "try to avoid", "discouraged" | 0.55-0.65 |
| `deadline` | "by 2026-02-09", "due date", "deadline", "target date" | 0.70-0.80 |

Detection confidence is boosted by observation type: `decision` (+0.2), `correction` (+0.15). Detections below the minimum confidence threshold (0.6) are discarded.

Detected constraints are added as tags on the observation (e.g., `constraint:must`, `constraint:deadline`).

### Node Promotion

During consolidation, the pipeline's constraint step (phase 20, enrichment) promotes tagged observations to constraint nodes:

1. **Find** observations with `constraint:*` tags not yet linked to a constraint node
2. **Match or Create** a constraint node (`role_type: 'constraint'`, `layer: 1`) with the extracted label and type
3. **Link** the observation to the constraint via an `IMPLEMENTS_CONSTRAINT` edge (initial weight: 1.0)
4. **Reinforce** existing constraints: increment `reinforcement_count` and edge weight (+0.1) on repeated matches

### Label Extraction

Constraint node names are extracted from the first sentence of the observation content (up to the first period/newline, max 120 characters).

## Usage

```bash
# List all constraints in a space
curl -s "http://localhost:9999/v1/constraints?space_id=mdemg-dev" | jq

# Get constraint statistics
curl -s "http://localhost:9999/v1/constraints/stats?space_id=mdemg-dev" | jq
```

### Example Response — List

```json
{
  "space_id": "mdemg-dev",
  "constraints": [
    {
      "node_id": "uuid",
      "name": "Must use CMS for all operations",
      "constraint_type": "must",
      "content": "Must use CMS for all operations...",
      "confidence": 0.9,
      "created_at": "2026-02-07T...",
      "updated_at": "2026-02-07T...",
      "source_count": 3
    }
  ]
}
```

### Example Response — Stats

```json
{
  "space_id": "mdemg-dev",
  "total_constraint_nodes": 15,
  "by_type": [
    { "constraint_type": "must", "count": 8, "avg_confidence": 0.82 },
    { "constraint_type": "must_not", "count": 5, "avg_confidence": 0.78 }
  ],
  "tagged_observation_count": 23
}
```

## Pipeline Integration

The constraint step runs at phase 20 (enrichment), after core hidden node creation (phase 10) but before dynamic edges (phase 25). It is non-required — failure does not block the pipeline.

Results appear in the consolidation response under `steps.constraint`:

```json
{
  "steps": {
    "constraint": {
      "created": 2,
      "updated": 1,
      "linked": 3
    }
  }
}
```

## Related Files

| File | Description |
|------|-------------|
| `internal/conversation/constraint_detector.go` | Regex-based auto-detection with confidence scoring |
| `internal/hidden/constraint_nodes.go` | Node promotion and `IMPLEMENTS_CONSTRAINT` edge creation |
| `internal/hidden/step_constraint.go` | Pipeline step adapter (phase 20) |
| `internal/api/handlers_conversation.go` | `handleConstraintsList`, `handleConstraintStats` handlers |
| `docs/api/api-spec/uats/specs/constraints_list.uats.json` | Contract test for list endpoint |
| `docs/api/api-spec/uats/specs/constraints_stats.uats.json` | Contract test for stats endpoint |

## Dependencies

- Consolidation pipeline (Phase 46-PR) — constraint step registered in `buildPipeline()`
- CMS observe endpoint — constraints detected during `Observe()` calls
