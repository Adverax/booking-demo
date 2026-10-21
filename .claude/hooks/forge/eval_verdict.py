# forge
#!/usr/bin/env python3
"""
forge release eval VERDICT reader — the machinery, without a position.

WHAT THIS IS. Everything needed to answer ONE question: **does a recorded
verdict of the baseline eval suite answer for the commit that is about to be
published, and what does it say?** (NFR-004, INV-006, ADR-0020, COMP-058.) It
only ever READS a stored verdict; it never replays the suite (G-1), so a push
never waits on fixture time.

WHAT THIS IS NOT, and that line is the reason the file exists at all. Nothing
here knows what a `git push` publishes. Every entry point is handed a COMMIT
and answers about that commit. "Which commit does this push publish?" is a
different question with a different authority, and ADR-0023 settled where that
authority lives: git's own `pre-push` hook, which git hands
``<local ref> <local sha> <remote ref> <remote sha>`` on stdin, with no text to
parse. That half is `hooks/pre-push.py::_git_hook_gate`, and the only thing it
hands this module is a COMMIT — which is the whole seam.

WHY A SHARED MODULE RATHER THAN A HALF OF `hooks/pre-push.py`. ADR-0023's
decision leaves gates in TWO positions — the git `pre-push` hook (authority on
what is published) and the PreToolUse interception (detector of gate
disarmament) — and DEC-018 named the cost of two positions as the DIVERGENCE
between them: two places asking one question drift apart, and the drift is
invisible until a release goes out on the answer of the stale one. So the
answer lives once, here, and each position supplies only its own question.

That is why :func:`_eval_gate_applies` had to come here even though only one
position reads a verdict: "does this project HAVE an eval corpus?" is a
repository fact, and the two positions must not be able to disagree about it.

WHERE THE SEAM WAS DRAWN, COUNTED FROM THE CODE. Before the split
`hooks/pre-push.py` held 22 top-level definitions — 21 functions and the
:class:`_GateUnavailable` exception. FOURTEEN came here with CARD-014: the
verdict question and the git plumbing it cannot answer without. THREE stayed
because they answered the POSITION question and nothing else —
`_push_arguments`, `_publication`, `_refuse_undetermined` — and CARD-018
DELETED all three rather than moving them, because the position they served
does not exist any more. A fourth was on that "stayed" list wrongly and is now
here: :func:`_eval_gate_applies` (see its own docstring for who corrected
whom). What is left in the hook is its own body and its second, unrelated
gate — the two entry points and the dispatcher between them, the authority's
stdin reader, the disarm detector, `_decision_gate`, and `_blocks_synthesis`.
That last one was on the extraction inventory and the code disagreed: its only
caller is `_decision_gate` (the FR-009 / INV-014 gate, which knows nothing
about an eval verdict), so moving it would have made this module's own name a
false statement about its contents. The split is asserted, in both directions,
by `ExtractedModuleTest::test_the_verdict_machinery_is_defined_only_in_the_module`
and `::test_the_classifier_is_gone_from_both_files`.

────────────────────────────────────────────────────────────────────────────
⚑ WHAT CONSULTS THIS MODULE, AND FROM WHERE
────────────────────────────────────────────────────────────────────────────

ONE caller asks the verdict question: `hooks/pre-push.py` **running as git's
own `pre-push` hook** (`--git-hook`, reached through
`hooks/git-pre-push.sh`). It is handed ``<local ref> <local sha> <remote ref>
<remote sha>`` on stdin by git itself and evaluates the verdict against the
LOCAL SHA of any line whose REMOTE REF is the release ref. Nothing is parsed,
nothing is enumerated, and there is no inverted default — a default is only
needed where the question has no answer.

The SAME file's PreToolUse position no longer asks this module anything. Its
question is now "does this command stop the authoritative gate from running?"
(`--no-verify`, `git send-pack`, a `core.hooksPath` override) — a question
bounded by git's own documentation, which is why it can be answered from
command text when "what does this publish?" could not be. Three review cycles'
worth of holes in the old text classifier
(``git push a && git push main``; ``bash -c``; a git alias; `push.default =
matching|upstream`) are not narrowed here, they are GONE: the code that could
have them was deleted.

WHAT THE MOVE COST, stated because it is not nothing. In the git position the
hook cannot see `--dry-run`: git hands a rehearsal the byte-identical stdin of
a real push, and no environment variable distinguishes them. The text position
read that flag correctly and no longer exists. The residue is declared and
EXECUTED — `PublicationResidueTest::test_a_dry_run_is_refused_exactly_like_a_real_push`
drives a real `git push --dry-run` against a real bare origin and asserts both
halves (the hook refuses; origin does not move), so the day git grows a signal
that test goes red instead of a paragraph going stale.

────────────────────────────────────────────────────────────────────────────
ONE CARRIED WART, NAMED RATHER THAN TIDIED
────────────────────────────────────────────────────────────────────────────

**The names still start with an underscore.** A module read from two entry
points would ordinarily present a public surface. These identifiers are
load-bearing in an executable check — `FailOpenEnumerationTest` in
`tests/test_prepush_eval_gate.py` counts `_fail_open` CALL SITES by name and
compares them against `hooks/pre-push.py`'s docstring enumeration — and in a
dozen ``:func:`` cross-references that a rename would silently stale. CARD-018
was named as "the change that can rename them in one step" and did not: it
rewrote both entry points of the hook, and a rename on top of that would have
put a mechanical diff through the same review as the load-bearing one. Filed
rather than done, and the cost is one convention, not a behaviour.

WHERE HUMAN TEXT GOES. Every line this module writes goes through :func:`_say`,
which resolves the stream through `_forge_common._err_stream`: a descriptor
closed at process start (``2>&-``) makes `sys.stderr` literally ``None`` and
`print(file=None)` writes to STDOUT (ADR-0007/R2). This file therefore holds
ZERO raw `sys.stderr` writes, which is what keeps `_err_stream`'s k-of-n
declaration in the G-3-frozen `hooks/_forge_common.py` true across the split:
its ``n`` is the HOOK modules that emit human text, and this file is a shared
module, not a hook — the same standing `_forge_common.py` itself has there.
Driven by `ExtractedModuleTest::test_the_module_writes_through_the_err_stream_helper`
and by `ClosedDescriptorTest::test_the_err_stream_guard_declares_k_of_n_not_a_property`.

Imported by `hooks/pre-push.py`; not a hook itself and not installed as one
(it has no `main`, and nothing in `.claude/settings.json` names it).
"""
from __future__ import annotations

