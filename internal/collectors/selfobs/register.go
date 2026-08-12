package selfobs

import (
	"context"
	"time"

	"github.com/rknightion/polylens2otel/internal/collector"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
	"github.com/rknightion/polylens2otel/internal/version"
)

const collectorID = "selfobs.internal"

type buildInfoCollector struct{ interval time.Duration }

func Register(d collector.Deps) {
	if d.Registry == nil {
		return
	}
	interval := time.Minute
	if d.Config != nil && d.Config.Collectors.SelfObs > 0 {
		interval = d.Config.Collectors.SelfObs
	}
	d.Registry.Register(buildInfoCollector{interval: interval})
}

func (c buildInfoCollector) ID() string              { return collectorID }
func (c buildInfoCollector) Interval() time.Duration { return c.interval }
func (buildInfoCollector) Collect(ctx context.Context, emitter telemetry.Emitter) error {
	return emitter.Gauge(ctx, semconv.MetricBuildInfo, 1,
		telemetry.Attr{Key: semconv.AttrVersion, Value: version.Version},
		telemetry.Attr{Key: semconv.AttrCommit, Value: version.Commit},
		telemetry.Attr{Key: semconv.AttrDate, Value: version.BuildDate},
	)
}
