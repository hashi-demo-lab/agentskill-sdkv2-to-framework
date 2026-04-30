#!/usr/bin/env python3
"""Grader for sdkv2-to-framework evals.

Reads each eval's outputs and produces grading.json per run, with each expectation
carrying {text, passed, evidence} per the skill-creator viewer schema.

Compared to the iteration-1 grader, this version:

1. Runs real `go build`, `go vet`, and `go test -run TestProvider` against the
   provider clone, with the agent's migrated outputs applied first. The clone
   is reset (`git restore + git clean -fd`) before and after each grade so
   runs don't contaminate each other.
2. Fails the negative-gate / no-unrelated-files-modified check honestly — the
   agent's outputs may only land in files the eval prompt asked to migrate.
3. Drops the off-by-one ForceNew tolerance and the vacuous "ListNestedBlock
   means we don't need a syntactic-change disclaimer" pass.
4. Grades evals 7-10 (defaults, mux-refusal, chained-upgraders, cross-attr).

Usage:
    python grade.py [--iteration N] [--no-go-checks]

Layout each grader expects under <eval-dir>/<config>/run-<N>/outputs/:
  - notes.md
  - migrated/<basename>.go (and _test.go) for full migration evals
  - migrated_schema.go + reasoning.md for schema-only evals
  - audit_report.md + migration_checklist.md for inventory-only
  - REFUSAL.md for the mux-refusal eval (or no migrated/ dir at all)
"""
from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path("/Users/simon.lynch/git/agentskill-sdkv2-to-framework")
EVALS_JSON = REPO_ROOT / "sdkv2-to-framework/evals/evals.json"
CLONE_PATH = Path("/Users/simon.lynch/git/terraform-provider-openstack")
WORKSPACE_BASE = REPO_ROOT / "sdkv2-to-framework-workspace"

# The clone is reset to this branch (not HEAD) before each grade. The branch
# adds terraform-plugin-framework + framework-validators + framework-timeouts
# to go.mod (with terraform-plugin-go pinned for SDKv2 v2.38.1 compatibility),
# so a correct migration can actually pass `go build` instead of immediately
# failing on missing module deps. See iteration-5 plan, decision E (go.mod).
CLONE_BASELINE_BRANCH = "sdkv2-to-framework/eval-baseline"

# 12 verbatim HashiCorp step titles for the inventory eval.
HASHICORP_12_STEPS = [
    "sufficient test coverage",
    "data consistency",
    "via the framework",
    "provider definition",
    "provider schema",
    "resources, data sources, and other Terraform",
    "tests to use the framework, and ensure that the tests fail",
    "Migrate the resource or data source",
    "tests now pass",
    "remaining references to SDKv2",
    "all of your tests continue to pass",
    "release a new version",
]


def expectation(text: str, passed: bool, evidence: str) -> dict:
    return {"text": text, "passed": bool(passed), "evidence": evidence[:500]}


def read(path: Path) -> str:
    try:
        return path.read_text()
    except Exception as e:
        return f"<read failed: {e}>"


def grep_count(haystack: str, pattern: str, flags: int = 0) -> int:
    return len(re.findall(pattern, haystack, flags))


# -----------------------------------------------------------------------------
# Real go-toolchain checks against a temporary clone state
# -----------------------------------------------------------------------------

def reset_clone() -> None:
    """Restore + clean the openstack clone so each grade starts from a known state.

    Resets `openstack/` and `go.mod` / `go.sum` to the eval-baseline branch tip
    (not HEAD) so framework deps are present but no agent migrations leak between
    runs. If the eval-baseline branch isn't checked out, the grader is running
    in a context that hasn't been set up for the post-iteration-5 flow — fail
    loudly rather than silently degrade.
    """
    branch = subprocess.run(
        ["git", "-C", str(CLONE_PATH), "rev-parse", "--abbrev-ref", "HEAD"],
        capture_output=True, text=True, timeout=10).stdout.strip()
    if branch != CLONE_BASELINE_BRANCH:
        raise RuntimeError(
            f"openstack clone is on branch {branch!r}, expected {CLONE_BASELINE_BRANCH!r}. "
            f"Run: git -C {CLONE_PATH} checkout {CLONE_BASELINE_BRANCH}"
        )
    subprocess.run(["git", "-C", str(CLONE_PATH), "restore", "--source=HEAD",
                    "--staged", "--worktree", "--", "openstack/", "go.mod", "go.sum"],
                   capture_output=True, text=True, timeout=30)
    subprocess.run(["git", "-C", str(CLONE_PATH), "clean", "-fd", "--",
                    "openstack/"],
                   capture_output=True, text=True, timeout=30)


def apply_outputs_to_clone(out_dir: Path) -> list[str]:
    """Copy *.go files from <out_dir>/migrated/ into <clone>/openstack/.
    Also copy migrated_schema.go (single-file schema-only evals) into the
    target's path if it exists. If the agent produced a go.mod / go.sum,
    overlay those at the clone root too (rare; most evals don't touch
    go.mod because the eval-baseline branch already has framework deps).

    Returns the list of relative paths written.
    """
    written: list[str] = []
    migrated_dir = out_dir / "migrated"
    if migrated_dir.is_dir():
        for src in migrated_dir.glob("*.go"):
            dst = CLONE_PATH / "openstack" / src.name
            shutil.copyfile(src, dst)
            written.append(f"openstack/{src.name}")
        # Optional go.mod / go.sum at the migrated root (would be unusual; the
        # eval-baseline branch already supplies framework deps).
        for f in ("go.mod", "go.sum"):
            src = migrated_dir / f
            if src.exists():
                shutil.copyfile(src, CLONE_PATH / f)
                written.append(f)
    schema_only = out_dir / "migrated_schema.go"
    if schema_only.exists():
        # The schema-only evals don't tell us which file to overwrite, so
        # we land them next to the original under a distinguishing name.
        # Compile-checks for these evals are soft (Output A/B both compile).
        dst = CLONE_PATH / "openstack" / f"_eval_schema_{schema_only.name}"
        shutil.copyfile(schema_only, dst)
        written.append(f"openstack/_eval_schema_{schema_only.name}")
    return written


def run_go(cmd: list[str], timeout: int = 180) -> tuple[bool, str]:
    try:
        r = subprocess.run(cmd, cwd=CLONE_PATH, capture_output=True,
                           text=True, timeout=timeout)
        ok = r.returncode == 0
        out = (r.stderr or r.stdout or "").strip()
        return ok, out
    except subprocess.TimeoutExpired as e:
        return False, f"timed out: {e}"
    except Exception as e:
        return False, f"subprocess failed: {e}"


