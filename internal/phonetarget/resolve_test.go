package phonetarget_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rknightion/polylens2otel/internal/lensclient"
	"github.com/rknightion/polylens2otel/internal/phoneclient"
	"github.com/rknightion/polylens2otel/internal/phonetarget"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetrytest"
)

func TestStaticTargetOverridesLensInternalIP(t *testing.T) {
	var contacted string
	resolver, err := phonetarget.New(phonetarget.Config{
		Targets:  map[string]string{"extra": "192.0.2.175"},
		Username: "Polycom",
		Password: phonetarget.NewSecret("configured-password-canary"),
		NewClient: func(cfg phoneclient.Config) (phonetarget.API, error) {
			contacted = cfg.BaseURL
			return stubPhone{state: phoneclient.StateOK}, nil
		},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	targets, err := resolver.Resolve(context.Background(), []lensclient.Device{{
		ID:         "extra",
		InternalIP: "192.0.2.173",
		MACAddress: "48:25:67:90:8b:97",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if contacted != "https://192.0.2.175" {
		t.Fatalf("contacted %q, want static target %q", contacted, "https://192.0.2.175")
	}
	if len(targets) != 1 || targets[0].Address != "192.0.2.175" {
		t.Fatalf("targets = %#v, want the static target", targets)
	}
}

func TestCertificateCNMismatchIsUnexpectedBeforeCredential(t *testing.T) {
	const canary = "phone-password-canary"
	var requests atomic.Int32
	server := mismatchedTLSServer(t, func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	})
	defer server.Close()

	recorder := telemetrytest.New()
	resolver, err := phonetarget.New(phonetarget.Config{
		Targets:  map[string]string{"deskie": server.URL},
		Username: "Polycom",
		Password: phonetarget.NewSecret(canary),
		Timeout:  time.Second,
	}, nil, recorder.Emitter())
	if err != nil {
		t.Fatal(err)
	}
	targets, err := resolver.Resolve(context.Background(), []lensclient.Device{{
		ID:         "deskie",
		InternalIP: "192.0.2.139",
		MACAddress: "48:25:67:90:8b:97",
	}})
	if err != nil {
		if strings.Contains(err.Error(), canary) {
			t.Fatalf("resolve error exposes password: %v", err)
		}
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %d, want mismatched phone retained for unreachable state", len(targets))
	}
	state, probeErr := targets[0].API.Probe(context.Background())
	if state != phoneclient.StateUnreachable || probeErr == nil {
		t.Fatalf("Probe() = %q, %v; want unreachable identity failure", state, probeErr)
	}
	if strings.Contains(probeErr.Error(), canary) {
		t.Fatalf("probe error exposes password: %v", probeErr)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("HTTP requests = %d, want no request or credential after CN mismatch", got)
	}
	if !recorder.HasMetric(semconv.MetricAPIUnexpected, map[string]string{semconv.AttrDeviceID: "deskie"}, 1) {
		t.Fatal("missing api.unexpected metric for certificate identity mismatch")
	}
	for _, metric := range recorder.Metrics() {
		for key, value := range metric.Attrs {
			if strings.Contains(key, canary) || strings.Contains(value, canary) {
				t.Fatalf("metric attribute exposes password: %q=%q", key, value)
			}
		}
	}
}

func TestLensPolicyPasswordUsesGetPolicyByID(t *testing.T) {
	const canary = "policy-password-canary"
	query := &stubLensQuery{response: `{"getPolicyById":{"configurationAttributes":[{"name":"device.auth.localAdminPassword","value":"` + canary + `"}]}}`}
	policy, err := phonetarget.NewLensPolicySource(query, phonetarget.PolicyIDFunc(func(_ context.Context, device lensclient.Device) (string, error) {
		if device.ID != "deskie" {
			t.Fatalf("policy requested for device %q", device.ID)
		}
		return "winning-policy", nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	var usedPassword string
	resolver, err := phonetarget.New(phonetarget.Config{
		Password:       phonetarget.NewSecret("configured-password-canary"),
		FromLensPolicy: true,
		NewClient: func(cfg phoneclient.Config) (phonetarget.API, error) {
			usedPassword = cfg.Password
			return stubPhone{state: phoneclient.StateOK}, nil
		},
	}, policy, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := resolver.Resolve(context.Background(), []lensclient.Device{{ID: "deskie", InternalIP: "192.0.2.139", MACAddress: "48:25:67:90:8b:97"}}); err != nil {
		t.Fatal(err)
	}
	if usedPassword != canary {
		t.Fatalf("phone password = %q, want policy password", usedPassword)
	}
	if !strings.Contains(query.document, "getPolicyById") || !strings.Contains(query.document, "configurationAttributes") {
		t.Fatalf("policy query does not use getPolicyById configurationAttributes: %s", query.document)
	}
	variables, ok := query.variables.(map[string]string)
	if !ok || variables["policyID"] != "winning-policy" {
		t.Fatalf("policy variables = %#v, want winning policy ID", query.variables)
	}
}

func TestPolicyFailureFallsBackWithoutLeakingCredential(t *testing.T) {
	const policyCanary = "policy-password-canary"
	const configuredCanary = "configured-password-canary"
	policy := stubPolicy{err: errors.New("upstream reflected " + policyCanary)}
	recorder := telemetrytest.New()
	var usedPassword string
	resolver, err := phonetarget.New(phonetarget.Config{
		Password:       phonetarget.NewSecret(configuredCanary),
		FromLensPolicy: true,
		NewClient: func(cfg phoneclient.Config) (phonetarget.API, error) {
			usedPassword = cfg.Password
			return stubPhone{state: phoneclient.StateOK}, nil
		},
	}, &policy, recorder.Emitter())
	if err != nil {
		t.Fatal(err)
	}

	_, err = resolver.Resolve(context.Background(), []lensclient.Device{{
		ID:         "deskie",
		Name:       policyCanary,
		InternalIP: "192.0.2.139",
		MACAddress: "48:25:67:90:8b:97",
	}})
	if err != nil {
		if strings.Contains(err.Error(), policyCanary) || strings.Contains(err.Error(), configuredCanary) {
			t.Fatalf("resolve error exposes credential: %v", err)
		}
		t.Fatal(err)
	}
	if usedPassword != configuredCanary {
		t.Fatalf("phone used password %q, want configured fallback", usedPassword)
	}
	if policy.calls != 1 {
		t.Fatalf("policy calls = %d, want 1", policy.calls)
	}
	if !recorder.HasMetric(semconv.MetricAPIUnexpected, nil, 1) {
		t.Fatal("missing api.unexpected metric for Lens policy fallback")
	}
	for _, metric := range recorder.Metrics() {
		for key, value := range metric.Attrs {
			if strings.Contains(key, policyCanary) || strings.Contains(value, policyCanary) ||
				strings.Contains(key, configuredCanary) || strings.Contains(value, configuredCanary) {
				t.Fatalf("metric attribute exposes credential: %q=%q", key, value)
			}
		}
	}
	if logs := recorder.Logs(); len(logs) != 0 {
		t.Fatalf("policy fallback wrote logs: %#v", logs)
	}
}

type stubPhone struct {
	state phoneclient.State
	err   error
}

type stubPolicy struct {
	password phonetarget.Secret
	err      error
	calls    int
}

type stubLensQuery struct {
	response  string
	document  string
	variables any
}

func (s *stubLensQuery) Query(_ context.Context, document string, variables, out any) error {
	s.document = document
	s.variables = variables
	return json.Unmarshal([]byte(s.response), out)
}

func mismatchedTLSServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "482567000000"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	server.StartTLS()
	return server
}

func (s *stubPolicy) LocalAdminPassword(context.Context, lensclient.Device) (phonetarget.Secret, error) {
	s.calls++
	return s.password, s.err
}

func (s stubPhone) Probe(context.Context) (phoneclient.State, error) {
	return s.state, s.err
}

func (stubPhone) NetworkStats(context.Context) (phoneclient.NetworkStats, error) {
	return phoneclient.NetworkStats{}, nil
}

func (stubPhone) LineInfo(context.Context) ([]phoneclient.Line, error) {
	return nil, nil
}

func (stubPhone) ConfigGet(context.Context, []string) (map[string]phoneclient.ConfigParam, []string, error) {
	return nil, nil, nil
}
