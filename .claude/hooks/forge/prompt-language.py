# forge
#!/usr/bin/env python3
"""
forge prompt-language hook  —  UserPromptSubmit lifecycle

Re-injects the project's conversation language (meta/.skills.yml `language:`)
into context on every user prompt. Counters the drift toward English in long
sessions: skill instruction files are English, and context compaction loses
the user's own language cue — without a per-prompt reminder the dialogue
gradually flips to English regardless of the configured language.

stdout is appended to the model context by Claude Code (exit 0).
No-op when meta/.skills.yml is absent, unreadable, or language is `en`.

Install via: /forge:init hooks
"""
from __future__ import annotations

import re
import sys

from _forge_common import find_meta

LANG_NAMES = {
    "ru": "Russian (русский)",
    "de": "German (Deutsch)",
    "es": "Spanish (español)",
    "fr": "French (français)",
    "uk": "Ukrainian (українська)",
    "it": "Italian (italiano)",
    "pt": "Portuguese (português)",
    "pl": "Polish (polski)",
    "ja": "Japanese (日本語)",
    "zh": "Chinese (中文)",
    "ko": "Korean (한국어)",
}


def main() -> None:
    meta = find_meta()
    if meta is None:
        sys.exit(0)
    cfg = meta / ".skills.yml"
    if not cfg.exists():
        sys.exit(0)

    try:
        text = cfg.read_text(encoding="utf-8", errors="ignore")
    except Exception:
        sys.exit(0)

    # top-level key only (no leading whitespace) — a nested `language:` under
    # some other block must not hijack the dialogue language
    m = re.search(r"^language:\s*['\"]?([A-Za-z-]+)", text, re.MULTILINE)
    if not m:
        sys.exit(0)

    code = m.group(1).lower()
    if code in ("en", "english"):
        sys.exit(0)
    # Unknown-but-valid code → still remind, naming the code itself; a project
    # that configured `language: cs` should not silently drift to English just
    # because the code is missing from the table.
    name = LANG_NAMES.get(code, f"the language with code `{code}`")

    print(
        f"forge: project language is `{m.group(1)}`. Address the user in {name} — "
        "all conversational replies, reports, ranked lists, questions, and triage "
        "prompts, even right after loading English skill instructions. Generated "
        "artifacts follow the same setting; git commit messages stay English."
    )


if __name__ == "__main__":
    main()
