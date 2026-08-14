---
id: PLO-0002
title: >-
  Receive phone-pushed events via apps.telNotification (registration state, call
  events)
status: To Do
assignee: []
created_date: '2026-08-14 17:01'
updated_date: '2026-08-14 17:02'
labels: []
dependencies: []
ordinal: 2000
---

## Description

<!-- SECTION:DESCRIPTION:BEGIN -->
Add a receiver for the Poly handset's own outbound event webhook, apps.telNotification.*. The phone POSTs to a configured URL on call and registration events. It is a push source and is currently unused: apps.telNotification.URL reads empty with Source: default on both handsets, so nothing has ever been configured.

Why this matters more than it looks: SIP registration state is the strongest signal the handsets expose, and the earlier reference work assumed it had to be polled off each handset's REST API. It does not — apps.telNotification.lineRegistrationEvent with .onlyStateChange pushes it. The 2026-08-11 incident, where both phones silently unregistered for hours after a config wipe, is exactly what this catches, and it catches it at the moment of transition rather than at the next poll. It also gives call events including caller ID without involving the upstream carrier or exposing anything publicly, which is the deciding advantage over a carrier-side syslog feed.

Events verified present in the live 8.6.0 parameter catalogue (getDeviceParametersExtended, 5047 slots), each an independent on/off: incomingEvent, outgoingEvent, offhookEvent, onhookEvent, callStateChangeEvent, lineRegistrationEvent (plus .onlyStateChange and .maxRandomInterval), muteUnmuteEvent, networkUpEvent, dialplanEvent, sipMsgEvent, userLogInOutEvent, and the appInitializationEvent / taInitializationEvent / uiInitializationEvent boot stages. Also present: an indexed apps.telNotification.NUM_REPLACE_1.URL, likely per-registration targets, and apps.telNotification.period.

Scope: an HTTP receiver with configurable listening address and port, disabled by default. Registration transitions are the load-bearing signal; call events are logs with a small set of bounded metrics (call counts by direction and result). Keep caller ID out of metric labels — it is unbounded and personal data, so logs only. Identify senders by source address, mirroring the senders config pattern already used in rfc6035-2otel, so an unmatched source collapses to one unknown value rather than inflating cardinality.

Out of scope: driving the phone. There is no remote-dial API on the Edge E350 — every callctrl/* verb (dial, endCall, answerCall, sendDTMF) returns 404, unlike the older VVX line, so this is receive-only. apps.restapi.sipNotify.enabled may be the intended control path and is unexplored. apps.push.* (server to phone) and apps.statePolling.* are separate surfaces and separate tasks.
<!-- SECTION:DESCRIPTION:END -->

## Acceptance Criteria
<!-- AC:BEGIN -->
- [ ] #1 A real telNotification POST is captured from a live handset and its format documented before any parser is written
- [ ] #2 Receiver implemented, off by default, listening address and port configurable
- [ ] #3 Registration up and down exported as OTLP with sender identification
- [ ] #4 Call events exported as logs, with no unbounded value in any metric label
- [ ] #5 The firewall path for the IOT-VLAN handset is confirmed working, not just the LAN one
- [ ] #6 Documented in docs/ alongside the existing signal docs
- [ ] #7 A registration-loss alert is demonstrated end to end against the live Grafana Cloud backend
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
## Traps to settle before writing code (carried from former GitHub #66)

**The payload format is not yet known.** Poly's telNotification has historically POSTed XML, but that is unverified for 8.6.0 on the Edge E350. Capture a real POST against a throwaway listener before writing any parser. Building against an assumed schema is the exact mistake rfc6035-2otel is carrying as a known caveat — its fixtures are RFC examples rather than a wire capture.

**There are no authentication parameters** for telNotification, unlike apps.statePolling.* which has username and password. Credentials would have to be embedded in the URL, or the listener bound to a trusted segment. Decide before enabling anything.

**The IOT-VLAN handset needs a firewall rule at a sequence below 336**, or IOT-to-LAN traffic is silently dropped by Private Network Denial. A rule added above 336 is shadowed and dead — this has already happened once on this network. Proving the path for the LAN handset proves nothing about the other one, which is why it is a separate acceptance criterion.

**Enabling every event at once will be noisy.** Start with lineRegistrationEvent plus .onlyStateChange, then add call events.
<!-- SECTION:NOTES:END -->
