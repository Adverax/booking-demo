# forge
#!/usr/bin/env python3
"""
forge post-pull hook  —  external-drift detector

Fires after externally-authored commits enter the working tree and classifies
what they touched against the forge model and the legacy safety net. The pull
moment is the ONE chance to notice semantic drift cheaply — nothing else fires
on `git pull` (session hooks watch Write/Edit, git-post-commit watches local
commits). The signal is PERSISTED to {drift.pending} (default
meta/drift-pending.yml) so it survives until acted on: forge:status surfaces it
every session, `reverse drift --scope` / pin re-runs clear it.

Two entry modes:
  - Claude Code PostToolUse on Bash (stdin JSON): fires when the command was a
    `git pull` / `git merge` (non-card branches only — kanban's own card merges
    are not drift).
  - `--git-hook`: delegated from .git/hooks/post-merge (terminal pulls) via
    git-post-merge.sh; the reflog decides whether this merge was a pull.

Exit 0 always — detection is advisory; the gates live in kanban/status.

Install via: /forge:init hooks
"""
from __future__ import annotations

import fnmatch
import json
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path, PurePosixPath

from _forge_common import command_matches, find_meta, load_yaml, warn


def main() -> None:
    git_hook_mode = "--git-hook" in sys.argv

    if not git_hook_mode:
        try:
            data = json.load(sys.stdin)
        except Exception:
            sys.exit(0)
        command: str = data.get("tool_input", {}).get("command", "")
        is_pull = command_matches(command, "git", "pull")
        is_merge = command_matches(command, "git", "merge")
        if not (is_pull or is_merge):
            sys.exit(0)
        # kanban's own card merges are not drift
        if is_merge and re.search(r"\bmerge\b[^|;&]*\bcard/", command):
            sys.exit(0)
    else:
        # Terminal post-merge: the reflog tells a pull from a local merge.
        msg = _git(["git", "reflog", "-1", "--format=%gs"])
        if msg is None or not msg.startswith("pull"):
            if msg is None or "merge card/" in msg:
                sys.exit(0)
            if not msg.startswith("merge"):
                sys.exit(0)

    # Never fire from inside a kanban worktree (its rebases replay changes the
    # main repo already classified at pull time).
    common = _git(["git", "rev-parse", "--git-common-dir"])
    git_dir = _git(["git", "rev-parse", "--git-dir"])
    if common and git_dir and common != git_dir:
        sys.exit(0)

    meta = find_meta()
    if meta is None:
        sys.exit(0)

    files = _changed_files()
    if not files:
        sys.exit(0)

    event = _classify(meta, files)
    if not any(event[k] for k in ("pinned", "modeled", "harness", "net")):
        sys.exit(0)  # only unmodeled files changed — the model grows on-touch, no signal

    pending = _pending_path(meta)
    _append_event(pending, event)
    _report(event, pending)


def _git(args: list[str]) -> str | None:
    try:
        out = subprocess.run(args, capture_output=True, text=True, timeout=10)
        return out.stdout.strip() if out.returncode == 0 else None
    except Exception:
        return None


def _changed_files() -> list[str]:
    """Files brought in by the pull/merge: ORIG_HEAD..HEAD (git sets ORIG_HEAD
    on pull/merge/rebase). No ORIG_HEAD → nothing to classify."""
    out = _git(["git", "diff", "--name-only", "ORIG_HEAD..HEAD"])
    if not out:
        return []
    return [ln.strip() for ln in out.splitlines() if ln.strip()]


def _pending_path(meta: Path) -> Path:
    cfg = load_yaml(meta / ".skills.yml") or {}
    rel = ((cfg.get("drift") or {}).get("pending")) or "meta/drift-pending.yml"
    return meta.parent / rel


def _glob_match(path: str, glob: str) -> bool:
    g = glob.strip().rstrip("/")
    return (fnmatch.fnmatch(path, g) or fnmatch.fnmatch(path, g + "/*")
            or path == g or path.startswith(g.rstrip("*").rstrip("/") + "/"))


