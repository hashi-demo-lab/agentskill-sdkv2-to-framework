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
    # -- Gap-analysis P0 additions --
    "resource-data-get":            "d.Get / d.GetOk / d.GetOkExists calls",
    "resource-data-set":            "d.Set calls",
    "env-default-func":             "schema.EnvDefaultFunc / MultiEnvDefaultFunc (provider-config)",
    "schema-default-timeout":       "schema.DefaultTimeout / d.Timeout (timeouts)",
    # -- Gap-analysis P1 additions --
    "resource-constructor":         "Resource constructor (count = resources to migrate)",
    "resource-diff-signature":      "*schema.ResourceDiff function (port to ModifyPlan body)",
    "schema-set-cast":              "Inline *schema.Set cast from d.Get",
    "helper-acctest":               "helper/acctest test utilities",
    "helper-structure":             "helper/structure JSON normalisation helpers",
    # -- Test-only additions --
    "test-provider-factories":      "ProviderFactories: (test config — must become ProtoV6ProviderFactories)",
    "test-resource-test-helper":    "resource.Test/UnitTest/ParallelTest (must use terraform-plugin-testing)",
    "test-providers-field":         "Providers: (older test field — pre-SDKv2.5)",
    "test-pre-check":               "PreCheck: (test pre-check, often references *schema.Provider plumbing)",
    # -- Step-2 data-consistency rules (the most-skipped step's detectors) --
    "optional-computed-without-usestateforunknown": "Optional+Computed without UseStateForUnknown (plan-noise / spurious replacement)",
    "default-on-non-computed":      "Default: without Computed:true (framework rejects at boot)",
    "force-new-on-computed":        "ForceNew on pure-Computed attribute (framework rejects at boot)",
    "sensitive-statefunc-hash-placeholder": "Sensitive + StateFunc (hash-placeholder — candidate WriteOnly migration)",
    "typelist-maxitems1-without-elem": "TypeList + MaxItems:1 without Elem (malformed schema)",
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
        # Helper-package weights
        + counts.get("retry-state-change-conf", 0) * 3
        + counts.get("customdiff-helper", 0) * 3
        + counts.get("crud-context-fields", 0)
        + counts.get("schema-resource-data", 0)
        + counts.get("helper-validation", 0)
        # Gap-analysis additions — d.Get/d.Set are very high-volume but each
        # individual hit is trivial-translation work. Weight 0.25 keeps them
        # contributing a useful bias without dominating files like provider.go.
        + counts.get("resource-data-get", 0) * 0.25
        + counts.get("resource-data-set", 0) * 0.25
        + counts.get("env-default-func", 0) * 2
        + counts.get("schema-default-timeout", 0) * 1
        + counts.get("resource-diff-signature", 0) * 4
        + counts.get("helper-structure", 0) * 2
        # Step-2 data-consistency signals get high weight — they're cheap to fix
        # in SDKv2 and turn into hard framework errors after migration.
        + counts.get("default-on-non-computed", 0) * 3
        + counts.get("force-new-on-computed", 0) * 3
        + counts.get("typelist-maxitems1-without-elem", 0) * 3
        + counts.get("optional-computed-without-usestateforunknown", 0) * 1
        + counts.get("sensitive-statefunc-hash-placeholder", 0) * 2
    )

