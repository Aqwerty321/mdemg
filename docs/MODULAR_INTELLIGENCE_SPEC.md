# MDEMG Modular Intelligence Specification

## 1. Overview
This document defines the **MDEMG Module System**, a plug-and-play architecture that transforms MDEMG from a passive memory store into a modular, skill-based cognitive substrate. Modules allow MDEMG to acquire domain-specific parsing skills, architectural reasoning patterns, and proactive reflection capabilities.

---

## 2. Module Definition (The `mdemg-module.json`)
Every module is defined by a manifest that specifies its hooks into the MDEMG lifecycle.

```json
{
  "id": "nestjs-architecture-module",
  "name": "NestJS Architecture SME",
  "version": "1.0.0",
  "type": "REASONING",
  "capabilities": {
    "ingestion_parsers": [".module.ts", ".controller.ts", ".service.ts"],
    "pattern_detectors": ["DependencyInjection", "GuardAuth", "InterceptorLog"],
    "reflection_strategies": ["ModuleDependencyGraph"]
  },
  "subscriptions": ["codebase-ingest", "session-end"],
  "config_schema": {
    "strict_dependency_check": "boolean",
    "generate_diagrams": "boolean"
  }
}
```

---

## 3. Module Types & Go Interfaces

To ensure technical depth and type safety, modules must implement specific Go interfaces.

### A. Ingestion Modules (Perception)
*   **Deliverable**: `internal/modules/ingest/`
*   **Interface**:
```go
type BaseParser interface {
    // ID returns the unique identifier for this parser (e.g., "golang-ast")
    ID() string
    // Matches returns true if this parser can handle the given file path
    Matches(filePath string) bool
    // Parse extracts code elements from the file content
    Parse(ctx context.Context, filePath string, content []byte) ([]models.CodeElement, error)
}
```
*   **Discovery**: Modules are registered at compile-time via an `init()` function into a central `ParserRegistry`.

### B. Reasoning Modules (Cognition)
*   **Deliverable**: `internal/modules/reasoning/`
*   **Interface**:
```go
type ReasoningModule interface {
    // ID returns the unique identifier (e.g., "llm-reranker")
    ID() string
    // Process executes the reasoning logic on a set of nodes or search results
    Process(ctx context.Context, spaceID string, input any) (any, error)
}
```
*   **Note**: The **LLM Re-ranking (v9)** should be refactored as a `ReasoningModule`. This allows the retrieval pipeline to stay lean while delegating sophisticated ranking to pluggable logic.

### C. Active Participant Modules (APE)
*   **Deliverable**: `internal/modules/ape/`
*   **Interface**:
```go
type APEModule interface {
    // Trigger specifies when this module should run (e.g., "on-ingest", "on-session-end", "hourly")
    Trigger() string
    // Execute runs the background task
    Execute(ctx context.Context, driver neo4j.DriverWithContext) error
}
```

---

## 4. Active Participant Engine (APE) Detail

The **APE** orchestrates the lifecycle of the memory graph.

### Key Components:
1.  **Consistency Guard**: Background worker implementing `APEModule`. Prunes weak edges and ensures hierarchical integrity.
2.  **LLM Reconciler**: Distinct from re-ranking. While re-ranking sorts results for a query, the **Reconciler** runs as an `APEModule` to resolve contradictory observations (e.g., if two agents report different versions of the same architectural decision).
3.  **Context Cooler (Short-Term Buffer)**: 
    *   **Mechanism**: Observations are initially written with a `volatile: true` flag and a high `co_activation` weight.
    *   **Graduation**: After a "cooling period" (e.g., 2 hours of inactivity), the APE runs a "graduation" task that removes the volatile flag, normalizes weights, and triggers consolidation.

---

## 5. Explainable Retrieval (Explain-RAG)

Updates to the `/v1/memory/retrieve` endpoint to provide transparency.

### Deliverable: `RetrieveResponse` Extension
```json
{
  "data": [
    {
      "node_id": "auth_service_01",
      "rationale": "Direct semantic match for 'authentication' + strongly linked to 'JWT' via CO_ACTIVATED_WITH (weight 0.85)",
      "confidence_score": 0.92,
      "source_module": "nestjs-architecture-module"
    }
  ]
}
```

---

## 6. Implementation Roadmap

### Phase 6.1: Ingestion Plugin Architecture
*   Refactor `ingest-codebase` to use an interface-based parser registry.
*   Implement the `BaseParser` interface.

### Phase 6.2: Module Manifest & Registry
*   Create the `/v1/modules` API for registering and configuring modules.
*   Store module state in Neo4j under a `:ModuleRegistry` label.

### Phase 6.3: Active Participant Orchestrator
*   Implement the APE as a long-running Go routine within the server.
*   Add event-based triggers (e.g., trigger `reflection` module on `session-end`).
