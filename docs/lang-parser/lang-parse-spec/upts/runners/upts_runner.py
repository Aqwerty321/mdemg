#!/usr/bin/env python3
"""
Universal Parser Test Specification (UPTS) Test Runner

Validates parser implementations against UPTS specifications.
Language-agnostic - works with any parser that outputs standardized Symbol JSON.

Usage:
    # Validate single spec
    python upts_runner.py validate --spec specs/typescript.upts.json --parser ./parse
    
    # Validate all specs
    python upts_runner.py validate-all --spec-dir specs/ --parser ./parse
    
    # Generate JSON report
    python upts_runner.py validate-all --spec-dir specs/ --parser ./parse --report report.json
    
    # Convert old expected.json to UPTS
    python upts_runner.py convert --input old.json --language python --fixture path/to/fixture.py

Parser Output Format:
    Your parser must output JSON with this structure:
    {
        "symbols": [
            {
                "name": "SymbolName",
                "type": "function|class|method|constant|interface|enum|type|struct",
                "line": 42,              // or "line_number"
                "exported": true,
                "parent": "ParentClass", // optional, for methods
                "signature": "...",      // optional
                "value": "...",          // optional, for constants
                "doc_comment": "..."     // optional
            }
        ]
    }

Author: reh3376
"""

from __future__ import annotations

import json
import subprocess
import sys
import re
import hashlib
from pathlib import Path
from dataclasses import dataclass, field
from typing import Optional, List, Dict, Any, Tuple, Set
from enum import Enum
from datetime import datetime
import argparse


# ============================================================
# CONSTANTS
# ============================================================

UPTS_VERSION = "1.0.0"

# Type compatibility groups - legitimate semantic equivalences only
# Parsers should output UPTS canonical names; these handle language variations
TYPE_COMPAT = {
    # Class/struct equivalence across languages
    "class": {"class", "struct"},
    "struct": {"class", "struct"},
    # Interface/trait/protocol semantic equivalence
    "interface": {"interface", "trait", "protocol"},
    "trait": {"interface", "trait", "protocol"},
    "protocol": {"interface", "trait", "protocol"},
}


# ============================================================
# DATA MODELS
# ============================================================

class Status(str, Enum):
    PASS = "pass"
    FAIL = "fail"
    WARN = "warn"
    SKIP = "skip"
    ERROR = "error"


@dataclass
class SymbolResult:
    """Result of validating one expected symbol"""
    name: str
    sym_type: str
    expected_line: int
    status: Status
    actual_line: Optional[int] = None
    actual_type: Optional[str] = None
    actual_parent: Optional[str] = None
    issues: List[str] = field(default_factory=list)
    pattern: Optional[str] = None
    tags: List[str] = field(default_factory=list)


@dataclass
class SpecResult:
    """Result of validating a UPTS spec"""
    spec_path: str
    language: str
    status: Status
    total_expected: int = 0
    matched: int = 0
    failed: int = 0
    warnings: int = 0
    skipped: int = 0
    symbol_results: List[SymbolResult] = field(default_factory=list)
    excluded_found: List[str] = field(default_factory=list)
    extra_symbols: List[Dict] = field(default_factory=list)
    error_message: Optional[str] = None
    duration_ms: float = 0.0
    
    @property
    def pass_rate(self) -> float:
        return (self.matched / self.total_expected * 100) if self.total_expected else 0.0
    
    def to_dict(self) -> Dict:
        return {
            "spec_path": self.spec_path,
            "language": self.language,
            "status": self.status.value,
            "total_expected": self.total_expected,
            "matched": self.matched,
            "failed": self.failed,
            "pass_rate": round(self.pass_rate, 2),
            "duration_ms": round(self.duration_ms, 2),
            "failures": [
                {"name": r.name, "type": r.sym_type, "expected_line": r.expected_line,
                 "actual_line": r.actual_line, "issues": r.issues, "pattern": r.pattern}
                for r in self.symbol_results if r.status == Status.FAIL
            ],
            "excluded_found": self.excluded_found,
            "extra_symbol_count": len(self.extra_symbols),
            "error": self.error_message
        }