import os
import subprocess
import tempfile
from pathlib import Path

from _forge_common import _err_stream, load_yaml, record_gate_event

#: Stable identifier of the eval gate in meta/gate-events.yml (read by
#: /forge:status, NFR-010). The decision gate above it has never journalled and
#: still does not — G-2 keeps that path byte-for-byte.
EVAL_GATE_ID = "pre-push-eval"

#: The suite whose verdict governs a release. ADR-0008's threshold
#: (`zero_critical_regressions`) is applied by the RUNNER and recorded in the
#: document; this hook reads the recorded verdict and never re-derives it.
EVAL_SUITE = "baseline"

#: Where the runner stores verdicts, and where its fixtures live — relative to
#: the project root. These MIRROR `ci/eval_runner.py`'s `REPORT_DIR_REL` and
#: `FIXTURES_REL`, which G-4 freezes as the runner's own. Mirrored rather than
#: imported: importing a 1710-line CLI module into a hook to read two path
#: constants buys a slower push and an import path to keep alive. The cost of
#: the mirror is real and stated — a relocation on the runner's side would make
#: this gate read an empty directory and refuse every release push, loudly and
#: wrongly, until the constant follows. `tests/test_prepush_eval_gate.py`
#: pins both against the runner's own values so the drift fails a test instead
#: of a release.
EVAL_REPORT_DIR_REL = "meta/eval/release"
EVAL_FIXTURES_REL = "ci/fixtures/eval"

#: Report schema this gate can read (mirrors the runner's `REPORT_VERSION`).
#: A document written in another schema is a near miss, not a verdict: this
#: hook cannot know what a field it has never seen means.
EVAL_REPORT_VERSION = 2

#: The only two verdicts a STORED document may carry (mirrors the runner's
#: `STORED_VERDICTS`). Anything else is a document that cannot be read as a
#: verdict — and `not_yet_run` is deliberately among the things it is not: the
#: runner never writes that to disk, it only prints it.
STORED_VERDICTS = ("passed", "blocked")

#: The closed set of tokens the eval gate leads its output with. A consumer
#: tells the refusals apart by these, NEVER by the exit code — and in the git
#: `pre-push` position that is not a preference, it is the only thing left:
#: git collapses every non-zero hook exit into its own 1 (ADR-0023,
#: Consequences → Negative; measured on 1, 2, 3 and 42, and driven by
#: `TestPublicationAuthority_EveryMeasuredShapeIsAnswered::
#: test_git_collapses_every_non_zero_hook_code_into_one`).
EVAL_VERDICT_TOKENS = ("eval-gate-failed", "eval-gate-missing",
                       "eval-gate-unverified")

