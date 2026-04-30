#!/usr/bin/env python3
"""Generic, data-driven grader for sdkv2-to-framework evals.

Each eval's pass/fail criteria live in `evals/evals.json` as a structured
`checks` array. This script is a dispatcher: it reads the checks, applies
generic check primitives (no provider-specific Python), and emits the
viewer-compatible grading.json per run.

Adding a new eval requires no Python changes — only an evals.json edit.

# Path resolution

Repo paths are derived via `git rev-parse --show-toplevel` from the script's
location, so the grader works regardless of where the user cloned the repo.
The provider clone (e.g., openstack, digitalocean) is supplied via
--clone-path (a positional substitution into a per-eval `clone_path` token
when the eval doesn't carry one).

# Layout

  <repo>/
    sdkv2-to-framework/evals/evals.json    # eval definitions + checks
    sdkv2-to-framework-workspace/
      grade.py                             # this script
      iteration-N/<eval-name>/<config>/run-<R>/
        outputs/                           # agent outputs (migrated/, notes.md, etc.)
        grading.json                       # written by this script

# Check primitives

Defined in CHECK_REGISTRY. Each takes a context (paths, src lookups) and
the check's args from evals.json, and returns (passed, evidence). Adding a
new primitive: register a function in CHECK_REGISTRY.

Usage:
    python grade.py --iteration 6
    python grade.py --workspace path/to/do-smoke --clone-path path/to/digitalocean
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

# -----------------------------------------------------------------------------
# Path resolution via git
# -----------------------------------------------------------------------------

def git_toplevel(start: Path) -> Path:
    r = subprocess.run(["git", "-C", str(start), "rev-parse", "--show-toplevel"],
                       capture_output=True, text=True, timeout=10)
    if r.returncode != 0:
        raise RuntimeError(f"not a git repo (or git missing): {start} — {r.stderr.strip()}")
    return Path(r.stdout.strip())

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = git_toplevel(SCRIPT_DIR)
EVALS_JSON = REPO_ROOT / "sdkv2-to-framework" / "evals" / "evals.json"
WORKSPACE_BASE = REPO_ROOT / "sdkv2-to-framework-workspace"

# Set in main() from --clone-path. Module-level for primitive functions to access.
CLONE_PATH: Path | None = None
CLONE_BASELINE_BRANCH = "sdkv2-to-framework/eval-baseline"


# -----------------------------------------------------------------------------
# Helpers
# -----------------------------------------------------------------------------

def read(path: Path) -> str:
    try:
        return path.read_text()
    except FileNotFoundError:
        return ""
    except Exception as e:
        return f"<read failed: {e}>"


def imports_sdkv2(src: str) -> bool:
    """True iff the Go source has an actual import of terraform-plugin-sdk/v2/...
    (not a string in a comment). Strips // and /* */ before scanning.
    """
    no_block = re.sub(r"/\*.*?\*/", "", src, flags=re.S)
    no_line = re.sub(r"//[^\n]*", "", no_block)
    return bool(re.search(r'"github\.com/hashicorp/terraform-plugin-sdk/v2[^"]*"', no_line))


def strip_go_comments(src: str) -> str:
    """Remove // line comments and /* */ block comments from Go source."""
    no_block = re.sub(r"/\*.*?\*/", "", src, flags=re.S)
    return re.sub(r"//[^\n]*", "", no_block)


