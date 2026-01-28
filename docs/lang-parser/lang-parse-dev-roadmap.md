# Language Parser Development Roadmap

This roadmap expands the language parser phases into concrete, reviewable tasks.
It is designed to improve parser precision without destabilizing existing ingestion.

## Phase 0: Baseline Spec and Invariants

Goal: lock down current behavior so improvements are explicit and safe.

Tasks:
- Phase0_Task0: Catalog current parser outputs for each language (elements + symbols).
- Phase0_Task1: Define invariants for each parser:
  - Phase0_Task1.1: Element kinds and typical counts
  - Phase0_Task1.2: Symbol extraction rules (what is in/out)
  - Phase0_Task1.3: Truncation limits and summary behavior
  - Phase0_Task1.4: Test file detection rules
- Phase0_Task2: Define change boundaries:
  - Phase0_Task2.1: What must remain stable
  - Phase0_Task2.2: What can be added without breaking compatibility
- Phase0_Task3: Document config and fallback behavior:
  - Phase0_Task3.1: JSON config-only ingestion
  - Phase0_Task3.2: YAML and env fallback parsing in ingest-codebase
- Phase0_Task4: Parser output contract (v1)
  - Phase0_Task4.1: Define element vs symbol semantics
  - Phase0_Task4.2: Minimum fields and stability guarantees
    - element_kind: file | symbol | section | keypath_fact | unit | snippet | migration | kernel | other
    - symbol_kind: function | class | method | struct | enum | macro | kernel | other
    - stable_id formula (v1):
      stable_id = hash(space_id + path + element_kind + symbol_qualname + start_line + end_line)
  - Phase0_Task4.3: Evidence requirements
    - required: path, start_line, end_line, signature, stable_id
    - recommended: file_sha256, span_hash, repo_commit

Tests:
- Phase0_Test0: Baseline snapshot tests for each existing parser (element count, symbol count).
- Phase0_Test1: Invariant checklist test (stable kinds, truncation length, test-file logic).
- Phase0_Test2: Fallback parsing tests for YAML and env files.
- Phase0_Test3: Parsing type coverage tests per existing parser.
- Phase0_Test4: Evidence shape sanity (line ranges, stable_id determinism, signature non-empty).

Deliverable:
- Phase0_Deliverable0: Parser baseline spec with accepted invariants and allowed changes.
  - Done means:
    - All Phase 0 tests pass in CI
    - Contract fields validated across fixtures (3 runs, deterministic)
    - Evidence shape sanity passes (line bounds, stable_id deterministic)
    - Baseline outputs captured and reviewed

---

## Phase 1: Parsing Type Matrix - Existing Parsers

Goal: define explicit parsing targets for current parsers as reviewable tasks.

Tasks:
- Phase1_Task0: Go parsing targets
  - Phase1_Task0.1: packages
  - Phase1_Task0.2: exported funcs
  - Phase1_Task0.3: exported types
  - Phase1_Task0.4: consts
  - Phase1_Task0.5: config files
- Phase1_Task1: Rust parsing targets
  - Phase1_Task1.1: modules
  - Phase1_Task1.2: pub items
  - Phase1_Task1.3: traits
  - Phase1_Task1.4: structs/enums
  - Phase1_Task1.5: macros
  - Phase1_Task1.6: consts
- Phase1_Task2: Java parsing targets
  - Phase1_Task2.1: packages
  - Phase1_Task2.2: classes/interfaces/enums
  - Phase1_Task2.3: methods
  - Phase1_Task2.4: static constants
- Phase1_Task3: JS/TS parsing targets
  - Phase1_Task3.1: modules
  - Phase1_Task3.2: classes/interfaces
  - Phase1_Task3.3: exported functions
  - Phase1_Task3.4: config files
- Phase1_Task4: Python parsing targets
  - Phase1_Task4.1: modules
  - Phase1_Task4.2: classes
  - Phase1_Task4.3: functions
  - Phase1_Task4.4: module-level constants
- Phase1_Task5: C parsing targets
  - Phase1_Task5.1: headers/sources
  - Phase1_Task5.2: structs/enums
  - Phase1_Task5.3: macros
  - Phase1_Task5.4: consts
  - Phase1_Task5.5: functions
