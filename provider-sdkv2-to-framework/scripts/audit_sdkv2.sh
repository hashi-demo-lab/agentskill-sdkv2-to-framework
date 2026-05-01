#!/bin/sh
# audit_sdkv2.sh — Inventory a Terraform provider written against terraform-plugin-sdk/v2.
#
# Output: a markdown report (per-file counts table + "needs manual review" bucket).
# Stdout is the report; stderr is progress.
#
# Drives semgrep with rules in audit_sdkv2.semgrep.yml. Semgrep is required —
# its AST-aware multi-line matching reliably identifies patterns the previous
# grep-based version had to flag as "needs manual review" (e.g., MaxItems:1
# paired with a nested Elem &schema.Resource{...}).
#
# Usage:
#   audit_sdkv2.sh <provider-repo-path> [--max-files N]
#
# Exit codes: 0 on success; 1 on bad args; 2 if go.mod is missing or the repo
# does not import terraform-plugin-sdk/v2; 127 if semgrep is missing.

set -eu

# -----------------------------------------------------------------------------
# Pre-req check: semgrep must be installed.
# -----------------------------------------------------------------------------
if ! command -v semgrep >/dev/null 2>&1; then
    cat >&2 <<'EOF'
ERROR: semgrep not found in $PATH.

This skill's audit uses semgrep for AST-aware pattern matching across multi-line
struct literals — something line-by-line grep cannot do reliably.

Install semgrep:
  pip install semgrep                # any platform
  brew install semgrep               # macOS
  sudo apt install semgrep           # Debian/Ubuntu (recent)

Then re-run this script.

If you cannot install semgrep, the previous grep-based audit lives at the
script's git history (commit before semgrep adoption); restoring it is a
supported workaround but loses multi-line pattern accuracy.
EOF
    exit 127
fi

# Pre-req check: python3 (used to aggregate semgrep's JSON output).
if ! command -v python3 >/dev/null 2>&1; then
    echo "ERROR: python3 not found in \$PATH (needed to aggregate semgrep output)." >&2
    exit 127
fi

# -----------------------------------------------------------------------------
# Argument parsing.
# -----------------------------------------------------------------------------
if [ "$#" -lt 1 ]; then
    echo "usage: $0 <provider-repo-path> [--max-files N]" >&2
    exit 1
fi

REPO="$1"; shift || true
MAX_FILES=20

while [ "$#" -gt 0 ]; do
    case "$1" in
        --max-files) MAX_FILES="$2"; shift 2 ;;
        *) echo "unknown arg: $1" >&2; exit 1 ;;
    esac
done

if [ ! -d "$REPO" ]; then
    echo "not a directory: $REPO" >&2
    exit 1
fi

if [ ! -f "$REPO/go.mod" ]; then
    echo "no go.mod at $REPO — not a Go module" >&2
    exit 2
fi

if ! grep -q 'terraform-plugin-sdk/v2' "$REPO/go.mod"; then
    echo "go.mod does not require terraform-plugin-sdk/v2 — nothing to migrate" >&2
    exit 2
fi

# Resolve the rules file relative to this script's location.
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
RULES="$SCRIPT_DIR/audit_sdkv2.semgrep.yml"

if [ ! -f "$RULES" ]; then
    echo "ERROR: rules file not found: $RULES" >&2
    exit 1
fi

# -----------------------------------------------------------------------------
# Run semgrep and aggregate the results in Python.
# -----------------------------------------------------------------------------
RESULTS_TMP=$(mktemp)

# Semgrep ships a default `.semgrepignore` that excludes `*_test.go`, even
# when given `--include='*_test.go'`. To audit test files we temporarily
# override it: write an empty `.semgrepignore` to the repo for the duration
# of the scan, restoring any existing one on exit.
SEMGREPIGNORE="$REPO/.semgrepignore"
SEMGREPIGNORE_BACKUP=""
if [ -f "$SEMGREPIGNORE" ]; then
    SEMGREPIGNORE_BACKUP=$(mktemp)
    cp "$SEMGREPIGNORE" "$SEMGREPIGNORE_BACKUP"
fi
: > "$SEMGREPIGNORE"

cleanup() {
    rm -f "$RESULTS_TMP"
    if [ -n "$SEMGREPIGNORE_BACKUP" ]; then
        mv "$SEMGREPIGNORE_BACKUP" "$SEMGREPIGNORE"
    else
        rm -f "$SEMGREPIGNORE"
    fi
}
trap cleanup EXIT

