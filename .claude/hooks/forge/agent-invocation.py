# forge
#!/usr/bin/env python3
"""forge agent-invocation journal writer — the SOLE writer of
``meta/agent-journal.yml`` (ADR-0037/R1, AGG-016, COMP-078).

WHAT THIS IS. A PostToolUse hook on the sub-agent spawn tool (matcher
``"Task"``, which EXP-0001 confirmed also matches the tool's current name
``"Agent"`` — CLI 2.1.251 aliases it). It writes exactly ONE journal entry per
real sub-agent spawn, operation ``invoke_agent`` (OTel GenAI semantic
conventions core — ``gen_ai.*`` only; NG-1 keeps unstable extensions like
``gen_ai.task.*`` out). Chat/execute_tool spans are out of scope (NG-2) — one
row per spawn, not per turn.

Feasibility for this exact chokepoint was NOT assumed — it was spiked first,
per ADR-0037/R2: EXP-0001 (outcome: go, 2026-08-31,
meta/experiments/EXP-0001-adr0037-chokepoint-feasibility.md) ran 10/10 PostToolUse
probes and found all four gen_ai/envelope fields directly in
``tool_response`` — no transcript reconstruction needed on the main path.
``tool_response.resolvedModel`` / ``.usage.input_tokens`` / ``.usage.output_tokens``
/ ``.content[0].text`` map onto the schema below one-for-one.

FIELD SCHEMA (flat, dotted keys — the OTel GenAI convention's own spelling,
kept literal rather than nested so a consumer reads exactly the name this
card and ADR-0037 write in prose):

    gen_ai.operation.name          "invoke_agent" (constant)
    gen_ai.request.model           tool_response.resolvedModel
    gen_ai.usage.input_tokens      tool_response.usage.input_tokens
    gen_ai.usage.output_tokens     tool_response.usage.output_tokens
    forge.envelope                 tool_response.content[0].text, VERBATIM —
                                    never re-parsed, never truncated, never a
                                    stand-in for the raw request/response body
                                    it is not (EC-032/INV-037, AC-076).
    forge.station                  best-effort, see EXTRACTION below
    forge.card                     best-effort, see EXTRACTION below
    forge.wave                     best-effort, see EXTRACTION below
    forge.effort                   best-effort, see EXTRACTION below
    forge.model                    the spawn's own `forge.model=` tag
                                    (CARD-157), see EXTRACTION below — the
                                    model the SPAWN SITE asked for, beside
                                    gen_ai.request.model, the one the host
                                    resolved; both are kept as read, never
                                    reconciled (FR-031 edge_cases)
    forge.transcript_ref           best-effort pointer to the host JSONL
                                    transcript; EXPLICIT null when
                                    unresolvable, never omitted (AC-077).
    forge.parent_invocation_id     UNCONDITIONAL field; null for a root spawn.
    run_id                         the live run's id (`run_id:` of
                                    meta/kanban/dispatcher.state), explicit
                                    null when no run is in flight
                                    (CARD-159, DEC-132 (15)).
    at                             UTC timestamp, ISO-8601, second precision.

EXTRACTION (station/card/wave/effort) — A DOCUMENTED DESIGN DECISION, NOT
PART OF THE SPIKE. EXP-0001 only measured the four gen_ai/envelope fields and
the parent-linkage question; it says nothing about where station/card/wave/
effort come from, and no spawn site is in this card's Touches or may be
edited under it (G-3: skills/kanban/** is owned by CARD-074/075/076). So
these four were read, best-effort, out of ``tool_input``'s own text (the
``prompt`` and ``description`` fields — since CARD-157 ONLY the prompt's tag
line, see THE TAG LINE IS THE ONLY TEXT READ below) via the
literal ``forge.<field>=<value>`` tag convention this schema itself uses —
e.g. a spawn prompt containing ``forge.station=review forge.card=CARD-050
forge.effort=high`` populates all three; a spawn prompt with no such tags
populates none of them (each stays an explicit null, same discipline as
``forge.transcript_ref``).

CARD-157 (R6c, FR-031 amended 2026-09-26) is that follow-up: every pipeline
spawn site now OPENS its prompt with ONE tag line, its FIRST line,
``forge.station=<s> forge.card=<CARD-NNN|none> forge.wave=<N|none>
forge.model=<model id>`` (``skills/kanban/references/agent-briefs.md`` —
PROVENANCE TAG LINE; the architect phase template; ``critic-lenses.yml``
``spawn_provenance``), and ``forge.model`` joins the tag set read here.

THE TAG LINE IS THE ONLY TEXT READ (CARD-157 review cycle 1, F-001). The tags
are read from :func:`tag_line` — the first non-blank line of
``tool_input.prompt`` — and from nothing else: not the rest of the prompt, not
``description``. Per tag, the FIRST mention of ``forge.<tag>=`` on that line
decides; a value outside the grammar there is null and the search STOPS — it
never falls through to a later mention. Searching the whole prompt used to let
a bad or missing tag on the tag line be replaced by a tag the brief merely
QUOTES further down (AC-488's ``forge.wave=12``, a ``/commit`` argument
carrying another spawn's tags), journalling a fabricated value — and an
untagged prompt quoting a full tag line got all four fields fabricated with
nothing flagged. A prompt whose first line is not the tag line now journals
nulls, which ``forge:status`` flags (EC-136). The same definition is
duplicated, not imported, in ``skills/commit/scripts/provenance_trailer.py``
(this file is a stdlib-only single file byte-copied into
``.claude/hooks/forge/``, and skills/ may not import hooks/);
tests/test_provenance.py pins the two readers together. Two additions on top:

* ``none`` is a VALUE for ``forge.card``/``forge.wave`` (a non-card spawn says
  so literally), read as populated, never as an absent tag. The placeholder
  ``—`` is not a value: it matches nothing and the field stays null.
* A spawn whose station is one of :data:`NON_CARD_STATIONS` (an architect
  phase, the architect critic or its skeptic) has no card/wave context BY
  DEFINITION, so an absent card/wave tag there is filled with the literal
  ``none`` (AC-508). For every other station an absent tag stays null — a
  genuinely missing tag (EC-136), which ``forge:status`` reports as the typed
  ``provenance: missing`` finding (``status.py::agent_provenance``) instead of
  this writer guessing. An explicit tag always wins over the fill.

This writer still writes only what it reads: it never refuses an entry for a
missing tag (the spawn already happened, NFR-006) — flagging is the reader's
job, and the reader never writes here (G-4, ADR-0037/R1).

CWD RULE. The payload's ``cwd`` IS the project root in the ordinary case;
inside a worktree the project root is the MAIN repo, resolved the same way
for both cases by one formula: ``dirname(git rev-parse --git-common-dir)``
run FROM the payload cwd (an ordinary checkout's common-dir is
``<cwd>/.git``, whose dirname is ``<cwd>`` itself — the same formula, no
branching). Falls back to the payload cwd verbatim when git cannot answer
(no ``.git`` at all — a fixture project in a test, say).

NEVER COMMITTED (G-1, NFR-009): the journal is a channel a raw dialogue
could leak secrets through, so it is `.gitignore`'d and this writer never
reads or replays request/response bodies — only the compact final envelope
text the spawning agent itself chose to report (EC-032/INV-037).

APPEND-ONLY WITH A CAP (spirit of NFR-012). Every entry is written as ONE
LINE — a YAML flow mapping that is, not coincidentally, also valid JSON — so
this file can be produced and consumed with the Python standard library
alone (ADR-0011/R1, CON-003): no YAML library is required for either side,
though any YAML reader parses it as an ordinary block sequence too, since a
flow-mapping sequence item is valid YAML. The concurrency and cap-eviction
mechanics below are the same shape as ``hooks/_forge_common.py``'s
``record_gate_event`` / ``_trim_gate_events`` (O_EXCL header race-guard,
atomic tempfile + os.replace rewrite) — reused as a PATTERN, not imported,
because this card's declared Touches is this one file.

THE CAP COUNTS ENTRIES, NOT BYTES — a DECLARED design decision, not an
oversight (F-007, review cycle 1). NFR-012's own "в духе" (spirit-of) framing
is stated in bytes (~2KB/record, <200KB/directory), and ``forge.envelope`` is
stored VERBATIM with no length limit — so ``DEFAULT_MAX_ENTRIES`` entries of
an arbitrarily large final message is not bounded in bytes at all. Matches
the reused pattern exactly: ``hooks/_forge_common.py``'s
``GATE_EVENTS_MAX_ENTRIES`` is the same entry-count cap over a ``detail``
field with no length limit of its own. Accepted for this card for the same
reason: a byte-level cap would have to choose between truncating
``forge.envelope`` (which EC-032/AC-076 forbid — it must be stored verbatim,
never a partial stand-in) or evicting entries by a size heuristic instead of
the simple, auditable "oldest N" rule this shares with its precedent. A
follow-up wanting the byte guarantee literally would add a per-record size
cap with an explicit truncation marker (never a silent truncation) — deferred
here as out of this card's scope, not silently dropped.

INSTALLED-COPY PARITY (EC-044, AC-122). A SEPARATE CLI mode of this same
file, not run automatically by the hook path (which stays fail-open like
every other forge hook — NFR-006): ``agent-invocation.py --parity-check
<source> <installed_copy>`` sha256-compares the two and exits loud (code 2,
stderr banner) on ANY divergence or a missing file, exit 0 on a match. This
is the mechanism EXP-0001's ``probe/parity_check.py`` (tag exp/0001-archive)
demonstrated works; wiring it into a periodic gate (``ci/selftest.py``) is
future follow-up outside this card's guardrails (G-4).

EVICTION TOMBSTONE (CARD-159, DEC-132 (15), INV-037 amended). ``_trim`` never
drops an entry silently: every trim APPENDS exactly one tombstone record as the
last line -- ``forge.record: tombstone``, the trimming spawn's ``run_id``,
``at``, ``forge.evicted_count`` (this trim's own evictions) and
``forge.evicted``, groups keyed run_id / card / model with their count and
input / output tokens. The previous tombstone is evicted by the same rewrite
and its groups CARRIED into the new one, so at most one tombstone is live and
no record is rewritten in place; groups of the ``TOMBSTONE_CARRY_RUNS`` most
recently evicted run ids are kept, older ones fold into ``forge.expired``. The
cap counts spawn entries only. The run breaker (``run_spend.py``) reads a run's
spend as its entries plus the live tombstone's groups for it.

SPEND RECORD (CARD-159, ADR-0125/R1). When ``dispatcher.state`` records
``mode: autonomy``, the hook -- after its journal write, and also when that
append raised once the entry was built (``record_spawn`` then counts the entry
the journal lacks) -- records the spawn's
tokens into ``dispatcher.state.spend`` through ``run_spend.record_spawn``: the
SAME single-writer transaction every breaker transition uses (one hold of the
state lock, bounded at ``run_spend.HOOK_LOCK_WAIT``). It resolves the plugin
through ``_forge_common.find_plugin_root`` (installed beside this file) and
imports the module; any failure is swallowed (NFR-006) and the next breaker
transaction recounts the entry from the journal. Outside autonomy nothing is
imported and the state is never touched. This hook still writes only the
journal itself (ADR-0037/R1): the state write is ``run_spend``'s, made through
``dispatcher_state`` as INV-062 requires.

LATENCY (EC-045). Measured separately, same-window, by
``tests/test_agent_journal.py::BenchAgentJournalWriter_SameWindowOverheadP95``
— this module carries no timing code of its own; the bench times the whole
process the way Claude Code actually runs it (subprocess, JSON on stdin).

Standard library only.
"""
from __future__ import annotations

