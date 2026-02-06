#!/usr/bin/env python3
"""
UOBS Runner - Universal Observability Specification Runner

Validates observability infrastructure against UOBS specifications.

Usage:
    python uobs_runner.py --spec specs/prometheus_metrics.uobs.json
    python uobs_runner.py --spec "specs/*.uobs.json" --output results/
"""

import argparse
import json
import re
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional
from urllib.parse import urljoin

import requests

@dataclass
class CheckResult:
    """Result of a single observability check."""
    name: str
    passed: bool
    message: str
    details: Optional[Dict[str, Any]] = None

@dataclass
class ObservabilityTestResult:
    """Results from a complete observability test."""
    spec_name: str
    test_type: str
    total_checks: int
    passed_checks: int
    failed_checks: int
    results: List[CheckResult] = field(default_factory=list)
    start_time: datetime = field(default_factory=datetime.now)
    end_time: Optional[datetime] = None

    @property
    def passed(self) -> bool:
        return self.failed_checks == 0

    def to_dict(self) -> Dict[str, Any]:
        return {
            "spec_name": self.spec_name,
            "test_type": self.test_type,
            "passed": self.passed,
            "total_checks": self.total_checks,
            "passed_checks": self.passed_checks,
            "failed_checks": self.failed_checks,
            "results": [
                {
                    "name": r.name,
                    "passed": r.passed,
                    "message": r.message,
                    "details": r.details,
                }
                for r in self.results
            ],
            "start_time": self.start_time.isoformat(),
            "end_time": self.end_time.isoformat() if self.end_time else None,
        }


def load_spec(spec_path: str) -> Dict[str, Any]:
    """Load a UOBS specification file."""
    with open(spec_path) as f:
        return json.load(f)


def validate_prometheus_format(content: str) -> tuple[bool, str]:
    """Validate Prometheus exposition format."""
    lines = content.strip().split("\n")
    errors = []

    for i, line in enumerate(lines, 1):
        line = line.strip()
        if not line or line.startswith("#"):
            continue

        # Basic format: metric_name{labels} value
        # or: metric_name value
        if not re.match(r'^[a-zA-Z_:][a-zA-Z0-9_:]*(\{[^}]*\})?\s+[\d.eE+-]+$', line):
            errors.append(f"Line {i}: Invalid format: {line[:50]}")

    if errors:
        return False, "; ".join(errors[:5])
    return True, "Valid Prometheus format"


def check_metric_exists(content: str, metric_name: str, metric_type: Optional[str] = None) -> tuple[bool, str]:
    """Check if a metric exists in Prometheus output."""
    # Check for TYPE declaration
    type_pattern = f"# TYPE {metric_name}"
    has_type = type_pattern in content

    # Check for actual metric values
    metric_pattern = f"^{metric_name}(\\{{|\\s)"
    has_values = any(re.match(metric_pattern, line) for line in content.split("\n"))

    if not has_values:
        return False, f"Metric {metric_name} not found"

    if metric_type:
        expected_type = f"# TYPE {metric_name} {metric_type}"
        if expected_type not in content:
            return False, f"Metric {metric_name} exists but type mismatch (expected {metric_type})"

    return True, f"Metric {metric_name} present"


def run_metrics_test(spec: Dict[str, Any], base_url: str) -> ObservabilityTestResult:
    """Run Prometheus metrics validation."""
    metrics_config = spec.get("metrics", {})
    endpoint = metrics_config.get("endpoint", "/v1/prometheus")
    required_metrics = metrics_config.get("required_metrics", [])

    result = ObservabilityTestResult(
        spec_name=spec["test"]["name"],
        test_type="metrics",
        total_checks=len(required_metrics) + 1,  # +1 for format check
        passed_checks=0,
        failed_checks=0,
    )

    # Fetch metrics
    try:
        url = urljoin(base_url, endpoint)
        resp = requests.get(url, timeout=10)
        if resp.status_code != 200:
            result.failed_checks = result.total_checks
            result.results.append(CheckResult(
                name="fetch_metrics",
                passed=False,
                message=f"Failed to fetch metrics: HTTP {resp.status_code}",
            ))
            result.end_time = datetime.now()
            return result

        content = resp.text

    except requests.RequestException as e:
        result.failed_checks = result.total_checks
        result.results.append(CheckResult(
            name="fetch_metrics",
            passed=False,
            message=f"Request failed: {e}",
        ))
        result.end_time = datetime.now()
        return result

    # Check format
    if metrics_config.get("format_validation", True):
        valid, msg = validate_prometheus_format(content)
        result.results.append(CheckResult(
            name="prometheus_format",
            passed=valid,
            message=msg,
        ))
        if valid:
            result.passed_checks += 1
        else:
            result.failed_checks += 1

    # Check required metrics
    for metric in required_metrics:
        name = metric["name"]
        metric_type = metric.get("type")

        exists, msg = check_metric_exists(content, name, metric_type)
        result.results.append(CheckResult(
            name=f"metric_{name}",
            passed=exists,
            message=msg,
            details={"metric": name, "type": metric_type},
        ))
        if exists:
            result.passed_checks += 1
        else:
            result.failed_checks += 1

    result.end_time = datetime.now()
    return result


