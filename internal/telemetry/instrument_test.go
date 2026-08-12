package telemetry

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rknightion/polylens2otel/internal/semconv"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type recordedMetric struct {
	name  string
	value float64
	attrs []Attr
}

type instrumentRecorder struct {
	metrics    []recordedMetric
	spanStarts int
	spanEnds   int
}

func (r *instrumentRecorder) Gauge(_ context.Context, name string, value float64, attrs ...Attr) error {
	r.metrics = append(r.metrics, recordedMetric{name: name, value: value, attrs: attrs})
	return nil
}
func (r *instrumentRecorder) Counter(ctx context.Context, name string, value float64, attrs ...Attr) error {
	return r.Gauge(ctx, name, value, attrs...)
}
func (*instrumentRecorder) LogEvent(context.Context, string, string, time.Time, ...Attr) error {
	return nil
}
func (r *instrumentRecorder) StartSpan(ctx context.Context, _ string, _ ...Attr) (context.Context, func(error)) {
	r.spanStarts++
	return context.WithValue(ctx, instrumentContextKey{}, true), func(error) { r.spanEnds++ }
}
func (r *instrumentRecorder) WithTenant(string) Emitter { return r }
func (r *instrumentRecorder) WithDevice(Device) Emitter { return r }

type instrumentContextKey struct{}

func TestInstrumentHTTPTransportEmitsMetricsAndChildSpan(t *testing.T) {
	for _, test := range []struct {
		name       string
		statusCode int
		counter    string
	}{
		{name: "4xx", statusCode: http.StatusTooManyRequests, counter: semconv.MetricHTTP4xx},
		{name: "5xx", statusCode: http.StatusBadGateway, counter: semconv.MetricHTTP5xx},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &instrumentRecorder{}
			childContextSeen := false
			transport := InstrumentHTTPTransport(transportFunc(func(request *http.Request) (*http.Response, error) {
				childContextSeen, _ = request.Context().Value(instrumentContextKey{}).(bool)
				return &http.Response{StatusCode: test.statusCode, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
			}), recorder, "lens")
			request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.test", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := transport.RoundTrip(request)
			if err != nil {
				t.Fatalf("round trip: %v", err)
			}
			_ = response.Body.Close()
			if !childContextSeen || recorder.spanStarts != 1 || recorder.spanEnds != 1 {
				t.Fatalf("child span context=%t starts=%d ends=%d, want true/1/1", childContextSeen, recorder.spanStarts, recorder.spanEnds)
			}
			if !recorder.hasMetric(semconv.MetricHTTPClientRequestDuration, semconv.AttrSource, "lens") {
				t.Fatal("missing request duration metric with source")
			}
			if !recorder.hasMetric(test.counter, semconv.AttrSource, "lens") {
				t.Fatalf("missing %s metric with source", test.counter)
			}
		})
	}
}

func (r *instrumentRecorder) hasMetric(name, key, value string) bool {
	for _, metric := range r.metrics {
		if metric.name != name {
			continue
		}
		for _, attr := range metric.attrs {
			if attr.Key == key && attr.Value == value {
				return true
			}
		}
	}
	return false
}