def score_breakdown(counts):
    """Return list of (rule_id, contribution) tuples sorted by contribution desc.
    Used to explain why a file ranks where it does."""
    weights = {
        "force-new": 1, "validate-func": 2, "state-upgraders": 5,
        "max-items-1-nested-block": 4, "nested-elem-resource": 2,
        "importer": 2, "customize-diff": 4, "state-func": 3,
        "diff-suppress-func": 2, "retry-state-change-conf": 3,
        "customdiff-helper": 3, "crud-context-fields": 1,
        "schema-resource-data": 1, "helper-validation": 1,
        "resource-data-get": 0.25, "resource-data-set": 0.25,
        "env-default-func": 2, "schema-default-timeout": 1,
        "resource-diff-signature": 4, "helper-structure": 2,
        "default-on-non-computed": 3, "force-new-on-computed": 3,
        "typelist-maxitems1-without-elem": 3,
        "optional-computed-without-usestateforunknown": 1,
        "sensitive-statefunc-hash-placeholder": 2,
    }
    contribs = [(rid, counts.get(rid, 0) * w) for rid, w in weights.items()
                if counts.get(rid, 0) > 0]
    return sorted(contribs, key=lambda x: -x[1])

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
    "resource-diff-signature":  "*schema.ResourceDiff function (port to ModifyPlan)",
    "helper-structure":         "helper/structure JSON normalisation (refactor to custom type or plan modifier)",
    "schema-set-cast":          "*schema.Set cast (TypeSet expansion → typed model)",
    # Step-2 data-consistency signals — fix in SDKv2 form before migrating.
    "optional-computed-without-usestateforunknown": "Optional+Computed without UseStateForUnknown (carry plan modifier across)",
    "default-on-non-computed":  "Default without Computed (framework rejects at boot — add Computed in SDKv2 first)",
    "force-new-on-computed":    "ForceNew + Computed (framework rejects at boot — fix in SDKv2 first)",
    "sensitive-statefunc-hash-placeholder": "Sensitive + StateFunc hash-placeholder (migrate to WriteOnly)",
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
    ("Step 2 — data-consistency (fix in SDKv2 first)", ["optional-computed-without-usestateforunknown", "default-on-non-computed", "force-new-on-computed", "typelist-maxitems1-without-elem", "sensitive-statefunc-hash-placeholder"]),
    ("Helper packages (need replacement)", ["retry-state-change-conf", "retry-retry-context", "customdiff-helper", "helper-validation", "helper-structure", "helper-acctest"]),
    ("CRUD-body shape", ["crud-context-fields", "schema-resource-data", "resource-constructor", "resource-diff-signature", "resource-data-id", "resource-data-get", "resource-data-set", "resource-data-change", "schema-set-cast", "diag-helpers", "schema-default-timeout"]),
    ("Provider-level wiring", ["resources-map", "data-sources-map", "configure-context-func", "schema-provider-type", "env-default-func"]),
]
PROVIDER_LEVEL_RULES = ["resources-map", "data-sources-map", "configure-context-func", "schema-provider-type", "env-default-func"]
TEST_RULES = [
    "test-provider-factories", "test-resource-test-helper", "test-providers-field", "test-pre-check",
    "helper-validation", "helper-acctest", "diag-helpers", "schema-resource-data",
    "resource-data-id", "resource-data-get", "resource-data-set", "resource-data-change",
]

# Emit report.
print(f"# SDKv2 → Framework Migration Audit\n")
print(f"**Provider:** {provider}    **Audited:** {today}    **SDKv2 version:** {sdk_version}\n")
print("## Summary\n")
print(f"- Production Go files audited: {total_files}")
print(f"- Test Go files audited: {test_files}")
print()

# Resource-count rollup — derived from the resource-constructor rule.
resource_ctor_count = rule_totals.get("resource-constructor", 0)
if resource_ctor_count > 0:
    print(f"- **Resource/data-source constructors detected: {resource_ctor_count}** (each is a `func ...() *schema.Resource` — direct migration count)")
    print()

# Provider-level migration cost rollup.
provider_total = sum(rule_totals.get(rid, 0) for rid in PROVIDER_LEVEL_RULES)
if provider_total > 0:
    print("### Provider-level migration cost\n")
    print("These patterns indicate work in `provider.go` / Configure path, separate from per-resource cost. The framework provider type and Configure method must be set up before any resource migration can be tested.\n")
    for rid in PROVIDER_LEVEL_RULES:
        if rid in RULE_LABELS and rule_totals.get(rid, 0) > 0:
            print(f"- {RULE_LABELS[rid]}: **{rule_totals.get(rid, 0)}**")
    print()

