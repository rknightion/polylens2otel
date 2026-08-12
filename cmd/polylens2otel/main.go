// Command polylens2otel exports read-only Poly Lens and phone telemetry over OTLP.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/grafana/pyroscope-go"
	"go.opentelemetry.io/otel"

	"github.com/rknightion/polylens2otel/internal/collector"
	phonecollector "github.com/rknightion/polylens2otel/internal/collectors/phone"
	"github.com/rknightion/polylens2otel/internal/config"
	"github.com/rknightion/polylens2otel/internal/lensclient"
	"github.com/rknightion/polylens2otel/internal/lensstream"
	"github.com/rknightion/polylens2otel/internal/phoneclient"
	"github.com/rknightion/polylens2otel/internal/phonetarget"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
	"github.com/rknightion/polylens2otel/internal/version"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("polylens2otel", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "path to YAML configuration")
	showVersion := flags.Bool("version", false, "print version and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, version.String())
		return 0
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "load configuration: %v\n", err)
		return 1
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(stderr, "validate configuration: %v\n", err)
		return 1
	}
	logger, err := newLogger(cfg.Log, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "configure logging: %v\n", err)
		return 1
	}
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Error("otel sdk error", "error", err)
	}))
	if err := ensureStateDir(cfg.State.Dir); err != nil {
		logger.Error("state directory is not writable", "error", err)
		return 1
	}

	providers, err := telemetry.NewProviders(ctx, telemetry.ProviderOptions{
		Endpoint: cfg.OTLP.Endpoint, Protocol: cfg.OTLP.Protocol, Insecure: cfg.OTLP.Insecure,
		InstanceID: cfg.OTLP.GrafanaCloud.InstanceID, Token: cfg.OTLP.GrafanaCloud.Token,
		ServiceName: "polylens2otel", ServiceVersion: version.Version, Interval: cfg.OTLP.ExportInterval,
	})
	if err != nil {
		logger.Error("build telemetry providers", "error", err)
		return 1
	}
	defer func() {
		if err := providers.Shutdown(context.Background()); err != nil {
			logger.Error("shutdown telemetry providers", "error", err)
		}
	}()
	runtimeEmitter := newTenantEmitter(providers.Emitter, cfg.Lens.Tenants)

	if profiler, err := startProfiler(cfg); err != nil {
		logger.Error("start Pyroscope profiling", "error", err)
	} else if profiler != nil {
		defer func() {
			if err := profiler.Stop(); err != nil {
				logger.Error("stop Pyroscope profiling", "error", err)
			}
		}()
	}

	lensTransport := telemetry.InstrumentHTTPTransport(http.DefaultTransport.(*http.Transport).Clone(), runtimeEmitter, "lens")
	lensHTTP := &http.Client{Transport: lensTransport, Timeout: cfg.Lens.RequestTimeout}
	lens, err := lensclient.New(lensclient.Config{
		TokenURL: cfg.Lens.TokenURL, GraphQLURL: cfg.Lens.GraphQLURL,
		ClientID: cfg.Lens.ClientID, ClientSecret: cfg.Lens.ClientSecret,
		PageSize: cfg.Lens.PageSize, MaxAttempts: cfg.Lens.Retry.MaxAttempts,
		MinBackoff: cfg.Lens.Retry.MinBackoff, MaxBackoff: cfg.Lens.Retry.MaxBackoff,
		HTTPClient: lensHTTP, Emitter: runtimeEmitter,
	})
	if err != nil {
		logger.Error("build Lens client", "error", err)
		return 1
	}

	tenantIDs, devices, err := discoverDevices(ctx, lens, cfg.Lens.Tenants)
	if err != nil {
		logger.Error("discover Lens devices", "error", err)
		return 1
	}
	runtimeEmitter.SetTenants(tenantIDs)
	runtimeCfg := *cfg
	runtimeCfg.Lens.Tenants = tenantIDs
	phoneTargets, err := resolvePhoneTargets(ctx, &runtimeCfg, runtimeEmitter, devices)
	if err != nil {
		logger.Error("resolve phone targets", "error", err)
		return 1
	}

	registry := collector.NewRegistry()
	deps := collector.Deps{
		Config: &runtimeCfg, Emitter: runtimeEmitter, Registry: registry,
		Services: map[string]any{
			"lens": lens, "lensclient": lens,
			phonecollector.ServiceTargets: phoneTargets,
		},
	}
	registerAll(deps)
	if err := runtimeEmitter.LogEvent(ctx, semconv.EventExporterStartup, "polylens2otel started", time.Now(),
		telemetry.Attr{Key: semconv.AttrVersion, Value: version.Version},
		telemetry.Attr{Key: semconv.AttrCommit, Value: version.Commit},
		telemetry.Attr{Key: semconv.AttrBuildDate, Value: version.BuildDate},
	); err != nil {
		logger.Error("emit startup event", "error", err)
		return 1
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	start := func(name string, fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil && runCtx.Err() == nil {
				select {
				case errCh <- fmt.Errorf("%s: %w", name, err):
				default:
				}
			}
		}()
	}
	start("collector scheduler", func() error { return collector.NewScheduler(runtimeEmitter).Run(runCtx, registry) })
	if runtimeCfg.Lens.Stream.Enabled && len(devices) > 0 {
		ids := make([]string, 0, len(devices))
		for _, device := range devices {
			ids = append(ids, device.ID)
		}
		stream := lensstream.New(lensstream.Config{
			URL: runtimeCfg.Lens.WebSocketURL, TokenSource: lens.AccessToken, DeviceIDs: ids,
			AckTimeout: runtimeCfg.Lens.Stream.AckTimeout, MinBackoff: runtimeCfg.Lens.Stream.MinBackoff,
			MaxBackoff: runtimeCfg.Lens.Stream.MaxBackoff, Emitter: runtimeEmitter,
		})
		start("Lens device stream", func() error { return stream.Run(runCtx) })
	}

	logger.Info("polylens2otel started", "version", version.Version, "tenants", len(tenantIDs), "devices", len(devices), "collectors", len(registry.Entries()))
	exitCode := 0
	select {
	case <-ctx.Done():
	case err := <-errCh:
		logger.Error("runtime stopped", "error", err)
		exitCode = 1
	}
	cancel()
	wg.Wait()
	return exitCode
}

