# forge
#!/usr/bin/env python3
"""
forge pre-push hook  —  TWO POSITIONS, TWO DIFFERENT QUESTIONS (ADR-0023)

This one file is invoked from two places, and the whole point of CARD-018 is
that they no longer ask the same thing:

1. **As git's own `pre-push` hook** (``--git-hook``, reached through
   `hooks/git-pre-push.sh`) — THE AUTHORITY ON WHAT IS PUBLISHED. git hands it
   ``<local ref> <local sha> <remote ref> <remote sha>`` on stdin, one line per
   ref being updated, and the release eval verdict (NFR-004, INV-006, ADR-0020,
   COMP-058) is evaluated against the LOCAL SHA of any line whose REMOTE REF is
   the release ref. **INV-006 lives here and nowhere else.**
2. **As a PreToolUse hook on Bash** — TWO gates, neither of which reads a
   verdict:
   * blocking architectural decisions (FR-009, INV-014) — unchanged, G-2:
     same condition, same message, same exit 2, still checked first;
   * the DISARM DETECTOR (:func:`_disarm`) — "does this command stop the
     authoritative gate from running?".

The verdict machinery both positions would otherwise duplicate lives in
`hooks/eval_verdict.py` (CARD-014). Only position 1 consults it.

────────────────────────────────────────────────────────────────────────────
WHY THE CLASSIFIER IS GONE, AND WHAT WENT WITH IT
────────────────────────────────────────────────────────────────────────────

Until CARD-018 the release gate ran at PreToolUse and had to RECONSTRUCT what
a push publishes by parsing the command string. Three review cycles measured
that this question does not close in that position: cycle 1 found a refspec
whose source was parsed and then discarded (`git push origin evil:main`
verified `HEAD`); cycle 2 found `--all` / `--mirror` publishing
`refs/heads/main` with no destination token at all; cycle 3 — after the default
had already been inverted — found a regression the fix introduced
(``git push a && git push main`` went silent), plus `bash -c` and git aliases.
Each cycle declared its residue honestly under ADR-0022/R1, and the next cycle
found a new form. ADR-0023/R1 is the rule that came out of it: extending the
enumeration and declaring the residue is NOT sufficient — the gate moves to the
position whose input is authoritative.

So `_push_arguments`, `_publication` and `_refuse_undetermined` are DELETED,
not narrowed, and `eval-gate-undetermined` left the token vocabulary with them
(a token no route can emit is a word a consumer waits for forever).
`_eval_gate_applies` moved to `hooks/eval_verdict.py` instead: its body asks
whether the project has an eval corpus at all, which is a repository fact both
positions need — CARD-014's review caught it filed as position-dependent, and
had it stayed here the git hook would have refused every push in every project
that never declared an eval corpus.

**26 push shapes were driven against real git 2.50.1 before ADR-0023 was
written.** The fifteen the text position got wrong or could not answer are
answered exactly by one stdin line, sha included. That measurement is not
repeated here; what IS driven here is this implementation, against a real bare
origin, in `tests/test_publication_authority.py`.

────────────────────────────────────────────────────────────────────────────
WHAT THIS POSITION MUST DECLARE RATHER THAN SOLVE — EVERY ONE MEASURED
────────────────────────────────────────────────────────────────────────────

**1. `--dry-run` is indistinguishable from a real push here.** git calls the
hook for a rehearsal and hands it the byte-identical stdin; no environment
variable distinguishes the two. Of 26 measured forms it is the single
structural divergence between what stdin STATES and what actually happens. The
old text position DID read the flag, and that was the one thing it did better —
past tense on both halves, because the classifier that read it is deleted.
**Nothing in this file reads `--dry-run` today.** In the PreToolUse position a
rehearsal exits 0 because no disarming spelling and no blocking decision were
found, not because a flag was recognised; in the git position it is refused
like any other publishing line. No signal is invented; the price is that a
rehearsal of a release push is refused. DRIVEN, not merely written:
`PublicationResidueTest::test_a_dry_run_is_refused_exactly_like_a_real_push`
runs a real ``git push --dry-run`` and asserts both halves (refused; origin
unmoved), so the day git grows a signal the test reddens.

**2. NFR-006's fail-open is NOT inherited by a git hook, so it is
RE-IMPLEMENTED here rather than assumed.** At PreToolUse a crashing hook exits
1 and Claude Code treats 1 as non-blocking — the work is not held hostage. In
the git position exit 1 means REFUSE (measured: a Python traceback rejects the
push). So fail-open is rebuilt in two layers, and both are driven:
* inside this file — every path where the gate could not RUN ends in an
  explicit `_fail_open` and an exit 0 (the enumeration below);
* in `hooks/git-pre-push.sh` — the only refusal is the gate's deliberate
  :data:`EXIT_REFUSE`; every other exit code, including a crash before Python
  starts, allows the push loudly. That is the half this file cannot implement,
  because it is the half where this file did not run.

**⚑ THE CONFLICT IS OPEN AND IS NOT CLOSED BY THIS CARD.** INV-006 says a
release without a verdict is blocked, always. NFR-006 says a broken gate does
not block. In this position those two meet on the same code path, and the
re-implementation above chooses NFR-006 for "the gate could not RUN" and
INV-006 for "the gate ran and found nothing". That is the same split CARD-014
drew, carried across; it does NOT amend NFR-006 or NFR-010, both of which still
read as if fail-open were inherited. ADR-0023 states the conflict explicitly
and hands the amendment to the architecture station. Until it is amended, the
requirement text and this code disagree about which of them describes the
other.

**3. ADR-0007/R1's declared exit set carries no information out of this
channel.** Measured on 1, 2, 3 and 42: `git push` returns exactly 1 and rejects
the push in all four cases — git collapses every non-zero hook code into its
own. So the DECLARED SET IS KEPT (this file still exits only 0 or
:data:`EXIT_REFUSE`, and the wrapper translates), but the discriminator a
consumer actually reads is the TOKEN VOCABULARY on stderr
(`hooks/eval_verdict.py::EVAL_VERDICT_TOKENS` for the verdict,
:data:`DISARM_TOKENS` for the detector). Driven by
`TestPublicationAuthority_EveryMeasuredShapeIsAnswered::test_git_collapses_every_non_zero_hook_code_into_one`.

**4. And one this card inherits rather than introduces: THE REMEDY HINT IS AN
ATTACK SURFACE.** ADR-0023 measured it with live agents — on the arm whose
refusal named a cure, BOTH agents ran the cure and published, and both then
noticed the cure evaluates nothing and writes `verdict: passed`
unconditionally. A refusal that names a remedy delegates trust to that remedy
wholesale. This card does not remove the hints (they are `_refuse_missing`'s
and `_fail_open`'s, pinned by CARD-014's own assertions, and ADR-0023 assigns
the observation to RISK-006 rather than to itself) — it BOUNDS them, and the
bound is SPLIT BY DANGER rather than drawn across "every route", which is the
sentence three passes got wrong.

* **Every route that REFUSES is driven and collected, exhaustively** — the
  eval refusals, all three disarm tokens, and :func:`_decision_gate`'s
  blocking refusal. That is the half ADR-0023 measured: on the arm whose
  refusal named a cure, both agents ran it. The INVENTORY is derived from the
  token vocabularies rather than counted, so a fourth refusal cannot arrive
  outside the bound — `RemedyHintSurfaceTest::test_every_refusal_route_is_driven`.
* **The fail-open routes are a declared k of n** — 2 of 12, with the other ten
  NAMED BY SHAPE in `RemedyHintSurfaceTest::UNCOLLECTED_FAIL_OPEN` and the
  partition derived from the `# fail-open <n>` / `# fail-open w<n>` tags in
  this file and in the wrapper. A fail-open message is advice beside a push
  that is already permitted, not a condition of a refusal, and the two
  collected routes exercise the only two places on that side where a hint is
  written at all: :func:`_fail_open` ITSELF (the seven uncollected python
  sites contribute a free-text `detail` and nothing else) and the wrapper's
  one route that names a remedy.

The comparison is against a declared set IN BOTH DIRECTIONS, so a new remedy
hint is a red test rather than a wider surface, and a declared hint that no
route emits is a red test rather than a wish list.

THE COUNT WAS WRONG THREE TIMES, and each time in this sentence. It drove
three under a claim of "all its routes" (`_fail_open`, `_refuse_blocked` and
the announcement were never run); then seven under the same claim, with
`_decision_gate`'s blocking refusal — the only real refusal outside the
collector — still uncollected, against about nineteen places that print to a
reader. The universal is gone rather than re-costed. What the collector still
cannot see is a hint OUTSIDE BACKTICKS that is not a slash command:
`_decision_gate`'s `(pip install ruamel.yaml)` is one, held instead as a
LITERAL by
`DecisionGateUnchangedTest::test_the_frozen_degradation_text_is_frozen_by_this_test_too`,
because the text is pre-card and G-2's "same message" was, until that test, a
sentence no assertion carried. Its `/forge:architect-adr-writer` is another —
and that one IS collected, because a slash command is executable text whether
or not anyone backticked it, and G-2 forbids backticking this one.

EXIT CODES (the declared set — ADR-0007/R1), for BOTH positions and every
route:

* ``0`` — allow. A clean gate, a push this gate does not govern, and every
  LOUD fail-open.
* ``2`` (:data:`EXIT_REFUSE`) — refuse. The only code Claude Code's PreToolUse
  protocol reads as "do not run this command"; in the git position the wrapper
  maps it to git's 1. EVERY refusal reason carries it, and they are told apart
  in the OUTPUT, never by the number. No count is written here on purpose:
  three passes over the fail-open set once produced three different numbers,
  and the sentence that got it wrong was the one promising a count nobody could
  check. The reasons are:

      push blocked - <n> blocking architectural decision(s)   (pre-existing)
      forge(eval-gate): eval-gate-failed - …                  (AC-020)
      forge(eval-gate): eval-gate-missing - …                 (INV-006)
      forge(disarm): disarm-no-verify - …                     (AC-040)
      forge(disarm): disarm-hooks-path - …                    (AC-040)
      forge(disarm): disarm-send-pack - …                     (AC-040)
      forge: selftest-gate-red - …                            (AC-639/641)
      forge: selftest-gate-unrunnable - …                     (AC-641, EC-155)

  ``1`` IS NOT IN THE SET, AND WAS, WHICH IS WHY THIS PARAGRAPH EXISTS. Review
  measured it twice: a stored document whose `scenarios:` was a scalar died in
  the refusal PRINTER after two refusal lines (cycle 1), and an ``open.yml``
  written as a YAML LIST made `_decision_gate` raise `AttributeError` before
  the eval gate was ever called — one malformed model file disarming INV-014
  and INV-006 together (cycle 2). Every read of an untyped `load_yaml` result
  and of the stdin payload is type-guarded at its own call site, and
  `OutputDisciplineTest::test_every_exit_code_is_one_of_the_two_declared`,
  `::test_the_declared_set_covers_the_DECISION_gate_too` and
  `::test_a_payload_that_is_not_the_expected_shape_is_never_a_third_code`
  drive all three surfaces.

  THEY ARE NOT SYMMETRIC. The eval and disarm refusals lead with a STABLE
  token from a closed set. The DECISION refusal carries no token at all: it is
  identified by its own stable prefix ``push blocked — ``, and a consumer that
  looks for a token identifies it by ABSENCE. G-2 freezes that refusal, so this
  card states the gap instead of closing it. The trigger CARD-014 wrote here
  has now fired ONCE — the disarm detector is the third exit-2 producer, and it
  arrived with its own closed vocabulary (:data:`DISARM_TOKENS`) in the same
  change, exactly as instructed. It fired AGAIN with CARD-174: the selftest
  gate is the fourth, and its two tokens (:data:`SELFTEST_GATE_RED`,
  :data:`SELFTEST_GATE_UNRUNNABLE`) arrived with it and are collected by
  `RemedyHintSurfaceTest::test_every_refusal_route_is_driven`. They are written
  through the shared `warn`, so they lead with ``forge: `` and the token is the
  discriminator. The trigger stands for a fifth.

THE SELFTEST PUSH GATE (CARD-174, FR-112(b)) — ONE ADDED BRANCH, OPPOSITE
POLARITY. After the eval gate, the git position asks a second question: is the
full selftest green on what this push publishes to the target branch? The
answer is the exit status of `ci/selftest.py` run as its own process
(:func:`_run_selftest`), never its output and never a pipeline status — pinned
over every push gate of the target branch by
`TestPushGate_SelftestExitCodeIsNotPiped`. Once in force it has NO fail-open:
a selftest that could not run is not a selftest that exited 0, so it refuses.
When it is in force, and every refusing path, is :func:`_selftest_verdict`'s
docstring; the polarity split is :func:`_selftest_gate`'s.

WHY NO STDOUT VERDICT OBJECT, STATED RATHER THAN OMITTED. `hooks/pre-commit.py`
prints a PreToolUse deny object on stdout for its secret-scan refusal, and the
same shape was available here. It is not taken, for two reasons that are worth
more than the extra structure: ADR-0020 prescribes this gate's shape verbatim
("отказывает в публикации кодом 2 … по той же схеме, по которой сегодня
блокируются push при нерешённых DEC"), and `_err_stream`'s k-of-n declaration in
`hooks/_forge_common.py` carries the other seven hooks' raw `sys.stderr` writes
on the stated ground that "none of the seven emits a stdout verdict object".
Growing one here fires that declaration's own revisit trigger, and G-3 freezes
the file where the declaration lives. THE TRIGGER, restated: the moment this
gate is asked for a JSON verdict, every human write in this file must move to
`_err_stream` and `_forge_common`'s count must go 1-of-8 → 2-of-8 in the same
change. In the GIT position a stdout object would be doubly wrong — git shows
hook stdout to the terminal, so a machine object there lands in a human's
scrollback.

Until then the declaration stays TRUE: every line the eval gate and the disarm
detector write goes through `hooks/eval_verdict.py::_say` or :func:`_disarm_say`,
both of which resolve the stream through `_err_stream`, and this file still
holds exactly the TWO raw stderr writes that declaration counts, both in
:func:`_decision_gate`, both untouched. Driven by
`ClosedDescriptorTest::test_the_err_stream_guard_declares_k_of_n_not_a_property`
in `tests/test_precommit_secret_scan.py`.

FAIL-OPEN, AND THE ONE THING THAT IS NOT ONE (G-2, NFR-006, NFR-010). The gate
splits two questions a single "no" used to conflate:

* **There is no valid verdict** — the gate RAN and its answer is "nothing
  answers for this commit". That is a REFUSAL (INV-006).
* **The gate could not run at all.** Nothing was measured, so nothing can be
  concluded: the push is ALLOWED, LOUDLY, and journalled to
  ``meta/gate-events.yml`` where /forge:status flags it.

  ONE ENTRY PER `_fail_open` CALL SITE, each naming the test that DRIVES it
  (ADR-0022/R1), all in `tests/test_prepush_eval_gate.py` unless said
  otherwise. THE COUNT IS NOT WRITTEN HERE and must not be: three passes over
  this set once produced three different numbers. It is DERIVED by
  `FailOpenEnumerationTest`, which parses this file, counts the call sites, and
  compares them against the entries below:

  1. not inside a git work tree —
     ``FailOpenTest::test_a_project_that_is_not_a_git_repository_fails_open``
  2. the RELEASE branch is not resolvable —
     ``FailOpenTest::test_an_unresolvable_release_branch_fails_open``
  3. no YAML library —
     ``FailOpenTest::test_no_yaml_library_fails_open_rather_than_refusing``
  4. a stdin line names a local sha that resolves to no commit HERE — git
     stated it and this repository cannot show it, so WHAT would be published
     is unknown —
     ``FailOpenTest::test_a_published_sha_that_resolves_to_nothing_fails_open``
  5. git could not ANSWER at all — missing from PATH, over
     `hooks/eval_verdict.py::GIT_TIMEOUT` (journalled ``outcome: timeout``, the
     only thing pinning `_GateUnavailable`'s ``"budget" in str(exc)`` split), or
     exiting non-zero on a plumbing call —
     ``FailOpenTest::test_git_missing_from_path_fails_open``,
     ``FailOpenTest::test_a_git_that_exceeds_its_budget_fails_open_as_a_timeout``
  6. the gate raised something none of the above classifies — the BACKSTOP, so
     that a crash in this gate cannot refuse a push it never measured —
     ``FailOpenTest::test_an_unexpected_failure_inside_the_gate_fails_open``
  7. a stdin line carried between one and three fields, so it is not the
     four-field statement git's pre-push protocol writes and cannot be read as
     one. Entries 7 and 8 are the same condition reached before and after the
     conclusion loop, and they are two call sites rather than one because a
     refusal exits from inside that loop — junk on stdin must not be a way to
     turn a block into a fail-open —
     ``FailOpenTest::test_a_partial_stdin_line_fails_open_rather_than_being_read``
  8. the same, on a payload that also carried readable lines, none of which
     refused —
     ``FailOpenTest::test_a_partial_line_beside_a_readable_one_still_fails_open``

  EXACTLY ONE PATH IS NOT BEHIND A `_fail_open`, and naming it is the point of
  naming any of them: resolving `meta/` itself (`find_meta`) happens before
  there is a journal to write to, so a failure there cannot be recorded by this
  process. Everything after it, the applicability check included, is inside the
  `try` — which is why that check reads as one line inside the block rather
  than as a guard clause above it. It is covered ONE LAYER OUT, by the wrapper's
  "anything but EXIT_REFUSE allows" rule, and driven from there —
  ``WrapperFailOpenTest::test_a_hook_that_crashes_allows_the_push``.

WHAT IS NOT A FAIL-OPEN: a gate that RAN and found no answering verdict
refuses. The could-not-RUN checks come FIRST.

WHICH PUSHES THIS GOVERNS. Only a push that updates the release ref, and only
in a project that has a release eval suite at all
(`hooks/eval_verdict.py::_eval_gate_applies`). "Which" is now git's answer, not
a reading: a ``(delete)`` line publishes no commit and is skipped, and an empty
stdin publishes nothing at all — in which case git does not run this hook.

WHERE THE VERDICT IS READ FROM. Out of the TREE OF THE COMMIT BEING PUBLISHED
(`git ls-tree` + `git cat-file` in `hooks/eval_verdict.py::_stored_documents`),
never out of the working tree. **A verdict that has not been committed is not
being published, and cannot answer for a push.**

THE BOOTSTRAP FIXED POINT — READ `hooks/eval_verdict.py::_answers_for_head`
BEFORE CHANGING THE FRESHNESS RULE. A verdict is produced by a run that happens
BEFORE the commit that carries it, so a rule of strict "verdict.commit == HEAD"
has NO fixed point: the artefact always names its own commit's parent.

Install via: /forge:init hooks  (BOTH halves — the PreToolUse entry in
`.claude/settings.json` AND the `pre-push` hook in the directory git names.
An install that has one and not the other is a half-armed gate, and
`ci/hook_install_drift.py` now reports it as one, alongside the other two git
hooks, including the EXECUTABLE BIT — git skips a non-executable hook silently.
That check ASKS git for the hook directory (`git rev-parse --git-path hooks`)
rather than building `<project>/.git/hooks`, because the built path is wrong
under `core.hooksPath` and wrong in every linked worktree, which is where every
forge card is developed.)
"""
from __future__ import annotations

