#!/usr/bin/env python3
"""Grader for iteration-2 (minimal-references variant). Same logic as grade.py
but only the with_skill arm; baseline data is reused from iteration-1."""
import json, sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
from grade import grade_run, EVALS_JSON  # reuse the iteration-1 grader functions

WORKSPACE = Path("/Users/simon.lynch/git/agentskill-sdkv2-to-framework/sdkv2-to-framework-workspace/iteration-2")

def main():
    evals = json.loads(EVALS_JSON.read_text())["evals"]
    for ev in evals:
        eval_dir = WORKSPACE / ev["name"]
        grading = grade_run(eval_dir, "with_skill", ev)
        out = eval_dir / "with_skill" / "grading.json"
        out.write_text(json.dumps(grading, indent=2))
        sr = grading["summary"]
        print(f"  {ev['name']}/with_skill: {sr['passed']}/{sr['total']} ({sr['pass_rate']:.0%})")

if __name__ == "__main__":
    main()
