package lens

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rknightion/polylens2otel/internal/collector"
	"github.com/rknightion/polylens2otel/internal/config"
	"github.com/rknightion/polylens2otel/internal/lensclient"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
)

const (
	serviceName   = "lens"
	devicesID     = "lens.devices"
	activeCallsID = "lens.active_calls"
	firmwareID    = "lens.firmware"
)

var errClientUnavailable = errors.New("Lens collector requires the lens service")

type client interface {
	Devices(context.Context, string) ([]lensclient.Device, error)
	ActiveCalls(context.Context, string) ([]lensclient.Call, error)
	LatestFirmware(context.Context, string) (lensclient.Firmware, error)
}

type tenantClient interface {
	Tenants(context.Context) ([]lensclient.Tenant, error)
}

type deviceTenant struct {
	tenantID string
	device   lensclient.Device
}

type baseCollector struct {
	id         string
	interval   time.Duration
	client     client
	tenantIDs  []string
	maxDevices int
}

func (c baseCollector) ID() string              { return c.id }
func (c baseCollector) Interval() time.Duration { return c.interval }

type devicesCollector struct {
	baseCollector
	now func() time.Time
}

type activeCallsCollector struct{ baseCollector }
type firmwareCollector struct{ baseCollector }

// Register adds the polling Lens collectors to the registry.
func Register(d collector.Deps) {
	if d.Registry == nil {
		return
	}
	c, _ := d.Service(serviceName).(client)
	cfg := collectorConfig(d.Config)
	d.Registry.Register(&devicesCollector{baseCollector: baseCollector{
		id: devicesID, interval: cfg.LensDevices, client: c, tenantIDs: cfg.Tenants, maxDevices: cfg.MaxDevices,
	}, now: time.Now})
	d.Registry.Register(&activeCallsCollector{baseCollector: baseCollector{
		id: activeCallsID, interval: cfg.LensActiveCalls, client: c, tenantIDs: cfg.Tenants, maxDevices: cfg.MaxDevices,
	}})
	d.Registry.Register(&firmwareCollector{baseCollector: baseCollector{
		id: firmwareID, interval: cfg.LensFirmware, client: c, tenantIDs: cfg.Tenants, maxDevices: cfg.MaxDevices,
	}})
}

type lensCollectorConfig struct {
	Tenants                                    []string
	LensDevices, LensActiveCalls, LensFirmware time.Duration
	MaxDevices                                 int
}

func collectorConfig(cfg *config.Config) lensCollectorConfig {
	if cfg == nil {
		return lensCollectorConfig{LensDevices: time.Minute, LensActiveCalls: time.Minute, LensFirmware: 24 * time.Hour, MaxDevices: 1}
	}
	return lensCollectorConfig{
		Tenants:         append([]string(nil), cfg.Lens.Tenants...),
		LensDevices:     cfg.Collectors.LensDevices,
		LensActiveCalls: cfg.Collectors.LensActiveCalls,
		LensFirmware:    cfg.Collectors.LensFirmware,
		MaxDevices:      cfg.Cardinality.MaxDevices,
	}
}

func (c *devicesCollector) Collect(ctx context.Context, emitter telemetry.Emitter) error {
	devices, err := c.devices(ctx, emitter)
	if err != nil {
		return err
	}
	for _, entry := range devices {
		deviceEmitter := withDevice(emitter.WithTenant(entry.tenantID), entry.device)
		connected := 0.0
		if entry.device.Connected {
			connected = 1
		}
		if err := deviceEmitter.Gauge(ctx, semconv.MetricLensDeviceConnected, connected); err != nil {
			return err
		}
		if err := c.emitAge(ctx, deviceEmitter, semconv.MetricLensDeviceLastDetectedSeconds, entry.device.LastDetected); err != nil {
			return err
		}
		if err := c.emitAge(ctx, deviceEmitter, semconv.MetricLensDeviceLastConfigRequestSeconds, entry.device.LastConfigRequestDate); err != nil {
			return err
		}
		if err := deviceEmitter.Gauge(ctx, semconv.MetricLensDeviceFirmwareInfo, 1,
			telemetry.Attr{Key: semconv.AttrVersion, Value: entry.device.SoftwareVersion},
			telemetry.Attr{Key: semconv.AttrBuild, Value: entry.device.SoftwareBuild},
		); err != nil {
			return err
		}
	}
	return nil
}

