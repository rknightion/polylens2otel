package phoneclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWave3CallLogFixturesPreserveRows(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"wave3_deskie_call_logs.json", "wave3_extra_call_logs.json"} {
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				Data CallLogs `json:"data"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Fatal(err)
			}
			if got := len(envelope.Data.Placed); got != 5 {
				t.Fatalf("placed rows = %d, want 5", got)
			}
			var row struct {
				LineNumber        string `json:"LineNumber"`
				StartTime         string `json:"StartTime"`
				RemotePartyName   string `json:"RemotePartyName"`
				RemotePartyNumber string `json:"RemotePartyNumber"`
				Duration          string `json:"Duration"`
			}
			if err := json.Unmarshal(envelope.Data.Placed[0], &row); err != nil {
				t.Fatal(err)
			}
			if row.LineNumber != "1" || row.StartTime == "" || row.RemotePartyName == "" || row.RemotePartyNumber == "" {
				t.Fatalf("first row = %#v, want actual call-log shape", row)
			}
		})
	}
}
