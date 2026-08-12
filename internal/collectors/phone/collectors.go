// Package phone collects the read-only phone management signals.
package phone

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rknightion/polylens2otel/internal/collector"
	"github.com/rknightion/polylens2otel/internal/phoneclient"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
)

const (
	StatusID = "phone.status"
	LinesID  = "phone.lines"
	ConfigID = "phone.config"
)

var apiStates = []phoneclient.State{
	phoneclient.StateOK,
	phoneclient.StateAPIDisabled,
	phoneclient.StateAuthFailed,
	phoneclient.StateUnreachable,
}

// ConfigAllowlist is the narrow set of configuration sources that identifies
// provisioning drift without exposing the phone's complete configuration.
var ConfigAllowlist = []string{
	"reg.1.address",
	"reg.2.address",
	"reg.1.label",
	"device.syslog.serverName",
	"tcpIpApp.sntp.address",
	"softkey.1.enable",
}

// API is the read-only subset of the phone client used by these collectors.
type API interface {
	Probe(context.Context) (phoneclient.State, error)
	NetworkStats(context.Context) (phoneclient.NetworkStats, error)
	NetworkInfo(context.Context) (phoneclient.NetworkInfo, error)
	CallLogs(context.Context) (phoneclient.CallLogs, error)
	LineInfo(context.Context) ([]phoneclient.Line, error)
	ConfigGet(context.Context, []string) (map[string]phoneclient.ConfigParam, []string, error)
}

// Target couples an already identity-checked phone API client to the Lens
// device resource attributes stamped at the emitter boundary.
type Target struct {
	TenantID string
	Device   telemetry.Device
	API      API
}

func targetEmitter(emitter telemetry.Emitter, target Target) telemetry.Emitter {
	if target.TenantID != "" {
		emitter = emitter.WithTenant(target.TenantID)
	}
	return emitter.WithDevice(target.Device)
}

type statusCollector struct{ targets []Target }

func NewStatus(targets []Target) collector.Collector {
	return statusCollector{targets: append([]Target(nil), targets...)}
}
func (statusCollector) ID() string              { return StatusID }
func (statusCollector) Interval() time.Duration { return time.Minute }
func (c statusCollector) Collect(ctx context.Context, emitter telemetry.Emitter) error {
	for _, target := range c.targets {
		state, err := target.API.Probe(ctx)
		if err != nil || !validState(state) {
			state = phoneclient.StateUnreachable
		}
		for _, candidate := range apiStates {
			value := 0.0
			if candidate == state {
				value = 1
			}
			if err := emitGauge(ctx, targetEmitter(emitter, target), semconv.MetricPhoneAPIState, value, telemetry.Attr{Key: semconv.AttrState, Value: string(candidate)}); err != nil {
				return err
			}
		}
		if state != phoneclient.StateOK {
			continue
		}
		stats, err := target.API.NetworkStats(ctx)
		if err != nil {
			return fmt.Errorf("phone %s network stats: %w", target.Device.ID, err)
		}
		uptime, err := parseUptime(stats.UpTime)
		if err != nil {
			return fmt.Errorf("phone %s network uptime: %w", target.Device.ID, err)
		}
		rx, err := parseCounter(stats.RxPackets)
		if err != nil {
			return fmt.Errorf("phone %s received packet count: %w", target.Device.ID, err)
		}
		tx, err := parseCounter(stats.TxPackets)
		if err != nil {
			return fmt.Errorf("phone %s transmitted packet count: %w", target.Device.ID, err)
		}
		deviceEmitter := targetEmitter(emitter, target)
		if err := emitGauge(ctx, deviceEmitter, semconv.MetricPhoneUptimeSeconds, uptime); err != nil {
			return err
		}
		if err := deviceEmitter.Counter(ctx, semconv.MetricPhoneNetworkPackets, rx, telemetry.Attr{Key: semconv.AttrDirection, Value: "rx"}); err != nil {
			return err
		}
		if err := deviceEmitter.Counter(ctx, semconv.MetricPhoneNetworkPackets, tx, telemetry.Attr{Key: semconv.AttrDirection, Value: "tx"}); err != nil {
			return err
		}
	}
	return nil
}

type linesCollector struct{ targets []Target }

