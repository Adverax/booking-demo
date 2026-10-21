# forge
#!/usr/bin/env python3
"""
forge pre-commit hook  —  PreToolUse on Bash

Runs forge's deterministic CI gate before git commit (earlier catch than pre-push).
The forge plugin is located at RUN TIME (ADR-0012, `find_plugin_root`) — never
from a version pinned into config.json at install time, which stopped existing
on the next upgrade and silently downgraded this gate. Falls back to a
blocking-decisions check when no installation resolves — and SAYS SO (a
silently weaker gate reads as a passing full gate).

The gate FAILS OPEN on timeout or crash (NFR-006, unchanged: a broken gate must
not hold the partner's commit hostage). What it does not do is fail open
QUIETLY — every such outcome is shouted on the error stream and appended to
meta/gate-events.yml, where /forge:status picks it up (NFR-010, TM-014). That
stream is resolved through `_err_stream`, never `sys.stderr` directly: a
descriptor closed at process start makes `sys.stderr` None, and `print` then
sends human text to STDOUT, where it corrupts the verdict below.

Since CARD-005 the same gate also scans what the commit would ADD for
credentials (NFR-009, TM-010). It is the same check point, deliberately: NFR-009
says "новая точка проверки не вводится". The scan runs FIRST because it is
local and cheap; when it fails it fails open like everything else here, and —
this part matters — it fails open by RETURNING, so the CI gate below still runs.
A new check that can disarm the old one is not an addition.

"What the commit would ADD" is NOT PREDICTED. Three review cycles produced three
Criticals of one family, and the third established why the family is unwinnable
at this check point: **at PreToolUse time the content of the coming commit does
not exist yet**. `git add -A && git commit -m x` stages the tree with the SECOND
command on the line, so `git diff --cached` is empty when the hook runs — the
`git add` has not run either — and the same is true of every `-a`/`-i`/`-o`/
bare-pathspec form, which stage at commit time. A parser asked "what will this
line stage?" is guessing about a future it cannot observe, and each cycle's
enumeration was defeated by one more spelling of the same question.

So the question is not asked. Any `git commit` ingress selects the UNRESTRICTED
set — the index diff UNION the working tree (tracked modifications AND untracked,
non-ignored files) — regardless of what the command line says. There is nothing
left to predict, so there is no confidence question on the critical path and no
pathspec narrowing; the selection is the same for `git commit -m x`,
`git add -A && git commit`, `/usr/bin/git commit x` and
`sh -c 'git commit -m x'` alike. The measured cost of that choice is stated in
`_commit_contents`. `git <verb> --continue` is the one form whose content DOES
exist when the hook runs — it is the index, exactly — and it is selected from
the index alone, narrowed to what the resolution introduced (see
`_resolution_paths`).

The DECLARED COST, stated because ADR-0022/R1 requires the price of a choice to
be named: a credential sitting in the working tree or in an untracked file
blocks EVERY commit in that repository until it is removed or pragma'd, even a
commit that does not touch it. That is a false alarm the committer can see and
suppress; a missed credential is not.

A match an allow-pragma silences is COUNTED and reported — on stderr, in the
deny object's `suppressed` field, and in `meta/gate-events.yml`. The pragma
silences the block, not the gate's account of itself; a run that found two
credentials and said nothing renders exactly like a clean one otherwise.

EXIT CODES (the declared set — ADR-0007/R1):

* ``0`` — allow. Covers a clean gate AND every fail-open outcome.
* ``2`` — refuse. The only code the consumer (Claude Code's PreToolUse
  protocol) reads as "do not run this command", so both refusal REASONS carry
  it; they are told apart in the machine-readable output, not by the number.
  A secret-scan refusal additionally prints a PreToolUse deny object on
  STDOUT (``permissionDecision: deny`` plus a ``forge`` block naming gate,
  file, line and pattern — and both ways the verdict is NARROWER than the
  scan, ``skipped`` and ``suppressed``), so a consumer that reads the verdict
  rather than the terminal can tell WHICH gate refused and why. STDOUT carries
  that object and nothing else — including when the process was started with
  its error descriptor closed, the case that used to put every human line on
  the same channel and make the object unparseable. The two pre-existing
  refusals (CI gate failed, blocking decisions unresolved) keep their exit-2 +
  stderr shape unchanged — outside this card's scope, and named here so the
  asymmetry is declared rather than discovered.

Nothing here may raise. A traceback exits 1, which this consumer treats as a
NON-blocking error — i.e. a crash silently disarms the gate. That is why stdio
is pinned to a lossy encoder below, why the payload is validated field by field
rather than trusted, why the scan's own failures are caught, and why
`_residual_guard` turns anything left over into a declared, loud fail-open
instead of letting it escape as exit 1.

Install via: /forge:init hooks
"""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path
from typing import NoReturn

from _forge_common import (CI_GATE_TIMEOUT_ENV, SECRET_ALLOW_PRAGMA, _err_stream,
                           find_meta, find_plugin_root, forge_paths, load_yaml,
                           precommit_gate_budget, record_gate_event, record_gate_run,
                           scan_files_for_secrets, warn, yaml_available)

#: The CI gate, relative to the plugin root.
CI_SCRIPT_REL = "skills/architect-validate/scripts/ci.py"

#: Stable identifier of this gate in meta/gate-events.yml (read by forge:status).
GATE_ID = "pre-commit-ci"

#: NFR-006 as amended (ADR-0111/R1, ADR-0116/R1): the gate's budget is
#: MEASURED — min(max(60 s, 2 × p95 of the last N journaled runs), cap) — and
#: computed by `_forge_common.precommit_gate_budget`, which owns the formula,
#: the cap key and the run journal. `CI_GATE_TIMEOUT_ENV` (imported) stays the
#: explicit escape hatch, announced whenever it is used — a gate whose budget
#: was changed out of band must not look like a gate running its measured one.

#: Stable identifier of the secret scan in meta/gate-events.yml.
SECRET_GATE_ID = "pre-commit-secret-scan"

#: Budget for the WHOLE secret scan: the git plumbing that lists the files
#: (each `subprocess.run` gets it) and the reading of those files, which is
#: bounded by a monotonic deadline measured from the moment the scan starts
#: (`_within_budget`). Until this was added the second half ran unbounded and a
#: large untracked tree stalled every commit without ever tripping the
#: fail-open. Overridable by the same convention as the CI gate's — and used by
#: the suite to drive the timeout branch deterministically instead of
#: pretending a fast command is slow.
SECRET_SCAN_TIMEOUT = 15
SECRET_SCAN_TIMEOUT_ENV = "FORGE_SECRET_SCAN_TIMEOUT"


def _harden_stdio() -> None:
    """Make this hook's own output unable to kill it (CARD-004's F-013 trap).

    A UnicodeEncodeError while PRINTING a verdict is a traceback, and a
    traceback exits 1 — which this consumer reads as a non-blocking error, so
    the commit proceeds. A gate that disarms itself by describing a file whose
    name is not representable in the ambient encoding is not a gate. The
    scanner reports arbitrary repository paths, so the encoder is made lossy
    instead of fatal, on both streams.
    """
    for stream in (sys.stdout, sys.stderr):
        try:
            stream.reconfigure(errors="backslashreplace")
        except (AttributeError, ValueError, OSError):
            pass  # pre-3.7 wrapper or a stream that is not one; nothing to fix


