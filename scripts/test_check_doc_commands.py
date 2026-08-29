"""Tests for documented-command task-surface parsing."""

from __future__ import annotations

import unittest

from check_doc_commands import ROOT, command_errors, recipe_names


class RecipeNamesTest(unittest.TestCase):
    def test_parses_recipe_headers_with_attributes_and_parameters(self) -> None:
        source = """set shell := [\"bash\"]\ntools_dir := \".tools\"\n\n# a comment\n[group('check')]\ntest filter=\"\":\n    go test ./...\n\n[private]\n_compile-check:\n    go build ./...\n\nbuild:\n    go build ./cmd/example\n"""

        self.assertEqual({"test", "_compile-check", "build"}, recipe_names(source))

    def test_rejects_retired_make_commands(self) -> None:
        errors = command_errors([(ROOT / "README.md", "make build")], {"build"})

        self.assertEqual(["README.md: `make` is retired; use `just build`"], errors)


if __name__ == "__main__":
    unittest.main()