def find_attr_block(src: str, attr_name: str) -> str | None:
    """Find a Go schema attribute block like `"name": schema.XAttribute{...}` and
    return the body between the matching { and } (brace-balanced — handles nested
    blocks correctly). Returns None if not found.

    The body has Go comments stripped so callers can do property-presence checks
    without false positives from `// Computed: true` etc.
    """
    head = re.search(
        rf'"{re.escape(attr_name)}":\s*schema\.\w+(?:Attribute|Block)\s*\{{',
        src)
    if not head:
        return None
    # Walk the source from the position right after the opening brace, tracking
    # depth. Skip over string literals and comments so braces inside them don't
    # confuse the scan.
    i = head.end()
    depth = 1
    n = len(src)
    while i < n and depth > 0:
        c = src[i]
        if c == '"':
            # Skip string literal (handle \" inside)
            i += 1
            while i < n and src[i] != '"':
                if src[i] == "\\" and i + 1 < n:
                    i += 2
                else:
                    i += 1
            i += 1
        elif c == "`":
            # Raw string — no escapes
            i += 1
            while i < n and src[i] != "`":
                i += 1
            i += 1
        elif c == "/" and i + 1 < n and src[i+1] == "/":
            # Line comment
            while i < n and src[i] != "\n":
                i += 1
        elif c == "/" and i + 1 < n and src[i+1] == "*":
            # Block comment
            i += 2
            while i + 1 < n and not (src[i] == "*" and src[i+1] == "/"):
                i += 1
            i += 2
        elif c == "{":
            depth += 1
            i += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                return strip_go_comments(src[head.end():i])
            i += 1
        else:
            i += 1
    return None


def evidence_truncate(s: str, n: int = 500) -> str:
    return s if len(s) <= n else s[:n] + "..."


# -----------------------------------------------------------------------------
# Clone management
# -----------------------------------------------------------------------------

def clone_required() -> Path:
    if CLONE_PATH is None:
        raise RuntimeError("--clone-path not set; clone-dependent check requested")
    return CLONE_PATH


def reset_clone() -> None:
    """Restore the clone to the baseline branch's HEAD and clean untracked files.

    Safety guards (each raises RuntimeError on failure):
      1. CLONE_PATH must NOT be the same git repo as REPO_ROOT — preventing the
         reset from nuking the eval workspace if --clone-path was misconfigured.
      2. The clone must be on the expected baseline branch.
      3. `git reset --hard` and `git clean -fd` must each succeed.
    """
    clone = clone_required()
    clone_top = git_toplevel(clone)
    if clone_top.resolve() == REPO_ROOT.resolve():
        raise RuntimeError(
            f"refusing to reset: --clone-path {clone} is the same repo as the "
            f"skill repo {REPO_ROOT}. The clone must be a SEPARATE provider repo."
        )
    branch = subprocess.run(
        ["git", "-C", str(clone), "rev-parse", "--abbrev-ref", "HEAD"],
        capture_output=True, text=True, timeout=10, check=True).stdout.strip()
    if branch != CLONE_BASELINE_BRANCH:
        raise RuntimeError(
            f"clone {clone} is on branch {branch!r}, expected {CLONE_BASELINE_BRANCH!r}. "
            f"Run: git -C {clone} checkout {CLONE_BASELINE_BRANCH}"
        )
    r = subprocess.run(["git", "-C", str(clone), "reset", "--hard", "HEAD"],
                       capture_output=True, text=True, timeout=30)
    if r.returncode != 0:
        raise RuntimeError(f"git reset --hard failed in {clone}: {r.stderr.strip()}")
    r = subprocess.run(["git", "-C", str(clone), "clean", "-fd"],
                       capture_output=True, text=True, timeout=30)
    if r.returncode != 0:
        raise RuntimeError(f"git clean -fd failed in {clone}: {r.stderr.strip()}")


def apply_outputs_to_clone(out_dir: Path, target_subdir: str = "") -> list[str]:
    """Copy *.go from out_dir/migrated/ (recursively) into <clone>/<target_subdir>/.
    Subdirectory structure is preserved (so an agent's `migrated/internal/foo.go`
    lands at `<target_subdir>/internal/foo.go`).

    Also copies migrated_schema.go (single-file schema-only evals) and any
    go.mod / go.sum the agent produced. go.mod / go.sum land at the clone root.

    Returns the list of clone-relative paths written.
    """
    clone = clone_required()
    written: list[str] = []
    target_dir = clone / target_subdir if target_subdir else clone

    migrated_dir = out_dir / "migrated"
    if migrated_dir.is_dir():
        for src in migrated_dir.rglob("*.go"):
            rel = src.relative_to(migrated_dir)
            dst = target_dir / rel
            dst.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(src, dst)
            written.append(str(dst.relative_to(clone)))
        for fname in ("go.mod", "go.sum"):
            src = migrated_dir / fname
            if src.exists():
                shutil.copyfile(src, clone / fname)
                written.append(fname)

    schema_only = out_dir / "migrated_schema.go"
    if schema_only.exists():
        dst_name = f"_eval_schema_{schema_only.name}"
        dst = target_dir / dst_name
        dst.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(schema_only, dst)
        written.append(str(dst.relative_to(clone)))

    return written


