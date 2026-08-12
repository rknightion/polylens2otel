// Package phonetarget turns Lens inventory addresses into identity-checked,
// read-only phone clients. It never scans for phones.
package phonetarget

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rknightion/polylens2otel/internal/lensclient"
	"github.com/rknightion/polylens2otel/internal/phoneclient"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
)

// API is the phone-client surface consumed by the phone collectors.
type API interface {
	Probe(context.Context) (phoneclient.State, error)
	NetworkStats(context.Context) (phoneclient.NetworkStats, error)
	NetworkInfo(context.Context) (phoneclient.NetworkInfo, error)
	CallLogs(context.Context) (phoneclient.CallLogs, error)
	LineInfo(context.Context) ([]phoneclient.Line, error)
	ConfigGet(context.Context, []string) (map[string]phoneclient.ConfigParam, []string, error)
}

// PolicyPasswordSource resolves the winning Lens policy password for a device.
type PolicyPasswordSource interface {
	LocalAdminPassword(context.Context, lensclient.Device) (Secret, error)
}

// Config contains target and credential settings needed to construct clients.
type Config struct {
	Targets        map[string]string
	Username       string
	Password       Secret
	FromLensPolicy bool
	Timeout        time.Duration
	TLS            phoneclient.TLSConfig
	HTTPEmitter    telemetry.Emitter
	NewClient      func(phoneclient.Config) (API, error)
}

// Target couples the selected address to its Lens device and phone API.
type Target struct {
	Device  lensclient.Device
	Address string
	API     API
}

// Resolver resolves only Lens-named or operator-configured addresses.
type Resolver struct {
	cfg     Config
	policy  PolicyPasswordSource
	emitter telemetry.Emitter
}

func New(cfg Config, policy PolicyPasswordSource, emitter telemetry.Emitter) (*Resolver, error) {
	if cfg.NewClient == nil {
		cfg.NewClient = func(cfg phoneclient.Config) (API, error) { return phoneclient.New(cfg) }
	}
	return &Resolver{cfg: cfg, policy: policy, emitter: emitter}, nil
}

func (r *Resolver) Resolve(ctx context.Context, devices []lensclient.Device) ([]Target, error) {
	targets := make([]Target, 0, len(devices))
	for _, device := range devices {
		address := strings.TrimSpace(device.InternalIP)
		if override, ok := r.cfg.Targets[device.ID]; ok {
			address = strings.TrimSpace(override)
		}
		if address == "" {
			continue
		}
		password := r.cfg.Password
		if r.cfg.FromLensPolicy {
			var err error
			if r.policy == nil {
				err = fmt.Errorf("lens policy password source is not configured")
			} else {
				password, err = r.policy.LocalAdminPassword(ctx, device)
				if err == nil && password.empty() {
					err = fmt.Errorf("lens policy password is empty")
				}
			}
			if err != nil {
				password = r.cfg.Password
				if emitErr := r.emitUnexpected(ctx, device.ID); emitErr != nil {
					return nil, emitErr
				}
			}
		}
		client, err := r.cfg.NewClient(phoneclient.Config{
			BaseURL:   phoneURL(address),
			DeviceMAC: device.MACAddress,
			Username:  r.cfg.Username,
			Password:  password.reveal(),
			Timeout:   r.cfg.Timeout,
			TLS:       r.cfg.TLS,
			Emitter:   r.httpEmitter(device, address),
		})
		if err != nil {
			return nil, err
		}
		state, probeErr := client.Probe(ctx)
		if state == phoneclient.StateUnreachable && probeErr != nil {
			if err := r.emitUnexpected(ctx, device.ID); err != nil {
				return nil, err
			}
		}
		targets = append(targets, Target{Device: device, Address: address, API: client})
	}
	return targets, nil
}

func (r *Resolver) httpEmitter(device lensclient.Device, address string) telemetry.Emitter {
	emitter := r.cfg.HTTPEmitter
	if emitter == nil {
		return nil
	}
	if device.TenantID != "" {
		emitter = emitter.WithTenant(device.TenantID)
	}
	return emitter.WithDevice(telemetry.Device{
		ID: device.ID, Name: device.Name, MAC: device.MACAddress,
		Model: device.HardwareModel, IP: address,
	})
}

func (r *Resolver) emitUnexpected(ctx context.Context, deviceID string) error {
	if r.emitter == nil {
		return nil
	}
	if err := r.emitter.Gauge(ctx, semconv.MetricAPIUnexpected, 1, telemetry.Attr{Key: semconv.AttrDeviceID, Value: deviceID}); err != nil {
		return fmt.Errorf("emit phone target api.unexpected: %w", err)
	}
	return nil
}

func phoneURL(address string) string {
	if strings.HasPrefix(strings.ToLower(address), "https://") {
		return address
	}
	return "https://" + address
}