def _net_anchor(entry: str) -> str | None:
    """The suffix a registered safety-net artifact is recognised by after a move.

    ``<parent directory>/<file name>`` — the last two segments of the registered
    path, and nothing above them. That is the whole of the anchor: everything
    ABOVE the parent may change (the depth, the directories the area was moved
    under) and the artifact is still recognised; the parent itself may not.

    Args:
        entry: A path as the safety-net registry spells it.

    Returns:
        The two-segment suffix, or ``None`` when the entry names no directory at
        all — a bare file name carries no context to anchor against, so for such
        an entry only an exact match can be trusted.
    """
    parts = [p for p in PurePosixPath(entry.strip()).parts if p not in (".", "/")]
    return "/".join(parts[-2:]) if len(parts) >= 2 else None


def _is_net_artifact(path: str, net_tests: list[str]) -> bool:
    """Whether a pulled file IS one of the registered safety-net artifacts.

    THE CONTRACT, DECLARED — this matcher recognises a registered artifact **by
    its anchored basename**, not only at the exact path the registry spells:

    * the exact registered path always matches;
    * so does any path ending in the registered ``<parent>/<file name>`` — the
      same characterization test at a different depth, or under a directory that
      was moved or renamed above its own parent, INCLUDING the case where
      nothing is left above the parent at all (the area moved to sit directly at
      the repository root: the pulled path then equals the anchor exactly, which
      is "ends in the anchor" with zero prefix, not a different case of it). A
      stale registry is the normal case here: the pull is precisely the moment
      someone else's move arrives, and a net artifact that goes unrecognised is
      a silent miss, not a warning;
    * a registry entry with NO parent directory (a bare file name) anchors
      against nothing — ``_net_anchor`` returns ``None`` for it — so such an
      entry is trusted only at its exact registered path; a same-named file
      arriving under any directory is not the safety net.

    WHAT THE ANCHOR DELIBERATELY EXCLUDES, so the widening does not buy a false
    positive: a file that shares only the BASENAME with a registered test
    (``vendor/acme/test_invoice_totals.py`` against
    ``tests/characterization/legacy/billing/test_invoice_totals.py``) is NOT the
    safety net. Same-named test files in unrelated trees are ordinary, and
    telling a developer to restore or re-pin a file that was never pinned costs
    the warning its credibility. The anchor is a PATH SEGMENT, not a raw string
    suffix: it must be preceded by a ``/`` (or be the whole path — the case
    above), so ``vendor/xxbilling/test_invoice_totals.py`` does not match the
    registered ``…/billing/test_invoice_totals.py`` even though the raw
    characters ``billing/test_invoice_totals.py`` are a string suffix of it —
    ``xxbilling`` is a different directory, not ``billing`` with something
    prepended.

    This was a dead branch until CARD-061: the predicate was written
    ``f == t or f.endswith("/" + Path(t).name) and f == t``, and since ``and``
    binds tighter than ``or`` the basename half could only ever be reached with
    ``f == t`` already true — the whole expression was an exact-path comparison
    declaring itself as more.

    Args:
        path: A repository-relative path brought in by the pull.
        net_tests: The registry file plus every test path it registers.

    Returns:
        True when the path is a safety-net artifact under the contract above.
    """
    if path in net_tests:
        return True
    for entry in net_tests:
        anchor = _net_anchor(entry)
        if anchor and (path == anchor or path.endswith("/" + anchor)):
            return True
    return False