@dataclass
class TestReport:
    """Full test report"""
    timestamp: str
    total_specs: int
    passed: int
    failed: int
    errors: int
    results: List[SpecResult] = field(default_factory=list)
    
    @property
    def pass_rate(self) -> float:
        return (self.passed / self.total_specs * 100) if self.total_specs else 0.0
    
    def to_dict(self) -> Dict:
        return {
            "timestamp": self.timestamp,
            "upts_version": UPTS_VERSION,
            "summary": {
                "total_specs": self.total_specs,
                "passed": self.passed,
                "failed": self.failed,
                "errors": self.errors,
                "pass_rate": round(self.pass_rate, 2)
            },
            "results": [r.to_dict() for r in self.results]
        }


# ============================================================
# SPEC LOADER
# ============================================================

class UPTSLoader:
    """Load and validate UPTS specification files"""
    
    def __init__(self, spec_path: Path):
        self.spec_path = spec_path
        self.spec: Dict = {}
        self.errors: List[str] = []
    
    def load(self) -> bool:
        """Load spec file and validate structure"""
        try:
            self.spec = json.loads(self.spec_path.read_text(encoding='utf-8'))
        except json.JSONDecodeError as e:
            self.errors.append(f"Invalid JSON: {e}")
            return False
        except Exception as e:
            self.errors.append(f"Load error: {e}")
            return False
        
        # Validate required fields
        for field in ["upts_version", "language", "fixture", "expected"]:
            if field not in self.spec:
                self.errors.append(f"Missing required field: {field}")
        
        # Validate fixture
        fixture = self.spec.get("fixture", {})
        ftype = fixture.get("type")
        if ftype == "file" and "path" not in fixture:
            self.errors.append("Fixture type 'file' requires 'path'")
        elif ftype == "inline" and "content" not in fixture:
            self.errors.append("Fixture type 'inline' requires 'content'")
        
        # Validate symbols have required fields
        for i, sym in enumerate(self.spec.get("expected", {}).get("symbols", [])):
            for req in ["name", "type", "line"]:
                if req not in sym:
                    self.errors.append(f"Symbol {i} missing '{req}'")
        
        return len(self.errors) == 0
    
    @property
    def language(self) -> str:
        return self.spec.get("language", "unknown")
    
    @property
    def config(self) -> Dict:
        return self.spec.get("config", {})
    
    @property
    def fixture_path(self) -> Optional[Path]:
        fixture = self.spec.get("fixture", {})
        if fixture.get("type") == "file":
            # Relative to spec file's directory
            return self.spec_path.parent / fixture.get("path", "")
        return None
    
    @property
    def fixture_content(self) -> Optional[str]:
        fixture = self.spec.get("fixture", {})
        if fixture.get("type") == "inline":
            return fixture.get("content")
        path = self.fixture_path
        if path and path.exists():
            return path.read_text(encoding='utf-8')
        return None
    
    @property
    def expected_symbols(self) -> List[Dict]:
        return self.spec.get("expected", {}).get("symbols", [])
    
    @property
    def excluded_symbols(self) -> List[Dict]:
        return self.spec.get("expected", {}).get("excluded", [])


# ============================================================
# PARSER INTERFACE
# ============================================================

class ParserInterface:
    """Interface for calling parser implementations"""

    def __init__(self, parser_cmd: str):
        self.parser_cmd = parser_cmd

    def parse(self, file_path: Path) -> Tuple[List[Dict], Optional[str]]:
        """
        Run parser on file and return normalized symbols.

        Returns: (symbols_list, error_message)
        """
        try:
            # Support parser commands with arguments (e.g., "./bin/extract-symbols --json")
            import shlex
            cmd_parts = shlex.split(self.parser_cmd)
            cmd_parts.append(str(file_path))

            result = subprocess.run(
                cmd_parts,
                capture_output=True,
                text=True,
                timeout=60
            )
            
            if result.returncode != 0:
                return [], f"Parser exit {result.returncode}: {result.stderr[:500]}"
            
            output = json.loads(result.stdout)
            symbols = self._normalize(output.get("symbols", []))
            return symbols, None
            
        except subprocess.TimeoutExpired:
            return [], "Parser timeout (60s)"
        except json.JSONDecodeError as e:
            return [], f"Invalid JSON output: {e}"
        except FileNotFoundError:
            return [], f"Parser not found: {self.parser_cmd}"
        except Exception as e:
            return [], f"Parser error: {e}"
    
    def _normalize(self, symbols: List[Dict]) -> List[Dict]:
        """Normalize field names"""
        normalized = []
        for sym in symbols:
            normalized.append({
                "name": sym.get("name", ""),
                "type": sym.get("type", ""),
                "line": sym.get("line") or sym.get("line_number", 0),
                "exported": sym.get("exported", True),
                "parent": sym.get("parent", ""),
                "signature": sym.get("signature", ""),
                "value": sym.get("value", ""),
                "doc_comment": sym.get("doc_comment", ""),
            })
        return normalized