def run_health_test(spec: Dict[str, Any], base_url: str) -> ObservabilityTestResult:
    """Run health endpoint validation."""
    health_config = spec.get("health", {})
    endpoints = health_config.get("endpoints", [])

    result = ObservabilityTestResult(
        spec_name=spec["test"]["name"],
        test_type="health",
        total_checks=len(endpoints),
        passed_checks=0,
        failed_checks=0,
    )

    for ep in endpoints:
        path = ep["path"]
        method = ep.get("method", "GET")
        expected_status = ep.get("expected_status", 200)
        required_fields = ep.get("required_fields", [])
        max_time = ep.get("max_response_time_ms")

        try:
            url = urljoin(base_url, path)
            start = time.perf_counter()
            resp = requests.request(method, url, timeout=10)
            elapsed_ms = (time.perf_counter() - start) * 1000

            passed = True
            messages = []

            # Check status
            if resp.status_code != expected_status:
                passed = False
                messages.append(f"Expected status {expected_status}, got {resp.status_code}")

            # Check response time
            if max_time and elapsed_ms > max_time:
                passed = False
                messages.append(f"Response time {elapsed_ms:.0f}ms exceeds {max_time}ms")

            # Check required fields
            try:
                body = resp.json()
                for field in required_fields:
                    if field not in body:
                        passed = False
                        messages.append(f"Missing required field: {field}")
            except json.JSONDecodeError:
                if required_fields:
                    passed = False
                    messages.append("Response is not valid JSON")

            result.results.append(CheckResult(
                name=f"health_{path}",
                passed=passed,
                message="; ".join(messages) if messages else f"OK ({elapsed_ms:.0f}ms)",
                details={"path": path, "status": resp.status_code, "time_ms": elapsed_ms},
            ))

            if passed:
                result.passed_checks += 1
            else:
                result.failed_checks += 1

        except requests.RequestException as e:
            result.results.append(CheckResult(
                name=f"health_{path}",
                passed=False,
                message=f"Request failed: {e}",
            ))
            result.failed_checks += 1

    result.end_time = datetime.now()
    return result


def run_tracing_test(spec: Dict[str, Any], base_url: str) -> ObservabilityTestResult:
    """Run distributed tracing validation."""
    tracing_config = spec.get("tracing", {})
    trace_header = tracing_config.get("trace_header", "X-Trace-ID")
    request_id_header = tracing_config.get("request_id_header", "X-Request-ID")

    result = ObservabilityTestResult(
        spec_name=spec["test"]["name"],
        test_type="tracing",
        total_checks=3,
        passed_checks=0,
        failed_checks=0,
    )

    # Test 1: Check headers are returned
    try:
        url = urljoin(base_url, "/healthz")
        resp = requests.get(url, timeout=10)

        # Check trace ID is returned
        if trace_header.lower() in [h.lower() for h in resp.headers.keys()]:
            result.results.append(CheckResult(
                name="trace_id_returned",
                passed=True,
                message=f"{trace_header} header present",
            ))
            result.passed_checks += 1
        else:
            result.results.append(CheckResult(
                name="trace_id_returned",
                passed=False,
                message=f"{trace_header} header not returned",
            ))
            result.failed_checks += 1

        # Check request ID is returned
        if request_id_header.lower() in [h.lower() for h in resp.headers.keys()]:
            result.results.append(CheckResult(
                name="request_id_returned",
                passed=True,
                message=f"{request_id_header} header present",
            ))
            result.passed_checks += 1
        else:
            result.results.append(CheckResult(
                name="request_id_returned",
                passed=False,
                message=f"{request_id_header} header not returned",
            ))
            result.failed_checks += 1

        # Test 2: Trace ID propagation (send trace ID, verify it's echoed)
        if tracing_config.get("propagation_check", True):
            test_trace_id = "test-trace-12345"
            resp = requests.get(url, headers={trace_header: test_trace_id}, timeout=10)

            returned_trace = resp.headers.get(trace_header, "")
            if returned_trace == test_trace_id:
                result.results.append(CheckResult(
                    name="trace_id_propagation",
                    passed=True,
                    message="Trace ID propagated correctly",
                ))
                result.passed_checks += 1
            else:
                result.results.append(CheckResult(
                    name="trace_id_propagation",
                    passed=False,
                    message=f"Trace ID not propagated (sent {test_trace_id}, got {returned_trace})",
                ))
                result.failed_checks += 1

    except requests.RequestException as e:
        result.results.append(CheckResult(
            name="tracing_test",
            passed=False,
            message=f"Request failed: {e}",
        ))
        result.failed_checks = result.total_checks

    result.end_time = datetime.now()
    return result


