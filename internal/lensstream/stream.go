// Package lensstream implements the edge-triggered Lens device subscription.
package lensstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
)

const (
	subprotocol      = "graphql-transport-ws"
	connectionInit   = "connection_init"
	connectionAck    = "connection_ack"
	subscribeMessage = "subscribe"
	nextMessage      = "next"
	completeMessage  = "complete"
	operationID      = "1"
)

const subscription = `subscription DevStream($deviceIDs: [ID!]!) {
  deviceStream(deviceIds: $deviceIDs) {
    connected externalIp hardwareRevision id macAddress modelId name productId roomId siteId softwareBuild softwareVersion tenantId
  }
}`

// Config contains the subscription's runtime settings. Token or TokenSource
// must return environment-backed application credentials; neither is included
// in errors.
type Config struct {
	URL         string
	Token       string
	TokenSource func(context.Context) (string, error)
	DeviceIDs   []string
	AckTimeout  time.Duration
	MinBackoff  time.Duration
	MaxBackoff  time.Duration
	Emitter     telemetry.Emitter
}

// Stream owns one graphql-transport-ws connection for all configured devices.
type Stream struct {
	cfg Config
}

// New validates the stream settings.
func New(cfg Config) *Stream {
	return &Stream{cfg: cfg}
}

// Run reconnects until ctx is cancelled. It deliberately imposes no deadline
// after acknowledgement: deviceStream is edge-triggered and is quiet when no
// device attribute changes.
func (s *Stream) Run(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	for failures := 0; ; failures++ {
		if failures > 0 {
			if err := s.cfg.Emitter.Counter(ctx, semconv.MetricStreamReconnects, 1); err != nil {
				return fmt.Errorf("emit stream reconnect: %w", err)
			}
			if err := sleepContext(ctx, s.backoff(failures)); err != nil {
				return contextResult(err)
			}
		}
		err := s.session(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			return nil
		}
	}
}

func (s *Stream) validate() error {
	if strings.TrimSpace(s.cfg.URL) == "" {
		return errors.New("lens stream URL is required")
	}
	if strings.TrimSpace(s.cfg.Token) == "" && s.cfg.TokenSource == nil {
		return errors.New("lens stream token is required")
	}
	if len(s.cfg.DeviceIDs) == 0 {
		return errors.New("lens stream requires at least one device ID")
	}
	if s.cfg.Emitter == nil {
		return errors.New("lens stream emitter is required")
	}
	if s.cfg.AckTimeout <= 0 {
		return errors.New("lens stream acknowledgement timeout must be positive")
	}
	if s.cfg.MinBackoff <= 0 || s.cfg.MaxBackoff < s.cfg.MinBackoff {
		return errors.New("invalid Lens stream reconnect backoff")
	}
	return nil
}

func (s *Stream) session(ctx context.Context) (err error) {
	token := s.cfg.Token
	if s.cfg.TokenSource != nil {
		var err error
		token, err = s.cfg.TokenSource(ctx)
		if err != nil {
			return fmt.Errorf("get Lens stream token: %w", err)
		}
	}
	if strings.TrimSpace(token) == "" {
		return errors.New("lens stream token source returned an empty token")
	}
	conn, response, dialErr := websocket.Dial(ctx, s.cfg.URL, &websocket.DialOptions{
		HTTPHeader:   http.Header{"authorization": []string{"Bearer " + token}},
		Subprotocols: []string{subprotocol},
	})
	var responseCloseErr error
	if response != nil && response.Body != nil {
		// Dial closes the handshake body itself; the explicit close documents
		// ownership for static analysis and is safe on the already-closed body.
		responseCloseErr = response.Body.Close()
	}
	if dialErr != nil {
		return errors.Join(fmt.Errorf("dial Lens stream: %w", dialErr), responseCloseErr)
	}
	if responseCloseErr != nil {
		return errors.Join(fmt.Errorf("close Lens stream handshake response: %w", responseCloseErr), conn.CloseNow())
	}
	defer func() { err = errors.Join(err, conn.CloseNow()) }()

	if err := s.write(ctx, conn, frame{Type: connectionInit, Payload: map[string]any{"authorization": "Bearer " + token}}); err != nil {
		return fmt.Errorf("send Lens stream connection_init: %w", err)
	}
	ackCtx, cancel := context.WithTimeout(ctx, s.cfg.AckTimeout)
	ack, err := s.read(ackCtx, conn)
	cancel()
	if err != nil {
		return fmt.Errorf("wait for Lens stream acknowledgement: %w", err)
	}
	if ack.Type != connectionAck {
		return fmt.Errorf("unexpected Lens stream acknowledgement frame type %q", ack.Type)
	}
	if err := s.write(ctx, conn, frame{ID: operationID, Type: subscribeMessage, Payload: map[string]any{
		"query": subscription, "variables": map[string]any{"deviceIDs": s.cfg.DeviceIDs},
	}}); err != nil {
		return fmt.Errorf("start Lens stream subscription: %w", err)
	}
	if err := s.cfg.Emitter.Gauge(ctx, semconv.MetricLensStreamConnected, 1); err != nil {
		return fmt.Errorf("emit connected Lens stream: %w", err)
	}
	defer s.cfg.Emitter.Gauge(context.Background(), semconv.MetricLensStreamConnected, 0) //nolint:errcheck // context cancellation must not suppress the disconnect measurement.

	for {
		message, err := s.read(ctx, conn)
		if err != nil {
			return fmt.Errorf("read Lens stream: %w", err)
		}
		switch message.Type {
		case nextMessage:
			if err := s.emitDevice(ctx, message.Payload); err != nil {
				return err
			}
		case completeMessage:
			return errors.New("lens stream completed")
		case "error":
			return errors.New("lens stream returned a subscription error")
		}
	}
}

