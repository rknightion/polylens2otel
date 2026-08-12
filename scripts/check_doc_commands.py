#!/usr/bin/env python3
"""Check that commands documented in README.md and docs/ have real entry points."""

from __future__ import annotations

import pathlib
import re
import shlex
import shutil

ROOT = pathlib.Path(__file__).resolve().parents[1]
FILES = [ROOT / "README.md", *sorted((ROOT / "docs").glob("*.md"))]
MAKEFILE = (ROOT / "Makefile").read_text(encoding="utf-8")
TARGETS = set(re.findall(r"^([A-Za-z0-9_.-]+):", MAKEFILE, re.MULTILINE))
BUILTINS = {"export"}
GENERATED = {"bin/polylens2otel": "build"}


def commands() -> list[tuple[pathlib.Path, str]]:
    found = []
    for path in FILES:
        body = path.read_text(encoding="utf-8")
        for block in re.findall(r"^```(?:sh|shell)\n(.*?)^```", body, re.MULTILINE | re.DOTALL):
            for raw in block.splitlines():
                line = raw.strip()
                if line and not line.startswith("#"):
                    found.append((path, line))
    return found


def main() -> None:
    errors = []
    checked = commands()
    for path, line in checked:
        words = shlex.split(line)
        command = words[0]
        if command == "make":
            for target in (word for word in words[1:] if not word.startswith("-")):
                if target not in TARGETS:
                    errors.append(f"{path.relative_to(ROOT)}: no Make target {target!r}")
            continue
        if command in BUILTINS or shutil.which(command):
            continue
        if command in GENERATED and GENERATED[command] in TARGETS:
            continue
        errors.append(f"{path.relative_to(ROOT)}: command not found: {command!r}")
    if errors:
        raise SystemExit("\n".join(errors))
    print(f"checked {len(checked)} documented commands across {len(FILES)} files")


if __name__ == "__main__":
    main()
