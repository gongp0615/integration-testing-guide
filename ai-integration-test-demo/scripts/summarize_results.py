#!/usr/bin/env python3
"""Summarize DSMB-Agent test run logs with metrics against ground truth."""

import argparse
import json
import os
import re
import sys
from collections import defaultdict
from pathlib import Path


def load_ground_truth(script_dir: str) -> dict:
    gt_path = os.path.join(script_dir, "ground_truth.json")
    with open(gt_path, encoding="utf-8") as f:
        return json.load(f)


def extract_json_report(content: str) -> dict | None:
    """Try to extract a JSON report block from agent output."""
    # Look for JSON blocks containing "correlations" and "bugs"
    matches = re.finditer(r'\{[^{}]*"correlations"[^{}]*\}', content, re.DOTALL)
    for m in matches:
        try:
            return json.loads(m.group())
        except json.JSONDecodeError:
            continue
    # Try larger blocks
    matches = re.finditer(r'\{[\s\S]*?"correlations"[\s\S]*?"bugs"[\s\S]*?\}', content)
    for m in matches:
        try:
            return json.loads(m.group())
        except json.JSONDecodeError:
            continue
    return None


def regex_parse_log(content: str) -> dict:
    """Fallback regex-based parsing."""
    result = {"correlations": [], "bugs": [], "iterations": 0}
    result["iterations"] = len(re.findall(r"AI →", content))
    for m in re.finditer(r"(?:Bug|bug)\s*#?\s*(\d+).*?(Critical|High|Medium|Low)", content, re.IGNORECASE | re.DOTALL):
        result["bugs"].append({"id": f"B{m.group(1)}", "severity": m.group(2).capitalize()})
    for m in re.finditer(r"R(\d+)\s*[:→]", content):
        rid = f"R{m.group(1)}"
        if rid not in [c.get("id") for c in result["correlations"]]:
            result["correlations"].append({"id": rid})
    return result


def parse_log(filepath: str) -> dict:
    content = Path(filepath).read_text(encoding="utf-8", errors="replace")
    report = extract_json_report(content)
    if report:
        report["iterations"] = report.get("iterations", len(re.findall(r"AI →", content)))
        report["_source"] = "json"
        return report
    result = regex_parse_log(content)
    result["_source"] = "regex"
    return result