import json
import os
import re
import signal
import subprocess
import sys
from pathlib import Path

from _forge_common import (_err_stream, command_matches, find_meta,
                           forge_paths, load_yaml, warn, yaml_available)
from eval_verdict import (EVAL_REPORT_DIR_REL, EVAL_SUITE,
                          RELEASE_BRANCH_FALLBACKS, _eval_gate_applies,
                          _fail_open, _GateUnavailable, _git, _nearest,
                          _read_verdicts, _refuse_blocked, _refuse_missing,
                          _release_branch, _repo_root, _say)

#: The declared exit codes, as named constants because two files depend on the
#: refusal one: `hooks/git-pre-push.sh` translates exactly this number into
#: git's own 1 and treats every other non-zero as a crash to fail open on.
EXIT_ALLOW = 0
EXIT_REFUSE = 2

#: The line git writes for a ref being DELETED. Its companion is a local sha of
#: all zeros; both are checked, because git states the deletion twice and a
#: reader that trusts one field is a reader that can be fooled by the other.
DELETE_REF = "(delete)"

#: The closed vocabulary of the disarm detector, mirroring
#: `hooks/eval_verdict.py::EVAL_VERDICT_TOKENS` in purpose: the exit code is not
#: a discriminator (it is 2 for every refusal in this file), so the token is.
#: Separate from the verdict vocabulary because these are separate FACTS —
#: "this command stops the gate from running" is not "no verdict answers", and
#: AC-040 requires a consumer to tell them apart.
DISARM_NO_VERIFY = "disarm-no-verify"
DISARM_HOOKS_PATH = "disarm-hooks-path"
DISARM_SEND_PACK = "disarm-send-pack"
DISARM_TOKENS = (DISARM_NO_VERIFY, DISARM_HOOKS_PATH, DISARM_SEND_PACK)

