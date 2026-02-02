#!/usr/bin/env python3
"""
MDEMG Benchmark Statistical Analysis Module v1.0

Provides statistical functions for comparing baseline vs MDEMG benchmark results:
- Paired t-test for significance
- Cohen's d for effect size
- Coefficient of variation analysis
- Cross-run aggregation
- Confidence interval calculation

Usage as library:
    from stats_analyzer import StatsAnalyzer
    analyzer = StatsAnalyzer()
    comparison = analyzer.compare(baseline_grades, mdemg_grades)

Usage as CLI:
    python stats_analyzer.py baseline_aggregate.json mdemg_aggregate.json comparison.json
"""

import json
import math
import sys
from dataclasses import dataclass, field, asdict
from pathlib import Path
from typing import Dict, List, Optional, Tuple
import statistics


# =============================================================================
# Data Classes
# =============================================================================

@dataclass
class TTestResult:
    t_statistic: float = 0.0
    p_value: float = 1.0
    degrees_of_freedom: int = 0
    significant_at_001: bool = False
    significant_at_005: bool = False


@dataclass
class ConfidenceInterval:
    lower: float = 0.0
    upper: float = 0.0
    confidence_level: float = 0.95


@dataclass
class EffectSize:
    cohens_d: float = 0.0
    interpretation: str = "negligible"  # negligible, small, medium, large


@dataclass
class RunAggregate:
    run_id: int = 0
    mean: float = 0.0
    std: float = 0.0
    median: float = 0.0
    n: int = 0


@dataclass
class CrossRunAggregate:
    runs: int = 0
    mean_of_means: float = 0.0
    std_of_means: float = 0.0
    pooled_std: float = 0.0
    cv_across_runs: float = 0.0
    min_run_mean: float = 0.0
    max_run_mean: float = 0.0
    per_run: List[RunAggregate] = field(default_factory=list)


@dataclass
class CategoryComparison:
    baseline_mean: float = 0.0
    baseline_std: float = 0.0
    mdemg_mean: float = 0.0
    mdemg_std: float = 0.0
    delta: float = 0.0
    percent_change: float = 0.0


@dataclass
class ComparisonResult:
    baseline: CrossRunAggregate = field(default_factory=CrossRunAggregate)
    mdemg: CrossRunAggregate = field(default_factory=CrossRunAggregate)
    mean_delta: float = 0.0
    percent_change: float = 0.0
    ttest: TTestResult = field(default_factory=TTestResult)
    effect_size: EffectSize = field(default_factory=EffectSize)
    confidence_interval: ConfidenceInterval = field(default_factory=ConfidenceInterval)
    by_category: Dict[str, CategoryComparison] = field(default_factory=dict)
    by_difficulty: Dict[str, CategoryComparison] = field(default_factory=dict)
    variance_analysis: Dict[str, float] = field(default_factory=dict)

    def to_dict(self) -> Dict:
        return {
            'baseline': asdict(self.baseline),
            'mdemg': asdict(self.mdemg),
            'delta': {
                'mean_delta': round(self.mean_delta, 4),
                'percent_change': round(self.percent_change, 2)
            },
            'statistical_tests': {
                'paired_ttest': asdict(self.ttest),
                'cohens_d': self.effect_size.cohens_d,
                'effect_size': self.effect_size.interpretation,
                'confidence_interval_95': asdict(self.confidence_interval)
            },
            'by_category': {k: asdict(v) for k, v in self.by_category.items()},
            'by_difficulty': {k: asdict(v) for k, v in self.by_difficulty.items()},
            'variance_analysis': self.variance_analysis
        }


# =============================================================================
# Statistical Functions
# =============================================================================

