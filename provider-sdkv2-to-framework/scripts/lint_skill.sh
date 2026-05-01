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

# -----------------------------------------------------------------------------
# YAML frontmatter parse check.
#
# GitHub's strict YAML parser rejects unquoted scalars containing `: ` (colon
# followed by space) because it treats them as nested mappings — the SKILL.md
# description has historically been long-form prose, which collides with this
# rule (e.g., "Does NOT cover: SDK v1 ..."). Validate the frontmatter parses
# under PyYAML's safe_load before shipping.
# -----------------------------------------------------------------------------
if ! command -v python3 >/dev/null 2>&1; then
    echo "WARN: python3 not on PATH; skipping YAML frontmatter check." >&2
    exit 0
fi

python3 - <<'PY' "$SKILL_ROOT/SKILL.md" "$SKILL_ROOT"
import re, sys, os
from pathlib import Path
try:
    import yaml
except ImportError:
    print("WARN: PyYAML not installed; skipping frontmatter check.", file=sys.stderr)
    sys.exit(0)

skill_md_path, skill_root = sys.argv[1], sys.argv[2]
text = open(skill_md_path).read()

# 1. Frontmatter parses + required keys present.
m = re.match(r"^---\n(.*?)\n---", text, re.S)
if not m:
    print(f"FAIL: SKILL.md missing YAML frontmatter delimited by ---", file=sys.stderr)
    sys.exit(1)
try:
    fm = yaml.safe_load(m.group(1))
except yaml.YAMLError as e:
    print(f"FAIL: SKILL.md YAML frontmatter doesn't parse: {e}", file=sys.stderr)
    sys.exit(1)
if not isinstance(fm, dict) or "name" not in fm or "description" not in fm:
    print(f"FAIL: SKILL.md frontmatter missing required keys (name, description). Got: {list(fm) if isinstance(fm, dict) else type(fm).__name__}", file=sys.stderr)
    sys.exit(1)

# 2. Description fits Anthropic's 1024-char hard cap.
DESC_MAX = 1024
desc_len = len(fm["description"])
if desc_len > DESC_MAX:
    print(f"FAIL: SKILL.md description is {desc_len} chars (max {DESC_MAX}).", file=sys.stderr)
    sys.exit(1)

print(f"OK: SKILL.md frontmatter parses (name={fm['name']!r}, description={desc_len} chars).", file=sys.stderr)

# 3. Cross-links from SKILL.md to references/*.md and scripts/*.{sh,yml} resolve.
#    Catches typos before they ship — "see references/blocks.md" pointing at a
#    file that was renamed or never existed is a load-bearing failure once the
#    skill ships, since Claude won't get a graceful "not found" — it'll just
#    proceed without the referenced material. References to assets/ also count.
REF_PATTERNS = [
    re.compile(r'`?references/([\w\-]+\.md)`?'),
    re.compile(r'`?scripts/([\w\-]+\.(?:sh|yml|yaml|py))`?'),
    re.compile(r'`?assets/([\w\-]+\.md)`?'),
]
SUBDIR_BY_PATTERN = ["references", "scripts", "assets"]

# Body only — frontmatter description may contain phrases that look like paths.
body = text[m.end():]
broken = []
for sub, pat in zip(SUBDIR_BY_PATTERN, REF_PATTERNS):
    for fname in sorted(set(pat.findall(body))):
        target = Path(skill_root) / sub / fname
        if not target.exists():
            broken.append(f"{sub}/{fname}")

if broken:
    print(f"FAIL: SKILL.md mentions {len(broken)} file(s) that don't exist:", file=sys.stderr)
    for b in broken:
        print(f"  - {b}", file=sys.stderr)
    sys.exit(1)

print(f"OK: SKILL.md cross-links resolve.", file=sys.stderr)
PY

exit $?
