#!/usr/bin/env python3
"""Check that commands documented in README.md and docs/ have real entry points."""

from __future__ import annotations

import pathlib
import re
import shlex
import shutil

ROOT = pathlib.Path(__file__).resolve().parents[1]
FILES = [ROOT / "README.md", *sorted((ROOT / "docs").glob("*.md"))]
JUSTFILE = ROOT / "justfile"
BUILTINS = {"export"}
GENERATED = {"bin/polylens2otel": "build"}


def recipe_names(source: str) -> set[str]:
    """Return just recipe names, excluding variable assignments."""
    return set(re.findall(r"^([A-Za-z0-9_.-]+)(?:[ \t]+[^:\n]+)?:(?![=])", source, re.MULTILINE))


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


def command_errors(checked: list[tuple[pathlib.Path, str]], targets: set[str]) -> list[str]:
    errors = []
    for path, line in checked:
        words = shlex.split(line)
        command = words[0]
        relative_path = path.relative_to(ROOT)
        if command == "make":
            errors.append(f"{relative_path}: `make` is retired; use `just build`")
            continue
        if command == "just":
            for target in (word for word in words[1:] if not word.startswith("-")):
                if target not in targets:
                    errors.append(f"{relative_path}: no just recipe {target!r}")
            continue
        if command in BUILTINS or shutil.which(command):
            continue
        if command in GENERATED and GENERATED[command] in targets:
            continue
        errors.append(f"{relative_path}: command not found: {command!r}")
    return errors


def main() -> None:
    targets = recipe_names(JUSTFILE.read_text(encoding="utf-8"))
    checked = commands()
    errors = command_errors(checked, targets)
    if errors:
        raise SystemExit("\n".join(errors))
    print(f"checked {len(checked)} documented commands across {len(FILES)} files")


if __name__ == "__main__":
    main()