func (c *devicesCollector) emitAge(ctx context.Context, emitter telemetry.Emitter, metricName, timestamp string) error {
	observed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return nil
	}
	age := c.now().Sub(observed).Seconds()
	if age < 0 {
		age = 0
	}
	return emitter.Gauge(ctx, metricName, age)
}

func (c *activeCallsCollector) Collect(ctx context.Context, emitter telemetry.Emitter) error {
	devices, err := c.devices(ctx, emitter)
	if err != nil {
		return err
	}
	for _, entry := range devices {
		calls, err := c.client.ActiveCalls(ctx, entry.device.ID)
		if err != nil {
			return fmt.Errorf("list active calls for device %s: %w", entry.device.ID, err)
		}
		byState := make(map[string]float64)
		for _, call := range calls {
			byState[call.State]++
		}
		deviceEmitter := withDevice(emitter.WithTenant(entry.tenantID), entry.device)
		for state, count := range byState {
			if err := deviceEmitter.Gauge(ctx, semconv.MetricLensDeviceActiveCalls, count, telemetry.Attr{Key: semconv.AttrState, Value: state}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *firmwareCollector) Collect(ctx context.Context, emitter telemetry.Emitter) error {
	devices, err := c.devices(ctx, emitter)
	if err != nil {
		return err
	}
	for _, entry := range devices {
		firmware, err := c.client.LatestFirmware(ctx, entry.device.ProductID)
		if err != nil {
			return fmt.Errorf("get latest firmware for device %s: %w", entry.device.ID, err)
		}
		current := 0.0
		if entry.device.SoftwareVersion == firmware.Version {
			current = 1
		}
		if err := withDevice(emitter.WithTenant(entry.tenantID), entry.device).Gauge(ctx, semconv.MetricLensDeviceFirmwareCurrent, current); err != nil {
			return err
		}
	}
	return nil
}

func (c baseCollector) devices(ctx context.Context, emitter telemetry.Emitter) ([]deviceTenant, error) {
	if c.client == nil {
		return nil, errClientUnavailable
	}
	tenantIDs, err := c.resolveTenants(ctx)
	if err != nil {
		return nil, err
	}
	devices := make([]deviceTenant, 0)
	for _, tenantID := range tenantIDs {
		found, err := c.client.Devices(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("list devices for tenant %s: %w", tenantID, err)
		}
		for _, device := range found {
			devices = append(devices, deviceTenant{tenantID: tenantID, device: device})
		}
	}
	if len(devices) > c.maxDevices {
		if err := emitter.Gauge(ctx, semconv.MetricAPIUnexpected, 1, telemetry.Attr{Key: semconv.AttrSource, Value: "lens"}); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("Lens device count %d exceeds cardinality.max_devices %d", len(devices), c.maxDevices)
	}
	return devices, nil
}

func (c baseCollector) resolveTenants(ctx context.Context) ([]string, error) {
	if len(c.tenantIDs) > 0 {
		return c.tenantIDs, nil
	}
	tenantLookup, ok := c.client.(tenantClient)
	if !ok {
		return nil, errors.New("Lens tenant discovery requires a client with Tenants")
	}
	tenants, err := tenantLookup.Tenants(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Lens tenants: %w", err)
	}
	ids := make([]string, 0, len(tenants))
	for _, tenant := range tenants {
		ids = append(ids, tenant.ID)
	}
	return ids, nil
}

func withDevice(emitter telemetry.Emitter, device lensclient.Device) telemetry.Emitter {
	site := ""
	if device.Site != nil {
		site = device.Site.Name
	}
	return emitter.WithDevice(telemetry.Device{
		ID: device.ID, Name: device.Name, MAC: device.MACAddress, Model: device.HardwareModel, Site: site, IP: device.InternalIP,
	})
}
