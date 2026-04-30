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
trap 'rm -f "$RESULTS_TMP"' EXIT

echo "running semgrep (this may take 10-30s on large providers)..." >&2

semgrep \
    --config "$RULES" \
    --json --quiet --no-git-ignore \
    --include='*.go' \
    --exclude='*_test.go' \
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

# Aggregate results in Python.
python3 - "$RESULTS_TMP" "$REPO" "$MAX_FILES" "$PROVIDER_NAME" "$SDK_VERSION" "$TODAY" "$TOTAL_FILES" <<'PYTHON'
import json, os, sys
from collections import defaultdict

results_path, repo, max_files, provider, sdk_version, today, total_files = sys.argv[1:]
max_files = int(max_files)
total_files = int(total_files)

with open(results_path) as f:
    data = json.load(f)

# Per-rule and per-file counts.
rule_totals = defaultdict(int)
per_file = defaultdict(lambda: defaultdict(int))

for r in data.get("results", []):
    # check_id includes the full path of the rules file; take the last segment.
    rid = r["check_id"].split(".")[-1]
    # Make the path relative to the repo for cleaner output.
    rel = os.path.relpath(r["path"], repo) if r["path"].startswith(repo) else r["path"]
    rule_totals[rid] += 1
    per_file[rel][rid] += 1

# Summary table — one row per rule, with the friendly name.
RULE_LABELS = {
    "resources-map":                "ResourcesMap references",
    "data-sources-map":             "DataSourcesMap references",
    "force-new":                    "ForceNew: true",
    "validate-func":                "ValidateFunc / ValidateDiagFunc",
    "diff-suppress-func":           "DiffSuppressFunc",
    "customize-diff":               "CustomizeDiff",
    "state-func":                   "StateFunc",
    "sensitive":                    "Sensitive: true",
    "deprecated-attr":              "Deprecated attribute",
    "importer":                     "Importer",
    "timeouts":                     "Timeouts",
    "state-upgraders":              "StateUpgraders",
    "schema-version":               "SchemaVersion",
    "max-items-1-nested-block":     "MaxItems:1 + nested Elem (block decision)",
    "nested-elem-resource":         "Nested Elem &Resource (any block)",
    "min-items-positive":           "MinItems > 0 (true repeating block)",
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
    )

ranked = sorted(per_file.items(), key=lambda x: -score(x[1]))[:max_files]

# Identify "needs manual review" files — those with patterns that always need
# human/LLM judgment regardless of how reliably the audit detected them.
nmr_signals = {
    "max-items-1-nested-block": "MaxItems:1 (block-vs-nested-attribute decision)",
    "state-upgraders":          "StateUpgraders/SchemaVersion (single-step semantics)",
    "importer":                 "custom Importer (composite ID parsing?)",
    "timeouts":                 "Timeouts (separate framework package)",
    "customize-diff":           "CustomizeDiff (becomes ModifyPlan)",
    "state-func":               "StateFunc (becomes custom type)",
    "diff-suppress-func":       "DiffSuppressFunc (analyse intent)",
    "nested-elem-resource":     "nested Elem &Resource (block-vs-nested decision)",
}

needs_review = []
for path, counts in per_file.items():
    reasons = [label for rid, label in nmr_signals.items() if counts.get(rid, 0) > 0]
    if reasons:
        needs_review.append((path, reasons))
needs_review.sort()

# Emit report.
print(f"# SDKv2 → Framework Migration Audit\n")
print(f"**Provider:** {provider}    **Audited:** {today}    **SDKv2 version:** {sdk_version}\n")
print("## Summary\n")
print(f"- Files audited: {total_files}")
for rid, label in RULE_LABELS.items():
    print(f"- {label}: **{rule_totals.get(rid, 0)}**")
print()

print(f"## Per-file findings (top {max_files} by complexity)\n")
print("| File | ForceNew | Validators | StateUpgraders | MaxItems:1 (block) | NestedElem | Importer | CustomizeDiff | StateFunc |")
print("|------|---------:|-----------:|---------------:|-------------------:|-----------:|---------:|--------------:|----------:|")
for path, counts in ranked:
    print(f"| {path} | {counts.get('force-new', 0)} | {counts.get('validate-func', 0)} | {counts.get('state-upgraders', 0)} | {counts.get('max-items-1-nested-block', 0)} | {counts.get('nested-elem-resource', 0)} | {counts.get('importer', 0)} | {counts.get('customize-diff', 0)} | {counts.get('state-func', 0)} |")
print()

print("## Needs manual review\n")
print("Read these files directly. Even with semgrep's AST-aware matching, the *decision* (block vs nested attribute, single-step state upgrade, composite-ID importer parsing) requires human/LLM judgment.\n")
if needs_review:
    for path, reasons in needs_review:
        print(f"- {path} — " + "; ".join(reasons))
else:
    print("_None — the provider has no patterns that require special migration handling._")
print()

print("## Next steps\n")
print("1. Read every file listed under 'Needs manual review' before proposing edits.")
print("2. Populate `assets/checklist_template.md` from this audit (one entry per resource).")
print("3. Confirm scope with the user before starting workflow step 1.")
PYTHON
