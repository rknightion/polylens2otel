package phone

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rknightion/polylens2otel/internal/collector"
	"github.com/rknightion/polylens2otel/internal/config"
	"github.com/rknightion/polylens2otel/internal/phoneclient"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
	"github.com/rknightion/polylens2otel/internal/telemetrytest"
)

func TestCollectorsEmitDegradedAndAPIDisabledFixturesPerInstance(t *testing.T) {
	t.Parallel()
	deskie := loadDeskFixture(t)
	extra := &fakeAPI{state: phoneclient.StateAPIDisabled}
	targets := []Target{
		{TenantID: "tenant-a", Device: telemetry.Device{ID: "482567139733", Name: "deskie", MAC: "482567139733", Model: "Edge E350", IP: "192.0.2.139"}, API: deskie},
		{TenantID: "tenant-a", Device: telemetry.Device{ID: "482567908b97", Name: "extra", MAC: "482567908b97", Model: "Edge E350", IP: "192.0.2.175"}, API: extra},
	}
	recorder := telemetrytest.New()
	emitter := recorder.Emitter()

	if err := NewStatus(targets).Collect(context.Background(), emitter); err != nil {
		t.Fatalf("collect phone status: %v", err)
	}
	if err := NewLines(targets).Collect(context.Background(), emitter); err != nil {
		t.Fatalf("collect phone lines: %v", err)
	}
	if err := NewConfig(targets, ConfigAllowlist).Collect(context.Background(), emitter); err != nil {
		t.Fatalf("collect phone config: %v", err)
	}

	assertMetric(t, recorder, semconv.MetricPhoneAPIState, 1, map[string]string{semconv.AttrTenantID: "tenant-a", semconv.AttrDeviceName: "deskie", semconv.AttrState: string(phoneclient.StateOK)})
	assertMetric(t, recorder, semconv.MetricPhoneAPIState, 1, map[string]string{semconv.AttrDeviceName: "extra", semconv.AttrState: string(phoneclient.StateAPIDisabled)})
	assertExactlyOneAPIState(t, recorder, "deskie", phoneclient.StateOK)
	assertExactlyOneAPIState(t, recorder, "extra", phoneclient.StateAPIDisabled)
	assertMetric(t, recorder, semconv.MetricPhoneUptimeSeconds, 8760, map[string]string{semconv.AttrDeviceName: "deskie"})
	assertMetric(t, recorder, semconv.MetricPhoneNetworkPackets, 174384, map[string]string{semconv.AttrDeviceName: "deskie", semconv.AttrDirection: "rx"})
	assertMetric(t, recorder, semconv.MetricPhoneNetworkPackets, 3624, map[string]string{semconv.AttrDeviceName: "deskie", semconv.AttrDirection: "tx"})
	assertMetric(t, recorder, semconv.MetricPhoneLinesTotal, 1, map[string]string{semconv.AttrDeviceName: "deskie"})
	assertMetric(t, recorder, semconv.MetricPhoneLineRegistered, 0, map[string]string{semconv.AttrDeviceName: "deskie", semconv.AttrLine: "1", semconv.AttrLabel: "Edge E350", semconv.AttrSIPAddress: "EdgeE350"})
	for _, name := range ConfigAllowlist {
		assertMetric(t, recorder, semconv.MetricPhoneConfigParamSource, 1, map[string]string{semconv.AttrDeviceName: "deskie", semconv.AttrParam: name, semconv.AttrSource: deskie.config[name].Source})
	}

	for _, metric := range recorder.Metrics() {
		if metric.Attrs[semconv.AttrDeviceName] == "extra" && metric.Name != semconv.MetricPhoneAPIState {
			t.Fatalf("extra API-off fixture emitted %s: %#v", metric.Name, metric)
		}
	}
	if extra.networkStatsCalls != 0 || extra.linesCalls != 0 || extra.configCalls != 0 {
		t.Fatalf("extra API-off fixture made read calls: stats=%d lines=%d config=%d", extra.networkStatsCalls, extra.linesCalls, extra.configCalls)
	}
}

