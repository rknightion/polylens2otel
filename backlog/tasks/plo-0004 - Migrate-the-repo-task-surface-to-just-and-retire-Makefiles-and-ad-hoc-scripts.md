---
id: PLO-0004
title: Migrate the repo task surface to just and retire Makefiles and ad-hoc scripts
status: Parked
assignee: []
created_date: '2026-08-28 19:27'
updated_date: '2026-08-29 14:29'
labels:
  - 'wave:2-fleet'
dependencies: []
priority: medium
type: chore
ordinal: 4000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
# Migrate polylens2otel to `just`

This task implements the fleet `just` migration standard (rknightion / m7kni / BroTEK-Solutions) for
`rknightion/polylens2otel`. Follow this document exactly — it is self-contained and precise on
purpose; do not re-derive the plan.

## 1. Outcome

`polylens2otel` gets a single top-level `justfile` that is the entire developer/CI task surface. The
Makefile is deleted. `scripts/notices.sh`, `scripts/regen-generated.sh` and `scripts/sbom.sh` are
absorbed into recipes and deleted. `scripts/check-secret-hygiene.sh` and `scripts/check_doc_commands.py`
stay as files, each reachable only via a recipe. `.github/workflows/ci.yml` and `.github/workflows/helm.yml`
call `just <recipe>` instead of `make <target>` / raw `go`/`python3` commands. `AGENTS.md`, `README.md`,
`docs/signals.md`, `CONTRIBUTING.md` and the generator string literals in `internal/configdoc/main.go`
say `just`, not `make`. `backlog/config.yml`'s `definition_of_done` names `just` recipes. `just check`
passes locally and is exactly what CI's `build-test`/`lint`/`govulncheck`/`grafana`/`secret-hygiene`
jobs enforce collectively.

## 2. The complete justfile

Drop this in at the repo root as `justfile`. Versions (`golangci-lint`, `govulncheck`, `helm-docs`) are
carried over from the current `Makefile` except where noted in §9 (Traps) — align the golangci-lint
pin with CI's `v2.13.2` rather than the Makefile's stale `v2.13.1`.

```just
set shell := ["bash", "-euo", "pipefail", "-c"]

tools_dir := justfile_directory() / ".tools"
export PATH := tools_dir + ":" + env_var('PATH')

golangci_lint_version := "v2.13.2"
govulncheck_version := "v1.3.0"
helm_docs_version := "v1.14.2"

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
commit := `git rev-parse HEAD 2>/dev/null || echo unknown`
build_date := `date -u +%Y-%m-%dT%H:%M:%SZ`
ldflags := "-s -w -X github.com/rknightion/polylens2otel/internal/version.Version=" + version + " -X github.com/rknightion/polylens2otel/internal/version.Commit=" + commit + " -X github.com/rknightion/polylens2otel/internal/version.BuildDate=" + build_date

# show the task surface
default:
    @just --list

# install repo-local dev tooling and wire the pre-commit hook (idempotent)
setup:
    mkdir -p {{tools_dir}}
    test -x {{tools_dir}}/golangci-lint || GOBIN={{tools_dir}} go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@{{golangci_lint_version}}
    test -x {{tools_dir}}/govulncheck || GOBIN={{tools_dir}} go install golang.org/x/vuln/cmd/govulncheck@{{govulncheck_version}}
    test -x {{tools_dir}}/helm-docs || GOBIN={{tools_dir}} go install github.com/norwoodj/helm-docs/cmd/helm-docs@{{helm_docs_version}}
    git config core.hooksPath .githooks

# format Go source in place
[group('check')]
fmt: setup
    golangci-lint fmt

# verify formatting; non-zero if reformatting is needed
[group('check')]
[no-exit-message]
fmt-check: setup
    just --fmt --check
    golangci-lint fmt --diff

# static analysis: go vet + golangci-lint
[group('check')]
[no-exit-message]
lint: setup
    go vet ./...
    golangci-lint run

# dependency vulnerability scan
[group('check')]
[no-exit-message]
audit: setup
    govulncheck ./...

# verify go.mod/go.sum need no changes
[group('check')]
[no-exit-message]
tidy-check:
    go mod tidy -diff

# full default test suite: Go (race) + Grafana asset Python tests
[group('check')]
test filter="":
    go test -race {{ if filter == "" { "./..." } else { "-run " + filter + " ./..." } }}
    cd grafana && python3 -m unittest discover -s tests -t . -q

# scan tracked files for committed secrets and unignored local/sensitive paths
[group('check')]
[no-exit-message]
secret-hygiene:
    ./scripts/check-secret-hygiene.sh

# verify README/docs command blocks resolve to a real recipe or binary
[group('check')]
[no-exit-message]
doc-commands:
    python3 scripts/check_doc_commands.py

# whole-module compile check (catches build errors outside cmd/)
[private]
[no-exit-message]
_compile-check:
    go build ./...

# verify generated docs/dashboards/rules match their sources (drift gate)
[group('gen')]
[no-exit-message]
gen-check:
    go run ./internal/configdoc --check
    cd grafana && python3 build_dashboard.py --check
    cd grafana && python3 build_rules.py --check

# THE GATE: everything a PR must pass
[group('check')]
check: fmt-check lint audit tidy-check gen-check test secret-hygiene doc-commands _compile-check

# compile the release binary to bin/polylens2otel
[group('build')]
build:
    go build -trimpath -ldflags "{{ldflags}}" -o bin/polylens2otel ./cmd/polylens2otel

# build the dev container image (no push)
[group('build')]
image tag="polylens2otel:dev":
    docker build --build-arg VERSION={{version}} --build-arg COMMIT={{commit}} --build-arg BUILD_DATE={{build_date}} -t {{tag}} .

# generate the SPDX SBOM at dist/polylens2otel.spdx.json (requires syft on PATH)
[group('build')]
sbom:
    require('syft')
    mkdir -p dist
    syft dir:. -o spdx-json=dist/polylens2otel.spdx.json

# validate the dependency inventory (go-licenses not wired; prints status only)
[group('build')]
notices:
    go list -m -json all > /dev/null
    @echo "dependency inventory validated; install go-licenses to generate THIRD_PARTY_NOTICES.md"

# regenerate all committed generated artifacts (config docs, Grafana dashboard, alert rules)
[group('gen')]
gen:
    go generate ./internal/configdoc
    cd grafana && python3 build_dashboard.py
    cd grafana && python3 build_rules.py

# regenerate the Helm chart README from values.yaml + README.md.gotmpl
[group('gen')]
docs: setup
    helm-docs --chart-search-root charts

# go.sum/go.mod maintenance: apply `go mod tidy`
[group('dev')]
deps-update:
    go mod tidy

# test coverage report
[group('check')]
coverage:
    go test -race -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out

# remove build/tooling artifacts reproducible by setup + build
[group('build')]
clean:
    rm -rf bin dist coverage.out {{tools_dir}}
```