class MockParser(ParserInterface):
    """Mock parser for testing the runner"""
    
    def __init__(self, mock_output: List[Dict]):
        super().__init__("mock")
        self.mock_output = mock_output
    
    def parse(self, file_path: Path) -> Tuple[List[Dict], Optional[str]]:
        return self._normalize(self.mock_output), None


# ============================================================
# VALIDATOR
# ============================================================

class Validator:
    """Validate parser output against UPTS spec"""
    
    def __init__(self, spec: UPTSLoader, parser: ParserInterface):
        self.spec = spec
        self.parser = parser
    
    def validate(self) -> SpecResult:
        """Run validation"""
        import time
        start = time.time()
        
        result = SpecResult(
            spec_path=str(self.spec.spec_path),
            language=self.spec.language,
            status=Status.PASS,
            total_expected=len(self.spec.expected_symbols)
        )
        
        # Get fixture path
        fixture_path = self.spec.fixture_path
        if not fixture_path:
            result.status = Status.ERROR
            result.error_message = "No fixture path"
            return result
        
        if not fixture_path.exists():
            result.status = Status.ERROR
            result.error_message = f"Fixture not found: {fixture_path}"
            return result
        
        # Parse
        actual_symbols, error = self.parser.parse(fixture_path)
        if error:
            result.status = Status.ERROR
            result.error_message = error
            result.duration_ms = (time.time() - start) * 1000
            return result
        
        # Config
        line_tolerance = self.spec.config.get("line_tolerance", 2)
        
        # Index actual symbols by name
        actual_by_name: Dict[str, List[Dict]] = {}
        for sym in actual_symbols:
            name = sym["name"]
            actual_by_name.setdefault(name, []).append(sym)
        
        matched_ids: Set[str] = set()
        
        # Get validation config
        config = self.spec.config
        validate_signature = config.get("validate_signature", True)
        validate_parent = config.get("validate_parent", True)
        validate_value = config.get("validate_value", True)

        # Validate each expected symbol
        for expected in self.spec.expected_symbols:
            sym_result = self._validate_symbol(
                expected, actual_by_name, matched_ids, line_tolerance,
                validate_signature=validate_signature,
                validate_parent=validate_parent,
                validate_value=validate_value
            )
            result.symbol_results.append(sym_result)
            
            if sym_result.status == Status.PASS:
                result.matched += 1
            elif sym_result.status == Status.FAIL:
                result.failed += 1
            elif sym_result.status == Status.WARN:
                result.warnings += 1
            elif sym_result.status == Status.SKIP:
                result.skipped += 1
        
        # Check excluded
        for excl in self.spec.excluded_symbols:
            name = excl.get("name", "")
            pattern = excl.get("name_pattern")
            
            if name and name in actual_by_name:
                result.excluded_found.append(name)
            elif pattern:
                regex = re.compile(pattern)
                for n in actual_by_name:
                    if regex.match(n):
                        result.excluded_found.append(n)
        
        # Collect extra symbols
        expected_names = {s["name"] for s in self.spec.expected_symbols}
        for sym in actual_symbols:
            sid = f"{sym['name']}:{sym['line']}"
            if sid not in matched_ids and sym["name"] not in expected_names:
                result.extra_symbols.append(sym)
        
        # Final status
        if result.failed > 0:
            result.status = Status.FAIL
        elif result.warnings > 0 or result.excluded_found:
            result.status = Status.WARN
        
        result.duration_ms = (time.time() - start) * 1000
        return result
    
    def _validate_symbol(
        self,
        expected: Dict,
        actual_by_name: Dict[str, List[Dict]],
        matched_ids: Set[str],
        line_tolerance: int,
        validate_signature: bool = True,
        validate_parent: bool = True,
        validate_value: bool = True
    ) -> SymbolResult:
        """Validate single symbol"""

        name = expected["name"]
        exp_type = expected["type"]
        exp_line = expected["line"]
        exp_parent = expected.get("parent", "")
        is_optional = expected.get("optional", False)
        pattern = expected.get("pattern")
        tags = expected.get("tags", [])
        
        candidates = actual_by_name.get(name, [])
        
        if not candidates:
            if is_optional:
                return SymbolResult(name, exp_type, exp_line, Status.SKIP,
                                   issues=["Optional symbol not found"], pattern=pattern, tags=tags)
            return SymbolResult(name, exp_type, exp_line, Status.FAIL,
                               issues=["Symbol not found"], pattern=pattern, tags=tags)
        
        # Find best match
        best_match = None
        best_score = -1
        
        for actual in candidates:
            score = 0
            
            # Type match
            actual_type = actual["type"]
            if actual_type == exp_type:
                score += 10
            elif self._types_compatible(exp_type, actual_type):
                score += 5
            
            # Line match
            line_diff = abs(actual["line"] - exp_line)
            if line_diff == 0:
                score += 5
            elif line_diff <= line_tolerance:
                score += 3
            
            # Parent match
            if exp_parent:
                if actual["parent"] == exp_parent:
                    score += 5
                elif not actual["parent"]:
                    score -= 2
            
            if score > best_score:
                best_score = score
                best_match = actual
        
        if not best_match:
            return SymbolResult(name, exp_type, exp_line, Status.FAIL,
                               issues=["No matching candidate"], pattern=pattern, tags=tags)
        
        # Mark matched
        matched_ids.add(f"{best_match['name']}:{best_match['line']}")
        
        # Collect issues
        issues = []
        actual_type = best_match["type"]
        actual_line = best_match["line"]
        actual_parent = best_match["parent"]
        
        if actual_type != exp_type and not self._types_compatible(exp_type, actual_type):
            issues.append(f"type: expected '{exp_type}', got '{actual_type}'")
        
        if abs(actual_line - exp_line) > line_tolerance:
            issues.append(f"line: expected {exp_line}, got {actual_line}")
        
        if validate_parent and exp_parent and actual_parent != exp_parent:
            issues.append(f"parent: expected '{exp_parent}', got '{actual_parent or 'none'}'")

        # Signature validation (if enabled)
        if validate_signature:
            sig_contains = expected.get("signature_contains", [])
            actual_sig = best_match.get("signature", "").lower()
            for substr in sig_contains:
                if substr.lower() not in actual_sig:
                    issues.append(f"signature missing '{substr}'")

        # Value validation (if enabled)
        if validate_value and "value" in expected and expected["value"] is not None:
            actual_value = best_match.get("value", "")
            if actual_value != expected["value"]:
                issues.append(f"value: expected '{expected['value']}', got '{actual_value}'")
        
        if issues:
            return SymbolResult(name, exp_type, exp_line, Status.FAIL,
                               actual_line=actual_line, actual_type=actual_type,
                               actual_parent=actual_parent, issues=issues,
                               pattern=pattern, tags=tags)
        
        return SymbolResult(name, exp_type, exp_line, Status.PASS,
                           actual_line=actual_line, actual_type=actual_type,
                           actual_parent=actual_parent, pattern=pattern, tags=tags)
    
    def _types_compatible(self, exp: str, actual: str) -> bool:
        """Check if types are compatible"""
        return actual in TYPE_COMPAT.get(exp, {exp})


