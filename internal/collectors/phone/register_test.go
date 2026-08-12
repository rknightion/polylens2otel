package phone

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	assertAPIStateEnum(t, recorder, "deskie", phoneclient.StateOK)
	assertAPIStateEnum(t, recorder, "extra", phoneclient.StateAPIDisabled)
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

func TestCollectorsEmitHealthyFixturesPerInstance(t *testing.T) {
	t.Parallel()
	instances := []struct {
		name      string
		rxPackets float64
		txPackets float64
		uptime    float64
	}{
		{name: "deskie", rxPackets: 382143, txPackets: 67852, uptime: 13293},
		{name: "extra", rxPackets: 10, txPackets: 0, uptime: 7407},
	}
	targets := make([]Target, 0, len(instances))
	fixtures := make(map[string]*fakeAPI, len(instances))
	for _, instance := range instances {
		api := loadHealthyFixture(t, instance.name)
		fixtures[instance.name] = api
		targets = append(targets, Target{
			TenantID: "tenant-a",
			Device: telemetry.Device{
				ID: "device-" + instance.name, Name: instance.name, MAC: "000000000000", Model: "Edge E350",
			},
			API: api,
		})
	}
	recorder := telemetrytest.New()
	if err := NewStatus(targets).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect phone status: %v", err)
	}
	if err := NewLines(targets).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect phone lines: %v", err)
	}
	if err := NewConfig(targets, ConfigAllowlist).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect phone config: %v", err)
	}

	for _, instance := range instances {
		if got := len(fixtures[instance.name].lines); got != 4 {
			t.Fatalf("%s lineInfo records = %d, want 4", instance.name, got)
		}
		registrations := make(map[string]struct{})
		for _, line := range fixtures[instance.name].lines {
			registrations[line.SIPAddress] = struct{}{}
		}
		if got := len(registrations); got != 2 {
			t.Fatalf("%s logical SIP registrations = %d, want 2", instance.name, got)
		}
		assertAPIStateEnum(t, recorder, instance.name, phoneclient.StateOK)
		assertMetric(t, recorder, semconv.MetricPhoneUptimeSeconds, instance.uptime, map[string]string{semconv.AttrDeviceName: instance.name})
		assertMetric(t, recorder, semconv.MetricPhoneNetworkPackets, instance.rxPackets, map[string]string{semconv.AttrDeviceName: instance.name, semconv.AttrDirection: "rx"})
		assertMetric(t, recorder, semconv.MetricPhoneNetworkPackets, instance.txPackets, map[string]string{semconv.AttrDeviceName: instance.name, semconv.AttrDirection: "tx"})
		for line := 1; line <= 4; line++ {
			lineNumber := strconv.Itoa(line)
			registration := 1
			if line > 2 {
				registration = 2
			}
			registrationNumber := strconv.Itoa(registration)
			assertMetric(t, recorder, semconv.MetricPhoneLineRegistered, 1, map[string]string{
				semconv.AttrDeviceName: instance.name,
				semconv.AttrLine:       lineNumber,
				semconv.AttrLabel:      "Line " + registrationNumber,
				semconv.AttrSIPAddress: "line-" + registrationNumber + "@example.test",
			})
		}
		for _, name := range ConfigAllowlist {
			assertMetric(t, recorder, semconv.MetricPhoneConfigParamSource, 1, map[string]string{
				semconv.AttrDeviceName: instance.name,
				semconv.AttrParam:      name,
				semconv.AttrSource:     fixtures[instance.name].config[name].Source,
			})
		}
	}
}

func TestStatusEmitsAllFourAPIStatesPerInstance(t *testing.T) {
	t.Parallel()
	targets := make([]Target, 0, len(apiStates))
	for _, state := range apiStates {
		targets = append(targets, Target{
			Device: telemetry.Device{ID: "device-" + string(state), Name: string(state)},
			API: &fakeAPI{state: state, networkStats: phoneclient.NetworkStats{
				UpTime: "0 day 0:00:00", RxPackets: "0", TxPackets: "0",
			}},
		})
	}
	recorder := telemetrytest.New()
	if err := NewStatus(targets).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect phone status: %v", err)
	}
	for _, state := range apiStates {
		assertAPIStateEnum(t, recorder, string(state), state)
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

func TestStatusEmitsFullAPIStateEnum(t *testing.T) {
	t.Parallel()
	recorder := telemetrytest.New()
	state := phoneclient.StateAuthFailed
	if err := NewStatus([]Target{{Device: telemetry.Device{ID: "deskie", Name: "deskie"}, API: &fakeAPI{state: state}}}).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect phone status: %v", err)
	}

	assertAPIStateEnum(t, recorder, "deskie", state)
}