func discoverDevices(ctx context.Context, lens *lensclient.Client, configured []string) ([]string, []lensclient.Device, error) {
	tenantIDs := append([]string(nil), configured...)
	if len(tenantIDs) == 0 {
		tenants, err := lens.Tenants(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, tenant := range tenants {
			tenantIDs = append(tenantIDs, tenant.ID)
		}
	}
	devices := make([]lensclient.Device, 0)
	for _, tenantID := range tenantIDs {
		found, err := lens.Devices(ctx, tenantID)
		if err != nil {
			return nil, nil, err
		}
		for i := range found {
			found[i].TenantID = tenantID
		}
		devices = append(devices, found...)
	}
	return tenantIDs, devices, nil
}

func resolvePhoneTargets(ctx context.Context, cfg *config.Config, emitter telemetry.Emitter, devices []lensclient.Device) ([]phonecollector.Target, error) {
	if !cfg.Phone.Enabled {
		return nil, nil
	}
	resolver, err := phonetarget.New(phonetarget.Config{
		Targets: cfg.Phone.Targets, Username: cfg.Phone.Auth.Username,
		Password: phonetarget.NewSecret(cfg.Phone.Auth.Password), FromLensPolicy: cfg.Phone.Auth.FromLensPolicy,
		Timeout: cfg.Phone.RequestTimeout, HTTPEmitter: emitter,
		TLS: phoneclient.TLSConfig{VerifyChain: cfg.Phone.TLS.VerifyChain, CAFile: cfg.Phone.TLS.CAFile},
	}, nil, emitter)
	if err != nil {
		return nil, err
	}
	resolved, err := resolver.Resolve(ctx, devices)
	if err != nil {
		return nil, err
	}
	targets := make([]phonecollector.Target, 0, len(resolved))
	for _, target := range resolved {
		site := ""
		if target.Device.Site != nil {
			site = target.Device.Site.Name
		}
		targets = append(targets, phonecollector.Target{
			TenantID: target.Device.TenantID,
			Device: telemetry.Device{
				ID: target.Device.ID, Name: target.Device.Name, MAC: target.Device.MACAddress,
				Model: target.Device.HardwareModel, Site: site, IP: target.Address,
			},
			API: target.API,
		})
	}
	return targets, nil
}

func ensureStateDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("state directory is empty")
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return err
	}
	probe, err := os.CreateTemp(path, ".write-probe-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		return err
	}
	return os.Remove(name)
}

func newLogger(cfg config.LogConfig, writer io.Writer) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{Level: level}
	switch strings.ToLower(cfg.Format) {
	case "json":
		return slog.New(slog.NewJSONHandler(writer, opts)), nil
	case "text":
		return slog.New(slog.NewTextHandler(writer, opts)), nil
	default:
		return nil, fmt.Errorf("log format must be json or text, got %q", cfg.Format)
	}
}

func startProfiler(cfg *config.Config) (*pyroscope.Profiler, error) {
	p := cfg.Profiling.Pyroscope
	if p.Endpoint == "" || p.BasicAuthUser == "" || p.BasicAuthPassword == "" {
		return nil, nil
	}
	return pyroscope.Start(pyroscope.Config{
		ApplicationName: p.Application, ServerAddress: p.Endpoint,
		BasicAuthUser: p.BasicAuthUser, BasicAuthPassword: p.BasicAuthPassword,
	})
}
