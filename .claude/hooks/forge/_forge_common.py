# forge
"""Shared helpers for forge hooks — path resolution that survives worktrees and
non-default `paths.*` config.

Every hook used to hardcode `meta/architecture`, `meta/kanban/cards`, … relative
to the cwd. That silently no-ops (a) in a project that relocates paths via
`meta/.skills.yml → paths.*` and (b) inside a kanban git worktree, where `meta/`
may not exist at the cwd — exactly the phase the hooks are meant to guard.

It also owns two mechanisms every gate-running hook needs:

* :func:`find_plugin_root` — WHERE the forge plugin lives, resolved at run time
  (ADR-0012). A path pinned into ``config.json`` at install time carries a
  version number and stops existing on the next upgrade, which used to weaken
  the commit gate silently.
* :func:`record_gate_event` — the durable record of a gate that FAILED OPEN
  (NFR-010). The commit is still allowed (NFR-006 is unchanged); what changes
  is that the outcome outlives the hook process, so ``/forge:status`` can
  surface it later.
* :func:`scan_text_for_secrets` / :func:`scan_files_for_secrets` — the
  deterministic secret detector (NFR-009, TM-010). It lives here rather than
  in one hook so a second gate can reuse the same rule table and the same
  verdict shape; the rules, their evidence and their DECLARED GAPS are
  documented at the bottom of this module.

Hooks import this module by filename (they live in the same directory):

    from _forge_common import find_meta, forge_paths, to_relative, command_matches
"""
from __future__ import annotations

import base64
import json
import math
import os
import re
import subprocess
import sys
import tempfile
from collections import Counter
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable  # noqa: F401 — referenced from a deferred annotation

_HOOK_DIR = Path(__file__).resolve().parent

#: Relative path of the manifest that marks a Claude Code plugin root (EXT-001).
PLUGIN_MARKER = Path(".claude-plugin") / "plugin.json"

#: Environment variable the host exports into a plugin hook process.
PLUGIN_ROOT_ENV = "CLAUDE_PLUGIN_ROOT"

#: Where the host keeps installed plugins, relative to its config home:
#: ``<config home>/plugins/cache/<marketplace>/<plugin>/<version>``.
PLUGIN_CACHE_REL = ("plugins", "cache", "adverax", "forge")

#: The host's own authoritative record of which installation is active,
#: relative to its config home. Written by the host at install/update time —
#: unlike a cache directory's NAME (a string this module does not control),
#: this file states the active ``installPath`` directly.
INSTALLED_PLUGINS_REL = ("plugins", "installed_plugins.json")

#: Generated journal of gates that failed open, written next to the model.
GATE_EVENTS_FILE = "gate-events.yml"

#: The journal carries TWO kinds of event, and conflating them already produced
#: a false declaration in a real project's journal: the header said "appended
#: when a deterministic gate FAILS OPEN … outcome: timeout | failure" while the
#: file itself carried `outcome: suppressed`. A suppression is not a fail-open —
#: the gate RAN and reported; a pragma silenced one of its matches. Everything
#: around `record_gate_event` in the hooks is named for the other meaning
#: (`_fail_open`, `_announce_fail_open`, "COMMIT ALLOWED WITHOUT THE GATE"), so
#: the conflation leaks wherever it is not split.
#:
#: Hence two vocabularies plus an explicit `kind:` field on every entry, so the
#: consumer (CARD-006, /forge:status) can tell "the gate did not run" from "the
#: gate ran and said less than it saw" without re-deriving it from the outcome
#: name.
FAIL_OPEN_OUTCOMES = ("timeout", "failure")
REPORT_OUTCOMES = ("suppressed",)

#: The union `record_gate_event` validates against. Kept as a name because it
#: is the whole accepted vocabulary and callers/tests assert on it.
GATE_OUTCOMES = FAIL_OPEN_OUTCOMES + REPORT_OUTCOMES

#: `kind:` values, derived from the outcome — never passed in, so the two can
#: not drift apart.
GATE_EVENT_KINDS = ("fail-open", "report")

#: Journal is capped at this many events; older ones are dropped on write
#: (F-008) — a machine with a chronically slow/broken gate must not grow this
#: file forever. It is a recent-signal file, never an audit log.
GATE_EVENTS_MAX_ENTRIES = 200

_GATE_ID_RE = re.compile(r"^[a-z0-9][a-z0-9._-]*$")

_GATE_EVENTS_HEADER = f"""\
# forge gate events - GENERATED, do not edit and do not commit (.gitignore).
#
# Appended by forge hooks when a deterministic gate has something to say that
# the terminal alone would lose (NFR-010). The operation is allowed either way
# (fail-open policy, NFR-006, unchanged); this file makes the outcome outlive
# the hook process, and /forge:status flags it in the next snapshot.
#
# TWO KINDS OF EVENT, told apart by `kind:` rather than by reading the outcome:
#
#   kind: fail-open  - the gate did NOT run to a verdict, and the operation
#                      went through without it. outcome: timeout | failure.
#   kind: report     - the gate RAN and reported less than it saw. It did not
#                      fail. outcome: suppressed (an allow-pragma silenced a
#                      real match).
#
# `at` is UTC, ISO-8601. Append-only, capped at the most recent
# {GATE_EVENTS_MAX_ENTRIES} events (older ones dropped on write) - safe to
# delete wholesale regardless; it is a signal, never a source of truth.
events:
"""


def load_hook_config() -> dict:
    """config.json written by /forge:init hooks (forge_plugin_dir, paths, …)."""
    try:
        return json.loads((_HOOK_DIR / "config.json").read_text(encoding="utf-8"))
    except Exception:
        return {}


def _is_plugin_root(path: Path, require: str | None = None) -> bool:
    """True when ``path`` is a usable forge plugin installation.

    Args:
        path: Candidate directory.
        require: Optional repo-relative file the caller needs (e.g. the CI
            script). A root that carries the manifest but not the file the
            hook came for is NOT usable — accepting it would trade the stale
            path for a resolved-but-useless one.

    Returns:
        True when the plugin manifest (and ``require``, if given) exists.
    """
    try:
        if not (path / PLUGIN_MARKER).is_file():
            return False
        return require is None or (path / require).is_file()
    except OSError:
        return False


def _version_key(root: Path) -> tuple:
    """Sort key that orders plugin installations newest-last.

    The host names each cache directory after the plugin version
    (``…/adverax/forge/2026.8.17``), so a name is treated as a dotted numeric
    version ONLY when it has at least two ``[._-]``-separated segments and
    EVERY segment is all-digit (F-001) — a bare digit string is not enough: a
    12-hex-char cache directory name that happens to contain only the digits
    0-9 (~0.5% of hashes do) is one segment, not a version, and must never
    outrank a real ``2026.8.17``. ``is_version`` therefore sorts before the
    numeric comparison, so every real version beats every non-version
    regardless of either one's magnitude or name. Two names that both are (or
    both are not) versions and compare equal fall back to modification time,
    then to the name, so the choice stays total and deterministic rather than
    filesystem-order.
    """
    parts = re.split(r"[._-]", root.name)
    is_version = len(parts) >= 2 and all(p.isdigit() for p in parts)
    numeric = tuple(int(p) for p in parts) if is_version else ()
    try:
        mtime = root.stat().st_mtime
    except OSError:
        mtime = 0.0
    return (is_version, numeric, mtime, root.name)


def _config_home() -> Path | None:
    """The host's config home — where its plugin cache lives.

    ``CLAUDE_CONFIG_DIR`` RELOCATES that home rather than adding a second one,
    so when it is set it is the only answer: falling back to ``~/.claude``
    would search a directory the host is not using, and on a machine that has
    both, resolve to an installation nobody is running.
    """
    env = os.environ.get("CLAUDE_CONFIG_DIR", "").strip()
    if env:
        return Path(env).expanduser()
    try:
        return Path.home() / ".claude"
    except (OSError, RuntimeError):
        return None


def _plugin_cache_dirs() -> list[Path]:
    """Directories that hold one sub-directory per installed forge version.

    The recorded ``forge_plugin_dir`` is used for its PARENT even when the
    path itself has gone: a pinned ``…/adverax/forge/2026.8.17`` still tells
    us exactly which cache the plugin was installed into, which is the fact
    that survives an upgrade.
    """
    dirs: list[Path] = []
    recorded = load_hook_config().get("forge_plugin_dir")
    if isinstance(recorded, str) and recorded.strip():
        dirs.append(Path(recorded.strip()).expanduser().parent)
    home = _config_home()
    if home is not None:
        dirs.append(home.joinpath(*PLUGIN_CACHE_REL))

    unique: list[Path] = []
    seen: set[str] = set()
    for d in dirs:
        key = str(d)
        if key not in seen and d.is_dir():
            seen.add(key)
            unique.append(d)
    return unique


def _installed_plugin_root(require: str | None = None) -> Path | None:
    """The host's own statement of which installation is active, if readable.

    ``<config home>/plugins/installed_plugins.json`` is written by the host at
    install/update time and names the ``installPath`` directly — it does not
    need to be inferred from a cache directory's NAME the way the fallback
    scan does, so it cannot be fooled by a hash that happens to look like a
    version (see :func:`_version_key`). Consulted before that scan; not used
    at all when the file is missing, unreadable, or does not mention this
    plugin — the cache scan is what covers those cases.
    """
    home = _config_home()
    if home is None:
        return None
    manifest = home.joinpath(*INSTALLED_PLUGINS_REL)
    try:
        data = json.loads(manifest.read_text(encoding="utf-8"))
    except Exception:  # noqa: BLE001 — best-effort lookup with a documented
        return None    # None fallback (F-011); a hot-path helper must never
                        # take the hook process down over a host file it does
                        # not control the shape of.
    if not isinstance(data, dict):
        return None
    plugins = data.get("plugins")
    if not isinstance(plugins, dict):
        # F-011: `plugins` can be any TRUTHY non-dict (a string, a list, a
        # number) in a manifest this module does not control — `.get` on it
        # unguarded raised AttributeError here, outside every except above,
        # and crashed the hook before any fail-open/fallback path could run.
        return None
    key = f"{PLUGIN_CACHE_REL[-1]}@{PLUGIN_CACHE_REL[-2]}"
    entries = plugins.get(key)
    if not isinstance(entries, list):
        return None
    for entry in entries:
        if not isinstance(entry, dict):
            continue
        path = entry.get("installPath")
        if isinstance(path, str) and path.strip():
            candidate = Path(path.strip()).expanduser()
            if _is_plugin_root(candidate, require):
                return candidate
    return None


