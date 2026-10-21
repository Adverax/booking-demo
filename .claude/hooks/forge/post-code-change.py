# forge
#!/usr/bin/env python3
"""
forge post-code-change hook  —  PostToolUse on Write / Edit / MultiEdit

Fires when a source file (non-meta, implementation code) is written or edited
directly in a session — outside the standard kanban pipeline. Suggests:
  • /forge:explain  when source files change (how-it-works docs may be stale)
  • /forge:readme   for the changed directory (per-dir READMEs are refreshed by
                    the kanban card cycle — a direct edit bypasses that)
  • /forge:docs     when requirements.yml changes (role docs may be stale)

Complements post-edit.py (trace.yml drift). This hook focuses on documentation
freshness after direct code edits that bypass the kanban flow.
No-op when the architecture model does not exist.

Exit 0 always — advisory only.

Install via: /forge:init hooks
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

from _forge_common import find_meta, forge_paths, to_relative

# Source file extensions that trigger /forge:explain
_SOURCE_EXTS = {
    ".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".java", ".cs",
    ".rb", ".rs", ".cpp", ".c", ".h", ".kt", ".swift", ".vue",
}

# Path prefixes that are NOT implementation code (meta dir added dynamically)
_SKIP_PREFIXES = (".claude/", "docs/", ".git/", "node_modules/")


def main() -> None:
    try:
        data = json.load(sys.stdin)
    except Exception:
        sys.exit(0)

    file_path: str = data.get("tool_input", {}).get("file_path", "")
    if not file_path:
        sys.exit(0)

    meta = find_meta()
    if meta is None:
        sys.exit(0)
    paths = forge_paths(meta)
    if not paths["architecture"].is_dir():
        sys.exit(0)

    rel = to_relative(file_path).replace("\\", "/")
    path = Path(rel)

    # requirements.yml change → role docs may be stale
    try:
        abs_written = Path(file_path) if Path(file_path).is_absolute() else Path.cwd() / rel
        if abs_written.resolve() == paths["requirements"].resolve():
            print(
                "forge: requirements.yml changed — role docs may be stale."
                "\n  → /forge:docs  to regenerate end-user documentation by role",
                file=sys.stderr,
            )
            sys.exit(0)
    except Exception:
        pass

    # Skip non-source directories (including the configured meta/kanban dirs,
    # not just the literal "meta/" prefix)
    meta_rel = str(meta.relative_to(meta.parent)) + "/"
    for prefix in (_SKIP_PREFIXES + (meta_rel,)):
        if rel.startswith(prefix):
            sys.exit(0)

    # Skip non-source files
    if path.suffix not in _SOURCE_EXTS:
        sys.exit(0)

    print(
        f"forge: '{rel}' edited outside kanban pipeline — docs may be stale."
        f"\n  → /forge:readme {path.parent}/  to refresh that directory's README"
        "\n  → /forge:explain [aspect]  to regenerate how-it-works docs for the area"
        "\n  → /forge:explain all        for a full refresh",
        file=sys.stderr,
    )


if __name__ == "__main__":
    main()