#: The closed vocabulary of the SELFTEST push gate (CARD-174, FR-112(b)) — the
#: fourth exit-2 producer, arriving with its own tokens as the trigger in this
#: module's docstring requires. Two facts, told apart for the consumer: the
#: selftest RAN and its own exit code was not 0 (red), versus the gate is in
#: force and the selftest could not be run on the tree being published
#: (unrunnable). Both REFUSE — this gate has no fail-open; see
#: :func:`_selftest_gate`.
SELFTEST_GATE_RED = "selftest-gate-red"
SELFTEST_GATE_UNRUNNABLE = "selftest-gate-unrunnable"

#: The script whose OWN exit code decides a push of the target branch, relative
#: to the work tree root. Its presence is also what puts the gate in force.
SELFTEST_SCRIPT = "ci/selftest.py"

#: Seconds the selftest may run before the push is refused as unrunnable. The
#: variable cannot open the gate: a shorter budget only refuses sooner, and a
#: value that is not a positive integer refuses outright.
SELFTEST_TIMEOUT_ENV = "FORGE_SELFTEST_TIMEOUT"
SELFTEST_TIMEOUT_DEFAULT = 3600


def _disarm_say(line: str) -> None:
    """One line of the disarm detector's output, on the stream humans read.

    Through `_err_stream` rather than `sys.stderr` for the ADR-0007/R2 reason:
    a descriptor closed at process start makes `sys.stderr` literally None and
    `print(file=None)` writes to STDOUT. Its own prefix rather than
    `hooks/eval_verdict.py::_say`'s: `forge(eval-gate):` on a refusal that
    never looked at a verdict would be the confusion AC-040 exists to prevent.
    """
    print(f"forge(disarm): {line}", file=_err_stream())


