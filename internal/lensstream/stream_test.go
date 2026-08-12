package lensstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
	"github.com/rknightion/polylens2otel/internal/telemetrytest"
)

func TestRunNamedSubscriptionEmitsDeviceAndSelfObservation(t *testing.T) {
	rawFrame := readFixture(t, "testdata/device_frame.synthetic.json")
	var headersOK atomic.Bool
	var tokenCalls atomic.Int32
	delivered := make(chan struct{}, 1)
	server := websocketServer(t, func(ctx context.Context, r *http.Request, conn *websocket.Conn) error {
		headersOK.Store(r.Header.Get("Authorization") == "Bearer test-token" && conn.Subprotocol() == subprotocol)
		init, err := readFrame(ctx, conn)
		if err != nil {
			return err
		}
		if init.Type != connectionInit || init.Payload["authorization"] != "Bearer test-token" {
			return fmt.Errorf("connection_init = %#v", init)
		}
		if err := writeFrame(ctx, conn, frame{Type: connectionAck}); err != nil {
			return err
		}
		subscribe, err := readFrame(ctx, conn)
		if err != nil {
			return err
		}
		query, _ := subscribe.Payload["query"].(string)
		variables, _ := subscribe.Payload["variables"].(map[string]interface{})
		deviceIDs, _ := variables["deviceIDs"].([]interface{})
		if subscribe.Type != subscribeMessage || subscribe.ID != operationID || query != subscription || len(deviceIDs) != 2 {
			return fmt.Errorf("subscribe = %#v", subscribe)
		}
		if err := writeRaw(ctx, conn, rawFrame); err != nil {
			return err
		}
		delivered <- struct{}{}
		<-ctx.Done()
		return nil
	})

	recorder := telemetrytest.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- New(Config{URL: server.URL, TokenSource: func(context.Context) (string, error) {
			tokenCalls.Add(1)
			return "test-token", nil
		}, DeviceIDs: []string{"1", "2"}, AckTimeout: 100 * time.Millisecond, MinBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond, Emitter: recorder.Emitter()}).Run(ctx)
	}()
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("server did not receive the named subscription")
	}
	waitForMetric(t, recorder, semconv.MetricLensDeviceConnected)
	cancel()
	err := <-done
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !headersOK.Load() {
		t.Fatal("upgrade did not carry bearer authorization and graphql-transport-ws subprotocol")
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token source calls = %d, want 1", tokenCalls.Load())
	}
	if !recorder.HasMetric(semconv.MetricLensStreamConnected, nil, 1) {
		t.Fatal("missing connected stream metric")
	}
	if !recorder.HasMetric(semconv.MetricLensDeviceConnected, map[string]string{"device.id": "482567139733", "tenant.id": "00000000-0000-4000-8000-000000000001"}, 1) {
		t.Fatal("missing device connection metric")
	}
	if !recorder.HasMetric(semconv.MetricStreamMessages, map[string]string{"tenant.id": "00000000-0000-4000-8000-000000000001"}, 1) {
		t.Fatal("missing stream message counter")
	}
	if !recorder.HasMetric(semconv.MetricStreamLastMessageSeconds, map[string]string{"tenant.id": "00000000-0000-4000-8000-000000000001"}, 0) {
		t.Fatal("missing stream last-message observation")
	}
}

func TestRunReconnectsAfterConnectionCloses(t *testing.T) {
	var connections atomic.Int32
	secondSubscribed := make(chan struct{}, 1)
	server := websocketServer(t, func(ctx context.Context, _ *http.Request, conn *websocket.Conn) error {
		connection := connections.Add(1)
		if _, err := readFrame(ctx, conn); err != nil {
			return err
		}
		if err := writeFrame(ctx, conn, frame{Type: connectionAck}); err != nil {
			return err
		}
		if _, err := readFrame(ctx, conn); err != nil {
			return err
		}
		if connection == 2 {
			secondSubscribed <- struct{}{}
			<-ctx.Done()
			return nil
		}
		_ = conn.Close(websocket.StatusNormalClosure, "test reconnect")
		return nil
	})
	recorder := telemetrytest.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- New(Config{URL: server.URL, Token: "test-token", DeviceIDs: []string{"1"}, AckTimeout: 20 * time.Millisecond, MinBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond, Emitter: recorder.Emitter()}).Run(ctx)
	}()
	select {
	case <-secondSubscribed:
	case <-time.After(time.Second):
		t.Fatal("server did not receive the second subscription")
	}
	cancel()
	err := <-done
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if connections.Load() < 2 {
		t.Fatalf("connections = %d, want at least 2", connections.Load())
	}
	if !recorder.HasMetric(semconv.MetricStreamReconnects, nil, 1) {
		t.Fatal("missing reconnect counter")
	}
}