echo "running semgrep on all .go files (this may take 10-30s on large providers)..." >&2

# Single scan over all .go files. We partition production-vs-test results in
# Python by checking whether the path ends in `_test.go`.
semgrep \
    --config "$RULES" \
    --json --quiet --no-git-ignore \
    --include='*.go' \
    --exclude='vendor/' \
    --exclude='.git/' \
    "$REPO" \
    > "$RESULTS_TMP" 2>/dev/null || {
        echo "ERROR: semgrep exited non-zero" >&2
        exit 1
}

PROVIDER_NAME=$(grep -E '^module ' "$REPO/go.mod" | awk '{print $2}' | sed 's|^.*/||')
SDK_VERSION=$(grep 'terraform-plugin-sdk/v2' "$REPO/go.mod" | head -n 1 | awk '{print $2}')
TODAY=$(date '+%Y-%m-%d')

# Count Go source files (excluding tests/vendor) for the summary.
TOTAL_FILES=$(find "$REPO" -type f -name '*.go' \
    ! -path "$REPO/vendor/*" \
    ! -path "$REPO/.git/*" \
    ! -name '*_test.go' \
    | wc -l | tr -d ' ')

TEST_FILES=$(find "$REPO" -type f -name '*_test.go' \
    ! -path "$REPO/vendor/*" \
    ! -path "$REPO/.git/*" \
    | wc -l | tr -d ' ')

# Aggregate results in Python.
python3 - "$RESULTS_TMP" "$REPO" "$MAX_FILES" "$PROVIDER_NAME" "$SDK_VERSION" "$TODAY" "$TOTAL_FILES" "$TEST_FILES" <<'PYTHON'
import json, os, sys
from collections import defaultdict

results_path, repo, max_files, provider, sdk_version, today, total_files, test_files = sys.argv[1:]
max_files = int(max_files)
total_files = int(total_files)
test_files = int(test_files)

with open(results_path) as f:
    data = json.load(f)

# Per-rule and per-file counts. Partition into production code vs test files
# based on the path suffix (semgrep doesn't reliably honour `--include='*_test.go'`
# alone, so we scan everything and partition here).
rule_totals = defaultdict(int)
per_file = defaultdict(lambda: defaultdict(int))
test_rule_totals = defaultdict(int)
test_per_file = defaultdict(lambda: defaultdict(int))

for r in data.get("results", []):
    rid = r["check_id"].split(".")[-1]
    rel = os.path.relpath(r["path"], repo) if r["path"].startswith(repo) else r["path"]
    if rel.endswith("_test.go"):
        test_rule_totals[rid] += 1
        test_per_file[rel][rid] += 1
    else:
        rule_totals[rid] += 1
        per_file[rel][rid] += 1

# Summary table — one row per rule, with the friendly name.
RULE_LABELS = {
    # -- Existing rules --
    "resources-map":                "ResourcesMap references",
    "data-sources-map":             "DataSourcesMap references",
    "force-new":                    "ForceNew: true",
    "validate-func":                "ValidateFunc / ValidateDiagFunc",
    "diff-suppress-func":           "DiffSuppressFunc",
    "customize-diff":               "CustomizeDiff",
    "state-func":                   "StateFunc",
    "sensitive":                    "Sensitive: true",
    "deprecated-attr":              "Deprecated attribute",
    "schema-default":               "Default: ... (defaults package, NOT PlanModifiers)",
    "cross-attr-constraint":        "ConflictsWith / ExactlyOneOf / AtLeastOneOf / RequiredWith",
    "set-hash-func":                "Set: hashFunc (drop in framework)",
    "migrate-state-legacy":         "MigrateState (legacy SDKv2 v1.x)",
    "importer":                     "Importer",
    "timeouts":                     "Timeouts",
    "state-upgraders":              "StateUpgraders",
    "schema-version":               "SchemaVersion",
    "max-items-1-nested-block":     "MaxItems:1 + nested Elem (block decision)",
    "nested-elem-resource":         "Nested Elem &Resource (any block)",
    "min-items-positive":           "MinItems > 0 (true repeating block)",
    # -- P0 additions: helper packages, CRUD shape, provider --
    "retry-state-change-conf":      "retry.StateChangeConf (no framework equivalent)",
    "retry-retry-context":          "retry.RetryContext",
    "customdiff-helper":            "helper/customdiff combinators",
    "helper-validation":            "helper/validation.* calls (replace with framework-validators)",
    "import-state-passthrough":     "schema.ImportStatePassthroughContext (trivial importer)",
    "crud-context-fields":          "CreateContext/ReadContext/UpdateContext/DeleteContext",
    "exists-callback":              "Exists callback (gone in framework)",
    "configure-context-func":       "Provider ConfigureContextFunc",
    "schema-provider-type":         "*schema.Provider type references",
    # -- P1 additions: CRUD bodies --
    "schema-resource-data":         "*schema.ResourceData function-param references",
    "resource-data-id":             "d.Id() / d.SetId() calls",
    "resource-data-change":         "d.HasChange / d.GetChange / d.IsNewResource / d.Partial",
    "diag-helpers":                 "diag.FromErr / diag.Errorf",
    "type-collection-primitive-elem": "TypeList/Set/Map of primitive (Elem &Schema{Type:})",
    # -- Test-only additions --
    "test-provider-factories":      "ProviderFactories: (test config — must become ProtoV6ProviderFactories)",
    "test-resource-test-helper":    "resource.Test/UnitTest/ParallelTest (must use terraform-plugin-testing)",
}

