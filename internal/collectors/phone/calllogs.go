package phone

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rknightion/polylens2otel/internal/collector"
	"github.com/rknightion/polylens2otel/internal/phoneclient"
	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
)

const callLogsID = "phone.call_logs"

// NewCallLogs is the frozen wave-3 collector seam.
func NewCallLogs(targets []Target) collector.Collector {
	stateDir := ""
	interval := 5 * time.Minute
	if len(targets) != 0 {
		stateDir = targets[0].StateDir
		if targets[0].CallLogsInterval > 0 {
			interval = targets[0].CallLogsInterval
		}
	}
	return newCallLogs(targets, stateDir, interval)
}

type callLogsCollector struct {
	targets  []Target
	stateDir string
	interval time.Duration
}

func newCallLogs(targets []Target, stateDir string, interval time.Duration) *callLogsCollector {
	return &callLogsCollector{targets: append([]Target(nil), targets...), stateDir: stateDir, interval: interval}
}

func (*callLogsCollector) ID() string                { return callLogsID }
func (c *callLogsCollector) Interval() time.Duration { return c.interval }

func (c *callLogsCollector) Collect(ctx context.Context, emitter telemetry.Emitter) error {
	for _, target := range c.targets {
		state, err := target.API.Probe(ctx)
		if err != nil || state != phoneclient.StateOK {
			continue
		}
		checkpoint, err := c.loadWatermark(target)
		if err != nil {
			return fmt.Errorf("load phone %s call-log watermark: %w", target.Device.ID, err)
		}
		seen := make(map[string]struct{}, len(checkpoint.Keys))
		for _, key := range checkpoint.Keys {
			seen[key] = struct{}{}
		}
		logs, err := target.API.CallLogs(ctx)
		if err != nil {
			return fmt.Errorf("phone %s call logs: %w", target.Device.ID, err)
		}
		deviceEmitter := targetEmitter(emitter, target)
		callsByDirection := make(map[string]float64, 3)
		for _, entry := range callLogEntries(logs) {
			row, start, ok := parseCallLog(entry.raw)
			if !ok {
				continue
			}
			key := callLogKey(target.Device.ID, row.StartTime)
			if !callLogIsNew(key, start, checkpoint, seen) {
				continue
			}
			attrs := []telemetry.Attr{
				{Key: semconv.AttrDirection, Value: entry.direction},
				{Key: semconv.AttrRemoteParty, Value: remoteParty(row)},
				{Key: semconv.AttrLine, Value: row.LineNumber},
				{Key: semconv.AttrDurationSeconds, Value: durationSeconds(row.Duration)},
				{Key: semconv.AttrDisposition, Value: entry.direction},
				{Key: semconv.AttrStartedAt, Value: row.StartTime},
			}
			if err := deviceEmitter.LogEvent(ctx, semconv.LogPhoneCallRecord, string(entry.raw), start, attrs...); err != nil {
				return fmt.Errorf("emit phone %s call record: %w", target.Device.ID, err)
			}
			callsByDirection[entry.direction]++
			callLogAdvance(&checkpoint, key, start)
			seen[key] = struct{}{}
		}
		for _, direction := range []string{"placed", "received", "missed"} {
			if callsByDirection[direction] == 0 {
				continue
			}
			if err := deviceEmitter.Counter(ctx, semconv.MetricPhoneCallsTotal, callsByDirection[direction], telemetry.Attr{Key: semconv.AttrDirection, Value: direction}); err != nil {
				return fmt.Errorf("emit phone %s call metric: %w", target.Device.ID, err)
			}
		}
		if err := c.saveWatermark(target, checkpoint); err != nil {
			return fmt.Errorf("persist phone %s call-log watermark: %w", target.Device.ID, err)
		}
	}
	return nil
}

type callLogEntry struct {
	direction string
	raw       json.RawMessage
}

func callLogEntries(logs phoneclient.CallLogs) []callLogEntry {
	entries := make([]callLogEntry, 0, len(logs.Placed)+len(logs.Received)+len(logs.Missed))
	for _, group := range []struct {
		direction string
		rows      []json.RawMessage
	}{{"placed", logs.Placed}, {"received", logs.Received}, {"missed", logs.Missed}} {
		for _, raw := range group.rows {
			entries = append(entries, callLogEntry{direction: group.direction, raw: raw})
		}
	}
	return entries
}

type callLogRow struct {
	LineNumber        string `json:"LineNumber"`
	StartTime         string `json:"StartTime"`
	RemotePartyName   string `json:"RemotePartyName"`
	RemotePartyNumber string `json:"RemotePartyNumber"`
	Duration          string `json:"Duration"`
}

func parseCallLog(raw json.RawMessage) (callLogRow, time.Time, bool) {
	var row callLogRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return callLogRow{}, time.Time{}, false
	}
	start, err := time.ParseInLocation("2006-01-02T15:04:05", row.StartTime, time.UTC)
	if err != nil {
		return callLogRow{}, time.Time{}, false
	}
	return row, start, true
}

func remoteParty(row callLogRow) string {
	if row.RemotePartyName != "" {
		return row.RemotePartyName
	}
	return row.RemotePartyNumber
}

func durationSeconds(raw string) string {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return "0"
	}
	seconds, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return "0"
	}
	return strconv.FormatUint(seconds, 10)
}

type callLogWatermark struct {
	StartTime string   `json:"start_time"`
	Keys      []string `json:"keys"`
}

func callLogKey(deviceID, startTime string) string { return deviceID + "\x00" + startTime }

func callLogIsNew(key string, start time.Time, watermark callLogWatermark, seen map[string]struct{}) bool {
	if _, ok := seen[key]; ok {
		return false
	}
	if watermark.StartTime == "" {
		return true
	}
	checkpoint, err := time.Parse(time.RFC3339Nano, watermark.StartTime)
	return err != nil || !start.Before(checkpoint)
}

func callLogAdvance(watermark *callLogWatermark, key string, start time.Time) {
	if watermark.StartTime == "" {
		watermark.StartTime, watermark.Keys = start.Format(time.RFC3339Nano), []string{key}
		return
	}
	checkpoint, err := time.Parse(time.RFC3339Nano, watermark.StartTime)
	if err != nil || start.After(checkpoint) {
		watermark.StartTime, watermark.Keys = start.Format(time.RFC3339Nano), []string{key}
		return
	}
	if start.Equal(checkpoint) {
		watermark.Keys = append(watermark.Keys, key)
	}
}

func (c *callLogsCollector) watermarkPath(target Target) string {
	sum := sha256.Sum256([]byte(target.TenantID + "\x00" + target.Device.ID))
	return filepath.Join(c.stateDir, callLogsID, hex.EncodeToString(sum[:])+".json")
}

func (c *callLogsCollector) loadWatermark(target Target) (callLogWatermark, error) {
	if c.stateDir == "" {
		return callLogWatermark{}, nil
	}
	body, err := os.ReadFile(c.watermarkPath(target))
	if errors.Is(err, os.ErrNotExist) {
		return callLogWatermark{}, nil
	}
	if err != nil {
		return callLogWatermark{}, err
	}
	var watermark callLogWatermark
	return watermark, json.Unmarshal(body, &watermark)
}

func (c *callLogsCollector) saveWatermark(target Target, watermark callLogWatermark) error {
	if c.stateDir == "" {
		return nil
	}
	dir := filepath.Dir(c.watermarkPath(target))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	body, err := json.Marshal(watermark)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".watermark-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(body); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Chmod(0o600); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, c.watermarkPath(target))
}
