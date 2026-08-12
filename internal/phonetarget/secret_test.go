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
	numericFormat := "%d"
	formatted := []string{
		fmt.Sprint(secret),
		fmt.Sprintf("%s", secret),
		fmt.Sprintf("%q", secret),
		fmt.Sprintf("%v", secret),
		fmt.Sprintf("%+v", secret),
		fmt.Sprintf("%#v", secret),
		fmt.Sprintf("%x", secret),
		fmt.Sprintf(numericFormat, secret),
	}
	for _, got := range formatted {
		if got != "[redacted]" {
			t.Errorf("formatted secret = %q, want [redacted]", got)
		}
		if strings.Contains(got, canary) {
			t.Fatalf("formatted secret exposes canary: %q", got)
		}
	}

	err := fmt.Errorf("resolve phone credential: %v", secret)
	if strings.Contains(err.Error(), canary) || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("formatted error is not safely redacted: %v", err)
	}
}
