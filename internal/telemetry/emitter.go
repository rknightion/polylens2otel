package telemetry

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/rknightion/polylens2otel/internal/semconv"
)

type otelEmitter struct {
	meter  metric.Meter
	logger otellog.Logger
	tracer trace.Tracer
	base   []Attr
	shared *instruments
}
type instruments struct {
	mu       sync.Mutex
	gauges   map[string]metric.Float64Gauge
	counters map[string]metric.Float64Counter
}

func NewEmitter(m metric.Meter, l otellog.Logger, t trace.Tracer) Emitter {
	return &otelEmitter{meter: m, logger: l, tracer: t, shared: &instruments{gauges: map[string]metric.Float64Gauge{}, counters: map[string]metric.Float64Counter{}}}
}
func (e *otelEmitter) attrs(extra []Attr) []attribute.KeyValue {
	all := append(append([]Attr(nil), e.base...), extra...)
	out := make([]attribute.KeyValue, 0, len(all))
	for _, a := range all {
		if a.Key != "" {
			out = append(out, attribute.String(a.Key, a.Value))
		}
	}
	return out
}
func (e *otelEmitter) Gauge(ctx context.Context, name string, value float64, attrs ...Attr) error {
	e.shared.mu.Lock()
	g := e.shared.gauges[name]
	var err error
	if g == nil {
		g, err = e.meter.Float64Gauge(name)
		if err == nil {
			e.shared.gauges[name] = g
		}
	}
	e.shared.mu.Unlock()
	if err != nil {
		return err
	}
	g.Record(ctx, value, metric.WithAttributes(e.attrs(attrs)...))
	return nil
}
func (e *otelEmitter) Counter(ctx context.Context, name string, value float64, attrs ...Attr) error {
	e.shared.mu.Lock()
	c := e.shared.counters[name]
	var err error
	if c == nil {
		c, err = e.meter.Float64Counter(name)
		if err == nil {
			e.shared.counters[name] = c
		}
	}
	e.shared.mu.Unlock()
	if err != nil {
		return err
	}
	c.Add(ctx, value, metric.WithAttributes(e.attrs(attrs)...))
	return nil
}
func (e *otelEmitter) LogEvent(ctx context.Context, event, body string, ts time.Time, attrs ...Attr) error {
	if ts.IsZero() {
		return ErrMissingTimestamp
	}
	var r otellog.Record
	r.SetTimestamp(ts)
	r.SetObservedTimestamp(time.Now())
	r.SetBody(attribute.StringValue(body))
	r.AddAttributes(attribute.String(semconv.AttrEventName, event))
	for _, a := range append(append([]Attr(nil), e.base...), attrs...) {
		r.AddAttributes(attribute.String(a.Key, a.Value))
	}
	e.logger.Emit(ctx, r)
	return nil
}
func (e *otelEmitter) StartSpan(ctx context.Context, name string, attrs ...Attr) (context.Context, func(error)) {
	ctx, sp := e.tracer.Start(ctx, name, trace.WithAttributes(e.attrs(attrs)...))
	return ctx, func(err error) {
		if err != nil {
			sp.RecordError(err)
			sp.SetStatus(codes.Error, err.Error())
		}
		sp.End()
	}
}
func (e *otelEmitter) WithTenant(id string) Emitter {
	c := *e
	c.base = append(append([]Attr(nil), e.base...), Attr{semconv.AttrTenantID, id})
	return &c
}
func (e *otelEmitter) WithDevice(d Device) Emitter {
	c := *e
	c.base = append([]Attr(nil), e.base...)
	for _, a := range []Attr{{semconv.AttrDeviceID, d.ID}, {semconv.AttrDeviceName, d.Name}, {semconv.AttrDeviceMAC, d.MAC}, {semconv.AttrDeviceModel, d.Model}, {semconv.AttrSiteName, d.Site}, {semconv.AttrNetHostIP, d.IP}} {
		if a.Value != "" {
			c.base = append(c.base, a)
		}
	}
	return &c
}