# Per-file complexity score, weighted to surface files that need most attention.
def score(counts):
    return (
        counts.get("force-new", 0)
        + counts.get("validate-func", 0) * 2
        + counts.get("state-upgraders", 0) * 5
        + counts.get("max-items-1-nested-block", 0) * 4
        + counts.get("nested-elem-resource", 0) * 2
        + counts.get("importer", 0) * 2
        + counts.get("customize-diff", 0) * 4
        + counts.get("state-func", 0) * 3
        + counts.get("diff-suppress-func", 0) * 2
        # New weights — high-leverage indicators of migration cost
        + counts.get("retry-state-change-conf", 0) * 3
        + counts.get("customdiff-helper", 0) * 3
        + counts.get("crud-context-fields", 0)            # 4 per resource normally; weight=1
        + counts.get("schema-resource-data", 0)            # CRUD-method count
        + counts.get("helper-validation", 0)               # validator replacement count
    )

ranked = sorted(per_file.items(), key=lambda x: -score(x[1]))[:max_files]

# Identify "needs manual review" files — those with patterns that always need
# human/LLM judgment regardless of how reliably the audit detected them.
nmr_signals = {
    "max-items-1-nested-block": "MaxItems:1 (block-vs-nested-attribute decision)",
    "state-upgraders":          "StateUpgraders/SchemaVersion (single-step semantics)",
    "migrate-state-legacy":     "MigrateState (upgrade to StateUpgraders first)",
    "importer":                 "custom Importer (composite ID parsing?)",
    "timeouts":                 "Timeouts (separate framework package)",
    "customize-diff":           "CustomizeDiff (becomes ModifyPlan)",
    "state-func":               "StateFunc (becomes custom type)",
    "diff-suppress-func":       "DiffSuppressFunc (analyse intent)",
    "nested-elem-resource":     "nested Elem &Resource (block-vs-nested decision)",
    "cross-attr-constraint":    "ConflictsWith/ExactlyOneOf/etc. (validator routing decision)",
    "retry-state-change-conf":  "retry.StateChangeConf (replace with inline ticker loop)",
    "customdiff-helper":        "customdiff helper combinators (refactor into ModifyPlan)",
    "exists-callback":          "Exists callback (gone — use RemoveResource in Read)",
    "configure-context-func":   "Provider ConfigureContextFunc (provider-level migration)",
}

needs_review = []
for path, counts in per_file.items():
    reasons = [label for rid, label in nmr_signals.items() if counts.get(rid, 0) > 0]
    if reasons:
        needs_review.append((path, reasons))
needs_review.sort()