Notes on the recipes above:

- `fmt-check` composes `just --fmt --check` (formats the justfile itself, §5.10 of the standard) with
  `golangci-lint fmt --diff`, which was live-verified in this task's own analysis: it exits `1` with a
  unified diff when Go source is unformatted and `0` when clean (tested against a throwaway module —
  not the repo — since this analysis was read-only).
- `test` takes the standard optional `filter=""` param. Only the Go leg is filterable; when `filter`
  is set the Grafana Python suite still runs unfiltered (it's a much smaller, fast suite; splitting it
  would add complexity nothing needs).
- `gen-check` is the drift gate using each generator's own `--check` flag (`internal/configdoc`'s
  `main.go` already implements `--check` as "fail if generated files differ"; `grafana/common.py`'s
  `write_or_check` gives `build_dashboard.py --check` / `build_rules.py --check` the same contract).
  It deliberately does **not** also run `gen` — the three generators already support a non-mutating
  check mode natively, so there is no need to generate-then-diff.
- The Grafana Python unittest suite (`grafana/tests`) moved into `test` rather than `gen-check`
  because it's a test suite, not a drift check — matches the `test` recipe's contract.
- `doc-commands` and `secret-hygiene` are both included as `check` dependencies. They are currently
  separate CI jobs / separate `backlog/config.yml` `definition_of_done` lines rather than part of
  `make check` — folding them into `just check` makes local verification a strict superset of what CI
  enforces, which the standard requires (§1: "If `check` does not [cover something CI runs], the
  contract is broken").
- `sbom`'s `require('syft')` line is pseudocode for the real gotchas doc's `require(name)` builtin —
  write it as `syft_bin := require("syft")` as a recipe-local statement, or as the first body line
  `require("syft")` if `just`'s script-line evaluation order accepts a bare `require()` call as a
  statement in this repo's pinned `just` version. If it does not evaluate as a statement, use the
  existing shell guard instead: `command -v syft >/dev/null || { echo "syft is required" >&2; exit 2; }`
  as line 1 of the recipe body (this is what `scripts/sbom.sh` already does — verify locally with
  `just --dry-run sbom` before deleting `scripts/sbom.sh`).

## 3. Makefile disposition

`Makefile` (repo root) — every target below, then `git rm Makefile`.

| Make target | Replacement | Notes |
|---|---|---|
| `build` | `just build` | Same ldflags, same output path. |
| `test` | `just test` | Now also runs the Grafana Python suite (previously only reachable via `make grafana-check`). |
| `vet` | folded into `just lint` | Matches `Makefile`'s own `check: vet test lint …` sequencing — vet is static analysis, belongs with lint per §11 of the standard. |
| `tools` | folded into `just setup` | Same `test -x … || GOBIN=… go install` idempotency pattern. |
| `lint` | `just lint` | Runs `go vet` then `golangci-lint run`. |
| `fmt` | `just fmt` | Same `golangci-lint fmt` invocation. |
| `govulncheck` | `just audit` | Renamed to the fleet's mandatory-vocabulary-adjacent optional name `audit`. |
| `tidy` | `just deps-update` | Mutating `go mod tidy`. |
| `tidy-check` | folded into `just check` (as `tidy-check` dependency) | Non-mutating `go mod tidy -diff`. |
| `regen` | `just gen` | `gen` now also regenerates the Grafana dashboard/rules (`dashboard`/`rules` targets), matching the fleet's "regenerate ALL committed generated artifacts" contract for `gen`. |
| `dashboard` | folded into `just gen` | No longer has its own recipe; `gen` is the one regen entry point. |
| `rules` | folded into `just gen` | Same. |
| `grafana-check` | split: drift checks → `just gen-check`; `python3 -m unittest discover` → `just test` | See §2 notes above. |
| `helm-docs` | `just docs` | Renamed to the fleet's optional vocabulary name `docs`. |
| `notices` | `just notices` | Body absorbed verbatim from `scripts/notices.sh`; script deleted. |
| `sbom` | `just sbom` | Body absorbed verbatim from `scripts/sbom.sh`; script deleted. |
| `coverage` | `just coverage` | Same two-line body. |
| `install-hooks` | folded into `just setup` | `git config core.hooksPath .githooks` is now one of setup's idempotent steps. |
| `docker` | `just image` | Renamed to the fleet's mandatory-vocabulary-adjacent optional name `image`; same three build-args. |
| `check` | `just check` | Expanded — see §2 notes (now also covers `secret-hygiene` and `doc-commands`, previously CI-only / doc-only checks). |

After every row above is ported and `just check` passes, run `git rm Makefile`.

## 4. Script disposition

| Script | Disposition | Replacement | Why |
|---|---|---|---|
| `scripts/notices.sh` | ABSORB, delete | `just notices` (body: `go list -m -json all > /dev/null` then the same echo) | Two lines, no control flow, purely sequential — a thin wrapper per §6. |
| `scripts/sbom.sh` | ABSORB, delete | `just sbom` (body: syft guard, `mkdir -p dist`, `syft dir:. -o spdx-json=dist/polylens2otel.spdx.json`) | Same: a guard plus one tool invocation, no control flow. |
| `scripts/regen-generated.sh` | ABSORB, delete | Its one line (`go generate ./internal/configdoc`) is already inside `just gen` | The script is currently **orphaned** — nothing in `Makefile`, `.github/workflows/`, or `backlog/config.yml` invokes it (verified by grep across the tree). Delete it; do not wire a recipe that calls the file, just inline the one command it contains. |
| `scripts/check-secret-hygiene.sh` | KEEP | `just secret-hygiene` calls it | Has a shell function (`fail_matches`), loops over path lists, and multi-stage conditional logic — non-trivial control flow, §6 KEEP criterion. |
| `scripts/check_doc_commands.py` | KEEP | `just doc-commands` calls it | A real (if small) Python program: parses Markdown, regex-extracts fenced shell blocks, cross-checks against a task-runner's target list. §6 KEEP criterion ("a real program... doc generators, schema renderers, validators"). **Requires a code change, not just a recipe wrapper** — see Trap 1 in §9: it currently parses `Makefile` targets and must be updated to parse the justfile's recipe names instead, or its `make` cross-check silently stops finding anything and the check goes permanently green-by-omission. |
| `.github/scripts/install-syft.sh` | KEEP, untouched, no recipe | N/A — out of scope | Not invoked by a developer or by this repo's own CI; it's passed by *path* as the `pre-cmd` input to the `rknightion/.github/.github/workflows/binaries.yml` reusable workflow from `release-assets.yml` (`pre-cmd: "bash .github/scripts/install-syft.sh"`), which executes it directly on the runner with no `just` involved. Migrating it would break that reusable-workflow contract. |

## 5. CI changes

### `.github/workflows/ci.yml`

Insert the pinned `setup-just` step once per job that needs `just` (every job below except
`goreleaser-snapshot` and `docker-build`, which do not call `just`), immediately after the
`actions/checkout` step and before `actions/setup-go` (order doesn't matter between those two, but
keep it consistent):

```yaml
      - uses: extractions/setup-just@<pin-to-current-fleet-SHA> # v4
        with:
          just-version: '1.58.0'
```

Find the fleet's current pinned SHA for `extractions/setup-just@v4` from another already-migrated
rknightion/m7kni repo's workflow (or the fleet standard's own example) and use the same SHA — do not
invent a new pin.

