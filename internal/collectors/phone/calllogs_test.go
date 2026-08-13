package phone

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rknightion/polylens2otel/internal/phoneclient"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
	"github.com/rknightion/polylens2otel/internal/telemetrytest"
)

func TestCallLogsEmitRowsPerDevice(t *testing.T) {
	t.Parallel()
	targets := []Target{
		{TenantID: "tenant-a", Device: telemetry.Device{ID: "deskie-id", Name: "deskie"}, API: callLogAPI(loadCallLogsFixture(t, "wave3_deskie_call_logs.json"))},
		{TenantID: "tenant-a", Device: telemetry.Device{ID: "extra-id", Name: "extra"}, API: callLogAPI(loadCallLogsFixture(t, "wave3_extra_call_logs.json"))},
	}
	recorder := telemetrytest.New()

	if err := NewCallLogs(targets).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect call logs: %v", err)
	}
	logs := recorder.Logs()
	if got := len(logs); got != 10 {
		t.Fatalf("logs = %d, want 10: %#v", got, logs)
	}
	for _, device := range []string{"deskie", "extra"} {
		count := 0
		for _, log := range logs {
			if log.Attrs[semconv.AttrDeviceName] != device {
				continue
			}
			count++
			if log.Event != semconv.LogPhoneCallRecord {
				t.Errorf("%s event = %q, want %q", device, log.Event, semconv.LogPhoneCallRecord)
			}
			for _, key := range []string{semconv.AttrDirection, semconv.AttrRemoteParty, semconv.AttrLine, semconv.AttrDurationSeconds, semconv.AttrDisposition, semconv.AttrStartedAt} {
				if _, ok := log.Attrs[key]; !ok {
					t.Errorf("%s log missing %s: %#v", device, key, log)
				}
			}
		}
		if count != 5 {
			t.Errorf("%s logs = %d, want 5", device, count)
		}
		assertMetric(t, recorder, semconv.MetricPhoneCallsTotal, 5, map[string]string{semconv.AttrDeviceName: device, semconv.AttrDirection: "placed"})
	}
}

func TestCallLogsEmptyAndUnparseableRowsEmitNothing(t *testing.T) {
	t.Parallel()
	invalidStart := json.RawMessage(`{"LineNumber":"1","StartTime":"not-a-time","RemotePartyName":"ignored","Duration":"15 secs"}`)
	targets := []Target{
		{Device: telemetry.Device{ID: "empty", Name: "empty"}, API: callLogAPI(phoneclient.CallLogs{})},
		{Device: telemetry.Device{ID: "invalid", Name: "invalid"}, API: callLogAPI(phoneclient.CallLogs{Placed: []json.RawMessage{invalidStart}})},
	}
	recorder := telemetrytest.New()

	if err := NewCallLogs(targets).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect call logs: %v", err)
	}
	if got := recorder.Logs(); len(got) != 0 {
		t.Fatalf("logs = %#v, want none", got)
	}
	if got := recorder.Metrics(); len(got) != 0 {
		t.Fatalf("metrics = %#v, want none", got)
	}
}

func TestCallLogsCheckpointPreventsRestartReplay(t *testing.T) {
	t.Parallel()
	target := Target{
		TenantID: "tenant-a",
		Device:   telemetry.Device{ID: "deskie-id", Name: "deskie"},
		API:      callLogAPI(loadCallLogsFixture(t, "wave3_deskie_call_logs.json")),
		StateDir: t.TempDir(),
	}
	recorder := telemetrytest.New()
	if err := NewCallLogs([]Target{target}).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("first collect call logs: %v", err)
	}
	if got := len(recorder.Logs()); got != 5 {
		t.Fatalf("first logs = %d, want 5", got)
	}
	if err := NewCallLogs([]Target{target}).Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("second collect call logs: %v", err)
	}
	if got := len(recorder.Logs()); got != 5 {
		t.Fatalf("logs after restart = %d, want 5 without replay", got)
	}
	if got := len(recorder.Metrics()); got != 1 {
		t.Fatalf("metrics after restart = %#v, want first-pass metric only", got)
	}
}

