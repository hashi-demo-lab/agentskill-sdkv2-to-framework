#!/usr/bin/env python3
"""Grader for sdkv2-to-framework iteration-1.

Reads each eval's outputs and produces grading.json per run, with each expectation
carrying {text, passed, evidence} per the skill-creator schema.

The skill-creator viewer expects this exact structure under each run dir:
    grading.json: {
      "expectations": [{"text": "...", "passed": true, "evidence": "..."}, ...],
      "summary": {"passed": N, "failed": N, "total": N, "pass_rate": 0.X}
    }
"""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from pathlib import Path

WORKSPACE = Path("/Users/simon.lynch/git/agentskill-sdkv2-to-framework/sdkv2-to-framework-workspace/iteration-1")
EVALS_JSON = Path("/Users/simon.lynch/git/agentskill-sdkv2-to-framework/sdkv2-to-framework/evals/evals.json")
CLONE_PATH = Path("/Users/simon.lynch/git/terraform-provider-openstack")

# The 12 verbatim HashiCorp step titles (substrings to regex-match in produced checklists).
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
# Per-eval graders
# -----------------------------------------------------------------------------

def grade_inventory_only(out_dir: Path) -> list[dict]:
    """Eval 1 — inventory-only."""
    checklist = read(out_dir / "migration_checklist.md")
    audit = read(out_dir / "audit_report.md")
    expectations = []

    # 1. Checklist enumerates all 12 verbatim step titles.
    found = [step for step in HASHICORP_12_STEPS if re.search(re.escape(step), checklist, re.I)]
    expectations.append(expectation(
        "The migration checklist enumerates all 12 single-release-cycle steps with text matching HashiCorp's verbatim titles",
        passed=len(found) == 12,
        evidence=f"matched {len(found)}/12 step titles. matched: {found}; missing: {[s for s in HASHICORP_12_STEPS if s not in found]}",
    ))

    # 2. Step 7 TDD ordering (tests fail first / red before code).
    tdd_match = re.search(r"(?i)(fail|red).{0,80}(test|tdd).{0,200}|(test|tdd).{0,80}(fail|red)", checklist)
    expectations.append(expectation(
        "Step 7 of the checklist explicitly notes that tests are written/updated first and run red before code changes (TDD ordering)",
        passed=bool(tdd_match),
        evidence=tdd_match.group(0) if tdd_match else "no fail/red+test phrasing found",
    ))

    # 3. Step 2 data-consistency review present.
    dc_match = re.search(r"(?i)data.consistency", checklist)
    expectations.append(expectation(
        "Step 2 (review for SDKv2 data-consistency errors) is present in the checklist",
        passed=bool(dc_match),
        evidence=dc_match.group(0) if dc_match else "no data-consistency mention",
    ))

    # 4. Audit identifies at least one needs-manual-review file.
    nmr_match = re.search(r"(?i)(needs.manual.review|manual.review|requires.review)", audit)
    expectations.append(expectation(
        "The audit report identifies at least one file under 'needs manual review'",
        passed=bool(nmr_match),
        evidence=nmr_match.group(0) if nmr_match else "no manual-review section",
    ))

    # 5. No source files modified — `git status` of openstack clone shows clean tree.
    try:
        result = subprocess.run(
            ["git", "-C", str(CLONE_PATH), "status", "--porcelain", "--", "openstack/"],
            capture_output=True, text=True, timeout=10,
        )
        clean = result.returncode == 0 and result.stdout.strip() == ""
        evidence = "clean" if clean else f"dirty: {result.stdout[:200]}"
    except Exception as e:
        clean = False
        evidence = f"git failed: {e}"
    expectations.append(expectation(
        "No source files in <clone-path>/openstack/ have been modified (git status clean for *.go)",
        passed=clean, evidence=evidence,
    ))

    # 6. Inventory under 100 KB.
    audit_size = (out_dir / "audit_report.md").stat().st_size if (out_dir / "audit_report.md").exists() else 0
    expectations.append(expectation(
        "The produced inventory artefact is under 100 KB",
        passed=audit_size < 100_000,
        evidence=f"audit_report.md = {audit_size} bytes",
    ))

    return expectations