def evaluate(results: list[dict], gt: dict) -> None:
    total_runs = len(results)
    if total_runs == 0:
        print("No results to evaluate.")
        return

    gt_corr_ids = {c["id"] for c in gt["correlations"]}
    gt_bug_ids = {b["id"] for b in gt["bugs"]}
    gt_bug_levels = {b["id"]: b["level"] for b in gt["bugs"]}
    gt_bug_modules = {b["id"]: b["module"] for b in gt["bugs"]}
    level_names = ["L1", "L2", "L3", "L4"]

    # Aggregate
    corr_tp = 0
    corr_fp = 0
    corr_total_reported = 0
    bug_tp = 0
    bug_fp = 0
    bug_total_reported = 0
    level_found = defaultdict(lambda: defaultdict(int))  # level -> run -> set of bug ids found
    total_iterations = 0

    for run_idx, r in enumerate(results):
        total_iterations += r.get("iterations", 0)

        # Correlation metrics
        reported_corr = set()
        counted_corr = set()
        for c in r.get("correlations", []):
            cid = c.get("id", "")
            if not cid:
                continue
            reported_corr.add(cid)
            if cid not in counted_corr:
                counted_corr.add(cid)
                if cid in gt_corr_ids:
                    corr_tp += 1
                else:
                    corr_fp += 1
        corr_total_reported += len(reported_corr)

        # Bug metrics
        reported_bugs = set()
        counted_bugs = set()
        for b in r.get("bugs", []):
            bid = b.get("id", "")
            if not bid:
                # Try to match by description
                desc = b.get("description", "").lower()
                for gt_bug in gt["bugs"]:
                    if gt_bug["module"] in desc or any(kw in desc for kw in ["negative", "repeat", "collector", "claimreward", "consume", "subscriber", "conflict"]):
                        bid = gt_bug["id"]
                        break
            if not bid:
                continue
            reported_bugs.add(bid)
            if bid not in counted_bugs:
                counted_bugs.add(bid)
                if bid in gt_bug_ids:
                    bug_tp += 1
                    level = gt_bug_levels.get(bid, "L1")
                    level_found[level][run_idx] = level_found[level].get(run_idx, set())
                    level_found[level][run_idx].add(bid)
                else:
                    bug_fp += 1
        bug_total_reported += len(reported_bugs)

    # Calculate metrics
    corr_precision = corr_tp / corr_total_reported if corr_total_reported > 0 else 0
    corr_recall = corr_tp / (len(gt_corr_ids) * total_runs) if total_runs > 0 else 0
    bug_precision = bug_tp / bug_total_reported if bug_total_reported > 0 else 0
    bug_recall = bug_tp / (len(gt_bug_ids) * total_runs) if total_runs > 0 else 0
    bug_f1 = 2 * bug_precision * bug_recall / (bug_precision + bug_recall) if (bug_precision + bug_recall) > 0 else 0
    fpr = bug_fp / bug_total_reported if bug_total_reported > 0 else 0
    avg_iterations = total_iterations / total_runs if total_runs > 0 else 0
    efficiency = (corr_tp + bug_tp) / total_iterations if total_iterations > 0 else 0

    print(f"\n{'=' * 60}")
    print(f"  DSMB-Agent Evaluation Report ({total_runs} runs)")
    print(f"{'=' * 60}\n")

    print("## Overall Metrics")
    print(f"  Correlation Precision: {corr_precision:.1%} ({corr_tp}/{corr_total_reported})")
    print(f"  Correlation Recall:    {corr_recall:.1%} ({corr_tp}/{len(gt_corr_ids) * total_runs})")
    print(f"  Bug Precision:         {bug_precision:.1%} ({bug_tp}/{bug_total_reported})")
    print(f"  Bug Recall:            {bug_recall:.1%} ({bug_tp}/{len(gt_bug_ids) * total_runs})")
    print(f"  Bug F1:                {bug_f1:.3f}")
    print(f"  False Positive Rate:   {fpr:.1%}")
    print(f"  Avg Iterations:        {avg_iterations:.1f}")
    print(f"  Exploration Efficiency: {efficiency:.3f} findings/iteration")

    print(f"\n## Bug Level Discovery")
    for level in level_names:
        gt_count = sum(1 for b in gt["bugs"] if b["level"] == level)
        found_runs = sum(1 for run_idx in level_found.get(level, {}))
        print(f"  {level}: {found_runs}/{total_runs} runs found at least one {level} bug ({gt_count} bugs total)")

    print(f"\n## Per-Run Details")
    for i, r in enumerate(results):
        src = r.get("_source", "?")
        iters = r.get("iterations", 0)
        corrs = len(r.get("correlations", []))
        bugs = len(r.get("bugs", []))
        print(f"  Run {i+1}: {iters} iterations, {corrs} correlations, {bugs} bugs (parsed via {src})")

    print(f"\n{'=' * 60}\n")


def main():
    parser = argparse.ArgumentParser(description="Evaluate DSMB-Agent test results")
    parser.add_argument("results_dir", help="Directory containing .log files")
    parser.add_argument("--pattern", default="*.log", help="File glob (default: *.log)")
    args = parser.parse_args()

    results_dir = Path(args.results_dir)
    if not results_dir.exists():
        print(f"Error: directory '{args.results_dir}' not found")
        sys.exit(1)

    script_dir = os.path.dirname(os.path.abspath(__file__))
    gt = load_ground_truth(script_dir)

    log_files = sorted(results_dir.glob(args.pattern))
    if not log_files:
        print(f"No files matching '{args.pattern}' in '{args.results_dir}'")
        sys.exit(1)

    print(f"Found {len(log_files)} log files. Ground truth: {len(gt['correlations'])} correlations, {len(gt['bugs'])} bugs.")

    results = [parse_log(str(f)) for f in log_files]
    evaluate(results, gt)


if __name__ == "__main__":
    main()