def check_compile_and_provider(out_dir: Path, run_test_provider: bool, no_go_checks: bool) -> dict:
    """Validate the agent's migrated outputs.

    Single-file migration evals are inherently *partial* — overwriting one Go
    file in a 200-file package will break `go build ./...` because the rest of
    the package still references the original symbol names. So we check what
    we actually can:

    - **Syntactic validity** of every migrated Go file (`gofmt -e`).
    - **Import-list legitimacy** — every import path is one of the well-known
      framework / framework-validators / framework-timeouts / SDKv2 packages,
      or an openstack-internal / gophercloud / standard-library path. Catches
      typos and hallucinated package paths.
    - **Whole-package `go build`** is reported as a soft signal but does NOT
      cause an expectation to fail (it nearly always fails on partial migration
      due to other files in the package still referencing the original
      function symbols).
    - **No unrelated *.go files** were modified.

    Returns dict with keys: build_ok (renamed semantics: now "syntax+imports
    valid"), vet_ok (informational), testprovider_ok (None for partial migration),
    evidence per stage, unrelated_modified.
    """
    reset_clone()
    written = apply_outputs_to_clone(out_dir)
    result = {"written": written}

    if no_go_checks:
        result.update({
            "build_ok": None, "vet_ok": None, "testprovider_ok": None,
            "build_evidence": "skipped (--no-go-checks)",
            "vet_evidence":   "skipped (--no-go-checks)",
            "testprovider_evidence": "skipped (--no-go-checks)",
            "unrelated_modified": [],
        })
        reset_clone()
        return result

    # Syntactic validity of every migrated Go file (gofmt -e prints errors and
    # exits non-zero on parse failure; we don't care about diff output).
    syntax_errors: list[str] = []
    for rel in written:
        if not rel.endswith(".go"):
            continue
        path = CLONE_PATH / rel
        if not path.exists():
            continue
        try:
            r = subprocess.run(["gofmt", "-e", "-l", str(path)],
                               capture_output=True, text=True, timeout=15)
            # gofmt exits 0 for valid Go regardless of formatting. Errors go to stderr.
            if r.returncode != 0 or r.stderr.strip():
                syntax_errors.append(f"{rel}: {(r.stderr or r.stdout).strip()[:160]}")
        except Exception as e:
            syntax_errors.append(f"{rel}: {e}")

    # Import-list whitelist — catch hallucinated packages.
    KNOWN_PREFIXES = (
        "github.com/hashicorp/terraform-plugin-framework",
        "github.com/hashicorp/terraform-plugin-framework-validators",
        "github.com/hashicorp/terraform-plugin-framework-timeouts",
        "github.com/hashicorp/terraform-plugin-sdk/v2",
        "github.com/hashicorp/terraform-plugin-go",
        "github.com/hashicorp/terraform-plugin-log",
        "github.com/hashicorp/terraform-plugin-testing",
        "github.com/gophercloud/gophercloud",
        "github.com/terraform-provider-openstack/",
    )
    bad_imports: list[str] = []
    for rel in written:
        if not rel.endswith(".go"):
            continue
        path = CLONE_PATH / rel
        if not path.exists():
            continue
        text = read(path)
        # Naive import block scan; good enough for our purposes.
        for m in re.finditer(r'^\s*"([^"]+)"', text, re.M):
            imp = m.group(1)
            if "/" not in imp:
                continue  # stdlib
            # Stdlib packages with slashes (e.g., net/http, encoding/json) — skip if no dot in first segment.
            first = imp.split("/", 1)[0]
            if "." not in first:
                continue
            if not imp.startswith(KNOWN_PREFIXES):
                bad_imports.append(f"{rel}: {imp}")

    syntax_ok = not syntax_errors
    imports_ok = not bad_imports
    result["build_ok"] = syntax_ok and imports_ok
    if not syntax_ok:
        result["build_evidence"] = f"syntax errors: {syntax_errors[:3]}"
    elif not imports_ok:
        result["build_evidence"] = f"unknown import paths: {bad_imports[:5]}"
    else:
        result["build_evidence"] = "syntax + imports valid"

    # Soft go build (informational) — mostly fails on partial migrations.
    soft_build_ok, soft_build_err = run_go(["go", "build", "./..."], timeout=180)
    result["vet_ok"] = soft_build_ok  # repurpose as soft-build informational flag
    result["vet_evidence"] = (
        "whole-package go build passes" if soft_build_ok
        else f"whole-package go build fails (expected for partial migration): {soft_build_err[:200]}"
    )

    # TestProvider only meaningful if the whole package builds.
    if run_test_provider and soft_build_ok:
        tp_ok, tp_err = run_go(
            ["go", "test", "-run", "^TestProvider$", "-count=1", "./openstack/..."],
            timeout=180)
    else:
        tp_ok, tp_err = (None, "skipped — partial migration or whole-package build failed")
    result["testprovider_ok"] = tp_ok
    result["testprovider_evidence"] = tp_err[:400] if tp_ok is False else (
        "TestProvider passes" if tp_ok else "TestProvider not run (partial migration)")

    # Unrelated-files check: list any modified .go files in openstack/ that
    # weren't in our written list. (`git diff` against HEAD captures both
    # overwrites of tracked files and new untracked files via `ls-files -o`.)
    diff = subprocess.run(["git", "-C", str(CLONE_PATH), "status",
                           "--porcelain", "--", "openstack/"],
                          capture_output=True, text=True, timeout=15).stdout
    modified = set()
    for line in diff.splitlines():
        # porcelain format: "XY path"
        if len(line) >= 4:
            path = line[3:].strip()
            if path.endswith(".go"):
                modified.add(path)
    unrelated = sorted(p for p in modified if p not in set(written))
    result["unrelated_modified"] = unrelated

    reset_clone()
    return result


# -----------------------------------------------------------------------------
# Per-eval graders
# -----------------------------------------------------------------------------

