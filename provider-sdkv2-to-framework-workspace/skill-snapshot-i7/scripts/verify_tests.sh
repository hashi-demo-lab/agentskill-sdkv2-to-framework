#!/bin/sh
# verify_tests.sh — Layered verification gate for SDKv2 → framework migrations.
#
# Designed to recover signal even without TF_ACC: it does NOT require live
# cloud creds. The negative gate (--migrated-files) closes the
# "all-green-on-an-unmigrated-tree" loophole — if you pass a list of files
# that should be migrated and any of them still import terraform-plugin-sdk/v2,
# the check fails.
#
# Usage:
#   verify_tests.sh <provider-repo-path> --migrated-files "f1.go f2.go ..." [--with-acc]
#   verify_tests.sh <provider-repo-path> --no-migrated-files [--with-acc]   (only for fresh-baseline runs)
#
# --migrated-files is REQUIRED when validating a migration. The flag drives the
# negative gate; without it, a no-op run that touched zero files would pass all
# other gates and look successful. If you really need to run the static layers
# without a migration in progress (e.g., establishing the baseline before step 1),
# pass --no-migrated-files explicitly to acknowledge the negative gate is being
# skipped on purpose.
#
# Layers (each must pass before the next):
#   1. go build ./...
#   2. go vet ./...
#   3. ^TestProvider$ (provider construction + InternalValidate-equivalent)
#   4. Non-TestAcc unit tests
#   5. Negative gate (only if --migrated-files supplied)
#   6. Optional: TF_ACC=1 (only if --with-acc)
#
# Exits 0 if all run gates pass, 1 otherwise. Layer that failed is reported on stderr.

set -eu

if [ "$#" -lt 1 ]; then
    echo "usage: $0 <provider-repo-path> [--migrated-files \"f1.go f2.go\"] [--with-acc]" >&2
    exit 1
fi

REPO="$1"; shift || true
MIGRATED_FILES=""
NO_MIGRATED_FILES=0
WITH_ACC=0

while [ "$#" -gt 0 ]; do
    case "$1" in
        --migrated-files)    MIGRATED_FILES="$2"; shift 2 ;;
        --no-migrated-files) NO_MIGRATED_FILES=1; shift ;;
        --with-acc)          WITH_ACC=1; shift ;;
        *) echo "unknown arg: $1" >&2; exit 1 ;;
    esac
done

if [ -z "$MIGRATED_FILES" ] && [ "$NO_MIGRATED_FILES" -ne 1 ]; then
    cat >&2 <<'EOF'
ERROR: --migrated-files is required when validating a migration.

Without it, a no-op run that touched zero files would pass all gates and look
like a successful migration — that is the exact failure mode the negative gate
exists to prevent.

Pass it as a single space-separated argument:
  verify_tests.sh <repo> --migrated-files "internal/provider/resource_foo.go internal/provider/resource_bar.go"

If you intentionally want to run the static layers without a migration in
progress (rare; e.g., establishing the SDKv2 baseline before step 1), pass
--no-migrated-files to acknowledge that the negative gate is skipped on purpose.
EOF
    exit 2
fi

if [ ! -d "$REPO" ]; then
    echo "not a directory: $REPO" >&2
    exit 1
fi

cd "$REPO"

step() { printf '\n=== %s ===\n' "$1" >&2; }
fail() { printf '\n!!! FAILED: %s\n' "$1" >&2; exit 1; }

step "1/6 go build ./..."
go build ./... || fail "go build"

step "2/6 go vet ./..."
go vet ./... || fail "go vet"

step "3/6 TestProvider (provider boots, schema is internally valid)"
# Ignore failures here only when no TestProvider exists at all — otherwise propagate.
if grep -rqE '^func TestProvider\(' --include='*_test.go' . 2>/dev/null; then
    go test -run '^TestProvider$' ./... || fail "TestProvider"
else
    echo "(no TestProvider found — consider adding one; see references/testing.md)" >&2
fi

step "4/6 Non-TestAcc unit tests"
# -run 'Test[^A]' excludes TestAcc* names. Some packages may have no matching tests, that's fine.
go test -run '^Test[^A]' -count=1 ./... || fail "non-TestAcc unit tests"

if [ -n "$MIGRATED_FILES" ]; then
    step "5/6 Negative gate: no terraform-plugin-sdk/v2 imports remain in migrated files"
    leak=""
    for f in $MIGRATED_FILES; do
        if [ -f "$f" ] && grep -qE 'terraform-plugin-sdk/v2' "$f"; then
            leak="$leak $f"
        fi
    done
    if [ -n "$leak" ]; then
        echo "files still importing terraform-plugin-sdk/v2:" >&2
        for f in $leak; do echo "  $f" >&2; done
        fail "migrated files still import sdk/v2"
    fi
    echo "(all $(echo "$MIGRATED_FILES" | wc -w | tr -d ' ') migrated files clean)" >&2
else
    step "5/6 Negative gate skipped — explicit --no-migrated-files acknowledged"
fi

if [ "$WITH_ACC" -eq 1 ]; then
    step "6/6 TF_ACC=1 acceptance tests"
    TF_ACC=1 go test -count=1 ./... || fail "acceptance tests"
else
    step "6/6 Acceptance tests skipped (--with-acc not set)"
fi

printf '\nALL GATES PASSED\n' >&2