def _disarm(command: str) -> list[tuple[str, str]]:
    """Which documented ways of skipping the git `pre-push` hook this command uses.

    ─────────────────────────────────────────────────────────────────────────
    WHY THIS QUESTION CLOSES WHERE THE OLD ONE DID NOT (ADR-0023/R1)
    ─────────────────────────────────────────────────────────────────────────

    Its predecessor in this file asked "does this command publish the release
    branch?" — a question whose answer lives in git's configuration, in the
    second command after ``&&``, and in another interpreter's argument, none of
    which the text can be trusted for. Three review cycles measured three
    different unlisted spellings.

    This question is different in kind, not in length: **the list of ways to
    prevent a git hook from running is git's own documentation.** githooks(5)
    and git-push(1) name three client-side mechanisms, and all three must NAME
    THEMSELVES in the text, because each is an option or a configuration KEY:

    * ``--no-verify`` — git-push(1): "bypass the pre-push hook". There is NO
      short form on `git push`: ``-n`` there is ``--dry-run``, which RUNS the
      hook (measured against git 2.50.1 for this card, correcting the
      parenthetical "`--no-verify`/`-n`" in ADR-0023's Decision §2 — the short
      form belongs to `git commit`, a different hook).
    * ``core.hooksPath`` — git-config(1): relocates the hook directory. Its
      three spellings (``git -c core.hooksPath=…``, the
      ``GIT_CONFIG_COUNT``/``KEY``/``VALUE`` environment triple, and a
      persisted ``git config core.hooksPath …``) are ONE rule here, and that is
      the argument for boundedness in one line: every documented way to set a
      config value carries the KEY NAME in the text. Matched
      case-insensitively, because git config keys are.
    * ``git send-pack`` — the plumbing that pushes without running the
      porcelain's hooks.

    **THIS DETECTOR OVER-DETECTS ON PURPOSE, AND THAT IS THE INVERSION.** It
    does not strip quoted segments (so ``bash -c "git push --no-verify"`` is
    caught, which the classifier's `command_matches` would have missed), and it
    does not verify that the token is really an argument of a really-executing
    git. A command that merely MENTIONS ``--no-verify`` is refused. That is a
    false refusal, and a false refusal is a sentence in a terminal; the
    opposite error is a release published past its gate. The old classifier's
    failure mode ran the other way, which is what made every miss a Critical.
    Pinned by ``TestDisarmDetector_RefusesEveryDocumentedBypass::
    test_a_mere_mention_is_refused_and_that_is_the_declared_direction``.

    **WHAT IT DOES NOT COVER, AND WHY THAT IS NOT A LONGER LIST.** Three of the
    eight disarm spellings ADR-0023 measured — ``chmod -x``, ``rm`` and ``mv``
    of the installed hook file — differ in kind from the three above: what they
    leave behind is a STATE, and the command that produced it is unbounded in
    spelling. A hook file can be unlinked by an editor, a script, `find -exec`,
    or a shell that no PreToolUse hook ever sees; enumerating "commands that
    can remove a file" is precisely the defect ADR-0023/R1 names.

    **A PERSISTED ``core.hooksPath`` IS THE FOURTH, AND IT IS NOT THIS
    FUNCTION'S.** The rule above catches the one command that SETS the key,
    which is worth having; it catches nothing afterwards. The key lives in
    `.git/config`, and every push that follows is ungated with no token in any
    command text — set once by an editor, a script, a terminal outside Claude
    Code, or an inherited config, and this detector never sees anything at all.
    The residue is therefore STATE-shaped exactly like the three above, and
    review cycle 1 found both files handing it to each other: this paragraph
    once argued the case "stays in the list above", while the drift gate
    reconstructed `<project>/.git/hooks` and could not see a relocation either.
    Neither owned it, and a `git config core.hooksPath /dev/null` published an
    unevaluated commit to the release ref with both halves silent (measured).

    All four have ONE remedy, and it is a STATE check:
    `ci/hook_install_drift.py` asks git where the hooks are
    (`git rev-parse --git-path hooks` — which honours `core.hooksPath` and
    every linked worktree) and surveys what it finds there for existence, byte
    identity, the executable bit, and a relocation (AC-041). This is the honest
    boundary of the bounded question, and it is driven —
    ``TestDisarmDetector_TheInvisibleHalfIsCoveredByDriftDetection``.

    Args:
        command: The shell command Claude Code is about to run.

    Returns:
        ``[(token, what_is_disarmed)]``, empty when the command uses none of
        them. A command may use more than one, and all are reported: a refusal
        that names one of two bypasses invites the other.
    """
    found: list[tuple[str, str]] = []
    if re.search(r"(?<![\w-])--no-verify(?![\w-])", command):
        found.append((
            DISARM_NO_VERIFY,
            "`--no-verify` tells git to skip the pre-push hook entirely "
            "(git-push(1)), so the release eval verdict would never be read"))
    if re.search(r"core\.hookspath", command, re.IGNORECASE):
        found.append((
            DISARM_HOOKS_PATH,
            "`core.hooksPath` relocates the hook directory (git-config(1)), so "
            "git looks for pre-push somewhere the installed gate is not — "
            "whether it is set with `git -c`, with GIT_CONFIG_KEY_*, or "
            "persisted with `git config`"))
    if re.search(r"(?<![\w])send-pack(?![\w-])", command):
        found.append((
            DISARM_SEND_PACK,
            "`git send-pack` is the plumbing behind `git push` and does not run "
            "the porcelain's pre-push hook, so the release eval verdict would "
            "never be read"))
    return found


def _refuse_disarm(findings: list[tuple[str, str]]) -> None:
    """AC-040: refuse a command that would stop the authoritative gate running.

    A DIFFERENT FACT from every eval-gate refusal, and it carries its own
    vocabulary for exactly that reason (ADR-0007/R1): nothing was looked up
    here. No verdict was read, no commit was resolved, and this refusal says
    nothing about whether the release would have passed — only that the thing
    that could have answered would not have run.

    Each finding is printed with the token FIRST and the mechanism named, so a
    command using two bypasses is refused with both named; a refusal that names
    one invites the other.

    ⚑ NO REMEDY IS NAMED, and the omission is the measurement. ADR-0023 ran 12
    live agents against a refusing hook: on the arm whose refusal named a cure,
    BOTH agents ran the cure and published — the cure being a runner that
    writes `verdict: passed` unconditionally. A refusal that names a remedy
    delegates trust to that remedy wholesale. The eval gate's own refusals
    inherit their hints from CARD-014 and are bounded by
    `RemedyHintSurfaceTest` rather than removed; this refusal, which is new, is
    written without one. What it says instead is what is TRUE and not
    actionable-by-command: the gate is the authority, and going around it is
    itself the reportable act.
    """
    for token, what in findings:
        _disarm_say(f"{token} — push blocked: this command disarms the release "
                    f"eval gate.")
        _disarm_say(f"  {what}.")
    _disarm_say("The git `pre-push` hook is the AUTHORITY on what a push "
                "publishes (ADR-0023): it is the only position handed what git "
                "itself is about to do. A command that prevents it from running "
                "is not a faster push — it is an unmeasured release, and this "
                "refusal is the record that it was attempted.")


def _stated_publications(payload: str,
                  release: str) -> tuple[list[tuple[str, str]], list[str]]:
    """Which commits this push publishes to the release ref, according to git.

    THE WHOLE OF THE AUTHORITY, and it is nine lines because the answer is
    given rather than derived. git writes one line per ref it is about to
    update:

        <local ref> <local sha> <remote ref> <remote sha>

    The question "does this push publish the release branch?" is the fourth
    field's third column; the question "which commit?" is the second. Neither
    is reconstructed: `push.default`, `remote.<name>.push`, a refspec whose
    source is a tag or a tree, a shell-expanded ``$BRANCH``, ``--all``,
    ``--mirror``, a second push after ``&&``, `bash -c` and a git alias are all
    already RESOLVED by the time git writes these lines. That is what ADR-0023
    means by an authoritative input, and it is why this function has no list of
    forms in it.

    TWO STATES PUBLISH NOTHING and both are handled here rather than by a
    caller's default:

    * a DELETION — git writes ``(delete)`` as the local ref and a local sha of
      all zeros. All three spellings (``:main``, ``--delete main``, ``-d main``)
      produce the identical line. Both fields are checked: git states the
      deletion twice, and a reader that trusts only one can be fooled by the
      other.
    * an EMPTY payload — nothing is being updated. git does not even run the
      hook in that case, so this is belt-and-braces against a caller that
      invokes the hook by hand.

    A PARTIAL LINE IS NOT GUESSED AT AND IS NOT PASSED OVER EITHER, and the
    boundary between those two is where a mutant of this function survived its
    own battery. git's protocol is four fields; a line carrying one, two or
    three is not a statement this hook can read, and reading `fields[2]` out of
    it anyway would be inventing one. It is therefore reported back to the
    caller as UNREADABLE, which is a loud fail-open — "the gate could not read
    its input" is the same class as "git could not answer", not a verdict.
    An EMPTY line is different and stays silent: it carries no statement to
    fail to read, and git writes none.

    That distinction is deliberately NOT drawn the other way round. Failing
    open on the whole payload the moment any line is unreadable would make a
    single junk byte on stdin a way to disarm the gate, so the readable lines
    are judged FIRST — a refusal exits from inside the caller's loop — and the
    fail-open is reached only if nothing concluded.

    Args:
        payload: Everything git wrote to stdin, verbatim.
        release: The release branch NAME, as `_release_branch` resolved it.

    Returns:
        ``([(local ref, local sha)], [unreadable line])`` — the first for every
        line that publishes a commit to the release ref, in the order git
        stated them; the second for every line that carried between one and
        three fields.
    """
    release_ref = f"refs/heads/{release}"
    publishing: list[tuple[str, str]] = []
    unreadable: list[str] = []
    for raw in payload.splitlines():
        fields = raw.split()
        if not fields:
            continue
        if len(fields) < 4:
            unreadable.append(raw.strip()[:120])
            continue
        local_ref, local_sha, remote_ref = fields[0], fields[1], fields[2]
        if remote_ref != release_ref:
            continue
        if local_ref == DELETE_REF or set(local_sha) == {"0"}:
            continue
        publishing.append((local_ref, local_sha))
    return publishing, unreadable


# ── the gates ─────────────────────────────────────────────────────────────


