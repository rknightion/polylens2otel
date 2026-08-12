package collector

import (
	"context"
	"testing"
	"time"

	"github.com/rknightion/polylens2otel/internal/telemetry"
	"github.com/rknightion/polylens2otel/internal/telemetrytest"
)

type panicCollector struct{}

func (panicCollector) ID() string              { return "panic" }
func (panicCollector) Interval() time.Duration { return time.Minute }
func (panicCollector) Collect(context.Context, telemetry.Emitter) error {
	panic("boom")
}

func TestRunOnceRecoversPanicsAndRecordsAvailability(t *testing.T) {
	r := telemetrytest.New()
	s := NewScheduler(r.Emitter())
	if err := s.RunOnce(context.Background(), panicCollector{}); err == nil {
		t.Fatal("panic was not surfaced as an error")
	}
	if !r.HasMetric("polylens2otel.collector.availability", map[string]string{"collector.id": "panic"}, 0) {
		t.Fatal("missing failed availability point")
	}
}