def find_plugin_root(require: str | None = None) -> tuple[Path | None, str | None]:
    """Resolve the forge plugin root AT RUN TIME (ADR-0012).

    A path pinned at install time carries a version number, so every upgrade
    made it stop existing and quietly downgraded the hook that depended on it.
    Sources are tried in this order, and the first one that yields a directory
    that exists on disk wins:

    1. ``CLAUDE_PLUGIN_ROOT`` — the host's own statement about the run in
       progress, and therefore the explicit override (ADR-0012, source 1).
       When the host sets it but it does not validate, that is reported once
       (F-007) rather than silently falling through — the host's declared
       intent for THIS run was overridden without a word, which is the same
       silence this card exists to remove, one level up;
    2. the hook file's OWN location, searched upwards for the plugin marker,
       bounded at the first directory that itself looks like a repository
       root (``.git``) unless that directory IS the plugin root (F-002) — a
       project nested inside an unrelated, older forge checkout must not
       inherit that checkout's plugin root merely because the walk would
       eventually reach it. Known residual (F-015): a nested project that
       does NOT itself have a ``.git`` still resolves through to the outer
       clone — near-unreachable for this caller, since a pre-commit hook by
       construction runs inside a git repository, and the resolved source is
       now always announced (F-002's real ask), so a wrong hit here is no
       longer silent. Left as-is rather than widened;
    3. ``forge_plugin_dir`` from ``config.json``, when it still resolves
       (ADR-0012, source 2) — kept as the cheap path, no longer the only one;
    4. the host's own authoritative record,
       ``<config home>/plugins/installed_plugins.json`` (F-001) — see
       :func:`_installed_plugin_root`;
    5. the host's plugin cache, newest valid installation first by
       :func:`_version_key` (ADR-0012, source 3) — the fallback for a host
       that predates ``installed_plugins.json`` or does not name this plugin
       there.

    Sources 2 and 4 are this implementation's additions to the ADR's three:
    both are live facts (the running file's own location; the host's own
    install record) rather than something inferred from a name or a stale
    pin, so neither can go stale the way a pinned path can.

    Args:
        require: Optional repo-relative file that must exist inside the root
            for it to count as valid (see :func:`_is_plugin_root`).

    Returns:
        A ``(root, source)`` pair. ``root`` is ``None`` when no source
        yields a valid one — in which case the CALLER must degrade loudly
        (ADR-0012), never silently — and then ``source`` is also ``None``.
        Otherwise ``source`` names which of the above resolved it: one of
        ``"override"``, ``"self"``, ``"config"``, ``"installed_plugins"``,
        ``"cache"``.
    """
    env = os.environ.get(PLUGIN_ROOT_ENV, "").strip()
    if env:
        candidate = Path(env).expanduser()
        if _is_plugin_root(candidate, require):
            return candidate, "override"
        missing = require if require and not (candidate / require).is_file() else str(PLUGIN_MARKER)
        warn(f"{PLUGIN_ROOT_ENV}={candidate} does not look like a usable forge "
             f"plugin install (missing {missing}) — falling back to the next source.")

    for candidate in (_HOOK_DIR, *_HOOK_DIR.parents):
        if _is_plugin_root(candidate, require):
            return candidate, "self"
        if (candidate / ".git").exists():
            break  # reached a repo root without finding the plugin inside it

    recorded = load_hook_config().get("forge_plugin_dir")
    if isinstance(recorded, str) and recorded.strip():
        candidate = Path(recorded.strip()).expanduser()
        if _is_plugin_root(candidate, require):
            return candidate, "config"

    installed_root = _installed_plugin_root(require)
    if installed_root is not None:
        return installed_root, "installed_plugins"

    candidates: list[Path] = []
    for cache in _plugin_cache_dirs():
        try:
            entries = sorted(cache.iterdir())
        except OSError:
            continue
        candidates.extend(p for p in entries if _is_plugin_root(p, require))
    if candidates:
        return max(candidates, key=_version_key), "cache"
    return None, None


def _yaml_single_quoted(text: str) -> str:
    """Render ``text`` as a one-line single-quoted YAML scalar."""
    flat = " ".join(str(text).split())
    if len(flat) > 200:
        flat = flat[:197] + "..."
    return "'" + flat.replace("'", "''") + "'"


def gate_events_path(meta: Path) -> Path:
    """Path of the generated gate-event journal for a project's ``meta`` dir."""
    return meta / GATE_EVENTS_FILE


def record_gate_event(
    meta: Path,
    gate: str,
    outcome: str,
    detail: str = "",
    *,
    now: datetime | None = None,
    elapsed: float | None = None,
) -> Path:
    """Append one gate event to ``meta/gate-events.yml`` (NFR-010).

    The commit or push that triggered this is ALLOWED regardless — the
    fail-open policy (NFR-006) is untouched. This only makes the outcome
    outlive the hook process, so ``/forge:status`` can flag it afterwards.

    Two families of outcome, one reason to journal both, and a ``kind:`` field
    so the consumer never has to conflate them: a gate that could not run
    (:data:`FAIL_OPEN_OUTCOMES` — ``kind: fail-open``) and a gate that RAN and
    deliberately said less than it saw (:data:`REPORT_OUTCOMES` — ``kind:
    report``). Both render as a clean terminal otherwise, which is why they are
    both here; they are not the same event, which is why they are labelled.

    Args:
        meta: The project's ``meta`` directory (from :func:`find_meta`).
        gate: Stable gate identifier, e.g. ``"pre-commit-ci"``.
        outcome: One of :data:`GATE_OUTCOMES` (the union of the two
            families). ``kind`` is derived from it, never passed in.
        detail: Free-text explanation shown next to the flag; flattened to one
            line and truncated.
        now: Event time; defaults to the current UTC time.
        elapsed: Seconds the gate held before this outcome (CARD-152). An
            ADDITIVE field, written only when given: readers that predate it
            skip a line they do not know (G-3), so no existing entry shape
            changes.

    Returns:
        The path written to.

    Raises:
        ValueError: On an unknown outcome or a malformed gate id — a caller
            bug, not a runtime condition.
        OSError: If the journal cannot be written. Callers on a fail-open path
            MUST catch this and say so, never let it break the operation.
    """
    if outcome not in GATE_OUTCOMES:
        raise ValueError(
            f"unknown gate outcome {outcome!r}; expected one of {GATE_OUTCOMES}")
    if not _GATE_ID_RE.match(gate or ""):
        raise ValueError(f"malformed gate id {gate!r}; expected [a-z0-9._-]")

    kind = "fail-open" if outcome in FAIL_OPEN_OUTCOMES else "report"
    entry = (f"  - gate: {gate}\n"
             f"    kind: {kind}\n"
             f"    outcome: {outcome}\n"
             f"    at: '{_utc_stamp(now)}'\n"
             f"    detail: {_yaml_single_quoted(detail)}\n")
    if elapsed is not None:
        entry += f"    elapsed: {_seconds(elapsed)}\n"
    return _append_journal_entry(gate_events_path(meta), _GATE_EVENTS_HEADER,
                                 entry, GATE_EVENTS_MAX_ENTRIES)


def _utc_stamp(now: datetime | None = None) -> str:
    """``now`` (default: the current time) as a second-precision UTC ISO stamp."""
    moment = (now or datetime.now(timezone.utc)).astimezone(timezone.utc)
    return moment.replace(microsecond=0).isoformat().replace("+00:00", "Z")


def _seconds(value: float) -> str:
    """A non-negative elapsed time as a plain YAML float, millisecond precision."""
    return f"{max(float(value), 0.0):.3f}"


def _append_journal_entry(path: Path, header: str, entry: str,
                          max_entries: int) -> Path:
    """Append one entry to a generated journal: header once, then append, trim.

    Shared by the events journal and the run journal (CARD-152) so the two
    carry the same concurrency guarantees (F-005, CARD-162, F-012) rather than
    two copies of them that can drift.
    """
    path.parent.mkdir(parents=True, exist_ok=True)
    # The header is written exactly once, by whichever process wins the
    # exclusive publish (F-005): a plain "does it exist?" check-then-act lets
    # two concurrent first writers (kanban runs several agents in parallel
    # worktrees that all resolve to this same MAIN-repo file) each conclude
    # they are first and each write an `events:` key, which ruamel's safe
    # loader then rejects as a duplicate key — the CARD-006 consumer gets
    # nothing instead of two gates' worth of signal.
    #
    # The exclusivity must be atomic WITH the header, not before it
    # (CARD-162): an `O_CREAT | O_EXCL` create followed by a separate header
    # write leaves the journal existing-but-empty in between, and a loser's
    # append into that gap is then overwritten by the header. So the would-be
    # first writer builds a sibling temp file that already carries the full
    # header and publishes it with `os.link`, which fails with
    # `FileExistsError` when someone else won — the journal can never be
    # observed without its header. The existence check is only a fast path;
    # correctness rests on the link alone.
    if not path.exists():
        _publish_gate_events_header(path, header)
    with path.open("a", encoding="utf-8") as fh:
        fh.write(entry)
    _trim_gate_events(path, max_entries, header)
    return path


