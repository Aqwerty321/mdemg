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

To ensure technical depth and type safety while avoiding the fragility of native Go `plugin` binaries, MDEMG utilizes a **gRPC-based Sidecar Architecture**. This allows modules to be written in any language and avoids "dependency hell."

### A. Ingestion Modules (Perception)
*   **Deliverable**: `internal/modules/ingest/`
*   **GRPC Service Definition**:
```proto
service IngestionModule {
    rpc Matches(MatchRequest) returns (MatchResponse);
    rpc Parse(ParseRequest) returns (ParseResponse);
}
```
*   **General Examples (Non-Code)**:
    *   **P&ID Module**: Parses industrial process diagrams (XML/SVG).
    *   **Linear Module**: High-velocity task history and engineering decision chains.
    *   **Obsidian Module**: Unstructured second-brain notes, research fragments, and personal knowledge graphs.
    *   **Conversation Module**: Extracts specific commitments from Slack/Discord logs.

### B. Reasoning Modules (Cognition)
*   **Deliverable**: `internal/modules/reasoning/`
*   **GRPC Service Definition**:
```proto
service ReasoningModule {
    rpc Process(ReasonRequest) returns (ReasonResponse);
}
```
*   **General Examples (Non-Code)**:
    *   **Strategic Consistency Module**: Cross-references new decisions against "Company North Star" principles.
    *   **Logical Conflict Module**: Identifies when a new observation contradicts a 1-year-old "Ground Truth" node.

### C. Active Participant Modules (APE)
*   **Deliverable**: `internal/modules/ape/`
*   **GRPC Service Definition**:
```proto
service APEModule {
    rpc Execute(ExecuteRequest) returns (ExecuteResponse);
}
```

---

## 4. Detailed Technical Implementation Path

### Phase 1: gRPC Plugin Host (The Discovery Layer)
1.  **Define Protobufs**: Create `mdemg-module.proto` defining the service interfaces above.
2.  **Plugin Manager**: Develop a Go service that scans a `/plugins` directory for binary executables or a `docker-compose` for sidecar containers.
3.  **Automatic Handshake**: On startup, MDEMG executes the plugin and performs a handshake to negotiate capabilities (Ingestion vs Reasoning).

### Phase 2: Refactoring Ingest & Retrieval for RPC
1.  **Ingest Hook**: Instead of hardcoded Go parsers, `IngestObservation` calls the `IngestionModule.Parse` RPC for all registered modules.
2.  **Retrieval Hook**: Refactor the retrieval pipeline to allow `ReasoningModule.Process` to run between the **Spreading Activation** and **Final Top-K** steps.

### Phase 3: The "General Intelligence" Baseline
To prove MDEMG isn't just for code, we will implement the **"Internal Dialog Baseline"**:
*   **Reflection Module**: A module that takes the last 10 observations (regardless of source) and generates a "What I learned today" node.
*   **Commitment Module**: A module that extracts "IF/THEN" rules from text observations and creates specific `CONSTRAINT` nodes.

---

## 5. Active Participant Engine (APE) Detail

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