#: AC-020's literal marking, and AC-021's silence: a release whose recorded
#: verdict is `blocked` is refused AND marked `eval-gate-failed`.
#:
#: There WAS a fourth, ``eval-gate-undetermined``, and its departure is the
#: shape of CARD-018 in one line. It meant "I could not read what this command
#: publishes" — a fact only a gate that PARSES A COMMAND can have. The
#: authority now reads git's own stdin, where what is published is stated
#: rather than reconstructed, so no route can produce that fact and a token
#: nothing emits is a vocabulary entry a consumer would wait for forever. Its
#: producer was a function called `_refuse_undetermined`, and it is named here
#: WITHOUT a locator on purpose (ADR-0022/R3): it no longer exists in either
#: file, so a pointer to it would be a pointer that cannot resolve. Both it and
#: the classifier that fed it left in one change, exactly as this module's
#: docstring said they would, and
#: `ExtractedModuleTest::test_the_classifier_is_gone_from_both_files` is what
#: keeps them gone.
#:
#: ⚑ AND ONE OF THE THREE IS MANY-TO-ONE. `eval-gate-unverified` leads EVERY
#: fail-open route there is: every `_fail_open` call site in
#: `hooks/pre-push.py` and every `forge_allow_loudly` in
#: `hooks/git-pre-push.sh`. The count is deliberately not written here — three
#: passes over that set once produced three different numbers — and each HALF
#: is derived by its OWN test, named separately because they are separate
#: derivations and one sentence covering both was false for a whole card:
#:
#: * the python half, through the docstring-enumeration ↔ `# fail-open <n>` tag
#:   correspondence in `hooks/pre-push.py` —
#:   `tests/test_prepush_eval_gate.py::FailOpenEnumerationTest::
#:   test_the_enumeration_has_one_entry_per_call_site` and
#:   `::FailOpenEnumerationTest::test_every_entry_is_bound_to_exactly_one_call_site`;
#: * the wrapper half, through the same correspondence in
#:   `hooks/git-pre-push.sh` (`# fail-open w<n>`) —
#:   `::WrapperFailOpenEnumerationTest::test_the_enumeration_has_one_entry_per_wrapper_route`
#:   and `::WrapperFailOpenEnumerationTest::test_every_wrapper_route_carries_a_tag_bound_to_one_entry`.
#:
#: ⚑ THAT ATTRIBUTION IS A CORRECTION, AND THE THING IT CORRECTS IS THIS
#: PARAGRAPH. It named ONE test —
#: `::FailOpenEnumerationTest::test_every_unverified_route_leads_with_the_same_token`
#: — for both halves, and that test derives NEITHER count: it asserts
#: `len(...) > 1` on each side plus exactly one emitter per side, which is a
#: real property (nothing can leave the vocabulary by writing its own line) and
#: keeps its own name below. Measured: a FIFTH `forge_allow_loudly` added to
#: the wrapper moved nothing anywhere, because the wrapper's count was derived
#: by no test at all. It is now derived by the tests named above, which redden
#: on exactly that mutation.
#:
#: ⚑ AND THE NAMES IN THIS PARAGRAPH ARE THEMSELVES RESOLVED, which they were
#: not until fix cycle 4. What was asserted was that certain SUBSTRINGS appear
#: here; measured against that, renaming
#: `::WrapperFailOpenEnumerationTest::test_the_enumeration_has_one_entry_per_wrapper_route`
#: to a name nothing defines — in the test file AND in this paragraph — left the
#: whole suite green and `ci/locator_check.py` with zero findings, because that
#: checker resolves `path:line` locators and a `::name` is not one. Every
#: `::Name` written here is now looked up in both suites by
#: `::FailOpenEnumerationTest::test_the_attribution_names_tests_that_exist`,
#: which is the treatment `hooks/git-pre-push.sh`'s header enumeration has had
#: since fix cycle 3. An attribution nobody can resolve is prose with colons in
#: it.
#:
#: This does NOT violate ADR-0007/R1: the token's MEANING is single and exact —
#: "the gate was not consulted, so nothing about this release was measured" —
#: and every route emitting it means precisely that. What a consumer CANNOT do
#: is tell those routes apart: "no python3 on PATH", "git did not answer", "the
#: release branch would not resolve" and "the gate crashed" all arrive under
#: one word, discriminated only by the FREE-TEXT detail on the next line, which
#: is prose and not a contract. The machine consumer that exists today
#: (/forge:status, through `meta/gate-events.yml`) reads the JOURNAL instead,
#: where `outcome` already separates `timeout` from `failure`; the wrapper's
#: routes reach no journal at all, because they happen before Python starts.
#:
#: DECLARED, NOT BUILT, and the trigger is exactly one thing: the first
#: consumer that must BRANCH on WHICH fail-open happened. New tokens are not
#: the answer to that either — the discriminator belongs in the journal's typed
#: fields, beside the `outcome` that is already there, before it belongs in a
#: fourth word on stderr. Inventing tokens no consumer reads is the mistake
#: `eval-gate-undetermined` above already made once.
(TOKEN_FAILED, TOKEN_MISSING, TOKEN_UNVERIFIED) = EVAL_VERDICT_TOKENS

#: PER-CALL ceiling on every git invocation this gate makes (all read-only and
#: local). Stated as per-call because that is what it is: a WEDGED git costs the
#: push one ceiling — the first call raises and the whole gate fails open loudly
#: (measured, 10.06 s) — but a merely SLOW git is called once per stored
#: document (`cat-file`) plus once per trailing document in :func:`_nearest`, so
#: a report directory with N documents can hold a push for roughly N times the
#: per-call cost without any single call reaching this ceiling. A deadline for
#: the gate AS A WHOLE is the thing this constant is NOT; that is filed in
#: `meta/kanban/backlog.md`, not silently implied here.
#:
#: The environment override exists for ONE reason and it is stated so it is not
#: mistaken for a knob: the only test that can pin the `"budget" in str(exc)`
#: timeout classification has to WAIT OUT this ceiling, and at 10 s that single
#: case was ~9% of the whole suite's wall clock. The test sets it to 1 and
#: asserts the shipped default in the same case, so a push always runs at 10.
GIT_TIMEOUT = int(os.environ.get("FORGE_GIT_TIMEOUT") or "10")

#: Branch names tried, in order, when the remote's HEAD does not name one.
RELEASE_BRANCH_FALLBACKS = ("main", "master")


class _GateUnavailable(Exception):
    """The gate could not RUN. Never a verdict — always a loud fail-open."""


# ── git plumbing ──────────────────────────────────────────────────────────

