---
id: PLO-0003
title: Upgrade Go toolchain to 1.27
status: In Progress
assignee: []
created_date: '2026-08-23 19:06'
updated_date: '2026-08-23 20:21'
labels: []
dependencies: []
ordinal: 3000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Adopt Go 1.27 consistently across the application, nested modules, build images, CI configuration, setup automation, and version-specific contributor documentation.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [x] #1 All active Go module and toolchain pins require Go 1.27
- [x] #2 Build images, CI jobs, setup automation, and current documentation agree with the Go 1.27 requirement
- [x] #3 The repository green-bar validation passes under Go 1.27
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [x] #1 make check (vet, race tests, lint, govulncheck, tidy-check, grafana-check, build)
- [x] #2 make regen (only if the koanf config surface changed; the generated-doc CI job fails on drift)
- [x] #3 python3 scripts/check_doc_commands.py (only if docs/ or README.md changed)
- [x] #4 ./scripts/check-secret-hygiene.sh
<!-- DOD:END -->

## Implementation Plan

<!-- SECTION:PLAN:BEGIN -->
1. Inventory every active Go version pin, including nested modules and container or CI toolchains. 2. Update the pins and version-specific documentation to Go 1.27.0 without changing historical records or fixtures. 3. Run the repository-defined validation gate, review the diff, commit to main, push, and confirm hosted CI.
<!-- SECTION:PLAN:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
Local Go 1.27.0 make check passed: vet, race tests, lint, govulncheck, tidy, Grafana checks, and build. Documentation command validation checked 19 commands across 10 files, and secret hygiene passed. No koanf config surface changed, so regeneration was not required. CodeRabbit was skipped because only declarative module, container, and current documentation pins changed.

Exact-head CI run 32662554010 exposed two stale tool-distribution pins under Go 1.27: the v2.12.2 release binary was built with Go 1.26, and govulncheck v1.1.4 panicked in its older SSA builder. CI now compiles golangci-lint v2.12.2 with install-mode goinstall under the job's Go 1.27 toolchain and pins govulncheck v1.3.0. A fresh v1.3.0 install reported no vulnerabilities and actionlint accepted the workflow. The failed run is retained as before-fix evidence.

The lint repair was advanced from v2.12.2 to current v2.13.1 after Linux-target analysis confirmed v2.12.2's embedded analyzers also panic on Go 1.27 syntax when built from source. CI still uses goinstall so the linter is compiled by the job's Go 1.27 toolchain. Linux-target v2.13.1 lint passed with 0 issues; govulncheck v1.3.0 again reported no vulnerabilities.
<!-- SECTION:NOTES:END -->