def _publish_gate_events_header(path: Path, header: str = _GATE_EVENTS_HEADER) -> None:
    """Make ``path`` appear already carrying the header, or not at all (CARD-162).

    A loser of the publish (``FileExistsError``) is not an error: someone
    else's header is in place, which is all the caller needs. Any other
    ``OSError`` propagates — the fail-open contract of
    :func:`record_gate_event`. The temp file is always removed.
    """
    fd, tmp_name = tempfile.mkstemp(
        dir=str(path.parent), prefix=f".{path.name}.", suffix=".tmp")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            fh.write(header)
        try:
            os.link(tmp_name, str(path))
        except FileExistsError:
            pass
    finally:
        try:
            os.unlink(tmp_name)
        except OSError:
            pass


def _trim_gate_events(path: Path, max_entries: int = GATE_EVENTS_MAX_ENTRIES,
                      default_header: str = _GATE_EVENTS_HEADER) -> None:
    """Drop the oldest events past ``max_entries`` (F-008), best-effort.

    The journal is a signal, never a source of truth (its own header says
    so), so a failure here must not surface as a failure to RECORD the event
    that just succeeded — every exception is swallowed.

    The rewrite goes through a sibling temp file plus ``os.replace`` (F-012):
    a plain ``path.write_text`` is a truncate-then-write, which a concurrent
    APPENDER can observe half-done, and which can itself race a concurrent
    trim by another process into a torn file — exactly the failure class
    F-005 closed for the header, reopened here for the body. ``os.replace``
    is atomic on POSIX and Windows alike, so every reader always sees either
    the pre-trim or the post-trim file, never a partial one. A racer whose
    OWN append lands on the pre-trim file after it was read here still loses
    that one entry to the trim — bounded, and the accepted cost of a signal
    file that is never a source of truth (see the module docstring).
    """
    tmp: Path | None = None
    try:
        text = path.read_text(encoding="utf-8")
        blocks = re.split(r"(?=^  - gate: )", text, flags=re.M)
        header, entries = blocks[0], blocks[1:]
        if len(entries) <= max_entries:
            return
        new_header = header if header.strip() else default_header
        fd, tmp_name = tempfile.mkstemp(
            dir=str(path.parent), prefix=f".{path.name}.", suffix=".tmp")
        tmp = Path(tmp_name)
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            fh.write(new_header + "".join(entries[-max_entries:]))
        os.replace(tmp, path)
        tmp = None
    except Exception:  # noqa: BLE001 — trimming is maintenance, never load-bearing
        pass
    finally:
        if tmp is not None:
            try:
                tmp.unlink()
            except OSError:
                pass


# ── the run journal and the measured budget (CARD-152, ADR-0111/R1-R2,
#    ADR-0116/R1) ─────────────────────────────────────────────────────────
#
# WHY A SECOND FILE, and not a new kind in `gate-events.yml`: that journal is a
# fail-open SIGNAL — every entry in it reaches a ⚑ line, readers that predate a
# kind treat an unknown one as a fail-open (by design, see status.py's
# `gate_event_kind`), and it is capped at 200 entries. A per-run timing entry
# on EVERY commit would therefore (a) flag every passing commit in any older
# reader and (b) push the real fail-open events out of the cap within a day.
# Timings are a different signal with a different reader; they get their own
# generated, capped, disposable file, and the fail-open event keeps its shape
# plus one additive `elapsed:` field (G-3).

#: Generated journal of pre-commit gate run timings, next to the events one.
GATE_RUNS_FILE = "gate-runs.yml"

#: What a run can end in. `pass`/`block` are verdicts; the fail-open pair is
#: the same vocabulary the events journal uses, so the two never disagree.
GATE_RUN_OUTCOMES = ("pass", "block") + FAIL_OPEN_OUTCOMES

#: The run journal keeps more entries than the budget reads (N), so the p95
#: window is always full once N runs exist, and bounded like the events file.
GATE_RUNS_MAX_ENTRIES = 100

#: NFR-006's floor: the budget never drops below this.
PRECOMMIT_BUDGET_FLOOR_SECONDS = 60

#: N of ADR-0111/R1 ("a named constant fixed by the implementing card"): the
#: budget is derived from the last N runs, and stays at the floor until N
#: exist. 20 makes the nearest-rank p95 the second-slowest run of the window —
#: one outlier (a cold cache, a swapped laptop) does not set the budget alone.
PRECOMMIT_BUDGET_MIN_RUNS = 20

#: ADR-0116/R1's default cap, `ci.precommit_budget_cap_seconds`.
PRECOMMIT_BUDGET_CAP_DEFAULT = 300

#: The config keys (under `ci:` in meta/.skills.yml). Raising the cap above
#: the default needs the reason key set too (ADR-0116/R2, CeilingDeclaration).
PRECOMMIT_BUDGET_CAP_KEY = "precommit_budget_cap_seconds"
PRECOMMIT_BUDGET_CAP_REASON_KEY = "precommit_budget_cap_reason"

#: The explicit override (announced whenever it is used).
CI_GATE_TIMEOUT_ENV = "FORGE_CI_GATE_TIMEOUT"

_GATE_RUNS_HEADER = f"""\
# forge gate runs - GENERATED, do not edit and do not commit (.gitignore).
#
# One entry per pre-commit CI gate run, whatever its outcome (pass | block |
# timeout | failure), with the seconds it held the commit (ADR-0111/R2). The
# gate's budget is derived from the last {PRECOMMIT_BUDGET_MIN_RUNS} entries (ADR-0111/R1,
# ADR-0116/R1); /forge:status reads the same window for "gate too slow".
#
# `at` is UTC, ISO-8601. Capped at the most recent {GATE_RUNS_MAX_ENTRIES} runs - safe to
# delete wholesale (the budget falls back to {PRECOMMIT_BUDGET_FLOOR_SECONDS}s until it refills).
runs:
"""

_GATE_RUN_RE = re.compile(
    r"^  - gate: (?P<gate>\S+)\n    outcome: \S+\n    at: '[^']*'\n"
    r"    elapsed: (?P<elapsed>[0-9]+(?:\.[0-9]+)?)$", re.M)


def gate_runs_path(meta: Path) -> Path:
    """Path of the generated run-timing journal for a project's ``meta`` dir."""
    return meta / GATE_RUNS_FILE


def record_gate_run(meta: Path, gate: str, outcome: str, elapsed: float,
                    *, now: datetime | None = None) -> Path:
    """Journal one gate run and the seconds it took (ADR-0111/R2).

    Raises:
        ValueError: unknown outcome or malformed gate id (a caller bug).
        OSError: the journal cannot be written — callers MUST catch it; a
            timing record never decides a commit.
    """
    if outcome not in GATE_RUN_OUTCOMES:
        raise ValueError(
            f"unknown run outcome {outcome!r}; expected one of {GATE_RUN_OUTCOMES}")
    if not _GATE_ID_RE.match(gate or ""):
        raise ValueError(f"malformed gate id {gate!r}; expected [a-z0-9._-]")
    entry = (f"  - gate: {gate}\n"
             f"    outcome: {outcome}\n"
             f"    at: '{_utc_stamp(now)}'\n"
             f"    elapsed: {_seconds(elapsed)}\n")
    return _append_journal_entry(gate_runs_path(meta), _GATE_RUNS_HEADER,
                                 entry, GATE_RUNS_MAX_ENTRIES)


def gate_run_elapsed(meta: Path, gate: str) -> list[float]:
    """Elapsed seconds of ``gate``'s journaled runs, oldest first.

    A line scan of the single producer's fixed shape (no YAML library needed,
    ADR-0011/R1). A missing journal is an empty history; an entry that does
    not match the shape is not a measurement and is skipped.
    """
    try:
        text = gate_runs_path(meta).read_text(encoding="utf-8")
    except FileNotFoundError:
        return []
    return [float(m.group("elapsed")) for m in _GATE_RUN_RE.finditer(text)
            if m.group("gate") == gate]


def p95(samples) -> float:
    """Nearest-rank 95th percentile: the ceil(0.95 × n)-th smallest sample."""
    ordered = sorted(samples)
    if not ordered:
        raise ValueError("p95 of no samples")
    return ordered[max(math.ceil(0.95 * len(ordered)), 1) - 1]


def precommit_budget(samples, cap: float) -> float:
    """ADR-0116/R1's formula over a run history (pure).

    ``min(max(floor, 2 × p95 of the last N), cap)``, and the floor itself while
    fewer than N runs exist.
    """
    window = list(samples)[-PRECOMMIT_BUDGET_MIN_RUNS:]
    if len(window) < PRECOMMIT_BUDGET_MIN_RUNS:
        return float(PRECOMMIT_BUDGET_FLOOR_SECONDS)
    return float(min(max(PRECOMMIT_BUDGET_FLOOR_SECONDS, 2 * p95(window)), cap))