def _git(root: Path, *args: str) -> tuple[int, str]:
    """Run a read-only git command; return ``(returncode, stdout)``.

    A non-zero return code is an ANSWER (the ref does not exist, the commit is
    not an ancestor) and is handed back. Not being able to run git at all is
    not an answer, and leaves as :class:`_GateUnavailable`.

    Raises:
        _GateUnavailable: git is missing, timed out, or could not be spawned.
    """
    try:
        done = subprocess.run(
            ["git", *args], capture_output=True, timeout=GIT_TIMEOUT,
            cwd=str(root), encoding="utf-8", errors="replace")
    except subprocess.TimeoutExpired:
        raise _GateUnavailable(
            f"git {' '.join(args)} exceeded its {GIT_TIMEOUT}s budget")
    except FileNotFoundError:
        raise _GateUnavailable("git executable not found on PATH")
    except OSError as exc:  # noqa: BLE001 — spawn failure, not an answer
        raise _GateUnavailable(f"git could not be run: {exc!r}")
    return done.returncode, (done.stdout or "").strip()


def _git_paths(root: Path, *args: str) -> list[str]:
    """Run a ``-z`` git command and split its NUL-separated paths.

    ``-z`` rather than plain ``--name-only``: git QUOTES a path that is not
    plain ASCII, and a quoted path compared against a prefix silently stops
    matching — which here would turn "only the report artefact changed" into
    "something else changed" and refuse a legitimate release.
    """
    try:
        done = subprocess.run(
            ["git", *args], capture_output=True, timeout=GIT_TIMEOUT,
            cwd=str(root))
    except subprocess.TimeoutExpired:
        raise _GateUnavailable(
            f"git {' '.join(args)} exceeded its {GIT_TIMEOUT}s budget")
    except FileNotFoundError:
        raise _GateUnavailable("git executable not found on PATH")
    except OSError as exc:  # noqa: BLE001
        raise _GateUnavailable(f"git could not be run: {exc!r}")
    if done.returncode != 0:
        raise _GateUnavailable(
            "git {0} exited {1}: {2}".format(
                " ".join(args), done.returncode,
                done.stderr.decode("utf-8", "replace").strip()[:200]))
    return [chunk.decode("utf-8", "surrogateescape")
            for chunk in done.stdout.split(b"\x00") if chunk]


def _repo_root(start: Path) -> Path | None:
    """The git work tree the push would run in, or None when there is none."""
    code, out = _git(start, "rev-parse", "--show-toplevel")
    if code != 0 or not out:
        return None
    return Path(out)


def _release_branch(root: Path) -> str | None:
    """The branch a release is published on.

    The remote's own HEAD first (``refs/remotes/origin/HEAD``), because that is
    the repository's statement about its trunk rather than this hook's guess;
    then the conventional local names. None when neither answers — which is not
    a licence to guess: the caller fails open LOUDLY, because a gate that
    silently decides "this is not a release" has disarmed itself.
    """
    code, out = _git(root, "symbolic-ref", "--short", "-q",
                     "refs/remotes/origin/HEAD")
    if code == 0 and out:
        return out.split("/", 1)[1] if "/" in out else out
    for name in RELEASE_BRANCH_FALLBACKS:
        code, _ = _git(root, "rev-parse", "--verify", "--quiet",
                       f"refs/heads/{name}")
        if code == 0:
            return name
    return None


def _eval_gate_applies(meta: Path) -> bool:
    """Does this project have a release eval contour at all?

    "Absence of a verdict = blocked" (INV-006) is a rule about a project that
    HAS key scenarios to regress. Read as a rule about every repository, it
    blocks every push in every consumer project that never declared an eval
    corpus, and in every card worktree — an outage, not a gate. So the gate is
    in force exactly when the project ships the suite the verdict is about.

    The suite directory, not the report directory, is the signal, and the
    difference is the point: deleting `meta/eval/release/` would otherwise
    disarm the gate by removing the evidence it demands, while deleting
    `ci/fixtures/eval/<suite>/` deletes the eval corpus itself — a visible act
    that `ci/selftest.py`'s own eval-fixtures check fails on independently.

    Checked BEFORE any subprocess, deliberately: a project without an eval
    contour must cost its caller nothing and must say nothing (a gate that
    prints on every unrelated push is a gate people uninstall).

    ⚑ WHY THIS IS HERE AND NOT IN A HOOK, AND WHO CORRECTED WHOM. CARD-014
    kept it in `hooks/pre-push.py` as one of "the FOUR that answer the POSITION
    question and nothing else". Its review measured that claim against the body
    and it does not hold: there is no command here, no refspec and no push —
    only a REPOSITORY FACT, and the two ADR-0023 positions need the identical
    answer to it. Left in the PreToolUse hook it would have had to be either
    duplicated in the git hook or omitted from it, and omitted means the git
    hook refuses every push in every project that never declared an eval
    corpus. Duplication is precisely the divergence this module exists to
    prevent (DEC-018), so the position-dependent set is THREE, not four, and
    all three of those were deleted by CARD-018 rather than moved.
    """
    return (meta.parent / EVAL_FIXTURES_REL / EVAL_SUITE).is_dir()


# ── reading the stored verdict (the hook only ever READS — G-1) ───────────