- Phase1_Task6: C++ parsing targets
  - Phase1_Task6.1: headers/sources
  - Phase1_Task6.2: classes/structs
  - Phase1_Task6.3: namespaces
  - Phase1_Task6.4: templates
  - Phase1_Task6.5: macros
- Phase1_Task7: JSON parsing targets
  - Phase1_Task7.1: structured config
  - Phase1_Task7.2: top-level keys
  - Phase1_Task7.3: config values
- Phase1_Task8: Markdown parsing targets
  - Phase1_Task8.1: documentation files
  - Phase1_Task8.2: config docs (if applicable)
- Phase1_Task9: XML parsing targets
  - Phase1_Task9.1: build config
  - Phase1_Task9.2: schema/config metadata
  - Phase1_Task9.3: namespaces
- Phase1_Task10: SQL parsing targets
  - Phase1_Task10.1: schema objects
  - Phase1_Task10.2: DDL
  - Phase1_Task10.3: migrations
  - Phase1_Task10.4: dialect indicators

Deliverable:
- Phase1_Deliverable0: Explicit parsing targets checklist for existing parsers.
  - Done means:
    - Checklist reviewed and approved
    - Parsing targets map to current parser outputs
    - No ambiguity in element_kind/symbol_kind mapping

---

## Phase 2: Parsing Type Matrix - Planned Parsers

Goal: define explicit parsing targets for new parsers as reviewable tasks.

Tasks:
- Phase2_Task0: YAML parsing targets
  - Phase2_Task0.1: config
  - Phase2_Task0.2: CI workflows
  - Phase2_Task0.3: K8s manifests
  - Phase2_Task0.4: compose
  - Phase2_Task0.5: Helm values
- Phase2_Task1: TOML parsing targets
  - Phase2_Task1.1: project config
  - Phase2_Task1.2: tool config
  - Phase2_Task1.3: dependency metadata
- Phase2_Task2: CUDA parsing targets
  - Phase2_Task2.1: kernels
  - Phase2_Task2.2: device functions
  - Phase2_Task2.3: host wrappers
  - Phase2_Task2.4: launch sites
- Phase2_Task3: Shell parsing targets
  - Phase2_Task3.1: env exports
  - Phase2_Task3.2: command pipelines
  - Phase2_Task3.3: conditionals
  - Phase2_Task3.4: functions
- Phase2_Task4: Dockerfile parsing targets
  - Phase2_Task4.1: stages
  - Phase2_Task4.2: build args
  - Phase2_Task4.3: runtime config
  - Phase2_Task4.4: entrypoints
- Phase2_Task5: INI / dotenv / properties parsing targets
  - Phase2_Task5.1: key/value defaults
  - Phase2_Task5.2: env overrides
  - Phase2_Task5.3: profiles
- Phase2_Task6: Cypher parsing targets
  - Phase2_Task6.1: schema ops
  - Phase2_Task6.2: query patterns
  - Phase2_Task6.3: constraints/indexes

Deliverable:
- Phase2_Deliverable0: Explicit parsing targets checklist for planned parsers.
  - Done means:
    - Checklist reviewed and approved
    - Parsing targets map to planned parser tasks
    - Coverage includes CUDA and build-context inputs

---

## Phase 3: Additive Parsers (No changes to existing parsers)

Goal: add missing languages as new modules without altering current outputs.

Tasks:
- Phase3_Task0: YAML parser
  - Parsing types: config, CI workflows, K8s manifests, compose, Helm values
  - Phase3_Task0.1: Define element kinds (unit, keypath_fact, doc_header)
  - Phase3_Task0.2: Flatten key paths
  - Phase3_Task0.3: Handle anchors/aliases
  - Phase3_Task0.4: Identify jobs/steps/kinds/services for CI/K8s/compose
  - Phase3_Task0.5: Cap keypaths per file (prefer scalars/defaults)
  - Phase3_Task0.6: Deterministic keypath capping order
    - scalars over sequences/maps
    - keys matching default|defaults|env|image|version|port|timeout|limit|memory|cpu
    - shortest path depth first
    - stable lexical order tie-break
