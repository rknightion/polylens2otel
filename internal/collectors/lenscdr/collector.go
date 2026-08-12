// Package lenscdr collects Poly Lens call-detail records as OTLP logs.
package lenscdr

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
	"time"

	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
)

const collectorID = "lens.cdr"

const query = `query MeetingRecordLists($tenantID: ID!, $after: String) {
  meetingRecordLists(tenantId: [$tenantID], meetingRecordType: [CDR], timeRange: {relativeRange: {increment: DAY, value: 2}}, first: 100, after: $after) {
    pageInfo { hasNextPage endCursor }
    edges { node { deviceId mac startTime endTime callDirection protocol disconnectInfo remoteParty hardwareModel ipAddress softwareRelease userId productFamily answeredInSoftphone softphoneVersion callRate } }
  }
}`

// QueryClient is the read-only Lens query seam used by this collector.
type QueryClient interface {
	Query(context.Context, string, any, any) error
}

// Collector emits one log for each new Lens CDR row.
type Collector struct {
	client   QueryClient
	stateDir string
	tenantID string
	interval time.Duration
}

type cdr struct {
	DeviceID          string  `json:"deviceId"`
	MAC               string  `json:"mac"`
	StartTime         string  `json:"startTime"`
	EndTime           string  `json:"endTime"`
	CallDirection     string  `json:"callDirection"`
	HardwareModel     string  `json:"hardwareModel"`
	IPAddress         string  `json:"ipAddress"`
	Protocol          string  `json:"protocol"`
	DisconnectInfo    string  `json:"disconnectInfo"`
	RemoteParty       string  `json:"remoteParty"`
	SoftwareRelease   string  `json:"softwareRelease"`
	UserID            *string `json:"userId"`
	ProductFamily     string  `json:"productFamily"`
	AnsweredSoftphone *bool   `json:"answeredInSoftphone"`
	SoftphoneVersion  *string `json:"softphoneVersion"`
	CallRate          *string `json:"callRate"`
}

type watermark struct {
	StartTime string   `json:"start_time"`
	Keys      []string `json:"keys"`
}

// New creates a CDR collector for one tenant.
func New(client QueryClient, stateDir, tenantID string) *Collector {
	return &Collector{client: client, stateDir: stateDir, tenantID: tenantID, interval: time.Hour}
}

func (c *Collector) ID() string              { return collectorID }
func (c *Collector) Interval() time.Duration { return c.interval }

// Collect obtains every available page. The API supplies no totalCount, so
// pagination stops only when hasNextPage is false.
func (c *Collector) Collect(ctx context.Context, emitter telemetry.Emitter) error {
	if c.client == nil {
		return errors.New("lens CDR client is required")
	}
	wm, err := c.loadWatermark()
	if err != nil {
		return fmt.Errorf("load CDR watermark: %w", err)
	}
	seen := make(map[string]struct{}, len(wm.Keys))
	for _, key := range wm.Keys {
		seen[key] = struct{}{}
	}
	checkpoint := wm
	tenantEmitter := emitter.WithTenant(c.tenantID)
	var after *string
	for {
		var response struct {
			MeetingRecordLists struct {
				PageInfo struct {
					HasNextPage bool    `json:"hasNextPage"`
					EndCursor   *string `json:"endCursor"`
				} `json:"pageInfo"`
				Edges []struct {
					Node cdr `json:"node"`
				} `json:"edges"`
			} `json:"meetingRecordLists"`
		}
		if err := c.client.Query(ctx, query, map[string]any{"tenantID": c.tenantID, "after": after}, &response); err != nil {
			return fmt.Errorf("query Lens CDRs: %w", err)
		}
		for _, edge := range response.MeetingRecordLists.Edges {
			_ = tenantEmitter.Counter(ctx, semconv.MetricIngestSourceRecords, 1, telemetry.Attr{Key: semconv.AttrCollectorID, Value: collectorID})
			start, end, ok := eventTimes(edge.Node)
			if !ok {
				continue
			}
			key := dedupeKey(edge.Node.DeviceID, edge.Node.StartTime)
			if !isNew(key, start, checkpoint, seen) {
				continue
			}
			body, err := json.Marshal(edge.Node)
			if err != nil {
				return fmt.Errorf("encode CDR log body: %w", err)
			}
			duration := end.Sub(start).Seconds()
			attrs := []telemetry.Attr{
				{Key: semconv.AttrDeviceID, Value: edge.Node.DeviceID},
				{Key: semconv.AttrDeviceMAC, Value: edge.Node.MAC},
				{Key: semconv.AttrDeviceModel, Value: edge.Node.HardwareModel},
				{Key: semconv.AttrNetHostIP, Value: edge.Node.IPAddress},
				{Key: semconv.AttrDirection, Value: edge.Node.CallDirection},
				{Key: semconv.AttrDurationSeconds, Value: strconv.FormatFloat(duration, 'f', -1, 64)},
			}
			if err := tenantEmitter.LogEvent(ctx, semconv.EventLensCDR, string(body), start, nonEmpty(attrs)...); err != nil {
				return fmt.Errorf("emit CDR log: %w", err)
			}
			_ = tenantEmitter.Counter(ctx, semconv.MetricIngestEmittedPoints, 1, telemetry.Attr{Key: semconv.AttrCollectorID, Value: collectorID})
			advanceWatermark(&wm, key, start)
			seen[key] = struct{}{}
		}
		page := response.MeetingRecordLists.PageInfo
		if !page.HasNextPage {
			break
		}
		if page.EndCursor == nil || *page.EndCursor == "" {
			return errors.New("lens CDR pagination reported another page without endCursor")
		}
		after = page.EndCursor
	}
	if err := c.saveWatermark(wm); err != nil {
		_ = tenantEmitter.Counter(ctx, semconv.MetricCheckpointPersistErrors, 1, telemetry.Attr{Key: semconv.AttrCollectorID, Value: collectorID})
		return fmt.Errorf("persist CDR watermark: %w", err)
	}
	return nil
}