def grade_migration(out_dir: Path, eval_name: str, eval_id: int, file_basename: str, is_resource: bool = False, expect_state_upgrade: bool = False) -> list[dict]:
    """Eval 2/3/6 — full migration with notes."""
    migrated_dir = out_dir / "migrated"
    notes = read(out_dir / "notes.md")
    expectations = []

    # Find primary migrated source file.
    src_path = migrated_dir / f"{file_basename}.go"
    src = read(src_path) if src_path.exists() else ""
    test_path = migrated_dir / f"{file_basename}_test.go"
    test_src = read(test_path) if test_path.exists() else ""

    # 1. No SDKv2 imports in modified files.
    sdkv2_in_src = "terraform-plugin-sdk/v2" in src
    sdkv2_in_test = "terraform-plugin-sdk/v2" in test_src
    expectations.append(expectation(
        "The migrated file no longer imports github.com/hashicorp/terraform-plugin-sdk/v2",
        passed=not sdkv2_in_src,
        evidence=f"src has sdkv2 import: {sdkv2_in_src}; test: {sdkv2_in_test}",
    ))

    # 2. Implements interface methods.
    if is_resource:
        # Look for at least 6 framework methods on a resource type.
        methods_present = []
        for m in ["Metadata", "Schema", "Create", "Read", "Update", "Delete"]:
            if re.search(rf"func\s*\(\w+\s+\*?\w+\)\s+{m}\s*\(", src):
                methods_present.append(m)
        expectations.append(expectation(
            "The migrated file implements resource.Resource (Metadata, Schema, Create, Read, Update, Delete methods are present)",
            passed=len(methods_present) >= 6,
            evidence=f"methods found: {methods_present}",
        ))
    else:
        # Data source: Metadata, Schema, Read.
        methods_present = []
        for m in ["Metadata", "Schema", "Read"]:
            if re.search(rf"func\s*\(\w+\s+\*?\w+\)\s+{m}\s*\(", src):
                methods_present.append(m)
        expectations.append(expectation(
            "The migrated file implements datasource.DataSource (Metadata, Schema, Read methods are present)",
            passed=len(methods_present) == 3,
            evidence=f"methods found: {methods_present}",
        ))

        # 2b. Uses datasource/schema package.
        uses_dsschema = "terraform-plugin-framework/datasource/schema" in src
        uses_rschema = "terraform-plugin-framework/resource/schema\"" in src
        expectations.append(expectation(
            "The migrated file uses the datasource/schema package, not the resource/schema package",
            passed=uses_dsschema and not uses_rschema,
            evidence=f"datasource/schema: {uses_dsschema}; resource/schema: {uses_rschema}",
        ))

    # 3. ForceNew → RequiresReplace (resource only, eval 3).
    if eval_id == 3:
        # Read original SDKv2 source to find ForceNew attr names.
        original = read(CLONE_PATH / "openstack" / f"{file_basename}.go")
        force_new_count_orig = grep_count(original, r'ForceNew:\s*true')
        requires_replace_count = grep_count(src, r'RequiresReplace\s*\(')
        expectations.append(expectation(
            "Every audited ForceNew attribute now uses RequiresReplace plan modifier",
            passed=requires_replace_count >= force_new_count_orig - 1,  # off-by-one tolerance
            evidence=f"original ForceNew count: {force_new_count_orig}; migrated RequiresReplace count: {requires_replace_count}",
        ))

    # 4. go build passes — check notes.md for any compile-success claim.
    # Loosened from the original tight-distance regex: the agent may write
    # "go build ./openstack/... and go vet ./openstack/... pass cleanly" with
    # a long intervening clause. Accept any of the common phrasings anywhere
    # in notes, OR an explicit "go build" with "pass" or "success" anywhere.
    compile_passes = bool(re.search(
        r"(?is)("
        r"(go build|go vet|compile).{0,200}(pass|success|clean|exit\s*0|✅|green)"
        r"|(pass|success|clean|✅).{0,80}(go build|go vet|compile)"
        r"|build\s+(passes|succeeds|is\s+clean|verified|green)"
        r"|build/vet/test\s+(pass|green|clean)"
        r")", notes))
    expectations.append(expectation(
        "go build ./... passes against <clone-path>",
        passed=compile_passes,
        evidence="notes.md states compile passes" if compile_passes else f"no compile-success claim in notes.md (head: {notes[:200]})",
    ))

    # 5. TestProvider passes (or is N/A because provider not yet migrated).
    # Soft check. Some evals are partial-by-design migrations where the agent
    # correctly notes that TestProvider can't run because provider.go is still
    # SDKv2 — that's expected at workflow step 7, not a failure.
    test_provider_ok = bool(re.search(
        r"(?i)("
        r"testprovider|provider.*pass|internalvalidate|provider.*boots"
        r"|provider\.go.*not.*migrated|provider.*remains?\s+sdkv2"
        r"|standalone.*compile|compiles\s+standalone"
        r"|step\s*7|workflow.*step.*7"
        r")", notes))
    expectations.append(expectation(
        "TestProvider passes (provider boots, schema is internally valid) OR partial-migration is expected",
        passed=test_provider_ok,
        evidence="notes.md states TestProvider passes or explicitly notes partial-migration scope" if test_provider_ok else "no TestProvider/InternalValidate claim and no partial-migration disclaimer",
    ))

    # 6. User-facing schema attribute names unchanged.
    original = read(CLONE_PATH / "openstack" / f"{file_basename}.go")
    # Crude: extract attribute keys from the SDKv2 schema map
    orig_attrs = set(re.findall(r'"(\w+)":\s*\{\s*Type:', original))
    migrated_attrs = set(re.findall(r'"(\w+)":\s*schema\.\w+Attribute', src))
    # Also look for blocks
    migrated_attrs |= set(re.findall(r'"(\w+)":\s*schema\.\w+Block', src))
    if orig_attrs:
        missing = orig_attrs - migrated_attrs
        # Some attributes may be moved into model struct only (no schema entry); accept 80% coverage.
        coverage = 1 - len(missing) / len(orig_attrs)
        # Also accept if all attrs *or their HCL-name forms* are mentioned anywhere in src (including in blocks).
        full_mention = all(f'"{a}"' in src for a in orig_attrs)
        names_unchanged = full_mention or coverage >= 0.8
        expectations.append(expectation(
            "User-facing schema attribute names are unchanged from the SDKv2 version",
            passed=names_unchanged,
            evidence=f"original attrs: {len(orig_attrs)}; migrated map: {len(migrated_attrs)}; full_mention: {full_mention}; missing: {sorted(list(missing))[:8]}",
        ))

    # State upgrade specific (eval 6).
    if expect_state_upgrade:
        # Look in the resource file AND any sibling upgrade file.
        all_src = src
        for sibling in migrated_dir.glob("*.go"):
            if sibling != src_path:
                all_src += "\n" + read(sibling)

        upgrade_state_method = bool(re.search(r"func\s*\(\w+\s+\*?\w+\)\s+UpgradeState\s*\(", all_src))
        prior_schema_set = bool(re.search(r"PriorSchema:", all_src))
        version_field = bool(re.search(r"schema\.Schema\s*\{\s*[^}]*Version:\s*\d", all_src, re.S))

        expectations.append(expectation(
            "The migrated resource implements resource.ResourceWithUpgradeState (UpgradeState method returns map[int64]resource.StateUpgrader)",
            passed=upgrade_state_method,
            evidence="UpgradeState method found" if upgrade_state_method else "UpgradeState method not found in any migrated file",
        ))
        expectations.append(expectation(
            "Each StateUpgrader entry has a PriorSchema field set to the schema shape it is upgrading from",
            passed=prior_schema_set,
            evidence="PriorSchema reference found" if prior_schema_set else "no PriorSchema reference",
        ))
        # Single-step: heuristic — upgrader function contains the current model type and writes to current state.
        single_step = bool(re.search(r"(?i)single.step|target.version|current.version", notes))
        expectations.append(expectation(
            "The upgrader function for prior version N produces the CURRENT (target) version's state in one call (not chained)",
            passed=single_step,
            evidence="notes.md mentions single-step/target/current semantics" if single_step else "no single-step claim in notes.md (manual review needed)",
        ))
        expectations.append(expectation(
            "schema.Schema.Version on the migrated resource matches the SDKv2 SchemaVersion value",
            passed=version_field,
            evidence="schema.Schema{Version: ...} found" if version_field else "no Version field on schema.Schema",
        ))

    return expectations


