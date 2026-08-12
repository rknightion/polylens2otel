package phoneclient

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testMAC = "48:25:67:90:8b:97"

func TestDeskieFixtureReadsUseDigestAndDecode(t *testing.T) {
	fixtures := map[string]string{
		"/api/v1/mgmt/device/info":   "deskie_device_info.json",
		"/api/v1/mgmt/lineInfo":      "deskie_line_info.json",
		"/api/v1/mgmt/network/stats": "deskie_network_stats.json",
		"/api/v1/mgmt/network/info":  "deskie_network_info.json",
		"/api/v1/mgmt/callLogs":      "deskie_call_logs.json",
		"/api/v1/mgmt/config/get":    "deskie_config_get.json",
	}
	server, requests := digestServer(t, testMAC, func(w http.ResponseWriter, r *http.Request) {
		name, ok := fixtures[r.URL.Path]
		if !ok {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Path == "/api/v1/mgmt/config/get" {
			if r.Method != http.MethodPost {
				t.Fatalf("config/get method = %s, want POST", r.Method)
			}
			var payload struct {
				Data []string `json:"data"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if strings.Join(payload.Data, ",") != "reg.1.address,softkey.1.enable" {
				t.Fatalf("config/get parameters = %#v", payload.Data)
			}
		} else if r.Method != http.MethodGet {
			t.Fatalf("read method = %s, want GET", r.Method)
		}
		writeFixture(t, w, name)
	})
	defer server.Close()

	client := newTestClient(server.URL, testMAC)
	ctx := context.Background()
	if state, err := client.Probe(ctx); err != nil || state != StateOK {
		t.Fatalf("Probe() = %q, %v; want ok, nil", state, err)
	}
	info, err := client.DeviceInfo(ctx)
	if err != nil || info.ModelNumber != "Edge E350" || info.MACAddress != "482567139733" {
		t.Fatalf("DeviceInfo() = %#v, %v", info, err)
	}
	lines, err := client.LineInfo(ctx)
	if err != nil || len(lines) != 1 || lines[0].SIPAddress != "EdgeE350" || lines[0].RegistrationStatus != "unregistered" {
		t.Fatalf("LineInfo() = %#v, %v", lines, err)
	}
	stats, err := client.NetworkStats(ctx)
	if err != nil || stats.RxPackets != "174384" || stats.TxPackets != "3624" {
		t.Fatalf("NetworkStats() = %#v, %v", stats, err)
	}
	if _, err := client.NetworkInfo(ctx); err != nil {
		t.Fatalf("NetworkInfo(): %v", err)
	}
	if logs, err := client.CallLogs(ctx); err != nil || len(logs.Missed) != 0 {
		t.Fatalf("CallLogs() = %#v, %v", logs, err)
	}
	params, invalid, err := client.ConfigGet(ctx, []string{"reg.1.address", "softkey.1.enable"})
	if err != nil || params["softkey.1.enable"].Value != "0" || len(invalid) != 0 {
		t.Fatalf("ConfigGet() = %#v, %#v, %v", params, invalid, err)
	}
	if got := *requests; got < 14 { // one challenge and one authenticated request per call
		t.Fatalf("requests = %d, expected digest challenge flow", got)
	}
}

func TestExtraFixtureClassifiesUnauthenticated404AsAPIDisabled(t *testing.T) {
	server, requests := tlsServer(t, testMAC, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("credential authorization sent before API-disabled classification")
		}
		writeFixture(t, w, "extra_404.txt")
	})
	defer server.Close()

	state, err := newTestClient(server.URL, testMAC).Probe(context.Background())
	if err != nil || state != StateAPIDisabled {
		t.Fatalf("Probe() = %q, %v; want api_disabled, nil", state, err)
	}
	if got := *requests; got != 1 {
		t.Fatalf("requests = %d, want one unauthenticated probe", got)
	}
}

func TestProbeClassifiesDigest401AsAuthFailed(t *testing.T) {
	server, _ := tlsServer(t, testMAC, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Digest realm="phone", nonce="nonce", qop="auth", algorithm=MD5`)
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer server.Close()

	state, err := newTestClient(server.URL, testMAC).Probe(context.Background())
	if err != nil || state != StateAuthFailed {
		t.Fatalf("Probe() = %q, %v; want auth_failed, nil", state, err)
	}
}

func TestCertificateMismatchPreventsCredentialTransmission(t *testing.T) {
	server, requests := digestServer(t, "482567000000", func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("HTTP request reached server after certificate identity mismatch")
	})
	defer server.Close()

	state, err := newTestClient(server.URL, testMAC).Probe(context.Background())
	if err == nil || state != StateUnreachable {
		t.Fatalf("Probe() = %q, %v; want unreachable identity error", state, err)
	}
	if strings.Contains(err.Error(), "test-password") {
		t.Fatalf("error exposes password: %v", err)
	}
	if got := *requests; got != 0 {
		t.Fatalf("requests = %d, want no HTTP request", got)
	}
}

func newTestClient(baseURL, mac string) *Client {
	testClient, err := New(Config{BaseURL: baseURL, DeviceMAC: mac, Username: "Polycom", Password: "test-password", Timeout: time.Second})
	if err != nil {
		panic(err)
	}
	return testClient
}

func digestServer(t *testing.T, cn string, handler http.HandlerFunc) (*httptest.Server, *int) {
	return tlsServer(t, cn, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="phone", nonce="nonce", qop="auth", algorithm=MD5`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Digest ") || !strings.Contains(r.Header.Get("Authorization"), `username="Polycom"`) {
			t.Fatalf("unexpected authorization: %q", r.Header.Get("Authorization"))
		}
		handler(w, r)
	})
}

func tlsServer(t *testing.T, cn string, handler http.HandlerFunc) (*httptest.Server, *int) {
	t.Helper()
	cert := certificate(t, cn)
	requests := 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		handler(w, r)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	return server, &requests
}

func certificate(t *testing.T, cn string) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func writeFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(name, "extra_") {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(strings.SplitN(string(b), "\n\n", 2)[1]))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}