- Phase3_Task1: TOML parser
  - Parsing types: project config, tool config, dependency metadata
  - Phase3_Task1.1: Define element kinds (unit, keypath_fact, doc_header)
  - Phase3_Task1.2: Extract tables and arrays of tables
  - Phase3_Task1.3: Recognize pyproject/Cargo/ruff configs
  - Phase3_Task1.4: Cap keypaths per file (prefer scalars/defaults)
  - Phase3_Task1.5: Deterministic keypath capping order
    - scalars over sequences/maps
    - keys matching default|defaults|env|image|version|port|timeout|limit|memory|cpu
    - shortest path depth first
    - stable lexical order tie-break
- Phase3_Task2: CUDA parser
  - Parsing types: kernels, device functions, host wrappers, launch sites
  - Phase3_Task2.1: Extract kernel definitions (__global__) and launches (<<<>>>)
  - Phase3_Task2.2: Capture device/shared memory constants and guards
  - Phase3_Task2.3: Parse .cu/.cuh and detect nvcc flags from build files
  - Phase3_Task2.4: Build Context Registry (v1) for compile flags and include paths
- Phase3_Task3: Shell parser (bash/sh/zsh)
  - Parsing types: env exports, command pipelines, conditionals, functions
  - Phase3_Task3.1: Extract env exports, commands, and conditionals
  - Phase3_Task3.2: Capture command sequences as structured summaries
- Phase3_Task4: Dockerfile parser
  - Parsing types: stages, build args, runtime config, entrypoints
  - Phase3_Task4.1: Extract FROM, ARG, ENV, RUN, EXPOSE, ENTRYPOINT/CMD
  - Phase3_Task4.2: Detect multi-stage build structure
- Phase3_Task5: INI / dotenv / properties parser
  - Parsing types: key/value defaults, env overrides, profiles
  - Phase3_Task5.1: Define element kinds (unit, keypath_fact, doc_header)
  - Phase3_Task5.2: Flatten key/value pairs
  - Phase3_Task5.3: Preserve file:line for evidence
  - Phase3_Task5.4: Cap keypaths per file (prefer scalars/defaults)
  - Phase3_Task5.5: Deterministic keypath capping order
    - scalars over sequences/maps
    - keys matching default|defaults|env|image|version|port|timeout|limit|memory|cpu
    - shortest path depth first
    - stable lexical order tie-break
- Phase3_Task6: Cypher parser
  - Parsing types: schema ops, query patterns, constraints/indexes
  - Phase3_Task6.1: Extract labels, rel types, constraints/index creation
  - Phase3_Task6.2: Capture parameter names and query shapes

Tests:
- Phase3_Test0: Parser unit tests with fixtures per new language.
- Phase3_Test1: Golden element/symbol counts for new parsers.
- Phase3_Test2: File extension routing tests for new parsers.
- Phase3_Test3: Variant fixture packs (minimal, variant, nasty) with stable IDs.

Deliverable:
- Phase3_Deliverable0: New parser files + baseline tests for each new language.
  - Done means:
    - All Phase 3 tests pass in CI
    - Variant fixtures deterministic across 3 runs
    - Element counts within caps and stable
    - New parsers do not change existing outputs

---

## Phase 4: Precision Upgrades (Gated or additive-only)

Goal: improve existing parsers without breaking current outputs.

Tasks by language:
- Phase4_Task0: Go
  - Parsing types: packages, exported APIs, config defaults, build tags
  - Phase4_Task0.1: Import graph + build tags
  - Phase4_Task0.2: Struct fields with file:line evidence
  - Phase4_Task0.3: CLI defaults (flag, cobra, viper)
- Phase4_Task1: Rust
  - Parsing types: modules, pub API, traits/impls, build metadata
  - Phase4_Task1.1: Module + use graph
  - Phase4_Task1.2: Feature flags (cfg)
  - Phase4_Task1.3: build.rs metadata
- Phase4_Task2: Java
  - Parsing types: packages, annotations, DI wiring, entrypoints
  - Phase4_Task2.1: Annotations + DI wiring
  - Phase4_Task2.2: Entrypoints (main, Spring boot)
  - Phase4_Task2.3: Config defaults from application.*
- Phase4_Task3: JS/TS
  - Parsing types: modules, exports, framework hooks, config defaults
  - Phase4_Task3.1: Export graph and public API surfaces
  - Phase4_Task3.2: Resolve tsconfig path aliases
  - Phase4_Task3.3: Framework-specific tags (Next/React/Vite)