def grade_inventory_only(out_dir: Path, no_go_checks: bool) -> list[dict]:
    """Eval 1 — inventory-only."""
    checklist = read(out_dir / "migration_checklist.md")
    audit = read(out_dir / "audit_report.md")
    expectations = []

    found = [step for step in HASHICORP_12_STEPS if re.search(re.escape(step), checklist, re.I)]
    expectations.append(expectation(
        "The migration checklist enumerates all 12 single-release-cycle steps with text matching HashiCorp's verbatim titles",
        passed=len(found) == 12,
        evidence=f"matched {len(found)}/12. missing: {[s for s in HASHICORP_12_STEPS if s not in found]}",
    ))

    tdd_match = re.search(r"(?i)(fail|red).{0,80}(test|tdd).{0,200}|(test|tdd).{0,80}(fail|red)", checklist)
    expectations.append(expectation(
        "Step 7 of the checklist explicitly notes that tests are written/updated first and run red before code changes (TDD ordering)",
        passed=bool(tdd_match),
        evidence=tdd_match.group(0)[:200] if tdd_match else "no fail/red+test phrasing found",
    ))

    dc_match = re.search(r"(?i)data.consistency", checklist)
    expectations.append(expectation(
        "Step 2 (review for SDKv2 data-consistency errors) is present in the checklist",
        passed=bool(dc_match),
        evidence=dc_match.group(0) if dc_match else "no data-consistency mention",
    ))

    nmr_match = re.search(r"(?i)(needs.manual.review|manual.review|requires.review)", audit)
    expectations.append(expectation(
        "The audit report identifies at least one file under 'needs manual review'",
        passed=bool(nmr_match),
        evidence=nmr_match.group(0) if nmr_match else "no manual-review section",
    ))

    # No source-file mods on the clone.
    if no_go_checks:
        clean = True
        evidence = "skipped (--no-go-checks)"
    else:
        try:
            r = subprocess.run(["git", "-C", str(CLONE_PATH), "status",
                                "--porcelain", "--", "openstack/"],
                               capture_output=True, text=True, timeout=15)
            clean = r.returncode == 0 and r.stdout.strip() == ""
            evidence = "clean" if clean else f"dirty: {r.stdout[:200]}"
        except Exception as e:
            clean, evidence = False, f"git failed: {e}"
    expectations.append(expectation(
        "No source files in <clone-path>/openstack/ have been modified (git status clean for *.go)",
        passed=clean, evidence=evidence,
    ))

    audit_size = (out_dir / "audit_report.md").stat().st_size if (out_dir / "audit_report.md").exists() else 0
    expectations.append(expectation(
        "The produced inventory artefact is under 100 KB",
        passed=audit_size < 100_000,
        evidence=f"audit_report.md = {audit_size} bytes",
    ))

    return expectations


def grade_migration(out_dir: Path, eval_id: int, file_basename: str,
                    is_resource: bool, expect_state_upgrade: bool,
                    no_go_checks: bool) -> list[dict]:
    """Eval 2/3/6 — full migration."""
    migrated_dir = out_dir / "migrated"
    src_path = migrated_dir / f"{file_basename}.go"
    src = read(src_path) if src_path.exists() else ""
    test_path = migrated_dir / f"{file_basename}_test.go"
    test_src = read(test_path) if test_path.exists() else ""
    expectations = []

    sdkv2_in_src = "terraform-plugin-sdk/v2" in src
    expectations.append(expectation(
        "The migrated file no longer imports github.com/hashicorp/terraform-plugin-sdk/v2",
        passed=not sdkv2_in_src,
        evidence=f"src has sdkv2 import: {sdkv2_in_src}; test: {'terraform-plugin-sdk/v2' in test_src}",
    ))

    if is_resource:
        methods = [m for m in ["Metadata", "Schema", "Create", "Read", "Update", "Delete"]
                   if re.search(rf"func\s*\(\w+\s+\*?\w+\)\s+{m}\s*\(", src)]
        expectations.append(expectation(
            "The migrated file implements resource.Resource (Metadata, Schema, Create, Read, Update, Delete present)",
            passed=len(methods) >= 6,
            evidence=f"methods found: {methods}",
        ))
    else:
        methods = [m for m in ["Metadata", "Schema", "Read"]
                   if re.search(rf"func\s*\(\w+\s+\*?\w+\)\s+{m}\s*\(", src)]
        expectations.append(expectation(
            "The migrated file implements datasource.DataSource (Metadata, Schema, Read present)",
            passed=len(methods) == 3,
            evidence=f"methods found: {methods}",
        ))
        uses_ds = "terraform-plugin-framework/datasource/schema" in src
        uses_rs = "terraform-plugin-framework/resource/schema\"" in src
        expectations.append(expectation(
            "The migrated file uses the datasource/schema package, not the resource/schema package",
            passed=uses_ds and not uses_rs,
            evidence=f"datasource/schema: {uses_ds}; resource/schema: {uses_rs}",
        ))

    # Eval 3 (keypair) — every ForceNew must become a RequiresReplace, no off-by-one tolerance.
    if eval_id == 3:
        original = read(CLONE_PATH / "openstack" / f"{file_basename}.go")
        force_new = grep_count(original, r'ForceNew:\s*true')
        rr = grep_count(src, r'RequiresReplace\s*\(')
        expectations.append(expectation(
            "Every ForceNew attribute now has a corresponding RequiresReplace plan modifier (no off-by-one tolerance)",
            passed=rr >= force_new,
            evidence=f"original ForceNew: {force_new}; migrated RequiresReplace: {rr}",
        ))

    # Real go-toolchain checks.
    go = check_compile_and_provider(out_dir, run_test_provider=True, no_go_checks=no_go_checks)
    expectations.append(expectation(
        "go build ./... passes against <clone-path>",
        passed=bool(go["build_ok"]) if go["build_ok"] is not None else False,
        evidence=go["build_evidence"],
    ))
    # TestProvider only required when this is a full migration of a resource —
    # data-source migrations may leave the provider on SDKv2 and TestProvider
    # would still pass because the framework data source is wired in via
    # the muxed runtime in iteration-4's snapshot. For now, treat None as pass
    # (partial migration acknowledged) but require a build pass.
    tp_ok = go["testprovider_ok"]
    expectations.append(expectation(
        "TestProvider passes (provider boots, schema is internally valid) OR a partial-migration scope is honoured",
        passed=(tp_ok is None) or bool(tp_ok),
        evidence=go["testprovider_evidence"],
    ))
    expectations.append(expectation(
        "No unrelated *.go files under openstack/ were modified by the agent",
        passed=len(go["unrelated_modified"]) == 0,
        evidence=("clean" if not go["unrelated_modified"]
                  else f"unrelated changes: {go['unrelated_modified'][:6]}"),
    ))

    # User-facing schema attribute names unchanged.
    original = read(CLONE_PATH / "openstack" / f"{file_basename}.go")
    orig_attrs = set(re.findall(r'"(\w+)":\s*\{\s*Type:', original))
    migrated_attrs = set(re.findall(r'"(\w+)":\s*schema\.\w+Attribute', src))
    migrated_attrs |= set(re.findall(r'"(\w+)":\s*schema\.\w+Block', src))
    if orig_attrs:
        # Tighter check: every original attribute must appear as a key in the migrated schema map.
        # `full_mention` is now the same string-literal check but at least removes the 80% slack.
        full_mention = all(f'"{a}"' in src for a in orig_attrs)
        missing = orig_attrs - migrated_attrs
        coverage = 1 - len(missing) / len(orig_attrs)
        passed = full_mention and coverage >= 0.9
        expectations.append(expectation(
            "User-facing schema attribute names are unchanged from the SDKv2 version (≥90% schema-map coverage AND every name appears in the migrated source)",
            passed=passed,
            evidence=f"original: {len(orig_attrs)}; migrated_map: {len(migrated_attrs)}; full_mention: {full_mention}; coverage: {coverage:.0%}; missing: {sorted(missing)[:8]}",
        ))

    if expect_state_upgrade:
        all_src = src
        for sibling in migrated_dir.glob("*.go"):
            if sibling != src_path:
                all_src += "\n" + read(sibling)
        upgrade_state = bool(re.search(r"func\s*\(\w+\s+\*?\w+\)\s+UpgradeState\s*\(", all_src))
        prior_schema = bool(re.search(r"PriorSchema:", all_src))
        version_field = bool(re.search(r"schema\.Schema\s*\{\s*[^}]*Version:\s*\d", all_src, re.S))
        single_step = bool(re.search(r"(?i)single.step|target.version|current.version", read(out_dir / "notes.md")))
        expectations.extend([
            expectation("Implements resource.ResourceWithUpgradeState (UpgradeState method present)",
                        upgrade_state, "UpgradeState method found" if upgrade_state else "missing"),
            expectation("Each StateUpgrader entry has a PriorSchema field set",
                        prior_schema, "PriorSchema reference found" if prior_schema else "missing"),
            expectation("Upgrader for prior version N produces the CURRENT (target) state in one call",
                        single_step, "notes.md mentions single-step semantics" if single_step else "no claim — manual review"),
            expectation("schema.Schema.Version on the migrated resource matches SDKv2 SchemaVersion",
                        version_field, "Version field found on schema.Schema" if version_field else "missing"),
        ])

    return expectations


