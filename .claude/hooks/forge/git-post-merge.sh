#!/usr/bin/env bash
# forge git post-merge hook
#
# Fires after `git pull` / `git merge` in the terminal (outside Claude sessions)
# and delegates external-drift classification to post-pull.py — the same logic
# the in-session PostToolUse hook runs, so terminal pulls and session pulls
# produce one signal file (meta/drift-pending.yml).
#
# Install via: /forge:init hooks
# Or manually: cp .claude/hooks/forge/git-post-merge.sh .git/hooks/post-merge
#              chmod +x .git/hooks/post-merge

HOOK_DIR="$(git rev-parse --show-toplevel 2>/dev/null)/.claude/hooks/forge"

if command -v python3 >/dev/null 2>&1 && [ -f "${HOOK_DIR}/post-pull.py" ]; then
    (cd "$(git rev-parse --show-toplevel)" && python3 "${HOOK_DIR}/post-pull.py" --git-hook)
else
    echo "forge: pulled external changes — drift detection unavailable" >&2
    echo "  (python3 or .claude/hooks/forge/post-pull.py missing; run /forge:init hooks)" >&2
fi

exit 0