def run_go(cmd: list[str], cwd: Path, timeout: int = 180) -> tuple[bool, str]:
    """Run a go command and return (success, stderr/stdout)."""
    try:
        r = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout)
        ok = r.returncode == 0
        return ok, ((r.stderr or r.stdout).strip())
    except FileNotFoundError:
        return False, "go not on PATH"
    except subprocess.TimeoutExpired as e:
        return False, f"timed out: {e}"


def gofmt_check(file: Path) -> tuple[bool, str]:
    try:
        r = subprocess.run(["gofmt", "-e", str(file)],
                           capture_output=True, text=True, timeout=15)
        if r.returncode != 0 or r.stderr.strip():
            return False, (r.stderr or r.stdout).strip()[:300]
        return True, "gofmt -e clean"
    except FileNotFoundError:
        return False, "gofmt not on PATH"
    except subprocess.TimeoutExpired:
        return False, "gofmt timed out"


# -----------------------------------------------------------------------------
# Check context — what each primitive receives
# -----------------------------------------------------------------------------

class Ctx:
    """Holds per-run paths and lazy file reads. Passed to every check primitive."""
    def __init__(self, out_dir: Path, eval_def: dict, no_go_checks: bool):
        self.out_dir = out_dir
        self.eval_def = eval_def
        self.no_go_checks = no_go_checks
        self._src_cache: dict[Path, str] = {}

    def src(self, rel: str) -> str:
        """Read a file relative to out_dir, with caching."""
        path = self.out_dir / rel
        if path not in self._src_cache:
            self._src_cache[path] = read(path)
        return self._src_cache[path]

    def src_path(self, rel: str) -> Path:
        return self.out_dir / rel

    def src_files_glob(self, glob: str) -> list[Path]:
        return list(self.out_dir.glob(glob))


# -----------------------------------------------------------------------------
# Check primitives
# -----------------------------------------------------------------------------
#
# Each takes (ctx, args) where args is the dict from evals.json (excluding 'type'
# and 'text'). Returns (passed: bool, evidence: str).
# Add a new primitive: register a function below, then add to CHECK_REGISTRY.