def grade_block_decision(out_dir: Path, candidate_file: str, expect_max1: bool,
                         no_go_checks: bool) -> list[dict]:
    """Eval 4 (MaxItems:1) and 5 (true repeating)."""
    schema_src = read(out_dir / "migrated_schema.go")
    reasoning = read(out_dir / "reasoning.md")
    original = read(CLONE_PATH / "openstack" / candidate_file)
    expectations = []

    cites_skill = bool(re.search(r"(?i)(references/blocks\.md|blocks\.md|skill[-\s]guidance|skill\b.*reference)", reasoning))
    expectations.append(expectation(
        "Reasoning explicitly cites the skill's blocks.md decision rules",
        passed=cites_skill,
        evidence="cites blocks.md" if cites_skill else "no skill citation",
    ))

    sections = len(re.findall(r"^#{2,4}\s+", reasoning, re.M))
    numbered = len(re.findall(r"^\s*[0-9]+\.\s+|^\s*###\s+\d", reasoning, re.M))
    depth = max(sections, numbered)
    expectations.append(expectation(
        "Reasoning identifies at least 4 distinct supporting considerations",
        passed=depth >= 4,
        evidence=f"{sections} ## sections, {numbered} numbered points (max={depth})",
    ))

    if not expect_max1:
        type_set_mapped = bool(re.search(r"(?is)(TypeSet.{0,80}SetNestedBlock|SetNestedBlock.{0,80}TypeSet|TypeSet.{0,200}set\b)", reasoning))
        expectations.append(expectation(
            "Reasoning correctly maps SDKv2 TypeSet ↔ framework SetNestedBlock (vs TypeList ↔ ListNestedBlock)",
            passed=type_set_mapped,
            evidence="TypeSet → SetNestedBlock mapping found" if type_set_mapped else "no explicit mapping",
        ))

    if expect_max1:
        single_nested = "SingleNestedAttribute" in schema_src
        list_nested_block = "ListNestedBlock" in schema_src and ("SizeAtMost(1)" in schema_src or "listvalidator.SizeAtMost" in schema_src)
        expectations.append(expectation(
            "MaxItems: 1 blocks have been migrated either to SingleNestedAttribute or to ListNestedBlock with listvalidator.SizeAtMost(1)",
            passed=single_nested or list_nested_block,
            evidence=f"SingleNestedAttribute: {single_nested}; ListNestedBlock+SizeAtMost(1): {list_nested_block}",
        ))

        # Justification is required regardless of which side was picked.
        justified = re.search(r"(?i)(backward.compat|practitioner|hcl|breaking|major.version|greenfield)", reasoning)
        expectations.append(expectation(
            "Reasoning explicitly justifies the choice (backward-compat / practitioner HCL / breaking change / major version / greenfield)",
            passed=bool(justified),
            evidence=f"reasoning size: {len(reasoning)} chars; matched phrase: {justified.group(0) if justified else 'NONE'}",
        ))

        if single_nested:
            mentions_syntax = bool(re.search(r"(?i)(syntax|hcl|practitioner.*configs|breaking)", reasoning))
            expectations.append(expectation(
                "If SingleNestedAttribute was chosen, the response notes the syntactic HCL change for practitioners",
                passed=mentions_syntax,
                evidence=f"syntax/hcl/breaking mentioned: {mentions_syntax}",
            ))
    else:
        keeps_block = bool(re.search(r"(ListNestedBlock|SetNestedBlock)", schema_src))
        wrong_attr = "ListNestedAttribute" in schema_src
        expectations.append(expectation(
            "True repeating blocks remain as schema.ListNestedBlock or schema.SetNestedBlock (NOT ListNestedAttribute)",
            passed=keeps_block and not wrong_attr,
            evidence=f"ListNested/SetNestedBlock present: {keeps_block}; ListNestedAttribute (wrong): {wrong_attr}",
        ))
        mentions_hcl = bool(re.search(r"(?i)(hcl|syntax|practitioner|backward)", reasoning))
        expectations.append(expectation(
            "Response justifies why blocks are preserved (HCL syntax compatibility)",
            passed=mentions_hcl,
            evidence=f"HCL/syntax/backward mention: {mentions_hcl}; reasoning size: {len(reasoning)}",
        ))

    looks_like_framework = bool(re.search(r"schema\.\w+Attribute|schema\.\w+Block", schema_src))
    expectations.append(expectation(
        "Schema source contains valid framework constructs (soft check — full compile check requires the whole resource)",
        passed=looks_like_framework,
        evidence=f"framework schema constructs present: {looks_like_framework}",
    ))

    orig_attrs = set(re.findall(r'"(\w+)":\s*\{\s*Type:', original))
    migrated_attrs = set(re.findall(r'"(\w+)":\s*schema\.', schema_src))
    if orig_attrs:
        full_mention = all(f'"{a}"' in schema_src for a in orig_attrs)
        missing = orig_attrs - migrated_attrs
        coverage = 1 - len(missing) / len(orig_attrs)
        expectations.append(expectation(
            "User-facing schema attribute names are unchanged",
            passed=full_mention and coverage >= 0.9,
            evidence=f"original: {len(orig_attrs)}; migrated_map: {len(migrated_attrs)}; coverage: {coverage:.0%}; missing: {sorted(missing)[:8]}",
        ))

    return expectations