func NewLines(targets []Target) collector.Collector {
	return linesCollector{targets: append([]Target(nil), targets...)}
}
func (linesCollector) ID() string              { return LinesID }
func (linesCollector) Interval() time.Duration { return time.Minute }
func (c linesCollector) Collect(ctx context.Context, emitter telemetry.Emitter) error {
	for _, target := range c.targets {
		state, err := target.API.Probe(ctx)
		if err != nil || state != phoneclient.StateOK {
			continue
		}
		lines, err := target.API.LineInfo(ctx)
		if err != nil {
			return fmt.Errorf("phone %s line info: %w", target.Device.ID, err)
		}
		lines = uniqueLines(lines)
		deviceEmitter := targetEmitter(emitter, target)
		if err := emitGauge(ctx, deviceEmitter, semconv.MetricPhoneLinesTotal, float64(len(lines))); err != nil {
			return err
		}
		for _, line := range lines {
			registered := 0.0
			if strings.EqualFold(line.RegistrationStatus, "registered") {
				registered = 1
			}
			if err := emitGauge(ctx, deviceEmitter, semconv.MetricPhoneLineRegistered, registered,
				telemetry.Attr{Key: semconv.AttrLine, Value: line.LineNumber},
				telemetry.Attr{Key: semconv.AttrLabel, Value: line.Label},
				telemetry.Attr{Key: semconv.AttrSIPAddress, Value: line.SIPAddress},
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// uniqueLines collapses the API's duplicate records for a single registration.
// A line number identifies the logical registration; missing line numbers are
// retained because they cannot safely be associated with another record.
func uniqueLines(lines []phoneclient.Line) []phoneclient.Line {
	seen := make(map[string]struct{}, len(lines))
	unique := make([]phoneclient.Line, 0, len(lines))
	for _, line := range lines {
		if line.LineNumber == "" {
			unique = append(unique, line)
			continue
		}
		if _, ok := seen[line.LineNumber]; ok {
			continue
		}
		seen[line.LineNumber] = struct{}{}
		unique = append(unique, line)
	}
	return unique
}

type configCollector struct {
	targets []Target
	params  []string
}

func NewConfig(targets []Target, params []string) collector.Collector {
	return configCollector{targets: append([]Target(nil), targets...), params: append([]string(nil), params...)}
}
func (configCollector) ID() string              { return ConfigID }
func (configCollector) Interval() time.Duration { return 5 * time.Minute }
func (c configCollector) Collect(ctx context.Context, emitter telemetry.Emitter) error {
	for _, target := range c.targets {
		state, err := target.API.Probe(ctx)
		if err != nil || state != phoneclient.StateOK {
			continue
		}
		params, invalid, err := target.API.ConfigGet(ctx, c.params)
		if err != nil {
			return fmt.Errorf("phone %s config get: %w", target.Device.ID, err)
		}
		if len(invalid) != 0 {
			return fmt.Errorf("phone %s config get invalid parameters: %s", target.Device.ID, strings.Join(invalid, ", "))
		}
		deviceEmitter := targetEmitter(emitter, target)
		for _, name := range c.params {
			param, ok := params[name]
			if !ok {
				return fmt.Errorf("phone %s config get omitted parameter %q", target.Device.ID, name)
			}
			if err := emitGauge(ctx, deviceEmitter, semconv.MetricPhoneConfigParamSource, 1,
				telemetry.Attr{Key: semconv.AttrParam, Value: name},
				telemetry.Attr{Key: semconv.AttrSource, Value: param.Source},
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func validState(state phoneclient.State) bool {
	switch state {
	case phoneclient.StateOK, phoneclient.StateAPIDisabled, phoneclient.StateAuthFailed, phoneclient.StateUnreachable:
		return true
	default:
		return false
	}
}

func emitGauge(ctx context.Context, emitter telemetry.Emitter, name string, value float64, attrs ...telemetry.Attr) error {
	return emitter.Gauge(ctx, name, value, attrs...)
}

func parseCounter(raw string) (float64, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse unsigned counter %q: %w", raw, err)
	}
	return float64(value), nil
}

func parseUptime(raw string) (float64, error) {
	fields := strings.Fields(strings.ToLower(raw))
	if len(fields) != 3 || (fields[1] != "day" && fields[1] != "days") {
		return 0, fmt.Errorf("parse uptime %q", raw)
	}
	days, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse uptime days %q: %w", fields[0], err)
	}
	clock := strings.Split(fields[2], ":")
	if len(clock) != 3 {
		return 0, fmt.Errorf("parse uptime clock %q", fields[2])
	}
	hours, err := strconv.ParseUint(clock[0], 10, 64)
	if err != nil || hours > 23 {
		return 0, fmt.Errorf("parse uptime hours %q", clock[0])
	}
	minutes, err := strconv.ParseUint(clock[1], 10, 64)
	if err != nil || minutes > 59 {
		return 0, fmt.Errorf("parse uptime minutes %q", clock[1])
	}
	seconds, err := strconv.ParseUint(clock[2], 10, 64)
	if err != nil || seconds > 59 {
		return 0, fmt.Errorf("parse uptime seconds %q", clock[2])
	}
	return float64(days*86400 + hours*3600 + minutes*60 + seconds), nil
}