def precommit_budget_cap(meta: Path) -> tuple[float, str | None]:
    """The declared cap and, when the declaration was not honoured, why.

    Returns ``(cap, note)``. ``note`` is None when the cap is the default or a
    valid declaration; otherwise it says what was ignored — a value that is not
    a number of seconds at or above the floor, a cap raised above the default
    without ``precommit_budget_cap_reason`` (ADR-0116/R2), or a config that
    could not be read. Never raises: a bad config degrades to the default,
    loudly.
    """
    default = float(PRECOMMIT_BUDGET_CAP_DEFAULT)
    cfg_path = meta / ".skills.yml"
    if not cfg_path.is_file():
        return default, None
    if not yaml_available():
        return default, (f"{cfg_path.name} not read (no YAML library) — using "
                         f"the default cap {default:g}s")
    parsed, cfg = load_yaml_checked(cfg_path)
    if not parsed:
        return default, (f"{cfg_path.name} is unreadable (YAML parse error) — "
                         f"using the default cap {default:g}s")
    # parsed with cfg None = an empty / comment-only document: nothing declared.
    ci = cfg.get("ci") if isinstance(cfg, dict) else None
    if not isinstance(ci, dict) or PRECOMMIT_BUDGET_CAP_KEY not in ci:
        return default, None
    raw = ci.get(PRECOMMIT_BUDGET_CAP_KEY)
    try:
        # float() raises OverflowError for an int wider than a float.
        valid = (isinstance(raw, (int, float)) and not isinstance(raw, bool)
                 and math.isfinite(float(raw))
                 and raw >= PRECOMMIT_BUDGET_FLOOR_SECONDS)
        raw_repr = repr(raw) if len(repr(raw)) <= 40 else f"<{type(raw).__name__} too large>"
    except (OverflowError, ValueError, TypeError):
        valid, raw_repr = False, f"<{type(raw).__name__} too large>"
    if not valid:
        return default, (f"ignoring ci.{PRECOMMIT_BUDGET_CAP_KEY}={raw_repr} (not a "
                         f"number of seconds >= {PRECOMMIT_BUDGET_FLOOR_SECONDS}) "
                         f"— using the default cap {default:g}s")
    reason = ci.get(PRECOMMIT_BUDGET_CAP_REASON_KEY)
    if raw > default and not (isinstance(reason, str) and reason.strip()):
        return default, (f"ignoring ci.{PRECOMMIT_BUDGET_CAP_KEY}={raw:g}: a cap "
                         f"raised above {default:g}s needs "
                         f"ci.{PRECOMMIT_BUDGET_CAP_REASON_KEY} (ADR-0116/R2) — "
                         f"using the default cap {default:g}s")
    return float(raw), None


def precommit_gate_budget(meta: Path, env=None) -> tuple[float, str]:
    """The CI gate's budget for this run, and a one-line account of it.

    ``FORGE_CI_GATE_TIMEOUT`` (a positive number) overrides everything; a
    nonsense value is ignored, and the account says so. Otherwise the budget
    is :func:`precommit_budget` over the journaled ``pre-commit-ci`` runs under
    :func:`precommit_budget_cap`. A run journal that cannot be read is an empty
    history (the floor) — the account says that too. Never raises.
    """
    env = os.environ if env is None else env
    notes: list[str] = []
    raw = str(env.get(CI_GATE_TIMEOUT_ENV, "") or "").strip()
    if raw:
        try:
            seconds = float(raw)
        except ValueError:
            seconds = 0.0
        if seconds > 0 and math.isfinite(seconds):
            return seconds, (f"overridden via {CI_GATE_TIMEOUT_ENV} (the measured "
                             "budget is not used)")
        notes.append(f"ignoring {CI_GATE_TIMEOUT_ENV}={raw!r} (not a positive "
                     "number of seconds)")
    cap, cap_note = precommit_budget_cap(meta)
    if cap_note:
        notes.append(cap_note)
    try:
        samples = gate_run_elapsed(meta, "pre-commit-ci")
    except (OSError, UnicodeDecodeError) as e:
        samples = []
        notes.append(f"run journal unreadable ({e!r}) — treated as empty")
    budget = precommit_budget(samples, cap)
    n = PRECOMMIT_BUDGET_MIN_RUNS
    if len(samples) < n:
        basis = (f"floor: {len(samples)} of {n} runs journaled, cap {cap:g}s")
    else:
        basis = (f"min(max({PRECOMMIT_BUDGET_FLOOR_SECONDS}s, 2 × p95 "
                 f"{p95(samples[-n:]):g}s of the last {n} runs), cap {cap:g}s)")
    return budget, "; ".join([basis] + notes)


def find_meta() -> Path | None:
    """Resolve the project's meta dir; None when this is not a forge project.

    Order: FORGE_META_DIR env → <cwd>/meta → walk up the parents → the MAIN
    repo when cwd is a git worktree (kanban runs implementation agents in
    worktrees; hooks must still see the main repo's meta)."""
    env = os.environ.get("FORGE_META_DIR")
    if env:
        p = Path(env)
        if p.is_dir():
            return p

    cur = Path.cwd()
    for candidate in [cur, *cur.parents]:
        if (candidate / "meta").is_dir():
            return candidate / "meta"
        if (candidate / ".git").exists():
            break  # reached the repo root without finding meta/

    # Worktree case: resolve the main repository via the common git dir.
    try:
        out = subprocess.run(
            ["git", "rev-parse", "--git-common-dir"],
            capture_output=True, text=True, timeout=5,
        )
        common = out.stdout.strip()
        if out.returncode == 0 and common and common not in (".git",):
            main_root = Path(common).resolve().parent
            if (main_root / "meta").is_dir():
                return main_root / "meta"
    except Exception:
        pass
    return None


def forge_paths(meta: Path) -> dict[str, Path]:
    """Resolved artifact paths. Overrides come from config.json `paths`
    (repo-root-relative, mirrored from meta/.skills.yml by /forge:init hooks);
    defaults match shared/config-schema.md."""
    root = meta.parent
    rel = {
        "architecture": "meta/architecture",
        "kanban": "meta/kanban",
        "backlog": "meta/kanban/backlog.md",
        "requirements": None,   # derived below
        "skills_yml": "meta/.skills.yml",
    }
    cfg_paths = load_hook_config().get("paths") or {}
    for key in ("architecture", "kanban", "backlog"):
        v = cfg_paths.get(key)
        if isinstance(v, str) and v.strip():
            rel[key] = v.strip().rstrip("/")
    p = {k: root / v for k, v in rel.items() if v}
    p["decisions"] = p["architecture"] / "decisions"
    p["cards"] = p["kanban"] / "cards"
    p["trace"] = p["architecture"] / "trace.yml"
    p["requirements"] = p["architecture"] / "requirements.yml"
    p["retro"] = p["kanban"] / "retro.md"
    return p


def to_relative(path: str) -> str:
    cwd = os.getcwd()
    if path.startswith(cwd):
        return path[len(cwd):].lstrip("/\\")
    return path


def command_matches(command: str, *words: str) -> bool:
    """True when the shell command actually runs `git <verb>` — word-boundary
    match instead of a naive substring, so `git -C x push`, `git   push`,
    `git commit -am` all match, while `echo "git push"` (quoted) does not."""
    # strip single/double-quoted segments so quoted mentions don't false-positive
    stripped = re.sub(r"'[^']*'|\"[^\"]*\"", "", command)
    verb = words[-1]
    pattern = r"\bgit\b(?:\s+-[A-Za-z]\s+\S+|\s+--?[\w-]+(?:=\S+)?)*\s+" + re.escape(verb) + r"\b"
    return re.search(pattern, stripped) is not None


def load_yaml_checked(path: Path) -> tuple[bool, object]:
    """YAML load that tells "parsed to nothing" from "failed to parse".

    Returns ``(ok, doc)``. ``ok`` is False when no YAML library is installed,
    the file cannot be read, or it does not parse; ``ok`` is True with
    ``doc=None`` for a valid empty or comment-only document.
    """
    try:
        from ruamel.yaml import YAML
        return True, YAML(typ="safe").load(path.read_text(encoding="utf-8"))
    except ImportError:
        try:
            import yaml
            return True, yaml.safe_load(path.read_text(encoding="utf-8"))
        except Exception:
            return False, None
    except Exception:
        return False, None


def load_yaml(path: Path):
    """YAML load with library fallback; None on failure. Callers must treat a
    None from an EXISTING file as 'cannot verify' and say so — not as 'all clear'.
    (An empty document is also None here; :func:`load_yaml_checked` separates
    the two.)"""
    return load_yaml_checked(path)[1]


def yaml_available() -> bool:
    try:
        import ruamel.yaml  # noqa: F401
        return True
    except ImportError:
        try:
            import yaml  # noqa: F401
            return True
        except ImportError:
            return False


class _NullStream:
    """A sink for the case where this process has no error stream at all.

    Not `open(os.devnull)`: acquiring a descriptor is one more thing that can
    fail (and one more thing to close) on the path whose whole job is that
    nothing here may raise. Discarding a warning is a loss; corrupting the
    verdict on stdout is a defect.
    """

    def write(self, text: str) -> int:
        return len(text)

    def flush(self) -> None:
        pass


_NULL_STREAM = _NullStream()


