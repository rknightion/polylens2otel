package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadPrecedenceAndDoubleUnderscoreEnv(t *testing.T) {
	t.Setenv("PL2O_OTLP__ENDPOINT", "https://env.example/otlp")
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("otlp:\n  endpoint: https://file.example/otlp\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.OTLP.Endpoint; got != "https://env.example/otlp" {
		t.Fatalf("endpoint = %q", got)
	}
	if got := cfg.Collectors.LensDevices; got != time.Minute {
		t.Fatalf("lens devices interval = %s", got)
	}
}

func TestDefaultIncludesWave3PhoneCollectorIntervals(t *testing.T) {
	t.Parallel()

	cfg := Default()
	if got := cfg.Collectors.PhoneCallLogs; got != 5*time.Minute {
		t.Fatalf("phone call logs interval = %s; want 5m", got)
	}
	if got := cfg.Collectors.PhoneNetworkInfo; got != 5*time.Minute {
		t.Fatalf("phone network info interval = %s; want 5m", got)
	}
}

func TestDefaultUsesLensPolicyForPhoneAuthentication(t *testing.T) {
	t.Parallel()

	if !Default().Phone.Auth.FromLensPolicy {
		t.Fatal("phone.auth.from_lens_policy = false; want true")
	}
}

func TestLoadRejectsSecretsInYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	secretYAML := "lens:\n  client_" + "secret: \"canary\"\n"
	if err := os.WriteFile(path, []byte(secretYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted a YAML secret")
	}
}

func TestValidateRequiresPhoneOverrideSafetyFields(t *testing.T) {
	cfg := Default()
	cfg.Lens.ClientID = "id"
	cfg.Lens.ClientSecret = "secret"
	cfg.OTLP.Endpoint = "https://example.com/otlp"
	cfg.OTLP.GrafanaCloud.InstanceID = "1"
	cfg.OTLP.GrafanaCloud.Token = "token"
	cfg.Phone.Auth.Password = "password"
	cfg.Phone.Targets = map[string]string{"device": "phone.example"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAllowsStaticPhoneDeploymentWithoutLensCredentials(t *testing.T) {
	cfg := Default()
	cfg.OTLP.Endpoint = "https://example.com/otlp"
	cfg.OTLP.GrafanaCloud.InstanceID = "1"
	cfg.OTLP.GrafanaCloud.Token = "token"
	cfg.Phone.Auth.Password = "password"
	cfg.Phone.Targets = map[string]string{"482567000001": "phone.example"}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v; want static phone-only deployment accepted", err)
	}
}

func TestValidateRejectsUnverifiableStaticPhoneIdentityWithoutLensCredentials(t *testing.T) {
	cfg := Default()
	cfg.OTLP.Endpoint = "https://example.com/otlp"
	cfg.OTLP.GrafanaCloud.InstanceID = "1"
	cfg.OTLP.GrafanaCloud.Token = "token"
	cfg.Phone.Auth.Password = "password"
	cfg.Phone.Targets = map[string]string{"device-alias": "phone.example"}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "12-hex-digit MAC") {
		t.Fatalf("Validate() error = %v; want static target identity error", err)
	}
}