import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

#: Tool names that mean "a real sub-agent spawn" — the current name (`Agent`)
#: and the matcher name the card fixes (`Task`, which EXP-0001 found aliases
#: it 10/10 on CLI 2.1.251).
SPAWN_TOOL_NAMES = frozenset({"Task", "Agent"})

#: Where the journal lives, relative to the resolved project root.
JOURNAL_REL = Path("meta") / "agent-journal.yml"

#: Cap on entry count — spirit of NFR-012, oldest evicted on overflow.
#: Overridable for tests (FORGE_AGENT_JOURNAL_MAX_ENTRIES) so a cap-eviction
#: test does not need to seed 500 real entries to exercise the boundary.
DEFAULT_MAX_ENTRIES = 500

#: Run ids whose evicted groups a tombstone keeps (DEC-132 (15)); older runs
#: fold into ``forge.expired``.
TOMBSTONE_CARRY_RUNS = 4

#: Where the dispatcher's state lives, relative to the project root.
STATE_REL = Path("meta") / "kanban" / "dispatcher.state"
_STATE_RUN_ID_RE = re.compile(r'^run_id:[ \t]*"?([^"\n]+?)"?[ \t]*$', re.MULTILINE)
_STATE_MODE_RE = re.compile(r"^mode:[ \t]*(\S+)[ \t]*$", re.MULTILINE)