func TestLinesCountsOneLogicalRegistrationFromDuplicateAPIRecords(t *testing.T) {
	t.Parallel()
	api := &fakeAPI{
		state: phoneclient.StateOK,
		lines: []phoneclient.Line{
			{LineNumber: "1", RegistrationStatus: "registered", SIPAddress: "sip.example.test", Label: "desk"},
			{LineNumber: "1", RegistrationStatus: "registered", SIPAddress: "sip.example.test", Label: "desk"},
		},
	}
	recorder := telemetrytest.New()
	if err := NewLines([]Target{{Device: telemetry.Device{ID: "deskie", Name: "deskie"}, API: api}}).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect phone lines: %v", err)
	}

	assertMetric(t, recorder, semconv.MetricPhoneLinesTotal, 1, map[string]string{semconv.AttrDeviceName: "deskie"})
	assertMetric(t, recorder, semconv.MetricPhoneLineRegistered, 1, map[string]string{semconv.AttrDeviceName: "deskie", semconv.AttrLine: "1", semconv.AttrLabel: "desk", semconv.AttrSIPAddress: "sip.example.test"})
	if len(recorder.Metrics()) != 2 {
		t.Fatalf("metrics = %#v; want one lines_total and one logical line", recorder.Metrics())
	}
}

func TestRegisterUsesFrozenPhoneCollectors(t *testing.T) {
	t.Parallel()
	registry := collector.NewRegistry()
	cfg := config.Default()
	Register(collector.Deps{
		Config:   &cfg,
		Registry: registry,
		Services: map[string]any{ServiceTargets: []Target{{Device: telemetry.Device{ID: "deskie"}, API: &fakeAPI{state: phoneclient.StateAPIDisabled}}}},
	})
	entries := registry.Entries()
	if len(entries) != 3 {
		t.Fatalf("registered collectors = %d; want 3", len(entries))
	}
	want := map[string]time.Duration{StatusID: time.Minute, LinesID: time.Minute, ConfigID: 5 * time.Minute}
	for _, entry := range entries {
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
		t.Fatalf("missing collector IDs: %#v", want)
	}
}

func TestParseUptime(t *testing.T) {
	t.Parallel()
	got, err := parseUptime("0 day 2:26:00")
	if err != nil || got != 8760 {
		t.Fatalf("parseUptime() = %v, %v; want 8760, nil", got, err)
	}
	if _, err := parseUptime("unknown"); err == nil {
		t.Fatal("parseUptime(unknown) succeeded")
	}
}

type fakeAPI struct {
	state             phoneclient.State
	networkStats      phoneclient.NetworkStats
	lines             []phoneclient.Line
	config            map[string]phoneclient.ConfigParam
	networkStatsCalls int
	linesCalls        int
	configCalls       int
}

func (f *fakeAPI) Probe(context.Context) (phoneclient.State, error) { return f.state, nil }
func (f *fakeAPI) NetworkStats(context.Context) (phoneclient.NetworkStats, error) {
	f.networkStatsCalls++
	return f.networkStats, nil
}
func (f *fakeAPI) LineInfo(context.Context) ([]phoneclient.Line, error) {
	f.linesCalls++
	return f.lines, nil
}
func (f *fakeAPI) ConfigGet(_ context.Context, _ []string) (map[string]phoneclient.ConfigParam, []string, error) {
	f.configCalls++
	return f.config, nil, nil
}

func loadDeskFixture(t *testing.T) *fakeAPI {
	t.Helper()
	load := func(name string, into any) {
		t.Helper()
		body, err := os.ReadFile(filepath.Join("..", "..", "phoneclient", "testdata", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Fatalf("decode envelope %s: %v", name, err)
		}
		if err := json.Unmarshal(envelope.Data, into); err != nil {
			t.Fatalf("decode data %s: %v", name, err)
		}
	}
	f := &fakeAPI{state: phoneclient.StateOK}
	load("deskie_network_stats.json", &f.networkStats)
	load("deskie_line_info.json", &f.lines)
	load("deskie_config_get.json", &f.config)
	return f
}

func assertMetric(t *testing.T, recorder *telemetrytest.Recorder, name string, value float64, attrs map[string]string) {
	t.Helper()
	if !recorder.HasMetric(name, attrs, value) {
		t.Fatalf("missing metric %s=%v with attrs %#v; all=%#v", name, value, attrs, recorder.Metrics())
	}
}

func assertExactlyOneAPIState(t *testing.T, recorder *telemetrytest.Recorder, device string, want phoneclient.State) {
	t.Helper()
	count := 0
	for _, metric := range recorder.Metrics() {
		if metric.Name == semconv.MetricPhoneAPIState && metric.Attrs[semconv.AttrDeviceName] == device {
			count++
			if metric.Value != 1 || metric.Attrs[semconv.AttrState] != string(want) {
				t.Fatalf("API state for %s = %#v; want state=%q value=1", device, metric, want)
			}
		}
	}
	if count != 1 {
		t.Fatalf("API states for %s = %d; want exactly one", device, count)
	}
}
