#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

failed=0
fail_matches() {
  local label="$1"
  shift
  local output
  if output="$($@ 2>/dev/null)"; then
    printf 'FAIL: %s\n%s\n' "$label" "$output" >&2
    failed=1
  fi
}

# Key material, well-known token formats, and private deployment addresses are
# forbidden in every tracked file. This intentionally scans source and tests as
# well as configuration files.
fail_matches "private key or credential-shaped token in tracked files" \
  git grep -nEI '(BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{20,})' -- ':!scripts/check-secret-hygiene.sh'
fail_matches "private IPv4 address in tracked files" \
  git grep -nE '(10\.[0-9]+\.[0-9]+\.[0-9]+|192\.168\.[0-9]+\.[0-9]+|172\.(1[6-9]|2[0-9]|3[01])\.[0-9]+\.[0-9]+)' -- ':!CHANGELOG.md'

# Secret-like YAML keys are allowed only when empty. Restricting this check to
# YAML avoids false positives from Go field names and explanatory prose while
# still covering the committed deployment surfaces.
yaml_hits="$(git grep -nE '^[[:space:]]*(client_secret|password|token):[[:space:]]*[^#[:space:]]' -- '*.yaml' '*.yml' 2>/dev/null || true)"
if [[ -n "$yaml_hits" ]]; then
  yaml_hits="$(printf '%s\n' "$yaml_hits" | grep -Ev ':[[:space:]]*(client_secret|password|token):[[:space:]]*(""|'"''"')([[:space:]]*#.*)?$' || true)"
  if [[ -n "$yaml_hits" ]]; then
    printf 'FAIL: non-empty YAML secret value in tracked files\n%s\n' "$yaml_hits" >&2
    failed=1
  fi
fi

# Captured JSON may preserve an API response shape, but credential fields must
# contain the literal redaction marker and nothing else.
json_hits="$(git grep -nE '"(access_token|client_secret|password|token)"[[:space:]]*:[[:space:]]*"[^"]+"' -- '*.json' 2>/dev/null || true)"
if [[ -n "$json_hits" ]]; then
  json_hits="$(printf '%s\n' "$json_hits" | grep -Fv '[redacted]' || true)"
  if [[ -n "$json_hits" ]]; then
    printf 'FAIL: non-redacted JSON credential field in tracked files\n%s\n' "$json_hits" >&2
    failed=1
  fi
fi

# Local credential/config paths must stay ignored by both Git and the Docker
# build context. Examples and committed deployment templates remain trackable.
for path in .env config.local.yaml THIRD_PARTY_NOTICES.md; do
  if ! git check-ignore -q --no-index -- "$path"; then
    printf 'FAIL: sensitive/local path is not ignored by Git: %s\n' "$path" >&2
    failed=1
  fi
done
for path in codex docs/superpowers .tools bin dist; do
  if ! git check-ignore -q --no-index -- "$path/.hygiene-sentinel"; then
    printf 'FAIL: sensitive/local directory is not ignored by Git: %s\n' "$path" >&2
    failed=1
  fi
done
for path in .env config.local.yaml codex docs/superpowers .tools bin dist; do
  if ! grep -Fxq "$path" .dockerignore; then
    printf 'FAIL: sensitive/local path is not excluded from Docker context: %s\n' "$path" >&2
    failed=1
  fi
done
for path in config.example.yaml deploy/docker-compose.yaml; do
  if git check-ignore -q --no-index -- "$path"; then
    printf 'FAIL: committed example/template is ignored by Git: %s\n' "$path" >&2
    failed=1
  fi
done

if ((failed != 0)); then
  echo "secret hygiene check failed" >&2
  exit 1
fi
echo "secret hygiene check passed"