# Group rule labels into report sections for cleaner output.
SECTIONS = [
    ("Schema-level fields", ["force-new", "validate-func", "diff-suppress-func", "customize-diff", "state-func", "sensitive", "deprecated-attr", "schema-default", "cross-attr-constraint", "set-hash-func"]),
    ("Resource-level fields", ["importer", "import-state-passthrough", "timeouts", "state-upgraders", "schema-version", "migrate-state-legacy", "exists-callback"]),
    ("Block / nested-attribute decisions", ["max-items-1-nested-block", "nested-elem-resource", "min-items-positive", "type-collection-primitive-elem"]),
    ("Helper packages (need replacement)", ["retry-state-change-conf", "retry-retry-context", "customdiff-helper", "helper-validation"]),
    ("CRUD-body shape", ["crud-context-fields", "schema-resource-data", "resource-data-id", "resource-data-change", "diag-helpers"]),
    ("Provider-level wiring", ["resources-map", "data-sources-map", "configure-context-func", "schema-provider-type"]),
]
TEST_RULES = ["test-provider-factories", "test-resource-test-helper", "helper-validation", "diag-helpers", "schema-resource-data", "resource-data-id"]

# Emit report.
print(f"# SDKv2 → Framework Migration Audit\n")
print(f"**Provider:** {provider}    **Audited:** {today}    **SDKv2 version:** {sdk_version}\n")
print("## Summary\n")
print(f"- Production Go files audited: {total_files}")
print(f"- Test Go files audited: {test_files}")
print()
for section_name, rule_ids in SECTIONS:
    section_total = sum(rule_totals.get(rid, 0) for rid in rule_ids)
    if section_total == 0:
        continue
    print(f"### {section_name}")
    for rid in rule_ids:
        if rid in RULE_LABELS:
            count = rule_totals.get(rid, 0)
            if count > 0:
                print(f"- {RULE_LABELS[rid]}: **{count}**")
    print()

print(f"## Per-file findings (top {max_files} by complexity, production code)\n")
print("| File | ForceNew | Validators | StateUpgraders | MaxItems:1 | NestedElem | Importer | CustomizeDiff | StateFunc | retry.SCC | customdiff | CRUD ctxs |")
print("|------|---------:|-----------:|---------------:|-----------:|-----------:|---------:|--------------:|----------:|----------:|-----------:|----------:|")
for path, counts in ranked:
    print(f"| {path} | {counts.get('force-new', 0)} | {counts.get('validate-func', 0) + counts.get('helper-validation', 0)} | {counts.get('state-upgraders', 0)} | {counts.get('max-items-1-nested-block', 0)} | {counts.get('nested-elem-resource', 0)} | {counts.get('importer', 0)} | {counts.get('customize-diff', 0)} | {counts.get('state-func', 0)} | {counts.get('retry-state-change-conf', 0)} | {counts.get('customdiff-helper', 0)} | {counts.get('crud-context-fields', 0)} |")
print()

print("## Needs manual review\n")
print("Read these files directly. Even with semgrep's AST-aware matching, the *decision* (block vs nested attribute, single-step state upgrade, composite-ID importer parsing, customdiff structure) requires human/LLM judgment.\n")
if needs_review:
    for path, reasons in needs_review:
        print(f"- {path} — " + "; ".join(reasons))
else:
    print("_None — the provider has no patterns that require special migration handling._")
print()

# -- Test-file findings --
print("## Test-file findings\n")
if test_files == 0:
    print("_No `*_test.go` files in the repo._\n")
else:
    test_total = sum(test_rule_totals.values())
    if test_total == 0:
        print(f"_Scanned {test_files} test files; no migration-relevant patterns detected._\n")
    else:
        print(f"Scanned {test_files} test files. Test migration is part of step 7 (TDD gate) — these patterns must be translated alongside the production-code migration:\n")
        for rid in TEST_RULES:
            count = test_rule_totals.get(rid, 0)
            if count > 0 and rid in RULE_LABELS:
                print(f"- {RULE_LABELS[rid]}: **{count}**")
        # Top test files by total findings.
        ranked_tests = sorted(test_per_file.items(), key=lambda x: -sum(x[1].values()))[:10]
        if ranked_tests:
            print()
            print("Top 10 test files by SDKv2-pattern count:")
            for path, counts in ranked_tests:
                total = sum(counts.values())
                print(f"  - {path}: {total} patterns")
        print()

print("## Next steps\n")
print("1. Read every file listed under 'Needs manual review' before proposing edits.")
print("2. Populate `assets/checklist_template.md` from this audit (one entry per resource).")
print("3. Confirm scope with the user before starting workflow step 1.")
print("4. For test files: factor in `ProviderFactories: → ProtoV6ProviderFactories` and `helper/resource → terraform-plugin-testing/helper/resource` swaps when sizing step 7 (TDD gate).")
PYTHON
