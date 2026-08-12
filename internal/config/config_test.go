package config

import (
	"os"
	"path/filepath"
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
