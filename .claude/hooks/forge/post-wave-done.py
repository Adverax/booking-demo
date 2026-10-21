# forge
#!/usr/bin/env python3
"""
forge post-wave-done hook  —  PostToolUse on Write / Edit / MultiEdit

Suggests /forge:retrospective when a kanban card transitions to "done"
and no cards remain in progress — i.e. a wave just completed.

Fires on any write/edit to a card file (…/kanban/cards/*.md). The card's
CURRENT state is read from disk — not from tool_input, whose shape differs
per tool (`content` for Write, `new_string` for Edit, `edits[]` for MultiEdit;
the old content-only check meant Edit-based status flips never fired).

Install via: /forge:init hooks
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

from _forge_common import find_meta, forge_paths, to_relative


def main() -> None:
    try:
        data = json.load(sys.stdin)
    except Exception:
        sys.exit(0)

    file_path: str = data.get("tool_input", {}).get("file_path", "")
    if not file_path or not file_path.endswith(".md"):
        sys.exit(0)

    meta = find_meta()
    if meta is None:
        sys.exit(0)
    cards_dir = forge_paths(meta)["cards"]
    if not cards_dir.is_dir():
        sys.exit(0)

    # Only act on writes to card files (absolute or cwd-relative path)
    written = Path(file_path)
    if not written.is_absolute():
        written = Path.cwd() / to_relative(file_path)
    try:
        written.resolve().relative_to(cards_dir.resolve())
    except ValueError:
        sys.exit(0)

    # The write already happened (PostToolUse) — disk is the truth.
    try:
        card_text = written.read_text(encoding="utf-8", errors="ignore")
    except Exception:
        sys.exit(0)
    if "Status: done" not in card_text and "Status:** done" not in card_text:
        sys.exit(0)                     # this card isn't done — nothing to do

    in_progress = 0
    done = 0
    for card in cards_dir.glob("*.md"):
        try:
            text = card.read_text(encoding="utf-8", errors="ignore")
        except Exception:
            continue
        if "Status: in_progress" in text or "Status:** in_progress" in text:
            in_progress += 1
        elif "Status: done" in text or "Status:** done" in text:
            done += 1

    if in_progress == 0 and done > 0:
        print(
            f"forge: wave complete ({done} card(s) done, 0 in progress)"
            " — run /forge:retrospective to capture learnings.",
            file=sys.stderr,
        )


if __name__ == "__main__":
    main()