def main() -> None:
    _harden_stdio()
    try:
        data = json.load(sys.stdin)
    except Exception:
        sys.exit(0)

    # The payload is another process's JSON, so every level of it is checked
    # rather than assumed: `{"command": null}` and `{"tool_input": []}` each
    # raised here, and a raise is exit 1 — the one code this file's contract
    # says it may not speak, because the consumer reads it as "non-blocking
    # error" and runs the command anyway.
    tool_input = data.get("tool_input") if isinstance(data, dict) else None
    raw_command = tool_input.get("command") if isinstance(tool_input, dict) else None
    command: str = raw_command if isinstance(raw_command, str) else ""
    ingress = _commit_ingress(command)
    if ingress is None:
        sys.exit(0)

    meta = find_meta()
    if meta is None or not forge_paths(meta)["architecture"].is_dir():
        sys.exit(0)

    _run_secret_scan(meta, ingress)

    if ingress != "commit":
        # The CI gate's own trigger is deliberately UNCHANGED (`git commit`
        # only): widening it would put a 60s gate on every `git rebase
        # --continue` step, which is a different card's decision — and one that
        # now exists as a tracked backlog entry rather than only as this
        # comment (`meta/kanban/backlog.md`, "Решить, должен ли CI-гейт …
        # `--continue`"). The secret scan above is the same gate at the same
        # check point, so covering the `--continue` family costs it nothing
        # (NFR-009: no new check point).
        return

    ci_script, source = _find_ci_script()
    if ci_script:
        # A wrong-but-valid resolution used to be as silent as the stale path
        # this card removed (F-002): name which of the several sources this
        # run came from every time, not only when nothing resolves at all.
        warn(f"CI gate resolved plugin root via {source}: {ci_script.parent}")
        _run_ci_gate(ci_script, meta)
    else:
        warn("full CI gate unavailable (no forge plugin installation resolved) "
             "— checking blocking decisions only. Re-run /forge:init hooks to restore.")
        _check_blocking_only(meta)


def _find_ci_script() -> tuple[Path | None, str | None]:
    """Locate ci.py inside the plugin installation resolved for THIS run."""
    root, source = find_plugin_root(require=CI_SCRIPT_REL)
    return (root / CI_SCRIPT_REL if root else None), source


def _gate_timeout(meta: Path) -> float:
    """Seconds the CI gate may take before the commit proceeds without it.

    Always announced with its basis (floor / measured / capped / overridden):
    the number that decides whether this commit gets a verdict must never be
    invisible, and a gate that times out at a budget nobody saw is the quiet
    fail-open NFR-010 exists to remove.
    """
    seconds, basis = precommit_gate_budget(meta)
    warn(f"CI gate budget {seconds:g}s — {basis}.")
    return seconds


def _announce_fail_open(meta: Path, gate: str, banner: str, outcome: str,
                        detail: str, elapsed: float | None = None) -> None:
    """Shout a fail-open outcome and journal it — WITHOUT ending the process.

    Extracted from :func:`_fail_open` unchanged so a second gate in this hook
    can fail open without taking the first one down with it: the CI gate's
    fail-open is terminal (nothing runs after it), the secret scan's is not.
    """
    warn(f"!! {banner} - COMMIT ALLOWED WITHOUT THE GATE (fail-open, NFR-006).")
    warn(f"!! {detail}")
    try:
        journal = record_gate_event(meta, gate, outcome, detail, elapsed=elapsed)
        warn(f"!! recorded in {journal} - /forge:status will flag it. "
             "Run /forge:ci manually to verify this commit.")
    except Exception as e:  # noqa: BLE001 — a journal failure must not block
        warn(f"!! could NOT record the event ({e!r}) - it is visible ONLY here. "
             "Run /forge:ci manually to verify this commit.")


def _fail_open(meta: Path, outcome: str, detail: str,
               elapsed: float | None = None) -> NoReturn:
    """Let the commit through, LOUDLY (NFR-006 policy + NFR-010 visibility).

    The commit is allowed either way — that policy is not this function's to
    change. Its job is that the outcome cannot pass unnoticed: a banner on
    stderr now, and a durable entry that /forge:status flags later.
    """
    banner = ("CI GATE TIMED OUT" if outcome == "timeout"
              else "CI GATE FAILED TO RUN")
    _announce_fail_open(meta, GATE_ID, banner, outcome, detail, elapsed)
    sys.exit(0)