def _is_report_artefact(path: str) -> bool:
    """True for a path that is an OUTPUT of the release eval circuit.

    The exclusion `ci/eval_runner.py`'s dirty-check applies to the working tree
    (`<out_dir>/<suite>-*.yml`), applied here to a COMMIT RANGE — but widened
    from THIS SUITE's prefix to the report DIRECTORY, which is the unit that
    actually matches reality. The directory is per-repository and a repository
    may ship several suites (`ci/fixtures/eval/regression/` exists here), whose
    reports land beside the baseline's. Measured before the widening:
    committing `meta/eval/release/regression-<sha>.yml` blocked the next
    release push, blaming "1 path(s) THE EVAL READS changed" for a path the
    eval does not read — it fails closed, but it re-creates in miniature the
    unsatisfiable gate this whole card exists to remove.

    The `.yml` half is NOT decoration and is pinned separately
    (`VerdictFreshnessTest::test_a_non_yml_file_in_the_report_directory_ends_the_tolerance`):
    a non-YAML file dropped in this directory is not an output of the circuit,
    so it ends the tolerance like any other change.
    """
    return (path.startswith(f"{EVAL_REPORT_DIR_REL}/")
            and path.endswith(".yml"))


def _answers_for_head(root: Path, commit: str, head: str) -> tuple[bool, str]:
    """Does a verdict recorded at ``commit`` answer for ``head``?

    ─────────────────────────────────────────────────────────────────────────
    THE DEFECT THIS RULE EXISTS FOR, AND WHAT THE CHOICE COSTS
    ─────────────────────────────────────────────────────────────────────────

    The obvious rule — ``commit == head`` — is the one `ci/eval_runner.py
    report` applies, and it has NO FIXED POINT. The documented bootstrap is:
    clean tree → run the suite → commit `meta/eval/release/<suite>-<HEAD12>.yml`.
    The run necessarily happens BEFORE that commit, so the verdict names the
    PARENT of the commit that carries it, and the next `report` is exit 3 again
    at a new hash. Measured on this repository: at `e3a66fe` — the commit whose
    whole content is the recorded baseline for `9c6cade` — `report` exits 3 on
    a clean tree. A gate wired to that rule refuses every release push forever,
    which is an outage wearing a gate's clothes.

    Three resolutions were on the table (`meta/kanban/backlog.md`, 2026-08-21).
    This is (b): the verdict's identity is the eval-RELEVANT TREE, not the
    commit hash. A verdict answers for ``head`` when it names an ANCESTOR of
    ``head`` and the whole difference between that ancestor and ``head`` is
    output of the release eval circuit (:func:`_is_report_artefact`).

    ``commit == head`` is a special case of that and needs no branch of its
    own; it is also UNREACHABLE, which is worth one sentence because the
    absence of a shortcut here looks like an oversight otherwise. The documents
    are read out of ``head``'s own tree (:func:`_read_verdicts`), so a document
    naming ``head`` would have to contain the hash of a tree that contains it —
    a hash fixed point. `merge-base --is-ancestor X X` answers 0 and `diff X X`
    is empty, so the general rule covers the case anyway if one is ever found.

    (a) — "any ancestor whose fixture fingerprint is unchanged" — was rejected
    on evidence: the suite REPLAYS CODE (`hooks/pre-push.py`, `ci/*.py` are
    what EVS-003…005 run), so fixture identity is not run identity, and a
    verdict would ride unchanged over every code change that did not touch a
    fixture. (c) — "the artefact always trails by one commit" — describes the
    bootstrap and nothing else: after one further commit it trails by two, and
    the rule has to grow a number nobody can defend.

    **What (b) costs — the bound RE-DERIVED from the code, after the first
    statement of it turned out to be false.** Strict hash equality has one
    property this rule gives up: a verdict cannot be fabricated for a commit
    that does not exist yet. Under this rule a hand-written `passed` document
    naming an ancestor is accepted. The bound this docstring used to state was
    "the forgery must be COMMITTED, must be the ONLY change in that commit" —
    and REVIEW MEASURED BOTH HALVES WRONG, because the document was read off
    the working tree while freshness was computed over committed content. The
    read now comes out of the pushed commit's tree, which makes the first half
    true; the second half was never true and is not repaired by that, so it is
    restated as what it is:

      * the forged document must be COMMITTED and REACHABLE FROM WHAT IS BEING
        PUSHED — an uncommitted edit and an untracked file are both invisible
        to this gate (`ForgedVerdictTest`, two cases);
      * the window is N artefact-only commits, not one. This rule compares the
        NET range diff, so a chain of artefact-only commits (or empty commits,
        or a commit that adds a file and a later one that removes it) is
        accepted as readily as a single one (`ForgedVerdictTest::
        test_the_artefact_only_window_is_a_chain_not_a_single_commit`).

    None of this is a new SECURITY hole — anyone who can commit can also pass
    `--no-verify` (the ADR-0020 class). It is a bound, and a bound is only
    worth stating at the width the code actually has.

    Against STALENESS, the risk the card is actually about (RISK-006/TM-005),
    this rule is exactly as strong as hash equality: every eval-relevant byte
    between the verdict and ``head`` is compared, and any difference at all —
    one character of one script, one fixture, one case file — makes the verdict
    absent.

    The fixture-set fingerprint recorded in the document is NOT recomputed
    here, and that is a decision rather than an omission. Recomputing it means
    hashing the WORKING TREE, which answers a different question than the one a
    push asks (what is being PUSHED is a commit, not the tree). The binding the
    fingerprint stands for is established above it and more strictly: the
    fixture directory is inside the range this rule compares, so a verdict
    taken against different fixtures cannot pass. The residue: a hand-edited
    fingerprint field is not detected — the same residue as a hand-edited
    verdict field, and it needs the same commit to exploit.

    Returns:
        ``(True, "")`` when the verdict answers for ``head``, else
        ``(False, reason)`` naming exactly why it does not.

    Raises:
        _GateUnavailable: git could not answer; the caller fails open loudly.
    """
    code, _ = _git(root, "cat-file", "-e", f"{commit}^{{commit}}")
    if code != 0:
        return False, (f"recorded against commit {commit[:12]}, which is not a "
                       f"commit in this repository")
    code, _ = _git(root, "merge-base", "--is-ancestor", commit, head)
    if code != 0:
        return False, (f"recorded against commit {commit[:12]}, which is not an "
                       f"ancestor of what this push publishes ({head[:12]})")
    changed = _git_paths(root, "diff", "--name-only", "-z", commit, head)
    foreign = sorted(p for p in changed if not _is_report_artefact(p))
    if foreign:
        shown = ", ".join(foreign[:3])
        more = f" (+{len(foreign) - 3} more)" if len(foreign) > 3 else ""
        return False, (
            f"recorded against commit {commit[:12]}, and {len(foreign)} path(s) "
            f"OUTSIDE {EVAL_REPORT_DIR_REL}/ changed since: {shown}{more}")
    return True, ""