for section_name, rule_ids in SECTIONS:
    if section_name == "Provider-level wiring":
        continue  # Already shown above as "Provider-level migration cost".
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
print("| File | ForceNew | Validators | StateUpgr | MaxIt:1 | Imptr | CustDiff | StateFunc | retry.SCC | custdiff | CRUDctx | d.Get | d.Set |")
print("|------|---------:|-----------:|----------:|--------:|------:|---------:|----------:|----------:|---------:|--------:|------:|------:|")
for path, counts in ranked:
    print(f"| {path} | {counts.get('force-new', 0)} | {counts.get('validate-func', 0) + counts.get('helper-validation', 0)} | {counts.get('state-upgraders', 0)} | {counts.get('max-items-1-nested-block', 0)} | {counts.get('importer', 0)} | {counts.get('customize-diff', 0)} | {counts.get('state-func', 0)} | {counts.get('retry-state-change-conf', 0)} | {counts.get('customdiff-helper', 0)} | {counts.get('crud-context-fields', 0)} | {counts.get('resource-data-get', 0)} | {counts.get('resource-data-set', 0)} |")
print()

# Per-file score breakdown for the top 5, so the migrator can see why a file ranked high.
print("### Score breakdown for top 5 files\n")
for path, counts in ranked[:5]:
    breakdown = score_breakdown(counts)
    if breakdown:
        bits = ", ".join(f"{rid}×{counts.get(rid,0)}={contrib:g}" for rid, contrib in breakdown[:6])
        print(f"- `{path}` (score {score(counts):g}): {bits}")
print()

# Cross-rule correlations — files hitting multiple judgment-rich patterns.
correlations = []
for path, counts in per_file.items():
    flags = []
    if counts.get("state-upgraders", 0) > 0 and counts.get("importer", 0) > 0:
        flags.append("state-upgrade + composite-ID importer")
    if counts.get("customize-diff", 0) > 0 and counts.get("customdiff-helper", 0) > 0:
        flags.append("CustomizeDiff with customdiff combinators (multi-leg ModifyPlan)")
    if counts.get("max-items-1-nested-block", 0) > 0 and counts.get("nested-elem-resource", 0) > 2:
        flags.append("MaxItems:1 + many nested blocks (deep block-vs-attribute decision tree)")
    if counts.get("retry-state-change-conf", 0) > 0 and counts.get("timeouts", 0) > 0:
        flags.append("retry.StateChangeConf + Timeouts (full async state-change refactor)")
    if counts.get("state-func", 0) > 0 and counts.get("diff-suppress-func", 0) > 0:
        flags.append("StateFunc + DiffSuppressFunc (custom-type with normalisation — destructive-type trap)")
    if flags:
        correlations.append((path, flags))
correlations.sort()

if correlations:
    print("### Cross-rule correlations (files combining judgment-rich patterns)\n")
    print("Files hitting multiple high-judgment patterns at once. Read both/all references *before* editing.\n")
    for path, flags in correlations[:15]:
        print(f"- `{path}`:")
        for f in flags:
            print(f"  - {f}")
    print()

print("## Needs manual review\n")
print("Read these files directly. Even with semgrep's AST-aware matching, the *decision* (block vs nested attribute, single-step state upgrade, composite-ID importer parsing, customdiff structure) requires human/LLM judgment.\n")
if needs_review:
    for path, reasons in needs_review:
        print(f"- {path} — " + "; ".join(reasons))
else:
    print("_None — the provider has no patterns that require special migration handling._")
print()

# -- Shared test infrastructure detection --
# Files that hold provider-level test plumbing (TestAccProvider, factories,
# Meta-derived helpers). Per-resource test migration depends on these.
# Heuristic: path-based — common conventions across providers. We walk the
# filesystem (not the per-file rule hits) because shared-infra files often
# define types/vars that semgrep can't reliably match in standalone-type
# position (e.g. `var testAccProvider *schema.Provider` doesn't match the
# `*$PKG.Provider` pattern without surrounding context).
import re
SHARED_TEST_PATH_PATTERNS = [
    (re.compile(r'(^|/)acceptance/[^/]+\.go$'),    "acceptance/ dir"),
    (re.compile(r'(^|/)testutil/[^/]+\.go$'),      "testutil/ dir"),
    (re.compile(r'(^|/)internal/test/[^/]+\.go$'), "internal/test/ dir"),
    (re.compile(r'(^|/)helper/test[^/]*\.go$'),    "helper/test*.go"),
    (re.compile(r'(^|/)provider_test\.go$'),       "provider_test.go"),
    (re.compile(r'(^|/)acceptance_test\.go$'),     "acceptance_test.go"),
    (re.compile(r'(^|/)common_test\.go$'),         "common_test.go"),
    (re.compile(r'(^|/)test_helpers?\.go$'),       "test_helpers*.go"),
    (re.compile(r'(^|/)testing/[^/]+\.go$'),       "testing/ dir"),
]
def is_shared_test_infra(path):
    return any(p.search(path) for p, _ in SHARED_TEST_PATH_PATTERNS)