def _blocks_synthesis(entry: dict) -> bool:
    """Does this decision entry block synthesis?

    A separate function because `blocks:` is a field a human types into a YAML
    file, and `"synthesis" in <int>` raises `TypeError` — the SAME exit-1 shape
    as the `data.get` crash one call site up, found by the same battery. So
    membership is asked only of something that can answer it.

    A STRING is deliberately still asked: `blocks: synthesis` (unquoted, no
    list) has always blocked the push, and G-2 freezes the BLOCKING behaviour,
    not merely the message. Anything that is neither a container nor a string
    — an int, a float, a bool, a date — does not block, which is the same
    answer the previous code gave for `blocks: null` and `blocks: []`.

    WHY THIS DID NOT MOVE TO `hooks/eval_verdict.py`. It was on the CARD-014
    extraction list and does not belong there, checked against the code rather
    than the list: its only caller is :func:`_decision_gate` (the FR-009 /
    INV-014 gate G-2 freezes), and it knows nothing about an eval verdict. A
    verdict module holding a decision-gate helper would make its own name a
    false statement about its contents.
    """
    blocks = entry.get("blocks") or []
    if not isinstance(blocks, (list, tuple, set, frozenset, dict, str)):
        return False
    return "synthesis" in blocks


def _decision_gate(meta: Path) -> None:
    """Refuse the push while a decision blocks synthesis (FR-009, INV-014).

    CARD-014 moved this body out of :func:`main` and changed NOTHING inside it
    (G-2): same condition, same message, same exit 2. CARD-018 changed nothing
    inside it either, and left it in the PreToolUse position it has always
    occupied — INV-014 already records that this gate does not see a terminal
    push, and moving it would have been a second decision smuggled in beside
    ADR-0023's. What DID change once is what happens when it has nothing to say
    — it RETURNS instead of exiting 0, so a missing `open.yml` or a missing YAML
    library can no longer disarm the gate that follows it. The two raw
    ``sys.stderr`` writes below are the ones `_forge_common._err_stream`'s
    k-of-n declaration counts for this file; they are left exactly as they were.

    FOUR of the six sites in the ADR-0022/R2 type-guard enumeration are in this
    function, and the enumeration itself lives on
    `hooks/eval_verdict.py::_refuse_blocked`, which carries the other two. It
    stayed with that function through the CARD-014 extraction because the
    operation it enumerates — "reach into a value that came out of an untyped
    `load_yaml` result" — does not stop at the file boundary, and an n redrawn
    at that boundary would be the same defect review already found once here.
    """
    open_file = forge_paths(meta)["decisions"] / "open.yml"
    if not open_file.exists():
        return

    if not yaml_available():
        # Degrade LOUDLY, not silently: the gate cannot run, say so and allow.
        warn("pre-push gate skipped — no YAML library available "
             "(pip install ruamel.yaml); blocking decisions NOT verified.")
        return

    data = load_yaml(open_file)
    if not isinstance(data, dict):
        # `is None` was the whole guard, and `load_yaml` returns whatever the
        # document holds — so `open.yml` written as a LIST (or a scalar) got
        # past it and raised `AttributeError` at `data.get` OUTSIDE any handler:
        # traceback, exit 1, and the gate after it never called, so ONE
        # malformed model file disarmed INV-014 and INV-006 together. G-2
        # freezes this gate's BLOCKING behaviour (unchanged: a well-formed
        # document still refuses with the same message and the same code); its
        # second half asks for exactly this — loud degradation of every path on
        # which the check cannot be made.
        warn(f"pre-push gate skipped — could not read {open_file} as a "
             "mapping; blocking decisions NOT verified.")
        return

    entries = data.get("open")
    blocking = [
        d for d in (entries if isinstance(entries, list) else [])
        if isinstance(d, dict)
        and d.get("status") == "open"
        and _blocks_synthesis(d)
    ]
    if not blocking:
        return

    warn(f"push blocked — {len(blocking)} blocking architectural decision(s) unresolved:")
    for d in blocking:
        q = str(d.get("question") or "").replace("\n", " ").strip()
        print(f"  {d.get('id', '?')}: {q[:80]}", file=sys.stderr)
    print("\nResolve with /forge:architect-adr-writer, then push.", file=sys.stderr)
    sys.exit(EXIT_REFUSE)


def _conclude(root: Path, head: str) -> None:
    """Conclude about ONE commit being published: refuse, or return in silence.

    WHERE THE GUARD IS — k of n (ADR-0022/R2). The guarded operation is "this
    hook concludes something about a release". n is the THREE conclusions
    reachable from here: refuse-blocked, refuse-missing, and allow-verified
    (silent, or announced when the accepted verdict names an ancestor). **3 of
    3 are reached through a declared token or through silence with a named
    reason**, and no fourth path exists.

    That last clause used to be false in a way worth recording, because it is
    the shape ADR-0022/R1 is about: the claim said "every raise inside this
    function is caught below", while the `try` CLOSED at the `_read_verdicts`
    call and the whole conclusion region sat outside it. A malformed
    `scenarios:` therefore exited 1. The conclusion region is now inside the
    caller's `try`, which is what makes the sentence true rather than
    aspirational; `MalformedDocumentTest` and `OutputDisciplineTest::
    test_every_exit_code_is_one_of_the_two_declared` are its executable half.
    `sys.exit` raises `SystemExit`, a `BaseException`, and passes through the
    caller's handlers untouched — the refusals still refuse.
    """
    answering, misses = _read_verdicts(root, head)

    blocked = [d for d in answering if d.get("verdict") == "blocked"]
    if blocked:
        # Strictest answer wins: a passing document sitting beside a blocking
        # one does not overrule it, or adding a file would be the bypass.
        _refuse_blocked(blocked[0])
        sys.exit(EXIT_REFUSE)
    if not answering:
        _refuse_missing(head, misses)
        sys.exit(EXIT_REFUSE)
    trailing = [d for d in answering if d.get("commit") != head]
    if trailing and len(trailing) == len(answering):
        # The bootstrap case, said out loud: the accepted verdict names an
        # ancestor. A tolerance nobody can see is a tolerance nobody can audit.
        # The NEAREST answering ancestor is the one named — measured on the
        # repository, where several artefact-only commits in a row (a wave that
        # recorded a baseline, then recorded another) make every one of them a
        # valid answer, and naming the oldest reads as a verdict far staler
        # than the one actually relied on.
        _say(f"release eval verdict passed — recorded at "
             f"{str(_nearest(root, trailing, head).get('commit'))[:12]}, "
             f"which differs from what this push publishes ({head[:12]}) "
             f"only in {EVAL_REPORT_DIR_REL}/.")
        # THE DISAGREEMENT, SAID OUT LOUD RATHER THAN LEFT IN THE BACKLOG.
        # This tolerance is the gate's alone: `ci/eval_runner.py report` still
        # applies strict `commit == HEAD` and will exit 3 on this exact state.
        # The gate is the strictly MORE PERMISSIVE of the two, so the
        # divergence cannot turn a runner refusal into a publish the runner
        # would have blocked — but a developer who runs `report` after a green
        # push and gets exit 3 with no explanation is how a gate earns a
        # reputation for lying.
        # THE CLAUSE THAT NAMES ANOTHER COMPONENT IS TRUE OF IT.
        # `ci/eval_runner.py::report_artefacts` returns `<suite>-*.yml`, and its
        # docstring records forgiving the DIRECTORY as a measured fail-open it
        # REMOVED: `--out` then named the report location AND blinded the
        # working-tree check to whatever subtree it pointed at.
        _say("note: `python3 ci/eval_runner.py report --suite "
             f"{EVAL_SUITE}` still applies STRICT commit equality and will "
             f"exit 3 on this same state. What would converge them is the "
             f"runner applying to the COMMIT RANGE the per-suite artefact "
             f"exclusion its dirty check already applies to the WORKING "
             f"TREE ({EVAL_SUITE}-*.yml) — deliberately not the directory, "
             f"for the `--out` reason `report_artefacts` documents. The "
             f"two would still differ in width: this gate excludes the "
             f"whole directory {EVAL_REPORT_DIR_REL}/, which is a constant "
             f"here and not a caller-supplied path, so a SIBLING suite's "
             f"report does not end this tolerance. The gate's width is the "
             f"intended one. See the 2026-08-21 bootstrap entry in "
             f"meta/kanban/backlog.md (G-4 owns the runner, not this "
             f"gate).")


