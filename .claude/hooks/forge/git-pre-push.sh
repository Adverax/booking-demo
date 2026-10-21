#!/bin/sh
# forge git pre-push hook — THE AUTHORITY ON WHAT A PUSH PUBLISHES (ADR-0023)
#
# ⚑ `/bin/sh`, NOT `#!/usr/bin/env bash` like its two siblings, and the reason
#   is measured rather than stylistic: a wrapper whose interpreter cannot be
#   found exits 127, and in THIS position git reads any non-zero as "refuse".
#   Found by execution, not by review: with `#!/usr/bin/env bash` the push was
#   rejected with `env: bash: No such file or directory` and nothing else — a
#   gate holding a developer hostage over a shell it did not need. Everything
#   below is POSIX, and the choice is GUARDED rather than remembered:
#   `WrapperFailOpenTest::test_no_python3_allows_the_push_and_says_so` builds a
#   PATH holding `sh` and not `bash`, so a shebang that reached for bash again
#   turns that case red. The two suggestion hooks keep their own shebang,
#   because a post-commit hook that cannot start costs a hint, not a push.
#
# git runs this before it updates any remote ref and hands it, on stdin, one
# line per ref being updated:
#
#     <local ref> <local sha> <remote ref> <remote sha>
#
# That is the authoritative answer to "what does this push publish?" — it comes
# from the process performing the operation, not from a reading of a command
# string. The verdict logic itself is pre-push.py --git-hook; this wrapper only
# finds it, runs it, and TRANSLATES ITS EXIT CODE.
#
# ── WHY THE TRANSLATION EXISTS, AND WHY IT IS NOT COSMETIC ───────────────────
#
# Two measured facts collide in this file.
#
# 1. git COLLAPSES every non-zero hook exit into its own 1 (ADR-0023,
#    Consequences → Negative; re-measured for this card on codes 1, 2, 3 and
#    42 — `git push` returned 1 and rejected the push on all four). So the
#    declared exit set of ADR-0007/R1 carries no information out of this
#    channel; the token vocabulary on stderr is the only discriminator, and
#    stderr is passed through untouched below.
# 2. In THIS position a crashing gate BLOCKS, which inverts NFR-006's
#    fail-open. A Python traceback exits 1, git reads 1 as "refuse", and the
#    developer's push is held hostage by a broken gate — the exact policy
#    NFR-006 forbids. Inside pre-push.py that is re-implemented explicitly
#    (every error path ends in exit 0); but a failure BEFORE Python runs — no
#    interpreter, no hook file, a syntax error, a signal — cannot be caught
#    from inside it.
#
# So this wrapper is the outer half of the fail-open, and the rule is stated as
# a whitelist rather than a blacklist: **the ONLY refusal is the gate's own
# deliberate exit 2.** Every other outcome — including a crash, a missing
# interpreter and an uninstalled hook — allows the push, LOUDLY.
#
# ⚑ FORGE_PRE_PUSH_REFUSAL is the gate's declared refusal code and is NOT a
#   knob. It is duplicated from pre-push.py's EXIT_REFUSE, and the duplicate is
#   pinned: `PublicationResidueTest::test_the_wrapper_refuses_on_the_code_the_hook_declares`
#   parses both files and fails if they ever disagree — otherwise a renumbered
#   refusal would turn every block into a silent allow, which is the shape of
#   defect this whole card exists to remove.
#
# ── THE FAIL-OPEN ROUTES OF THIS FILE ────────────────────────────────────────
#
# ONE ENTRY PER `forge_allow_loudly` CALL SITE, each carrying a `# fail-open
# w<n>` tag on the line above it and each naming the test that DRIVES it
# (ADR-0022/R1). THE COUNT IS NOT WRITTEN HERE and must not be — the python
# half of this gate learned that the hard way, three passes producing three
# numbers. It is DERIVED by `tests/test_prepush_eval_gate.py::
# WrapperFailOpenEnumerationTest`, which parses this file, counts the call
# sites, and requires one entry per tag and one tag per site.
#
# ⚑ THIS ENUMERATION IS NEW WITH FIX CYCLE 3, AND THE REASON IS A MEASUREMENT.
#   `hooks/eval_verdict.py` claimed both halves of the many-to-one token count
#   were derived by a single test; that test asserted only that each side has
#   MORE THAN ONE route. A FIFTH route was added here and NOTHING moved — no
#   test, no count, no guard. The python side really was derived, by two other
#   tests through exactly the tag correspondence below; this side was derived
#   by nothing. Now it is derived the same way, and the attribution in
#   `hooks/eval_verdict.py` names which test derives which half.
#
#   Every entry names its driving test in `tests/test_publication_authority.py`
#   (ADR-0022/R1), on ONE line, because a name broken across two comment lines
#   is a name no checker can read back.
#
#   1. not inside a git work tree, so no repository could be looked at —
#      `WrapperFailOpenTest::test_outside_a_work_tree_the_wrapper_allows_and_says_so`
#   2. no `python3` on PATH, so the gate could not be spawned at all —
#      `WrapperFailOpenTest::test_no_python3_allows_the_push_and_says_so`
#   3. the gate file is not installed — the half-armed shape, and the ONE route
#      in this file that names a remedy —
#      `WrapperFailOpenTest::test_an_uninstalled_gate_allows_the_push_and_says_so`
#   4. the gate exited outside its declared set {0, 2}: it crashed rather than
#      concluded —
#      `WrapperFailOpenTest::test_a_hook_that_crashes_allows_the_push`,
#      `WrapperFailOpenTest::test_only_the_declared_refusal_code_refuses`
#
# Install via: /forge:init hooks
# Or manually: cp .claude/hooks/forge/git-pre-push.sh .git/hooks/pre-push
#              chmod +x .git/hooks/pre-push
#              (an installed hook without the executable bit is skipped by git
#               SILENTLY — which is why ci/hook_install_drift.py checks the
#               mode of all three installed git hooks, not just their bytes.)