func TestRunReconnectsReturnsNilWhenCancelledDuringReconnectEmission(t *testing.T) {
	server := websocketServer(t, func(ctx context.Context, _ *http.Request, conn *websocket.Conn) error {
		if _, err := readFrame(ctx, conn); err != nil {
			return err
		}
		if err := writeFrame(ctx, conn, frame{Type: connectionAck}); err != nil {
			return err
		}
		if _, err := readFrame(ctx, conn); err != nil {
			return err
		}
		_ = conn.Close(websocket.StatusNormalClosure, "test reconnect")
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	emitter := &cancelDuringReconnectEmitter{
		Emitter:      telemetrytest.New().Emitter(),
		reconnecting: make(chan struct{}, 1),
	}
	done := make(chan error, 1)
	go func() {
		done <- New(Config{URL: server.URL, Token: "test-token", DeviceIDs: []string{"1"}, AckTimeout: 20 * time.Millisecond, MinBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond, Emitter: emitter}).Run(ctx)
	}()

	select {
	case <-emitter.reconnecting:
	case <-time.After(time.Second):
		t.Fatal("Run did not begin reconnect accounting")
	}
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v, want nil after cancellation", err)
	}
}

func TestRunReconnectsWhenAckTimesOut(t *testing.T) {
	var connections atomic.Int32
	secondInitReceived := make(chan struct{}, 1)
	server := websocketServer(t, func(ctx context.Context, _ *http.Request, conn *websocket.Conn) error {
		connection := connections.Add(1)
		if _, err := readFrame(ctx, conn); err != nil {
			return err
		}
		if connection == 2 {
			secondInitReceived <- struct{}{}
		}
		<-ctx.Done()
		return nil
	})
	recorder := telemetrytest.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- New(Config{URL: server.URL, Token: "test-token", DeviceIDs: []string{"1"}, AckTimeout: 5 * time.Millisecond, MinBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond, Emitter: recorder.Emitter()}).Run(ctx)
	}()
	select {
	case <-secondInitReceived:
	case <-time.After(time.Second):
		t.Fatal("server did not receive the second connection_init")
	}
	cancel()
	err := <-done
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if connections.Load() < 2 {
		t.Fatalf("connections = %d, want acknowledgement timeout to reconnect", connections.Load())
	}
}

type cancelDuringReconnectEmitter struct {
	telemetry.Emitter
	reconnecting chan struct{}
}

func (e *cancelDuringReconnectEmitter) Counter(ctx context.Context, name string, value float64, attrs ...telemetry.Attr) error {
	if name != semconv.MetricStreamReconnects {
		return e.Emitter.Counter(ctx, name, value, attrs...)
	}
	e.reconnecting <- struct{}{}
	<-ctx.Done()
	return errors.New("reconnect telemetry transport failed")
}

func TestRunDoesNotTreatMessageSilenceAsFailure(t *testing.T) {
	var connections atomic.Int32
	subscribed := make(chan struct{}, 1)
	server := websocketServer(t, func(ctx context.Context, _ *http.Request, conn *websocket.Conn) error {
		connections.Add(1)
		if _, err := readFrame(ctx, conn); err != nil {
			return err
		}
		if err := writeFrame(ctx, conn, frame{Type: connectionAck}); err != nil {
			return err
		}
		if _, err := readFrame(ctx, conn); err != nil {
			return err
		}
		subscribed <- struct{}{}
		<-ctx.Done()
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- New(Config{URL: server.URL, Token: "test-token", DeviceIDs: []string{"1"}, AckTimeout: 10 * time.Millisecond, MinBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond, Emitter: telemetrytest.New().Emitter()}).Run(ctx)
	}()
	select {
	case <-subscribed:
	case <-time.After(time.Second):
		t.Fatal("server did not receive subscription")
	}
	time.Sleep(25 * time.Millisecond)
	cancel()
	err := <-done
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if connections.Load() != 1 {
		t.Fatalf("connections = %d, want 1: quiet deviceStream must remain connected", connections.Load())
	}
}

func websocketServer(t *testing.T, handler func(context.Context, *http.Request, *websocket.Conn) error) *httptest.Server {
	t.Helper()
	errs := make(chan error, 32)
	var handlers sync.WaitGroup
	serverCtx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.Add(1)
		defer handlers.Done()
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{subprotocol}})
		if err != nil {
			errs <- fmt.Errorf("accept websocket: %w", err)
			return
		}
		defer conn.CloseNow()
		if err := handler(serverCtx, r, conn); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
			errs <- err
		}
	}))
	t.Cleanup(func() {
		cancel()
		server.Close()
		handlers.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}
	})
	return server
}

func readFrame(ctx context.Context, conn *websocket.Conn) (frame, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return frame{}, err
	}
	var got frame
	if err := json.Unmarshal(data, &got); err != nil {
		return frame{}, err
	}
	return got, nil
}

func writeFrame(ctx context.Context, conn *websocket.Conn, message frame) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return writeRaw(ctx, conn, data)
}

func writeRaw(ctx context.Context, conn *websocket.Conn, data []byte) error {
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return err
	}
	return nil
}

func waitForConnections(t *testing.T, connections *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.After(time.Second)
	for connections.Load() < want {
		select {
		case <-deadline:
			t.Fatalf("connections = %d, want at least %d", connections.Load(), want)
		case <-time.After(time.Millisecond):
		}
	}
}

func waitForMetric(t *testing.T, recorder *telemetrytest.Recorder, name string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		for _, metric := range recorder.Metrics() {
			if metric.Name == name {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatalf("metric %q was not emitted", name)
		case <-time.After(time.Millisecond):
		}
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return b
}