def _git_hook_gate(meta: Path, payload: str) -> None:
    """The release eval gate, in the position that knows what is published.

    Reads the stored verdict; never replays the suite (G-1). Returns when the
    push may proceed, exits :data:`EXIT_REFUSE` when it may not, and fails open
    LOUDLY on every path where the question could not be answered at all.

    EVERY LINE OF STDIN IS EVALUATED, not the first one that matches. `git push
    --all` and `--mirror` hand this hook one line per branch, and a push that
    updates the release ref alongside three card branches must be judged on the
    release line wherever it sits. The loop is also why :func:`_conclude` is a
    function: the refusal has to be reachable from inside it.

    THE LOOP IS EQUIVALENT TO ITS OWN FIRST ITERATION TODAY, and saying so is
    cheaper than letting the next reader re-derive it. The one mutant that
    survived this card's battery of 25 is exactly this — judging only
    ``publishing[0]`` — and it survived because git will not hand this hook two
    lines with the same remote ref: it rejects a push naming one destination
    twice before any hook runs (measured, and pinned by
    `PublicationResidueTest::test_git_never_states_the_release_ref_twice`, which
    reddens if that stops being true). The loop is kept because ADR-0023 words
    the rule as "any line whose remote ref is the release ref", and a shortcut
    would make that sentence depend on a measurement nobody can see from here.
    """
    try:
        if not _eval_gate_applies(meta):
            return  # no eval corpus: this gate is not in force here

        root = _repo_root(Path.cwd())
        if root is None:
            # fail-open 1
            _fail_open(meta, "failure",
                       "this eval-gated project is not inside a git work tree, "
                       "so no commit can be bound to a verdict.")
            return
        release = _release_branch(root)
        if release is None:
            # fail-open 2
            _fail_open(meta, "failure",
                       "could not resolve the release branch (neither "
                       "refs/remotes/origin/HEAD nor a local "
                       f"{'/'.join(RELEASE_BRANCH_FALLBACKS)}), so the refs "
                       "this push updates could not be judged against it.")
            return
        publishing, unreadable = _stated_publications(payload, release)
        if not publishing:
            if unreadable:
                # fail-open 7
                _fail_open(meta, "failure",
                           f"git's stdin carried {len(unreadable)} line(s) "
                           f"that are not the four fields its pre-push "
                           f"protocol writes, so what this push updates could "
                           f"not be read: {unreadable[0]!r}")
                return
            return  # nothing goes to the release ref: not this gate's business
        if not yaml_available():
            # fail-open 3
            _fail_open(meta, "failure",
                       "no YAML library available (`pip install ruamel.yaml`), "
                       "so the stored release eval verdict cannot be read — "
                       "the verdict is NOT verified.")
            return
        for local_ref, local_sha in publishing:
            code, head = _git(root, "rev-parse", "--verify", "--quiet",
                              f"{local_sha}^{{commit}}")
            if code != 0 or not head:
                # fail-open 4
                _fail_open(meta, "failure",
                           f"git states that {local_ref!r} ({local_sha[:12]}) "
                           f"is being published to {release!r}, and that sha "
                           f"resolves to no commit in this repository — so "
                           f"WHAT would be published is unknown, and no "
                           f"verdict can be checked against it.")
                return
            _conclude(root, head)
        if unreadable:
            # Reached only when every readable line was allowed: a refusal
            # exits from inside the loop above, so junk on stdin cannot be used
            # to turn a block into a fail-open.
            # fail-open 8
            _fail_open(meta, "failure",
                       f"git's stdin also carried {len(unreadable)} line(s) "
                       f"that are not the four fields its pre-push protocol "
                       f"writes, so part of what this push updates could not "
                       f"be read: {unreadable[0]!r}")
    except _GateUnavailable as exc:
        # fail-open 5
        _fail_open(meta, "timeout" if "budget" in str(exc) else "failure",
                   f"the release eval gate could not run: {exc}")
        return
    except Exception as exc:  # noqa: BLE001 — a crash must not REFUSE a push
        # fail-open 6
        _fail_open(meta, "failure",
                   f"the release eval gate itself failed: {exc!r}")
        return


# ── the selftest push gate (CARD-174, FR-112(b)) ──────────────────────────


def _refuse_selftest(token: str, detail: str) -> None:
    """Say why a push of the target branch is refused; the caller exits.

    Names no remedy, deliberately (ADR-0023: a refusal that names a cure gets
    the cure run). The cure for a red selftest is a green tree, not a command.
    """
    warn(f"{token} — PUSH REFUSED: a push of the target branch is published "
         f"only when the full selftest's own exit code is 0 (FR-112).")
    warn(detail)


def _selftest_in_work_tree() -> bool:
    """Is `ci/selftest.py` in the directory git runs this hook from?

    git runs `pre-push` from the work tree root, so this is the in-force
    question asked WITHOUT git — the only form of it left when git itself could
    not answer. It never raises: `Path.is_file` answers False on an error.
    """
    return (Path.cwd() / SELFTEST_SCRIPT).is_file()


def _carries_selftest(root: Path, sha: str) -> bool:
    """Does commit ``sha``'s tree carry `ci/selftest.py`? False for no commit."""
    if not sha or set(sha) == {"0"}:
        return False
    code, _ = _git(root, "cat-file", "-e", f"{sha}:{SELFTEST_SCRIPT}")
    return code == 0


def _replaced_commits(payload: str, release: str) -> list[str]:
    """The REMOTE sha of every line that updates the target ref — the commit
    the push replaces. Read only to keep the gate in force when a push deletes
    `ci/selftest.py`; which commits are PUBLISHED is :func:`_stated_publications`'
    answer, and this does not re-derive it."""
    release_ref = f"refs/heads/{release}"
    return [fields[3] for fields in (raw.split() for raw in
                                     payload.splitlines())
            if len(fields) >= 4 and fields[2] == release_ref]


def _selftest_budget() -> int:
    """The selftest's time budget in seconds; ValueError when it is not one."""
    raw = os.environ.get(SELFTEST_TIMEOUT_ENV) or ""
    if not raw.strip():
        return SELFTEST_TIMEOUT_DEFAULT
    budget = int(raw)
    if budget <= 0:
        raise ValueError(f"{SELFTEST_TIMEOUT_ENV}={raw!r} is not a positive "
                         f"number of seconds")
    return budget


def _restore_signals(previous: dict) -> None:
    """Put back the handlers :func:`_run_selftest` replaced."""
    for number, handler in previous.items():
        try:
            signal.signal(number, handler)
        except (ValueError, OSError, TypeError):
            pass


def _stderr_present() -> bool:
    """Is descriptor 2 a descriptor this process can write to?

    A zero-byte ``os.writev`` asks the kernel exactly that and moves no byte:
    EBADF when 2 is closed or is open for reading only (a stderr closed at
    start whose number was then reused). Asked of the descriptor itself, not
    of ``sys.stderr``, because the descriptor is what the selftest inherits.
    """
    try:
        os.writev(2, [b""])
        return True
    except OSError:
        return False


