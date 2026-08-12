package phone

import (
	"context"
	"fmt"
	"time"

	"github.com/rknightion/polylens2otel/internal/collector"
	"github.com/rknightion/polylens2otel/internal/phoneclient"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
)

const NetworkInfoID = "phone.network_info"

type networkInfoCollector struct {
	targets  []Target
	interval time.Duration
}

func NewNetworkInfo(targets []Target) collector.Collector {
	interval := 5 * time.Minute
	if len(targets) != 0 && targets[0].NetworkInfoInterval > 0 {
		interval = targets[0].NetworkInfoInterval
	}
	return networkInfoCollector{targets: append([]Target(nil), targets...), interval: interval}
}

func (networkInfoCollector) ID() string                { return NetworkInfoID }
func (c networkInfoCollector) Interval() time.Duration { return c.interval }

func (c networkInfoCollector) Collect(ctx context.Context, emitter telemetry.Emitter) error {
	for _, target := range c.targets {
		state, err := target.API.Probe(ctx)
		if err != nil || state != phoneclient.StateOK {
			continue
		}
		info, err := target.API.NetworkInfo(ctx)
		if err != nil {
			return fmt.Errorf("phone %s network info: %w", target.Device.ID, err)
		}

		// Network-info payloads include the phone's address. It is deliberately
		// omitted from metric labels to avoid exposing a host identifier.
		target.Device.IP = ""
		if err := emitGauge(ctx, targetEmitter(emitter, target), semconv.MetricPhoneNetworkInfo, 1,
			telemetry.Attr{Key: semconv.AttrDHCPEnabled, Value: info.DHCP},
			telemetry.Attr{Key: semconv.AttrDHCPServer, Value: info.DHCPServer},
			telemetry.Attr{Key: semconv.AttrDefaultGateway, Value: info.DefaultGateway},
			telemetry.Attr{Key: semconv.AttrSubnetMask, Value: info.SubnetMask},
			telemetry.Attr{Key: semconv.AttrBootServerOption, Value: info.DHCPBootServerOption},
		); err != nil {
			return err
		}
	}
	return nil
}
