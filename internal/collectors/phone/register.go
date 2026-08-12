package phone

import "github.com/rknightion/polylens2otel/internal/collector"

// ServiceTargets is the integration seam populated after target discovery.
const ServiceTargets = "phone.targets"

func Register(d collector.Deps) {
	if d.Config == nil || !d.Config.Phone.Enabled || d.Registry == nil {
		return
	}
	targets, ok := d.Service(ServiceTargets).([]Target)
	if !ok || len(targets) == 0 {
		return
	}
	d.Registry.Register(NewStatus(targets))
	d.Registry.Register(NewLines(targets))
	d.Registry.Register(NewConfig(targets, d.Config.Phone.ConfigParams))
	d.Registry.Register(NewCallLogs(targets))
	d.Registry.Register(NewNetworkInfo(targets))
}
