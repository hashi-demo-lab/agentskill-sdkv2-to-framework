#!/bin/sh
# lint_skill.sh — Generic-skill leakage gate.
#
# This skill must be provider-agnostic. Eval prompts in evals/ may legitimately
# reference specific provider names (e.g., openstack as the test fixture), but
# nothing in SKILL.md, references/, scripts/, or assets/ should mention them.
#
# Exit codes:
#   0 — clean (no provider-specific names leaked)
#   1 — one or more matches found

set -eu

# Allow running from anywhere — derive skill root from this script's location.
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
SKILL_ROOT=$(dirname "$SCRIPT_DIR")

cd "$SKILL_ROOT"

# The skill must be provider-agnostic. The pattern below is two parts:
#   1. specific provider/SDK names from our chosen eval fixture (openstack +
#      its OpenStack-API package names). These must never leak into the skill
#      proper because they're the test-fixture detail.
#   2. the generic `terraform-provider-*` prefix that catches any other
#      provider name a maintainer might accidentally drop in.
# Eval prompts in evals/ legitimately mention openstack, so we --exclude-dir=evals.
# If you swap fixtures (different test provider), update the part-1 names too.
PATTERN='(openstack|gophercloud|keystone|nova|cinder|neutron|terraform-provider-[a-z][a-z0-9-]*)'

# --exclude-dir=evals so legitimate eval-prompt mentions don't fail the lint.
# Also exclude the lint script itself, which lists the forbidden patterns.
if grep -riE --exclude-dir=evals --exclude='lint_skill.sh' "$PATTERN" . ; then
    echo "" >&2
    echo "FAIL: generic-skill content mentions provider-specific names." >&2
    echo "The skill must be provider-agnostic — only evals/ may reference specific providers." >&2
    exit 1
fi

echo "OK: skill content is provider-agnostic." >&2
exit 0