# ── secret scan (CARD-005, NFR-009, TM-010) ──────────────────────────────
#
# WHERE THE GUARD IS, AND WHERE IT IS NOT (ADR-0022/R2 — a guard on one path is
# declared on all its twins). The guarded operation is "content this repository
# is about to accept". Enumerated by CALL SITE, not by file, because the first
# version of this block said "1 of 3" while `git commit <pathspec>` and four
# other commit-creating verbs walked past it. All of them, with the real status:
#
#   1. `git commit` run through the Bash tool — COVERED in every STAGING FORM,
#      because the form is no longer consulted: the selection is the index diff
#      UNION the working tree (tracked modifications and untracked, non-ignored
#      files). `-a`, `-i`, `-o`, a bare pathspec, `git add -A &&` earlier on
#      the line, `/usr/bin/git`, `git -C sub`, `cd sub &&` and `sh -c '…'` all
#      reach the same set — 32 rows (16 shapes x tracked/untracked) verified
#      end to end, each allowed row then actually run and checked with
#      `git show HEAD:<file>`. The forms that used to be holes were holes
#      because they stage AFTER this PreToolUse hook runs; nothing that stages
#      later can escape a selection that does not depend on staging at all.
#      NFR-009 names this as the check point and forbids introducing a second
#      one.
#
#      NOT in every SPELLING, and the difference is a claim this comment made
#      for two cycles that its own tests could not hold up. The selection is
#      unconditional; whether `_commit_ingress` RECOGNISES the line as a commit
#      at all is still a text question, and an OPEN CLASS of spellings walks
#      past it — not a list of three, which is what this line said for one
#      cycle and what cycle 5 refuted by producing four more in an afternoon.
#      The class, and why it is a class rather than an enumeration, is item (e)
#      below. The distinction is exactly: what the command STAGES no longer
#      matters, what the command IS still does.
#   2. `git merge|cherry-pick|revert|rebase|am --continue` run through the Bash
#      tool — COVERED since cycle 2. The previous version of this block
#      declared them uncovered because "when they CONFLICT the resolution is
#      concluded by a `git commit`, which is site 1". Both halves of that
#      sentence were refuted by execution: during a conflict the resolution IS
#      in the index (`UU <file>`), and `--continue` completes the commit
#      WITHOUT any `git commit` ever running — reproduced end to end, hook exit
#      0, credential in `git show HEAD:<file>`. Covering it costs no new check
#      point (the NFR-009 condition): the index is already populated when the
#      hook runs, so it is the same `--cached` selection site 1 reads. This is
#      the ONE site the widen above deliberately does not reach: the verb
#      stages nothing further, so the working tree cannot enter this commit and
#      scanning it would be a false alarm with no recall to buy. It is narrowed
#      FURTHER — to what the resolution introduced — for the reason
#      `_resolution_paths` states. What is NOT extended to them is the CI gate
#      — see `main`.
#   3. `git merge`, `git cherry-pick`, `git revert` WITHOUT `--continue` — NOT
#      covered, and not coverable AT THIS POINT IN TIME. This is a PreToolUse
#      hook: it runs BEFORE the command, and the content these verbs will
#      commit is not in the index or the working tree yet — it lives in another
#      commit object. Scanning here would scan the pre-merge state, which is
#      what the previous commit already admitted. Their content is also, by
#      construction, content this repository already holds.
#   4. `git am` (and `git apply` + commit) run through the Bash tool — the
#      non-`--continue` invocation is NOT covered, and is the ONE residual
#      ingress worth naming as such: unlike merge/cherry-pick/revert, `am`
#      applies a patch that originates OUTSIDE the repository, so it can
#      introduce a credential this repository has never seen. The same
#      PreToolUse timing argument applies — at hook time the patch is an unread
#      file and the index is untouched — so covering it would mean parsing
#      mailbox files here, i.e. a second check point (the thing NFR-009
#      forbids) with a parser of its own. Declared, not silently absent.
#      Mitigation in practice: a CONFLICTING `am` is concluded by `git am
#      --continue`, which site 2 now covers; a clean one is scanned by site 1
#      the next time anything in those files is committed, and by /forge:ci.
#   5. `hooks/pre-push.py` — NOT covered, deliberately. Push carries only what
#      commit already admitted, so the scan there would be a second check point
#      (NFR-009 condition) guarding nothing new; CARD-005's G-3 also puts the
#      file out of scope.
#   6. A `git commit` typed in a terminal rather than run through the Bash tool
#      — NOT covered, and NOT coverable from here: both forge hooks are
#      PreToolUse hooks on the Bash tool, not git hooks. This is a pre-existing
#      property of the whole deterministic tier, already recorded in ADR-0007's
#      Consequences; the secret scan inherits it and does not widen it.
#
# So: 2 of 6, with 3–6 declared, measured (each verified to exit 0 with a
# staged credential) and reasoned rather than left implicit.
#
# ONE FRAME FURTHER OUT — what still decides a commit's content that the list
# above does not see. Named because every previous review cycle found the
# defect exactly one frame beyond where the fix looked, and because two of the
# reasons previously given here turned out to be refutable:
#
#   a. `sh -c 'git commit x'` / `bash -c …` — CLOSED here (`_shell_c_payloads`),
#      one level of unwrapping, in this hook only. The previous reason for
#      leaving it open ("it lives in `command_matches`, which pre-push also
#      calls, and G-3 freezes that hook") did not hold: G-3 forbids EDITING
#      `hooks/pre-push.py`, and an INGRESS check local to this file leaves that
#      file byte-identical. Measured before the fix: `git commit -m wip` ->
#      exit 2, `sh -c 'git commit -m wip'` -> exit 0 with nothing scanned at
#      all — the gap disarmed the whole gate, not just the selection. Residual:
#      nesting deeper than `_SHELL_C_MAX_DEPTH`, and a payload built at run
#      time (`sh -c "$CMD"`), which no static reader can see.
#   b. A user-defined git alias (`git ci` = `commit`) — NOT closed: the verb
#      this hook keys on is not the verb the user typed, and the alias table
#      lives in the user's config rather than on the command line. Unlike (a)
#      this cannot be decided from the text at all.
#   c. `git -c core.worktree=… commit <path>` / `git -C sub commit` /
#      `GIT_DIR=… git commit` — no longer relevant to the SELECTION, which is
#      the whole point of the ruling: a relocation changes which files a
#      PATHSPEC names, and no pathspec is consulted. What a relocation can
#      still do is point the hook at a different repository than the commit
#      will use; that is site 6's property (a command this hook cannot follow),
#      not a selection defect.
#   d. `git commit --amend` — the amended commit's content is index vs HEAD~1,
#      while this scan reads index vs HEAD (plus the working tree), so content
#      carried over from the commit being replaced is not re-read. It is
#      content the repository already admitted (the same argument site 3
#      makes), which is why it is declared here rather than fixed.
#   e. THE INGRESS RESIDUE — an open CLASS, deliberately not an enumeration.
#      Stated as a class because the enumeration shape has now failed three
#      times: cycle 4 named zero of it, cycle 5's fix named three families,
#      cycle 6 found five more in one sitting without trying hard, and cycle
#      6's verification gate found a ninth whose CAUSE the class statement did
#      not contain. A list that grows every time somebody looks is not a list,
#      it is a class with a bad name.
#
#      THE CLASS — TWO CAUSES, not one. The residue is every command line
#      whose literal `git <verb>` token pair the MATCHER does not see. It fails
#      to see it for two INDEPENDENT reasons, and for three cycles this block
#      named only the first:
#
#        (i)  the SHELL produces the pair and the text never carried it —
#             parameter expansion, command substitution, `eval`, an alias, a
#             function, a payload piped into another shell's stdin, a sourced
#             file. `_commit_ingress` decides on TEXT (the literal token `git`,
#             whitespace, the literal verb token, in the string the PreToolUse
#             payload carries, plus one to `_SHELL_C_MAX_DEPTH` levels of
#             `sh -c '…'` unwrapping), so the pre-images of "this line ends up
#             running git commit" are not a finite set any matcher short of a
#             shell can enumerate;
#        (ii) the pair IS in the text and `_runs_git_verb` DELETES it before
#             matching. That function strips quoted spans first
#             (`re.sub(r"'[^']*'|\"[^\"]*\"", "", command)`), so quoting the
#             VERB removes exactly the token the pattern needs: `git 'commit'`,
#             `git "commit"`, `git com"mit"`. The shell produces the pair
#             perfectly — the matcher destroys it. This cause lives in THIS
#             file, not in the shell, which is why the earlier all-shell-side
#             account sent readers to look in the wrong place; the same strip
#             is what buys `sh -c '…'` unwrapping its payload, so it is a cost
#             of this design and not an oversight in it.
#
#      Closing "all of cause (i)" is not a bounded task; each member is
#      individually closable, which is a different statement. Cause (ii) IS
#      bounded — it is one regex in this file — and is left open only by the
#      same scoping ruling as the rest (see WHY IT IS STILL OPEN).
#
#      MEASURED LOWER BOUND, not a total. Every row below was reproduced end to
#      end (a copy of hooks/ installed into a real repository, PreToolUse JSON
#      on stdin) at exit 0 with the credential then recovered from
#      `git show HEAD:<file>`. The first three plus the alias were measured in
#      cycle 5; the rest of cause (i) in cycle 6; cause (ii) by the
#      verification gate that closed cycle 6 and re-measured here — all
#      in tracked AND untracked variants:
#
#        cause (i) — the shell supplies what the text does not carry
#          the BINARY is not a literal `git` token
#              git add -A && $(which git) commit -m wip
#              git add -A && `which git` commit -m wip
#              GIT=git;      git add -A && $GIT commit -m wip
#              GIT=git;      git add -A && ${GIT} commit -m wip
#              GIT=git; r=GIT; git add -A && ${!r} commit -m wip   (BASH only:
#              indirect expansion is a bashism and is inert under zsh, so
#              re-test this row under bash or it will look closed)
#              g(){ git "$@"; }; git add -A && g commit -m wip
#              a user-defined git alias (`git ci`) — see (b): not decidable
#              from the text at all, the alias table is in the user's config
#          the VERB is not a literal `commit` token
#              V=commit;       git add -A && git $V -m wip
#              A=com; B=mit;   git add -A && git ${A}${B} -m wip
#          the LINE ITSELF is data that some other shell will read
#              git add -A && eval "git commit -m wip"      (and single-quoted)
#              git add -A && echo 'git commit -m wip' | sh
#              git add -A && printf 'git commit -m wip' | bash
#              git add -A && echo <base64 of the same> | base64 -d | sh
#              git add -A && printf '…' > .r.sh && . ./.r.sh
#        cause (ii) — the text carries it and the strip-quoting deletes it
#          the VERB is QUOTED, which is inert to the shell and fatal to us
#              git add -A; git 'commit' -m x
#              git add -A; git "commit" -m x
#              git add -A; git com"mit" -m x
#
#      Note (1): `eval "git commit …"` and a payload on a shell's STDIN are
#      invisible for the SAME two reasons stated above and in that order —
#      `_runs_git_verb` strips the quoted payload (cause (ii)'s mechanism
#      acting on a line whose pair is quoted DATA, not a quoted verb), and
#      `_SHELL_C_RE` unwraps only sh|bash|zsh|dash|ksh|ash with `-c`, so
#      nothing re-reads what the strip removed. Note (2): `sh <<'EOS' … EOS` is
#      BLOCKED, and that is an accident worth naming rather than coverage worth
#      claiming — the here-doc body is part of the command string, so the
#      literal token pair is present and the matcher fires on TEXT it does not
#      understand. Note (3): `echo commit | xargs git` also exits 0; it is left
#      out of the count because the credential did not reach history (the
#      resulting `git commit` has no `-m` and dies for want of an editor),
#      which is a functional non-event, not a guard. Note (4), an UNPROVEN
#      VARIANT recorded so the follow-up card checks it instead of
#      rediscovering it: two apostrophes STRADDLING the verb make the strip
#      regex span across it — `echo don't && git commit -m "won't fix"` yields
#      ingress `None` — but that line is not valid shell (the first apostrophe
#      opens a quote that never closes), and no valid-shell instance of the
#      shape has been constructed. It is the same root cause as cause (ii),
#      claimed as neither a family nor a demonstrated bypass.
#
#      Controls on the same repositories, all exit 2: `git commit`,
#      `/usr/bin/git commit`, `command git commit`, `sh -c 'git commit'`,
#      `env GIT_AUTHOR_NAME=x git commit`.
#
#      THE COUNT, re-derived from the table rather than asserted — the two
#      sentences that used to stand here said "three plus four" and then
#      "eight", which do not add up, and both predate cause (ii). NINE
#      families, grouped by mechanism exactly as `INGRESS_TABLE` groups them:
#      cause (i) has eight — (1) command substitution (`$(…)`, backticks),
#      (2) the binary in a parameter (`$GIT`, `${GIT}`, bash's `${!ref}`),
#      (3) a wrapper function, (4) a user-defined alias (see (b): declared, no
#      spelling to pin), (5) the verb in a parameter (`git $V`,
#      `git ${A}${B}`), (6) `eval`, (7) a payload on another shell's stdin
#      (`echo … | sh`, `printf … | bash`, `… | base64 -d | sh`), (8) a sourced
#      file; cause (ii) has one — (9) the quoted verb. By cycle: 3 (cycle 5 —
#      families 1, 2, 4) + 5 (cycle 6 — families 3, 5, 6, 7, 8) + 1 (cycle 6's
#      verification gate — family 9) = 9. Seventeen spellings are pinned line
#      by line in `INGRESS_TABLE` (fourteen of cause (i), three of cause (ii));
#      family 4 is the one with nothing to pin.
#
#      WHY IT IS STILL OPEN. Most members ARE decidable from the text (unlike
#      the alias in (b)) — substituting `$(…)`/backticks with their inner text
#      flips two of the three cycle-5 rows, which was measured, and cause (ii)
#      is one regex away from closed. It is not closed here on a deliberate
#      scoping ruling: the family this card was escalated for is staging
#      prediction, which IS closed. Nor is any of these spellings one a human
#      or an agent emits in practice — with cause (ii) the closest to one,
#      since quotes around a verb cost nothing to type and change nothing for
#      the shell. What is claimed here is only the lower bound and the class —
#      a count of SPELLINGS would be the third version of the same mistake.
#      Tracked for a follow-up card in `meta/kanban/backlog.md`; the rows above
#      are pinned as declared residue in `DeclaredScopeTest.INGRESS_TABLE`, so
#      closing any one of them flips a test rather than passing in silence.