def _err_stream():
    """The stream human text goes to — never ``None``, and never stdout.

    A descriptor closed AT PROCESS START (`sh -c 'exec python3 hook 2>&-'` —
    not the same thing as a pipe whose reader went away) makes `sys.stderr`
    literally ``None``, and `print(..., file=None)` does not fail: it resolves
    to `sys.stdout`. A human line emitted that way lands on the channel the
    consumer parses as JSON, and the PreToolUse deny object stops parsing —
    measured on six routes of `hooks/pre-commit.py` (deny, clean, skipped file,
    pragma suppression, scan-timeout fail-open, bad-budget warning), 6/6
    contaminated, 6/6 unparseable, while exit codes stayed inside the declared
    {0, 2}. So the stream is resolved HERE, once, and no caller IN THAT HOOK
    writes `file=sys.stderr` again: the live `sys.stderr`, else the
    interpreter's original one, else a sink that discards. That last branch is
    not dead code — measured, `sh -c 'exec python3 … 2>&-'` leaves BOTH
    `sys.stderr` and `sys.__stderr__` None, so it is the branch that runs
    there, and the alternative it avoids is an OSError from writing to a closed
    descriptor inside `warn` — a crash on the one path whose contract is that
    nothing may raise.

    WHERE THIS GUARD IS, AND WHERE IT IS NOT — k of n, because ADR-0022/R2 asks
    for the state of every call site and a property-shaped claim ("no caller
    writes `file=sys.stderr` again") read over a 1-of-9 reality is exactly what
    that rule forbids. The guarded operation is "a hook emitting human text
    when the error descriptor may be closed at process start". n is the 9 hook
    modules in this directory that emit human text on the error stream;
    `hooks/prompt-language.py` is the tenth hook and is not among them (its
    output is a stdout instruction, by design), and this file is the shared
    helper, not a hook.

    **1 of 9.** Applied in `hooks/pre-commit.py` (6 call sites), the ONLY hook
    that emits a machine verdict on stdout — checked, not assumed: it is the
    sole hit for `hookSpecificOutput`/`permissionDecision` in this directory.
    Seven of the other eight import from this very module and still write
    `file=sys.stderr` directly, at 15 sites:

        post-adr-write.py  :71 :76 :91          (3)
        post-code-change.py:66 :88              (2)
        post-edit.py       :59                  (1)
        post-pull.py       :201 :205 :208 :211  (4)
        post-wave-done.py  :75                  (1)
        pre-push.py        :64 :65              (2)
        session-start.py   :64 :66              (2)

    Those 15 are OUT OF SCOPE for CARD-005 and CARRIED, deliberately and not by
    oversight: none of the seven emits a stdout verdict object, so today there
    is no verdict for stray human text to corrupt, and G-3 freezes two of them
    (`pre-push.py`, `post-pull.py`) outright. They are a wave-level family with
    their own card. Revisit the moment a SECOND hook grows a stdout verdict —
    at that point the guard has to move with it.

    The ninth, `hooks/agent-invocation.py` (CARD-077, AGG-016/COMP-078), writes
    `file=sys.stderr` directly at 9 sites and does NOT import from this module
    — it is a single-file writer by design (its own docstring explains why).
    CARRIED for the same reason as the other seven — no stdout verdict object
    exists in it to corrupt — and for a second, narrower one specific to it:
    its actual PostToolUse hook entry point (`_run_hook_mode`) never writes to
    ANY stream at all (fail-open and silent, always exit 0); every one of its
    9 raw-stderr sites lives in `--parity-check`, a separate, standalone CLI
    mode a human or CI invokes directly — never a PostToolUse payload Claude
    Code parses, so there is no closed-descriptor path into it at all, not
    merely no verdict on it.

    One mechanism named and NOT asserted, because it could not be verified from
    inside this repository: `.claude/settings.json` wires `pre-push.py` and
    `pre-commit.py` on the same PreToolUse Bash matcher, so a line like
    `git add -A && git commit -m x && git push` runs both. IF the consumer
    concatenates hook stdout, pre-push's human text (emitted only when blocking
    decisions exist) would land beside pre-commit's deny object under a closed
    descriptor. The consumer's aggregation is asserted nowhere, so this is a
    reason to declare the twins here rather than a second claim.

    `argparse.print_usage`/`traceback.print_exc` resolve a ``None`` file to
    `sys.stdout`/`sys.stderr` on their own, BEFORE any override this module
    could install, so they would reopen the same hole. Neither is imported by
    any hook in this directory (checked, not assumed); if one is ever added it
    must be passed this stream explicitly.
    """
    stream = sys.stderr if sys.stderr is not None else sys.__stderr__
    return stream if stream is not None else _NULL_STREAM


def warn(msg: str) -> None:
    print(f"forge: {msg}", file=_err_stream())


# ── secret scanning (CARD-005, NFR-009, TM-010) ───────────────────────────
#
# WHAT THIS IS. A deterministic, stdlib-only detector for credentials that
# announce themselves STRUCTURALLY — a vendor prefix, a PEM header, a
# keyword-anchored assignment. It exists because a generated artifact (a review
# report, an eval case, documentation) can inherit a secret from the project's
# own files and carry it into repository history (TM-010).
#
# THE TRADEOFF, STATED. Precision is bought at the cost of recall, deliberately.
# A scanner that fires on every base64 blob teaches its users to bypass the
# hook, and a bypassed hook detects nothing at all — strictly worse than a
# narrower one. So:
#
#   * every rule is anchored on something a secret STATES about itself (an
#     `AKIA` prefix, `-----BEGIN … PRIVATE KEY-----`, `token = "…"`), never on
#     "this string looks random";
#   * the one entropy-based rule (`generic_high_entropy_assignment`) fires only
#     behind a key name drawn from a CLOSED, enumerated list
#     (`_SECRET_KEY_NAMES` — `password`, `token`, `api_key`, `client_secret`,
#     `private_key`, `secret_key`/`secret_key_base`, `signing_key`,
#     `encryption_key`, `master_key`, `account_key`, …), never behind "a name
#     containing the word key", and then only after a placeholder/pathish
#     filter and an entropy floor;
#   * `SECRET_ALLOW_PRAGMA` is the escape hatch, scoped to one line — the
#     alternative to a narrow, cheap suppression is a user disabling the gate.
#     A suppressed match is COUNTED and reported (`SecretScanReport.suppressed`)
#     — the pragma silences the block, not the gate's own account of itself.
#
# DECLARED GAPS (ADR-0022/R1 — no claim without evidence, and no silence about
# what is NOT claimed):
#
#   * binary files are SKIPPED, not scanned (a NUL byte in the first
#     `_BINARY_SNIFF_BYTES`) — so UTF-16 text is skipped too;
#   * files over `SECRET_SCAN_MAX_BYTES` are SKIPPED, never truncated (a
#     truncated scan reports "clean" about bytes it never read);
#   * unquoted YAML/JSON values are covered only for the charset
#     `[A-Za-z0-9_.\-/+=]` and must end at whitespace, `;` or `,`; a value with
#     spaces in it is not covered;
#   * a passphrase with no digit in it does not clear the entropy rule's
#     mixed-class requirement;
#   * key names OUTSIDE `_SECRET_KEY_NAMES` are not covered, deliberately: a
#     bare `…_key` alternation would fire on `cache_key`, `primary_key`,
#     `partition_key`, `idempotency_key`. A credential parked under a name this
#     list does not carry is MISSED, and that is the price of that precision;
#   * whitespace-separated credential syntax is not covered — `.netrc`'s
#     `machine … password <value>` and `curl -u user:pw` are MISSED, because
#     anchoring on `password` followed by a space fires on English prose;
#   * `Authorization:` is covered only for the `Basic`/`Bearer`/`Token`
#     schemes and only above the same entropy floor; a short opaque token in a
#     header is MISSED;
#   * a `Basic` value is DECODED before it is judged, and the entropy floor is
#     applied to the decoded PASSWORD, because judging the base64 armour fired
#     on RFC 7617's own `username:password` and `Aladdin:open sesame`. The
#     declared cost is the mirror of that: a real Basic credential whose
#     plaintext password is under 20 characters, single-class, or carries a
#     placeholder word is MISSED;
#   * the key names this list gained (`signing_key`, `encryption_key`,
#     `master_key`, `account_key`, …) also reach values that are PUBLISHED
#     identifiers of a key rather than the key: an all-hex fingerprint/digest
#     (32/40/64/128 chars) and a leading `arn:` are rejected by
#     `_is_key_identifier`. Declared cost: a real credential of exactly one of
#     those hex lengths, or one starting with `arn:`, is MISSED under those
#     names. A JWKS `kid` needs no carve-out — `kid` is not in the list;
#   * Stripe TEST-mode keys (`sk_test_…`, `pk_test_…`, `rk_test_…`) are
#     deliberately NOT flagged — they grant no access, and flagging them fires
#     on Stripe's own documentation. This is now IMPLEMENTED in BOTH rules that
#     can reach them (`_STRIPE_TEST_KEY_RE`), not only in the vendor rule: for
#     three cycles the generic keyword-anchored rule contradicted the
#     declaration by firing on Stripe's own quickstart line
#     `stripe.api_key = "sk_test_…"`, because `api_key` is in
#     `_SECRET_KEY_NAMES`. Declared cost: a value under a secret-shaped key
#     name beginning with a Stripe TEST prefix is MISSED;
#   * base64 armour around a PLACEHOLDER is decoded before it is judged
#     (`_decodes_to_placeholder`) — a Kubernetes Secret manifest's
#     `password: Y2hhbmdlbWUtcGxhY2Vob2xkZXI=` is "changeme-placeholder", which
#     `_PLACEHOLDER_RE` already rejects in the clear. The same move
#     `_decoded_basic_pair` makes for `Authorization: Basic`. Declared cost: a
#     real credential that is valid base64 decoding to printable text carrying
#     a placeholder word is MISSED; a manifest holding a REAL base64 credential
#     still fires;
#   * the same argument is now IMPLEMENTED for AWS: a value carrying the
#     literal word `EXAMPLE` is rejected by both AWS rules
#     (`_is_not_aws_documentation`), because AWS's own documented sample id and
#     40-char secret both carry it and are copied into countless READMEs. The
#     declared cost: a real AWS-assigned key that happens to contain that word
#     is MISSED;
#   * a URL password made only of upper-case letters, digits and separators
#     (`postgres://u:DB_PASSWORD@…`) is treated as a documentation stand-in and
#     MISSED — that shape is a template, and a real all-caps password is the
#     rarer case;
#   * a secret assembled at run time from parts is out of reach of any
#     regex scanner and is not claimed;
#   * PRECISION COST, measured on an independent 51-item innocent corpus and
#     named rather than fixed: a value that is CIPHERTEXT committed on purpose
#     — a sealed-secrets or SOPS-encrypted blob under a secret-shaped key name
#     — still fires. By shape it is exactly a real credential under a real
#     credential name, which is what this rule is for; the answer is the
#     pragma, not a weaker rule. (The same corpus is otherwise silent: 50 of
#     51 items, including the k8s placeholder and Stripe test-key classes this
#     card closed.)
#   * FOUR MORE PRECISION COSTS, from a SECOND independent corpus (53 items,
#     written for cycle 4 out of terraform/helm/k8s config, Go/JS/TS/Python/SQL
#     code, docs, hashes, ids, lockfiles and dev fixtures). Declared here with
#     the shapes named here and spelled out VERBATIM in
#     `DECLARED_PRECISION_COSTS`
#     (`tests/test_precommit_secret_scan.py`), which is where they are
#     executed, so that closing one of them flips a test rather than passing in
#     silence. They are elided in this block for the reason the block itself
#     describes: written out in full they fire on THIS file, and a declaration
#     that has to be pragma'd to be readable is a worse declaration. All four
#     re-measured on this delta — each still fires:
#
#       a dev-database DSN whose password is the same word as its user
#       (`postgres://<user>:<the same word>@localhost:5432/app_dev`)
#           -> url_embedded_credential. The boundary was measured, so the class
#           is sized rather than asserted: `myapp`, `admin`, `guest`, `root`
#           and `elastic` paired with themselves are all SILENT, and two
#           separate bounds do it — the rule's own `{6,}` on the password stops
#           the four short ones before any validator runs, and
#           `_is_url_credential`'s 8-character floor stops `elastic` (7). So
#           `postgres` (8) is the shortest common pair that clears BOTH: the
#           class is narrow, and its one member is the most-copied dev DSN
#           there is.
#       an argon2 password HASH under `password` (`$argon2id$v=19$<params>$
#       <salt>$<hash>`)
#           -> generic_high_entropy_assignment. Published on purpose in seed
#           data and fixtures, under the very key name the rule is anchored on.
#       a dashed UUID under `session_key`
#           -> generic_high_entropy_assignment. The key name was ADDED by this
#           card; `_is_key_identifier` rejects bare hex of 32/40/64/128
#           characters, and a dashed UUID is none of those.
#       a fixture value under `auth_key` whose words say "test only"
#           -> generic_high_entropy_assignment. The words say fixture; the rule
#           reads shape, and `_PLACEHOLDER_RE` carries no `test` alternative —
#           deliberately, since one would silence every value a project happens
#           to name `test…`, real ones included.
#
#     Not fixed here on a deliberate scoping ruling, not because the cost is
#     small: each carve-out is a new way for a real credential to hide (a
#     password equal to a username, a value that starts `$argon2…`, a
#     UUID-shaped key). Tracked for a follow-up card in
#     `meta/kanban/backlog.md`. What matters more than the rate: under the
#     unconditional selection in `hooks/pre-commit.py` ONE such line anywhere
#     in the working tree blocks every commit in that repository until it is
#     gitignored (the refusal names that remedy) or pragma'd.
#
# Evidence for every rule is named in `SecretPattern.evidence` and checked
# executably by `tests/test_precommit_secret_scan.py`.

