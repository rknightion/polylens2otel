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

func TestNetworkInfoEmitsGaugeForEachDeviceFixture(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		info  phoneclient.NetworkInfo
		attrs map[string]string
	}{
		{
			name: "deskie",
			info: loadNetworkInfoFixture(t, "wave3_deskie_network_info.json"),
			attrs: map[string]string{
				semconv.AttrDHCPEnabled: "enabled", semconv.AttrDHCPServer: "192.0.2.254",
				semconv.AttrDefaultGateway: "192.0.2.254", semconv.AttrSubnetMask: "255.255.255.0",
				semconv.AttrBootServerOption: "160",
			},
		},
		{
			name: "extra",
			info: loadNetworkInfoFixture(t, "wave3_extra_network_info.json"),
			attrs: map[string]string{
				semconv.AttrDHCPEnabled: "enabled", semconv.AttrDHCPServer: "198.51.100.254",
				semconv.AttrDefaultGateway: "198.51.100.254", semconv.AttrSubnetMask: "255.255.255.0",
				semconv.AttrBootServerOption: "160",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			recorder := telemetrytest.New()
			collector := NewNetworkInfo([]Target{{
				Device: telemetry.Device{ID: tc.name, Name: tc.name, IP: "203.0.113.9"},
				API:    &fakeAPI{state: phoneclient.StateOK, networkInfo: tc.info},
			}})
			if collector == nil {
				t.Fatal("NewNetworkInfo() returned nil")
			}
			if err := collector.Collect(context.Background(), recorder.Emitter()); err != nil {
				t.Fatalf("collect network info: %v", err)
			}

			assertMetric(t, recorder, semconv.MetricPhoneNetworkInfo, 1, tc.attrs)
			assertOnlyNetworkInfoLabels(t, recorder)
		})
	}
}

func TestNetworkInfoEmitsEmptyLabelsForMissingPayloadFields(t *testing.T) {
	t.Parallel()
	recorder := telemetrytest.New()
	collector := NewNetworkInfo([]Target{{
		Device: telemetry.Device{ID: "sparse", Name: "sparse"},
		API:    &fakeAPI{state: phoneclient.StateOK, networkInfo: phoneclient.NetworkInfo{}},
	}})
	if collector == nil {
		t.Fatal("NewNetworkInfo() returned nil")
	}
	if err := collector.Collect(context.Background(), recorder.Emitter()); err != nil {
		t.Fatalf("collect network info: %v", err)
	}
	assertMetric(t, recorder, semconv.MetricPhoneNetworkInfo, 1, map[string]string{
		semconv.AttrDHCPEnabled: "", semconv.AttrDHCPServer: "", semconv.AttrDefaultGateway: "",
		semconv.AttrSubnetMask: "", semconv.AttrBootServerOption: "",
	})
	assertOnlyNetworkInfoLabels(t, recorder)
}

func assertOnlyNetworkInfoLabels(t *testing.T, recorder *telemetrytest.Recorder) {
	t.Helper()
	metrics := recorder.Metrics()
	if len(metrics) != 1 {
		t.Fatalf("metrics = %#v; want exactly one network info gauge", metrics)
	}
	for key := range metrics[0].Attrs {
		switch key {
		case semconv.AttrDeviceID, semconv.AttrDeviceName,
			semconv.AttrDHCPEnabled, semconv.AttrDHCPServer, semconv.AttrDefaultGateway,
			semconv.AttrSubnetMask, semconv.AttrBootServerOption:
		default:
			t.Fatalf("unexpected network info label %q in %#v", key, metrics[0].Attrs)
		}
	}
}

func loadNetworkInfoFixture(t *testing.T, name string) phoneclient.NetworkInfo {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "phoneclient", "testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var envelope struct {
		Data phoneclient.NetworkInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return envelope.Data
}