def _run_selftest(root: Path, budget: int) -> int:
    """Run `ci/selftest.py` as ITS OWN PROCESS and return ITS exit status.

    THE SHAPE IS THE CONTRACT (AC-642, ADR-0023/R1), pinned by
    `tests/test_push_gate_selftest.py::TestPushGate_SelftestExitCodeIsNotPiped`:
    an argv list (no shell, so no pipe can be spelled), and the selftest's own
    output handed straight to this hook's error descriptor — never captured,
    never read. The one thing taken back from the child is ``returncode``,
    which is the process's own exit status (negative when a signal killed it).
    It runs in its own process group so that on EVERY non-normal exit of the
    wait — over budget (TimeoutExpired), Ctrl-C (KeyboardInterrupt), SystemExit,
    SIGTERM/SIGHUP (handlers raise SystemExit), any other exception — the WHOLE
    group is killed (``os.killpg``) and reaped before the exception continues
    to the refusal: no worker that was STARTED outlives the refusal — bar the one
    bytecode between ``Popen`` returning and its result being bound. A normal
    return kills nothing. (SIGKILL of the hook itself cannot be caught.)
    SIGTERM/SIGHUP are taken over only when they are at their default; one
    inherited as ignored stays ignored.

    THE REPOSITORY PIN IS REMOVED FROM ITS ENVIRONMENT. git exports `GIT_DIR`
    to a hook run from a linked worktree (measured, git 2.50.1); handed on, it
    would aim every git call the selftest makes — its tests' throwaway
    repositories included — at THIS repository. The variables removed are the
    ones git itself lists as repository-local (`git rev-parse
    --local-env-vars`), not a list kept here. Driven by
    `TestPushGate_ApplicabilityEdges::test_a_linked_worktree_push_runs_the_selftest_unpinned`.

    Raises:
        _GateUnavailable: git would not list its repository-local variables.
        subprocess.TimeoutExpired: the selftest outlived ``budget``.
        OSError: the selftest could not be started.
    """
    code, names = _git(root, "rev-parse", "--local-env-vars")
    if code != 0:
        raise _GateUnavailable("git would not list its repository-local "
                               "environment variables")
    pinned = set(names.split())
    environment = {name: value for name, value in os.environ.items()
                   if name not in pinned}
    # Its OWN SESSION (hence its own process group): a selftest that outlives
    # the budget has workers of its own, and killing only the direct child
    # would leave them running against the checked-out tree. The price of the
    # separate session is that the terminal's SIGINT no longer reaches it, so
    # EVERY way out of the wait below tears the group down itself.
    def _terminated(signum: int, _frame: object) -> None:
        raise SystemExit(128 + signum)

    previous: dict[int, object] = {}
    child: subprocess.Popen | None = None
    # ONE try covers the handlers, the spawn and the wait, so that no interval
    # between them is outside the cleanup in the `finally`. The one instant
    # left is the single bytecode between `Popen` returning and its result
    # being bound to `child`; Python gives no way to close that one.
    try:
        # Named directly, not looked up by name: this hook runs on POSIX only,
        # where both exist, and a dynamic lookup is a route the printing freeze
        # (tests/test_publication_authority.py) refuses in this file. A signal
        # INHERITED as ignored (`nohup`) stays ignored: the push was started
        # to survive a hangup, and turning it into a refusal would undo that.
        for number in (signal.SIGTERM, signal.SIGHUP):
            try:
                if signal.getsignal(number) is signal.SIG_DFL:
                    previous[number] = signal.signal(number, _terminated)
            except (ValueError, OSError):  # not the main thread / not settable
                pass
        # Descriptor 2 is the selftest's report channel. Absent at hook start
        # (`2>&-`), the number may since have been reused for something that is
        # not a writable stderr, and a green selftest that prints would exit
        # non-zero into it: then the report is discarded instead.
        present = _stderr_present()
        child = subprocess.Popen([sys.executable, str(root / SELFTEST_SCRIPT)],
                                 cwd=str(root), env=environment,
                                 stdin=subprocess.DEVNULL, stdout=2 if present
                                 else subprocess.DEVNULL,
                                 stderr=2 if present
                                 else subprocess.DEVNULL,
                                 start_new_session=True)
        child.wait(timeout=budget)
    finally:
        # Normal return: the child has exited (poll() is not None) and nothing
        # is killed. Every other exit — TimeoutExpired, KeyboardInterrupt,
        # SystemExit (incl. SIGTERM/SIGHUP turned into one), any exception —
        # leaves a started child running, and its whole group is killed and
        # reaped BEFORE the exception continues to the refusal.
        if child is not None and child.poll() is None:
            try:
                os.killpg(child.pid, signal.SIGKILL)
            except OSError:  # the group is already gone
                pass
            child.wait()
        _restore_signals(previous)
    return child.returncode


def _selftest_verdict(payload: str) -> tuple[str, str] | None:
    """``None`` when the push may proceed, else ``(token, detail)``.

    WHEN THE GATE IS IN FORCE. For a stdin line whose REMOTE REF is the target
    branch — the same branch the eval gate calls the release branch
    (`hooks/eval_verdict.py::_release_branch`) — and which publishes a commit
    (a deletion publishes none), in a repository where `ci/selftest.py` is in
    the work tree, OR in the commit being published, OR in the commit the push
    replaces. The last clause is what keeps a commit that DELETES the selftest
    from taking the gate out of force with it. A repository with none of the
    three is not gated and is not told so: the eval gate's own precedent, and
    `TestPushGate_NotInForceWithoutASelftest` pins the silence.

    IN FORCE, EVERY PATH THAT DOES NOT END IN A 0 EXIT REFUSES:
    * the selftest ran and its own exit code was not 0 — red;
    * a stdin line could not be read as git's four fields;
    * the target branch would not resolve AND a stdin line could be it (an
      unreadable line, or a publication to `main`/`master`, the names the
      resolution falls back on); any other branch's push is not the target's;
    * the commit being published is not the checked-out HEAD — the selftest
      runs on the checked-out tree and cannot vouch for another commit;
    * `ci/selftest.py` is not in the work tree;
    * the budget is malformed, the selftest outlived it, or it could not start.
    """
    root = _repo_root(Path.cwd())
    if root is None:
        if _selftest_in_work_tree():
            return (SELFTEST_GATE_UNRUNNABLE,
                    "git could not name the work tree this push runs in, so "
                    "the selftest could not be run on what is published.")
        return None
    in_tree = (root / SELFTEST_SCRIPT).is_file()
    release = _release_branch(root)
    if release is None:
        if in_tree:
            # The target is unknown, so only a line that COULD be it refuses:
            # an unreadable line (it may name anything), or a line publishing
            # to one of the conventional names the resolution falls back on.
            # A push of any other branch cannot be the target and proceeds.
            unknown_unreadable: list[str] = []
            candidates: list[tuple[str, str]] = []
            for name in RELEASE_BRANCH_FALLBACKS:
                found, unreadable = _stated_publications(payload, name)
                candidates += found
                unknown_unreadable = unreadable
            if candidates or unknown_unreadable:
                return (SELFTEST_GATE_UNRUNNABLE,
                        "the target branch would not resolve (neither "
                        "refs/remotes/origin/HEAD nor a local "
                        f"{'/'.join(RELEASE_BRANCH_FALLBACKS)}), so whether "
                        "this push publishes it cannot be told for a line "
                        "that could be the target.")
            if payload.strip():
                warn(f"selftest gate: NOT applied — the target branch would "
                     f"not resolve (no refs/remotes/origin/HEAD, no local "
                     f"{'/'.join(RELEASE_BRANCH_FALLBACKS)}) and no pushed "
                     f"line names a conventional target, so this push is "
                     f"let through ungated.")
        return None
    publishing, unreadable = _stated_publications(payload, release)
    in_force = in_tree or any(
        _carries_selftest(root, sha)
        for sha in [*(local for _ref, local in publishing),
                    *_replaced_commits(payload, release)])
    if not in_force:
        return None
    if unreadable:
        return (SELFTEST_GATE_UNRUNNABLE,
                f"git's stdin carried {len(unreadable)} line(s) that are not "
                f"the four fields its pre-push protocol writes, so whether "
                f"this push publishes {release!r} cannot be read: "
                f"{unreadable[0]!r}")
    if not publishing:
        return None
    code, head = _git(root, "rev-parse", "--verify", "--quiet",
                      "HEAD^{commit}")
    for local_ref, local_sha in publishing:
        if code != 0 or local_sha != head:
            return (SELFTEST_GATE_UNRUNNABLE,
                    f"{local_ref!r} ({local_sha[:12]}) is being published to "
                    f"{release!r} and is not the checked-out HEAD — the "
                    f"selftest runs on the checked-out tree and cannot vouch "
                    f"for another commit.")
    if not in_tree:
        return (SELFTEST_GATE_UNRUNNABLE,
                f"{SELFTEST_SCRIPT} is not in the work tree, while the commit "
                f"published to or replaced on {release!r} carries it — "
                f"there is no selftest to run, so there is no green.")
    try:
        budget = _selftest_budget()
    except ValueError as exc:
        return (SELFTEST_GATE_UNRUNNABLE,
                f"the selftest's time budget is not a number of seconds "
                f"({exc}).")
    warn(f"selftest gate: running the full selftest ({SELFTEST_SCRIPT}) on "
         f"the checked-out tree before {head[:12]} is published to "
         f"{release} — the push waits for its exit code and reads nothing "
         f"else.")
    status = _run_selftest(root, budget)
    if status != 0:
        return (SELFTEST_GATE_RED,
                f"{SELFTEST_SCRIPT} exited {status} on {head[:12]} — its own "
                f"exit code decides, whatever its output said.")
    return None


def _leave_refusing() -> None:
    """Exit :data:`EXIT_REFUSE` whatever state the output streams are in.

    ``sys.exit`` is not enough: with stderr a pipe whose reader is gone, the
    interpreter's finalization fails to flush it and REPLACES the status with
    120 — which the wrapper reads as a crash. ``os._exit`` runs no
    finalization to fail. Nothing is lost by skipping the flush: the refusal's
    text goes to stderr, which is line-buffered, and this hook writes nothing
    to stdout.
    """
    os._exit(EXIT_REFUSE)


