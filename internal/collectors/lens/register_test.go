package lens

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rknightion/polylens2otel/internal/collector"
	"github.com/rknightion/polylens2otel/internal/config"
	"github.com/rknightion/polylens2otel/internal/lensclient"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetrytest"
)

func TestRegisterEmitsFixtureDeviceMetrics(t *testing.T) {
	client := fixtureClient(t)
	recorder := telemetrytest.New()
	registry := collector.NewRegistry()
	Register(collector.Deps{
		Config: &config.Config{
			Lens:        config.LensConfig{Tenants: []string{"tenant-a"}},
			Collectors:  config.CollectorConfig{LensDevices: time.Minute},
			Cardinality: config.CardinalityConfig{MaxDevices: 2},
		},
		Registry: registry,
		Services: map[string]any{serviceName: client},
	})

	devices := collectorByID(t, registry, devicesID)
	devices.(*devicesCollector).now = func() time.Time {
		return time.Date(2026, time.August, 12, 9, 19, 59, 71_000_000, time.UTC)
	}
	if err := devices.Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	common := map[string]string{
		semconv.AttrTenantID:    "tenant-a",
		semconv.AttrDeviceID:    "482567139733",
		semconv.AttrDeviceName:  "deskie",
		semconv.AttrDeviceMAC:   "48:25:67:13:97:33",
		semconv.AttrDeviceModel: "Edge E350",
		semconv.AttrSiteName:    "Home",
		semconv.AttrNetHostIP:   "192.0.2.139",
	}
	if !recorder.HasMetric(semconv.MetricLensDeviceConnected, common, 1) {
		t.Fatal("missing connected metric for fixture device")
	}
	if !recorder.HasMetric(semconv.MetricLensDeviceLastDetectedSeconds, common, 75.331) {
		t.Fatal("missing last_detected age metric for fixture device")
	}
	if !recorder.HasMetric(semconv.MetricLensDeviceLastConfigRequestSeconds, common, 60) {
		t.Fatal("missing last_config_request age metric for fixture device")
	}
	infoAttrs := copyAttrs(common)
	infoAttrs[semconv.AttrVersion] = "8.6.0.1321"
	infoAttrs[semconv.AttrBuild] = "1321"
	if !recorder.HasMetric(semconv.MetricLensDeviceFirmwareInfo, infoAttrs, 1) {
		t.Fatal("missing firmware info metric for fixture device")
	}
}

func TestRegisterEmitsFirmwareCurrentFromFixture(t *testing.T) {
	client := fixtureClient(t)
	recorder := telemetrytest.New()
	registry := registerFixtureCollectors(t, client, 2)

	if err := collectorByID(t, registry, firmwareID).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if !recorder.HasMetric(semconv.MetricLensDeviceFirmwareCurrent, map[string]string{
		semconv.AttrTenantID: "tenant-a",
		semconv.AttrDeviceID: "482567139733",
	}, 1) {
		t.Fatal("missing current firmware metric for matching fixture version")
	}
}

func TestActiveCallsCollectorEmitsStateCount(t *testing.T) {
	client := fixtureClient(t)
	client.calls["482567139733"] = []lensclient.Call{{ID: "call-a", State: "connected"}, {ID: "call-b", State: "connected"}}
	recorder := telemetrytest.New()
	registry := registerFixtureCollectors(t, client, 2)

	if err := collectorByID(t, registry, activeCallsID).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if !recorder.HasMetric(semconv.MetricLensDeviceActiveCalls, map[string]string{
		semconv.AttrTenantID: "tenant-a",
		semconv.AttrDeviceID: "482567139733",
		semconv.AttrState:    "connected",
	}, 2) {
		t.Fatal("missing active-call count by state")
	}
}

func TestActiveCallsCollectorDoesNotInventStateForEmptyFixture(t *testing.T) {
	client := fixtureClient(t)
	recorder := telemetrytest.New()
	registry := registerFixtureCollectors(t, client, 2)

	if err := collectorByID(t, registry, activeCallsID).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, metric := range recorder.Metrics() {
		if metric.Name == semconv.MetricLensDeviceActiveCalls {
			t.Fatalf("empty active-calls fixture emitted %#v", metric)
		}
	}
}

func TestDevicesCollectorDropsInvalidEventTimestamp(t *testing.T) {
	client := fixtureClient(t)
	client.devices[0].LastDetected = "not-a-timestamp"
	recorder := telemetrytest.New()
	registry := registerFixtureCollectors(t, client, 2)
	devices := collectorByID(t, registry, devicesID).(*devicesCollector)
	devices.now = func() time.Time { return time.Date(2026, time.August, 12, 9, 20, 0, 0, time.UTC) }

	if err := devices.Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, metric := range recorder.Metrics() {
		if metric.Name == semconv.MetricLensDeviceLastDetectedSeconds && metric.Attrs[semconv.AttrDeviceID] == "482567139733" {
			t.Fatalf("invalid timestamp emitted %#v", metric)
		}
	}
}

