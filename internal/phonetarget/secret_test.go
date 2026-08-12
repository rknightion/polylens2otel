package phonetarget_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rknightion/polylens2otel/internal/phonetarget"
)

func TestSecretFormattingIsAlwaysRedacted(t *testing.T) {
	const canary = "policy-password-canary"
	secret := phonetarget.NewSecret(canary)
	formats := []string{
		"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%o", "%O", "%b", "%c", "%U",
		"%e", "%E", "%f", "%F", "%g", "%G", "%p", "%T", "%t",
	}
	for _, format := range formats {
		got := fmt.Sprintf(format, secret)
		if strings.Contains(got, canary) {
			t.Fatalf("format %q exposes canary: %q", format, got)
		}
	}
	if got := fmt.Sprint(secret); got != "[redacted]" {
		t.Errorf("formatted secret = %q, want [redacted]", got)
	}

	err := fmt.Errorf("resolve phone credential: %v", secret)
	if strings.Contains(err.Error(), canary) || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("formatted error is not safely redacted: %v", err)
	}
}