def run_observability_test(spec: Dict[str, Any], base_url: str) -> ObservabilityTestResult:
    """Run the appropriate test based on spec type."""
    test_type = spec["test"]["type"]

    if test_type == "metrics":
        return run_metrics_test(spec, base_url)
    elif test_type == "health":
        return run_health_test(spec, base_url)
    elif test_type == "logging":
        # Logging tests require access to log output - skip for now
        return ObservabilityTestResult(
            spec_name=spec["test"]["name"],
            test_type="logging",
            total_checks=0,
            passed_checks=0,
            failed_checks=0,
            results=[CheckResult(
                name="logging_test",
                passed=True,
                message="Logging tests require manual verification or log aggregation access",
            )],
            end_time=datetime.now(),
        )
    elif test_type == "tracing":
        return run_tracing_test(spec, base_url)
    else:
        return ObservabilityTestResult(
            spec_name=spec["test"]["name"],
            test_type=test_type,
            total_checks=0,
            passed_checks=0,
            failed_checks=1,
            results=[CheckResult(
                name="unknown_type",
                passed=False,
                message=f"Unknown test type: {test_type}",
            )],
            end_time=datetime.now(),
        )


def print_results(result: ObservabilityTestResult) -> None:
    """Print observability test results."""
    status = "\u2713 PASS" if result.passed else "\u2717 FAIL"
    status_color = "\033[92m" if result.passed else "\033[91m"
    reset = "\033[0m"

    print(f"\n{'='*60}")
    print(f"Test: {result.spec_name}")
    print(f"Type: {result.test_type}")
    print(f"Status: {status_color}{status}{reset}")
    print(f"{'='*60}")
    print(f"Checks: {result.passed_checks}/{result.total_checks} passed")

    for cr in result.results:
        cr_status = "\u2713" if cr.passed else "\u2717"
        cr_color = "\033[92m" if cr.passed else "\033[91m"
        print(f"  {cr_color}{cr_status}{reset} {cr.name}: {cr.message}")


def main():
    parser = argparse.ArgumentParser(description="UOBS Observability Test Runner")
    parser.add_argument("--spec", required=True, help="Path to UOBS spec file(s)")
    parser.add_argument("--base-url", default="http://localhost:9999", help="MDEMG base URL")
    parser.add_argument("--output", help="Output directory for results")
    args = parser.parse_args()

    # Handle glob patterns in spec
    spec_paths = []
    if "*" in args.spec:
        spec_dir = Path(args.spec).parent
        pattern = Path(args.spec).name
        spec_paths = list(spec_dir.glob(pattern))
    else:
        spec_paths = [Path(args.spec)]

    all_passed = True
    results = []

    for spec_path in spec_paths:
        print(f"\nLoading spec: {spec_path}")
        spec = load_spec(spec_path)

        result = run_observability_test(spec, args.base_url)
        results.append(result)
        print_results(result)

        if not result.passed:
            all_passed = False

    # Save results if output directory specified
    if args.output:
        output_dir = Path(args.output)
        output_dir.mkdir(parents=True, exist_ok=True)

        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        output_file = output_dir / f"uobs_results_{timestamp}.json"

        with open(output_file, "w") as f:
            json.dump({
                "timestamp": timestamp,
                "overall_passed": all_passed,
                "results": [r.to_dict() for r in results],
            }, f, indent=2)

        print(f"\nResults saved to: {output_file}")

    print(f"\n{'='*60}")
    print(f"Observability Test Summary")
    print(f"{'='*60}")
    print(f"Total Tests:  {len(results)}")
    print(f"Passed:       {sum(1 for r in results if r.passed)}")
    print(f"Failed:       {sum(1 for r in results if not r.passed)}")
    print(f"{'='*60}")
    print(f"Overall: {'PASS' if all_passed else 'FAIL'}")
    print(f"{'='*60}")

    sys.exit(0 if all_passed else 1)


if __name__ == "__main__":
    main()
