#!/usr/bin/env bash
set -euo pipefail
patterns='(BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|client_secret:[[:space:]]*[^"[:space:]]|password:[[:space:]]*[^"[:space:]]|token:[[:space:]]*[^"[:space:]]|10\.[0-9]+\.[0-9]+\.[0-9]+|192\.168\.[0-9]+\.[0-9]+|172\.(1[6-9]|2[0-9]|3[01])\.[0-9]+\.[0-9]+)'
if git grep -nE "$patterns" -- ':!scripts/check-secret-hygiene.sh' ':!CHANGELOG.md'; then
  echo "secret hygiene check failed" >&2
  exit 1
fi
echo "secret hygiene check passed"