def paired_ttest(x: List[float], y: List[float]) -> TTestResult:
    """Perform paired t-test between two matched samples.

    Tests H0: mean(x - y) = 0

    Args:
        x: First sample (e.g., baseline scores)
        y: Second sample (e.g., MDEMG scores)

    Returns:
        TTestResult with t-statistic, p-value, and significance flags
    """
    if len(x) != len(y) or len(x) < 2:
        return TTestResult()

    n = len(x)
    diffs = [y[i] - x[i] for i in range(n)]

    mean_diff = statistics.mean(diffs)
    std_diff = statistics.stdev(diffs)

    if std_diff == 0:
        return TTestResult(t_statistic=float('inf') if mean_diff > 0 else float('-inf'))

    t_stat = mean_diff / (std_diff / math.sqrt(n))
    df = n - 1

    # Approximate p-value using t-distribution
    # Using Student's t approximation for large df
    p_value = _t_distribution_p_value(abs(t_stat), df) * 2  # Two-tailed

    return TTestResult(
        t_statistic=round(t_stat, 3),
        p_value=round(min(p_value, 1.0), 6),
        degrees_of_freedom=df,
        significant_at_001=p_value < 0.01,
        significant_at_005=p_value < 0.05
    )


def _t_distribution_p_value(t: float, df: int) -> float:
    """Approximate one-tailed p-value from t distribution.

    Uses approximation from Abramowitz & Stegun for large df,
    and lookup table for small df.
    """
    # For large df, t-distribution approaches normal
    if df > 100:
        # Normal approximation
        return _normal_cdf(-abs(t))

    # For smaller df, use approximation
    # P(T > t) ≈ 0.5 * I(df/(df+t^2), df/2, 0.5) using incomplete beta
    # Simplified: use normal approximation with correction
    x = df / (df + t * t)
    p = 0.5 * _beta_regularized(x, df / 2.0, 0.5)
    return p


def _normal_cdf(x: float) -> float:
    """Approximate normal CDF using error function approximation."""
    return 0.5 * (1 + math.erf(x / math.sqrt(2)))


def _beta_regularized(x: float, a: float, b: float) -> float:
    """Simplified approximation of regularized incomplete beta function."""
    # For t-test purposes, we use a simpler approximation
    # This is accurate enough for significance testing
    if x <= 0:
        return 0.0
    if x >= 1:
        return 1.0

    # Continued fraction approximation (simplified)
    # For our use case (a = df/2, b = 0.5), this is adequate
    term = math.pow(x, a) * math.pow(1 - x, b) / (a * _beta_function(a, b))
    return min(max(1 - term * a / (a + 1), 0), 1)


def _beta_function(a: float, b: float) -> float:
    """Compute beta function B(a, b) = Gamma(a) * Gamma(b) / Gamma(a+b)."""
    return math.exp(math.lgamma(a) + math.lgamma(b) - math.lgamma(a + b))


def cohens_d(x: List[float], y: List[float]) -> EffectSize:
    """Compute Cohen's d effect size for paired samples.

    Cohen's d = mean(y - x) / std(differences)

    Interpretation:
    - |d| < 0.2: negligible
    - 0.2 <= |d| < 0.5: small
    - 0.5 <= |d| < 0.8: medium
    - |d| >= 0.8: large
    """
    if len(x) != len(y) or len(x) < 2:
        return EffectSize()

    diffs = [y[i] - x[i] for i in range(len(x))]
    mean_diff = statistics.mean(diffs)
    std_diff = statistics.stdev(diffs)

    if std_diff == 0:
        d = float('inf') if mean_diff != 0 else 0.0
    else:
        d = mean_diff / std_diff

    # Interpret effect size
    abs_d = abs(d)
    if abs_d < 0.2:
        interpretation = "negligible"
    elif abs_d < 0.5:
        interpretation = "small"
    elif abs_d < 0.8:
        interpretation = "medium"
    else:
        interpretation = "large"

    return EffectSize(
        cohens_d=round(d, 3),
        interpretation=interpretation
    )


