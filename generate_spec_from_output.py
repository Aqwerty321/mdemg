#!/usr/bin/env python3
"""
Generate UPTS spec from actual parser output with SHA256 hash.

This script creates a UPTS spec by:
1. Reading actual parser JSON output
2. Converting symbols to expected format
3. Computing SHA256 hash of the fixture file
4. Generating a complete, validated spec

Usage:
    # First, capture parser output:
    ./bin/extract-symbols --json fixtures/rust_test_fixture.rs > /tmp/rust_actual.json
    
    # Then generate spec with hash:
    python3 generate_spec_from_output.py rust /tmp/rust_actual.json ../fixtures/rust_test_fixture.rs

    # The fixture path argument can be:
    #   - Relative to spec location (for embedding in spec): ../fixtures/rust_test_fixture.rs
    #   - Absolute path (for hash computation): /full/path/to/fixture.rs

Options:
    --fixture-root DIR    Directory containing fixtures (for hash computation)
    --no-hash             Skip hash computation
    --output FILE         Write to file instead of stdout

Examples:
    # Basic usage
    python3 generate_spec_from_output.py rust /tmp/rust.json "../fixtures/rust_test_fixture.rs"
    
    # With explicit fixture root for hash computation
    python3 generate_spec_from_output.py rust /tmp/rust.json "../fixtures/rust_test_fixture.rs" \\
        --fixture-root /path/to/upts/specs
    
    # Output to file
    python3 generate_spec_from_output.py rust /tmp/rust.json "../fixtures/rust_test_fixture.rs" \\
        --output specs/rust.upts.json

Version: 1.1.0 (with SHA256 hash support)
"""

import json
import sys
import hashlib
import argparse
from pathlib import Path
from datetime import date
from typing import Optional


UPTS_VERSION = "1.1.0"