def max_entries() -> int:
    """The active entry cap — env override for tests, else the default."""
    raw = os.environ.get("FORGE_AGENT_JOURNAL_MAX_ENTRIES", "")
    if raw.strip():
        try:
            value = int(raw)
            if value > 0:
                return value
        except ValueError:
            pass
    return DEFAULT_MAX_ENTRIES


_HEADER = (
    "# forge agent-invocation journal — GENERATED, do not edit and do not\n"
    "# commit (NFR-009 — secret-leak channel; .gitignore'd). Single writer\n"
    "# (ADR-0037/R1): hooks/agent-invocation.py, a PostToolUse hook on\n"
    "# the sub-agent spawn tool. One entry per real spawn (operation\n"
    "# invoke_agent, OTel GenAI semantic conventions aligned) — never a\n"
    "# chat/execute_tool span. Append-only, oldest entries evicted past the\n"
    "# cap (spirit of NFR-012). Each entry below is one YAML flow mapping per\n"
    "# line — and, not coincidentally, also valid JSON: read with\n"
    "# json.loads() per line (no YAML dependency needed), or with any YAML\n"
    "# library as an ordinary block sequence.\n"
    "entries:\n"
)

#: Split point for the trim rewrite: the start of each data line.
_ENTRY_SPLIT_RE = re.compile(r"(?=^  - \{)", re.MULTILINE)

