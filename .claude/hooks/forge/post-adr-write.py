# forge
#!/usr/bin/env python3
"""
forge post-adr-write hook  —  PostToolUse on Write / Edit

After an ADR file is written, verifies that the corresponding DEC entry
in decisions/open.yml (or decisions/resolved.yml) has been updated with
resolved_by: ADR-NNNN. Also flags a revised Migration policy (rewrite /
on-touch) — that is the event /forge:audit exists for. Warns — does not block.

Detects the ADR number from the filename: NNNN-<slug>.md → ADR-NNNN.

Install via: /forge:init hooks
"""
from __future__ import annotations

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

    rel = to_relative(file_path)
    rel_path = Path(rel)

    # Only act on ADR files: …/decisions/adr/NNNN-*.md
    if rel_path.suffix != ".md":
        sys.exit(0)
    parts = rel_path.parts
    if "adr" not in parts or "decisions" not in parts:
        sys.exit(0)

    # Extract zero-padded ADR number from filename
    m = re.match(r"^(\d+)-", rel_path.name)
    if not m:
        sys.exit(0)

    adr_ref = f"ADR-{m.group(1)}"       # e.g. ADR-0003

    meta = find_meta()
    if meta is None:
        sys.exit(0)
    decisions = forge_paths(meta)["decisions"]
    open_file = decisions / "open.yml"
    resolved_file = decisions / "resolved.yml"

    if open_file.exists():
        try:
            combined = open_file.read_text(encoding="utf-8", errors="ignore")
            if resolved_file.exists():
                combined += resolved_file.read_text(encoding="utf-8", errors="ignore")
        except Exception:
            combined = ""
        adr_short = f"ADR-{int(m.group(1))}"
        if combined and adr_ref not in combined and adr_short not in combined:
            print(
                f"forge: {rel_path.name} written, but '{adr_ref}' not found in"
                " decisions/open.yml or decisions/resolved.yml.",
                file=sys.stderr,
            )
            print(
                "  Update the DEC entry (resolved_by, status: resolved)"
                " per /forge:architect-adr-writer Step 6.",
                file=sys.stderr,
            )

    # Migration policy → audit nudge: a revised ADR with rewrite/on-touch is
    # exactly the "norm changed, no repo event" case the audit loop covers.
    try:
        written = Path(file_path) if Path(file_path).is_absolute() else Path.cwd() / rel
        adr_text = written.read_text(encoding="utf-8", errors="ignore")
    except Exception:
        sys.exit(0)
    if re.search(r"^\*\*Revised:\*\*\s*\d{4}-", adr_text, re.MULTILINE) and \
            re.search(r"^\*\*Migration:\*\*\s*(rewrite|on-touch)", adr_text, re.MULTILINE):
        print(
            f"forge: {adr_ref} carries Migration: rewrite/on-touch — existing code may now"
            " be non-conformant. Run /forge:audit " + adr_ref + " to scope the debt.",
            file=sys.stderr,
        )


if __name__ == "__main__":
    main()
