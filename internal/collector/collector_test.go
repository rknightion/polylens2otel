package collector

import "testing"

func TestRegistryRegisterSkipsNilCollector(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	registry.Register(nil)

	if got := len(registry.Entries()); got != 0 {
		t.Fatalf("registered collectors = %d; want 0", got)
	}
}