Per-job edits:

- **`build-test` job** — replace the four `run:` steps:
  - `go.mod tidy check` step: keep the `if [[ -f internal/lensstream/stream.go ]] && rg -q
    'pyroscope-go' cmd internal; then … else …` conditional exactly as-is (this is repo-state-specific
    interim wave logic per Trap 2 in §9, not task-runner logic) but change the `then` branch's body
    from `make tidy-check` to `just tidy-check`.
  - `go vet` step (`run: go vet ./...`) and `go build` step (`run: go build ./...`): these duplicate
    what `just lint` and `just check`'s `_compile-check` dependency already do. Collapse both plus the
    `go test -race` step into one step: `run: just check`. This makes CI call the exact same gate a
    developer runs locally, which is the point of the migration — do this collapse even though it
    changes job granularity from 4 named steps to 1; the job name (`build / vet / test`) can stay as
    given or be reworded to `build / check`, your call, but keep the job's `id`/key `build-test` since
    `ci-success`'s `needs:` list references it by key, not by display name (`needs.*.result` is
    keyed by job id).
- **`lint` job** — this job uses the third-party `golangci/golangci-lint-action`, which is *not* the
  same code path as `just lint` (the action has its own caching, its own binary resolution, and
  pins `version: v2.13.2` directly). Leave this job's `uses:` step untouched per §8 of the standard —
  do not convert a `uses:` into `run: just`. This is deliberately a CI-only superset check
  (equivalent-but-not-identical to `just lint`) — no action needed here, note it in the workflow with
  a comment if you want future readers to know it's intentional.
- **`govulncheck` job** — replace both `run:` steps (`go install golang.org/x/vuln/cmd/govulncheck@v1.3.0`
  then `govulncheck ./...`) with `run: just audit` (which does the equivalent idempotent
  `GOBIN=… go install … || true` via `setup`, then runs `govulncheck ./...`).