func TestCallLogsMigrateLegacyUTCCheckpointToPhoneTimezone(t *testing.T) {
	t.Parallel()
	stateDir := t.TempDir()
	row := json.RawMessage(`{"LineNumber":"1","StartTime":"2026-08-13T11:15:00","RemotePartyName":"redacted","Duration":"15 secs"}`)
	target := Target{
		TenantID: "tenant-a",
		Device:   telemetry.Device{ID: "deskie-id", Name: "deskie"},
		API:      callLogAPI(phoneclient.CallLogs{Placed: []json.RawMessage{row}}),
		StateDir: stateDir,
	}
	collector := newCallLogs([]Target{target}, stateDir, 5*time.Minute)
	path := collector.watermarkPath(target)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create checkpoint directory: %v", err)
	}
	legacy := []byte(`{"start_time":"2026-08-13T10:59:48Z","keys":["deskie-id\\u00002026-08-13T10:59:48"]}`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatalf("write legacy checkpoint: %v", err)
	}
	recorder := telemetrytest.New()

	if err := collector.Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect call logs: %v", err)
	}
	logs := recorder.Logs()
	wantUTC := time.Date(2026, time.August, 13, 10, 15, 0, 0, time.UTC)
	if len(logs) != 1 || !logs[0].Timestamp.Equal(wantUTC) {
		t.Fatalf("logs = %#v; want one call at %s after legacy checkpoint migration", logs, wantUTC)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated checkpoint: %v", err)
	}
	var saved map[string]any
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatalf("decode migrated checkpoint: %v", err)
	}
	if saved["version"] != float64(1) {
		t.Fatalf("checkpoint version = %#v; want 1", saved["version"])
	}
	if err := collector.Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect migrated checkpoint again: %v", err)
	}
	if got := len(recorder.Logs()); got != 1 {
		t.Fatalf("logs after restart = %d; want no replay", got)
	}
}

func TestCallLogsUsePhoneTimezoneAcrossDaylightSaving(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		startTime string
		wantUTC   time.Time
	}{
		{
			name:      "summer",
			startTime: "2026-08-13T10:45:16",
			wantUTC:   time.Date(2026, time.August, 13, 9, 45, 16, 0, time.UTC),
		},
		{
			name:      "winter",
			startTime: "2026-12-13T10:45:16",
			wantUTC:   time.Date(2026, time.December, 13, 10, 45, 16, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := json.RawMessage(`{"LineNumber":"1","StartTime":"` + tt.startTime + `","RemotePartyName":"redacted","Duration":"15 secs"}`)
			recorder := telemetrytest.New()
			target := Target{
				Device: telemetry.Device{ID: "deskie", Name: "deskie"},
				API:    callLogAPI(phoneclient.CallLogs{Placed: []json.RawMessage{row}}),
			}

			if err := NewCallLogs([]Target{target}).Collect(context.Background(), recorder.Emitter()); err != nil {
				t.Fatalf("collect call logs: %v", err)
			}
			logs := recorder.Logs()
			if len(logs) != 1 {
				t.Fatalf("logs = %#v; want one call record", logs)
			}
			if !logs[0].Timestamp.Equal(tt.wantUTC) {
				t.Fatalf("timestamp = %s; want %s", logs[0].Timestamp, tt.wantUTC)
			}
		})
	}
}

func TestCallLogsRejectMissingOrInvalidPhoneTimezone(t *testing.T) {
	t.Parallel()
	row := json.RawMessage(`{"LineNumber":"1","StartTime":"2026-08-13T10:45:16","RemotePartyName":"redacted","Duration":"15 secs"}`)
	tests := []struct {
		name        string
		config      map[string]phoneclient.ConfigParam
		invalid     []string
		wantMessage string
	}{
		{
			name:        "missing",
			wantMessage: `missing parameter "tcpIpApp.sntp.olsonTimezoneID"`,
		},
		{
			name: "unsupported",
			config: map[string]phoneclient.ConfigParam{
				"tcpIpApp.sntp.olsonTimezoneID": {Value: "Europe/London"},
			},
			invalid:     []string{"tcpIpApp.sntp.olsonTimezoneID"},
			wantMessage: `unsupported parameter "tcpIpApp.sntp.olsonTimezoneID"`,
		},
		{
			name: "invalid IANA location",
			config: map[string]phoneclient.ConfigParam{
				"tcpIpApp.sntp.olsonTimezoneID": {Value: "Mars/Olympus"},
			},
			wantMessage: `load "Mars/Olympus"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := telemetrytest.New()
			api := &fakeAPI{
				state:         phoneclient.StateOK,
				callLogs:      phoneclient.CallLogs{Placed: []json.RawMessage{row}},
				config:        tt.config,
				invalidParams: tt.invalid,
			}
			target := Target{Device: telemetry.Device{ID: "deskie", Name: "deskie"}, API: api}

			err := NewCallLogs([]Target{target}).Collect(context.Background(), recorder.Emitter())
			if err == nil || !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("Collect() error = %v; want message containing %q", err, tt.wantMessage)
			}
			if logs := recorder.Logs(); len(logs) != 0 {
				t.Fatalf("logs = %#v; want none for an unresolved phone timezone", logs)
			}
		})
	}
}

func callLogAPI(logs phoneclient.CallLogs) *fakeAPI {
	return &fakeAPI{
		state:    phoneclient.StateOK,
		callLogs: logs,
		config: map[string]phoneclient.ConfigParam{
			"tcpIpApp.sntp.olsonTimezoneID": {Value: "Europe/London", Source: "config"},
		},
	}
}

func loadCallLogsFixture(t *testing.T, name string) phoneclient.CallLogs {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "phoneclient", "testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var envelope struct {
		Data phoneclient.CallLogs `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return envelope.Data
}