FORGE_PRE_PUSH_REFUSAL=2

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)"
HOOK="${ROOT}/.claude/hooks/forge/pre-push.py"

forge_allow_loudly() {
    echo "forge(eval-gate): !! eval-gate-unverified — PUSH ALLOWED WITHOUT THE RELEASE EVAL GATE (fail-open, NFR-006)." >&2
    echo "forge(eval-gate): !! $1" >&2
    exit 0
}

if [ -z "${ROOT}" ]; then
    # fail-open w1
    forge_allow_loudly "not inside a git work tree, so no release verdict could be looked up."
fi

if ! command -v python3 >/dev/null 2>&1; then
    # No remedy is named here, and the omission is not tidiness. The obvious
    # sentence ("run the eval runner before publishing") points a machine with
    # no python3 at a python3 script, and ADR-0023 measured that a refusal
    # which names a cure gets the cure RUN — 2 agents out of 2, on a cure that
    # evaluates nothing. A hint that cannot work is worse than none.
    # fail-open w2
    forge_allow_loudly "python3 is not on PATH, so the release eval gate did not run and nothing about this release was measured (CON-003: forge is Markdown and Python 3)."
fi

if [ ! -f "${HOOK}" ]; then
    # The hint is BACKTICKED on purpose: `RemedyHintSurfaceTest` collects every
    # backtick-quoted executable command the gate names and compares the set
    # against its declared bound, in both directions. An un-backticked hint is
    # invisible to that collector, so it would be a remedy inside the surface
    # and outside the guard — which is how this one stood undriven until
    # CARD-018's review counted the routes.
    # fail-open w3
    forge_allow_loudly ".claude/hooks/forge/pre-push.py is missing — the release eval gate is NOT installed and did not run. Re-run \`/forge:init hooks\`."
fi

python3 "${HOOK}" --git-hook
forge_status=$?

if [ "${forge_status}" -eq "${FORGE_PRE_PUSH_REFUSAL}" ]; then
    # The gate concluded. git turns any non-zero into its own 1; the reason is
    # on stderr above, led by a token from the closed set.
    exit 1
fi

if [ "${forge_status}" -ne 0 ]; then
    # fail-open w4
    forge_allow_loudly "the release eval gate exited ${forge_status}, which is outside its declared set {0, ${FORGE_PRE_PUSH_REFUSAL}} — it crashed rather than concluded, so nothing was measured and nothing may be concluded (NFR-006)."
fi

exit 0
