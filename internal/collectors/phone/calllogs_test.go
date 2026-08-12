package phone

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rknightion/polylens2otel/internal/phoneclient"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
	"github.com/rknightion/polylens2otel/internal/telemetrytest"
)

func TestCallLogsEmitRowsPerDevice(t *testing.T) {
	t.Parallel()
	targets := []Target{
		{TenantID: "tenant-a", Device: telemetry.Device{ID: "deskie-id", Name: "deskie"}, API: &fakeAPI{state: phoneclient.StateOK, callLogs: loadCallLogsFixture(t, "wave3_deskie_call_logs.json")}},
		{TenantID: "tenant-a", Device: telemetry.Device{ID: "extra-id", Name: "extra"}, API: &fakeAPI{state: phoneclient.StateOK, callLogs: loadCallLogsFixture(t, "wave3_extra_call_logs.json")}},
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
		{Device: telemetry.Device{ID: "empty", Name: "empty"}, API: &fakeAPI{state: phoneclient.StateOK}},
		{Device: telemetry.Device{ID: "invalid", Name: "invalid"}, API: &fakeAPI{state: phoneclient.StateOK, callLogs: phoneclient.CallLogs{Placed: []json.RawMessage{invalidStart}}}},
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
		API:      &fakeAPI{state: phoneclient.StateOK, callLogs: loadCallLogsFixture(t, "wave3_deskie_call_logs.json")},
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
