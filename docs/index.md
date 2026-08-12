# polylens2otel

`polylens2otel` turns the small, useful part of Poly Lens and Poly Edge phone management data into OpenTelemetry signals. It polls Lens and phones, keeps an edge-triggered Lens subscription open for faster connection updates, and pushes everything over OTLP.

There is no listener to expose and no control-plane write path. That is deliberate: an observability process should not be able to change a phone policy.

Live retrieval evidence for metrics, logs, traces, and profiles is kept in the
[backend verification record](verification.md).

Start with [Getting started](getting-started.md). The [Signals](signals.md) page lists the emitted data and its Prometheus-normalized names.