#: Verbs whose `--continue` CONCLUDES a commit with no `git commit` anywhere on
#: the line. Reproduced: hook exit 0, credential in `git show HEAD:<file>`.
#: The content is already in the index at that point, so this reads the same
#: `--cached` source — narrowed by :func:`_resolution_paths` — and introduces no
#: second check point (NFR-009).
CONTINUE_COMMIT_VERBS = ("merge", "cherry-pick", "revert", "rebase", "am")

_CONTINUE_RE = re.compile(r"(?:^|\s)--continue\b")

#: `<shell> -c '<command line>'` — the ingress this hook could not see through.
#: `command_matches` strips quoted segments before matching, so the payload was
#: invisible and `sh -c 'git commit -m wip'` exited 0 without scanning
#: anything. Matched here rather than in `command_matches` on purpose: that
#: function is shared with `hooks/pre-push.py`, which G-3 freezes, and this
#: check is local to the commit gate.
_SHELL_C_RE = re.compile(
    r"\b(?:sh|bash|zsh|dash|ksh|ash)\s+-[A-Za-z]*c\s+"
    r"(?:'(?P<sq>[^']*)'|\"(?P<dq>[^\"]*)\"|(?P<bare>\S+))")

#: How deep the unwrapping goes. Bounded because the payload is another
#: process's string: `sh -c "sh -c '…'"` is real, a thousand levels of it is an
#: attack on this loop rather than a commit.
_SHELL_C_MAX_DEPTH = 3

#: Past this length the payload is not unwrapped — the outer checks have
#: already run, and a megabyte-long command is not a shell one-liner anybody
#: typed. O(1) refusal, the same fail-fast choice the rest of this file makes.
COMMAND_UNWRAP_MAX_CHARS = 65536


def _shell_c_payloads(command: str) -> list:
    """The command lines carried inside `sh -c …` / `bash -c …` on this line."""
    return [(m.group("sq") if m.group("sq") is not None
             else m.group("dq") if m.group("dq") is not None
             else m.group("bare") or "")
            for m in _SHELL_C_RE.finditer(command)]