- **`goreleaser-snapshot` job** — no `just` involvement; unrelated to this migration.
- **`docker-build` job** — no `just` involvement; unrelated to this migration.
- **`secret-hygiene` job** — replace `run: ./scripts/check-secret-hygiene.sh` with `run: just
  secret-hygiene`.
- **`grafana` job** — replace the `go run ./internal/configdoc --check` step and the `if [[ -d grafana
  ]]; then make grafana-check; else …` step. The configdoc step becomes part of `just gen-check`. Keep
  the conditional shape (it's Trap 3 in §9 — this whole job is gated on a not-yet-landed `grafana/`
  directory in some historical states) but change the `then` branch to `just gen-check` (the drift
  checks) and add a second line running the test-suite half if you want CI parity with local `just
  check`: `just test` already runs `grafana/tests` unconditionally as part of the same `if [[ -d
  grafana ]]` guard — restructure so both `just gen-check` and the grafana-half of `just test` run
  inside that one `if` guard, e.g.:
  ```yaml
      - name: Verify generated configuration documentation and Grafana assets
        run: |
          go run ./internal/configdoc --check
          if [[ -d grafana ]]; then
            just gen-check
            cd grafana && python3 -m unittest discover -s tests -t . -q
          else
            echo "::notice::Grafana artifacts are owned by pending lane L12; the final exact-SHA gate must execute this check."
          fi
      ```
  (Note `just gen-check` already runs the configdoc check too — the explicit `go run
  ./internal/configdoc --check` line above stays separate and unconditional exactly as today, since
  it is not behind the `grafana`-directory guard in the current workflow. Do not fold it into the
  conditional.)
- **`ci-success` job** — do not touch. `needs: [build-test, lint, govulncheck, goreleaser-snapshot,
  docker-build, secret-hygiene, grafana]` stays exactly as-is; job ids are unchanged by every edit
  above.
- Leave `permissions:`, `concurrency:`, all SHA pins, `persist-credentials: false` untouched
  everywhere.

### `.github/workflows/helm.yml`

- **`lint-template` job** — replace the `helm-docs README is in sync` step's `run: | make helm-docs …`
  with:
  ```yaml
      - name: helm-docs README is in sync
        run: |
          just docs
          git diff --exit-code "$CHART_DIR/README.md"
  ```
  This requires the `setup-just` step added to this job too (same pinned block as above), inserted
  after checkout. The `helm lint` / `helm template` steps are unrelated to `just` — leave them.
- **`helm-success` job** — do not touch.

### Workflows explicitly NOT touched

`actionlint.yml`, `codeql.yml`, `dependency-review.yml`, `docker-security.yml`, `ghcr-cleanup.yml`,
`grafana-sync.yml`, `lens-schema-canary.yml`, `publish.yml`, `release-assets.yml`, `release-please.yml`,
`scorecard.yml`, `trigger-docs-sync.yml`, `zizmor.yml` — none of these run Makefile targets or the
absorbed scripts; see §10 for why each is out of scope.

## 6. Docs and agent-contract changes

- `README.md:19-20` — the fenced `sh` block:
  ```
  make build
  make check
  ```
  becomes:
  ```
  just build
  just check
  ```
- `README.md:23` — "`make build` embeds the resolved version…" → "`just build` embeds the resolved
  version…".
- `README.md:24` — "`make docker` passes the same three values…" → "`just image` passes the same
  three values…".
- `README.md:44` — the fenced block's `make docker` line → `just image`.
- `docs/signals.md:44` — "`make dashboard` generates a Grafana dashboard…" → "`just gen` regenerates
  the Grafana dashboard…" (name change: `dashboard` no longer exists as its own recipe, folded into
  `gen` — see §3).
- `docs/signals.md:52` — "`make grafana-check` enforces that coverage…" → "`just gen-check` enforces
  that coverage…".
- `docs/getting-started.md:13` — `make build` → `just build`.
- `CONTRIBUTING.md:3` — "run make check before submitting a change" → "run `just check` before
  submitting a change".
- `internal/configdoc/main.go` — three string literals reference `make regen`:
  - line 118: `fatal(fmt.Errorf("generated configuration drift: %s (run make regen)", name))` →
    `"generated configuration drift: %s (run just gen)"`.
  - line 273: the generated-docs header comment `<!-- Code generated by internal/configdoc; DO NOT
    EDIT. Run `make regen`. -->` → `` `just gen` ``.
  - line 293: the generated-config-template header comment `# Code generated by internal/configdoc;
    DO NOT EDIT. Run `make regen`.` → `` `just gen` ``.
  These are Go source changes (not just doc prose) — after editing, `just gen` must be run once to
  regenerate `docs/env-vars.md` and whatever config template main.go writes, so the new header text
  actually lands in the committed generated file. Do this in the same commit/step as the source edit,
  then `just gen-check` to confirm no drift.
