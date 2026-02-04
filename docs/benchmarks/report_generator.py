#!/usr/bin/env python3
"""
MDEMG Benchmark Report Generator v1.0

Generates human-readable markdown reports from benchmark comparison results.

Usage as library:
    from report_generator import ReportGenerator
    generator = ReportGenerator()
    markdown = generator.generate(comparison_result, config)

Usage as CLI:
    python report_generator.py comparison.json config.json report.md
"""

import json
import sys
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Any


@dataclass
class BenchmarkConfig:
    endpoint: str = ""
    space_id: str = ""
    questions_file: str = ""
    question_count: int = 0
    runs_per_mode: int = 3
    created_at: str = ""


class ReportGenerator:
    """Generates markdown reports from benchmark results."""

    def __init__(self):
        self.timestamp = datetime.utcnow().isoformat() + "Z"

    def _significance_stars(self, p_value: float) -> str:
        """Return significance stars based on p-value."""
        if p_value < 0.001:
            return "***"
        elif p_value < 0.01:
            return "**"
        elif p_value < 0.05:
            return "*"
        return ""

    def _format_delta(self, delta: float, baseline: float) -> str:
        """Format delta with sign and percentage."""
        if baseline > 0:
            pct = (delta / baseline) * 100
            return f"{delta:+.3f} ({pct:+.1f}%)"
        return f"{delta:+.3f}"

    def generate(self, comparison: Dict, config: Optional[BenchmarkConfig] = None) -> str:
        """Generate markdown report from comparison results.

        Args:
            comparison: Dictionary with comparison results from StatsAnalyzer
            config: Optional benchmark configuration

        Returns:
            Markdown string
        """
        lines = []

        # Header
        lines.append("# MDEMG Benchmark Comparison Report")
        lines.append("")
        lines.append(f"**Generated:** {self.timestamp}")
        if config:
            lines.append(f"**Space ID:** {config.space_id}")
            lines.append(f"**Questions:** {config.question_count}")
            lines.append(f"**Runs per mode:** {config.runs_per_mode}")
        lines.append("")

        # Executive Summary
        lines.append("## Executive Summary")
        lines.append("")

        baseline = comparison.get('baseline', {})
        mdemg = comparison.get('mdemg', {})
        delta = comparison.get('delta', {})
        stats = comparison.get('statistical_tests', {})

        baseline_mean = baseline.get('mean_of_means', 0)
        mdemg_mean = mdemg.get('mean_of_means', 0)
        mean_delta = delta.get('mean_delta', 0)
        pct_change = delta.get('percent_change', 0)
        p_value = stats.get('paired_ttest', {}).get('p_value', 1)
        effect = stats.get('effect_size', 'unknown')
        cohens_d = stats.get('cohens_d', 0)

        stars = self._significance_stars(p_value)

        if mean_delta > 0 and p_value < 0.05:
            verdict = f"MDEMG shows a **statistically significant improvement** ({pct_change:+.1f}%) with a {effect} effect size."
        elif mean_delta > 0:
            verdict = f"MDEMG shows improvement ({pct_change:+.1f}%) but it is **not statistically significant**."
        elif mean_delta < 0 and p_value < 0.05:
            verdict = f"MDEMG shows a **statistically significant decline** ({pct_change:+.1f}%)."
        elif mean_delta < 0:
            verdict = f"MDEMG shows slight decline ({pct_change:+.1f}%) but it is **not statistically significant**."
        else:
            verdict = "No significant difference between baseline and MDEMG."

        lines.append(verdict)
        lines.append("")

        # Summary Table
        lines.append("## Summary")
        lines.append("")
        lines.append("| Metric | Baseline | MDEMG | Delta | Significance |")
        lines.append("|--------|:--------:|:-----:|:-----:|:------------:|")

        # Mean Score
        lines.append(f"| Mean Score | {baseline_mean:.3f} | {mdemg_mean:.3f} | "
                     f"{self._format_delta(mean_delta, baseline_mean)} | p={p_value:.4f} {stars} |")

        # High Score Rate (if available from per_run data)
        baseline_runs = baseline.get('per_run', [])
        mdemg_runs = mdemg.get('per_run', [])

        # Std
        baseline_std = baseline.get('pooled_std', baseline.get('std_of_means', 0))
        mdemg_std = mdemg.get('pooled_std', mdemg.get('std_of_means', 0))
        lines.append(f"| Std Dev | {baseline_std:.3f} | {mdemg_std:.3f} | - | - |")

        # CV
        baseline_cv = baseline.get('cv_across_runs', 0)
        mdemg_cv = mdemg.get('cv_across_runs', 0)
        lines.append(f"| CV (across runs) | {baseline_cv:.1f}% | {mdemg_cv:.1f}% | - | - |")

        lines.append("")

        # Statistical Tests
        lines.append("## Statistical Analysis")
        lines.append("")

        ttest = stats.get('paired_ttest', {})
        ci = stats.get('confidence_interval_95', {})

        lines.append("### Paired t-test")
        lines.append("")
        lines.append(f"- **t-statistic:** {ttest.get('t_statistic', 0):.3f}")
        lines.append(f"- **p-value:** {ttest.get('p_value', 1):.6f}")
        lines.append(f"- **Degrees of freedom:** {ttest.get('degrees_of_freedom', 0)}")
        sig_01 = "Yes" if ttest.get('significant_at_001', False) else "No"
        sig_05 = "Yes" if ttest.get('significant_at_005', False) else "No"
        lines.append(f"- **Significant at α=0.01:** {sig_01}")
        lines.append(f"- **Significant at α=0.05:** {sig_05}")
        lines.append("")

        lines.append("### Effect Size")
        lines.append("")
        lines.append(f"- **Cohen's d:** {cohens_d:.3f} ({effect})")
        lines.append("")
        lines.append("| Cohen's d | Interpretation |")
        lines.append("|-----------|----------------|")
        lines.append("| < 0.2 | Negligible |")
        lines.append("| 0.2 - 0.5 | Small |")
        lines.append("| 0.5 - 0.8 | Medium |")
        lines.append("| > 0.8 | Large |")
        lines.append("")

        lines.append("### Confidence Interval")
        lines.append("")
        lines.append(f"- **95% CI for delta:** [{ci.get('lower', 0):.4f}, {ci.get('upper', 0):.4f}]")
        lines.append("")

        # By Category
        by_category = comparison.get('by_category', {})
        if by_category:
            lines.append("## Results by Category")
            lines.append("")
            lines.append("| Category | Baseline | MDEMG | Delta |")
            lines.append("|----------|:--------:|:-----:|:-----:|")
            for cat, data in sorted(by_category.items()):
                b_mean = data.get('baseline_mean', 0)
                m_mean = data.get('mdemg_mean', 0)
                d = data.get('delta', 0)
                pct = data.get('percent_change', 0)
                lines.append(f"| {cat} | {b_mean:.3f} | {m_mean:.3f} | {d:+.3f} ({pct:+.1f}%) |")
            lines.append("")

        # By Difficulty
        by_difficulty = comparison.get('by_difficulty', {})
        if by_difficulty:
            lines.append("## Results by Difficulty")
            lines.append("")
            lines.append("| Difficulty | Baseline | MDEMG | Delta |")
            lines.append("|------------|:--------:|:-----:|:-----:|")
            for diff in ['easy', 'medium', 'hard']:
                if diff in by_difficulty:
                    data = by_difficulty[diff]
                    b_mean = data.get('baseline_mean', 0)
                    m_mean = data.get('mdemg_mean', 0)
                    d = data.get('delta', 0)
                    pct = data.get('percent_change', 0)
                    lines.append(f"| {diff} | {b_mean:.3f} | {m_mean:.3f} | {d:+.3f} ({pct:+.1f}%) |")
            lines.append("")

        # Per-Run Results
        if baseline_runs or mdemg_runs:
            lines.append("## Per-Run Results")
            lines.append("")
            lines.append("### Baseline Runs")
            lines.append("")
            lines.append("| Run | Mean | Std | Median | N |")
            lines.append("|-----|:----:|:---:|:------:|:-:|")
            for run in baseline_runs:
                lines.append(f"| {run.get('run_id', '?')} | {run.get('mean', 0):.3f} | "
                             f"{run.get('std', 0):.3f} | {run.get('median', 0):.3f} | {run.get('n', 0)} |")
            lines.append("")

            lines.append("### MDEMG Runs")
            lines.append("")
            lines.append("| Run | Mean | Std | Median | N |")
            lines.append("|-----|:----:|:---:|:------:|:-:|")
            for run in mdemg_runs:
                lines.append(f"| {run.get('run_id', '?')} | {run.get('mean', 0):.3f} | "
                             f"{run.get('std', 0):.3f} | {run.get('median', 0):.3f} | {run.get('n', 0)} |")
            lines.append("")

        # Variance Analysis
        variance = comparison.get('variance_analysis', {})
        if variance:
            lines.append("## Variance Analysis")
            lines.append("")
            lines.append("| Metric | Baseline | MDEMG |")
            lines.append("|--------|:--------:|:-----:|")
            lines.append(f"| CV (%) | {variance.get('baseline_cv_pct', 0):.1f} | {variance.get('mdemg_cv_pct', 0):.1f} |")
            lines.append(f"| Pooled Std | {variance.get('baseline_pooled_std', 0):.3f} | {variance.get('mdemg_pooled_std', 0):.3f} |")
            lines.append("")
            stability = variance.get('run_stability', 'unknown')
            if stability == 'high':
                lines.append("**Run Stability:** High - Results are highly reproducible (CV < 5%)")
            elif stability == 'moderate':
                lines.append("**Run Stability:** Moderate - Results show acceptable variance (CV < 15%)")
            else:
                lines.append("**Run Stability:** Low - Results show high variance (CV >= 15%)")
            lines.append("")

        # Interpretation Guide
        lines.append("## Interpretation Guide")
        lines.append("")
        lines.append("### Significance Levels")
        lines.append("")
        lines.append("| Symbol | Meaning |")
        lines.append("|--------|---------|")
        lines.append("| *** | p < 0.001 (highly significant) |")
        lines.append("| ** | p < 0.01 (very significant) |")
        lines.append("| * | p < 0.05 (significant) |")
        lines.append("| (none) | p >= 0.05 (not significant) |")
        lines.append("")

        lines.append("### Coefficient of Variation (CV)")
        lines.append("")
        lines.append("| CV Range | Interpretation |")
        lines.append("|----------|----------------|")
        lines.append("| < 10% | Low variance, stable |")
        lines.append("| 10-25% | Moderate variance, acceptable |")
        lines.append("| > 25% | High variance, investigate |")
        lines.append("")

        # Footer
        lines.append("---")
        lines.append("")
        lines.append("*Generated by MDEMG Benchmark Framework v1.0*")

        return "\n".join(lines)

    def generate_quick_summary(self, comparison: Dict) -> str:
        """Generate a short text summary for console output.

        Args:
            comparison: Dictionary with comparison results

        Returns:
            Short summary string
        """
        baseline = comparison.get('baseline', {})
        mdemg = comparison.get('mdemg', {})
        delta = comparison.get('delta', {})
        stats = comparison.get('statistical_tests', {})

        baseline_mean = baseline.get('mean_of_means', 0)
        mdemg_mean = mdemg.get('mean_of_means', 0)
        mean_delta = delta.get('mean_delta', 0)
        pct_change = delta.get('percent_change', 0)
        p_value = stats.get('paired_ttest', {}).get('p_value', 1)
        effect = stats.get('effect_size', 'unknown')
        cohens_d = stats.get('cohens_d', 0)

        stars = self._significance_stars(p_value)

        return (f"Baseline: {baseline_mean:.3f} | MDEMG: {mdemg_mean:.3f} | "
                f"Delta: {mean_delta:+.3f} ({pct_change:+.1f}%) | "
                f"p={p_value:.4f}{stars} | Cohen's d={cohens_d:.2f} ({effect})")


def main():
    """CLI entry point."""
    if len(sys.argv) < 3:
        print("Usage: python report_generator.py comparison.json report.md [config.json]")
        sys.exit(1)

    comparison_path = Path(sys.argv[1])
    output_path = Path(sys.argv[2])
    config_path = Path(sys.argv[3]) if len(sys.argv) > 3 else None

    # Load comparison
    with open(comparison_path) as f:
        comparison = json.load(f)

    # Load config if provided
    config = None
    if config_path and config_path.exists():
        with open(config_path) as f:
            config_data = json.load(f)
            config = BenchmarkConfig(
                endpoint=config_data.get('endpoint', ''),
                space_id=config_data.get('space_id', ''),
                questions_file=config_data.get('questions_file', ''),
                question_count=config_data.get('question_count', 0),
                runs_per_mode=config_data.get('runs_per_mode', 3),
                created_at=config_data.get('created_at', '')
            )

    # Generate report
    generator = ReportGenerator()
    report = generator.generate(comparison, config)

    # Write output
    with open(output_path, 'w') as f:
        f.write(report)

    print(f"Report generated: {output_path}")
    print()
    print(generator.generate_quick_summary(comparison))


if __name__ == '__main__':
    main()