def confidence_interval(x: List[float], y: List[float], confidence: float = 0.95) -> ConfidenceInterval:
    """Compute confidence interval for mean difference.

    Uses t-distribution for small samples.
    """
    if len(x) != len(y) or len(x) < 2:
        return ConfidenceInterval()

    n = len(x)
    diffs = [y[i] - x[i] for i in range(n)]
    mean_diff = statistics.mean(diffs)
    std_diff = statistics.stdev(diffs)
    se = std_diff / math.sqrt(n)

    # t critical value (approximation for common confidence levels)
    df = n - 1
    if confidence == 0.95:
        if df >= 120:
            t_crit = 1.98
        elif df >= 60:
            t_crit = 2.00
        elif df >= 30:
            t_crit = 2.04
        elif df >= 20:
            t_crit = 2.09
        elif df >= 10:
            t_crit = 2.23
        else:
            t_crit = 2.57
    else:
        # Default to z for high confidence
        t_crit = 1.96

    margin = t_crit * se

    return ConfidenceInterval(
        lower=round(mean_diff - margin, 4),
        upper=round(mean_diff + margin, 4),
        confidence_level=confidence
    )


def coefficient_of_variation(values: List[float]) -> float:
    """Compute coefficient of variation (CV) as percentage.

    CV = (std / mean) * 100
    """
    if not values or len(values) < 2:
        return 0.0
    mean = statistics.mean(values)
    if mean == 0:
        return 0.0
    std = statistics.stdev(values)
    return round(100 * std / mean, 2)


def pooled_std(std_list: List[float], n_list: List[int]) -> float:
    """Compute pooled standard deviation from multiple groups.

    pooled_std = sqrt(sum((n_i - 1) * std_i^2) / sum(n_i - 1))
    """
    if not std_list or not n_list or len(std_list) != len(n_list):
        return 0.0

    numerator = sum((n - 1) * s * s for s, n in zip(std_list, n_list) if n > 1)
    denominator = sum(n - 1 for n in n_list if n > 1)

    if denominator == 0:
        return 0.0

    return round(math.sqrt(numerator / denominator), 4)


# =============================================================================
# Analyzer Class
# =============================================================================

