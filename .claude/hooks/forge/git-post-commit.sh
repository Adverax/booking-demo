#!/usr/bin/env bash
# forge git post-commit hook
#
# Suggests /forge:explain and /forge:docs when source files change outside
# the kanban pipeline (direct commits, hotfixes, manual edits).
#
# Install via: /forge:init hooks
# Or manually: cp .claude/hooks/forge/git-post-commit.sh .git/hooks/post-commit
#              chmod +x .git/hooks/post-commit

FORGE_META_DIR="${FORGE_META_DIR:-meta}"

# No-op when not a forge project
if [ ! -d "${FORGE_META_DIR}/architecture" ]; then
    exit 0
fi

# Get changed files in the last commit
# For the initial commit use the empty-tree SHA
empty_tree=$(git hash-object -t tree /dev/null 2>/dev/null)
parent=$(git rev-parse --verify HEAD~1 2>/dev/null || echo "$empty_tree")
changed=$(git diff --name-only "$parent" HEAD 2>/dev/null)

suggest_explain=0
suggest_docs=0

while IFS= read -r file; do
    [ -z "$file" ] && continue

    # requirements.yml → docs suggestion
    if [ "$file" = "${FORGE_META_DIR}/architecture/requirements.yml" ]; then
        suggest_docs=1
        continue
    fi

    # Skip the meta dir (configured, not hardcoded), docs/, .claude/, node_modules/
    case "$file" in
        "${FORGE_META_DIR}"/*|.claude/*|docs/*|.git/*|node_modules/*) continue ;;
    esac

    # Source file extensions
    case "${file##*.}" in
        go|py|ts|tsx|js|jsx|java|cs|rb|rs|cpp|c|h|kt|swift|vue)
            suggest_explain=1 ;;
    esac
done <<< "$changed"

if [ "$suggest_explain" -eq 1 ]; then
    echo "forge: source files changed — how-it-works docs may be stale." >&2
    echo "  → /forge:explain [aspect]  to regenerate docs for the affected area" >&2
    echo "  → /forge:explain all        for a full refresh" >&2
fi

if [ "$suggest_docs" -eq 1 ]; then
    echo "forge: requirements.yml changed — role docs may be stale." >&2
    echo "  → /forge:docs  to regenerate end-user documentation by role" >&2
fi

exit 0
