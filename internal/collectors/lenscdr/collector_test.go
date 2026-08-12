package lenscdr

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rknightion/polylens2otel/internal/collector"
	"github.com/rknightion/polylens2otel/internal/config"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetrytest"
)

func TestCollectParsesFixtureComputesDurationAndDeduplicates(t *testing.T) {
	fixture := readFixture(t)
	first := fixturePage(t, fixture, true, "second")
	second := fixturePage(t, fixture, false, "") // Deliberately repeats the same CDR rows.
	recorder := telemetrytest.New()
	c := New(&fakeQuery{responses: []json.RawMessage{first, second}}, filepath.Join(t.TempDir(), "state"), "tenant-a")

	if err := c.Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	logs := recorder.Logs()
	if len(logs) != 2 {
		t.Fatalf("emitted logs = %d, want 2 after page dedupe", len(logs))
	}
	if logs[0].Event != semconv.EventLensCDR {
		t.Errorf("event = %q, want %q", logs[0].Event, semconv.EventLensCDR)
	}
	if got := logs[0].Attrs[semconv.AttrDurationSeconds]; got != "9" {
		t.Errorf("duration_seconds = %q, want 9", got)
	}
	if got := logs[1].Attrs[semconv.AttrDurationSeconds]; got != "16" {
		t.Errorf("duration_seconds = %q, want 16", got)
	}
	if got := logs[0].Attrs[semconv.AttrDeviceID]; got != "482567139733" {
		t.Errorf("device.id = %q, want fixture device ID", got)
	}
}

func TestCollectWatermarkSurvivesRestart(t *testing.T) {
	fixture := readFixture(t)
	page := fixturePage(t, fixture, false, "")
	stateDir := filepath.Join(t.TempDir(), "state")

	first := telemetrytest.New()
	if err := New(&fakeQuery{responses: []json.RawMessage{page}}, stateDir, "tenant-a").Collect(context.Background(), first.Emitter()); err != nil {
		t.Fatalf("first Collect() error = %v", err)
	}
	if got := len(first.Logs()); got != 2 {
		t.Fatalf("first emitted logs = %d, want 2", got)
	}

	second := telemetrytest.New()
	if err := New(&fakeQuery{responses: []json.RawMessage{page}}, stateDir, "tenant-a").Collect(context.Background(), second.Emitter()); err != nil {
		t.Fatalf("second Collect() error = %v", err)
	}
	if got := len(second.Logs()); got != 0 {
		t.Errorf("logs after restart = %d, want 0 because watermark was persisted", got)
	}
}

func TestCollectEmitsAllValidRowsWhenPageIsNewestFirst(t *testing.T) {
	fixture := readFixture(t)
	page := fixturePage(t, fixture, false, "")
	var response map[string]any
	if err := json.Unmarshal(page, &response); err != nil {
		t.Fatalf("decode fixture page: %v", err)
	}
	edges := response["meetingRecordLists"].(map[string]any)["edges"].([]any)
	edges[0], edges[1] = edges[1], edges[0]
	page, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("encode newest-first fixture page: %v", err)
	}

	recorder := telemetrytest.New()
	c := New(&fakeQuery{responses: []json.RawMessage{page}}, filepath.Join(t.TempDir(), "state"), "tenant-a")
	if err := c.Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if got := len(recorder.Logs()); got != 2 {
		t.Errorf("emitted logs = %d, want 2 for a newest-first page", got)
	}
}

func TestCollectDropsRowWithInvalidTimestamp(t *testing.T) {
	fixture := readFixture(t)
	page := fixturePage(t, fixture, false, "")
	var response map[string]any
	if err := json.Unmarshal(page, &response); err != nil {
		t.Fatalf("decode fixture page: %v", err)
	}
	edges := response["meetingRecordLists"].(map[string]any)["edges"].([]any)
	edges[0].(map[string]any)["node"].(map[string]any)["startTime"] = "not-a-timestamp"
	page, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("encode invalid-timestamp fixture page: %v", err)
	}

	recorder := telemetrytest.New()
	c := New(&fakeQuery{responses: []json.RawMessage{page}}, filepath.Join(t.TempDir(), "state"), "tenant-a")
	if err := c.Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if got := len(recorder.Logs()); got != 1 {
		t.Errorf("emitted logs = %d, want 1 after invalid timestamp is dropped", got)
	}
}

func TestRegisterUsesConfiguredInterval(t *testing.T) {
	registry := collector.NewRegistry()
	cfg := config.Default()
	cfg.Lens.Tenants = []string{"tenant-a"}
	cfg.State.Dir = t.TempDir()
	Register(collector.Deps{Config: &cfg, Registry: registry, Services: map[string]any{"lensclient": &fakeQuery{}}})
	entries := registry.Entries()
	if len(entries) != 1 {
		t.Fatalf("registered collectors = %d, want 1", len(entries))
	}
	if got := entries[0].ID(); got != collectorID {
		t.Errorf("ID() = %q, want %q", got, collectorID)
	}
	if got := entries[0].Interval(); got != config.Default().Collectors.LensCDR {
		t.Errorf("Interval() = %v, want default %v", got, config.Default().Collectors.LensCDR)
	}
}

type fakeQuery struct {
	responses []json.RawMessage
	n         int
}

func (f *fakeQuery) Query(_ context.Context, _ string, _ any, out any) error {
	if f.n >= len(f.responses) {
		return nil
	}
	err := json.Unmarshal(f.responses[f.n], out)
	f.n++
	return err
}

func readFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/cdr.json")
	if err != nil {
		t.Fatalf("read CDR fixture: %v", err)
	}
	return b
}

func fixturePage(t *testing.T, fixture []byte, hasNext bool, cursor string) json.RawMessage {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(fixture, &envelope); err != nil {
		t.Fatalf("decode CDR fixture: %v", err)
	}
	var page map[string]any
	if err := json.Unmarshal(envelope.Data, &page); err != nil {
		t.Fatalf("decode CDR fixture data: %v", err)
	}
	info := page["meetingRecordLists"].(map[string]any)["pageInfo"].(map[string]any)
	info["hasNextPage"] = hasNext
	info["endCursor"] = cursor
	out, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("encode CDR page: %v", err)
	}
	return out
}