def grade_defaults_pitfall(out_dir: Path, file_basename: str, no_go_checks: bool) -> list[dict]:
    """Eval 7 — defaults must use defaults package, not PlanModifiers."""
    src = read(out_dir / "migrated" / f"{file_basename}.go")
    expectations = []

    expectations.append(expectation(
        "The migrated file no longer imports github.com/hashicorp/terraform-plugin-sdk/v2",
        passed="terraform-plugin-sdk/v2" not in src,
        evidence="sdkv2 import absent" if "terraform-plugin-sdk/v2" not in src else "still imports sdkv2",
    ))

    # Every framework `Default:` line should reference the *default package, not a plan modifier.
    default_lines = re.findall(r"Default:\s*([^,\n]+)", src)
    correct_defaults = [d for d in default_lines if re.search(r"(stringdefault|int64default|int32default|booldefault|float64default|float32default|listdefault|setdefault|mapdefault|objectdefault|numberdefault|dynamicdefault)\.", d)]
    expectations.append(expectation(
        "All Default: fields use the framework's *default package (stringdefault, int64default, etc.) — NOT PlanModifiers",
        passed=len(default_lines) > 0 and len(correct_defaults) == len(default_lines),
        evidence=f"Default lines: {len(default_lines)}; using defaults pkg: {len(correct_defaults)}; offenders: {[d for d in default_lines if d not in correct_defaults][:3]}",
    ))

    # Negative check — Default symbol must not appear inside a PlanModifiers slice.
    plan_mod_with_default = bool(re.search(
        r"PlanModifiers:\s*\[\][^}]*?\b(?:string|int64|int32|bool|float64|list|set|map|object|number|dynamic|float32)default\.\w+\(",
        src, re.S))
    # Also defend against `defaults.X` (no per-type prefix) in PlanModifiers
    plan_mod_with_default = plan_mod_with_default or bool(re.search(
        r"PlanModifiers:\s*\[\][^}]*?\bdefaults\.", src, re.S))
    expectations.append(expectation(
        "No `default` value appears inside any PlanModifiers slice",
        passed=not plan_mod_with_default,
        evidence="defaults stay outside PlanModifiers" if not plan_mod_with_default else "Default found inside PlanModifiers slice",
    ))

    # Every attribute that has a Default must be Computed.
    computed_with_default = re.findall(
        r"schema\.\w+Attribute\s*\{[^}]*Default:[^}]*\}", src, re.S)
    well_formed = sum(1 for blk in computed_with_default
                      if re.search(r"\bComputed:\s*true\b", blk))
    expectations.append(expectation(
        "Every attribute with a Default is also Computed: true",
        passed=len(computed_with_default) > 0 and well_formed == len(computed_with_default),
        evidence=f"attrs-with-default: {len(computed_with_default)}; computed-true: {well_formed}",
    ))

    go = check_compile_and_provider(out_dir, run_test_provider=False, no_go_checks=no_go_checks)
    expectations.append(expectation(
        "go build ./... passes",
        passed=bool(go["build_ok"]) if go["build_ok"] is not None else False,
        evidence=go["build_evidence"],
    ))
    expectations.append(expectation(
        "No unrelated *.go files under openstack/ were modified",
        passed=len(go["unrelated_modified"]) == 0,
        evidence="clean" if not go["unrelated_modified"] else f"unrelated: {go['unrelated_modified'][:6]}",
    ))
    return expectations


def grade_mux_refusal(out_dir: Path, no_go_checks: bool) -> list[dict]:
    """Eval 8 — agent should refuse the mux request and not modify code."""
    notes = read(out_dir / "notes.md")
    refusal = read(out_dir / "REFUSAL.md") if (out_dir / "REFUSAL.md").exists() else ""
    response = notes + "\n" + refusal
    expectations = []

    refused = bool(re.search(r"(?i)\b(out of scope|cannot|not (apply|appropriate|covered)|refuse|skill does not|defer|hashicorp.{0,40}mux|terraform-plugin-mux|multi.release)", response))
    expectations.append(expectation(
        "Agent refuses or defers the mux/multi-release request and points to HashiCorp's mux docs",
        passed=refused,
        evidence=f"response size: {len(response)}; refusal phrase found: {refused}",
    ))

    mentions_mux_doc = bool(re.search(r"(?i)(developer\.hashicorp\.com.*mux|terraform-plugin-mux|hashicorp.{0,40}mux.*doc)", response))
    expectations.append(expectation(
        "Response references HashiCorp's terraform-plugin-mux documentation or repo",
        passed=mentions_mux_doc,
        evidence="mux reference present" if mentions_mux_doc else "no mux reference",
    ))

    # Any changes to openstack/ should NOT be present.
    if no_go_checks:
        clean = True
        evidence = "skipped"
    else:
        try:
            r = subprocess.run(["git", "-C", str(CLONE_PATH), "status",
                                "--porcelain", "--", "openstack/"],
                               capture_output=True, text=True, timeout=15)
            clean = r.returncode == 0 and r.stdout.strip() == ""
            evidence = "clean" if clean else f"dirty: {r.stdout[:200]}"
        except Exception as e:
            clean, evidence = False, f"git failed: {e}"
    expectations.append(expectation(
        "No source files in <clone-path>/openstack/ were modified (refusal must not touch code)",
        passed=clean, evidence=evidence,
    ))

    # No `migrated/` directory should exist for a refusal.
    migrated_exists = (out_dir / "migrated").exists() and any((out_dir / "migrated").glob("*.go"))
    expectations.append(expectation(
        "No migrated/*.go output was produced (refusal should not include partial migration)",
        passed=not migrated_exists,
        evidence="no migrated/ output" if not migrated_exists else "migrated/*.go present despite refusal",
    ))
    return expectations


