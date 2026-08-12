package config

import (
	"strings"
	"testing"
)

func TestValidatePhoneConfigParamsRejectsMoreThan50(t *testing.T) {
	params := make([]string, 51)
	if err := validatePhoneConfigParams(params); err == nil || !strings.Contains(err.Error(), "50") {
		t.Fatalf("validatePhoneConfigParams(51 parameters) error = %v, want error naming limit 50", err)
	}
}

func TestValidatePhoneConfigParamsAccepts50(t *testing.T) {
	params := make([]string, 50)
	if err := validatePhoneConfigParams(params); err != nil {
		t.Fatalf("validatePhoneConfigParams(50 parameters) error = %v, want nil", err)
	}
}
