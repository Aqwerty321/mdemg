#!/usr/bin/env python3
"""
USTS Runner - Universal Security Test Specification Runner

Executes USTS security test specifications against MDEMG endpoints.

Usage:
    python usts_runner.py --spec specs/auth_required.usts.json
    python usts_runner.py --spec "specs/*.usts.json" --output results/
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
class TestResult:
    """Result of a single security test."""
    name: str
    passed: bool
    status_code: int
    response_time_ms: float
    assertions_passed: Dict[str, bool] = field(default_factory=dict)
    failures: List[str] = field(default_factory=list)
    response_body: str = ""

@dataclass
class SecurityTestResult:
    """Results from a complete security test specification."""
    spec_name: str
    category: str
    severity: str
    total_requests: int
    passed_requests: int
    failed_requests: int
    test_results: List[TestResult] = field(default_factory=list)
    start_time: datetime = field(default_factory=datetime.now)
    end_time: Optional[datetime] = None

    @property
    def passed(self) -> bool:
        return self.failed_requests == 0

    def to_dict(self) -> Dict[str, Any]:
        return {
            "spec_name": self.spec_name,
            "category": self.category,
            "severity": self.severity,
            "passed": self.passed,
            "total_requests": self.total_requests,
            "passed_requests": self.passed_requests,
            "failed_requests": self.failed_requests,
            "test_results": [
                {
                    "name": r.name,
                    "passed": r.passed,
                    "status_code": r.status_code,
                    "response_time_ms": r.response_time_ms,
                    "assertions_passed": r.assertions_passed,
                    "failures": r.failures,
                }
                for r in self.test_results
            ],
            "start_time": self.start_time.isoformat(),
            "end_time": self.end_time.isoformat() if self.end_time else None,
        }


def load_spec(spec_path: str) -> Dict[str, Any]:
    """Load a USTS specification file."""
    with open(spec_path) as f:
        return json.load(f)


def load_payload_file(payload_path: str, base_dir: str) -> List[str]:
    """Load payloads from a file."""
    full_path = Path(base_dir) / payload_path
    if not full_path.exists():
        return []
    with open(full_path) as f:
        return [line.strip() for line in f if line.strip() and not line.startswith("#")]


def render_variables(obj: Any, variables: Dict[str, str]) -> Any:
    """Replace {{variable}} placeholders with values."""
    if isinstance(obj, str):
        for key, value in variables.items():
            obj = obj.replace(f"{{{{{key}}}}}", value)
        return obj
    elif isinstance(obj, dict):
        return {k: render_variables(v, variables) for k, v in obj.items()}
    elif isinstance(obj, list):
        return [render_variables(item, variables) for item in obj]
    return obj


def check_assertion(assertion_name: str, assertion_value: Any, response: requests.Response) -> tuple[bool, str]:
    """Check a single assertion against a response. Returns (passed, failure_message)."""
    body = response.text.lower()

    if assertion_name == "status_code":
        if response.status_code != assertion_value:
            return False, f"Expected status {assertion_value}, got {response.status_code}"
        return True, ""

    if assertion_name == "status_in":
        if response.status_code not in assertion_value:
            return False, f"Expected status in {assertion_value}, got {response.status_code}"
        return True, ""

    if assertion_name == "body_contains":
        for pattern in assertion_value:
            if pattern.lower() not in body:
                return False, f"Body does not contain: {pattern}"
        return True, ""

    if assertion_name == "body_not_contains":
        for pattern in assertion_value:
            if pattern.lower() in body:
                return False, f"Body contains forbidden pattern: {pattern}"
        return True, ""

    if assertion_name == "headers_present":
        for header in assertion_value:
            if header.lower() not in [h.lower() for h in response.headers.keys()]:
                return False, f"Missing required header: {header}"
        return True, ""

    if assertion_name == "headers_not_present":
        for header in assertion_value:
            if header.lower() in [h.lower() for h in response.headers.keys()]:
                return False, f"Forbidden header present: {header}"
        return True, ""

    if assertion_name == "response_time_ms_max":
        actual = response.elapsed.total_seconds() * 1000
        if actual > assertion_value:
            return False, f"Response time {actual:.0f}ms exceeds max {assertion_value}ms"
        return True, ""

    return True, ""


def run_request(
    base_url: str,
    test_spec: Dict[str, Any],
    request_spec: Dict[str, Any],
    assertions: Dict[str, Any],
    variables: Dict[str, str],
) -> TestResult:
    """Execute a single test request and check assertions."""
    endpoint = test_spec["endpoint"]
    method = test_spec.get("method", "POST")

    # Merge and render headers
    headers = {"Content-Type": "application/json"}
    headers.update(request_spec.get("headers", {}))
    headers = render_variables(headers, variables)

    # Render payload
    payload = render_variables(request_spec.get("payload", {}), variables)

    url = urljoin(base_url, endpoint)

    result = TestResult(
        name=request_spec["name"],
        passed=True,
        status_code=0,
        response_time_ms=0,
    )

    try:
        start = time.perf_counter()
        if method == "GET":
            resp = requests.get(url, headers=headers, params=payload, timeout=30)
        elif method == "POST":
            resp = requests.post(url, headers=headers, json=payload, timeout=30)
        elif method == "PUT":
            resp = requests.put(url, headers=headers, json=payload, timeout=30)
        elif method == "DELETE":
            resp = requests.delete(url, headers=headers, timeout=30)
        elif method == "OPTIONS":
            resp = requests.options(url, headers=headers, timeout=30)
        else:
            result.passed = False
            result.failures.append(f"Unsupported method: {method}")
            return result

        result.response_time_ms = (time.perf_counter() - start) * 1000
        result.status_code = resp.status_code
        result.response_body = resp.text[:500]  # Truncate for storage

        # Check expected status from request spec
        if "expected_status" in request_spec:
            if resp.status_code != request_spec["expected_status"]:
                result.passed = False
                result.failures.append(
                    f"Expected status {request_spec['expected_status']}, got {resp.status_code}"
                )

        if "expected_status_range" in request_spec:
            low, high = request_spec["expected_status_range"]
            if not (low <= resp.status_code <= high):
                result.passed = False
                result.failures.append(
                    f"Expected status in range [{low}, {high}], got {resp.status_code}"
                )

        # Check global assertions
        for assertion_name, assertion_value in assertions.items():
            passed, failure = check_assertion(assertion_name, assertion_value, resp)
            result.assertions_passed[assertion_name] = passed
            if not passed:
                result.passed = False
                result.failures.append(failure)

    except requests.RequestException as e:
        result.passed = False
        result.failures.append(f"Request failed: {e}")

    return result


def run_security_test(
    spec: Dict[str, Any],
    base_url: str,
    variables: Dict[str, str],
) -> SecurityTestResult:
    """Run all requests in a security test specification."""
    test_info = spec["test"]
    assertions = spec.get("assertions", {})
    requests_list = spec.get("requests", [{}])

    result = SecurityTestResult(
        spec_name=test_info["name"],
        category=test_info["category"],
        severity=test_info.get("severity", "medium"),
        total_requests=len(requests_list),
        passed_requests=0,
        failed_requests=0,
    )

    for request_spec in requests_list:
        test_result = run_request(base_url, test_info, request_spec, assertions, variables)
        result.test_results.append(test_result)

        if test_result.passed:
            result.passed_requests += 1
        else:
            result.failed_requests += 1

    result.end_time = datetime.now()
    return result


def print_results(result: SecurityTestResult) -> None:
    """Print security test results."""
    severity_colors = {
        "critical": "\033[91m",  # Red
        "high": "\033[93m",      # Yellow
        "medium": "\033[94m",    # Blue
        "low": "\033[92m",       # Green
    }
    reset = "\033[0m"
    color = severity_colors.get(result.severity, "")

    status = "\u2713 PASS" if result.passed else "\u2717 FAIL"
    status_color = "\033[92m" if result.passed else "\033[91m"

    print(f"\n{'='*60}")
    print(f"Test: {result.spec_name}")
    print(f"Category: {result.category}")
    print(f"Severity: {color}{result.severity.upper()}{reset}")
    print(f"Status: {status_color}{status}{reset}")
    print(f"{'='*60}")

    for tr in result.test_results:
        tr_status = "\u2713" if tr.passed else "\u2717"
        tr_color = "\033[92m" if tr.passed else "\033[91m"
        print(f"  {tr_color}{tr_status}{reset} {tr.name} (HTTP {tr.status_code}, {tr.response_time_ms:.0f}ms)")

        for failure in tr.failures:
            print(f"      \033[91m{failure}{reset}")


def main():
    parser = argparse.ArgumentParser(description="USTS Security Test Runner")
    parser.add_argument("--spec", required=True, help="Path to USTS spec file(s)")
    parser.add_argument("--base-url", default="http://localhost:9999", help="MDEMG base URL")
    parser.add_argument("--api-key", help="Valid API key for authenticated tests")
    parser.add_argument("--output", help="Output directory for results")
    args = parser.parse_args()

    # Set up variables for template rendering
    variables = {}
    if args.api_key:
        variables["valid_api_key"] = args.api_key

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

        result = run_security_test(spec, args.base_url, variables)
        results.append(result)
        print_results(result)

        if not result.passed:
            all_passed = False

    # Save results if output directory specified
    if args.output:
        output_dir = Path(args.output)
        output_dir.mkdir(parents=True, exist_ok=True)

        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        output_file = output_dir / f"usts_results_{timestamp}.json"

        with open(output_file, "w") as f:
            json.dump({
                "timestamp": timestamp,
                "overall_passed": all_passed,
                "results": [r.to_dict() for r in results],
            }, f, indent=2)

        print(f"\nResults saved to: {output_file}")

    # Summary
    critical_failures = sum(1 for r in results if not r.passed and r.severity == "critical")
    high_failures = sum(1 for r in results if not r.passed and r.severity == "high")

    print(f"\n{'='*60}")
    print(f"Security Test Summary")
    print(f"{'='*60}")
    print(f"Total Tests:      {len(results)}")
    print(f"Passed:           {sum(1 for r in results if r.passed)}")
    print(f"Failed:           {sum(1 for r in results if not r.passed)}")
    print(f"Critical Failures: {critical_failures}")
    print(f"High Failures:    {high_failures}")
    print(f"{'='*60}")
    print(f"Overall: {'PASS' if all_passed else 'FAIL'}")
    print(f"{'='*60}")

    # Exit with error if critical or high severity tests failed
    if critical_failures > 0 or high_failures > 0:
        sys.exit(2)
    elif not all_passed:
        sys.exit(1)
    else:
        sys.exit(0)


if __name__ == "__main__":
    main()
