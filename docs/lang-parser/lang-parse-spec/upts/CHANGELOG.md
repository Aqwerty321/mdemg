# UPTS Changelog

## [1.1.0] - 2026-01-29

### Fixed

#### Fixture Path Resolution
- **Issue:** Spec files referenced fixtures with paths relative to `upts/` root
- **Fix:** Changed all fixture paths to be relative to spec file location
- **Before:** `"path": "fixtures/go_test_fixture.go"`
- **After:** `"path": "../fixtures/go_test_fixture.go"`
- **Affected:** All 16 spec files in `specs/`

#### Runner: Parser Command Parsing
- **Issue:** Parser commands with spaces/quotes failed
- **Fix:** Use `shlex.split()` for proper shell-style parsing
- **Before:** `subprocess.run([self.parser_cmd, str(file_path)], ...)`
- **After:** `subprocess.run(shlex.split(self.parser_cmd) + [str(file_path)], ...)`

#### Runner: Config-Aware Validation
- **Issue:** Validation flags in config were ignored
- **Fix:** Read and apply `validate_signature`, `validate_value`, `validate_parent` from spec config
- **Impact:** Parent validation now skippable via `"validate_parent": false`

### Status After v1.1

| Category | Passing | Total |
|----------|---------|-------|
| Tree-sitter | 7 | 8 |
| Config | 4 | 8 |
| **Total** | **11** | **16** |

---

## [1.0.0] - 2026-01-29

### Added
- Initial UPTS schema and runner
- Specs for 16 languages: Go, Python, TypeScript, Rust, C, C++, CUDA, Java, YAML, TOML, JSON, INI, Shell, Dockerfile, SQL, Cypher
- Test fixtures for all 16 languages
- Fallback parsers for config languages
- Documentation: README, PARSER_ROADMAP, PARSER_GAPS, PARSER_FIX_GUIDE, INCONSISTENCIES_REPORT

### Passing Languages (v1.0)
- Go (100%)
- Python (100%)
- TypeScript (100%)