def _runs_git_verb(command: str, verb: str) -> bool:
    """`command_matches(command, "git", verb)` with the verb a WHOLE token.

    The shared matcher ends its verb at ``\\b``, and a word boundary sits
    between `commit` and the `-` of `commit-graph`: `git commit-graph write`
    and `git commit-tree …` therefore engaged the whole commit gate — a full
    scan, and a refusal, over two plumbing commands that create no commit from
    the working tree at all. Measured before this fix: both exit 2 with a
    credential anywhere in the tree, tracked or untracked.

    Written HERE rather than in `command_matches` on purpose: that function is
    shared with `hooks/pre-push.py`, which G-3 freezes, and the narrowing is a
    property of the commit gate's ingress. The declared cost of that choice is
    the duplicated pattern below — it mirrors `command_matches` and is pinned
    in both directions by `DeclaredScopeTest.INGRESS_TABLE`, so a divergence
    fails a test instead of ageing quietly.

    THE STRIP IS ITSELF AN INGRESS HOLE, and it is the one the residual list's
    item (e) calls cause (ii). Removing quoted spans is what keeps a quoted
    MENTION (`echo "git commit -m wip"`) from engaging the gate, and it is what
    lets `_SHELL_C_RE` decide `sh -c '…'` on the payload instead of the
    wrapper. It also deletes a quoted VERB: `git 'commit'`, `git "commit"` and
    `git com"mit"` all reduce to a line with no `commit` token in it, so this
    function answers False while the shell runs an ordinary commit. Measured
    end to end, tracked and untracked: exit 0, credential recovered from
    `git show HEAD:<file>`. Pinned as declared residue in `INGRESS_TABLE`
    ("DECLARED RESIDUE: the verb in quotes …"); closing it means matching on
    something other than the naked stripped text, not deleting the strip.
    """
    stripped = re.sub(r"'[^']*'|\"[^\"]*\"", "", command)
    pattern = (r"\bgit\b(?:\s+-[A-Za-z]\s+\S+|\s+--?[\w-]+(?:=\S+)?)*\s+"
               + re.escape(verb) + r"(?![\w.-])")
    return re.search(pattern, stripped) is not None


def _commit_ingress(command: str, depth: int = 0) -> str | None:
    """Which commit-creating form this command is — or None for anything else.

    This is the ONE question the command line still decides: everything
    downstream of it is unconditional, so a bypass now lives here and nowhere
    else. What it recognises is a `git` invocation whose verb is a WHOLE token
    (`git commit`, not `git commit-graph`), in any staging form, named by path,
    with global options in front, and through one to
    :data:`_SHELL_C_MAX_DEPTH` levels of `sh -c '…'`.

    What it does NOT recognise is stated as a CLASS, because it is one and
    enumerating it has now been wrong three times. The class is "every line
    whose literal `git <verb>` token pair the MATCHER does not see", and there
    are TWO reasons it does not see one — stating only the first is how the
    ninth family stayed hidden behind a causal account that sent readers
    shell-side:

    * the SHELL produces the pair and the text never carried it — parameter
      expansion, command substitution, `eval`, an alias, a function, a payload
      piped into another shell, a sourced file. Those pre-images are not a
      finite set a matcher short of a shell can enumerate;
    * the pair IS in the text and `_runs_git_verb` deletes it, because that
      function strips quoted spans before matching: `git 'commit'`,
      `git "commit"`, `git com"mit"` — the shell runs the commit, the matcher
      loses the token. That cause is in this file, not in the shell.

    The measured LOWER BOUND (NINE families — eight shell-side, one
    matcher-side — every row reproduced end to end with the credential
    recovered from `git show HEAD:<file>`) is item (e) of the residual list
    above, where the count is derived family by family, and the rows are
    executed by `DeclaredScopeTest.INGRESS_TABLE`. Neither the class nor its
    bound is claimed here as "every spelling", and the bound is not claimed as
    a total.

    Returns:
        ``"commit"`` for `git commit …`, ``"continue"`` for
        `git <verb> --continue` (the index is already the commit's content), or
        ``None`` when this command creates no commit here.
    """
    if _runs_git_verb(command, "commit"):
        return "commit"
    stripped = re.sub(r"'[^']*'|\"[^\"]*\"", "", command)
    if _CONTINUE_RE.search(stripped):
        for verb in CONTINUE_COMMIT_VERBS:
            if _runs_git_verb(command, verb):
                return "continue"
    if depth >= _SHELL_C_MAX_DEPTH or len(command) > COMMAND_UNWRAP_MAX_CHARS:
        return None
    for payload in _shell_c_payloads(command):
        inner = _commit_ingress(payload, depth + 1)
        if inner is not None:
            return inner
    return None


def _secret_scan_timeout() -> float:
    """Seconds the whole scan — plumbing and files — may take before it fails
    open."""
    raw = os.environ.get(SECRET_SCAN_TIMEOUT_ENV, "").strip()
    if not raw:
        return float(SECRET_SCAN_TIMEOUT)
    try:
        seconds = float(raw)
    except ValueError:
        seconds = 0.0
    if seconds <= 0:
        warn(f"ignoring {SECRET_SCAN_TIMEOUT_ENV}={raw!r} (not a positive number "
             f"of seconds) — using the default {SECRET_SCAN_TIMEOUT}s.")
        return float(SECRET_SCAN_TIMEOUT)
    warn(f"secret scan budget overridden to {seconds}s via "
         f"{SECRET_SCAN_TIMEOUT_ENV} (default {SECRET_SCAN_TIMEOUT}s).")
    return seconds



def _git_lines(args: list, cwd: Path, timeout: float) -> list:
    """Run a read-only git command and split its NUL-separated output."""
    result = subprocess.run(
        ["git", *args], capture_output=True, timeout=timeout, cwd=str(cwd),
        # Bytes, not text: a path is bytes to git, and a filename that is not
        # valid UTF-8 must not raise inside the decoder before it can be
        # scanned or reported as skipped.
        env={**os.environ, "PYTHONIOENCODING": "utf-8"},
    )
    if result.returncode != 0:
        raise RuntimeError(
            f"git {' '.join(args)} exited {result.returncode}: "
            f"{result.stderr.decode('utf-8', 'replace').strip()[:200]}")
    return [p.decode("utf-8", "surrogateescape")
            for p in result.stdout.split(b"\x00") if p]


#: The other side of a conflicted replay, in the order the verbs are checked.
#: `git am --continue` has NO such ref — its patch is a file, not a commit
#: object — so an `am --continue` keeps the whole index, which is the right
#: answer for the one `--continue` whose content can come from outside this
#: repository. `REVERT_HEAD` is deliberately absent: a revert stages the
#: INVERSE of that commit, so every path differs from it and the narrowing
#: below would be a no-op wearing the look of a decision.
_CONFLICT_SIDE_REFS = ("REBASE_HEAD", "MERGE_HEAD", "CHERRY_PICK_HEAD")


def _conflict_side(root: Path, timeout: float) -> str | None:
    """The ref naming the commit being replayed, when one is being replayed."""
    for ref in _CONFLICT_SIDE_REFS:
        try:
            if _git_lines(["rev-parse", "--verify", "--quiet", ref], root, timeout):
                return ref
        except Exception:  # noqa: BLE001 — a missing ref exits non-zero; try the next
            continue
    return None