def _classify(meta: Path, files: list[str]) -> dict:
    """Sort the pulled files into the event's categories.

    The categories are tried in order and the first hit wins: safety net (per
    the contract in :func:`_is_net_artifact`), pinned area, modeled code,
    manifest/infra. A file matching none of them is unmodeled and is not
    recorded — the model grows on-touch.
    """
    cfg = load_yaml(meta / ".skills.yml") or {}
    root = meta.parent

    # pinned areas + the net's own artifacts
    net_rel = ((cfg.get("legacy") or {}).get("safety_net")) or "meta/architecture/safety-net.yml"
    net_data = load_yaml(root / net_rel) or {}
    pin_areas: dict[str, list[str]] = {}
    net_tests: list[str] = [net_rel]
    for a in (net_data.get("areas") or []):
        if isinstance(a, dict) and a.get("area"):
            pin_areas[str(a["area"])] = []
            net_tests += [str(t) for t in (a.get("tests") or [])]

    # modeled code globs from trace.yml
    arch_rel = ((cfg.get("paths") or {}).get("architecture")) or "meta/architecture"
    trace = load_yaml(root / arch_rel.rstrip("/") / "trace.yml") or {}
    model_globs: list[str] = []
    for comp in (trace.get("components") or []):
        if isinstance(comp, dict):
            model_globs += [str(g) for g in (comp.get("code") or [])]

    manifests = {"go.mod", "go.sum", "package.json", "package-lock.json",
                 "pyproject.toml", "requirements.txt", "Cargo.toml", "Cargo.lock",
                 "docker-compose.yml", "Makefile"}

    event = {
        "at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "range": _git(["git", "rev-parse", "--short", "ORIG_HEAD"]) or "?",
        "head": _git(["git", "rev-parse", "--short", "HEAD"]) or "?",
        "pinned": {}, "modeled": [], "harness": [], "net": [],
    }
    for f in files:
        if _is_net_artifact(f, net_tests):
            event["net"].append(f)
            continue
        hit_pin = False
        for area in pin_areas:
            if _glob_match(f, area):
                event["pinned"].setdefault(area, []).append(f)
                hit_pin = True
                break
        if hit_pin:
            continue
        if any(_glob_match(f, g) for g in model_globs):
            event["modeled"].append(f)
            continue
        if Path(f).name in manifests:
            event["harness"].append(f)
    return event


def _append_event(pending: Path, event: dict) -> None:
    data = load_yaml(pending) if pending.exists() else None
    if not isinstance(data, dict):
        data = {"version": 1, "events": []}
    data.setdefault("events", []).append(event)
    _dump(pending, data)


def _dump(path: Path, data: dict) -> None:
    """YAML if a lib is available; JSON otherwise (valid YAML subset)."""
    try:
        from ruamel.yaml import YAML
        y = YAML()
        y.default_flow_style = False
        with path.open("w", encoding="utf-8") as fh:
            y.dump(data, fh)
        return
    except ImportError:
        pass
    try:
        import yaml
        path.write_text(yaml.safe_dump(data, allow_unicode=True, sort_keys=False),
                        encoding="utf-8")
        return
    except ImportError:
        path.write_text(json.dumps(data, indent=2, ensure_ascii=False), encoding="utf-8")


def _report(event: dict, pending: Path) -> None:
    warn(f"external changes pulled ({event['range']}..{event['head']}) — drift recorded in {pending.name}:")
    if event["net"]:
        print(f"  ⚑ SAFETY NET ITSELF touched: {', '.join(event['net'][:3])} — someone "
              "changed the characterization tests/registry; restore or re-pin with an "
              "explicit verdict, never accept silently.", file=sys.stderr)
    for area, fs in event["pinned"].items():
        print(f"  ⚑ pinned area {area}: {len(fs)} file(s) — run the area's pin tests NOW; "
              "a red pin = behavior changed externally → triage (intended? re-pin + model "
              "delta / regression? @ops card, the pin is its regression test).", file=sys.stderr)
    if event["modeled"]:
        print(f"  ⚑ modeled code: {len(event['modeled'])} file(s) → /forge:reverse drift "
              "--since ORIG_HEAD (model catches up; clears this signal).", file=sys.stderr)
    if event["harness"]:
        print(f"  ⚑ manifests/infra: {', '.join(event['harness'][:3])} → /forge:harness check"
              " · /forge:audit deps.", file=sys.stderr)


if __name__ == "__main__":
    main()
