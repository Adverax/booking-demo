# forge
#!/usr/bin/env python3
"""
forge post-edit hook  —  PostToolUse on Write / Edit / MultiEdit

After any file is written or edited, checks whether that file is covered by
trace.yml — either referenced by path or matched by a components[].code glob.
If yes, suggests trace↔code drift validation.

(The old check was a whole-file substring test on the basename: `client.go`
matched ANY mention of any client.go — noisy — while a file covered only via a
glob like `internal/import/**` never matched at all.)

Exit 0 always (this hook never blocks — it only warns).

Install via: /forge:init hooks
"""
from __future__ import annotations

import fnmatch
import json
import re
import sys
from pathlib import Path

from _forge_common import find_meta, forge_paths, to_relative


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
    trace_file = forge_paths(meta)["trace"]
    if not trace_file.exists():
        sys.exit(0)

    rel = to_relative(file_path).replace("\\", "/")
    if not rel or rel.startswith((".git/", ".claude/")):
        sys.exit(0)

    try:
        content = trace_file.read_text(encoding="utf-8", errors="ignore")
    except Exception:
        sys.exit(0)

    if _covered_by_trace(rel, content):
        print(
            f"forge: '{rel}' is covered by trace.yml"
            " — run /forge:ci to verify trace↔code conformance.",
            file=sys.stderr,
        )


def _covered_by_trace(rel: str, trace_text: str) -> bool:
    # 1. Exact relative-path mention.
    if rel in trace_text:
        return True
    # 2. Glob coverage: collect quoted-or-bare values of `code:` list items and
    #    inline lists; match with fnmatch (globs use `**` and `*`).
    globs: list[str] = []
    for m in re.finditer(r"^[ \t]*(?:-[ \t]+|code:[ \t]*\[)['\"]?([^'\"\],\s]+)", trace_text, re.MULTILINE):
        v = m.group(1)
        if any(ch in v for ch in "*?") or v.endswith("/"):
            globs.append(v.rstrip("/"))
    for g in globs:
        if fnmatch.fnmatch(rel, g) or fnmatch.fnmatch(rel, g + "/*") \
                or fnmatch.fnmatch(rel, g.replace("**", "*")):
            return True
    # 3. Basename fallback ONLY for exact path-segment mentions (…/name or ^name),
    #    not bare substring — keeps the old catch without its noise.
    base = Path(rel).name
    return bool(re.search(rf"(^|[/\s'\"]){re.escape(base)}\b", trace_text, re.MULTILINE)
                and base in trace_text)


if __name__ == "__main__":
    main()
