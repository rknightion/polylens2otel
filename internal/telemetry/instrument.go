package telemetry

import (
	"net/http"
	"time"

	"github.com/rknightion/polylens2otel/internal/semconv"
)

// InstrumentHTTPTransport wraps base with the self-observability metrics and
// tracing required for outbound API calls. source is the stable API source
// label, currently lens or phone.
func InstrumentHTTPTransport(base http.RoundTripper, emitter Emitter, source string) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return instrumentedTransport{base: base, emitter: emitter, source: source}
}

type instrumentedTransport struct {
	base    http.RoundTripper
	emitter Emitter
	source  string
}

func (t instrumentedTransport) RoundTrip(request *http.Request) (response *http.Response, err error) {
	if t.emitter == nil {
		return t.base.RoundTrip(request)
	}
	attrs := []Attr{{Key: semconv.AttrSource, Value: t.source}}
	spanCtx, end := t.emitter.StartSpan(request.Context(), "http.client.request", attrs...)
	defer func() { end(err) }()

	started := time.Now()
	response, err = t.base.RoundTrip(request.WithContext(spanCtx))
	_ = t.emitter.Gauge(request.Context(), semconv.MetricHTTPClientRequestDuration, time.Since(started).Seconds(), attrs...)
	if response == nil {
		return response, err
	}
	switch {
	case response.StatusCode >= http.StatusBadRequest && response.StatusCode < http.StatusInternalServerError:
		_ = t.emitter.Counter(request.Context(), semconv.MetricHTTP4xx, 1, attrs...)
	case response.StatusCode >= http.StatusInternalServerError:
		_ = t.emitter.Counter(request.Context(), semconv.MetricHTTP5xx, 1, attrs...)
	}
	return response, err
}