def compute_sha256(filepath: Path) -> str:
    """Compute SHA256 hash of a file."""
    sha256_hash = hashlib.sha256()
    with open(filepath, "rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            sha256_hash.update(chunk)
    return sha256_hash.hexdigest()


def resolve_fixture_path(fixture_rel_path: str, fixture_root: Optional[Path] = None) -> Optional[Path]:
    """
    Resolve the actual fixture file path for hash computation.
    
    Args:
        fixture_rel_path: The relative path that will go in the spec (e.g., "../fixtures/rust.rs")
        fixture_root: The directory the spec will be in (e.g., /path/to/specs/)
    
    Returns:
        Resolved absolute path to fixture, or None if cannot resolve
    """
    # If fixture_root provided, resolve relative to it
    if fixture_root:
        resolved = (fixture_root / fixture_rel_path).resolve()
        if resolved.exists():
            return resolved
    
    # Try as absolute path
    abs_path = Path(fixture_rel_path)
    if abs_path.is_absolute() and abs_path.exists():
        return abs_path
    
    # Try relative to current directory
    cwd_path = Path.cwd() / fixture_rel_path
    if cwd_path.exists():
        return cwd_path
    
    # Try common patterns
    for pattern in ["fixtures/", "../fixtures/", "upts/fixtures/"]:
        test_path = Path.cwd() / pattern / Path(fixture_rel_path).name
        if test_path.exists():
            return test_path
    
    return None


def generate_spec(
    lang: str,
    actual_json_path: str,
    fixture_rel_path: str,
    fixture_root: Optional[Path] = None,
    include_hash: bool = True
) -> dict:
    """
    Generate a UPTS spec from actual parser output.
    
    Args:
        lang: Language name (e.g., "rust", "c", "java")
        actual_json_path: Path to JSON file with parser output
        fixture_rel_path: Relative path to fixture (for spec)
        fixture_root: Directory to resolve fixture path from
        include_hash: Whether to compute and include SHA256 hash
    
    Returns:
        Complete UPTS spec dictionary
    """
    # Load actual parser output
    with open(actual_json_path) as f:
        actual = json.load(f)
    
    symbols = actual.get("symbols", [])
    
    if not symbols:
        print(f"WARNING: No symbols found in {actual_json_path}", file=sys.stderr)
    
    # Build expected symbols list
    expected_symbols = []
    for sym in symbols:
        entry = {
            "name": sym["name"],
            "type": sym["type"],
            "line": sym["line"]
        }
        
        # Include optional fields that have meaningful values
        if sym.get("exported") is not None:
            entry["exported"] = sym["exported"]
        if sym.get("parent"):
            entry["parent"] = sym["parent"]
        if sym.get("value"):
            entry["value"] = sym["value"]
        if sym.get("signature"):
            entry["signature"] = sym["signature"]
        if sym.get("line_end") and sym["line_end"] != sym["line"]:
            entry["line_end"] = sym["line_end"]
        if sym.get("doc_comment"):
            entry["doc_comment"] = sym["doc_comment"]
            
        expected_symbols.append(entry)
    
    # Language variants
    variants_map = {
        "rust": [".rs"],
        "c": [".c", ".h"],
        "cpp": [".cpp", ".hpp", ".cc", ".cxx", ".hh", ".hxx"],
        "cuda": [".cu", ".cuh"],
        "java": [".java"],
        "go": [".go"],
        "python": [".py", ".pyw"],
        "typescript": [".ts", ".tsx"],
        "javascript": [".js", ".jsx", ".mjs"],
        "yaml": [".yaml", ".yml"],
        "toml": [".toml"],
        "json": [".json", ".jsonc"],
        "ini": [".ini", ".env", ".cfg", ".properties"],
        "shell": [".sh", ".bash", ".zsh"],
        "dockerfile": ["Dockerfile", ".dockerfile"],
        "sql": [".sql"],
        "cypher": [".cypher", ".cql"],
    }
    
    # Build fixture object
    fixture_obj = {
        "type": "file",
        "path": fixture_rel_path
    }
    
    # Compute hash if requested
    if include_hash:
        resolved_path = resolve_fixture_path(fixture_rel_path, fixture_root)
        if resolved_path:
            fixture_obj["sha256"] = compute_sha256(resolved_path)
            print(f"Fixture hash: {fixture_obj['sha256'][:16]}...", file=sys.stderr)
        else:
            print(f"WARNING: Could not resolve fixture path for hash: {fixture_rel_path}", file=sys.stderr)
            print(f"  Tried fixture_root: {fixture_root}", file=sys.stderr)
            print(f"  Use --fixture-root to specify location, or --no-hash to skip", file=sys.stderr)
    
    # Build spec
    spec = {
        "upts_version": UPTS_VERSION,
        "language": lang,
        "variants": variants_map.get(lang, []),
        "metadata": {
            "author": "auto-generated",
            "created": str(date.today()),
            "description": f"{lang.upper()} parser spec - generated from actual parser output",
            "parser_status": "functional"
        },
        "config": {
            "line_tolerance": 2,
            "require_all_symbols": True,
            "allow_extra_symbols": True,
            "validate_parent": True
        },
        "fixture": fixture_obj,
        "expected": {
            "symbol_count": {
                "min": max(1, len(symbols) - 5),
                "max": len(symbols) + 10
            },
            "symbols": expected_symbols
        }
    }
    
    return spec


def main():
    parser = argparse.ArgumentParser(
        description="Generate UPTS spec from actual parser output",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s rust /tmp/rust_actual.json "../fixtures/rust_test_fixture.rs"
  %(prog)s java /tmp/java.json "../fixtures/java_test_fixture.java" --fixture-root ./specs/
  %(prog)s c /tmp/c.json "../fixtures/c_test_fixture.c" --no-hash --output c.upts.json
        """
    )
    
    parser.add_argument("lang", help="Language name (e.g., rust, c, java)")
    parser.add_argument("actual_json", help="Path to JSON file with parser output")
    parser.add_argument("fixture_path", help="Relative path to fixture (as it will appear in spec)")
    parser.add_argument("--fixture-root", type=Path, 
                        help="Directory to resolve fixture path from (for hash computation)")
    parser.add_argument("--no-hash", action="store_true",
                        help="Skip SHA256 hash computation")
    parser.add_argument("--output", "-o", type=Path,
                        help="Output file (default: stdout)")
    
    args = parser.parse_args()
    
    try:
        spec = generate_spec(
            lang=args.lang,
            actual_json_path=args.actual_json,
            fixture_rel_path=args.fixture_path,
            fixture_root=args.fixture_root,
            include_hash=not args.no_hash
        )
        
        output_json = json.dumps(spec, indent=2)
        
        if args.output:
            args.output.write_text(output_json)
            print(f"Wrote spec to: {args.output}", file=sys.stderr)
        else:
            print(output_json)
            
    except FileNotFoundError as e:
        print(f"Error: File not found: {e}", file=sys.stderr)
        sys.exit(1)
    except json.JSONDecodeError as e:
        print(f"Error: Invalid JSON: {e}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
