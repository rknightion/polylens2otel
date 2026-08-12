package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rknightion/polylens2otel/internal/config"
	"github.com/rknightion/polylens2otel/internal/phoneclient"
	"github.com/rknightion/polylens2otel/internal/phonetarget"
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

func TestStaticPhoneDevicesProvideCertificateIdentityWithoutLens(t *testing.T) {
	devices, tenants, err := staticPhoneDevices(map[string]string{
		"482567000002": "phone-b.example",
		"482567000001": "phone-a.example",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{discoveryTenantID}; !reflect.DeepEqual(tenants, want) {
		t.Fatalf("tenants = %#v, want %#v", tenants, want)
	}
	if len(devices) != 2 {
		t.Fatalf("devices = %d, want 2", len(devices))
	}
	if devices[0].ID != "482567000001" || devices[0].MACAddress != "482567000001" || devices[0].InternalIP != "phone-a.example" || devices[0].TenantID != discoveryTenantID {
		t.Fatalf("first static device = %#v", devices[0])
	}
}

func TestStaticPhoneRuntimeFallsBackWithoutLensCredentials(t *testing.T) {
	cfg := config.Default()
	cfg.Phone.Targets = map[string]string{"482567000001": "phone.example"}
	cfg.Phone.Auth.Password = "configured-password"
	devices, tenants, err := staticPhoneDevices(cfg.Phone.Targets, nil)
	if err != nil {
		t.Fatal(err)
	}
	recorder := telemetrytest.New()
	emitter := newTenantEmitter(recorder.Emitter(), tenants)
	var clientConfig phoneclient.Config
	targets, err := resolvePhoneTargetsWithFactory(context.Background(), &cfg, nil, emitter, devices, func(got phoneclient.Config) (phonetarget.API, error) {
		clientConfig = got
		return staticPhoneAPI{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(targets))
	}
	if clientConfig.DeviceMAC != "482567000001" || clientConfig.Password != "configured-password" {
		t.Fatalf("phone client config = %#v", clientConfig)
	}
	if !recorder.HasMetric(semconv.MetricAPIUnexpected, map[string]string{semconv.AttrDeviceID: "482567000001"}, 1) {
		t.Fatal("missing api.unexpected for unavailable Lens policy source")
	}
}

type staticPhoneAPI struct{}

func (staticPhoneAPI) Probe(context.Context) (phoneclient.State, error) {
	return phoneclient.StateOK, nil
}
func (staticPhoneAPI) NetworkStats(context.Context) (phoneclient.NetworkStats, error) {
	return phoneclient.NetworkStats{}, nil
}
func (staticPhoneAPI) NetworkInfo(context.Context) (phoneclient.NetworkInfo, error) {
	return phoneclient.NetworkInfo{}, nil
}
func (staticPhoneAPI) CallLogs(context.Context) (phoneclient.CallLogs, error) {
	return phoneclient.CallLogs{}, nil
}
func (staticPhoneAPI) LineInfo(context.Context) ([]phoneclient.Line, error) { return nil, nil }
func (staticPhoneAPI) ConfigGet(context.Context, []string) (map[string]phoneclient.ConfigParam, []string, error) {
	return nil, nil, nil
}

var _ phonetarget.API = staticPhoneAPI{}