#: F-012 (review cycle 2): the separator is ``[:=]`` immediately after the
#: tag name and ``[ \t]*`` (space/tab only, NEVER ``\s*``) before the value —
#: the tolerant ``\s*`` on both sides used to let the value cross a NEWLINE
#: (``"forge.station:\nreview the diff"`` captured "review", fabricating a
#: value where the tag was merely mentioned, not set) and let a stray
#: same-line ``=`` bind to unrelated prose. Restricting the separator makes
#: the match require the tag literally adjacent to its value on one line —
#: still best-effort prose matching, not a parser, but no longer able to
#: reach across a line break for its capture.
#:
#: CARD-157: `none` is a card/wave VALUE (a non-card spawn), and `\b` closes
#: both so `forge.wave=12abc` or `forge.card=nonesuch` is no value at all
#: rather than a truncated one. A model id is not one word (`opus[1m]`,
#: `claude-opus-4-1`, `us.anthropic.claude-sonnet-4-5-20250929-v1:0`), so its
#: class takes the punctuation real ids carry — still never whitespace, so the
#: value cannot run into the next tag. Station and model also END at
#: whitespace, a quote or the end of the text (`(?=[\s"]|\Z)`): without it an
#: EMPTY tag followed by the next one (`forge.station= forge.model=sonnet`)
#: captured the word `forge` as the station — a fabricated value where none
#: was set, F-012's class again. The price, declared: a tag glued to sentence
#: punctuation (`forge.station=review.`) is no value — the spawn sites write
#: the tag line on its own, never inside prose.
#: A wave is a POSITIVE number, the same grammar ``wave_branch.py``'s
#: ``_WAVE_VALUE_RE`` and the trailer renderer use (F-006): `forge.wave=0` or
#: `forge.wave=012` is no value.
_CARD_TAG_RE = re.compile(r"forge\.card[:=][ \t]*\"?(CARD-\d+|none)\b\"?", re.IGNORECASE)
_WAVE_TAG_RE = re.compile(r"forge\.wave[:=][ \t]*\"?([1-9]\d*|none)\b\"?", re.IGNORECASE)
_STATION_TAG_RE = re.compile(r"forge\.station[:=][ \t]*\"?([\w-]+)\"?(?=[\s\"]|\Z)", re.IGNORECASE)
_EFFORT_TAG_RE = re.compile(r"forge\.effort[:=][ \t]*\"?([\w-]+)\"?", re.IGNORECASE)
_MODEL_TAG_RE = re.compile(r"forge\.model[:=][ \t]*\"?([\w.:/\[\]-]+)\"?(?=[\s\"]|\Z)", re.IGNORECASE)

#: Stations that carry no card/wave context by definition (AC-508): the
#: architect phase spawns (COMP-001) and the architect critic / its skeptic
#: (COMP-075). An absent card/wave tag on one of these is the literal `none`;
#: on any other station it stays null and is flagged (EC-136). A kanban
#: skeptic is NOT here — it verifies a card's findings and tags that card.
NON_CARD_STATIONS = frozenset({"architect", "critic", "critic-skeptic"})

#: The literal a non-card spawn uses for card/wave — never `—`, never null.
NONE_VALUE = "none"

#: Every pattern :func:`_tag_value` is ever called with — validated below,
#: AT MODULE LOAD, not merely at each call site (F-011, review cycle 2). A
#: runtime assert inside the reader (then `_first_group`) looked like it "failed loud" for
#: F-001, but it does not: it fires from WITHIN `_run_hook_mode`'s
#: try/except (fail-open, NFR-006 — see below), which swallows an
#: `AssertionError` exactly like it swallowed F-001's `IndexError`, so a
#: future group-less pattern would still lose the whole record silently.
#: Checking the fixed set of patterns ONCE, here, at import time, runs
#: BEFORE `_run_hook_mode`'s try/except exists to catch anything — a bad
#: pattern crashes the whole process with a traceback on stderr the moment
#: the hook is invoked, which is what "fail loud" actually requires.
_TAG_PATTERNS = {"card": _CARD_TAG_RE, "wave": _WAVE_TAG_RE,
                 "station": _STATION_TAG_RE, "effort": _EFFORT_TAG_RE,
                 "model": _MODEL_TAG_RE}

#: Where a tag is MENTIONED on the tag line at all — the one position its
#: value is read from (:func:`_tag_value`).
_TAG_MENTION_RE = {name: re.compile(rf"forge\.{name}[:=]", re.IGNORECASE)
                   for name in _TAG_PATTERNS}

for _pattern in _TAG_PATTERNS.values():
    assert _pattern.groups == 1, (
        f"module-level invariant violated: {_pattern.pattern!r} must have "
        f"exactly one capturing group for _tag_value, got {_pattern.groups}")
del _pattern

#: Candidate payload fields naming the PARENT of this spawn, tried in order.
#: No shape is confirmed by EXP-0001 (question (г) was non-gating and only
#: confirmed that WITHIN one spawn's own events agent_id ties them together —
#: not that a payload names a cross-spawn parent). Best-effort, documented.
_PARENT_ID_KEYS = ("parent_tool_use_id", "parent_agent_id")

