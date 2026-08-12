# Backend verification

This page records retrieval proof from the configured Grafana Cloud backends.
It is deliberately separate from exporter configuration: a configured exporter
or a successful write does not prove that data can be queried back.

## Traces

On 2026-08-12, a 24-hour Tempo search for
`resource.service.name = "polylens2otel"` returned this trace:

```json
{
  "traceID": "5699f6f660f2df338bbeb3b21dfc638c",
  "rootServiceName": "polylens2otel",
  "rootTraceName": "collector.phone.lines",
  "durationMs": 3829
}
```

Fetching that trace by ID returned `service.version` `0.1.0`, one
`collector.phone.lines` parent span, and twelve `http.client.request` child
spans. This proves an emitted trace was stored and retrieved; it is not inferred
from exporter logs.

## Profiles

On 2026-08-12, a 24-hour Pyroscope query for
`{service_name="polylens2otel"}` and profile type
`process_cpu:cpu:nanoseconds:cpu:nanoseconds` returned a non-empty flame graph:

```json
{
  "total": "27980000000",
  "maxSelf": "5650000000"
}
```

The returned frames included
`github.com/rknightion/polylens2otel/internal/collector.(*Scheduler).loop` and
`github.com/grafana/pyroscope-go.(*Session).takeSnapshots`. A profile exemplar
for the same selector reported `service_name=polylens2otel` and Go runtime
`go1.26.5`. This proves a profile was stored and queried back.
