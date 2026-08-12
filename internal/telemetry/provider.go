package telemetry

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type ProviderOptions struct {
	Endpoint, Protocol, InstanceID, Token, ServiceName, ServiceVersion string
	Insecure                                                           bool
	Interval                                                           time.Duration
}
type Providers struct {
	Emitter Emitter
	metrics *sdkmetric.MeterProvider
	logs    *sdklog.LoggerProvider
	traces  *sdktrace.TracerProvider
}

func NewProviders(ctx context.Context, o ProviderOptions) (*Providers, error) {
	h := map[string]string{}
	if o.InstanceID != "" && o.Token != "" {
		h["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(o.InstanceID+":"+o.Token))
	}
	base := strings.TrimRight(o.Endpoint, "/")
	var mx sdkmetric.Exporter
	var lx sdklog.Exporter
	var tx sdktrace.SpanExporter
	var err error
	switch o.Protocol {
	case "", "http":
		mo := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpointURL(base + "/v1/metrics"), otlpmetrichttp.WithHeaders(h)}
		lo := []otlploghttp.Option{otlploghttp.WithEndpointURL(base + "/v1/logs"), otlploghttp.WithHeaders(h)}
		to := []otlptracehttp.Option{otlptracehttp.WithEndpointURL(base + "/v1/traces"), otlptracehttp.WithHeaders(h)}
		if o.Insecure {
			mo = append(mo, otlpmetrichttp.WithInsecure())
			lo = append(lo, otlploghttp.WithInsecure())
			to = append(to, otlptracehttp.WithInsecure())
		}
		if mx, err = otlpmetrichttp.New(ctx, mo...); err != nil {
			return nil, err
		}
		if lx, err = otlploghttp.New(ctx, lo...); err != nil {
			return nil, err
		}
		if tx, err = otlptracehttp.New(ctx, to...); err != nil {
			return nil, err
		}
	case "grpc":
		mo := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(base), otlpmetricgrpc.WithHeaders(h)}
		lo := []otlploggrpc.Option{otlploggrpc.WithEndpoint(base), otlploggrpc.WithHeaders(h)}
		to := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(base), otlptracegrpc.WithHeaders(h)}
		if o.Insecure {
			mo = append(mo, otlpmetricgrpc.WithInsecure())
			lo = append(lo, otlploggrpc.WithInsecure())
			to = append(to, otlptracegrpc.WithInsecure())
		}
		if mx, err = otlpmetricgrpc.New(ctx, mo...); err != nil {
			return nil, err
		}
		if lx, err = otlploggrpc.New(ctx, lo...); err != nil {
			return nil, err
		}
		if tx, err = otlptracegrpc.New(ctx, to...); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported OTLP protocol")
	}
	res, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", o.ServiceName), attribute.String("service.version", o.ServiceVersion)))
	if err != nil {
		return nil, err
	}
	if o.Interval <= 0 {
		o.Interval = 15 * time.Second
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mx, sdkmetric.WithInterval(o.Interval))))
	lp := sdklog.NewLoggerProvider(sdklog.WithResource(res), sdklog.WithProcessor(sdklog.NewBatchProcessor(lx)))
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(tx))
	otel.SetMeterProvider(mp)
	otel.SetTracerProvider(tp)
	return &Providers{Emitter: NewEmitter(mp.Meter(o.ServiceName), lp.Logger(o.ServiceName), tp.Tracer(o.ServiceName)), metrics: mp, logs: lp, traces: tp}, nil
}
func (p *Providers) Shutdown(ctx context.Context) error {
	return errors.Join(p.metrics.Shutdown(ctx), p.logs.Shutdown(ctx), p.traces.Shutdown(ctx))
}