- `AGENTS.md` — replace the "## Commands" section:
  ```markdown
  ## Commands

  - make build — compile the binary
  - make test — race-enabled tests
  - make check — repository green bar
  - make regen — regenerate configuration documentation
  - make dashboard / make rules — regenerate Grafana artifacts
  - make grafana-check — check generated dashboards and alerts
  - make docker — build the container
  ```
  with the fleet-standard "Task interface" section (§9 of the standard):
  ```markdown
  ## Task interface

  This repo's task surface is a `justfile`. Discover it, don't guess it:

      just --list                        # human-readable
      just --dump --dump-format json     # machine-readable
      just --show <recipe>               # what a recipe actually runs

  - `just check` is the full gate and is exactly what CI enforces. It must pass before you commit.
  - Prefer `just <recipe>` over the underlying tool. If you are typing `pytest` or `go test`, you want
    `just test`.
  - Run `just` with stdin from /dev/null. No recipe in this repo is currently marked `[confirm]`, but
    treat any future one that way: stop and ask before running it; never pass `--yes` or
    `JUST_YES=1`.
  - If a task you need does not exist, add a recipe with a `#` doc comment and a `[group(...)]` rather
    than running a bare command.
  ```
  Do NOT paste the recipe list itself into `AGENTS.md` (it rots). `CLAUDE.md` is a one-line pointer
  (`@AGENTS.md`) and needs no edit — verified identical delegation pattern already in place.
- Everywhere else in `AGENTS.md` (task tracking, method, architecture seams, telemetry model,
  non-negotiable traps, the Backlog.md guidelines block) is unrelated to this migration — do not
  touch it.

## 7. `backlog/config.yml`

Current:
```yaml
definition_of_done:
  - "make check (vet, race tests, lint, govulncheck, tidy-check, grafana-check, build)"
  - "make regen (only if the koanf config surface changed; the generated-doc CI job fails on drift)"
  - "python3 scripts/check_doc_commands.py (only if docs/ or README.md changed)"
  - "./scripts/check-secret-hygiene.sh"
```

New:
```yaml
definition_of_done:
  - "just check (fmt-check, lint, audit, tidy-check, gen-check, test, secret-hygiene, doc-commands, build)"
  - "just gen (only if the koanf config surface changed; the generated-doc CI job fails on drift)"
  - "just doc-commands (only if docs/ or README.md changed — also covered by just check)"
  - "just secret-hygiene (also covered by just check)"