def _resolution_paths(root: Path, index_paths: list, timeout: float) -> list:
    """What a `--continue` RESOLUTION introduced, not the whole replayed commit.

    Index-only was still too wide, and measurably so: during a rebase the index
    carries the entire commit being replayed, so a branch holding a file with a
    pre-existing credential — committed before this gate existed, or from a
    terminal — could not be rebased AT ALL. Exit 2 on every step of an
    interactive rebase, over content the repository already holds, with no
    escape: the committer can neither unstage the file (it is part of the
    commit being replayed) nor pragma it without altering that commit.

    That contradicted the enumeration's own principle for site 3 — content
    already in this repository is not content it is about to accept — so the
    selection is intersected with "differs from the OTHER side too". A path
    staged byte-identical to the replayed commit is that commit's content; a
    path the resolution actually wrote differs from both sides and stays in.

    DECLARED COST: a resolution that takes the incoming side VERBATIM is not
    re-scanned, so a credential introduced by the replayed commit is not caught
    here. It is caught where it entered — that commit's own gate — and it is,
    by construction, content this repository already holds. `git am` is the one
    verb where that argument fails (the patch comes from outside), and it is
    also the one verb with no such ref, so it keeps the whole index.

    Returns:
        The subset of ``index_paths`` the resolution introduced — or
        ``index_paths`` unchanged when there is no other side to compare with,
        or when git could not be asked. Failing WIDE is the fail-closed
        direction here.
    """
    other = _conflict_side(root, timeout)
    if other is None:
        return index_paths
    try:
        changed = set(_git_lines(
            ["diff", "--name-only", "--diff-filter=ACM", "-z", "--cached", other],
            root, timeout))
    except Exception:  # noqa: BLE001 — cannot tell => keep the wider selection
        return index_paths
    return [p for p in index_paths if p in changed]


class _ScanBudgetExceeded(Exception):
    """The file loop ran past the scan's deadline; the caller fails open loudly.

    Its own class rather than `TimeoutError`, so it cannot be confused with an
    OS-level timeout raised by something the scanner called: this exception is
    only ever raised by :func:`_within_budget`, and the handler that catches it
    announces a TIMED OUT fail-open on that basis.
    """


def _within_budget(pairs, deadline: float, budget: float):
    """Yield scan candidates while the budget lasts, then raise.

    The deadline is monotonic and checked BETWEEN files — the one place a scan
    can be cut short without reporting bytes it never read as clean. A file
    already started is always finished (`SECRET_SCAN_MAX_BYTES` bounds it), so
    the overshoot is one file, not one tree.

    Raises:
        _ScanBudgetExceeded: when the deadline passed before the next file.
    """
    for index, pair in enumerate(pairs):
        if time.monotonic() >= deadline:
            raise _ScanBudgetExceeded(
                f"scanning exceeded its {budget:g}s budget after {index} file(s)")
        yield pair


def _commit_contents(timeout: float, ingress: str = "commit") -> tuple:
    """The files this commit could add, as ``(repo_root, [relative paths])``.

    NOT the files it WILL add — that question is unanswerable here and asking
    it produced three Criticals of one family. At PreToolUse time the commit's
    content does not exist yet: `git add -A && git commit` stages with the
    second command on the line, `-a`/`-i`/`-o`/a bare pathspec stage at commit
    time, and `git diff --cached` is empty in all of them. So the selection is
    unconditional and covers every source the commit can draw from:

    * ``git diff --cached`` — what is already staged;
    * ``git diff`` — tracked files modified in the working tree;
    * ``git ls-files --others --exclude-standard`` — untracked, non-ignored
      files. This is the source no previous version read at all, and the one
      TM-010 names verbatim: a freshly generated artifact that inherited a
      credential, committed with `git add -A && git commit`.

    ``command`` is not a parameter: the selection stopped consulting it when
    the ruling made it unconditional. It was kept for two cycles on the
    justification that "the caller's journal and messages name it" — they do
    not, and did not; a parameter no line of this function reads is a claim
    that this function depends on it.

    MEASURED COST — stated as a RATE first, because that is the part that
    survives being read on another machine. Three cycles have now written a
    different absolute figure here (0.124 / 0.121 / this one) and none of them
    reproduced the previous one; an absolute wall-clock number in a docstring
    is a property of a host and a tree state, not of this code.

    * the SELECTION is unrestricted but it is still a DIFF — only files that
      actually differ can enter a commit, so an ordinary commit costs the size
      of the working-tree delta, not the size of the repository;
    * the SCAN is linear in the BYTES it reads. Measured throughput ~5.5 MiB/s;
      per tracked file of this repository (mean 12 KiB) that is ~2.1 ms;
    * so the CEILING — every tracked file dirty at once — is
      `tracked_files x 2.1 ms` here, and it is ABOVE A SECOND, which is the
      fact the number exists to carry. It is unreachable by any ordinary state
      of a working tree, and it is what `_within_budget` exists to bound.

    THE INSTANCE these rates came from, with its conditions, so a reader can
    tell drift from noise: this repository, 901 tracked files / 10.4 MiB;
    macOS 26.5.2 arm64, CPython 3.9.6; warm cache; median of 7 end-to-end hook
    runs after a discarded warm-up; measured 2026-08-21. Ordinary commit, 17
    candidate files (0.48 MiB): 0.176-0.189 s total, of which 0.039 s is the
    hook process itself, so ~0.14 s of scan. All-dirty ceiling, 902 candidate
    files: 1.91-1.94 s total, so ~1.90 s of scan. The 1.95 s this docstring
    used to state, and the 1.82 s a review cycle re-measured, both sit inside
    the host-to-host spread of that same quantity — which is the reason the
    rate is what is claimed here and the absolutes are dated.

    WHAT BOUNDS IT, precisely (the previous sentence here said the ceiling
    "is bounded by ``SECRET_SCAN_TIMEOUT`` into a loud fail-open", which was
    false for the part that costs the time). ``timeout`` is passed to the git
    plumbing BELOW — each `subprocess.run` gets it. Reading the selected files
    is not this function's work at all; it happens in
    :func:`_run_secret_scan`, and until this fix it ran with no deadline, no
    file cap and no byte cap. Measured: 8 000 untracked, non-ignored files
    (17.6 MB, i.e. a `node_modules` before anyone writes a `.gitignore`) with
    ``FORGE_SECRET_SCAN_TIMEOUT=1`` cost 3.44 s of wall clock, exit 0, no
    banner and no journal entry — the declared budget did not bind at all, and
    at 5 MB/s a 200 MB tree would cost ~35 s on EVERY commit, silently. The
    same budget now bounds the WHOLE scan (:func:`_within_budget`), measured
    from the moment :func:`_run_secret_scan` starts, so an over-budget scan is
    a loud, journalled fail-open instead of a stall.

    ``ingress`` of ``"continue"`` (`git merge|cherry-pick|revert|rebase|am
    --continue`) is the one form whose content DOES exist when the hook runs —
    it is the index, exactly, and the verb stages nothing further. It is read
    from the index alone and narrowed further by :func:`_resolution_paths`.

    Returns:
        ``(root, paths)``, or ``(None, [])`` when this is not a git repository
        — there is no commit content to inspect, which is not a failure.

    Raises:
        RuntimeError, OSError, subprocess.TimeoutExpired: git could not be run
            or refused; the caller turns these into a loud fail-open.
    """
    try:
        root_out = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            capture_output=True, text=True, timeout=timeout,
            encoding="utf-8", errors="replace")
    except FileNotFoundError:
        raise RuntimeError("git executable not found on PATH")
    if root_out.returncode != 0:
        return None, []  # not a repository: nothing is being committed here
    root = Path(root_out.stdout.strip())

    base = ["diff", "--name-only", "--diff-filter=ACM", "-z"]
    #: `--cached` is index vs HEAD (what is staged); no selector at all is
    #: working tree vs index (what any staging form would add on top of it).
    paths: list = _git_lines(base + ["--cached"], root, timeout)

    if ingress == "continue":
        paths = _resolution_paths(root, paths, timeout)
    else:
        paths.extend(_git_lines(base, root, timeout))
        paths.extend(_git_lines(
            ["ls-files", "--others", "--exclude-standard", "-z"], root, timeout))
    ordered, seen = [], set()
    for p in paths:
        if p not in seen:
            seen.add(p)
            ordered.append(p)
    return root, ordered


