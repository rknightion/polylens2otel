package telemetrytest

import (
	"context"
	"sync"
	"time"

	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
)

type Metric struct {
	Name  string
	Value float64
	Attrs map[string]string
}
type Log struct {
	Event, Body string
	Timestamp   time.Time
	Attrs       map[string]string
}
type store struct {
	mu      sync.Mutex
	metrics []Metric
	logs    []Log
}
type Recorder struct {
	s    *store
	base []telemetry.Attr
}

func New() *Recorder                           { return &Recorder{s: &store{}} }
func (r *Recorder) Emitter() telemetry.Emitter { return r }
func (r *Recorder) attrs(extra []telemetry.Attr) map[string]string {
	m := map[string]string{}
	for _, a := range append(append([]telemetry.Attr(nil), r.base...), extra...) {
		m[a.Key] = a.Value
	}
	return m
}
func (r *Recorder) Gauge(_ context.Context, n string, v float64, a ...telemetry.Attr) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.metrics = append(r.s.metrics, Metric{n, v, r.attrs(a)})
	return nil
}
func (r *Recorder) Counter(c context.Context, n string, v float64, a ...telemetry.Attr) error {
	return r.Gauge(c, n, v, a...)
}
func (r *Recorder) LogEvent(_ context.Context, e, b string, t time.Time, a ...telemetry.Attr) error {
	if t.IsZero() {
		return telemetry.ErrMissingTimestamp
	}
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.logs = append(r.s.logs, Log{e, b, t, r.attrs(a)})
	return nil
}
func (r *Recorder) StartSpan(ctx context.Context, _ string, _ ...telemetry.Attr) (context.Context, func(error)) {
	return ctx, func(error) {}
}
func (r *Recorder) WithTenant(id string) telemetry.Emitter {
	c := *r
	c.base = append(append([]telemetry.Attr(nil), r.base...), telemetry.Attr{Key: semconv.AttrTenantID, Value: id})
	return &c
}
func (r *Recorder) WithDevice(d telemetry.Device) telemetry.Emitter {
	c := *r
	c.base = append([]telemetry.Attr(nil), r.base...)
	for _, a := range []telemetry.Attr{
		{Key: semconv.AttrDeviceID, Value: d.ID},
		{Key: semconv.AttrDeviceName, Value: d.Name},
		{Key: semconv.AttrDeviceMAC, Value: d.MAC},
		{Key: semconv.AttrDeviceModel, Value: d.Model},
		{Key: semconv.AttrSiteName, Value: d.Site},
		{Key: semconv.AttrNetHostIP, Value: d.IP},
	} {
		if a.Value != "" {
			c.base = append(c.base, a)
		}
	}
	return &c
}
func (r *Recorder) Metrics() []Metric {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return append([]Metric(nil), r.s.metrics...)
}
func (r *Recorder) Logs() []Log {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return append([]Log(nil), r.s.logs...)
}
func (r *Recorder) HasMetric(n string, want map[string]string, v float64) bool {
	for _, m := range r.Metrics() {
		if m.Name != n || m.Value != v {
			continue
		}
		ok := true
		for k, x := range want {
			if m.Attrs[k] != x {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}
