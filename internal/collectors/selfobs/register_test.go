package selfobs

import (
	"context"
	"testing"
	"time"

	"github.com/rknightion/polylens2otel/internal/collector"
	"github.com/rknightion/polylens2otel/internal/config"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetrytest"
	"github.com/rknightion/polylens2otel/internal/version"
)

func TestRegisterBuildInfoCollector(t *testing.T) {
	cfg := config.Default()
	registry := collector.NewRegistry()
	Register(collector.Deps{Config: &cfg, Registry: registry})

	entries := registry.Entries()
	if len(entries) != 1 {
		t.Fatalf("registered collectors = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.ID() != "selfobs.internal" {
		t.Fatalf("collector ID = %q, want selfobs.internal", entry.ID())
	}
	if entry.Interval() != time.Minute {
		t.Fatalf("collector interval = %s, want %s", entry.Interval(), time.Minute)
	}

	recorder := telemetrytest.New()
	if err := collector.NewScheduler(recorder.Emitter()).RunOnce(context.Background(), entry); err != nil {
		t.Fatalf("run self-observability collector: %v", err)
	}
	attrs := map[string]string{
		semconv.AttrVersion: version.Version,
		semconv.AttrCommit:  version.Commit,
		semconv.AttrDate:    version.BuildDate,
	}
	if !recorder.HasMetric(semconv.MetricBuildInfo, attrs, 1) {
		t.Fatal("missing build_info metric")
	}
	collectorAttrs := map[string]string{semconv.AttrCollectorID: "selfobs.internal"}
	for _, name := range []string{
		semconv.MetricCollectorDuration,
		semconv.MetricCollectorAvailability,
		semconv.MetricCollectorExpectedInterval,
	} {
		if !hasMetricWithAttrs(recorder, name, collectorAttrs) {
			t.Fatalf("missing scheduler-owned metric %s", name)
		}
	}
}

func hasMetricWithAttrs(r *telemetrytest.Recorder, name string, attrs map[string]string) bool {
	for _, metric := range r.Metrics() {
		if metric.Name != name {
			continue
		}
		matched := true
		for key, value := range attrs {
			if metric.Attrs[key] != value {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