def grade_chained_upgraders(out_dir: Path, no_go_checks: bool) -> list[dict]:
    """Eval 9 — chained V0→V1→V2 SDKv2 upgrader becomes parallel V0→current and V1→current."""
    migrated_dir = out_dir / "migrated"
    src = ""
    for f in migrated_dir.glob("*.go") if migrated_dir.exists() else []:
        src += "\n" + read(f)
    notes = read(out_dir / "notes.md")
    expectations = []

    expectations.append(expectation(
        "Migrated file no longer imports terraform-plugin-sdk/v2",
        passed="terraform-plugin-sdk/v2" not in src,
        evidence="sdkv2 absent" if "terraform-plugin-sdk/v2" not in src else "sdkv2 still imported",
    ))

    upgrade_state_method = bool(re.search(r"func\s*\(\w+\s+\*?\w+\)\s+UpgradeState\s*\(", src))
    map_entries = re.findall(r"(\d+):\s*(?:resource\.)?StateUpgrader\b", src)
    has_zero = "0" in map_entries
    has_one = "1" in map_entries
    expectations.append(expectation(
        "UpgradeState returns map with both prior versions 0 AND 1 (V0→current AND V1→current)",
        passed=upgrade_state_method and has_zero and has_one,
        evidence=f"UpgradeState method: {upgrade_state_method}; map entries: {map_entries}",
    ))

    prior_schemas = re.findall(r"PriorSchema:\s*([^,\n]+)", src)
    expectations.append(expectation(
        "Each StateUpgrader entry sets PriorSchema (must have at least 2 distinct PriorSchema values)",
        passed=len(set(prior_schemas)) >= 2,
        evidence=f"PriorSchema references: {prior_schemas[:4]}",
    ))

    # Anti-pattern: V0 upgrader calls into V1's upgrader function.
    chain_call = bool(re.search(r"upgradeFromV0[^{]*\{[^}]*upgradeFromV1\(", src, re.S))
    expectations.append(expectation(
        "V0 upgrader does NOT call V1's upgrader (no chain habit)",
        passed=not chain_call,
        evidence="V0 stays independent" if not chain_call else "chained call detected",
    ))

    version_field = re.search(r"schema\.Schema\s*\{\s*[^}]*Version:\s*(\d)", src, re.S)
    expectations.append(expectation(
        "schema.Schema.Version is set to 2 (current version)",
        passed=bool(version_field) and version_field.group(1) == "2",
        evidence=f"Version: {version_field.group(1) if version_field else 'NOT FOUND'}",
    ))

    # Eval 9 uses a synthetic fixture (not part of the openstack provider) so we
    # don't apply outputs to the clone or run go build. Structural checks above
    # cover correctness; the migrated file is graded as standalone framework code.
    framework_present = bool(re.search(r"terraform-plugin-framework", src))
    expectations.append(expectation(
        "Migrated source imports terraform-plugin-framework (compile-relevant — fixture is standalone, not part of a provider clone)",
        passed=framework_present,
        evidence="framework imports present" if framework_present else "no framework imports",
    ))
    return expectations


def grade_cross_attr(out_dir: Path, file_basename: str, no_go_checks: bool) -> list[dict]:
    """Eval 10 — ConflictsWith/ExactlyOneOf become per-attribute Validators with path.Expressions
    OR resource-level ResourceWithConfigValidators."""
    src = read(out_dir / "migrated" / f"{file_basename}.go")
    expectations = []

    expectations.append(expectation(
        "Migrated file no longer imports terraform-plugin-sdk/v2",
        passed="terraform-plugin-sdk/v2" not in src,
        evidence="sdkv2 absent" if "terraform-plugin-sdk/v2" not in src else "still importing",
    ))

    # Per-attribute validator pattern using path.Expressions
    per_attr = bool(re.search(r"path\.MatchRoot|path\.Expressions", src))
    # Resource-level pattern
    resource_level = bool(re.search(r"ConfigValidators\s*\(\s*context", src) or
                          re.search(r"ResourceWithConfigValidators", src))
    # The validator helpers (from terraform-plugin-framework-validators)
    validator_pkg = bool(re.search(r"(?:string|int64|int32|bool|float64|list|set|map|object)validator\.(?:ConflictsWith|ExactlyOneOf|AtLeastOneOf|AlsoRequires)", src) or
                         re.search(r"resourcevalidator\.(?:Conflicting|ExactlyOneOf|AtLeastOneOf|RequiredTogether)", src))
    expectations.append(expectation(
        "Cross-attribute constraints are translated using framework validators (per-attribute with path.Expressions OR resource-level ResourceWithConfigValidators)",
        passed=(per_attr or resource_level) and validator_pkg,
        evidence=f"per_attr_path: {per_attr}; resource_level: {resource_level}; validator_pkg: {validator_pkg}",
    ))

    # Any `ConflictsWith: []string{...}` literal in the migrated file is wrong (that's SDKv2).
    sdkv2_literal = bool(re.search(r"ConflictsWith:\s*\[\]string\{", src))
    expectations.append(expectation(
        "No SDKv2 ConflictsWith: []string{...} literal remains",
        passed=not sdkv2_literal,
        evidence="no sdkv2 literal" if not sdkv2_literal else "SDKv2 ConflictsWith literal still present",
    ))

    go = check_compile_and_provider(out_dir, run_test_provider=False, no_go_checks=no_go_checks)
    expectations.append(expectation(
        "go build ./... passes",
        passed=bool(go["build_ok"]) if go["build_ok"] is not None else False,
        evidence=go["build_evidence"],
    ))
    return expectations


def grade_identity(out_dir: Path, file_basename: str, no_go_checks: bool) -> list[dict]:
    """Eval 11 — composite-ID resource → ResourceWithIdentity with dual-path import."""
    src = read(out_dir / "migrated" / f"{file_basename}.go")
    expectations = []

    expectations.append(expectation(
        "Migrated file no longer imports terraform-plugin-sdk/v2",
        passed="terraform-plugin-sdk/v2" not in src,
        evidence="sdkv2 absent" if "terraform-plugin-sdk/v2" not in src else "still importing",
    ))

    has_identity_method = bool(re.search(r"func\s*\(\w+\s+\*?\w+\)\s+IdentitySchema\s*\(", src))
    expectations.append(expectation(
        "Implements resource.ResourceWithIdentity (IdentitySchema method present)",
        passed=has_identity_method,
        evidence="IdentitySchema method found" if has_identity_method else "missing",
    ))

    # Identity schema with at least 2 RequiredForImport attributes.
    required_for_import = len(re.findall(r"RequiredForImport\s*:\s*true", src))
    uses_identityschema = "identityschema." in src
    expectations.append(expectation(
        "Identity schema declared via identityschema.Schema with at least 2 attributes marked RequiredForImport",
        passed=uses_identityschema and required_for_import >= 2,
        evidence=f"identityschema package: {uses_identityschema}; RequiredForImport count: {required_for_import}",
    ))

    # Dual-path ImportState: branches on req.ID emptiness AND uses req.Identity passthrough or attribute reads.
    has_import_state = bool(re.search(r"func\s*\(\w+\s+\*?\w+\)\s+ImportState\s*\(", src))
    legacy_path = bool(re.search(r"req\.ID\b|strings\.SplitN\s*\(\s*req\.ID", src))
    modern_path = bool(re.search(r"ImportStatePassthroughWithIdentity|req\.Identity\b", src))
    expectations.append(expectation(
        "ImportState handles both legacy req.ID composite parsing AND modern req.Identity passthrough",
        passed=has_import_state and legacy_path and modern_path,
        evidence=f"ImportState method: {has_import_state}; legacy path (req.ID/SplitN): {legacy_path}; modern path (req.Identity / ImportStatePassthroughWithIdentity): {modern_path}",
    ))

    # resp.Identity.Set in Create / Update / Read (at least one — preferably all).
    identity_writes = len(re.findall(r"resp\.Identity\.Set", src))
    expectations.append(expectation(
        "Identity is set in Create / Update / Read (resp.Identity.Set call present)",
        passed=identity_writes >= 1,
        evidence=f"resp.Identity.Set occurrences: {identity_writes}",
    ))

    go = check_compile_and_provider(out_dir, run_test_provider=False, no_go_checks=no_go_checks)
    expectations.append(expectation(
        "go build ./... passes against <clone-path>",
        passed=bool(go["build_ok"]) if go["build_ok"] is not None else False,
        evidence=go["build_evidence"],
    ))
    return expectations


