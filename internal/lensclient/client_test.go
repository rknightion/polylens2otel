package lensclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetrytest"
)

func TestTokenRequestUsesJSONAndCachesToken(t *testing.T) {
	var tokenCalls, queryCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenCalls++
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("content-type = %q, want application/json", got)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["grant_type"] != "client_credentials" || body["client_id"] != "client" || body["client_secret"] != "secret" {
				t.Errorf("unexpected token body: %#v", body)
			}
			_, _ = io.WriteString(w, fixture(t, "token.json"))
		case "/graphql":
			queryCalls++
			if got := r.Header.Get("authorization"); got != "Bearer [redacted]" {
				t.Errorf("authorization = %q, want lowercase bearer value", got)
			}
			_, _ = io.WriteString(w, `{"data":{"tenants":[]}}`)
		}
	}))
	defer server.Close()

	c := newTestClient(t, server.URL)
	for range 2 {
		if _, err := c.Tenants(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if tokenCalls != 1 || queryCalls != 2 {
		t.Fatalf("token calls=%d query calls=%d, want 1 and 2", tokenCalls, queryCalls)
	}
}

func TestTokenRefreshEmitsLifecycleMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, fixture(t, "token.json"))
			return
		}
		_, _ = io.WriteString(w, `{"data":{"tenants":[]}}`)
	}))
	defer server.Close()
	recorder := telemetrytest.New()
	c, err := New(Config{
		TokenURL: server.URL + "/token", GraphQLURL: server.URL + "/graphql",
		ClientID: "client", ClientSecret: "secret", Emitter: recorder.Emitter(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.AccessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	attrs := map[string]string{semconv.AttrSource: "lens"}
	if !recorder.HasMetric(semconv.MetricAuthTokenRefresh, attrs, 1) {
		t.Fatal("missing token refresh counter")
	}
	if !recorder.HasMetric(semconv.MetricAuthTokenExpiresSeconds, attrs, 86400) {
		t.Fatal("missing token expiry gauge")
	}
}

func TestGraphQL4xxPreservesResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, fixture(t, "token.json"))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, fixture(t, "error_validation.json"))
	}))
	defer server.Close()

	var out struct{}
	err := newTestClient(t, server.URL).Query(context.Background(), `{ typo }`, nil, &out)
	if err == nil || !strings.Contains(err.Error(), `Did you mean \"lastDetected\"`) {
		t.Fatalf("error = %v; body was not retained", err)
	}
}

func TestGraphQLUsesLowercaseBearerAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, fixture(t, "token.json"))
			return
		}
		if got := r.Header.Get("authorization"); got != "Bearer [redacted]" {
			t.Errorf("authorization = %q, want Bearer token", got)
		}
		_, _ = io.WriteString(w, `{"data":{"tenants":[]}}`)
	}))
	defer server.Close()
	if _, err := newTestClient(t, server.URL).Tenants(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDevicePaginationRetainsPageSize(t *testing.T) {
	var pages []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, fixture(t, "token.json"))
			return
		}
		var request struct {
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		pages = append(pages, request.Variables)
		if len(pages) == 1 {
			_, _ = io.WriteString(w, `{"data":{"tenant":{"inventory":{"deviceSearch":{"pageInfo":{"totalCount":2,"hasNextPage":true,"nextToken":"next"},"edges":[{"node":{"id":"one","name":"one"}}]}}}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"tenant":{"inventory":{"deviceSearch":{"pageInfo":{"totalCount":2,"hasNextPage":false,"nextToken":null},"edges":[{"node":{"id":"two","name":"two"}}]}}}}}`)
	}))
	defer server.Close()

	devices, err := newTestClient(t, server.URL).Devices(context.Background(), "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("devices = %#v", devices)
	}
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(pages))
	}
	for i, page := range pages {
		params := page["params"].(map[string]any)
		if got := int(params["pageSize"].(float64)); got != 17 {
			t.Errorf("page %d pageSize = %d, want 17", i+1, got)
		}
	}
	if pages[1]["params"].(map[string]any)["nextToken"] != "next" {
		t.Error("follow-up did not carry nextToken")
	}
}

