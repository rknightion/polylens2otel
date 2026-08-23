---
id: PLO-0003
title: Upgrade Go toolchain to 1.27
status: In Progress
assignee: []
created_date: '2026-08-23 19:06'
updated_date: '2026-08-23 19:49'
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
<!-- SECTION:NOTES:END -->