def check_no_sdkv2_import(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """Verify the named file (relative to out_dir) does not import terraform-plugin-sdk/v2.
    Comments and string literals don't count.
    args: file (str)
    """
    src = ctx.src(args["file"])
    if not src:
        return False, f"file not found: {args['file']}"
    has = imports_sdkv2(src)
    return not has, ("sdkv2 import absent" if not has else "still imports terraform-plugin-sdk/v2")


def check_imports_package(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """File imports the given package path (any version of it).
    args: file, package
    """
    src = ctx.src(args["file"])
    if not src:
        return False, f"file not found: {args['file']}"
    pkg = re.escape(args["package"])
    no_block = re.sub(r"/\*.*?\*/", "", src, flags=re.S)
    no_line = re.sub(r"//[^\n]*", "", no_block)
    has = bool(re.search(rf'"{pkg}[^"]*"', no_line))
    return has, ("imports " + args["package"]) if has else (f"missing import: {args['package']}")


def check_has_method(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """File defines `func (r *T) MethodName(`.
    args: file, method
    """
    src = ctx.src(args["file"])
    if not src:
        return False, f"file not found: {args['file']}"
    m = args["method"]
    has = bool(re.search(rf"func\s*\(\w+\s+\*?\w+\)\s+{re.escape(m)}\s*\(", src))
    return has, (f"{m} method present" if has else f"{m} method not found")


def check_has_methods(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """File defines all listed methods.
    args: file, methods (list of str), min_count (optional, default len(methods))
    """
    src = ctx.src(args["file"])
    if not src:
        return False, f"file not found: {args['file']}"
    methods = args["methods"]
    found = [m for m in methods
             if re.search(rf"func\s*\(\w+\s+\*?\w+\)\s+{re.escape(m)}\s*\(", src)]
    min_count = args.get("min_count", len(methods))
    return len(found) >= min_count, f"found {len(found)}/{len(methods)}: {found}"


def check_implements_interface(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """File has a compile-time interface assertion `var _ <iface> = &T{}` (or similar).
    args: file, interface (e.g., "resource.ResourceWithIdentity")
    """
    src = ctx.src(args["file"])
    if not src:
        return False, f"file not found: {args['file']}"
    iface = re.escape(args["interface"])
    has = bool(re.search(rf"\b{iface}\s*=", src))
    return has, (f"asserts {args['interface']}" if has else "no compile-time interface assertion")


def check_attribute_property(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """An attribute "name" in the schema has the given property values.

    args:
      file (str)
      attribute (str): the schema attribute key (e.g., "password")
      must_have (dict): {property → expected}, where expected is True/False
                        for boolean props, or "present"/"absent" for any-value.
      must_not_have (list[str]): property names that must NOT be set to true.

    The attribute body is brace-balanced and Go comments are stripped before
    matching, so commented-out properties don't trigger false positives.
    """
    src = ctx.src(args["file"])
    if not src:
        return False, f"file not found: {args['file']}"
    body = find_attr_block(src, args["attribute"])  # already comment-stripped
    if body is None:
        return False, f"attribute \"{args['attribute']}\" not found in schema"

    must_have = args.get("must_have", {})
    must_not = args.get("must_not_have", [])
    issues = []
    for prop, expected in must_have.items():
        if expected is True:
            if not re.search(rf"\b{prop}:\s*true\b", body):
                issues.append(f"missing {prop}: true")
        elif expected is False:
            if re.search(rf"\b{prop}:\s*true\b", body):
                issues.append(f"unexpectedly has {prop}: true")
        elif expected == "present":
            if not re.search(rf"\b{prop}:\s*\S", body):
                issues.append(f"missing {prop}")
        elif expected == "absent":
            if re.search(rf"\b{prop}:\s*\S", body):
                issues.append(f"unexpectedly has {prop}")

    for prop in must_not:
        if re.search(rf"\b{prop}:\s*true\b", body):
            issues.append(f"forbidden: {prop}: true")

    return (not issues, "; ".join(issues) if issues else f"\"{args['attribute']}\" matches expected properties")


def check_regex(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """Generic regex search.

    args:
      file (str)
      pattern (str): Python regex
      expect (str): "present" or "absent" (default "present")
      flags (list[str]): subset of ['s','i','m']
      ignore_comments (bool, default False): strip Go comments before matching
      allow_missing (bool, default False): if True, missing file is treated as
        "no match" (so expect=absent passes); if False (default), missing file
        always fails — refusal evals should use no_migrated_dir instead.
    """
    src = ctx.src(args["file"])
    if not src:
        if args.get("allow_missing", False):
            return args.get("expect", "present") == "absent", f"file not found (allowed): {args['file']}"
        return False, f"file not found: {args['file']}"
    if args.get("ignore_comments"):
        src = strip_go_comments(src)
    flag_map = {"s": re.S, "i": re.I, "m": re.M}
    flags = 0
    for f in args.get("flags", []):
        flags |= flag_map.get(f, 0)
    found = re.search(args["pattern"], src, flags)
    expect = args.get("expect", "present")
    if expect == "present":
        return bool(found), (f"matched: {found.group(0)[:120]}" if found else "no match")
    elif expect == "absent":
        return not found, ("absent" if not found else f"unexpectedly matched: {found.group(0)[:120]}")
    return False, f"unknown expect={expect!r}"


def check_regex_count(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """Regex must match >= min (or exactly == exact) times.
    args: file, pattern, min (int) OR exact (int) OR max (int), flags (list)
    """
    src = ctx.src(args["file"])
    if not src:
        return False, f"file not found: {args['file']}"
    flag_map = {"s": re.S, "i": re.I, "m": re.M}
    flags = 0
    for f in args.get("flags", []):
        flags |= flag_map.get(f, 0)
    matches = re.findall(args["pattern"], src, flags)
    n = len(matches)
    if "exact" in args:
        return n == args["exact"], f"matched {n}/{args['exact']}"
    if "min" in args:
        return n >= args["min"], f"matched {n} (need >= {args['min']})"
    if "max" in args:
        return n <= args["max"], f"matched {n} (need <= {args['max']})"
    return False, f"check_regex_count: must specify exact, min, or max"


def check_syntax_valid(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """gofmt -e on the named file (or all *.go under outputs/migrated if file omitted).
    args: file (optional)
    """
    if "file" in args:
        path = ctx.src_path(args["file"])
        if not path.exists():
            return False, f"file not found: {args['file']}"
        return gofmt_check(path)
    # Scan all migrated *.go.
    files = ctx.src_files_glob("migrated/*.go")
    if not files:
        return False, "no migrated/*.go to check"
    for f in files:
        ok, evidence = gofmt_check(f)
        if not ok:
            return False, f"{f.name}: {evidence}"
    return True, f"{len(files)} files gofmt-clean"


def check_imports_whitelist(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """Every import inside an `import (...)` block (or single-line `import "..."`)
    in the named files (or all migrated/*.go) starts with one of the allowed
    prefixes. Stdlib (no dot in first segment) is always allowed.

    Anchored to actual import declarations — does NOT match arbitrary quoted
    strings elsewhere in the file (so a struct literal `"foo/bar"` won't trip).

    args:
      files (list[str], optional): out_dir-relative paths; defaults to migrated/*.go (recursive)
      allowed (list[str]): import-path prefixes
    """
    if "files" in args:
        files = [ctx.src_path(f) for f in args["files"]]
    else:
        files = list((ctx.out_dir / "migrated").rglob("*.go")) if (ctx.out_dir / "migrated").is_dir() else []
    allowed_prefixes = tuple(args.get("allowed", []))
    if not allowed_prefixes:
        return False, "check_imports_whitelist: no 'allowed' list configured"
    bad: list[str] = []
    for path in files:
        if not path.exists():
            continue
        text = strip_go_comments(read(path))
        # Match `import (...)` blocks AND single-line `import "..."`.
        for blk in re.finditer(r'\bimport\s*(?:\(\s*([^)]*)\)|"([^"]+)")', text):
            if blk.group(2):
                paths = [blk.group(2)]
            else:
                paths = re.findall(r'(?:^|\s)(?:\w+\s+)?"([^"]+)"', blk.group(1))
            for imp in paths:
                if "/" not in imp:
                    continue
                first = imp.split("/", 1)[0]
                if "." not in first:
                    continue  # stdlib (e.g., net/http)
                if not imp.startswith(allowed_prefixes):
                    bad.append(f"{path.name}: {imp}")
    return not bad, "all imports in allowed set" if not bad else f"unknown imports: {bad[:5]}"


def check_go_build(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """Apply outputs to clone, run `go build` on a target package, then reset.
    Single-file migrations of multi-file packages will commonly fail with
    "undefined: <symbol>" because sibling files reference the original
    function names — that's expected and not graded by this primitive directly.
    Use check_apply_and_gofmt for syntax-only validation.

    args:
      target (str, optional): go package to build (default ./...)
      target_subdir (str, optional): subdir to apply outputs into
      timeout (int, default 180)
    """
    if ctx.no_go_checks:
        return True, "skipped (--no-go-checks)"
    reset_clone()
    apply_outputs_to_clone(ctx.out_dir, args.get("target_subdir", ""))
    target = args.get("target", "./...")
    ok, msg = run_go(["go", "build", target], cwd=clone_required(),
                     timeout=args.get("timeout", 180))
    reset_clone()
    return ok, ("go build passed" if ok else msg[:400])


def check_go_test(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """Apply outputs to clone, run `go test -run <pattern> <package>`, then reset.

    args:
      package (str): package path (e.g., ./openstack/...)
      run (str, optional): -run pattern (default ^TestProvider$)
      target_subdir (str, optional)
      timeout (int, default 180)
    """
    if ctx.no_go_checks:
        return True, "skipped (--no-go-checks)"
    reset_clone()
    apply_outputs_to_clone(ctx.out_dir, args.get("target_subdir", ""))
    pattern = args.get("run", "^TestProvider$")
    pkg = args["package"]
    ok, msg = run_go(["go", "test", "-run", pattern, "-count=1", pkg],
                     cwd=clone_required(), timeout=args.get("timeout", 180))
    reset_clone()
    return ok, (f"go test {pattern} on {pkg} passed" if ok else msg[:400])


def check_clone_clean(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """`git status --porcelain` on the clone (optionally scoped) returns nothing.
    Used for inventory-only and refusal evals where the agent must not modify code.
    args: subdir (optional, e.g. 'openstack/' for openstack)
    """
    clone = clone_required()
    pathspec = [args["subdir"]] if args.get("subdir") else []
    r = subprocess.run(["git", "-C", str(clone), "status", "--porcelain", *pathspec],
                       capture_output=True, text=True, timeout=15)
    clean = r.returncode == 0 and r.stdout.strip() == ""
    return clean, ("clean" if clean else f"dirty: {r.stdout[:200]}")


def check_no_outputs_modified_unrelated(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """Apply migrated outputs to the clone, then verify only the declared `expected_files`
    were modified. Resets the clone before and after.
    args: expected_files (list of clone-relative paths the agent should have touched)
    """
    if ctx.no_go_checks:
        return True, "skipped (--no-go-checks)"
    clone = clone_required()
    reset_clone()
    written = apply_outputs_to_clone(ctx.out_dir, args.get("target_subdir", ""))
    expected = set(args.get("expected_files", []))
    # The agent's outputs land at the same path as the source; written list IS the touched set.
    diff = subprocess.run(["git", "-C", str(clone), "status", "--porcelain"],
                          capture_output=True, text=True, timeout=15).stdout
    modified = set()
    for line in diff.splitlines():
        if len(line) >= 4:
            path = line[3:].strip()
            if path.endswith(".go"):
                modified.add(path)
    unrelated = sorted(p for p in modified if p not in expected and p not in set(written))
    reset_clone()
    return (not unrelated,
            "no unrelated *.go modifications" if not unrelated
            else f"unrelated: {unrelated[:5]}")


def check_apply_and_gofmt(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """Apply outputs to clone and gofmt-check every written *.go file.
    Reports as a single pass/fail; failures are propagated with file names.
    args: target_subdir (optional)
    """
    if ctx.no_go_checks:
        return True, "skipped (--no-go-checks)"
    reset_clone()
    written = apply_outputs_to_clone(ctx.out_dir, args.get("target_subdir", ""))
    errors: list[str] = []
    clone = clone_required()
    for rel in written:
        if not rel.endswith(".go"):
            continue
        ok, ev = gofmt_check(clone / rel)
        if not ok:
            errors.append(f"{rel}: {ev[:120]}")
    reset_clone()
    return not errors, "all migrated files gofmt-clean" if not errors else f"errors: {errors[:3]}"


def check_file_size_under(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """File size in bytes is under the cap.
    args: file, max_bytes
    """
    path = ctx.src_path(args["file"])
    if not path.exists():
        return False, f"file not found: {args['file']}"
    size = path.stat().st_size
    return size < args["max_bytes"], f"{path.name} = {size} bytes (cap {args['max_bytes']})"


def check_attrs_preserved(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """Every SDKv2 attribute name in `original_file` (clone-relative) appears in `migrated_file`
    as a quoted key. Catches user-facing schema renames.
    args: original_file (clone-relative), migrated_file (out_dir-relative), min_coverage (default 0.9)
    """
    clone = clone_required()
    orig_path = clone / args["original_file"]
    if not orig_path.exists():
        return False, f"original not found: {args['original_file']}"
    orig = read(orig_path)
    migrated = ctx.src(args["migrated_file"])
    if not migrated:
        return False, f"migrated not found: {args['migrated_file']}"
    orig_attrs = set(re.findall(r'"(\w+)":\s*\{\s*Type:', orig))
    if not orig_attrs:
        return True, "no attributes detected in original (vacuously satisfied)"
    missing = [a for a in orig_attrs if f'"{a}"' not in migrated]
    coverage = 1 - len(missing) / len(orig_attrs)
    min_cov = args.get("min_coverage", 0.9)
    full = not missing
    return (full or coverage >= min_cov,
            f"{len(orig_attrs)} attrs; coverage={coverage:.0%}; missing={missing[:5]}")


def check_response_refuses(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """The named response files (default: notes.md and REFUSAL.md) contain at
    least one matching phrase from the list (or all if require_all=True).

    args:
      phrases (list[str]): regex patterns
      files (list[str], optional): out_dir-relative paths to scan;
        default ["notes.md", "REFUSAL.md"]
      require_all (bool, default False): every phrase must match
    """
    files = args.get("files", ["notes.md", "REFUSAL.md"])
    text = "\n".join(ctx.src(f) for f in files)
    matched: list[str] = []
    for pat in args.get("phrases", []):
        if re.search(pat, text, re.I):
            matched.append(pat)
    if args.get("require_all"):
        ok = len(matched) == len(args.get("phrases", []))
    else:
        ok = len(matched) >= 1
    return ok, f"matched {len(matched)}/{len(args.get('phrases', []))}: {matched[:3]}"


def check_no_migrated_dir(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """outputs/migrated/ does not contain any *.go files. For refusal evals."""
    files = ctx.src_files_glob("migrated/*.go")
    return not files, "no migrated/*.go" if not files else f"unexpected: {[f.name for f in files][:5]}"


def check_phrases_present(ctx: Ctx, args: dict) -> tuple[bool, str]:
    """Every phrase in `phrases` appears at least once in the named file (case-insensitive,
    regex-aware). Useful for "verbatim 12 step titles" style checks where a single
    pass/fail with a missing-list is more useful than 12 separate rows.

    args:
      file (str): out_dir-relative
      phrases (list[str]): regex patterns; case-insensitive
      min_matches (int, optional): require at least N of the phrases to match
        (default: all of them)
    """
    src = ctx.src(args["file"])
    if not src:
        return False, f"file not found: {args['file']}"
    phrases = args["phrases"]
    matched = [p for p in phrases if re.search(p, src, re.I)]
    missing = [p for p in phrases if p not in matched]
    min_n = args.get("min_matches", len(phrases))
    return len(matched) >= min_n, f"matched {len(matched)}/{len(phrases)}; missing: {missing[:5]}"


CHECK_REGISTRY: dict[str, callable] = {
    "no_sdkv2_import":              check_no_sdkv2_import,
    "imports_package":              check_imports_package,
    "has_method":                   check_has_method,
    "has_methods":                  check_has_methods,
    "implements_interface":         check_implements_interface,
    "attribute_property":           check_attribute_property,
    "regex":                        check_regex,
    "regex_count":                  check_regex_count,
    "syntax_valid":                 check_syntax_valid,
    "imports_whitelist":            check_imports_whitelist,
    "clone_clean":                  check_clone_clean,
    "apply_and_gofmt":              check_apply_and_gofmt,
    "no_outputs_modified_unrelated":check_no_outputs_modified_unrelated,
    "file_size_under":              check_file_size_under,
    "attrs_preserved":              check_attrs_preserved,
    "response_refuses":             check_response_refuses,
    "no_migrated_dir":              check_no_migrated_dir,
    "go_build":                     check_go_build,
    "go_test":                      check_go_test,
    "phrases_present":              check_phrases_present,
}


# -----------------------------------------------------------------------------
# Driver
# -----------------------------------------------------------------------------

def grade_run(out_dir: Path, eval_def: dict, no_go_checks: bool) -> dict:
    """Run every check declared in eval_def['checks'] against out_dir."""
    ctx = Ctx(out_dir=out_dir, eval_def=eval_def, no_go_checks=no_go_checks)
    expectations = []
    for chk in eval_def.get("checks", []):
        ctype = chk.get("type")
        text = chk.get("text") or ctype or "<unnamed>"
        fn = CHECK_REGISTRY.get(ctype)
        if not fn:
            expectations.append({"text": text, "passed": False,
                                 "evidence": f"unknown check type: {ctype}"})
            continue
        args = {k: v for k, v in chk.items() if k not in ("type", "text")}
        try:
            passed, evidence = fn(ctx, args)
        except Exception as e:
            passed, evidence = False, f"check error: {type(e).__name__}: {e}"
        expectations.append({"text": text, "passed": bool(passed),
                             "evidence": evidence_truncate(str(evidence))})
    passed_n = sum(1 for e in expectations if e["passed"])
    total = len(expectations)
    return {"expectations": expectations,
            "summary": {"passed": passed_n, "failed": total - passed_n, "total": total,
                        "pass_rate": passed_n / total if total else 0.0}}


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--iteration", type=int, default=None,
                    help="iteration-N directory under workspace")
    ap.add_argument("--workspace", type=str, default=None,
                    help="explicit workspace dir (overrides --iteration)")
    ap.add_argument("--clone-path", type=str, default=None,
                    help="provider clone the grader applies migrated outputs to")
    ap.add_argument("--no-go-checks", action="store_true",
                    help="skip subprocess go/gofmt/clone-mutating checks")
    args = ap.parse_args()

    global CLONE_PATH
    if args.clone_path:
        CLONE_PATH = Path(args.clone_path).resolve()
        # Verify it's a git repo.
        try:
            git_toplevel(CLONE_PATH)
        except Exception as e:
            print(f"--clone-path is not a git repo: {e}", file=sys.stderr)
            sys.exit(1)

    if args.workspace:
        workspace = Path(args.workspace).resolve()
    elif args.iteration is not None:
        workspace = WORKSPACE_BASE / f"iteration-{args.iteration}"
    else:
        print("must pass --iteration N or --workspace path", file=sys.stderr)
        sys.exit(1)
    if not workspace.is_dir():
        print(f"workspace not found: {workspace}", file=sys.stderr)
        sys.exit(1)

    evals = json.loads(EVALS_JSON.read_text())["evals"]

    for ev in evals:
        if "checks" not in ev:
            continue  # eval not yet migrated to structured checks
        eval_dir = workspace / ev["name"]
        if not eval_dir.exists():
            continue
        for config_dir in sorted(p for p in eval_dir.iterdir() if p.is_dir()):
            run_dirs = sorted(d for d in config_dir.iterdir()
                              if d.is_dir() and d.name.startswith("run-"))
            cells = run_dirs if run_dirs else [config_dir]
            for cell in cells:
                if not (cell / "outputs").is_dir():
                    continue
                grading = grade_run(cell / "outputs", ev, args.no_go_checks)
                (cell / "grading.json").write_text(json.dumps(grading, indent=2))
                sr = grading["summary"]
                rel = cell.relative_to(workspace)
                print(f"  {rel}: {sr['passed']}/{sr['total']} ({sr['pass_rate']:.0%})")

    print("Done.")


if __name__ == "__main__":
    main()
