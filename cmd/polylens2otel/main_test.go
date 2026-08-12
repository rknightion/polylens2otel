package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rknightion/polylens2otel/internal/config"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetrytest"
)

func TestVersionExitsWithoutConfiguration(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "commit=") {
		t.Fatalf("version output = %q", stdout.String())
	}
}

func TestEnsureStateDirCreatesAndProbesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	if err := ensureStateDir(path); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("write probe was not removed: %#v", entries)
	}
}

func TestLoggerRejectsUnknownFormat(t *testing.T) {
	if _, err := newLogger(config.LogConfig{Level: "info", Format: "xml"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown log format accepted")
	}
}

func TestTenantEmitterScopesProcessSignalsAcrossTenants(t *testing.T) {
	recorder := telemetrytest.New()
	emitter := newTenantEmitter(recorder.Emitter(), []string{"tenant-a", "tenant-b", "tenant-a"})
	if err := emitter.Gauge(context.Background(), semconv.MetricBuildInfo, 1); err != nil {
		t.Fatal(err)
	}
	metrics := recorder.Metrics()
	if len(metrics) != 2 {
		t.Fatalf("metric count = %d, want 2", len(metrics))
	}
	for i, tenant := range []string{"tenant-a", "tenant-b"} {
		if got := metrics[i].Attrs[semconv.AttrTenantID]; got != tenant {
			t.Fatalf("metric %d tenant.id = %q, want %q", i, got, tenant)
		}
	}
}

func TestTenantEmitterExplicitTenantDoesNotFanOut(t *testing.T) {
	recorder := telemetrytest.New()
	emitter := newTenantEmitter(recorder.Emitter(), []string{"tenant-a", "tenant-b"})
	if err := emitter.WithTenant("tenant-device").Gauge(context.Background(), semconv.MetricLensDeviceConnected, 1); err != nil {
		t.Fatal(err)
	}
	metrics := recorder.Metrics()
	if len(metrics) != 1 || metrics[0].Attrs[semconv.AttrTenantID] != "tenant-device" {
		t.Fatalf("metrics = %#v", metrics)
	}
}

func TestTenantEmitterUpdatesAfterDiscovery(t *testing.T) {
	recorder := telemetrytest.New()
	emitter := newTenantEmitter(recorder.Emitter(), nil)
	if err := emitter.Counter(context.Background(), semconv.MetricAuthTokenRefresh, 1); err != nil {
		t.Fatal(err)
	}
	emitter.SetTenants([]string{"tenant-a"})
	if err := emitter.LogEvent(context.Background(), semconv.EventExporterStartup, "started", time.Now()); err != nil {
		t.Fatal(err)
	}
	metrics := recorder.Metrics()
	logs := recorder.Logs()
	if len(metrics) != 1 || metrics[0].Attrs[semconv.AttrTenantID] != discoveryTenantID {
		t.Fatalf("bootstrap metrics = %#v", metrics)
	}
	if len(logs) != 1 || logs[0].Attrs[semconv.AttrTenantID] != "tenant-a" {
		t.Fatalf("startup logs = %#v", logs)
	}
}