func TestRegisterUsesFrozenLensCollectors(t *testing.T) {
	cfg := config.Default()
	registry := collector.NewRegistry()
	Register(collector.Deps{Config: &cfg, Registry: registry, Services: map[string]any{serviceName: fixtureClient(t)}})

	want := map[string]time.Duration{
		devicesID:     time.Minute,
		activeCallsID: time.Minute,
		firmwareID:    24 * time.Hour,
	}
	for _, entry := range registry.Entries() {
		interval, ok := want[entry.ID()]
		if !ok {
			t.Fatalf("unexpected collector ID %q", entry.ID())
		}
		if entry.Interval() != interval {
			t.Errorf("%s interval = %s; want %s", entry.ID(), entry.Interval(), interval)
		}
		delete(want, entry.ID())
	}
	if len(want) != 0 {
		t.Fatalf("missing collectors: %#v", want)
	}
}

func TestDevicesCollectorRejectsDeviceCountOverMax(t *testing.T) {
	client := fixtureClient(t)
	recorder := telemetrytest.New()
	registry := registerFixtureCollectors(t, client, 1)

	if err := collectorByID(t, registry, devicesID).Collect(context.Background(), recorder.Emitter()); err == nil {
		t.Fatal("Collect() error = nil, want cardinality guard failure")
	}
	if !recorder.HasMetric(semconv.MetricAPIUnexpected, map[string]string{semconv.AttrSource: "lens"}, 1) {
		t.Fatal("missing api.unexpected metric for cardinality guard failure")
	}
}

func registerFixtureCollectors(t *testing.T, client *fixtureLens, maxDevices int) *collector.Registry {
	t.Helper()
	registry := collector.NewRegistry()
	Register(collector.Deps{
		Config: &config.Config{
			Lens:        config.LensConfig{Tenants: []string{"tenant-a"}},
			Collectors:  config.CollectorConfig{LensDevices: time.Minute, LensActiveCalls: time.Minute, LensFirmware: 24 * time.Hour},
			Cardinality: config.CardinalityConfig{MaxDevices: maxDevices},
		},
		Registry: registry,
		Services: map[string]any{serviceName: client},
	})
	return registry
}

func collectorByID(t *testing.T, registry *collector.Registry, id string) collector.Collector {
	t.Helper()
	for _, entry := range registry.Entries() {
		if entry.ID() == id {
			return entry
		}
	}
	t.Fatalf("collector %q not registered", id)
	return nil
}

type fixtureLens struct {
	devices  []lensclient.Device
	calls    map[string][]lensclient.Call
	firmware lensclient.Firmware
}

func (c *fixtureLens) Devices(context.Context, string) ([]lensclient.Device, error) {
	return c.devices, nil
}
func (c *fixtureLens) ActiveCalls(_ context.Context, deviceID string) ([]lensclient.Call, error) {
	return c.calls[deviceID], nil
}
func (c *fixtureLens) LatestFirmware(context.Context, string) (lensclient.Firmware, error) {
	return c.firmware, nil
}

func fixtureClient(t *testing.T) *fixtureLens {
	t.Helper()
	var devicesResponse struct {
		Data struct {
			Tenant struct {
				Inventory struct {
					DeviceSearch struct {
						Edges []struct {
							Node lensclient.Device `json:"node"`
						} `json:"edges"`
					} `json:"deviceSearch"`
				} `json:"inventory"`
			} `json:"tenant"`
		} `json:"data"`
	}
	decodeFixture(t, "devicesearch.json", &devicesResponse)
	var firmwareResponse struct {
		Data struct {
			Firmware lensclient.Firmware `json:"availableProductSoftwareByPid"`
		} `json:"data"`
	}
	decodeFixture(t, "firmware.json", &firmwareResponse)
	var activeCallsResponse struct {
		Data struct {
			ActiveCalls struct {
				Calls []lensclient.Call `json:"calls"`
			} `json:"activeCalls"`
		} `json:"data"`
	}
	decodeFixture(t, "activecalls.json", &activeCallsResponse)

	devices := make([]lensclient.Device, 0, len(devicesResponse.Data.Tenant.Inventory.DeviceSearch.Edges))
	for _, edge := range devicesResponse.Data.Tenant.Inventory.DeviceSearch.Edges {
		devices = append(devices, edge.Node)
	}
	calls := make(map[string][]lensclient.Call, len(devices))
	if len(devices) > 0 {
		calls[devices[0].ID] = activeCallsResponse.Data.ActiveCalls.Calls
	}
	return &fixtureLens{devices: devices, calls: calls, firmware: firmwareResponse.Data.Firmware}
}

func decodeFixture(t *testing.T, name string, out any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "lensclient", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode fixture %s: %v", name, err)
	}
}

func copyAttrs(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