# ============================================================
# FORMAT CONVERTER
# ============================================================

class Converter:
    """Convert old expected.json format to UPTS"""
    
    PATTERN_MAP = {
        "constant": "P1_CONSTANT",
        "function": "P2_FUNCTION",
        "class": "P3_CLASS_STRUCT",
        "struct": "P3_CLASS_STRUCT",
        "interface": "P4_INTERFACE_TRAIT",
        "enum": "P5_ENUM",
        "method": "P6_METHOD",
        "type": "P7_TYPE_ALIAS",
    }
    
    VARIANTS = {
        "typescript": [".ts", ".tsx", ".mts"],
        "javascript": [".js", ".mjs", ".jsx"],
        "python": [".py", ".pyi"],
        "go": [".go"],
        "rust": [".rs"],
        "java": [".java"],
        "c": [".c", ".h"],
        "cpp": [".cpp", ".hpp", ".cc"],
        "cuda": [".cu", ".cuh"],
    }
    
    @classmethod
    def convert(cls, old: Dict, language: str, fixture_path: str) -> Dict:
        """Convert old expected.json to UPTS"""
        
        # Detect line field naming
        line_field = "line_number" if any(
            "line_number" in s for s in old.get("expected_symbols", [])
        ) else "line"
        
        symbols = []
        for s in old.get("expected_symbols", []):
            sym = {
                "name": s["name"],
                "type": s["type"],
                "line": s.get(line_field, s.get("line", 0)),
            }
            
            # Copy optional fields
            for f in ["exported", "parent", "signature", "value", "doc_comment"]:
                if f in s:
                    sym[f] = s[f]
            
            # Add pattern
            pattern = cls.PATTERN_MAP.get(s["type"])
            if pattern:
                sym["pattern"] = pattern
            
            symbols.append(sym)
        
        upts = {
            "upts_version": UPTS_VERSION,
            "language": language,
            "variants": cls.VARIANTS.get(language, [f".{language}"]),
            "metadata": {
                "created": datetime.now().strftime("%Y-%m-%d"),
                "description": f"Converted from {old.get('parser', language)}_expected.json",
                "parser_status": old.get("parser_status", "unknown"),
            },
            "config": {
                "line_tolerance": old.get("line_tolerance", 2),
                "require_all_symbols": True,
                "allow_extra_symbols": True
            },
            "fixture": {
                "type": "file",
                "path": fixture_path
            },
            "expected": {
                "symbol_count": {"min": old.get("expected_count", len(symbols))},
                "symbols": symbols
            }
        }
        
        # Copy enhancements
        if "enhancements_made" in old:
            upts["metadata"]["enhancements"] = old["enhancements_made"]
        
        return upts