#: A finding on a line carrying this marker — or on the line immediately after
#: one — is suppressed. Deliberately line-scoped: a file-level pragma silences
#: the file's future content too.
SECRET_ALLOW_PRAGMA = "forge:allow-secret"

#: Files larger than this are skipped whole, never truncated (see gaps above).
SECRET_SCAN_MAX_BYTES = 2 * 1024 * 1024

#: A NUL in this many leading bytes means "binary" — skip, do not scan.
_BINARY_SNIFF_BYTES = 8192

#: Cap on findings carried out of one scan; the count is still exact.
SECRET_SCAN_MAX_FINDINGS = 20

#: Values that name a secret instead of being one.
_PLACEHOLDER_RE = re.compile(
    r"(?i)(example|sample|changeme|change[_-]me|placeholder|redact|dummy|fake"
    r"|your[_-]?|todo|xxxx|<[^>]*>|\$\{|\{\{|%\(|not[_-]?a?[_-]?real)")

#: Values that are a location OF a secret, not the secret.
_PATHISH_RE = re.compile(
    r"^(?:[~.]?/|[A-Za-z]:\\|https?://)"
    r"|^[\w.+\-]+\.(?:pem|key|json|ya?ml|txt|md|py|crt|cfg|ini|env)$"
    r"|^[\w.+\-/]*/[\w.+\-/]*$")

#: URL-credential values that every runbook uses as a stand-in. Compared
#: case-INSENSITIVELY (`_is_url_credential` lower-cases first), so `PASSWORD`
#: and `Password` are stand-ins too.
_URL_CRED_PLACEHOLDERS = frozenset(
    {"password", "passwd", "pass", "secret", "token", "hunter", "changeme"})

#: The other half of the same class, which an exact-name set cannot reach: a
#: URL password written in SCREAMING_SNAKE (`DB_PASSWORD`, `PASSWORD_HERE`,
#: `ACCESSKEY`) is a template slot, not a password. Requires at least one
#: letter, so an all-digit PIN is still judged on its own merits.
_URL_CRED_ALLCAPS_RE = re.compile(r"^(?=[^a-z]*[A-Z])[A-Z0-9]+(?:[_.\-][A-Z0-9]+)*$")

#: AWS publishes ONE example access-key id and ONE example 40-char secret, both
#: carrying this word, and they are copied verbatim into READMEs, terraform
#: guides and (measurably) review reports about this very scanner.
_AWS_DOC_MARKER = "EXAMPLE"

#: Stripe TEST-mode keys. The module has declared them out of scope since the
#: first version — "they grant no access, and flagging them fires on Stripe's
#: own documentation" — but only the `stripe_live_key` RULE honoured it: the
#: generic keyword-anchored rule fired on Stripe's own quickstart snippet
#: (`stripe.api_key = "sk_test_4eC39HqLyjWDarjtT1zdp7dc"`), on
#: `STRIPE_API_KEY=sk_test_…` and on `api_key: sk_test_51…`, because `api_key`
#: IS in `_SECRET_KEY_NAMES`. A declaration one rule honours and another
#: contradicts is not a declaration (ADR-0022/R1), so it is implemented here —
#: the same shape as `_is_not_aws_documentation`.
#:
#: Declared cost: a value under a secret-shaped key name that begins
#: `sk_test_` / `pk_test_` / `rk_test_` is MISSED. Such a value IS a Stripe
#: test key; the LIVE prefixes stay covered by `stripe_live_key` AND by the
#: generic rule.
_STRIPE_TEST_KEY_RE = re.compile(r"(?i)^[sprk]k_test_[0-9A-Za-z]{4,}$")


def shannon_entropy(text: str) -> float:
    """Bits of Shannon entropy per character of ``text`` (0.0 when empty)."""
    if not text:
        return 0.0
    total = len(text)
    entropy = 0.0
    for count in Counter(text).values():
        p = count / total
        entropy -= p * math.log2(p)
    return entropy


def _is_high_entropy_secret(value: str) -> bool:
    """True when a keyword-anchored value looks like a real credential.

    Five filters, each one paying for itself against a class of false positive
    the reviewer will otherwise meet on day one: a length floor (`REDACTED`,
    `changeme`), a placeholder denylist (`${VAR}`, `<your-key>`, `…EXAMPLE…`),
    a pathish/URL filter (`/etc/ssl/private/server.key`), a mixed-class
    requirement (prose), and an alphabet-width + entropy floor (`0000…`, a
    repeated word).
    """
    if len(value) < 20:
        return False
    if _PLACEHOLDER_RE.search(value):
        return False
    if _PATHISH_RE.search(value):
        return False
    if not (any(c.isdigit() for c in value) and any(c.isalpha() for c in value)):
        return False
    if len(set(value)) < 10:
        return False
    return shannon_entropy(value) >= 3.5


def _is_jwt(value: str) -> bool:
    """True when ``value`` is a three-part JWT whose header really is one.

    The shape `eyJ….eyJ….sig` alone is not enough: the header segment must
    base64url-decode to a JSON object carrying `alg`. That single structural
    check is what separates a token from a dotted base64 string in a document.
    """
    parts = value.split(".")
    if len(parts) != 3 or not all(parts):
        return False
    head = parts[0]
    try:
        raw = base64.urlsafe_b64decode(head + "=" * (-len(head) % 4))
        header = json.loads(raw.decode("utf-8"))
    except Exception:  # noqa: BLE001 — any decode failure means "not a JWT"
        return False
    return isinstance(header, dict) and "alg" in header


def _is_url_credential(value: str) -> bool:
    """True when a `user:pw@host` password is not a documentation stand-in.

    Three filters, in the order they pay off: a length floor, the exact-name
    denylist (case-insensitively — `PASSWORD` is the same stand-in as
    `password`), the SCREAMING_SNAKE template shape that the name list cannot
    enumerate, and finally the shared placeholder vocabulary.
    """
    if len(value) < 8 or value.lower() in _URL_CRED_PLACEHOLDERS:
        return False
    if _URL_CRED_ALLCAPS_RE.match(value):
        return False
    return not _PLACEHOLDER_RE.search(value)


def _is_not_aws_documentation(value: str) -> bool:
    """False for AWS's own published sample credentials.

    Both AWS rules key on a vendor prefix / key name rather than on entropy, so
    neither ever consulted the placeholder filter — and AWS's documented
    example id (`…7EXAMPLE`) and 40-char secret (`…EXAMPLEKEY`) are therefore
    the single most-copied string this scanner could fire on. This is the same
    argument the Stripe TEST-key carve-out already makes, implemented rather
    than only stated. Declared cost: a real AWS-assigned key containing the
    word EXAMPLE is missed.
    """
    return _AWS_DOC_MARKER not in value.upper()


def _decoded_basic_pair(value: str) -> tuple | None:
    """``(user, password)`` when ``value`` is an RFC 7617 Basic credential.

    The `Authorization` rule captured a base64 blob and judged it as an opaque
    string, so RFC 7617's OWN examples cleared the entropy floor: the encoding
    of `Aladdin:open sesame` is 26 mixed-class characters with 19 distinct
    ones. Decoding is the only way to see what is actually in there.

    Returns None when the value is not a decodable `user:password` pair — a
    Bearer/Token value, or a Basic value that is not base64 — in which case the
    caller falls back to judging the encoded form.
    """
    if len(value) % 4 or not value:
        return None
    try:
        raw = base64.b64decode(value, validate=True)
        text = raw.decode("utf-8")
    except Exception:  # noqa: BLE001 — anything undecodable is simply not a pair
        return None
    if text.count(":") != 1 or not text.isprintable():
        return None
    user, password = text.split(":", 1)
    return (user, password) if user and password else None