#: What was READ, per ingress — named in the refusal because the selection no
#: longer follows the command line. A committer who typed `git commit -m wip`
#: and is refused over an untracked file needs to be told, in the message
#: itself, that the gate reads everything the commit could draw from; a
#: refusal whose scope is a surprise is the kind people route around.
SELECTION_LABELS = {
    "commit": "the index, tracked working-tree edits and untracked files "
              "(everything this commit could add — the form of the command is "
              "deliberately not consulted)",
    "continue": "the index, narrowed to what the conflict resolution "
                "introduced",
}


def _deny_secret(report, root: Path, ingress: str = "commit") -> NoReturn:
    """Refuse the commit, legibly to a human AND to the consumer (ADR-0007/R1).

    stderr carries the human message; stdout carries the PreToolUse deny
    object, which is what a consumer reads. Both name the file, the line, the
    rule and WHAT WAS SCANNED — and neither carries the credential: this
    message reaches a terminal, a transcript and the model's context, so it
    must not become the leak it exists to prevent.
    """
    selection = SELECTION_LABELS.get(ingress, SELECTION_LABELS["commit"])
    lines = [f.describe() for f in report.findings]
    for line in lines:
        print(f"forge(secret): {line}", file=_err_stream())
    if report.total > len(report.findings):
        warn(f"secret scan: {report.total - len(report.findings)} further "
             "finding(s) not listed.")
    reason = (f"forge secret scan blocked this commit — {report.total} "
              f"credential-shaped match(es) in the files it would add: "
              + "; ".join(lines[:5]))
    print(json.dumps({
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": reason,
        },
        "forge": {
            "gate": SECRET_GATE_ID,
            "verdict": "blocked",
            "selection": selection,
            "total": report.total,
            "findings": [
                {"path": f.path, "line": f.line, "pattern": f.pattern,
                 "redacted": f.redacted} for f in report.findings],
            "skipped": report.skipped,
            # Both of the ways this verdict is narrower than the scan: files it
            # could not read, and matches a pragma silenced.
            "suppressed": report.suppressed,
        },
    }, ensure_ascii=True))
    print(f"\nforge: commit blocked — a secret pattern was found in the files "
          f"this commit could add (NFR-009). Remove the credential and rotate "
          f"it; if the file is a LOCAL one this commit was never meant to "
          f"carry (an untracked `.env`, a scratch config), add it to "
          f"`.gitignore` — ignored files are not scanned and not committed; "
          f"if the value is genuinely not a credential, put "
          f"`{SECRET_ALLOW_PRAGMA}` on the line above it; and if the file is "
          f"TRACKED and MACHINE-GENERATED (a review report, a generated doc), "
          f"neither of those two reaches it — `.gitignore` does not apply to a "
          f"tracked path, and a pragma does not survive the next regeneration "
          f"— so the fix belongs in the GENERATOR, which must elide the "
          f"credential-shaped literal it quotes (write the shape, not the "
          f"string).\n"
          f"Scanned: {selection}.\nRepository: {root}",
          file=_err_stream())
    sys.exit(2)


def _run_secret_scan(meta: Path, ingress: str = "commit") -> None:
    """Scan what this commit COULD add; block on a finding, else return.

    Failure of the SCANNER is not a finding (G-2): it is announced and
    journalled exactly like the CI gate's own fail-open, and then this function
    RETURNS so the CI gate still runs.

    The budget is taken ONCE, here, and it bounds the whole scan: the git
    plumbing keeps it per command, and the file loop inherits the same deadline
    measured from this line. Over budget is a loud fail-open, never a stall —
    the exposure the unconditional selection created is a slow scan, not a
    wrong one, and a slow one is invisible precisely because nothing times out.
    """
    timeout = _secret_scan_timeout()
    deadline = time.monotonic() + timeout
    try:
        root, paths = _commit_contents(timeout, ingress)
    except subprocess.TimeoutExpired:
        _announce_fail_open(
            meta, SECRET_GATE_ID, "SECRET SCAN TIMED OUT", "timeout",
            f"listing the commit's files exceeded its {timeout:g}s budget.")
        return
    except Exception as e:  # noqa: BLE001 — any failure to RUN it fails open
        _announce_fail_open(
            meta, SECRET_GATE_ID, "SECRET SCAN FAILED TO RUN", "failure",
            f"could not list the commit's files: {e!r}")
        return
    if root is None or not paths:
        return

    try:
        report = scan_files_for_secrets(
            _within_budget(((root / p, p) for p in paths), deadline, timeout),
            root=root)
    except _ScanBudgetExceeded as e:
        # The scan RAN and could not finish in its budget. Same outcome as the
        # plumbing timing out — the gate did not report on this commit — so it
        # takes the same loud, journalled fail-open rather than the silent
        # stall this branch replaces.
        _announce_fail_open(
            meta, SECRET_GATE_ID, "SECRET SCAN TIMED OUT", "timeout",
            f"{e} of {len(paths)} candidate file(s); raise "
            f"{SECRET_SCAN_TIMEOUT_ENV} or narrow the tree (a large untracked "
            "directory that belongs in .gitignore is the usual cause).")
        return
    except Exception as e:  # noqa: BLE001 — the scanner reads arbitrary bytes
        _announce_fail_open(
            meta, SECRET_GATE_ID, "SECRET SCAN FAILED TO RUN", "failure",
            f"scanning {len(paths)} candidate file(s) raised: {e!r}")
        return

    if report.skipped:
        # A skipped file is a hole in the scan, not a clean file — say so.
        shown = list(report.skipped.items())[:5]
        warn("secret scan did NOT read " + ", ".join(f"{p} ({why})" for p, why in shown)
             + (f" and {len(report.skipped) - len(shown)} more"
                if len(report.skipped) > len(shown) else ""))
    _announce_suppressions(meta, report, len(paths))
    if report.clean:
        return
    _deny_secret(report, root, ingress)