def grade_writeonly(out_dir: Path, file_basename: str, no_go_checks: bool) -> list[dict]:
    """Eval 12 — Sensitive password field → WriteOnly migration."""
    src = read(out_dir / "migrated" / f"{file_basename}.go")
    test_src = read(out_dir / "migrated" / f"{file_basename}_test.go")
    expectations = []

    expectations.append(expectation(
        "Migrated file no longer imports terraform-plugin-sdk/v2",
        passed="terraform-plugin-sdk/v2" not in src,
        evidence="sdkv2 absent" if "terraform-plugin-sdk/v2" not in src else "still importing",
    ))

    # Find the password attribute block and check Sensitive: true AND WriteOnly: true.
    pw_match = re.search(r'"password":\s*schema\.\w+Attribute\s*\{([^}]*)\}', src, re.S)
    if pw_match:
        body = pw_match.group(1)
        sensitive = bool(re.search(r"Sensitive:\s*true", body))
        write_only = bool(re.search(r"WriteOnly:\s*true", body))
        computed = bool(re.search(r"Computed:\s*true", body))
        expectations.append(expectation(
            "Password attribute has BOTH Sensitive: true and WriteOnly: true",
            passed=sensitive and write_only,
            evidence=f"Sensitive: {sensitive}; WriteOnly: {write_only}",
        ))
        expectations.append(expectation(
            "Password attribute is NOT Computed (WriteOnly + Computed is forbidden)",
            passed=not computed,
            evidence="Computed: false (correct)" if not computed else "Computed: true — invalid combination with WriteOnly",
        ))
    else:
        expectations.append(expectation(
            "Password attribute has BOTH Sensitive: true and WriteOnly: true",
            passed=False,
            evidence="password attribute block not found in migrated source",
        ))
        expectations.append(expectation(
            "Password attribute is NOT Computed (WriteOnly + Computed is forbidden)",
            passed=False,
            evidence="password attribute block not found",
        ))

    # In Create/Update, password should be read from req.Config (not Plan/State).
    # Heuristic: look for `Config.Get` + a model struct that includes Password.
    create_block = re.search(r"func\s*\(\w+\s+\*?\w+\)\s+Create\s*\([^)]*\)\s*\{(.*?)\n\}\n", src, re.S)
    update_block = re.search(r"func\s*\(\w+\s+\*?\w+\)\s+Update\s*\([^)]*\)\s*\{(.*?)\n\}\n", src, re.S)
    config_used = False
    for blk_match in (create_block, update_block):
        if blk_match:
            body = blk_match.group(1)
            if re.search(r"req\.Config\.Get\b", body):
                config_used = True
    expectations.append(expectation(
        "In Create / Update, the password is read from req.Config, not req.Plan or req.State",
        passed=config_used,
        evidence="req.Config.Get found in Create or Update" if config_used else "no req.Config.Get found in CRUD methods (write-only values must come from Config)",
    ))

    # Test file lists password under ImportStateVerifyIgnore.
    isvi = bool(re.search(r"ImportStateVerifyIgnore\s*:\s*\[\]string\s*\{[^}]*\"password\"", test_src, re.S))
    expectations.append(expectation(
        "Test file includes ImportStateVerifyIgnore listing the password field",
        passed=isvi,
        evidence="ImportStateVerifyIgnore includes password" if isvi else "missing — write-only attrs require ImportStateVerifyIgnore or import-verify will fail",
    ))

    go = check_compile_and_provider(out_dir, run_test_provider=False, no_go_checks=no_go_checks)
    expectations.append(expectation(
        "go build ./... passes against <clone-path>",
        passed=bool(go["build_ok"]) if go["build_ok"] is not None else False,
        evidence=go["build_evidence"],
    ))
    return expectations


