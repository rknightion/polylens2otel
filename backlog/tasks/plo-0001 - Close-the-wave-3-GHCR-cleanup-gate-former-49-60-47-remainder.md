---
id: PLO-0001
title: 'Close the wave-3 GHCR cleanup gate (former #49/#60/#47 remainder)'
status: Parked
assignee: []
created_date: '2026-08-14 17:01'
updated_date: '2026-08-14 17:01'
labels: []
dependencies: []
ordinal: 1000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Complete the two wave-3 items that were left truthfully partial because they are blocked on a time condition rather than on work: the guarded GHCR cleanup needs a positive candidate to prove its filter, and the ten-item integrated gate cannot claim a full pass until it does. Items 1-8 of the gate PASSED at final head ad36b32 and are recorded in the closed-issue history doc; only items 9 and 10 remain.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A dry run positively names at least one candidate and every named candidate is verified to still carry zero tags
- [ ] #2 Every protected tag still resolves after the run: release manifests, semver, latest, buildcache-*, and main-<sha>
- [ ] #3 Scheduled deletion is enabled only if the positive candidate list is clean, and no existing guard is weakened to achieve it
- [ ] #4 Gate item 10 is dispositioned honestly rather than forced: the historical manual closures of former #50 and #59 make a strict all-issues-closed-by-commit claim false, and Backlog.md replaces that mechanism entirely
<!-- AC:END -->

## Definition of Done
<!-- DOD:BEGIN -->
- [ ] #1 make check (vet, race tests, lint, govulncheck, tidy-check, grafana-check, build)
- [ ] #2 make regen (only if the koanf config surface changed; the generated-doc CI job fails on drift)
- [ ] #3 python3 scripts/check_doc_commands.py (only if docs/ or README.md changed)
- [ ] #4 ./scripts/check-secret-hygiene.sh
<!-- DOD:END -->

## Implementation Notes

<!-- SECTION:NOTES:BEGIN -->
## Resume boundary (carried from former GitHub #49, #60 and parent #47)

**Do not attempt this before 2026-09-11T12:42:58Z.** Live GHCR inventory at the wave-3 close was 356 total version objects, 291 untagged, and **zero** older than 30 days; the oldest untagged object was created 2026-08-12T12:42:58Z, so under the frozen strictly-older-than-30-days rule the first possible eligibility is 30 days after that. Running earlier just reproduces the same empty result.

**Why it is Parked rather than To Do.** It was attempted twice. Dry run 31639113101 failed before listing because the Octokit pagination route was called with the wrong signature; `7642e48` fixed that, and dry runs 31639166675 and 31642040481 then both succeeded with deletion disabled and reported `Dry run: 0 untagged manifests older than 30 days would be deleted.` A green run over an empty candidate set is not proof the candidate filter works — that is the false-pass-by-absence rule, and it is why the lane was left partial instead of counted as a pass.

**Exact sequence at or after the eligibility timestamp:**

1. Run the GHCR cleanup workflow in its default dry-run mode.
2. Verify every positively listed candidate still carries zero tags.
3. Recheck the protected manifests and tags — release manifests, semver, `latest`, `buildcache-*` and `main-<sha>`. `main-<sha>` retention is load-bearing because the deployment host follows `:main`.
4. Enable scheduled deletion **only** if the positive list is clean, and without weakening any guard.

**Gate item 10 needs an honest disposition, not a pass.** The original criterion was "every lane issue is closed by its completing commit". That was already historically false — former #59 was external-only and former #50 carried literal \n characters before its `Closes` footer, so both were closed by hand. Under Backlog.md the mechanism no longer exists at all: status is set by an explicit `backlog task edit`. Record that and move on rather than trying to satisfy a criterion whose subject is gone.

Items 1-8 of the ten-item gate PASSED at final head `ad36b32` and their evidence is in the *Closed GitHub issues (pre-Backlog history record)* doc; they do not need re-running unless `main` has moved materially.
<!-- SECTION:NOTES:END -->