func TestQueryRejectsMutationBeforeTransport(t *testing.T) {
	var calls atomic.Int32
	c, err := New(Config{TokenURL: "http://example.invalid/token", GraphQLURL: "http://example.invalid/graphql", ClientID: "client", ClientSecret: "secret", HTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) { calls.Add(1); return nil, nil })}})
	if err != nil {
		t.Fatal(err)
	}
	var out struct{}
	err = c.Query(context.Background(), `mutation { rebootDevice }`, nil, &out)
	if err == nil || !strings.Contains(err.Error(), "mutation") {
		t.Fatalf("error = %v, want mutation rejection", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("transport calls = %d, want 0", calls.Load())
	}
}

func TestQueryRetriesTransientResponse(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, fixture(t, "token.json"))
			return
		}
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "temporarily unavailable")
			return
		}
		_, _ = io.WriteString(w, `{"data":{"tenants":[]}}`)
	}))
	defer server.Close()
	c := newTestClient(t, server.URL)
	c.sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := c.Tenants(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("graphql calls = %d, want 2", calls)
	}
}

func TestQueryRetriesLensHTTP400RateLimitAndIsBounded(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, fixture(t, "token.json"))
			return
		}
		calls++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errors":[{"message":"Rate limit exceeded"}]}`)
	}))
	defer server.Close()
	c := newTestClient(t, server.URL)
	c.sleep = func(context.Context, time.Duration) error { return nil }
	var out struct{}
	err := c.Query(context.Background(), `{ tenants { id } }`, nil, &out)
	if err == nil || !strings.Contains(err.Error(), "Rate limit exceeded") {
		t.Fatalf("error = %v", err)
	}
	if calls != c.cfg.MaxAttempts {
		t.Fatalf("graphql calls = %d, want bounded %d attempts", calls, c.cfg.MaxAttempts)
	}
}

func TestQueryGraphQLEnvelopeErrorDoesNotReflectData(t *testing.T) {
	const canary = "policy-password-canary"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, fixture(t, "token.json"))
			return
		}
		_, _ = io.WriteString(w, `{"data":{"configurationAttributes":[{"value":"`+canary+`"}]},"errors":[{"message":"field resolver failed"}]}`)
	}))
	defer server.Close()
	var out struct{}
	err := newTestClient(t, server.URL).Query(context.Background(), `{ getPolicyById { configurationAttributes { value } } }`, nil, &out)
	if err == nil || !strings.Contains(err.Error(), "field resolver failed") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatalf("error reflected GraphQL data: %v", err)
	}
}

func TestGoldenFixturesDecodeReadOnlyQueries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, fixture(t, "token.json"))
			return
		}
		var request struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch {
		case strings.Contains(request.Query, "deviceSearch"):
			_, _ = io.WriteString(w, fixture(t, "devicesearch.json"))
		case strings.Contains(request.Query, "activeCalls"):
			_, _ = io.WriteString(w, fixture(t, "activecalls.json"))
		case strings.Contains(request.Query, "availableProductSoftwareByPid"):
			_, _ = io.WriteString(w, fixture(t, "firmware.json"))
		default:
			t.Fatalf("unexpected query: %s", request.Query)
		}
	}))
	defer server.Close()
	c := newTestClient(t, server.URL)
	devices, err := c.Devices(context.Background(), "tenant")
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 || devices[0].Name != "deskie" || devices[1].MACAddress != "48:25:67:90:8b:97" {
		t.Fatalf("devices = %#v", devices)
	}
	calls, err := c.ActiveCalls(context.Background(), devices[0].ID)
	if err != nil || len(calls) != 0 {
		t.Fatalf("calls = %#v, err = %v", calls, err)
	}
	firmware, err := c.LatestFirmware(context.Background(), devices[0].ProductID)
	if err != nil || firmware.Version != "8.6.0.1321" || firmware.ReleaseChannel != nil {
		t.Fatalf("firmware = %#v, err = %v", firmware, err)
	}
}

func TestHTTPErrorRedactsConfiguredClientSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, fixture(t, "token.json"))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `server repeated secret`)
	}))
	defer server.Close()
	c := newTestClient(t, server.URL)
	c.cfg.ClientSecret = "secret"
	var out struct{}
	err := c.Query(context.Background(), `{ tenants { id } }`, nil, &out)
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("error = %v", err)
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	c, err := New(Config{TokenURL: baseURL + "/token", GraphQLURL: baseURL + "/graphql", ClientID: "client", ClientSecret: "secret", PageSize: 17, MaxAttempts: 2, MinBackoff: time.Millisecond, MaxBackoff: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