def _nearest(root: Path, documents: list[dict], head: str) -> dict:
    """The answering document whose commit is closest to ``head``.

    Only ever used to NAME one of several equally valid answers — every
    document handed here already passed :func:`_answers_for_head`, so the
    choice cannot change the gate's verdict, only which commit the announcement
    quotes. A git failure here is therefore not worth a fail-open: the first
    document stands, which is the behaviour this function refines.
    """
    def distance(document: dict) -> int:
        try:
            code, out = _git(root, "rev-list", "--count",
                             f"{document.get('commit')}..{head}")
            return int(out) if code == 0 and out.isdigit() else 1 << 30
        except _GateUnavailable:
            return 1 << 30
    return min(documents, key=distance)


def _stored_documents(root: Path, head: str,
                      scratch: Path) -> list[tuple[str, object]]:
    """Every ``*.yml`` in the report directory OF THE COMMIT BEING PUBLISHED.

    Read out of git rather than off the disk, and that is the whole point. A
    push publishes a COMMIT; a document sitting in the working tree — edited
    but not committed, or never tracked at all — is not part of it, and a gate
    that reads one is answering a question nobody asked. Measured before this
    changed: a committed `blocked` verdict flipped to `passed` by an
    UNCOMMITTED edit turned exit 2 into exit 0, and an untracked hand-written
    `passed` was accepted after real work had landed. Both are now invisible
    here (`ForgedVerdictTest`).

    The YAML text is spilled to a scratch file because `load_yaml` in the
    G-3-frozen `hooks/_forge_common.py` takes a Path, and this hook may not
    grow a second YAML entry point (CON-003, ADR-0011/R1) just to avoid a
    tempfile. The scratch directory belongs to the caller and dies with it.

    Returns:
        ``(name, parsed)`` pairs in path order; ``parsed`` is whatever the YAML
        loader made of the blob, including ``None``.

    Raises:
        _GateUnavailable: git could not list or read the commit's tree.
    """
    listed = _git_paths(root, "ls-tree", "-r", "-z", "--name-only", head,
                        "--", EVAL_REPORT_DIR_REL)
    documents: list[tuple[str, object]] = []
    for index, path in enumerate(sorted(p for p in listed
                                        if p.endswith(".yml"))):
        code, text = _git(root, "cat-file", "-p", f"{head}:{path}")
        if code != 0:
            continue  # the tree named it a moment ago; a race is not a verdict
        spill = scratch / f"{index:04d}.yml"
        spill.write_text(text + "\n", encoding="utf-8")
        documents.append((path.rsplit("/", 1)[-1], load_yaml(spill)))
    return documents