#: Ceiling on the ONE blocking call on the hook path (`git rev-parse
#: --git-common-dir` in :func:`project_root`). Set FROM the EC-045 budget
#: (500ms p95 added latency per spawn), not an unrelated round constant —
#: F-008, review cycle 1: the previous value (10s) sat 20x over that budget,
#: so a slow/hung git (network filesystem, fsmonitor stall, lock contention)
#: could blow the certified p95 with no bench catching it, since the bench's
#: fixture project is not a git repository at all and exits `git` immediately
#: on every run. On expiry, :func:`project_root` already degrades correctly
#: (falls back to the payload cwd verbatim) — this only tightens how long
#: that fallback may take to trigger.
GIT_TIMEOUT = 0.5


def project_root(payload_cwd: str) -> Path:
    """Resolve the journal's project root per the card's cwd rule.

    ``dirname(git rev-parse --git-common-dir)``, asked FROM the payload's own
    cwd. One formula covers both cases the card names: in an ordinary
    checkout the common git dir is ``<cwd>/.git``, whose dirname is ``<cwd>``
    itself; inside a linked worktree (every kanban card is developed in one)
    it is the MAIN repository's ``.git``, whose dirname is the main repo
    root. Falls back to the payload cwd verbatim when git cannot answer at
    all (no ``.git`` reachable — a plain fixture directory in a test).
    """
    cwd = Path(payload_cwd) if payload_cwd else Path.cwd()
    try:
        done = subprocess.run(
            ["git", "-C", str(cwd), "rev-parse", "--git-common-dir"],
            capture_output=True, text=True, timeout=GIT_TIMEOUT,
        )
    except (OSError, subprocess.SubprocessError):
        return cwd
    if done.returncode != 0:
        return cwd
    answer = done.stdout.strip()
    if not answer:
        return cwd
    common = Path(answer)
    if not common.is_absolute():
        common = cwd / common
    try:
        return common.resolve().parent
    except OSError:
        return common.parent


def tag_line(text: str) -> str:
    """THE tag line of a spawn prompt: its first non-blank line, or ``""``.

    The PROVENANCE TAG LINE is declared as the FIRST line of the spawn prompt
    (agent-briefs.md); leading blank lines are tolerated, nothing else is. A
    prompt whose first line is prose has no tag line — its tags are absent,
    never looked for further down (F-001, CARD-157 review cycle 1). Twin of
    ``provenance_trailer.py::tag_line`` (pinned by tests/test_provenance.py).
    """
    for line in str(text).splitlines():
        if line.strip():
            return line
    return ""


def _spawn_tag_line(payload: dict) -> str:
    """The tag line of this spawn: :func:`tag_line` of ``tool_input.prompt``.

    Only the prompt's first line is read — ``description`` and the rest of
    the prompt are never searched (F-001): a later quote of the convention
    cannot stand in for a bad or missing tag on the spawn's own line. Nothing
    here widens what EC-032 allows into the record.
    """
    tool_input = payload.get("tool_input") or {}
    if not isinstance(tool_input, dict):
        return ""
    return tag_line(tool_input.get("prompt") or "")


def _tag_value(name: str, line: str) -> str | None:
    """The value of tag ``name`` on the tag line ``line``, or None.

    The FIRST mention of ``forge.<name>[:=]`` on the line decides: its value
    must match the tag's grammar AT that position, else the result is None
    and the search STOPS — a second mention is never consulted, so a bad
    value is null, never a guess (failure matrix rows 1-2). The pattern's
    single capturing group is checked at module load (F-011); this assert is
    the narrower second defense, not the primary guarantee.
    """
    pattern = _TAG_PATTERNS[name]
    assert pattern.groups == 1, (
        f"_tag_value requires exactly one capturing group, got "
        f"{pattern.groups} in {pattern.pattern!r}")
    mention = _TAG_MENTION_RE[name].search(line)
    if mention is None:
        return None
    match = pattern.match(line, mention.start())
    return match.group(1) if match else None


def _none_normalized(value: str | None) -> str | None:
    """``none`` in any case is the one literal ``none`` (the tag regexes are
    case-insensitive; the journal carries one spelling)."""
    if value is not None and value.lower() == NONE_VALUE:
        return NONE_VALUE
    return value


def _resolve_transcript_ref(payload: dict) -> str | None:
    """Best-effort pointer to the host JSONL transcript; explicit null else.

    ``agent_transcript_path`` (SubagentStop's own field, EXP-0001) is tried
    first, then the general ``transcript_path`` every hook payload carries.
    An empty or absent value is NOT silently dropped from the record — the
    caller always writes the key, with this function's None (AC-077).
    """
    for key in ("agent_transcript_path", "transcript_path"):
        value = payload.get(key)
        if value:
            return str(value)
    return None


def _resolve_parent_invocation_id(payload: dict) -> str | None:
    """Best-effort parent linkage; None (root spawn) when nothing names one."""
    for key in _PARENT_ID_KEYS:
        value = payload.get(key)
        if value:
            return str(value)
    return None