def _is_authorization_credential(value: str) -> bool:
    """True when an `Authorization:` value is a credential, not a JWT or a slot.

    JWTs are excluded on purpose: the `jwt` rule already reports them, with a
    structural check this one cannot make, and one credential should produce
    one finding.

    A `Basic` value is DECODED first (FIX 3): judged as an opaque base64 blob,
    this rule fired on RFC 7617 §2's own examples — `username:password`,
    `Aladdin:open sesame` — and on `test:test1234567890`,
    `admin:admin123456789012`, inside curl one-liners and fenced ```http
    blocks. That is precisely the AWS-EXAMPLE class the module carves out one
    rule over, so it gets the same carve-out: the decoded pair is held to the
    filters a credential must clear, and a documentation pair fails them.

    DECLARED COST (the mirror of that precision): a REAL Basic credential whose
    decoded password is shorter than 20 characters, is single-class, or carries
    a placeholder word is MISSED. That is the same floor `_is_high_entropy_secret`
    already imposes everywhere else, now applied to the plaintext instead of to
    its base64 armour — where it means what it says.
    """
    if _is_jwt(value):
        return False
    pair = _decoded_basic_pair(value)
    if pair is not None:
        user, password = pair
        if _PLACEHOLDER_RE.search(user) or _PLACEHOLDER_RE.search(password):
            return False
        if password.lower() in _URL_CRED_PLACEHOLDERS:
            return False
        if user.lower() in _URL_CRED_PLACEHOLDERS:
            return False
        return _is_high_entropy_secret(password)
    return _is_high_entropy_secret(value)


#: A digest/fingerprint written under a `*_key` name: an all-hex value of a
#: hash length. A 40-hex GPG fingerprint under `signing_key` is PUBLISHED on
#: purpose — it names a key, it is not one.
_KEY_IDENTIFIER_HEX_RE = re.compile(r"(?i)^[0-9a-f]{32}$|^[0-9a-f]{40}$"
                                    r"|^[0-9a-f]{64}$|^[0-9a-f]{128}$")


def _is_key_identifier(value: str) -> bool:
    """True when a `*_key` value NAMES a key rather than being one.

    Two shapes, both of them things people publish deliberately, and both of
    them newly reachable once `_SECRET_KEY_NAMES` grew `signing_key` /
    `encryption_key`:

    * an all-hex value of a digest length — a GPG fingerprint, a key id, a
      content hash (`signing_key: 3AA5C343…ABCDEF01`);
    * a leading `arn:` — a KMS key ARN is a resource identifier, and the whole
      point of it is to appear in configuration and documentation.

    Declared cost: a real credential that happens to be exactly 32/40/64/128
    hex characters, or to start with `arn:`, is MISSED under these key names.
    """
    return bool(_KEY_IDENTIFIER_HEX_RE.match(value)
                or value.lower().startswith("arn:"))


def _decodes_to_placeholder(value: str) -> bool:
    """True when ``value`` is base64 armour around a PLACEHOLDER.

    The same move `_decoded_basic_pair` makes for `Authorization: Basic`, one
    rule over, and for the same reason: base64 hides a plaintext the module
    already rejects. A Kubernetes Secret manifest, a sealed-secrets template
    or a Helm chart example is full of

        password: Y2hhbmdlbWUtcGxhY2Vob2xkZXI=   -> "changeme-placeholder"
        token: eW91ci10b2tlbi1nb2VzLWhlcmU=      -> "your-token-goes-here"
        api_key: PHlvdXItYXBpLWtleS1oZXJlPg==    -> "<your-api-key-here>"

    and every one of those plaintexts is already in `_PLACEHOLDER_RE`. Firing
    on a manifest carrying a REAL base64 credential is correct and stays;
    firing on the template shape is the false-positive class that teaches
    people to bypass the gate.

    Declared cost: a real credential that happens to be valid base64 decoding
    to PRINTABLE text containing a placeholder word is MISSED.
    """
    if len(value) % 4 or len(value) < 8:
        return False
    try:
        text = base64.b64decode(value, validate=True).decode("utf-8")
    except Exception:  # noqa: BLE001 — not base64, or not text: judge it as-is
        return False
    return text.isprintable() and bool(_PLACEHOLDER_RE.search(text))


def _is_generic_secret_assignment(value: str) -> bool:
    """The `generic_high_entropy_assignment` validator: a secret, not an id.

    Scoped to this rule rather than folded into `_is_high_entropy_secret`, so
    the digest carve-out cannot cost the Authorization rule its recall — a
    40-hex bearer token is a plausible real credential, a 40-hex `signing_key`
    is a fingerprint. Three disqualifiers, each with its own look-alike row:
    a key IDENTIFIER, a Stripe TEST key (the module's oldest declared
    carve-out, previously honoured by the vendor rule only), and base64 armour
    around a placeholder.
    """
    return (_is_high_entropy_secret(value)
            and not _is_key_identifier(value)
            and not _STRIPE_TEST_KEY_RE.match(value)
            and not _decodes_to_placeholder(value))


@dataclass(frozen=True)
class SecretPattern:
    """One detection rule, with the evidence that proves it detects (ADR-0022/R1).

    Attributes:
        id: Stable rule name, reported to the user and asserted on by tests.
        regex: Compiled rule. A named group ``secret`` (or ``secret2``) marks
            the credential itself; otherwise the whole match is the credential.
        description: What the rule claims to catch — no more than the evidence
            supports.
        evidence: Repo-relative fixture file containing a sample this rule
            matches. `PatternEvidenceProperty` fails when the file is missing
            or the rule does not actually fire on it, so the claim above can
            never outrun the code.
        validator: Optional structural check applied to the captured value; a
            rejected value is not a finding.
    """

    id: str
    regex: "re.Pattern[str]"
    description: str
    evidence: str
    validator: "Callable[[str], bool] | None" = None


@dataclass(frozen=True)
class SecretFinding:
    """One credential-shaped match, with the credential itself redacted."""

    path: str
    line: int
    pattern: str
    redacted: str

    def describe(self) -> str:
        """One-line rendering for a gate message (ASCII only, never the value)."""
        return f"{self.path}:{self.line}: {self.pattern} [{self.redacted}]"


@dataclass(frozen=True)
class SecretScanReport:
    """Outcome of scanning a set of files.

    Attributes:
        findings: Up to `SECRET_SCAN_MAX_FINDINGS` findings.
        total: Exact number of findings, even when `findings` was capped.
        skipped: ``path -> reason`` for every file NOT scanned. A skip is a
            gap in coverage and is reported as one, never as "clean".
        suppressed: How many matches an allow-pragma silenced. Same reasoning
            as `skipped`, one function over: a suppressed match is a place
            where the scan FOUND something and reported nothing, so the run
            must account for it rather than render as a clean scan. The pragma
            silences the block, not the gate's account of itself.
    """

    findings: tuple[SecretFinding, ...] = ()
    total: int = 0
    skipped: dict = field(default_factory=dict)
    suppressed: int = 0

    @property
    def clean(self) -> bool:
        return self.total == 0


#: The CLOSED list of key names the keyword-anchored rule fires behind, as a
#: regex alternation. Closed on purpose (ADR-0022/R1): a bare `\w+_key` would
#: fire on `cache_key`, `primary_key`, `partition_key`, `idempotency_key` —
#: the false-positive class that teaches people to bypass the gate. Every name
#: here either IS a credential name or is one in a named framework:
#: `secret_key` (Django/Flask), `secret_key_base` (Rails), `account_key`
#: (Azure storage connection strings), `master_key` (Rails credentials).
#: Names NOT on this list are a declared gap, not an oversight.
_SECRET_KEY_NAMES = (
    r"(?:passwd|password|secret|token|api[_-]?key|apikey"
    r"|access[_-]?key|auth[_-]?key|credential"
    r"|(?:private|signing|encryption|encrypt|master|account"
    r"|session|secret|refresh)[_-]?key(?:[_-]?base)?)")

_FIXTURES = "tests/fixtures/secrets/leaked-artifacts/artifacts"