# ============================================================
# REPORTER
# ============================================================

class Reporter:
    """Generate test reports"""
    
    def __init__(self, color: bool = True):
        self.color = color
    
    def _c(self, text: str, color: str) -> str:
        if not self.color:
            return text
        codes = {"green": "\033[92m", "red": "\033[91m", "yellow": "\033[93m",
                 "blue": "\033[94m", "bold": "\033[1m", "reset": "\033[0m"}
        return f"{codes.get(color, '')}{text}{codes['reset']}"
    
    def print_result(self, r: SpecResult):
        """Print single result"""
        icons = {Status.PASS: self._c("✓ PASS", "green"),
                 Status.FAIL: self._c("✗ FAIL", "red"),
                 Status.WARN: self._c("⚠ WARN", "yellow"),
                 Status.ERROR: self._c("✗ ERROR", "red")}
        
        print(f"\n{'='*60}")
        print(f"{self._c(r.language.upper(), 'bold')} Parser Test")
        print(f"Spec: {r.spec_path}")
        print(f"Status: {icons.get(r.status, r.status.value)}")
        print(f"Matched: {r.matched}/{r.total_expected} ({r.pass_rate:.1f}%)")
        print(f"Duration: {r.duration_ms:.1f}ms")
        
        if r.error_message:
            print(f"{self._c('Error:', 'red')} {r.error_message}")
        
        failures = [s for s in r.symbol_results if s.status == Status.FAIL]
        if failures:
            print(f"\n{self._c('Failures:', 'red')}")
            for f in failures[:10]:
                print(f"  - {f.name} ({f.sym_type}): {', '.join(f.issues)}")
            if len(failures) > 10:
                print(f"  ... and {len(failures) - 10} more")
        
        if r.excluded_found:
            print(f"\n{self._c('Warning:', 'yellow')} Excluded symbols found: {', '.join(r.excluded_found)}")
    
    def print_summary(self, report: TestReport):
        """Print summary"""
        print(f"\n{'='*60}")
        print(f"{self._c('UPTS Test Summary', 'bold')}")
        print(f"{'='*60}")
        print(f"Total: {report.total_specs}")
        print(f"Passed: {self._c(str(report.passed), 'green')}")
        print(f"Failed: {self._c(str(report.failed), 'red')}")
        print(f"Errors: {report.errors}")
        print(f"Pass Rate: {report.pass_rate:.1f}%")
    
    def save_report(self, report: TestReport, path: Path):
        """Save JSON report"""
        path.write_text(json.dumps(report.to_dict(), indent=2))
        print(f"\nReport saved: {path}")