def read_run(root: Path) -> tuple[str | None, str | None]:
    """``(run_id, mode)`` of the live run, read by regex from the state file
    (one small read, no YAML library -- EC-045); ``(None, None)`` when absent
    or unreadable."""
    try:
        text = (root / STATE_REL).read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return None, None
    run = _STATE_RUN_ID_RE.search(text)
    mode = _STATE_MODE_RE.search(text)
    return (run.group(1) if run else None), (mode.group(1) if mode else None)


def build_record(payload: dict, *, now: datetime | None = None,
                 run_id: str | None = None) -> dict[str, Any]:
    """Build one journal entry from a PostToolUse (or SubagentStop-fallback)
    payload. Pure function — no I/O — so it is unit-testable on its own.

    Args:
        payload: The hook's stdin payload, already JSON-decoded.
        now: Timestamp to stamp the record with; defaults to current UTC.

    Returns:
        The flat, dotted-key record dict (see module docstring for schema).
    """
    moment = (now or datetime.now(timezone.utc)).astimezone(timezone.utc)
    stamp = moment.replace(microsecond=0).strftime("%Y-%m-%dT%H:%M:%SZ")

    tool_response = payload.get("tool_response")
    if not isinstance(tool_response, dict):
        tool_response = {}
    usage = tool_response.get("usage")
    if not isinstance(usage, dict):
        usage = {}
    content = tool_response.get("content")
    envelope: str | None = None
    if isinstance(content, list) and content:
        first = content[0]
        if isinstance(first, dict) and isinstance(first.get("text"), str):
            envelope = first["text"]
    if envelope is None:
        # SubagentStop fallback (reserve path, EXP-0001): the PostToolUse
        # path above already found it 10/10 on the main chokepoint, so this
        # only fires when reconstruction is genuinely needed.
        last_msg = payload.get("last_assistant_message")
        if isinstance(last_msg, str):
            envelope = last_msg

    line = _spawn_tag_line(payload)
    station = _tag_value("station", line)
    station = station.lower() if station is not None else None   # F-006: one spelling
    card = _none_normalized(_tag_value("card", line))
    wave = _none_normalized(_tag_value("wave", line))
    if station is not None and station in NON_CARD_STATIONS:
        card = NONE_VALUE if card is None else card
        wave = NONE_VALUE if wave is None else wave

    record: dict[str, Any] = {
        "gen_ai.operation.name": "invoke_agent",
        "gen_ai.request.model": tool_response.get("resolvedModel"),
        "gen_ai.usage.input_tokens": usage.get("input_tokens"),
        "gen_ai.usage.output_tokens": usage.get("output_tokens"),
        "forge.envelope": envelope,
        "forge.station": station,
        "forge.card": card,
        "forge.wave": wave,
        "forge.effort": _tag_value("effort", line),
        "forge.model": _tag_value("model", line),
        "forge.transcript_ref": _resolve_transcript_ref(payload),
        "forge.parent_invocation_id": _resolve_parent_invocation_id(payload),
        "run_id": run_id,
        "at": stamp,
    }
    return record


def _is_spawn_event(payload: dict) -> bool:
    """True when this payload is a real sub-agent spawn's PostToolUse/
    SubagentStop event — the only thing this writer ever journals (NG-2)."""
    hook_event = payload.get("hook_event_name")
    if hook_event == "PostToolUse":
        return payload.get("tool_name") in SPAWN_TOOL_NAMES
    if hook_event == "SubagentStop":
        return True
    # hook_event_name is undocumented for PostToolUse["Task"] per EXP-0001 —
    # fall back to shape-sniffing when it is absent so this writer degrades
    # gracefully rather than going silent on an unlabelled payload.
    if "tool_response" in payload:
        return payload.get("tool_name") in SPAWN_TOOL_NAMES
    return "agent_transcript_path" in payload