func TestRegisterUsesFrozenPhoneCollectors(t *testing.T) {
	t.Parallel()
	registry := collector.NewRegistry()
	cfg := config.Default()
	cfg.Collectors.PhoneCallLogs = 7 * time.Minute
	cfg.Collectors.PhoneNetworkInfo = 11 * time.Minute
	Register(collector.Deps{
		Config:   &cfg,
		Registry: registry,
		Services: map[string]any{ServiceTargets: []Target{{
			Device: telemetry.Device{ID: "deskie"}, API: &fakeAPI{state: phoneclient.StateAPIDisabled},
			CallLogsInterval: cfg.Collectors.PhoneCallLogs, NetworkInfoInterval: cfg.Collectors.PhoneNetworkInfo,
		}}},
	})
	entries := registry.Entries()
	if len(entries) != 5 {
		t.Fatalf("registered collectors = %d; want 5", len(entries))
	}
	want := map[string]time.Duration{
		StatusID: time.Minute, LinesID: time.Minute, ConfigID: 5 * time.Minute,
		callLogsID: 7 * time.Minute, NetworkInfoID: 11 * time.Minute,
	}
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

func TestRegisterConfigCollectorUsesConfiguredParamAllowlist(t *testing.T) {
	t.Parallel()
	params := []string{"reg.7.address", "feature.example.enabled"}
	api := &fakeAPI{state: phoneclient.StateOK, config: map[string]phoneclient.ConfigParam{
		params[0]: {Source: "config"},
		params[1]: {Source: "default"},
	}}
	cfg := config.Default()
	cfg.Phone.ConfigParams = params
	registry := collector.NewRegistry()
	Register(collector.Deps{
		Config: &cfg, Registry: registry,
		Services: map[string]any{ServiceTargets: []Target{{Device: telemetry.Device{ID: "deskie", Name: "deskie"}, API: api}}},
	})
	var configEntry collector.Collector
	for _, entry := range registry.Entries() {
		if entry.ID() == ConfigID {
			configEntry = entry
			break
		}
	}
	if configEntry == nil {
		t.Fatal("config collector was not registered")
	}
	recorder := telemetrytest.New()
	if err := configEntry.Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect configured params: %v", err)
	}
	if !slices.Equal(api.configParams, params) {
		t.Fatalf("ConfigGet params = %#v; want %#v", api.configParams, params)
	}
	metrics := recorder.Metrics()
	if len(metrics) != len(params) {
		t.Fatalf("config param metrics = %#v; want exactly %d", metrics, len(params))
	}
	for _, name := range params {
		assertMetric(t, recorder, semconv.MetricPhoneConfigParamSource, 1, map[string]string{semconv.AttrParam: name})
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
	networkInfo       phoneclient.NetworkInfo
	callLogs          phoneclient.CallLogs
	lines             []phoneclient.Line
	config            map[string]phoneclient.ConfigParam
	networkStatsCalls int
	linesCalls        int
	configCalls       int
	configParams      []string
}

func (f *fakeAPI) Probe(context.Context) (phoneclient.State, error) { return f.state, nil }
func (f *fakeAPI) NetworkStats(context.Context) (phoneclient.NetworkStats, error) {
	f.networkStatsCalls++
	return f.networkStats, nil
}
func (f *fakeAPI) NetworkInfo(context.Context) (phoneclient.NetworkInfo, error) {
	return f.networkInfo, nil
}
func (f *fakeAPI) CallLogs(context.Context) (phoneclient.CallLogs, error) {
	return f.callLogs, nil
}
func (f *fakeAPI) LineInfo(context.Context) ([]phoneclient.Line, error) {
	f.linesCalls++
	return f.lines, nil
}
func (f *fakeAPI) ConfigGet(_ context.Context, params []string) (map[string]phoneclient.ConfigParam, []string, error) {
	f.configCalls++
	f.configParams = append([]string(nil), params...)
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

func loadHealthyFixture(t *testing.T, device string) *fakeAPI {
	t.Helper()
	load := func(suffix string, into any) {
		t.Helper()
		name := "healthy_" + device + "_" + suffix + ".json"
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
		if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
			t.Fatalf("fixture %s has no data", name)
		}
		if err := json.Unmarshal(envelope.Data, into); err != nil {
			t.Fatalf("decode data %s: %v", name, err)
		}
	}
	f := &fakeAPI{state: phoneclient.StateOK}
	load("network_stats", &f.networkStats)
	load("line_info", &f.lines)
	load("config_get", &f.config)
	return f
}

func assertMetric(t *testing.T, recorder *telemetrytest.Recorder, name string, value float64, attrs map[string]string) {
	t.Helper()
	if !recorder.HasMetric(name, attrs, value) {
		t.Fatalf("missing metric %s=%v with attrs %#v; all=%#v", name, value, attrs, recorder.Metrics())
	}
}

func assertAPIStateEnum(t *testing.T, recorder *telemetrytest.Recorder, device string, current phoneclient.State) {
	t.Helper()
	for _, state := range apiStates {
		want := 0.0
		if state == current {
			want = 1
		}
		assertMetric(t, recorder, semconv.MetricPhoneAPIState, want, map[string]string{
			semconv.AttrDeviceName: device,
			semconv.AttrState:      string(state),
		})
	}
}