def _selftest_gate(payload: str, stream_unreadable: bool = False) -> None:
    """Refuse a push of the target branch unless the full selftest exits 0.

    ONE ADDED BRANCH in the git-hook position (CARD-174, FR-112(b)), asked
    AFTER the release eval gate (G-1: that gate and its verdicts are
    unchanged, and a push it refuses never pays for a selftest run). Returns
    when the push may proceed; exits :data:`EXIT_REFUSE` when it may not.

    ⚑ THE POLARITY IS THE OPPOSITE OF THE EVAL GATE'S, ON PURPOSE. The eval
    gate fails OPEN on every path where it could not run (NFR-006). This gate,
    once in force, has NO fail-open: "the selftest could not run" is not "the
    selftest exited 0", and only the latter publishes (AC-641, EC-155). So every
    handler below REFUSES — except where the failure came before the in-force
    question could be answered at all and the work tree carries no
    `ci/selftest.py`: there nothing says the gate is in force.

    FAIL-OPEN AND REFUSE, PER CHECK (ADR-0120/R2), for THIS check:
    * could not run → REFUSE (in force) — every path in
      :func:`_selftest_verdict`'s list, plus the three handlers here;
    * ran and found something → REFUSE (`selftest-gate-red`);
    * fail-open → none inside this file. The wrapper's own routes
      (`hooks/git-pre-push.sh`, w1–w4) are where THIS PROCESS did not run or
      did not conclude, and there the selftest did not run either; they are
      NFR-006's, unchanged by this card.

    THE REFUSAL LEAVES AS EXIT_REFUSE WHEN ITS MESSAGE CANNOT BE WRITTEN
    (:func:`_leave_refusing`, in a ``finally``): a stderr that is a pipe with
    no reader, or absent, must still refuse, because any OTHER code is read by
    the wrapper as a crash and lets the push through. Driven by
    `TestPushGate_ApplicabilityEdges::test_a_refusal_with_dead_output_pipes_still_exits_refuse`.
    """
    try:
        if stream_unreadable:
            # The twin of the unreadable-LINE guard: no payload, so no way to
            # tell whether main is published. In force -> refuse, never allow.
            refusal = ((SELFTEST_GATE_UNRUNNABLE,
                        "git's stdin could not be read, so whether this push "
                        "publishes the target branch cannot be told.")
                       if _selftest_in_work_tree() else None)
        else:
            refusal = _selftest_verdict(payload)
    except subprocess.TimeoutExpired as exc:
        refusal = (SELFTEST_GATE_UNRUNNABLE,
                   f"{SELFTEST_SCRIPT} did not finish within its {exc.timeout}s "
                   f"budget ({SELFTEST_TIMEOUT_ENV}) — no exit code, so no "
                   f"green.")
    except _GateUnavailable as exc:
        refusal = (None if not _selftest_in_work_tree() else
                   (SELFTEST_GATE_UNRUNNABLE,
                    f"git could not answer, so the selftest could not be run "
                    f"on what is published: {exc}"))
    except BaseException as exc:  # noqa: BLE001 — in force, a crash refuses
        refusal = (None if not _selftest_in_work_tree() else
                   (SELFTEST_GATE_UNRUNNABLE,
                    f"the selftest gate itself failed: {exc!r}"))
    if refusal is None:
        return
    try:
        _refuse_selftest(*refusal)
    finally:
        _leave_refusing()


def _eval_gate_view(raw: bytes) -> str:
    """The stdin text the release eval gate has always been handed.

    What ``sys.stdin.read()`` returned: the bytes decoded with the stream's
    OWN encoding and errors mode (locale-dependent: surrogateescape under a
    C-family locale, strict under en_US.UTF-8), with no newline translation
    (the raw bytes are decoded as they came), and "" when that decode raises (NFR-006 fail-open). Re-deriving it from the
    stream's attributes, rather than hardcoding UTF-8, is what keeps G-1: a
    hardcoded strict decode let a blocked verdict through under LC_ALL=C.
    """
    try:
        return raw.decode(sys.stdin.encoding or "utf-8",
                          sys.stdin.errors or "strict")
    except (UnicodeDecodeError, LookupError):
        return ""


def _git_hook_main() -> None:
    """Entry point for git's own `pre-push` hook (``--git-hook``).

    Reads stdin verbatim (as bytes) — the payload is git's statement of what it is about
    to do, and the ONE thing this hook must not do to it is interpret it before
    :func:`_stated_publications`.

    Polarity is explicit on every route, because it is INVERTED from the other
    entry point (ADR-0023, Consequences → Negative): here, exiting non-zero
    refuses the push, so anything that is not a deliberate refusal must exit 0.

    Two gates, in this order: the release eval gate (forge projects only — it
    reads `meta/`), then the selftest gate (:func:`_selftest_gate`), which
    needs no `meta/` because its in-force question is about `ci/selftest.py`.
    """
    meta = find_meta()
    # Read BYTES: a git ref name may be any bytes, and a text read under a
    # UTF-8 locale raises on one that is not. Two views of the same stream,
    # the ONLY two consumers of it:
    # * eval gate (`_git_hook_gate`) — EXACTLY what `sys.stdin.read()` returned
    #   before CARD-174: the stream's own encoding and errors mode
    #   (:func:`_eval_gate_view`), "" on UnicodeDecodeError (NFR-006); G-1.
    # * selftest gate — every byte, decoded UTF-8 + surrogateescape, so an
    #   undecodable ref stays VISIBLE; a stream that could not be read at all
    #   is carried as `stream_unreadable`.
    raw: bytes | None
    try:
        raw = sys.stdin.buffer.read()
    except Exception:  # noqa: BLE001 — an unreadable stdin measured nothing
        raw = None
    if not isinstance(raw, bytes):
        raw, stream_unreadable = b"", True
    else:
        stream_unreadable = False
    selftest_payload = raw.decode("utf-8", errors="surrogateescape")
    payload = "" if stream_unreadable else _eval_gate_view(raw)
    if meta is not None:
        _git_hook_gate(meta, payload)
    _selftest_gate(selftest_payload, stream_unreadable)
    sys.exit(EXIT_ALLOW)


def _pretooluse_main() -> None:
    """Entry point for Claude Code's PreToolUse hook on Bash.

    Every read below is TYPE-GUARDED rather than annotated. The payload comes
    from another process, and the annotations here describe what Claude Code
    sends rather than what this hook is handed: a list, a scalar, a `null`, a
    non-mapping `tool_input` and a non-string `command` all reached `.get` /
    `command_matches` and exited 1 — a code this hook's own docstring declares
    it does not produce, and which PreToolUse reads as NON-BLOCKING.

    THE TWO GATES HAVE DIFFERENT TRIGGERS NOW, and the order is deliberate. The
    decision gate is a `git push` gate and G-2 freezes it, so it is asked
    exactly the question it was always asked, first. The disarm detector is NOT
    a `git push` gate: `git config core.hooksPath …` disarms the authority and
    contains no `push` at all, so it is asked of every Bash command.
    """
    try:
        data = json.load(sys.stdin)
    except Exception:
        sys.exit(EXIT_ALLOW)
    if not isinstance(data, dict):
        sys.exit(EXIT_ALLOW)

    tool_input = data.get("tool_input")
    command = tool_input.get("command") if isinstance(tool_input, dict) else ""
    if not isinstance(command, str):
        sys.exit(EXIT_ALLOW)

    findings = _disarm(command)
    pushing = command_matches(command, "git", "push")
    if not findings and not pushing:
        # THE COST OF WIDENING THE TRIGGER, PAID HERE RATHER THAN ON EVERY
        # COMMAND. The decision gate is a `git push` gate; the disarm detector
        # is not (`git config core.hooksPath …` contains no `push` at all), so
        # this hook is now consulted for EVERY Bash tool call. Both questions
        # above are pure text — one `shlex`-free regex sweep and one word-
        # boundary match — and the directory walk `find_meta` does is reached
        # only when one of them has something to say. A gate that costs a
        # filesystem walk per command is a gate people uninstall.
        sys.exit(EXIT_ALLOW)

    meta = find_meta()
    if meta is None:
        # Every forge hook is a no-op outside a forge project (Step 4b), and
        # that includes this refusal: a disarm detector that fired in a
        # repository forge does not manage would be refusing a command about
        # nothing.
        sys.exit(EXIT_ALLOW)

    if pushing:
        _decision_gate(meta)

    if findings:
        _refuse_disarm(findings)
        sys.exit(EXIT_REFUSE)
    sys.exit(EXIT_ALLOW)


def main() -> None:
    """Dispatch on the position this process was started in.

    A flag rather than two files, following `hooks/post-pull.py --git-hook`,
    which is the precedent this repository already installs a shell wrapper
    for (G-5). Two files would mean two install inventory rows, two drift
    surfaces, and two places for the release token vocabulary to drift apart.
    """
    if "--git-hook" in sys.argv[1:]:
        _git_hook_main()
    else:
        _pretooluse_main()


if __name__ == "__main__":
    main()