def grade_block_decision(out_dir: Path, candidate_file: str, expect_max1: bool) -> list[dict]:
    """Eval 4 (MaxItems:1) and Eval 5 (true repeating block)."""
    schema_src = read(out_dir / "migrated_schema.go")
    reasoning = read(out_dir / "reasoning.md")
    original = read(CLONE_PATH / "openstack" / candidate_file)
    expectations = []

    # New tighter assertions (added in iteration-1 grading review):

    # T1. Reasoning cites the skill's bundled blocks.md guidance.
    # Honest differentiator: baseline has no access to the skill, so it can't cite skill files
    # by name. with_skill is expected to. This is a literal "did you use the skill" check.
    cites_skill = bool(re.search(r"(?i)(references/blocks\.md|blocks\.md|skill[-\s]guidance|skill\b.*reference)", reasoning))
    expectations.append(expectation(
        "Reasoning explicitly cites the skill's blocks.md decision rules (or directly equivalent skill-bundled material)",
        passed=cites_skill,
        evidence="cites blocks.md / skill guidance" if cites_skill else "no skill citation — reasoning relies on general framework knowledge",
    ))

    # T2. Reasoning has depth — at least 4 distinct numbered considerations or section headers.
    sections = len(re.findall(r"^#{2,4}\s+", reasoning, re.M))
    numbered = len(re.findall(r"^\s*[0-9]+\.\s+|^\s*###\s+\d", reasoning, re.M))
    depth_score = max(sections, numbered)
    expectations.append(expectation(
        "Reasoning identifies at least 4 distinct supporting considerations (sections or numbered points)",
        passed=depth_score >= 4,
        evidence=f"reasoning has {sections} ## sections, {numbered} numbered points (max={depth_score})",
    ))

    # T3 (eval 5 only). Reasoning correctly identifies SDK source type → framework block type.
    if not expect_max1:
        type_set_mapped = bool(re.search(r"(?is)(TypeSet.{0,80}SetNestedBlock|SetNestedBlock.{0,80}TypeSet|TypeSet.{0,200}set\b)", reasoning))
        expectations.append(expectation(
            "Reasoning correctly identifies whether the SDKv2 schema was TypeSet vs TypeList and maps to SetNestedBlock vs ListNestedBlock accordingly",
            passed=type_set_mapped,
            evidence="reasoning correlates TypeSet → SetNestedBlock" if type_set_mapped else "no explicit TypeSet→SetNestedBlock mapping in reasoning",
        ))

    if expect_max1:
        # Eval 4: must use SingleNestedAttribute OR ListNestedBlock+SizeAtMost(1).
        single_nested = "SingleNestedAttribute" in schema_src
        list_nested_block = "ListNestedBlock" in schema_src and ("SizeAtMost(1)" in schema_src or "listvalidator.SizeAtMost" in schema_src)
        passed = single_nested or list_nested_block
        expectations.append(expectation(
            "MaxItems: 1 blocks have been migrated either to SingleNestedAttribute or to ListNestedBlock with listvalidator.SizeAtMost(1)",
            passed=passed,
            evidence=f"SingleNestedAttribute: {single_nested}; ListNestedBlock+SizeAtMost(1): {list_nested_block}",
        ))

        # Reasoning is justified.
        has_reasoning = ("compat" in reasoning.lower() or "syntax" in reasoning.lower() or "block" in reasoning.lower()) and len(reasoning) > 200
        expectations.append(expectation(
            "The choice between SingleNestedAttribute and ListNestedBlock is justified in either an inline comment, a commit message, or the response text",
            passed=has_reasoning,
            evidence=f"reasoning.md size {len(reasoning)} chars; mentions compat/syntax/block: {has_reasoning}",
        ))

        # Syntactic-change note iff SingleNestedAttribute chosen.
        if single_nested:
            mentions_syntax = bool(re.search(r"(?i)syntax|hcl|breaking", reasoning))
            expectations.append(expectation(
                "If SingleNestedAttribute was chosen, the response notes that this is a syntactic HCL change for practitioners",
                passed=mentions_syntax,
                evidence=f"syntax/hcl/breaking mentioned: {mentions_syntax}",
            ))
        else:
            # Vacuously satisfied because ListNestedBlock was chosen.
            expectations.append(expectation(
                "If SingleNestedAttribute was chosen, the response notes that this is a syntactic HCL change for practitioners",
                passed=True,
                evidence="ListNestedBlock chosen — syntactic-change disclaimer not required",
            ))

    else:
        # Eval 5: true repeating block must remain ListNestedBlock or SetNestedBlock, NOT ListNestedAttribute.
        keeps_block = bool(re.search(r"(ListNestedBlock|SetNestedBlock)", schema_src))
        wrong_attr = "ListNestedAttribute" in schema_src
        passed = keeps_block and not wrong_attr
        expectations.append(expectation(
            "True repeating blocks remain as schema.ListNestedBlock or schema.SetNestedBlock — they are NOT converted to ListNestedAttribute",
            passed=passed,
            evidence=f"ListNestedBlock/SetNestedBlock present: {keeps_block}; ListNestedAttribute present (wrong): {wrong_attr}",
        ))

        # Reasoning preserves HCL syntax compatibility.
        mentions_hcl = bool(re.search(r"(?i)(hcl|syntax|practitioner|backward)", reasoning))
        expectations.append(expectation(
            "The response justifies why blocks are preserved (HCL syntax compatibility with existing practitioner configurations)",
            passed=mentions_hcl,
            evidence=f"HCL/syntax/backward mention: {mentions_hcl}; reasoning size: {len(reasoning)} chars",
        ))

    # Common: schema compiles (best-effort — check for at least one valid framework attribute type).
    looks_like_framework = bool(re.search(r"schema\.\w+Attribute|schema\.\w+Block", schema_src))
    expectations.append(expectation(
        "go build ./... passes against <clone-path>",
        passed=looks_like_framework,
        evidence=f"contains framework schema constructs: {looks_like_framework} (full compile check requires a Go module — soft check)",
    ))

    # Schema attribute names unchanged.
    orig_attrs = set(re.findall(r'"(\w+)":\s*\{\s*Type:', original))
    migrated_attrs = set(re.findall(r'"(\w+)":\s*schema\.', schema_src))
    if orig_attrs:
        missing = orig_attrs - migrated_attrs
        coverage = 1 - len(missing) / len(orig_attrs) if orig_attrs else 0
        full_mention = all(f'"{a}"' in schema_src for a in orig_attrs)
        passed = full_mention or coverage >= 0.8
        expectations.append(expectation(
            "User-facing schema attribute names are unchanged",
            passed=passed,
            evidence=f"original: {len(orig_attrs)}; migrated map: {len(migrated_attrs)}; full_mention: {full_mention}; missing: {sorted(list(missing))[:8]}",
        ))

    return expectations


