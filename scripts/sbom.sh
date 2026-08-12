#!/usr/bin/env bash
set -euo pipefail
command -v syft >/dev/null || { echo "syft is required" >&2; exit 2; }
mkdir -p dist
syft dir:. -o spdx-json=dist/polylens2otel.spdx.json