def shared_infra_reason(path):
    for p, label in SHARED_TEST_PATH_PATTERNS:
        if p.search(path):
            return label
    return ""

# Walk the repo to find every candidate shared-infra file, regardless of
# whether any rule fired in it. This is the authoritative list.
shared_infra_paths = []
for root, dirs, files in os.walk(repo):
    # Skip vendor/ and .git/.
    dirs[:] = [d for d in dirs if d not in ('vendor', '.git')]
    for fn in files:
        if not fn.endswith('.go'):
            continue
        full = os.path.join(root, fn)
        rel = os.path.relpath(full, repo)
        if is_shared_test_infra(rel):
            shared_infra_paths.append(rel)

# Decorate with rule-hit counts (zero if the file had no findings).
shared_infra = []
for path in shared_infra_paths:
    counts = dict(per_file.get(path, {}))
    counts.update(dict(test_per_file.get(path, {})))
    total = sum(counts.values())
    shared_infra.append((path, total, counts))
shared_infra.sort(key=lambda x: -x[1])

# -- Test-file findings --
print("## Test-file findings\n")
if test_files == 0:
    print("_No `*_test.go` files in the repo._\n")
else:
    test_total = sum(test_rule_totals.values())
    if test_total == 0 and not shared_infra:
        print(f"_Scanned {test_files} test files; no migration-relevant patterns detected._\n")
    else:
        print(f"Scanned {test_files} test files. Test migration is a **provider-level prerequisite** — per-resource test rewrites (workflow step 7) cannot succeed until shared test plumbing has a framework path. Plan this work *before* touching per-resource tests.\n")
        if test_total > 0:
            for rid in TEST_RULES:
                count = test_rule_totals.get(rid, 0)
                if count > 0 and rid in RULE_LABELS:
                    print(f"- {RULE_LABELS[rid]}: **{count}**")
            print()

        # Shared test infrastructure (provider-level prerequisites).
        if shared_infra:
            print("### Shared test infrastructure (migrate first — per-resource tests depend on these)\n")
            print("Files matching test-infra path conventions (acceptance/, testutil/, provider_test.go, etc.). Every migrated test file references something here; flipping ProviderFactories per resource is wasted effort if the factory isn't framework-aware yet.\n")
            for path, total, counts in shared_infra[:15]:
                non_zero = [f"{rid}={n}" for rid, n in sorted(counts.items()) if n > 0]
                reason = shared_infra_reason(path)
                hits_str = ', '.join(non_zero) if non_zero else 'no audit-rule hits'
                print(f"- `{path}` [{reason}] — {hits_str}")
            print()

        # Top per-resource test files by total findings.
        ranked_tests = sorted(test_per_file.items(), key=lambda x: -sum(x[1].values()))
        # Drop shared-infra files from the per-resource list so the ranking is honest.
        ranked_tests = [(p, c) for p, c in ranked_tests if not is_shared_test_infra(p)][:10]
        if ranked_tests:
            print("### Top 10 per-resource test files by SDKv2-pattern count\n")
            for path, counts in ranked_tests:
                total = sum(counts.values())
                print(f"- `{path}`: {total} patterns")
            print()

print("## Next steps\n")
print("1. Read every file listed under 'Needs manual review' before proposing edits.")
print("2. Populate `assets/checklist_template.md` from this audit (one entry per resource).")
print("3. Confirm scope with the user before starting workflow step 1.")
print("4. For test files: factor in `ProviderFactories: → ProtoV6ProviderFactories` and `helper/resource → terraform-plugin-testing/helper/resource` swaps when sizing step 7 (TDD gate).")
PYTHON