- Phase4_Task4: Python
  - Parsing types: modules, dataclasses, type hints, config sources
  - Phase4_Task4.1: Dataclasses, type hints
  - Phase4_Task4.2: Config sources (env, pydantic, argparse, click/typer)
- Phase4_Task5: SQL
  - Parsing types: schema objects, constraints, defaults, migrations
  - Phase4_Task5.1: Columns, constraints, defaults
  - Phase4_Task5.2: Migration order/version detection
- Phase4_Task6: Markdown
  - Parsing types: sections, headings, code fences, doc links
  - Phase4_Task6.1: Section nodes by heading
  - Phase4_Task6.2: Code fence extraction with language tags
- Phase4_Task7: JSON / XML
  - Parsing types: structured config, schemas, metadata trees
  - Phase4_Task7.1: Add JSONC/JSON5 support
  - Phase4_Task7.2: XML namespace preservation + path flattening

Tests:
- Phase4_Test0: Gated behavior tests (on/off flag parity).
- Phase4_Test1: Regression tests vs Phase 0 baseline outputs.
- Phase4_Test2: Precision tests for new extraction fields.

Deliverable:
- Phase4_Deliverable0: Gated upgrades with explicit toggle or additive-only behavior.
  - Done means:
    - All Phase 4 tests pass in CI
    - Gated outputs match baseline when disabled
    - Precision fields deterministic across 3 runs
    - No baseline drift without explicit approval

---

## Phase 5: Evidence Accuracy and Stability

Goal: make evidence trustworthy and consistent for retrieval.

Tasks:
- Phase5_Task0: Standardize symbol signatures across parsers.
- Phase5_Task1: Ensure file:line mapping is consistent and stable.
- Phase5_Task2: Refine stable_id normalization rules (v1 alignment).
  - Phase5_Task2.1: Normalize stable_id components (case, path separators, whitespace)
- Phase5_Task3: Add evidence validation checks for common parser outputs.

Tests:
- Phase5_Test0: Evidence mapping tests (file/line validation).
- Phase5_Test1: Stable ID determinism tests.
- Phase5_Test2: Cross-parser signature normalization tests.

Deliverable:
- Phase5_Deliverable0: Evidence consistency spec + validation checks.
  - Done means:
    - Evidence validation tests pass in CI
    - Stable_id normalization documented and verified
    - Evidence fields present where required

---

## Phase 6: Parser Test Suite and Regression Harness

Goal: prevent regressions and quantify output stability.

Tasks:
- Phase6_Task0: Build fixtures for each language and variant.
- Phase6_Task1: Add golden tests for element/symbol counts.
- Phase6_Task2: Create parser performance benchmarks by file size.
- Phase6_Task3: Add CI-friendly test runs (limited fixtures, deterministic output).

Tests:
- Phase6_Test0: Fixture integrity tests (no drift).
- Phase6_Test1: Performance budget tests (time/size).
- Phase6_Test2: CI smoke tests for all parsers.

Deliverable:
- Phase6_Deliverable0: Regression harness + parser fixture suite.
  - Done means:
    - Fixture integrity tests pass
    - Golden diffs reviewed and approved
    - Performance budget met for fixture tiers

---

## Phase 7: Performance and Scaling Controls

Goal: control ingestion size and runtime for large repos.

Tasks:
- Phase7_Task0: Per-file element caps.
- Phase7_Task1: Per-file symbol caps.
- Phase7_Task2: Size-based early exit for oversized files.
- Phase7_Task3: Optional sampling mode for huge directories.
- Phase7_Task4: Embedding policy controls (what to embed vs skip).
- Phase7_Task5: Exclusion policy presets
  - default
  - ml_cuda
  - web_monorepo

Tests:
- Phase7_Test0: Cap enforcement tests (element/symbol limits).
- Phase7_Test1: Oversized file handling tests.
- Phase7_Test2: Sampling mode determinism tests.
- Phase7_Test3: Embedding policy determinism tests.
- Phase7_Test4: Exclusion preset determinism tests.

Deliverable:
- Phase7_Deliverable0: Guardrails for large-codebase ingestion.
  - Done means:
    - Guardrail tests pass in CI
    - Exclusion presets deterministic and documented
    - Embedding policy enforced with no regressions

---

## Review Gate

Each phase should be reviewed and accepted before implementation begins.
No phase should alter existing ingestion outputs without explicit approval.