def _read_verdicts(root: Path, head: str) -> tuple[list[dict], list[str]]:
    """Every stored document that answers for ``head``, plus the near misses.

    Every rejection is a NEAR MISS naming the file and the scan CONTINUES: a
    directory is not a verdict, and one unreadable file in it must not be able
    to deny a valid one sitting beside it. The vocabulary deliberately echoes
    `ci/eval_runner.py`'s `stored_report`, because a developer reading the
    hook's refusal and the runner's output should not have to translate.

    Raises:
        _GateUnavailable: git could not answer a freshness question.
    """
    answering: list[dict] = []
    misses: list[str] = []
    with tempfile.TemporaryDirectory(prefix="forge-eval-gate-") as tmp:
        stored = _stored_documents(root, head, Path(tmp))
    if not stored:
        return [], [f"{EVAL_REPORT_DIR_REL}/ holds no report document in the "
                    f"commit being published ({head[:12]}) — no release eval "
                    f"run has ever been recorded there"]
    for name, document in stored:
        if not isinstance(document, dict):
            misses.append(f"{name}: cannot be read as a report document")
            continue
        if str(document.get("suite") or "") != EVAL_SUITE:
            continue  # a verdict about another suite is not this gate's business
        if document.get("version") != EVAL_REPORT_VERSION:
            misses.append(
                f"{name}: written in report schema version "
                f"{document.get('version')!r}, this gate reads "
                f"{EVAL_REPORT_VERSION}")
            continue
        if str(document.get("status") or "") != "completed":
            misses.append(f"{name}: status is "
                          f"{document.get('status')!r}, not 'completed'")
            continue
        if str(document.get("worktree") or "") != "clean":
            misses.append(f"{name}: measured on a MODIFIED working tree, "
                          f"so it is not a verdict about any commit")
            continue
        if str(document.get("commit_binding") or "") != "bound":
            misses.append(f"{name}: recorded UNBOUND "
                          f"(commit_binding: {document.get('commit_binding')!r})")
            continue
        commit = str(document.get("commit") or "")
        if len(commit) != 40 or not all(c in "0123456789abcdef" for c in commit):
            misses.append(f"{name}: names no usable commit "
                          f"({document.get('commit')!r})")
            continue
        if not str(document.get("fixture_set_fingerprint") or "").strip():
            misses.append(f"{name}: records no fixture-set fingerprint")
            continue
        verdict = str(document.get("verdict") or "")
        if verdict not in STORED_VERDICTS:
            misses.append(
                f"{name}: verdict {document.get('verdict')!r} is not one "
                f"of {list(STORED_VERDICTS)} — the document cannot be read as "
                f"a verdict")
            continue
        answers, why = _answers_for_head(root, commit, head)
        if not answers:
            misses.append(f"{name}: {why}")
            continue
        answering.append(document)
    return answering, misses


# ── output ────────────────────────────────────────────────────────────────


def _say(line: str) -> None:
    """One line of the eval gate's output, on the stream humans read.

    Through `_err_stream` rather than `sys.stderr`: a descriptor closed at
    process start makes `sys.stderr` literally None and `print(file=None)`
    writes to STDOUT. This gate publishes nothing on stdout and intends to keep
    it that way — including under `2>&-`, where the naive spelling would start.
    """
    print(f"forge(eval-gate): {line}", file=_err_stream())


def _fail_open(meta: Path, outcome: str, detail: str) -> None:
    """The gate could not RUN: allow the push, loudly, and journal it.

    NFR-006 keeps the policy (a broken gate does not hold the push hostage);
    NFR-010 keeps it visible past the terminal. This is NOT a verdict and never
    prints one of the two refusal tokens — a consumer must be able to tell "the
    eval said no" from "the eval was never consulted".
    """
    _say(f"!! {TOKEN_UNVERIFIED} — PUSH ALLOWED WITHOUT THE RELEASE EVAL GATE "
         f"(fail-open, NFR-006).")
    _say(f"!! {detail}")
    try:
        journal = record_gate_event(meta, EVAL_GATE_ID, outcome, detail)
        _say(f"!! recorded in {journal} — /forge:status will flag it. Run "
             f"`python3 ci/eval_runner.py report --suite {EVAL_SUITE}` "
             f"manually before publishing.")
    except Exception as exc:  # noqa: BLE001 — a journal failure must not block
        _say(f"!! could NOT record the event ({exc!r}) — it is visible ONLY "
             f"here.")