def append_entry(journal_path: Path, record: dict[str, Any], cap: int,
                 run_id: str | None = None) -> None:
    """Append one entry to the journal, creating it if absent, capped at
    ``cap`` entries with the oldest evicted first.

    Mirrors ``hooks/_forge_common.py``'s ``record_gate_event`` /
    ``_trim_gate_events`` pattern (O_EXCL header race-guard so two concurrent
    first-writers — kanban runs several worktrees against the same MAIN-repo
    journal — never both think they are first; atomic tempfile + os.replace
    rewrite so a trim is NEVER OBSERVED HALF-DONE by a concurrent reader or
    appender — no reader ever sees a torn file, and no two trims interleave
    into one). Reused as a pattern, not imported: this card's Touches is one
    file.

    What that guarantee does NOT cover (F-009, review cycle 1, same
    exposure the reused pattern already accepts): a read-modify-write race
    can still LOSE an append. Once the journal is at ``cap``, every
    subsequent append triggers :func:`_trim` (steady state, not a corner),
    and if a second writer appends between one trim's read and its
    ``os.replace``, that second entry is silently dropped by the rewrite —
    the file is never torn, but it can be short one entry. Accepted for the
    same reason the reused pattern accepts it: the journal is an
    observability signal, never a source of truth (its own header says so),
    and forge is local-first with no cross-process lock primitive in its
    core (ADR-0011/CON-003). A future card wanting the stronger guarantee
    would add an advisory lock (``fcntl.flock``) around append+trim.
    """
    journal_path.parent.mkdir(parents=True, exist_ok=True)
    try:
        fd = os.open(str(journal_path), os.O_CREAT | os.O_EXCL | os.O_WRONLY)
        try:
            os.write(fd, _HEADER.encode("utf-8"))
        finally:
            os.close(fd)
    except FileExistsError:
        pass

    line = "  - " + json.dumps(record, ensure_ascii=False) + "\n"
    with journal_path.open("a", encoding="utf-8") as fh:
        fh.write(line)

    _trim(journal_path, cap, run_id)


def _block_record(block: str) -> dict | None:
    """The JSON object of one entry block, or None when it does not decode."""
    try:
        value = json.loads(block.strip()[2:])
    except (json.JSONDecodeError, IndexError):
        return None
    return value if isinstance(value, dict) else None


def _tok(value: Any) -> int:
    return value if isinstance(value, int) and not isinstance(value, bool) and value > 0 else 0


def build_tombstone(previous: dict | None, evicted: list[dict | None],
                    run_id: str | None, stamp: str) -> dict[str, Any]:
    """One tombstone for one trim: ``previous``'s groups carried, the evicted
    spawn entries folded in by (run_id, card, model), the
    ``TOMBSTONE_CARRY_RUNS`` most recently evicted run ids kept and the rest
    folded into ``forge.expired``. Pure function."""
    groups: dict[tuple, dict[str, Any]] = {}
    for group in (previous or {}).get("forge.evicted") or []:
        if isinstance(group, dict):
            key = (group.get("run_id"), group.get("card"), group.get("model"))
            groups[key] = dict(group)
    expired = dict((previous or {}).get("forge.expired")
                   or {"count": 0, "input_tokens": 0, "output_tokens": 0})
    for entry in evicted:
        entry = entry or {}
        model = entry.get("gen_ai.request.model") or entry.get("forge.model")
        key = (entry.get("run_id"), entry.get("forge.card"), model)
        group = groups.pop(key, None) or {"run_id": key[0], "card": key[1], "model": key[2],
                                          "count": 0, "input_tokens": 0, "output_tokens": 0}
        group["count"] += 1
        group["input_tokens"] += _tok(entry.get("gen_ai.usage.input_tokens"))
        group["output_tokens"] += _tok(entry.get("gen_ai.usage.output_tokens"))
        groups[key] = group          # re-inserted: most recently evicted last
    order: list = []
    for key in groups:
        if key[0] in order:
            order.remove(key[0])
        order.append(key[0])
    keep = set(order[-TOMBSTONE_CARRY_RUNS:])
    kept = []
    for key, group in groups.items():
        if key[0] in keep:
            kept.append(group)
        else:
            for f in ("count", "input_tokens", "output_tokens"):
                expired[f] = expired.get(f, 0) + group[f]
    return {"forge.record": "tombstone", "run_id": run_id, "at": stamp,
            "forge.evicted_count": len(evicted), "forge.evicted": kept,
            "forge.expired": expired}


def _trim(journal_path: Path, cap: int, run_id: str | None = None) -> None:
    """Evict the oldest SPAWN entries past ``cap`` and append the one
    tombstone that accounts for them (EVICTION TOMBSTONE above), best-effort
    (see ``_forge_common.py::_trim_gate_events`` — the journal is append-only
    infrastructure, not the record that just succeeded; a trim failure must
    never surface as a failure to have recorded the entry)."""
    tmp: Path | None = None
    try:
        text = journal_path.read_text(encoding="utf-8")
        blocks = _ENTRY_SPLIT_RE.split(text)
        header, entries = blocks[0], blocks[1:]
        previous = None
        spawns: list[str] = []
        for block in entries:
            record = _block_record(block)
            if record is not None and record.get("forge.record") == "tombstone":
                previous = record
            else:
                spawns.append(block)
        if len(spawns) <= cap:
            return
        evicted = spawns[:len(spawns) - cap]
        stamp = datetime.now(timezone.utc).replace(microsecond=0).strftime("%Y-%m-%dT%H:%M:%SZ")
        stone = build_tombstone(previous, [_block_record(b) for b in evicted], run_id, stamp)
        new_header = header if header.strip() else _HEADER
        fd, tmp_name = tempfile.mkstemp(
            dir=str(journal_path.parent), prefix=f".{journal_path.name}.",
            suffix=".tmp")
        tmp = Path(tmp_name)
        body = "".join(b if b.endswith("\n") else b + "\n" for b in spawns[-cap:])
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            fh.write(new_header + body + "  - " + json.dumps(stone, ensure_ascii=False) + "\n")
        os.replace(tmp, journal_path)
        tmp = None
    except Exception:  # noqa: BLE001 — the journal is a signal, never a source of truth
        pass
    finally:
        if tmp is not None:
            try:
                tmp.unlink()
            except OSError:
                pass