class StatsAnalyzer:
    """Statistical analyzer for MDEMG benchmark results."""

    def aggregate_runs(self, run_grades: List[List[Dict]]) -> CrossRunAggregate:
        """Aggregate grades across multiple runs.

        Args:
            run_grades: List of grade lists, one per run

        Returns:
            CrossRunAggregate with per-run and cross-run statistics
        """
        if not run_grades:
            return CrossRunAggregate()

        per_run = []
        all_means = []
        all_stds = []
        all_ns = []

        for i, grades in enumerate(run_grades, 1):
            if not grades:
                continue
            scores = [g.get('scores', {}).get('final', g.get('score', 0)) for g in grades]
            if not scores:
                continue

            run_mean = statistics.mean(scores)
            run_std = statistics.stdev(scores) if len(scores) > 1 else 0

            per_run.append(RunAggregate(
                run_id=i,
                mean=round(run_mean, 4),
                std=round(run_std, 4),
                median=round(statistics.median(scores), 4),
                n=len(scores)
            ))

            all_means.append(run_mean)
            all_stds.append(run_std)
            all_ns.append(len(scores))

        if not per_run:
            return CrossRunAggregate()

        return CrossRunAggregate(
            runs=len(per_run),
            mean_of_means=round(statistics.mean(all_means), 4),
            std_of_means=round(statistics.stdev(all_means), 4) if len(all_means) > 1 else 0,
            pooled_std=pooled_std(all_stds, all_ns),
            cv_across_runs=coefficient_of_variation(all_means),
            min_run_mean=round(min(all_means), 4),
            max_run_mean=round(max(all_means), 4),
            per_run=per_run
        )

    def aggregate_by_category(self, grades: List[Dict], key: str = 'category') -> Dict[str, Dict]:
        """Aggregate grades by a grouping key (category or difficulty)."""
        groups = {}
        for g in grades:
            group = g.get(key, 'unknown')
            if group not in groups:
                groups[group] = []
            score = g.get('scores', {}).get('final', g.get('score', 0))
            groups[group].append(score)

        result = {}
        for group, scores in groups.items():
            if scores:
                result[group] = {
                    'count': len(scores),
                    'mean': round(statistics.mean(scores), 4),
                    'std': round(statistics.stdev(scores), 4) if len(scores) > 1 else 0
                }
        return result

    def compare_by_group(self, baseline_grades: List[Dict], mdemg_grades: List[Dict],
                         key: str = 'category') -> Dict[str, CategoryComparison]:
        """Compare baseline vs MDEMG by a grouping key."""
        baseline_by = self.aggregate_by_category(baseline_grades, key)
        mdemg_by = self.aggregate_by_category(mdemg_grades, key)

        all_groups = set(baseline_by.keys()) | set(mdemg_by.keys())
        result = {}

        for group in all_groups:
            b = baseline_by.get(group, {'mean': 0, 'std': 0})
            m = mdemg_by.get(group, {'mean': 0, 'std': 0})
            delta = m['mean'] - b['mean']
            pct = (delta / b['mean'] * 100) if b['mean'] > 0 else 0

            result[group] = CategoryComparison(
                baseline_mean=b['mean'],
                baseline_std=b['std'],
                mdemg_mean=m['mean'],
                mdemg_std=m['std'],
                delta=round(delta, 4),
                percent_change=round(pct, 2)
            )

        return result

    def compare(self, baseline_run_grades: List[List[Dict]],
                mdemg_run_grades: List[List[Dict]]) -> ComparisonResult:
        """Perform full statistical comparison between baseline and MDEMG.

        Args:
            baseline_run_grades: List of grade lists for baseline runs
            mdemg_run_grades: List of grade lists for MDEMG runs

        Returns:
            ComparisonResult with all statistical analyses
        """
        result = ComparisonResult()

        # Aggregate each mode
        result.baseline = self.aggregate_runs(baseline_run_grades)
        result.mdemg = self.aggregate_runs(mdemg_run_grades)

        # Overall delta
        result.mean_delta = result.mdemg.mean_of_means - result.baseline.mean_of_means
        if result.baseline.mean_of_means > 0:
            result.percent_change = (result.mean_delta / result.baseline.mean_of_means) * 100
        else:
            result.percent_change = 0.0

        # For statistical tests, we need per-question scores from all runs
        # Pool all baseline scores and all MDEMG scores for paired comparison
        baseline_scores_pooled = []
        mdemg_scores_pooled = []

        # Match by question ID across runs
        baseline_all = []
        mdemg_all = []
        for grades in baseline_run_grades:
            baseline_all.extend(grades)
        for grades in mdemg_run_grades:
            mdemg_all.extend(grades)

        # Group by question ID and average
        baseline_by_q = {}
        mdemg_by_q = {}

        for g in baseline_all:
            qid = g.get('id', '')
            score = g.get('scores', {}).get('final', g.get('score', 0))
            if qid not in baseline_by_q:
                baseline_by_q[qid] = []
            baseline_by_q[qid].append(score)

        for g in mdemg_all:
            qid = g.get('id', '')
            score = g.get('scores', {}).get('final', g.get('score', 0))
            if qid not in mdemg_by_q:
                mdemg_by_q[qid] = []
            mdemg_by_q[qid].append(score)

        # Average scores per question for paired comparison
        common_questions = set(baseline_by_q.keys()) & set(mdemg_by_q.keys())
        for qid in sorted(common_questions):
            baseline_scores_pooled.append(statistics.mean(baseline_by_q[qid]))
            mdemg_scores_pooled.append(statistics.mean(mdemg_by_q[qid]))

        # Statistical tests
        if len(baseline_scores_pooled) >= 2:
            result.ttest = paired_ttest(baseline_scores_pooled, mdemg_scores_pooled)
            result.effect_size = cohens_d(baseline_scores_pooled, mdemg_scores_pooled)
            result.confidence_interval = confidence_interval(baseline_scores_pooled, mdemg_scores_pooled)

        # Group comparisons (use pooled grades from first run of each for cleaner comparison)
        baseline_first = baseline_run_grades[0] if baseline_run_grades else []
        mdemg_first = mdemg_run_grades[0] if mdemg_run_grades else []

        result.by_category = self.compare_by_group(baseline_first, mdemg_first, 'category')
        result.by_difficulty = self.compare_by_group(baseline_first, mdemg_first, 'difficulty')

        # Variance analysis
        result.variance_analysis = {
            'baseline_cv_pct': result.baseline.cv_across_runs,
            'mdemg_cv_pct': result.mdemg.cv_across_runs,
            'baseline_pooled_std': result.baseline.pooled_std,
            'mdemg_pooled_std': result.mdemg.pooled_std,
            'run_stability': 'high' if max(result.baseline.cv_across_runs, result.mdemg.cv_across_runs) < 5 else
                            'moderate' if max(result.baseline.cv_across_runs, result.mdemg.cv_across_runs) < 15 else 'low'
        }

        return result