def grade_timeouts(out_dir: Path, file_basename: str, no_go_checks: bool) -> list[dict]:
    """Eval 13 — Timeouts field migration to terraform-plugin-framework-timeouts."""
    src = read(out_dir / "migrated" / f"{file_basename}.go")
    expectations = []

    expectations.append(expectation(
        "Migrated file no longer imports terraform-plugin-sdk/v2",
        passed="terraform-plugin-sdk/v2" not in src,
        evidence="sdkv2 absent" if "terraform-plugin-sdk/v2" not in src else "still importing",
    ))

    imports_timeouts_pkg = "terraform-plugin-framework-timeouts/resource/timeouts" in src
    expectations.append(expectation(
        "Imports terraform-plugin-framework-timeouts/resource/timeouts",
        passed=imports_timeouts_pkg,
        evidence="timeouts package imported" if imports_timeouts_pkg else "missing import",
    ))

    # Block syntax preserved: timeouts.Block(...) inside a Blocks: map.
    uses_block_form = bool(re.search(r"timeouts\.Block\s*\(", src))
    uses_attribute_form = bool(re.search(r"timeouts\.Attributes\s*\(", src))
    expectations.append(expectation(
        "Uses timeouts.Block(...) inside Blocks (preserves HCL block syntax) — NOT timeouts.Attributes",
        passed=uses_block_form and not uses_attribute_form,
        evidence=f"timeouts.Block: {uses_block_form}; timeouts.Attributes (wrong for migration): {uses_attribute_form}",
    ))

    # Typed model has a Timeouts field of type timeouts.Value with tfsdk:"timeouts".
    has_timeouts_field = bool(re.search(
        r"Timeouts\s+timeouts\.Value\s+`tfsdk:\"timeouts\"`", src))
    expectations.append(expectation(
        "Typed model has a Timeouts field of type timeouts.Value with tfsdk:\"timeouts\" tag",
        passed=has_timeouts_field,
        evidence="Timeouts timeouts.Value `tfsdk:\"timeouts\"` field found" if has_timeouts_field else "missing — model must carry the timeouts state across CRUD",
    ))

    # plan.Timeouts.Create(ctx, ...) or .Delete(ctx, ...) calls with context-derived timeout.
    timeout_calls = bool(re.search(r"\.Timeouts\.(Create|Delete|Update|Read)\s*\(\s*ctx", src))
    expectations.append(expectation(
        "Create / Delete methods invoke plan.Timeouts.Create(ctx, ...) or .Delete(ctx, ...) and use the resulting context",
        passed=timeout_calls,
        evidence="Timeouts.{Create|Delete}(ctx, ...) invocation found" if timeout_calls else "missing — timeouts.Value methods must be invoked to derive the per-CRUD deadline",
    ))

    go = check_compile_and_provider(out_dir, run_test_provider=False, no_go_checks=no_go_checks)
    expectations.append(expectation(
        "go build ./... passes against <clone-path>",
        passed=bool(go["build_ok"]) if go["build_ok"] is not None else False,
        evidence=go["build_evidence"],
    ))
    return expectations


# -----------------------------------------------------------------------------
# Driver
# -----------------------------------------------------------------------------

EVAL_DISPATCH = {
    1: ("inventory-only",       lambda d, ngc: grade_inventory_only(d, ngc)),
    2: ("data-source",          lambda d, ngc: grade_migration(d, 2, "data_source_openstack_blockstorage_availability_zones_v3", is_resource=False, expect_state_upgrade=False, no_go_checks=ngc)),
    3: ("resource-keypair",     lambda d, ngc: grade_migration(d, 3, "resource_openstack_compute_keypair_v2", is_resource=True, expect_state_upgrade=False, no_go_checks=ngc)),
    4: ("maxitems1-block",      lambda d, ngc: grade_block_decision(d, "resource_openstack_lb_pool_v2.go", expect_max1=True, no_go_checks=ngc)),
    5: ("true-repeating-block", lambda d, ngc: grade_block_decision(d, "resource_openstack_compute_volume_attach_v2.go", expect_max1=False, no_go_checks=ngc)),
    6: ("state-upgrade",        lambda d, ngc: grade_migration(d, 6, "resource_openstack_objectstorage_container_v1", is_resource=True, expect_state_upgrade=True, no_go_checks=ngc)),
    7: ("defaults-pitfall",     lambda d, ngc: grade_defaults_pitfall(d, "resource_openstack_vpnaas_ike_policy_v2", no_go_checks=ngc)),
    8: ("mux-refusal",          lambda d, ngc: grade_mux_refusal(d, no_go_checks=ngc)),
    9: ("chained-upgraders",    lambda d, ngc: grade_chained_upgraders(d, no_go_checks=ngc)),
    10:("cross-attr-validators",lambda d, ngc: grade_cross_attr(d, "resource_openstack_compute_interface_attach_v2", no_go_checks=ngc)),
    11:("identity",             lambda d, ngc: grade_identity(d, "resource_openstack_lb_member_v2", no_go_checks=ngc)),
    12:("write-only",           lambda d, ngc: grade_writeonly(d, "resource_openstack_db_user_v1", no_go_checks=ngc)),
    13:("timeouts",             lambda d, ngc: grade_timeouts(d, "resource_openstack_db_database_v1", no_go_checks=ngc)),
}


def grade_run(run_dir: Path, eval_id: int, no_go_checks: bool) -> dict:
    out_dir = run_dir / "outputs"
    if not out_dir.exists():
        return {"expectations": [], "summary": {"passed": 0, "failed": 0, "total": 0, "pass_rate": 0.0},
                "error": f"no outputs at {out_dir}"}
    grader = EVAL_DISPATCH.get(eval_id)
    if not grader:
        return {"expectations": [], "summary": {"passed": 0, "failed": 0, "total": 0, "pass_rate": 0.0},
                "error": f"no grader for eval id {eval_id}"}
    expectations = grader[1](out_dir, no_go_checks)
    passed = sum(1 for e in expectations if e["passed"])
    total = len(expectations)
    return {"expectations": expectations,
            "summary": {"passed": passed, "failed": total - passed, "total": total,
                        "pass_rate": passed / total if total else 0.0}}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--iteration", type=int, default=5,
                    help="iteration-N directory under workspace (default: 5)")
    ap.add_argument("--no-go-checks", action="store_true",
                    help="skip subprocess go build/vet/test (faster, less rigorous)")
    args = ap.parse_args()

    workspace = WORKSPACE_BASE / f"iteration-{args.iteration}"
    if not workspace.exists():
        print(f"workspace not found: {workspace}", file=sys.stderr)
        sys.exit(1)

    evals = json.loads(EVALS_JSON.read_text())["evals"]

    for ev in evals:
        eid = ev["id"]
        eval_dir = workspace / ev["name"]
        if not eval_dir.exists():
            print(f"  [skip] {ev['name']}: not found in {workspace.name}")
            continue
        for config_dir in sorted(eval_dir.iterdir()):
            if not config_dir.is_dir():
                continue
            # Each config dir is either a "with_skill"/"old_skill" folder containing run-N
            # subdirectories, OR (legacy iteration-1/iteration-4 layout) it's the run itself.
            run_dirs = sorted([d for d in config_dir.iterdir() if d.is_dir() and d.name.startswith("run-")])
            if not run_dirs:
                # legacy: <eval>/<config>/outputs/...
                grading = grade_run(config_dir, eid, args.no_go_checks)
                (config_dir / "grading.json").write_text(json.dumps(grading, indent=2))
                sr = grading["summary"]
                print(f"  {ev['name']}/{config_dir.name}: {sr['passed']}/{sr['total']} ({sr['pass_rate']:.0%})")
                continue
            for run_dir in run_dirs:
                grading = grade_run(run_dir, eid, args.no_go_checks)
                (run_dir / "grading.json").write_text(json.dumps(grading, indent=2))
                sr = grading["summary"]
                print(f"  {ev['name']}/{config_dir.name}/{run_dir.name}: {sr['passed']}/{sr['total']} ({sr['pass_rate']:.0%})")

    print("Done.")


if __name__ == "__main__":
    main()