```

This file is edited by hand per `AGENTS.md`'s own note ("`backlog/config.yml` is the one exception and
is edited by hand, because list-valued keys cannot be set through `backlog config set`") — this is the
one file in the repo this migration touches directly with a text editor rather than through a CLI.

## 8. Order of work

1. Add `justfile` (§2) at the repo root. Do not touch `Makefile`, scripts, or CI yet.
2. Run `just --fmt --check` — fix formatting if it fails (it shouldn't on a freshly-authored file, but
   confirm).
3. Run `just setup` then `just check` locally. Fix any recipe body that doesn't match this repo's real
   tool output (paths, flag names, working directories) — the bodies above were written from reading
   the Makefile and scripts, not executed end-to-end against this checkout.
4. Edit `internal/configdoc/main.go` (§6) and run `just gen` once to regenerate `docs/env-vars.md` /
   the config template with the new "run just gen" text. Run `just gen-check` to confirm zero drift.
5. Update `scripts/check_doc_commands.py` (Trap 1, §9) to check against justfile recipe names instead
   of Makefile targets. Run `just doc-commands` to confirm it passes.
6. Update `README.md`, `docs/signals.md`, `docs/getting-started.md`, `CONTRIBUTING.md`, `AGENTS.md`
   (§6).
7. Update `backlog/config.yml` by hand (§7).
8. Update `.github/workflows/ci.yml` and `.github/workflows/helm.yml` (§5). Push to a branch or open a
   PR so CI actually runs (this repo's CI trigger is `pull_request` + `push: branches: [main]` — verify
   with the fleet's normal push-to-main-on-own-repos policy whether that means a direct push is fine
   here, or whether to check CI on a throwaway branch first before the direct push).
9. Confirm CI is green with the new `just`-based steps.
10. Only once CI is green: `git rm Makefile scripts/notices.sh scripts/regen-generated.sh
    scripts/sbom.sh`.
11. Final `just check` locally, final CI run, done.

Justfile first and proven locally; CI switched second; deletions last — never delete a file that
something (a doc, a workflow, `check_doc_commands.py`) still references.

## 9. Traps specific to this repo

1. **`scripts/check_doc_commands.py` hardcodes Makefile parsing.** `TARGETS = set(re.findall(r"^([A-Za-z0-9_.-]+):", MAKEFILE, re.MULTILINE))` reads `Makefile` directly and its `main()` special-cases
   `command == "make"` to validate `make <target>` lines in fenced shell blocks in `README.md` and
   `docs/*.md`. After the doc edits in §6 replace every `make …` with `just …`, this script's `make`
   branch will simply never trigger again (false negative, not a crash) unless it's updated to also
   special-case `command == "just"` and parse recipe names out of the justfile (via `just --summary`
   output, or `just --dump --dump-format json` and read the `recipes` keys) instead of/in addition to
   `MAKEFILE`. Do this update before relying on `just doc-commands` to catch a stale command in the
   docs — otherwise the check silently stops checking anything.
2. **The `go.mod tidy check` CI step has interim-wave conditional logic that must NOT be simplified
   away.** It only runs `tidy-check` when `internal/lensstream/stream.go` exists AND `pyroscope-go` is
   grepped into `cmd`/`internal` — this is gating on the state of pending Backlog lanes (referenced as
   "L7/L14"), not on anything to do with `make` vs `just`. Preserve the `if`/`else` shape verbatim,
   only swap `make tidy-check` for `just tidy-check` inside it.
3. **The `grafana` CI job has the same interim-wave pattern**, gated on `[[ -d grafana ]]` and citing
   "pending lane L12". The `grafana/` directory currently exists in this checkout (verified:
   `grafana/build_dashboard.py`, `build_rules.py`, `common.py`, `v2.py`, `waivers.json`, `tests/` are
   all present) — so today the `then` branch runs — but do not delete the `else`/notice branch; another
   lane may be mid-flight when this migration lands.
4. **golangci-lint version drift between `Makefile` (`v2.13.1`) and CI's `golangci-lint-action`
   (`v2.13.2`).** The justfile in §2 pins `v2.13.2` (CI's version) for `just setup`/`just lint`,
   deliberately changing the historical Makefile pin. Confirm `just lint` passes clean at v2.13.2
   before deleting the Makefile — if v2.13.2 surfaces new findings v2.13.1 didn't, fix them or note
   them as a follow-up task; do not silently downgrade the pin to avoid the work.
5. **`golangci-lint fmt --diff` as the `fmt-check` primitive was verified against a throwaway module,
   not this repo**, because this task is read-only analysis. Step 3 of §8 (Order of work) covers
   running `just fmt-check` for real against this checkout before trusting it.
6. **`gen` writes to three different tools' working directories in one recipe** (`go generate` runs
   from the repo root against `./internal/configdoc`; the two Python calls need `cd grafana` first).
   Each `just` recipe line is its own shell (§10 of the standard) — the `cd grafana && python3 …` lines
   above already account for this; do not collapse them into a multi-line shell block or the `cd` will
   not persist to the second Python call.
7. **`golangci-lint` invocations in `fmt`/`fmt-check`/`lint` assume `.tools` is on `PATH`.** The
   justfile's top-level `export PATH := tools_dir + ":" + env_var('PATH')` makes this automatic for
   every recipe, but only once `just setup` has actually populated `{{tools_dir}}` — hence `fmt`,
   `fmt-check`, `lint`, `audit`, and `docs` all list `setup` as a recipe dependency in §2. Do not strip
   those dependencies as "redundant" — a fresh checkout with an empty `.tools/` would otherwise fail
   with "command not found".
8. **`.tools/`, `bin/`, `dist/` are already gitignored** (`.gitignore:50-52`) and already excluded from
   the Docker build context (`.dockerignore`) — no change needed there, but don't second-guess it
   into adding a redundant ignore entry for `justfile`-produced artifacts; there are none new.
9. **`sbom` needs `syft` on `PATH` locally** (CI installs it via `.github/scripts/install-syft.sh`,
   which is out of scope per §4 — that script is release-workflow-only). A developer running `just
   sbom` locally without `syft` installed should get a clear failure; use `require("syft")` if the
   pinned `just` version's `require()` works as a bare statement in a recipe body, otherwise fall back
   to the shell guard already in `scripts/sbom.sh` (see the note under §2's recipe list).

## 10. Out of scope

- **Every KEEP script**: `scripts/check-secret-hygiene.sh`, `scripts/check_doc_commands.py` (content
  changes to make it just-aware are IN scope per Trap 1 — but rewriting its architecture, or turning
  it into a Go program, is not), `.github/scripts/install-syft.sh`.
- **GitHub-native workflows, untouched**: `actionlint.yml`, `codeql.yml`, `dependency-review.yml`,
  `release-please.yml`, `scorecard.yml`, `zizmor.yml`.
- **`docker-security.yml`** — hadolint + Trivy filesystem scans via third-party actions, no Make/script
  logic.
- **`ghcr-cleanup.yml`** — not read in this analysis pass but named in the fleet's standard out-of-scope
  list for container-publish-adjacent workflows; do not touch without first confirming it contains no
  Make/script logic.
- **`grafana-sync.yml`** — copies already-generated `dashboards/*.json` into a GitSync repo and commits;
  no build/test/lint logic, nothing to migrate.
- **`lens-schema-canary.yml`** — runs two tagged `go test` invocations as a security canary against
  historical wrong-type regressions. This duplicates the *shape* of `just test` but is deliberately
  narrow, single-purpose, and unrelated to the standard dev loop; leave it calling `go test` directly
  rather than inventing a `just lens-canary` recipe nothing else would use.
- **`publish.yml`, `release-assets.yml`** — container-publish and release-binary reusable-workflow
  callers; explicitly named as out of scope by the fleet standard (§8: "Never convert a `uses:` into
  `run: just`"; both are wrappers around `uses: rknightion/.github/.github/workflows/…`).
- **`trigger-docs-sync.yml`** — docs-sync dispatcher, unrelated.
- **Renaming or restructuring `grafana/` Python modules** (`common.py`, `v2.py`, `waivers.json`) — only
  their two CLI entry points (`build_dashboard.py`, `build_rules.py`) get recipe wrappers; their
  internals are untouched.
- **Changing `golangci-lint` findings/config** beyond what Trap 4 requires to get a clean `just lint`
  at the new pinned version.
- **`.goreleaser.yaml`, `Dockerfile`, `charts/polylens2otel/**`** — read for context (build-arg names,
  chart layout) but not edited by this migration beyond the `helm.yml` CI step change in §5.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A top-level justfile exists defining all seven mandatory recipes (default, setup, fmt, fmt-check, lint, test, check) plus gen, gen-check, build, image, sbom, notices, docs, deps-update, coverage, clean
- [ ] #2 just check passes locally on this checkout and is a strict superset of what ci.yml's build-test, lint, govulncheck, secret-hygiene and grafana jobs enforce
- [ ] #3 just --fmt --check passes
- [ ] #4 just --list shows a # doc comment and [group(...)] for every public recipe
- [ ] #5 Makefile is deleted (git rm)
- [ ] #6 scripts/notices.sh, scripts/regen-generated.sh and scripts/sbom.sh are deleted; their logic is absorbed into just notices, just gen and just sbom
- [ ] #7 scripts/check-secret-hygiene.sh and scripts/check_doc_commands.py remain as files and are each reachable only via a just recipe (secret-hygiene, doc-commands)
- [ ] #8 scripts/check_doc_commands.py is updated to validate just recipe names instead of Makefile targets
- [ ] #9 ci.yml and helm.yml call just recipes (via a pinned extractions/setup-just step) instead of make targets or raw go/python3 build-test-lint commands, and the ci-success/helm-success aggregators still gate on the same job names
- [ ] #10 README.md, docs/signals.md, docs/getting-started.md, CONTRIBUTING.md, AGENTS.md and internal/configdoc/main.go's generated-file header strings reference just instead of make
- [ ] #11 backlog/config.yml's definition_of_done names just recipes instead of make targets and script paths
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 make check (vet, race tests, lint, govulncheck, tidy-check, grafana-check, build)
- [ ] #2 make regen (only if the koanf config surface changed; the generated-doc CI job fails on drift)
- [ ] #3 python3 scripts/check_doc_commands.py (only if docs/ or README.md changed)
- [ ] #4 ./scripts/check-secret-hygiene.sh
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Inspect the current task surface, hooks, workflow callers, and generated-doc command validator against the binding migration contract.
2. Add and validate the justfile; update the validator and generated-doc source before migrating references.
3. Replace Makefile/script/CI/doc task paths, preserving reusable calls and repointing the tracked pre-commit hook without changing local Git config.
4. Run formatting, task-surface, targeted workflow/config checks, just check, hook verification, review, push main, and confirm the final-SHA CI run before finalizing the task.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Parked before commit by the mandatory CodeRabbit gate. The staged migration has objective local evidence: isolated `just check` passed (format, vet, golangci-lint v2.13.2, govulncheck, tidy drift, generated docs/dashboard/rules, Go race tests, two task-surface tests, secret hygiene, documentation commands, and build); `just --dump --dump-format json`, `just --fmt --check`, `just docs` plus chart README diff, manual pre-commit hook execution, and `actionlint` on ci.yml/helm.yml passed.

Review blocker: `coderabbit review --agent` emitted only startup/status records through `{"type":"status","phase":"analyzing","status":"summarizing"}` and never a `complete` line. A bounded wait observed multiple lingering `coderabbit review --agent` processes and produced no findings or terminal review output. Per the mandatory review gate, the named-path staged WIP remains intentionally uncommitted.

Resume boundary: after the CodeRabbit service/process contention clears and no inherited review process remains, rerun `coderabbit review --agent`, require and read its `complete` result, address applicable findings, rerun the isolated just gate, then commit/push and verify CI at the final SHA. The remaining Makefile and three absorbable scripts must stay until that CI proof is available.
<!-- SECTION:NOTES:END -->

## Comments

<!-- COMMENTS:BEGIN -->
author: campaign-ordering
created: 2026-08-29 09:18
---
## Fleet ordering — WAVE 2. Starts after the Wave 0 pilot (`sf2loki` / SFL-0073) and the Wave 1 hubs land.

Within Wave 2 the order is free — these repos do not depend on each other. Batching by language is worthwhile so one lane reuses its Makefile-to-recipe mapping across similar repos.

Do not start before the pilot reports. The standard may be amended off the back of it, and picking this up early risks coding against a superseded seam.

**Provisioning `just` in CI.** Which mechanism depends on the runner, and the two must not be mixed:

| Runner | Mechanism |
| --- | --- |
| `arc-arm64` (m7kni self-hosted) | `just` is **baked into the runner image** by `m7kni/ci-tools` (`runner-image/Dockerfile`, `ARG JUST_VERSION`). Do **not** add `extractions/setup-just`, and delete the step if this repo already has one — it installs a second `just` earlier on `PATH` and turns the image pin into a lie. |
| GitHub-hosted (all `rknightion` repos) | `extractions/setup-just`, SHA-pinned, with an explicit `just-version:`. |

Both sides currently sit on **1.58.0** and are Renovate-managed. `ci-tools`' `Tool version drift` workflow fails if the Dockerfile `ARG` and the published image ever disagree, and lists any repo still carrying a second pin.

**While you are in the workflow files, check the hub pin.** On 2026-08-29 Renovate was unfrozen for `rknightion/.github` in `m7kni/renovate-config` — it had been `enabled: false` on the mistaken belief that callers tracked `@main`, which froze the fleet across 19 different hub SHAs (v1.3.1 June → v1.9.7 August) so that no hub fix ever propagated. Bumps now arrive as one grouped, CI-gated, automerged PR per repo. **A `uses:` whose comment is not a real `# vX.Y.Z` still cannot be bumped** (it resolves to a digest-only update, which the fleet rules disable) — if you find one, repair the comment as part of this task.
---

author: campaign-ordering
created: 2026-08-29 10:43
---
## Standard amendment — `ci` is the sanctioned superset of `check` (RATIFIED)

This supersedes the frozen wording *"`check` is the complete local gate and reproduces every CI job that can run off a GitHub runner"*, which several lanes could not honour without making the pre-commit gate depend on a Docker daemon.

**The definitions now are:**

- **`check`** — everything that runs with **only the language toolchain installed**. This is the pre-commit gate. A leg that runs on a bare toolchain belongs here *however long it takes*.
- **`ci`** — `check` plus the legs CI gates that need a **Docker daemon, a service container, or cross-compilation**, and nothing else. Written as `ci: check <heavy legs>`.

**Every leg you put in `ci` must carry a comment naming which of those three it needs.** That comment is the guard: without it `ci` becomes the bin for anything slow or awkward, `check` quietly stops meaning much, and the fleet is back to a per-repo gate.

Eleven of the 42 lanes arrived at this shape independently before it was ratified, which is why it won.

**If this repo has no such legs, it has no `ci` recipe at all** and `check` is the whole gate. Do not add an empty one.
---

author: campaign-ordering
created: 2026-08-29 10:57
---
## Fleet alignment — the 2otel family converges on one CI shape

These seven Go repos are near-identical applications and had drifted into **two naming dialects and materially different coverage**. The migration rewrites every `run:` block anyway, so converge them in the same change rather than preserving the drift in new clothes.

**Canonical job names** — used by tailscale2otel, graph2otel, polylens2otel and rfc6035-2otel, so this is the majority convention, not an invention:

`build-test` · `lint` · `govulncheck` · `goreleaser-snapshot` · `docker-build` · `coverage` · `ci-success`

`opnsense2otel` and `transceiver-exporter` currently use a second dialect — `tests`, `race`, `docker-build-verify`. Rename to the canonical set as part of this task.

**`ci-success` is the only check the branch ruleset gates**, so jobs can be renamed or merged freely *provided* `ci-success`'s `needs:` list is updated in the same commit. Never rename `ci-success` itself.

**Required gates, and where each lives after the migration:**

| Gate | Recipe | Note |
| --- | --- | --- |
| build + test + `-race` | `just test` | `-race` belongs in the standard test run |
| golangci-lint | `just lint` | needs a `.golangci.yml`, schema v2 |
| **gosec** | `just lint` | **a golangci-lint linter, NOT a separate job** — enable it in `.golangci.yml`. Four of the seven already do it this way; a standalone gosec job would be a third dialect |
| govulncheck | `just vuln` | pinned `golang.org/x/vuln/cmd/govulncheck@v1.3.0`, matching the family |
| goreleaser snapshot | `just snapshot` | cross-compile ⇒ belongs in `ci`, not `check` |
| container build | `just image` | needs a Docker daemon ⇒ belongs in `ci`, not `check` |

**Already done for you (2026-08-29):** `govulncheck` was added to `opnsense2otel`, `transceiver-exporter` and `codexlb2otel` ahead of the migration, because those three had no dependency vulnerability scanning at all. Convert those jobs to `just vuln` like any other; do not re-add them.

**Still missing, fix as part of this task:**

- `opnsense2otel` — has `.golangci.yml` but **`gosec` is not enabled** in it.
- `transceiver-exporter` — **no `.golangci.yml` at all**, and no `-race` in its test job.
- `codexlb2otel` — no `.golangci.yml`, no `-race`, no container build, and **no `ci-success` job and no branch ruleset**, so nothing gates its CI. Adding an aggregator is the right fix but is a separate decision; raise it rather than assuming.

**One known trap:** the `govulncheck@v1.3.0` pins are invisible to Renovate — `go install pkg@version` inside a `run:` block matches no manager. All five are four minor versions behind (current is v1.7.0). Once the version moves into the justfile as a `# renovate:`-annotated `:=` assignment, it becomes managed. That is a real benefit of this migration, not incidental.
---

author: campaign-ordering
created: 2026-08-29 11:20
---
## Correction — moving a pin into the justfile does NOT make it Renovate-managed

The 2otel alignment comment above ends with a claim that needs narrowing. It says the `govulncheck@v1.3.0` pins become managed "once the version moves into the justfile as a `# renovate:`-annotated `:=` assignment". The conditional in that sentence is doing real work, and **the first completed migration did not satisfy it**.

Verified on `tailscale2otel` at `origin/main` after TSO-0025 closed:

- `justfile:21` — `govulncheck_version := "v1.3.0"`, with **no `# renovate:` annotation** above it.
- `renovate.json` — **no justfile matcher at all**: no `customManagers` entry, nothing pointing at `/^justfile$/`.
- `justfile:17` — a comment stating the pin *tracks* the `go install` line in the CI jobs, so the workflow is still the source of truth.

The version relocated and nothing else changed. It is exactly as invisible to Renovate as it was in the `run:` block, and still four minors behind (`v1.3.0` against `v1.7.0`).

**Two things are required, and neither is implied by "move the pin into the justfile":**

1. The `:=` assignment carries a `# renovate: datasource=… depName=…` annotation directly above it.
2. `renovate.json` points a custom manager at the justfile — the `customManagers:dockerfileVersions` preset does **not** cover justfiles; that one only matches Dockerfiles and Containerfiles.

Treat "the pin is now managed" as **false unless you have done both and checked**. Do not record it as a benefit of this migration in a final summary without verifying `renovate.json` yourself.

Credit: caught by the `tailscale2otel` lane on its closeout, against the claim as originally written here.
---
<!-- COMMENTS:END -->
