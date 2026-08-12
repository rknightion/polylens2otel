package collector

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
)

type Scheduler struct {
	emitter telemetry.Emitter
	logger  *slog.Logger
}

func NewScheduler(e telemetry.Emitter) *Scheduler {
	return &Scheduler{emitter: e, logger: slog.Default()}
}
func (s *Scheduler) Run(ctx context.Context, r *Registry) error {
	var wg sync.WaitGroup
	for _, c := range r.Entries() {
		wg.Add(1)
		go func(c Collector) { defer wg.Done(); s.loop(ctx, c) }(c)
	}
	wg.Wait()
	return ctx.Err()
}
func (s *Scheduler) loop(ctx context.Context, c Collector) {
	_ = s.RunOnce(ctx, c)
	t := time.NewTicker(c.Interval())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.RunOnce(ctx, c)
		}
	}
}
func (s *Scheduler) RunOnce(ctx context.Context, c Collector) (err error) {
	started := time.Now()
	attrs := []telemetry.Attr{{Key: semconv.AttrCollectorID, Value: c.ID()}}
	spanCtx, end := s.emitter.StartSpan(ctx, "collector."+c.ID(), attrs...)
	success := 0.0
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("collector %s panic: %v", c.ID(), v)
		}
		if err == nil {
			success = 1
		}
		_ = s.emitter.Gauge(ctx, semconv.MetricCollectorDuration, time.Since(started).Seconds(), attrs...)
		_ = s.emitter.Gauge(ctx, semconv.MetricCollectorAvailability, success, attrs...)
		_ = s.emitter.Gauge(ctx, semconv.MetricCollectorExpectedInterval, c.Interval().Seconds(), attrs...)
		end(err)
		if err != nil {
			s.logger.Warn("collector run failed", "collector", c.ID(), "error", err)
		}
	}()
	return c.Collect(spanCtx, s.emitter)
}
