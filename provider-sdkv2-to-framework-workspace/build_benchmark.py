#!/usr/bin/env python3
"""Build benchmark.json from grading.json + timing.json files.

Paths are derived from `git rev-parse --show-toplevel` so the script works
from any clone location. Override the iteration with --iteration N (default 1).
"""
from __future__ import annotations

import argparse
import json
import statistics
import subprocess
from datetime import datetime, timezone
from pathlib import Path


def _repo_root() -> Path:
    here = Path(__file__).resolve().parent
    r = subprocess.run(
        ["git", "-C", str(here), "rev-parse", "--show-toplevel"],
        capture_output=True, text=True, timeout=10,
    )
    if r.returncode != 0:
        raise RuntimeError(f"not a git repo: {here} — {r.stderr.strip()}")
    return Path(r.stdout.strip())


REPO_ROOT = _repo_root()
EVALS_JSON = REPO_ROOT / "provider-sdkv2-to-framework" / "evals" / "evals.json"
WORKSPACE_BASE = REPO_ROOT / "provider-sdkv2-to-framework-workspace"
SKILL_PATH = REPO_ROOT / "provider-sdkv2-to-framework"

# Default iteration; override via --iteration. The original hardcoded value was 1.
WORKSPACE = WORKSPACE_BASE / "iteration-1"


def main():
    global WORKSPACE
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--iteration", type=int, default=1,
                        help="iteration-N directory under sdkv2-to-framework-workspace (default 1)")
    args = parser.parse_args()
    WORKSPACE = WORKSPACE_BASE / f"iteration-{args.iteration}"

    evals = json.loads(EVALS_JSON.read_text())["evals"]
    runs = []

    for ev in evals:
        for config in ("with_skill", "without_skill"):
            run_dir = WORKSPACE / ev["name"] / config
            grading = json.loads((run_dir / "grading.json").read_text())
            timing = json.loads((run_dir / "timing.json").read_text())
            runs.append({
                "eval_id": ev["id"],
                "eval_name": ev["name"],
                "configuration": config,
                "run_number": 1,
                "result": {
                    "pass_rate": grading["summary"]["pass_rate"],
                    "passed": grading["summary"]["passed"],
                    "failed": grading["summary"]["failed"],
                    "total": grading["summary"]["total"],
                    "time_seconds": timing["total_duration_seconds"],
                    "tokens": timing["total_tokens"],
                    "tool_calls": 0,
                    "errors": 0,
                },
                "expectations": grading["expectations"],
                "notes": [],
            })

    # Order: with_skill before its baseline (per skill-creator instruction).
    runs.sort(key=lambda r: (r["eval_id"], 0 if r["configuration"] == "with_skill" else 1))

    def stats(values):
        if not values:
            return {"mean": 0, "stddev": 0, "min": 0, "max": 0}
        return {
            "mean": statistics.mean(values),
            "stddev": statistics.stdev(values) if len(values) > 1 else 0,
            "min": min(values),
            "max": max(values),
        }

    with_skill_runs = [r for r in runs if r["configuration"] == "with_skill"]
    baseline_runs = [r for r in runs if r["configuration"] == "without_skill"]

    summary = {
        "with_skill": {
            "pass_rate": stats([r["result"]["pass_rate"] for r in with_skill_runs]),
            "time_seconds": stats([r["result"]["time_seconds"] for r in with_skill_runs]),
            "tokens": stats([r["result"]["tokens"] for r in with_skill_runs]),
        },
        "without_skill": {
            "pass_rate": stats([r["result"]["pass_rate"] for r in baseline_runs]),
            "time_seconds": stats([r["result"]["time_seconds"] for r in baseline_runs]),
            "tokens": stats([r["result"]["tokens"] for r in baseline_runs]),
        },
    }

    def fmt_delta(a, b, suffix=""):
        d = a - b
        sign = "+" if d >= 0 else ""
        return f"{sign}{d:.2f}{suffix}"

    summary["delta"] = {
        "pass_rate": fmt_delta(summary["with_skill"]["pass_rate"]["mean"], summary["without_skill"]["pass_rate"]["mean"]),
        "time_seconds": fmt_delta(summary["with_skill"]["time_seconds"]["mean"], summary["without_skill"]["time_seconds"]["mean"], "s"),
        "tokens": f"{int(summary['with_skill']['tokens']['mean'] - summary['without_skill']['tokens']['mean']):+d}",
    }

    # Analyst notes — observations the aggregate stats might hide.
    notes = []
    for ev in evals:
        ws = next(r for r in runs if r["eval_id"] == ev["id"] and r["configuration"] == "with_skill")
        bs = next(r for r in runs if r["eval_id"] == ev["id"] and r["configuration"] == "without_skill")
        delta = ws["result"]["pass_rate"] - bs["result"]["pass_rate"]
        if delta == 0 and ws["result"]["pass_rate"] >= 0.95:
            notes.append(
                f"Eval {ev['id']} ({ev['name']}): both configurations pass at {ws['result']['pass_rate']:.0%} — assertions may not differentiate the skill on this case."
            )
        elif delta > 0.3:
            notes.append(
                f"Eval {ev['id']} ({ev['name']}): with_skill {ws['result']['pass_rate']:.0%} vs baseline {bs['result']['pass_rate']:.0%} — large skill lift (+{delta:.0%})."
            )
        elif delta < 0:
            notes.append(
                f"Eval {ev['id']} ({ev['name']}): baseline outperformed with_skill ({bs['result']['pass_rate']:.0%} vs {ws['result']['pass_rate']:.0%}) — investigate."
            )

    avg_token_overhead = summary["delta"]["tokens"]
    avg_time_overhead = summary["delta"]["time_seconds"]
    notes.append(
        f"Skill adds {avg_time_overhead} wall-clock and {avg_token_overhead} tokens on average compared to baseline."
    )
    notes.append(
        "Compile-pass assertions are graded by reading the agent's notes.md (where it claims compile passed in a temp clone). Re-running compile checks is left to manual verification in the viewer."
    )
    notes.append(
        "Baseline for eval-1 (inventory) introduced terraform-plugin-mux in its Phase 0 — exactly the path the user excluded. The skill version stayed strictly single-release. This is the strongest qualitative differentiator in iteration-1."
    )

    benchmark = {
        "metadata": {
            "skill_name": "provider-sdkv2-to-framework",
            "skill_path": str(SKILL_PATH),
            "executor_model": "claude-sonnet-4-6",  # what we used
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "evals_run": [e["id"] for e in evals],
            "runs_per_configuration": 1,
        },
        "runs": runs,
        "run_summary": summary,
        "notes": notes,
    }

    out = WORKSPACE / "benchmark.json"
    out.write_text(json.dumps(benchmark, indent=2))
    print(f"Wrote {out}")
    print()
    print("Summary:")
    print(f"  with_skill pass_rate: {summary['with_skill']['pass_rate']['mean']:.0%}")
    print(f"  baseline pass_rate:   {summary['without_skill']['pass_rate']['mean']:.0%}")
    print(f"  delta:                {summary['delta']['pass_rate']}")
    print(f"  time delta:           {summary['delta']['time_seconds']}")
    print(f"  token delta:          {summary['delta']['tokens']}")


if __name__ == "__main__":
    main()
