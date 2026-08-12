package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	lognoop "go.opentelemetry.io/otel/log/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/rknightion/polylens2otel/internal/semconv"
)

func TestEmitterAddressChangeKeepsOneDeviceMetricSeries(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown meter provider: %v", err)
		}
	})
	emitter := NewEmitter(provider.Meter("test"), lognoop.NewLoggerProvider().Logger("test"), tracenoop.NewTracerProvider().Tracer("test"))

	for _, ip := range []string{"192.0.2.10", "192.0.2.11"} {
		if err := emitter.WithDevice(Device{ID: "device-1", Name: "deskie", MAC: "00:11:22:33:44:55", Model: "Edge E350", Site: "HQ", IP: ip}).Gauge(context.Background(), "test.device.value", 1); err != nil {
			t.Fatalf("record device metric: %v", err)
		}
	}

	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	points := metricPoints(t, collected, "test.device.value")
	if len(points) != 1 {
		t.Fatalf("device metric series = %d; want 1 after address change", len(points))
	}
	for key, want := range map[string]string{
		semconv.AttrDeviceID:    "device-1",
		semconv.AttrDeviceName:  "deskie",
		semconv.AttrDeviceMAC:   "00:11:22:33:44:55",
		semconv.AttrDeviceModel: "Edge E350",
		semconv.AttrSiteName:    "HQ",
	} {
		value, ok := points[0].Attributes.Value(attribute.Key(key))
		if !ok || value.AsString() != want {
			t.Fatalf("device metric %q = %q, present=%t; want %q", key, value.AsString(), ok, want)
		}
	}
	if points[0].Attributes.HasValue(attribute.Key(semconv.AttrNetHostIP)) {
		t.Fatalf("device metric retained mutable %q attribute", semconv.AttrNetHostIP)
	}
}

func metricPoints(t *testing.T, collected metricdata.ResourceMetrics, name string) []metricdata.DataPoint[float64] {
	t.Helper()
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			gauge, ok := metric.Data.(metricdata.Gauge[float64])
			if !ok {
				t.Fatalf("metric %q data = %T; want float64 gauge", name, metric.Data)
			}
			return gauge.DataPoints
		}
	}
	t.Fatalf("metric %q was not collected", name)
	return nil
}