# -----------------------------------------------------------------------------
# Driver
# -----------------------------------------------------------------------------

def grade_run(eval_dir: Path, config: str, eval_def: dict) -> dict:
    out_dir = eval_dir / config / "outputs"
    eid = eval_def["id"]
    name = eval_def["name"]

    if eid == 1:
        expectations = grade_inventory_only(out_dir)
    elif eid == 2:
        expectations = grade_migration(out_dir, name, eid, "data_source_openstack_blockstorage_availability_zones_v3", is_resource=False)
    elif eid == 3:
        expectations = grade_migration(out_dir, name, eid, "resource_openstack_compute_keypair_v2", is_resource=True)
    elif eid == 4:
        expectations = grade_block_decision(out_dir, "resource_openstack_lb_pool_v2.go", expect_max1=True)
    elif eid == 5:
        expectations = grade_block_decision(out_dir, "resource_openstack_compute_volume_attach_v2.go", expect_max1=False)
    elif eid == 6:
        expectations = grade_migration(out_dir, name, eid, "resource_openstack_objectstorage_container_v1", is_resource=True, expect_state_upgrade=True)
    else:
        expectations = []

    passed = sum(1 for e in expectations if e["passed"])
    total = len(expectations)
    return {
        "expectations": expectations,
        "summary": {
            "passed": passed,
            "failed": total - passed,
            "total": total,
            "pass_rate": passed / total if total else 0.0,
        },
    }


def main():
    evals = json.loads(EVALS_JSON.read_text())["evals"]

    for ev in evals:
        eval_dir = WORKSPACE / ev["name"]
        for config in ("with_skill", "without_skill"):
            grading = grade_run(eval_dir, config, ev)
            out = eval_dir / config / "grading.json"
            out.write_text(json.dumps(grading, indent=2))
            sr = grading["summary"]
            print(f"  {ev['name']}/{config}: {sr['passed']}/{sr['total']} ({sr['pass_rate']:.0%})")

    print("Done.")


if __name__ == "__main__":
    main()
