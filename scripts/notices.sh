#!/usr/bin/env bash
set -euo pipefail
go list -m -json all > /dev/null
echo "dependency inventory validated; install go-licenses to generate THIRD_PARTY_NOTICES.md"