#: The rule table. Every entry names the fixture that proves it fires.
SECRET_PATTERNS: tuple[SecretPattern, ...] = (
    SecretPattern(
        "private_key_block",
        re.compile(r"-----BEGIN (?:[A-Z0-9 ]+ )?PRIVATE KEY(?: BLOCK)?-----"),
        "PEM/OpenSSH/PGP private key header",
        f"{_FIXTURES}/deploy-key.txt",
    ),
    SecretPattern(
        "aws_access_key_id",
        re.compile(r"\b(?P<secret>(?:AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16})\b"),
        "AWS access key id (AWS's own EXAMPLE sample deliberately not flagged)",
        f"{_FIXTURES}/aws-credentials.md",
        _is_not_aws_documentation,
    ),
    SecretPattern(
        "aws_secret_access_key",
        re.compile(r"(?i)aws_?secret_?access_?key[\"'\s]*[:=]\s*"
                   r"[\"']?(?P<secret>[A-Za-z0-9/+=]{40})\b"),
        "AWS secret access key, anchored on its own key name "
        "(AWS's own EXAMPLE sample deliberately not flagged)",
        f"{_FIXTURES}/aws-credentials.md",
        _is_not_aws_documentation,
    ),
    SecretPattern(
        "github_token",
        re.compile(r"\b(?P<secret>gh[pousr]_[A-Za-z0-9]{36})\b"),
        "GitHub personal/OAuth/app/refresh token",
        f"{_FIXTURES}/vendor-tokens.yml",
    ),
    SecretPattern(
        "github_fine_grained_pat",
        re.compile(r"\b(?P<secret>github_pat_[A-Za-z0-9_]{60,})\b"),
        "GitHub fine-grained personal access token",
        f"{_FIXTURES}/vendor-tokens.yml",
    ),
    SecretPattern(
        "slack_token",
        re.compile(r"\b(?P<secret>xox[abprs]-[A-Za-z0-9-]{12,})\b"),
        "Slack bot/user/app token",
        f"{_FIXTURES}/vendor-tokens.yml",
    ),
    SecretPattern(
        "slack_webhook_url",
        re.compile(r"(?P<secret>https://hooks\.slack\.com/services/"
                   r"T[A-Za-z0-9_+/-]{20,})"),
        "Slack incoming-webhook URL (a bearer credential in a URL)",
        f"{_FIXTURES}/jwt-and-webhook.md",
    ),
    SecretPattern(
        "google_api_key",
        re.compile(r"\b(?P<secret>AIza[0-9A-Za-z_\-]{35})\b"),
        "Google API key",
        f"{_FIXTURES}/vendor-tokens.yml",
    ),
    SecretPattern(
        "stripe_live_key",
        re.compile(r"\b(?P<secret>[sr]k_live_[0-9A-Za-z]{16,})\b"),
        "Stripe LIVE secret/restricted key (test-mode keys deliberately not flagged)",
        f"{_FIXTURES}/vendor-tokens.yml",
    ),
    SecretPattern(
        "anthropic_api_key",
        re.compile(r"\b(?P<secret>sk-ant-[a-z]{3,6}[0-9]{2}-[A-Za-z0-9_\-]{32,})\b"),
        "Anthropic API key",
        f"{_FIXTURES}/vendor-tokens.yml",
    ),
    SecretPattern(
        "openai_api_key",
        re.compile(r"\b(?P<secret>sk-(?!ant-)(?:proj-)?[A-Za-z0-9_\-]{40,})\b"),
        "OpenAI API key",
        f"{_FIXTURES}/vendor-tokens.yml",
    ),
    SecretPattern(
        "npm_token",
        re.compile(r"\b(?P<secret>npm_[A-Za-z0-9]{36})\b"),
        "npm access token",
        f"{_FIXTURES}/vendor-tokens.yml",
    ),
    SecretPattern(
        "jwt",
        re.compile(r"\b(?P<secret>eyJ[A-Za-z0-9_\-]{8,}\.eyJ[A-Za-z0-9_\-]{8,}"
                   r"\.[A-Za-z0-9_\-]{8,})\b"),
        "JSON Web Token whose header really decodes to a JOSE header",
        f"{_FIXTURES}/jwt-and-webhook.md",
        _is_jwt,
    ),
    SecretPattern(
        "url_embedded_credential",
        re.compile(r"[a-zA-Z][a-zA-Z0-9+.\-]*://[^/\s:@]+:(?P<secret>[^/\s:@]{6,})@"),
        "Password embedded in a URL / connection string",
        f"{_FIXTURES}/generic-assignment.py",
        _is_url_credential,
    ),
    SecretPattern(
        "generic_high_entropy_assignment",
        re.compile(
            r"(?i)\b(?P<key>[a-z0-9_.\-]{0,32}"
            + _SECRET_KEY_NAMES + r"s?)\s*[:=]\s*"
            r"(?:(?P<q>[\"'])(?P<secret>[^\"'\s]{20,})(?P=q)"
            r"|(?P<secret2>[A-Za-z0-9_\-./+=]{20,})(?=[\s;,]|$))"),
        "High-entropy value assigned to a key named in the enumerated "
        "secret-name list (no vendor prefix; key IDENTIFIERS — hex "
        "fingerprints, ARNs — deliberately not flagged)",
        f"{_FIXTURES}/generic-assignment.py",
        _is_generic_secret_assignment,
    ),
    SecretPattern(
        "http_authorization_header",
        re.compile(r"(?i)\bauthorization\s*[:=]\s*[\"']?"
                   r"(?:bearer|basic|token)\s+"
                   r"(?P<secret>[A-Za-z0-9_\-./+=]{20,})"),
        "Credential carried in an HTTP Authorization header "
        "(JWTs are reported by the `jwt` rule instead)",
        f"{_FIXTURES}/jwt-and-webhook.md",
        _is_authorization_credential,
    ),
)


def _redact(value: str) -> str:
    """Render a credential so it identifies itself without being usable.

    The gate's own message must not become the leak it exists to stop: it
    reaches a terminal, a transcript and (through the block) the model's
    context. Four leading characters keep it recognizable; the length keeps it
    diagnosable.
    """
    head = value[:4]
    return f"{head}{'*' * min(max(len(value) - 4, 0), 8)} ({len(value)} chars)"


def _pragma_suppresses(lines: list, index: int) -> bool:
    """True when line ``index`` (0-based) is covered by an allow pragma."""
    if SECRET_ALLOW_PRAGMA in lines[index]:
        return True
    return index > 0 and SECRET_ALLOW_PRAGMA in lines[index - 1]


def scan_text_for_secrets_detailed(text: str, path: str = "<text>") -> tuple:
    """Find credential-shaped matches in ``text``, and count the silenced ones.

    Line-oriented on purpose: a finding must name a line, a pragma must scope
    to one, and a rule must not be able to straddle megabytes of a file. Line
    endings are normalised first, so CRLF and LF text yield identical findings.

    Args:
        text: The already-decoded content to scan.
        path: Label used in the findings' ``path`` field.

    Returns:
        ``(findings, suppressed)`` — a list of :class:`SecretFinding` in file
        order, and how many matches an allow-pragma silenced. A finding never
        contains the credential itself (see :func:`_redact`); a suppression is
        a COUNT and never carries the line's content anywhere.
    """
    lines = text.replace("\r\n", "\n").replace("\r", "\n").split("\n")
    findings: list = []
    suppressed = 0
    for index, line in enumerate(lines):
        for pattern in SECRET_PATTERNS:
            for match in pattern.regex.finditer(line):
                groups = match.groupdict()
                value = groups.get("secret") or groups.get("secret2") or match.group(0)
                if pattern.validator is not None and not pattern.validator(value):
                    continue
                if _pragma_suppresses(lines, index):
                    # Not a finding — but not nothing, either. The bare
                    # `continue` this replaces made the gate silent about a
                    # match it really did make.
                    suppressed += 1
                    continue
                findings.append(SecretFinding(
                    path=path, line=index + 1, pattern=pattern.id,
                    redacted=_redact(value)))
    return findings, suppressed


def scan_text_for_secrets(text: str, path: str = "<text>") -> list:
    """The findings half of :func:`scan_text_for_secrets_detailed`.

    Kept as the narrow entry point for callers that only ask "does this text
    carry a credential the gate would block?" — the suppression count is a
    property of a RUN, not of a line.
    """
    return scan_text_for_secrets_detailed(text, path)[0]


def scan_file_for_secrets(path: Path, display: str | None = None) -> tuple:
    """Scan one file's BYTES, deciding deliberately what it cannot read.

    Arbitrary repository bytes reach this function, so every non-text case is
    an explicit outcome rather than an exception escaping into a hook:

    * unreadable (OSError) -> skipped, reason reported;
    * larger than :data:`SECRET_SCAN_MAX_BYTES` -> skipped whole, never
      truncated: a truncated scan would report "clean" about bytes it never
      read;
    * NUL byte in the first :data:`_BINARY_SNIFF_BYTES` -> treated as binary
      and skipped (this is also why UTF-16 text is skipped — a declared gap);
    * otherwise decoded as UTF-8 with ``errors="replace"``, so latin-1 logs
      and mixed-encoding files still scan. Every declared pattern is ASCII, so
      a replacement character can only appear OUTSIDE a credential.

    Args:
        path: File to read.
        display: Label for the findings (defaults to ``str(path)``).

    Returns:
        ``(findings, skip_reason, suppressed)``. A skipped file yields no
        findings, a reason and no suppressions; a scanned one yields a reason
        of ``None`` and the number of matches its allow-pragmas silenced.
    """
    label = display if display is not None else str(path)
    try:
        size = path.stat().st_size
        if size > SECRET_SCAN_MAX_BYTES:
            return [], f"larger than {SECRET_SCAN_MAX_BYTES} bytes ({size})", 0
        data = path.read_bytes()
    except OSError as exc:
        return [], f"unreadable ({exc.__class__.__name__})", 0
    if b"\x00" in data[:_BINARY_SNIFF_BYTES]:
        return [], "binary (NUL byte)", 0
    found, suppressed = scan_text_for_secrets_detailed(
        data.decode("utf-8", errors="replace"), label)
    return found, None, suppressed


def scan_files_for_secrets(paths, root: Path | None = None) -> SecretScanReport:
    """Scan many files and report findings, their exact count, and every skip.

    Args:
        paths: Iterable of ``(absolute_path, display_label)`` pairs, or of
            paths relative to ``root`` when ``root`` is given.
        root: Optional base directory for relative paths.

    Returns:
        A :class:`SecretScanReport`.
    """
    findings: list = []
    skipped: dict = {}
    suppressed = 0
    for item in paths:
        if isinstance(item, tuple):
            absolute, label = item
        else:
            label = str(item)
            absolute = (root / item) if root is not None else Path(item)
        found, reason, silenced = scan_file_for_secrets(Path(absolute), label)
        if reason is not None:
            skipped[label] = reason
        suppressed += silenced
        findings.extend(found)
    return SecretScanReport(
        findings=tuple(findings[:SECRET_SCAN_MAX_FINDINGS]),
        total=len(findings),
        skipped=skipped,
        suppressed=suppressed,
    )