def read_entries(journal_path: Path) -> list[dict[str, Any]]:
    """Read back every entry, oldest first — stdlib-only (no YAML library
    needed: each data line is valid JSON). Used by tests and by future
    consumers (forge:debug/redo, forge:retrospective, eval)."""
    if not journal_path.exists():
        return []
    text = journal_path.read_text(encoding="utf-8")
    out: list[dict[str, Any]] = []
    for line in text.splitlines():
        stripped = line.strip()
        if not stripped.startswith("- "):
            continue
        try:
            out.append(json.loads(stripped[2:]))
        except json.JSONDecodeError:
            continue
    return out


def _run_hook_mode() -> int:
    """The normal invocation: a PostToolUse/SubagentStop payload on stdin.

    Fail-open, like every other forge hook (NFR-006): any error here must
    never break the sub-agent spawn that already happened. Always exits 0.
    """
    try:
        raw = sys.stdin.read()
    except Exception:
        return 0
    try:
        payload = json.loads(raw)
    except Exception:
        return 0
    if not isinstance(payload, dict) or not _is_spawn_event(payload):
        return 0
    record: dict[str, Any] | None = None
    try:
        root = project_root(str(payload.get("cwd", "")))
        run_id, mode = read_run(root)
        record = build_record(payload, run_id=run_id)
        append_entry(root / JOURNAL_REL, record, max_entries(), run_id)
    except Exception:
        # The journal must never be the reason a sub-agent spawn appears to
        # fail — see the hook's own docstring / NFR-006. A failed APPEND
        # still records spend below (failure matrix row 3): `record_spawn`
        # counts an entry the journal lacks, so the exact counter keeps it.
        pass
    if record is not None and mode == "autonomy" and run_id:
        _record_spend(root, record)
    return 0


def _record_spend(root: Path, record: dict[str, Any]) -> None:
    """SPEND RECORD (module docstring): one ``run_spend`` transaction, fail-open."""
    try:
        here = str(Path(__file__).resolve().parent)
        if here not in sys.path:
            sys.path.insert(0, here)
        from _forge_common import find_plugin_root  # noqa: PLC0415 -- autonomy path only
        rel = "skills/kanban/scripts/run_spend.py"
        plugin, _source = find_plugin_root(require=rel)
        if plugin is None:
            return
        scripts = str(plugin / "skills" / "kanban" / "scripts")
        if scripts not in sys.path:
            sys.path.insert(0, scripts)
        import run_spend  # noqa: PLC0415 -- resolved at run time (ADR-0012)
        run_spend.record_spawn(run_spend.Paths.for_root(root), record)
    except Exception:  # noqa: BLE001 -- NFR-006: the spawn already happened
        pass


def _sha256(path: str) -> str:
    digest = hashlib.sha256()
    with open(path, "rb") as fh:
        for chunk in iter(lambda: fh.read(65536), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _parity_check_mode(argv: list[str]) -> int:
    """EC-044/AC-122: sha256-compare an installed copy against its source.

    Mirrors EXP-0001's ``probe/parity_check.py`` (tag exp/0001-archive)
    exactly: PASS (exit 0) on identical bytes, LOUD FAIL (exit 2, stderr
    banner naming both hashes) on any divergence or unreadable file — never a
    silent absence of records.
    """
    if len(argv) != 2:
        print("usage: agent-invocation.py --parity-check <source> <installed_copy>",
              file=sys.stderr)
        return 2
    source, installed = argv
    try:
        source_hash = _sha256(source)
        installed_hash = _sha256(installed)
    except OSError as exc:
        print("=" * 60, file=sys.stderr)
        print(f"PARITY FAIL: cannot read file: {exc}", file=sys.stderr)
        print("=" * 60, file=sys.stderr)
        return 2
    if source_hash != installed_hash:
        print("=" * 60, file=sys.stderr)
        print("PARITY FAIL: installed agent-journal writer diverges from source",
              file=sys.stderr)
        print(f"  source    {source}\n    sha256={source_hash}", file=sys.stderr)
        print(f"  installed {installed}\n    sha256={installed_hash}", file=sys.stderr)
        print("=" * 60, file=sys.stderr)
        return 2
    print(f"PARITY PASS sha256={source_hash}")
    return 0


def main() -> int:
    argv = sys.argv[1:]
    if argv and argv[0] == "--parity-check":
        return _parity_check_mode(argv[1:])
    return _run_hook_mode()


if __name__ == "__main__":
    sys.exit(main())