def _refuse_blocked(document: dict) -> None:
    """AC-020: a recorded `blocked` verdict refuses the push, marked.

    WHERE THE TYPE GUARD IS — 10 of 10, AND n IS DRAWN OVER BOTH FILES OF THE
    SPLIT (ADR-0022/R2). The guarded operation is "reach into a value that came
    out of an untyped `load_yaml` result". The first statement of this drew n at
    `_read_verdicts`'s two consumers, called it "2 of 2", and thereby excluded
    the sites in `_decision_gate` — where the one that still crashed was
    living. Review found it there, so n became every such site in the hook. The
    CARD-014 extraction then moved this function OUT of that hook, which would
    re-create the same defect the other way round: an n drawn at a file boundary
    the operation does not respect. So n is every such site in
    `hooks/pre-push.py` AND in this module, with the file named on each:

    1. `hooks/pre-push.py::_decision_gate`, the document itself —
       `isinstance(data, dict)`;
    2. `hooks/pre-push.py::_decision_gate`, `data["open"]` —
       `isinstance(entries, list)`;
    3. `hooks/pre-push.py::_decision_gate`, each entry —
       `isinstance(d, dict)`;
    4. `hooks/pre-push.py::_decision_gate`, `entry["blocks"]` —
       `hooks/pre-push.py::_blocks_synthesis`;
    5. this function, `document["aggregate"]` — normalised below;
    6. this function, `document["scenarios"]` — normalised below;
    7. this function, each ELEMENT of that list — `isinstance(scenario, dict)`
       below, which is what makes `scenario.get("outcome")` and the three reads
       on the line under it reads of a mapping. Guarded since cycle 3 and
       undeclared until now, which is the same defect one level down;
    8. `hooks/pre-push.py::_conclude`, `document["verdict"]`;
    9. `hooks/pre-push.py::_conclude`, `document["commit"]` — the trailing
       filter;
    10. `hooks/pre-push.py::_conclude`, the document :func:`_nearest` returns.

    ENTRIES 8-10 ARE NEW WITH CARD-018, and they are why this paragraph is
    re-drawn rather than re-worded: they read documents that came out of
    :func:`_read_verdicts`, while the exclusion below was worded for the reads
    INSIDE that function's own loop — so three guarded sites fell between two
    true sentences. That is the file-boundary error this enumeration says it
    exists to prevent, committed one function further along. Their guard is the
    same `isinstance(document, dict)` continue: a document that is not a mapping
    never reaches `answering`.

    NOT ENTRIES, each for a stated reason. :func:`_stored_documents` calls
    `load_yaml` and reaches into NOTHING — it appends whatever comes back, which
    is precisely why every consumer above has to guard. :func:`_read_verdicts`
    reaches in, and its guard is the `isinstance(document, dict)` continue at
    the top of its own loop, which is what makes every `document.get` below it a
    read of a mapping. `hooks/pre-push.py::_pretooluse_main`'s reads of the
    stdin PAYLOAD are the same shape from a different source (JSON, not YAML)
    and are guarded there, in that function's own docstring — as is
    `hooks/pre-push.py::_git_hook_main`'s read of the raw stdin text. (The
    pointer here said `::main` until CARD-018's review: `main` is now a six-line
    dispatcher that reads nothing, so the sentence named a function that could
    not have the property claimed for it.) The SCALAR reads beside all of these
    are a different operation and are covered differently — every one goes
    through `str(...)` or `.get(key, '?')`, neither of which can raise on a
    scalar.

    AND THE SCOPE OF n IS DERIVED RATHER THAN PROMISED, because this paragraph
    has now been re-drawn three times and each redraw was found by review rather
    than by a test. `TypeGuardEnumerationTest` in
    `tests/test_prepush_eval_gate.py` parses BOTH files, finds every function
    that consumes a `load_yaml` result — directly, or through
    :func:`_read_verdicts` / :func:`_stored_documents` — and fails unless each
    one is named here, so a function that starts reading untyped YAML in either
    file cannot slip past this enumeration in silence.

    THE EVIDENCE, and it is a run rather than a reading: 34 malformed
    `open.yml` documents × the push payload, plus 18 malformed payloads →
    52 invocations, exit codes {0, 2} only, zero tracebacks, zero bytes on
    stdout. The battery is the executable half of this enumeration and the
    thing that found site 4; its shapes live in `OutputDisciplineTest::
    test_the_declared_set_covers_the_DECISION_gate_too` and
    `::test_a_payload_that_is_not_the_expected_shape_is_never_a_third_code`.

    This was 1 of 2 and was reported as a property. A stored `blocked` document
    with `scenarios: 5` passed every guard above and died on the unguarded
    twin with `TypeError: 'int' object is not iterable`, AFTER two refusal
    lines had printed — exit 1, outside the declared set {0, 2}, which Claude
    Code reads as a non-blocking error. A malformed document turned a blocked
    release into a proceeding one; `MalformedDocumentTest` is what stops that
    coming back.
    """
    aggregate = document.get("aggregate")
    aggregate = aggregate if isinstance(aggregate, dict) else {}
    scenarios = document.get("scenarios")
    scenarios = scenarios if isinstance(scenarios, list) else []
    _say(f"{TOKEN_FAILED} — push blocked: the recorded release eval verdict "
         f"for {str(document.get('commit') or '?')[:12]} is BLOCKED.")
    _say(f"  suite {EVAL_SUITE!r}, threshold "
         f"{str(document.get('threshold') or '?')}: "
         f"{aggregate.get('regressions', '?')} regression(s) over "
         f"{aggregate.get('scenarios', '?')} scenario(s), regression_score "
         f"{aggregate.get('regression_score', '?')}")
    for scenario in scenarios:
        if isinstance(scenario, dict) and scenario.get("outcome") != "pass":
            _say(f"  {scenario.get('id', '?')}: {scenario.get('name', '?')} — "
                 f"{scenario.get('detail', 'no detail')}")
    _say("Fix the regression and re-run "
         f"`python3 ci/eval_runner.py run --suite {EVAL_SUITE}`; a release is "
         "not published below the threshold (NFR-004, INV-006).")


def _refuse_missing(head: str, misses: list[str]) -> None:
    """INV-006: nothing answers for the commit being published — refuse."""
    _say(f"{TOKEN_MISSING} — push blocked: no release eval verdict answers for "
         f"what this push publishes ({head[:12]}, suite {EVAL_SUITE!r}).")
    _say("An ABSENT verdict is BLOCKED, never 'passed' (INV-006) — a release "
         "nobody measured is not a release that passed.")
    for miss in misses:
        _say(f"  {miss}")
    _say(f"Recover: on a clean tree run `python3 ci/eval_runner.py run --suite "
         f"{EVAL_SUITE}`, commit the "
         f"{EVAL_REPORT_DIR_REL}/{EVAL_SUITE}-<commit12>.yml it writes, and "
         f"push — that commit's only change is the artefact, which is exactly "
         f"what this gate accepts.")