func (s *Stream) emitDevice(ctx context.Context, payload map[string]any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Lens stream payload: %w", err)
	}
	var result struct {
		Data struct {
			Device device `json:"deviceStream"`
		} `json:"data"`
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return fmt.Errorf("decode Lens stream payload: %w", err)
	}
	d := result.Data.Device
	if d.ID == "" || d.TenantID == "" {
		return errors.New("lens stream device payload missing id or tenantId")
	}
	emitter := s.cfg.Emitter.WithTenant(d.TenantID).WithDevice(telemetry.Device{
		ID: d.ID, Name: d.Name, MAC: d.MACAddress, Model: d.ProductID, IP: d.ExternalIP,
	})
	connected := 0.0
	if d.Connected {
		connected = 1
	}
	if err := emitter.Gauge(ctx, semconv.MetricLensDeviceConnected, connected); err != nil {
		return fmt.Errorf("emit Lens streamed device connection: %w", err)
	}
	if err := emitter.Counter(ctx, semconv.MetricStreamMessages, 1); err != nil {
		return fmt.Errorf("emit Lens stream message: %w", err)
	}
	if err := emitter.Gauge(ctx, semconv.MetricStreamLastMessageSeconds, 0); err != nil {
		return fmt.Errorf("emit Lens stream last message: %w", err)
	}
	return nil
}

func (s *Stream) read(ctx context.Context, conn *websocket.Conn) (frame, error) {
	_, raw, err := conn.Read(ctx)
	if err != nil {
		return frame{}, err
	}
	var message frame
	if err := json.Unmarshal(raw, &message); err != nil {
		return frame{}, fmt.Errorf("decode JSON frame: %w", err)
	}
	return message, nil
}

func (s *Stream) write(ctx context.Context, conn *websocket.Conn, message frame) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, raw)
}

func (s *Stream) backoff(failures int) time.Duration {
	d := s.cfg.MinBackoff
	for range failures - 1 {
		if d >= s.cfg.MaxBackoff/2 {
			return s.cfg.MaxBackoff
		}
		d *= 2
	}
	if d > s.cfg.MaxBackoff {
		return s.cfg.MaxBackoff
	}
	return d
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func contextResult(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

type frame struct {
	ID      string         `json:"id,omitempty"`
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload,omitempty"`
}

type device struct {
	Connected        bool   `json:"connected"`
	ExternalIP       string `json:"externalIp"`
	HardwareRevision string `json:"hardwareRevision"`
	ID               string `json:"id"`
	MACAddress       string `json:"macAddress"`
	ModelID          string `json:"modelId"`
	Name             string `json:"name"`
	ProductID        string `json:"productId"`
	RoomID           string `json:"roomId"`
	SiteID           string `json:"siteId"`
	SoftwareBuild    string `json:"softwareBuild"`
	SoftwareVersion  string `json:"softwareVersion"`
	TenantID         string `json:"tenantId"`
}