def _announce_suppressions(meta: Path, report, scanned: int) -> None:
    """Say that matches were silenced — the skip announcement's twin.

    Same argument, one function over: this module already refuses to report a
    SKIPPED file as "clean", because a skip is a place where the scan read
    nothing and said nothing. A pragma-suppressed match is a place where the
    scan FOUND something and said nothing, which is the stronger case. The
    pragma is durable, greppable and diff-visible — the gate's own RUNTIME
    output was not, so a suppressed run rendered exactly like a clean one:
    exit 0, empty stdout, no stderr line, no journal entry.

    One aggregate line, never a line per match, and never the matched content:
    the repository carries ~42 pragma lines, and a per-match log is how a
    warning becomes something people filter out.
    """
    if not report.suppressed:
        return
    warn(f"secret scan SUPPRESSED {report.suppressed} match(es) in "
         f"{scanned} file(s) via `{SECRET_ALLOW_PRAGMA}` — they were found and "
         "deliberately not blocked. Re-check them if you did not put the "
         "pragma there.")
    try:
        record_gate_event(
            meta, SECRET_GATE_ID, "suppressed",
            f"{report.suppressed} match(es) silenced by "
            f"{SECRET_ALLOW_PRAGMA} across {scanned} scanned file(s).")
    except Exception as e:  # noqa: BLE001 — visibility must never block a commit
        warn(f"secret scan could not record the suppression ({e!r}) — it is "
             "visible ONLY here.")


#: Tags that mark a line in the gate's own output as worth relaying to the
#: user — and, together, as evidence the gate reported SOMETHING rather than
#: merely dying (F-004): a non-zero exit with none of these anywhere in its
#: output crashed before it could produce a verdict (a traceback, not a
#: finding), and must fail OPEN like a timeout, not block like a violation.
_GATE_VERDICT_TAGS = ("WARN", "ERROR", "BLOCKED", "✗")


def _journal_run(meta: Path, outcome: str, started: float) -> float:
    """Journal one CI gate run and its elapsed seconds (ADR-0111/R2).

    Called on EVERY exit of :func:`_run_ci_gate` — pass, block, timeout,
    failure — before that exit is taken, so the verdict never depends on it:
    a journal that cannot be written is said out loud and the run proceeds to
    the same verdict it had (the record is a signal, never a gate).

    Returns:
        The elapsed seconds, for the caller's own fail-open event.
    """
    elapsed = time.monotonic() - started
    # ADR-0111/R2 "every pre-commit gate run" is read as the CI gate (NFR-006)
    # only: secret-scan runs are not journaled (CARD-152 F-005).
    try:
        record_gate_run(meta, GATE_ID, outcome, elapsed)
    except Exception as e:  # noqa: BLE001 — a timing record never decides a commit
        warn(f"could NOT journal this CI gate run's {elapsed:.1f}s ({e!r}) — the "
             "measured budget will not see it.")
    return elapsed


def _run_ci_gate(ci_script: Path, meta: Path) -> None:
    timeout = _gate_timeout(meta)
    started = time.monotonic()
    try:
        result = subprocess.run(
            [sys.executable, str(ci_script), "--meta", str(meta)],
            capture_output=True, text=True, timeout=timeout,
            # F-013: without this, a non-UTF-8 stdio environment (ASCII
            # locale, PYTHONIOENCODING=ascii, …) kills the child on
            # UnicodeEncodeError before it can print a verdict — on a
            # violating project exactly as on a clean one — so the
            # crashed-gate branch below fails EVERY commit open instead of
            # blocking real violations. Pinning UTF-8 on both the decode and
            # the child's own stdio lets the gate report in any locale.
            encoding="utf-8", errors="replace",
            env={**os.environ, "PYTHONIOENCODING": "utf-8"},
        )
    except subprocess.TimeoutExpired:
        elapsed = _journal_run(meta, "timeout", started)
        _fail_open(meta, "timeout", f"CI gate exceeded its {timeout:g}s budget.",
                   elapsed)
    except Exception as e:  # noqa: BLE001 — any failure to RUN it fails open
        elapsed = _journal_run(meta, "failure", started)
        _fail_open(meta, "failure", f"CI gate could not be executed: {e!r}", elapsed)
    output = result.stdout + result.stderr
    tagged = [line for line in output.splitlines()
              if any(tag in line for tag in _GATE_VERDICT_TAGS)]
    for line in tagged:
        print(f"forge(ci): {line}", file=_err_stream())
    if result.returncode == 0:
        _journal_run(meta, "pass", started)
        return
    if not tagged:
        # Distinguish a CRASHED gate from a VIOLATED one (F-004). The only
        # previous producer of `failure` was `except Exception` above, which
        # fires only when the child cannot be SPAWNED at all — a gate that
        # runs and then crashes exits non-zero same as a real violation, and
        # was silently treated as one (blocked, nothing journalled). A
        # non-zero exit that said nothing recognizable did not VIOLATE
        # anything; it just never got that far.
        elapsed = _journal_run(meta, "failure", started)
        _fail_open(
            meta, "failure",
            f"CI gate exited {result.returncode} without reporting a verdict "
            "(it likely crashed before it could) — run it standalone to see why.",
            elapsed)
    _journal_run(meta, "block", started)
    print(
        "\nforge: commit blocked — CI gate failed. Run /forge:ci for details.",
        file=_err_stream(),
    )
    sys.exit(2)


def _check_blocking_only(meta: Path) -> None:
    """Fallback: block only on unresolved blocking decisions."""
    open_file = forge_paths(meta)["decisions"] / "open.yml"
    if not open_file.exists():
        return
    if not yaml_available():
        warn("blocking-decisions check skipped — no YAML library available "
             "(pip install ruamel.yaml); the gate did NOT run.")
        return
    data = load_yaml(open_file)
    if data is None:
        warn(f"blocking-decisions check skipped — could not parse {open_file}.")
        return
    blocking = [
        d for d in (data.get("open") or [])
        if isinstance(d, dict)
        and d.get("status") == "open"
        and "synthesis" in (d.get("blocks") or [])
    ]
    if not blocking:
        return
    warn(f"commit blocked — {len(blocking)} blocking architectural decision(s) unresolved:")
    for d in blocking:
        q = str(d.get("question") or "").replace("\n", " ").strip()
        print(f"  {d.get('id', '?')}: {q[:80]}", file=_err_stream())
    print("\nResolve with /forge:architect-adr-writer, then commit.", file=_err_stream())
    sys.exit(2)


def _residual_guard() -> NoReturn:
    """Render an UNFORESEEN failure as a declared outcome instead of exit 1.

    Every known failure above is already handled; this exists for the ones that
    are not, because the cost of missing one is specific and severe: a
    traceback exits 1, this consumer reads 1 as a non-blocking error, and the
    gate disarms itself silently. So the residual case is turned into the same
    loud fail-open every other unrunnable-gate outcome uses (NFR-006/NFR-010).

    `Exception`, deliberately not `BaseException`: `SystemExit` is how BOTH
    verdicts leave this file — including the deny path's own ``sys.exit(2)`` —
    and `KeyboardInterrupt` is the user, not a bug. Catching those would make
    this handler swallow the refusal it is here to protect.
    """
    try:
        main()
    except Exception as e:  # noqa: BLE001 — see docstring: 1 must never escape
        warn(f"!! PRE-COMMIT HOOK CRASHED ({e!r}) - COMMIT ALLOWED WITHOUT THE "
             "GATE (fail-open, NFR-006). Run /forge:ci manually to verify it.")
        sys.exit(0)
    sys.exit(0)


if __name__ == "__main__":
    _residual_guard()