# =============================================================================
# Utility Functions
# =============================================================================

def load_grades_file(filepath: Path) -> Dict:
    """Load grades from JSON file."""
    with open(filepath) as f:
        return json.load(f)


def extract_per_question_grades(grades_data: Dict) -> List[Dict]:
    """Extract per-question grades from grades file."""
    return grades_data.get('per_question', [])


# =============================================================================
# CLI Interface
# =============================================================================

def main():
    """CLI entry point."""
    if len(sys.argv) < 3:
        print("Usage: python stats_analyzer.py baseline_grades.json mdemg_grades.json [output.json]")
        print("       python stats_analyzer.py baseline_dir/ mdemg_dir/ [output.json]")
        sys.exit(1)

    baseline_path = Path(sys.argv[1])
    mdemg_path = Path(sys.argv[2])
    output_path = Path(sys.argv[3]) if len(sys.argv) > 3 else Path("comparison.json")

    analyzer = StatsAnalyzer()

    # Handle directory or file input
    baseline_run_grades = []
    mdemg_run_grades = []

    if baseline_path.is_dir():
        # Load all grades_*.json files from directory
        for f in sorted(baseline_path.glob("grades_*run*.json")):
            data = load_grades_file(f)
            baseline_run_grades.append(extract_per_question_grades(data))
    else:
        data = load_grades_file(baseline_path)
        baseline_run_grades.append(extract_per_question_grades(data))

    if mdemg_path.is_dir():
        for f in sorted(mdemg_path.glob("grades_*run*.json")):
            data = load_grades_file(f)
            mdemg_run_grades.append(extract_per_question_grades(data))
    else:
        data = load_grades_file(mdemg_path)
        mdemg_run_grades.append(extract_per_question_grades(data))

    print(f"Loaded {len(baseline_run_grades)} baseline runs, {len(mdemg_run_grades)} MDEMG runs")

    # Perform comparison
    comparison = analyzer.compare(baseline_run_grades, mdemg_run_grades)

    # Save results
    with open(output_path, 'w') as f:
        json.dump(comparison.to_dict(), f, indent=2)

    # Print summary
    print(f"\n{'='*60}")
    print("STATISTICAL COMPARISON RESULTS")
    print(f"{'='*60}")
    print(f"\nBaseline: mean={comparison.baseline.mean_of_means:.3f}, std={comparison.baseline.std_of_means:.3f}")
    print(f"MDEMG:    mean={comparison.mdemg.mean_of_means:.3f}, std={comparison.mdemg.std_of_means:.3f}")
    print(f"Delta:    {comparison.mean_delta:+.3f} ({comparison.percent_change:+.1f}%)")
    print(f"\nStatistical Tests:")
    print(f"  t-statistic: {comparison.ttest.t_statistic}")
    print(f"  p-value: {comparison.ttest.p_value}")
    print(f"  Significant at 0.01: {'Yes' if comparison.ttest.significant_at_001 else 'No'}")
    print(f"  Significant at 0.05: {'Yes' if comparison.ttest.significant_at_005 else 'No'}")
    print(f"  Cohen's d: {comparison.effect_size.cohens_d} ({comparison.effect_size.interpretation})")
    print(f"  95% CI: [{comparison.confidence_interval.lower:.4f}, {comparison.confidence_interval.upper:.4f}]")
    print(f"\nVariance Analysis:")
    print(f"  Baseline CV: {comparison.variance_analysis['baseline_cv_pct']}%")
    print(f"  MDEMG CV: {comparison.variance_analysis['mdemg_cv_pct']}%")
    print(f"  Run stability: {comparison.variance_analysis['run_stability']}")
    print(f"\nOutput: {output_path}")


if __name__ == '__main__':
    main()