func eventTimes(row cdr) (time.Time, time.Time, bool) {
	start, err := time.Parse(time.RFC3339Nano, row.StartTime)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	end, err := time.Parse(time.RFC3339Nano, row.EndTime)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	return start, end, true
}

func dedupeKey(deviceID, startTime string) string { return deviceID + "\x00" + startTime }

func isNew(key string, start time.Time, wm watermark, seen map[string]struct{}) bool {
	if _, duplicate := seen[key]; duplicate {
		return false
	}
	if wm.StartTime == "" {
		return true
	}
	checkpoint, err := time.Parse(time.RFC3339Nano, wm.StartTime)
	if err != nil || start.After(checkpoint) {
		return true
	}
	if start.Before(checkpoint) {
		return false
	}
	return true
}

func advanceWatermark(wm *watermark, key string, start time.Time) {
	if wm.StartTime == "" {
		wm.StartTime, wm.Keys = start.Format(time.RFC3339Nano), []string{key}
		return
	}
	checkpoint, err := time.Parse(time.RFC3339Nano, wm.StartTime)
	if err != nil || start.After(checkpoint) {
		wm.StartTime, wm.Keys = start.Format(time.RFC3339Nano), []string{key}
		return
	}
	if start.Equal(checkpoint) {
		wm.Keys = append(wm.Keys, key)
	}
}

func nonEmpty(attrs []telemetry.Attr) []telemetry.Attr {
	out := attrs[:0]
	for _, attr := range attrs {
		if attr.Value != "" {
			out = append(out, attr)
		}
	}
	return out
}

func (c *Collector) watermarkPath() string {
	sum := sha256.Sum256([]byte(c.tenantID))
	return filepath.Join(c.stateDir, collectorID, hex.EncodeToString(sum[:])+".json")
}

func (c *Collector) loadWatermark() (watermark, error) {
	b, err := os.ReadFile(c.watermarkPath())
	if errors.Is(err, os.ErrNotExist) {
		return watermark{}, nil
	}
	if err != nil {
		return watermark{}, err
	}
	var wm watermark
	if err := json.Unmarshal(b, &wm); err != nil {
		return watermark{}, err
	}
	return wm, nil
}

func (c *Collector) saveWatermark(wm watermark) error {
	if err := os.MkdirAll(filepath.Dir(c.watermarkPath()), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(wm)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.watermarkPath()), ".watermark-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(b); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Chmod(0o600); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, c.watermarkPath())
}
