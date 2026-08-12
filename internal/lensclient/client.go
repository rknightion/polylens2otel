// Package lensclient implements the read-only Poly Lens HTTP and GraphQL API.
package lensclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rknightion/polylens2otel/internal/semconv"
	"github.com/rknightion/polylens2otel/internal/telemetry"
)

const defaultPageSize = 10

var mutationKeyword = regexp.MustCompile(`(?i)\bmutation\b`)

// Config contains the Lens connection settings. Credentials must be supplied by
// the application's environment-backed configuration, not persisted by this package.
type Config struct {
	TokenURL     string
	GraphQLURL   string
	ClientID     string
	ClientSecret string
	PageSize     int
	MaxAttempts  int
	MinBackoff   time.Duration
	MaxBackoff   time.Duration
	HTTPClient   *http.Client
	Emitter      telemetry.Emitter
}

// Client is safe for concurrent use.
type Client struct {
	cfg Config
	hc  *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
	now       func() time.Time
	sleep     func(context.Context, time.Duration) error
}

// HTTPError retains an API response body, including GraphQL validation hints.
// It never includes request headers or request bodies.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("lens API returned HTTP %d: %s", e.StatusCode, e.Body)
}

// New creates a read-only Lens client.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.TokenURL) == "" || strings.TrimSpace(cfg.GraphQLURL) == "" {
		return nil, errors.New("lens token and GraphQL URLs are required")
	}
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, errors.New("lens client credentials are required")
	}
	if cfg.PageSize == 0 {
		cfg.PageSize = defaultPageSize
	}
	if cfg.PageSize < 1 || cfg.PageSize > 5000 {
		return nil, fmt.Errorf("lens page size must be between 1 and 5000")
	}
	if cfg.MaxAttempts == 0 {
		cfg.MaxAttempts = 4
	}
	if cfg.MaxAttempts < 1 {
		return nil, errors.New("lens max attempts must be positive")
	}
	if cfg.MinBackoff == 0 {
		cfg.MinBackoff = time.Second
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 30 * time.Second
	}
	if cfg.MinBackoff < 0 || cfg.MaxBackoff < cfg.MinBackoff {
		return nil, errors.New("invalid Lens retry backoff")
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{cfg: cfg, hc: hc, now: time.Now, sleep: sleepContext}, nil
}

// Query runs a hand-written read-only GraphQL document. Any mutation document is
// rejected locally before a token is minted or a network request is made.
func (c *Client) Query(ctx context.Context, document string, variables any, out any) error {
	if mutationKeyword.MatchString(document) {
		return errors.New("lens mutations are forbidden by this read-only client")
	}
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		Query     string `json:"query"`
		Variables any    `json:"variables,omitempty"`
	}{Query: document, Variables: variables})
	if err != nil {
		return fmt.Errorf("encode GraphQL request: %w", err)
	}
	response, err := c.do(ctx, c.cfg.GraphQLURL, body, token)
	if err != nil {
		return err
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		return fmt.Errorf("decode GraphQL response: %w", err)
	}
	if len(envelope.Errors) > 0 && string(envelope.Errors) != "null" && string(envelope.Errors) != "[]" {
		return fmt.Errorf("lens GraphQL errors: %s", c.redact(string(envelope.Errors), token))
	}
	if out != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("decode GraphQL data: %w", err)
		}
	}
	return nil
}

// AccessToken returns the cached Lens token, refreshing it when required. It
// exists for the WebSocket transport, which shares the same OAuth credential.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	return c.accessToken(ctx)
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.now().Add(time.Minute).Before(c.expiresAt) {
		return c.token, nil
	}
	payload, err := json.Marshal(map[string]string{"client_id": c.cfg.ClientID, "client_secret": c.cfg.ClientSecret, "grant_type": "client_credentials"})
	if err != nil {
		return "", fmt.Errorf("encode token request: %w", err)
	}
	response, err := c.do(ctx, c.cfg.TokenURL, payload, "")
	if err != nil {
		return "", fmt.Errorf("mint Lens token: %w", err)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(response, &token); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if token.AccessToken == "" || token.ExpiresIn <= 0 {
		return "", errors.New("token response missing access_token or expires_in")
	}
	c.token, c.expiresAt = token.AccessToken, c.now().Add(time.Duration(token.ExpiresIn)*time.Second)
	if c.cfg.Emitter != nil {
		attrs := []telemetry.Attr{{Key: semconv.AttrSource, Value: "lens"}}
		if err := c.cfg.Emitter.Counter(ctx, semconv.MetricAuthTokenRefresh, 1, attrs...); err != nil {
			return "", fmt.Errorf("emit Lens token refresh: %w", err)
		}
		if err := c.cfg.Emitter.Gauge(ctx, semconv.MetricAuthTokenExpiresSeconds, float64(token.ExpiresIn), attrs...); err != nil {
			return "", fmt.Errorf("emit Lens token expiry: %w", err)
		}
	}
	return c.token, nil
}

func (c *Client) do(ctx context.Context, endpoint string, body []byte, bearer string) ([]byte, error) {
	var last error
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create Lens request: %w", err)
		}
		req.Header.Set("content-type", "application/json")
		if bearer != "" {
			req.Header.Set("authorization", "Bearer "+bearer)
		}
		resp, err := c.hc.Do(req)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			closeErr := resp.Body.Close()
			if readErr != nil {
				err = fmt.Errorf("read Lens response: %w", readErr)
			} else if closeErr != nil {
				err = fmt.Errorf("close Lens response: %w", closeErr)
			} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				err = &HTTPError{StatusCode: resp.StatusCode, Body: c.redact(string(data), bearer)}
			} else {
				return data, nil
			}
		}
		last = err
		if attempt == c.cfg.MaxAttempts || !retryable(err) {
			break
		}
		if err := c.sleep(ctx, c.backoff(attempt)); err != nil {
			return nil, err
		}
	}
	return nil, last
}

func retryable(err error) bool {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == http.StatusTooManyRequests || httpErr.StatusCode >= 500 {
			return true
		}
		// Lens reports rate-limit exhaustion as HTTP 400 rather than 429.
		body := strings.ToLower(httpErr.Body)
		return httpErr.StatusCode == http.StatusBadRequest && (strings.Contains(body, "rate limit") || strings.Contains(body, "rate-limit") || strings.Contains(body, "over limit"))
	}
	return true
}

func (c *Client) redact(body, bearer string) string {
	if c.cfg.ClientSecret != "" {
		body = strings.ReplaceAll(body, c.cfg.ClientSecret, "[redacted]")
	}
	if bearer != "" {
		body = strings.ReplaceAll(body, bearer, "[redacted]")
	}
	return body
}

func (c *Client) backoff(attempt int) time.Duration {
	d := c.cfg.MinBackoff
	for range attempt - 1 {
		if d >= c.cfg.MaxBackoff/2 {
			return c.cfg.MaxBackoff
		}
		d *= 2
	}
	if d > c.cfg.MaxBackoff {
		return c.cfg.MaxBackoff
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
