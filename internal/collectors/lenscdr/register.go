package lenscdr

import "github.com/rknightion/polylens2otel/internal/collector"

// Register adds one CDR collector for each configured Lens tenant.
func Register(d collector.Deps) {
	if d.Config == nil || d.Registry == nil {
		return
	}
	client, ok := d.Service("lensclient").(QueryClient)
	if !ok {
		return
	}
	for _, tenantID := range d.Config.Lens.Tenants {
		c := New(client, d.Config.State.Dir, tenantID)
		c.interval = d.Config.Collectors.LensCDR
		d.Registry.Register(c)
	}
}