# ============================================================
# RUNNER
# ============================================================

class Runner:
    """Main test runner"""
    
    def __init__(self, parser: ParserInterface):
        self.parser = parser
        self.reporter = Reporter()
    
    def run_spec(self, spec_path: Path) -> SpecResult:
        """Run single spec"""
        loader = UPTSLoader(spec_path)
        
        if not loader.load():
            return SpecResult(
                spec_path=str(spec_path),
                language="unknown",
                status=Status.ERROR,
                error_message=f"Spec errors: {'; '.join(loader.errors)}"
            )
        
        validator = Validator(loader, self.parser)
        return validator.validate()
    
    def run_all(self, spec_dir: Path, pattern: str = "*.upts.json") -> TestReport:
        """Run all specs in directory"""
        specs = sorted(spec_dir.glob(pattern))
        
        results = []
        passed = failed = errors = 0
        
        for spec in specs:
            result = self.run_spec(spec)
            self.reporter.print_result(result)
            results.append(result)
            
            if result.status == Status.PASS:
                passed += 1
            elif result.status == Status.ERROR:
                errors += 1
            else:
                failed += 1
        
        report = TestReport(
            timestamp=datetime.now().isoformat(),
            total_specs=len(specs),
            passed=passed,
            failed=failed,
            errors=errors,
            results=results
        )
        
        self.reporter.print_summary(report)
        return report


# ============================================================
# CLI
# ============================================================

def main():
    parser = argparse.ArgumentParser(description="UPTS Test Runner")
    sub = parser.add_subparsers(dest="cmd", required=True)
    
    # validate
    val = sub.add_parser("validate", help="Validate single spec")
    val.add_argument("--spec", type=Path, required=True)
    val.add_argument("--parser", type=str, required=True)
    val.add_argument("--report", type=Path)
    
    # validate-all
    val_all = sub.add_parser("validate-all", help="Validate all specs")
    val_all.add_argument("--spec-dir", type=Path, required=True)
    val_all.add_argument("--parser", type=str, required=True)
    val_all.add_argument("--pattern", type=str, default="*.upts.json")
    val_all.add_argument("--report", type=Path)
    
    # convert
    conv = sub.add_parser("convert", help="Convert old format to UPTS")
    conv.add_argument("--input", type=Path, required=True)
    conv.add_argument("--language", type=str, required=True)
    conv.add_argument("--fixture", type=str, required=True)
    conv.add_argument("--output", type=Path)
    
    args = parser.parse_args()
    
    if args.cmd == "validate":
        runner = Runner(ParserInterface(args.parser))
        result = runner.run_spec(args.spec)
        runner.reporter.print_result(result)
        
        if args.report:
            report = TestReport(datetime.now().isoformat(), 1,
                              1 if result.status == Status.PASS else 0,
                              0 if result.status in (Status.PASS, Status.ERROR) else 1,
                              1 if result.status == Status.ERROR else 0,
                              [result])
            runner.reporter.save_report(report, args.report)
        
        sys.exit(0 if result.status == Status.PASS else 1)
    
    elif args.cmd == "validate-all":
        runner = Runner(ParserInterface(args.parser))
        report = runner.run_all(args.spec_dir, args.pattern)
        
        if args.report:
            runner.reporter.save_report(report, args.report)
        
        sys.exit(0 if report.failed == 0 and report.errors == 0 else 1)
    
    elif args.cmd == "convert":
        old = json.loads(args.input.read_text())
        upts = Converter.convert(old, args.language, args.fixture)
        
        if args.output:
            args.output.write_text(json.dumps(upts, indent=2))
            print(f"Converted: {args.output}")
        else:
            print(json.dumps(upts, indent=2))


if __name__ == "__main__":
    main()
