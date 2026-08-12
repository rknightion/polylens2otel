// Package phoneclient reads the small, documented management API surface of a
// Poly Edge phone. It deliberately has no generic request or write API.
package phoneclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const managementPath = "/api/v1/mgmt"

type State string

const (
	StateOK          State = "ok"
	StateAPIDisabled State = "api_disabled"
	StateAuthFailed  State = "auth_failed"
	StateUnreachable State = "unreachable"
)

type Config struct {
	BaseURL   string
	DeviceMAC string
	Username  string
	Password  string
	Timeout   time.Duration
	TLS       TLSConfig
}

type TLSConfig struct {
	VerifyChain bool
	CAFile      string
}

type Client struct {
	baseURL  *url.URL
	username string
	password string
	http     *http.Client
}

type DeviceInfo struct {
	ModelNumber           string `json:"ModelNumber"`
	FirmwareRelease       string `json:"FirmwareRelease"`
	DeviceType            string `json:"DeviceType"`
	DeviceVendor          string `json:"DeviceVendor"`
	UpTimeSinceLastReboot string `json:"UpTimeSinceLastReboot"`
	IPV4Address           string `json:"IPV4Address"`
	IPV6Address           string `json:"IPV6Address"`
	MACAddress            string `json:"MACAddress"`
}

type Line struct {
	LineNumber         string `json:"LineNumber"`
	RegistrationStatus string `json:"RegistrationStatus"`
	LineType           string `json:"LineType"`
	SIPAddress         string `json:"SIPAddress"`
	Label              string `json:"Label"`
	UserID             string `json:"UserID"`
}

type NetworkStats struct {
	UpTime    string `json:"UpTime"`
	RxPackets string `json:"RxPackets"`
	TxPackets string `json:"TxPackets"`
}

type NetworkInfo struct {
	DHCP               string `json:"DHCP"`
	DHCPServer         string `json:"DHCPServer"`
	IPV4Address        string `json:"IPV4Address"`
	DefaultGateway     string `json:"DefaultGateway"`
	SubnetMask         string `json:"SubnetMask"`
	DNSServer          string `json:"DNSServer"`
	AlternateDNSServer string `json:"AlternateDNSServer"`
	DNSDomain          string `json:"DNSDomain"`
	SNTPAddress        string `json:"SNTPAddress"`
	LANPortStatus      string `json:"LANPortStatus"`
	LANSpeed           string `json:"LANSpeed"`
}

type CallLogs struct {
	Missed   []json.RawMessage `json:"Missed"`
	Received []json.RawMessage `json:"Received"`
	Placed   []json.RawMessage `json:"Placed"`
}

type ConfigParam struct {
	Value  string `json:"Value"`
	Source string `json:"Source"`
}

func New(cfg Config) (*Client, error) {
	baseURL, err := url.Parse(cfg.BaseURL)
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" {
		return nil, errors.New("phone base URL must be an HTTPS URL")
	}
	wantCN, err := normalizedMAC(cfg.DeviceMAC)
	if err != nil {
		return nil, fmt.Errorf("phone device MAC: %w", err)
	}
	if cfg.Username == "" {
		cfg.Username = "Polycom"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	rootCAs, err := roots(cfg.TLS)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{
		// The phones use a private issuing CA and IP targets. VerifyConnection below
		// always enforces certificate-CN identity before any credential is sent and,
		// when configured, verifies the chain without DNS-name validation.
		InsecureSkipVerify: true, //nolint:gosec // Identity verification is mandatory in VerifyConnection.
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("phone presented no TLS certificate")
			}
			if cfg.TLS.VerifyChain {
				opts := x509.VerifyOptions{Roots: rootCAs, Intermediates: x509.NewCertPool(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
				for _, cert := range cs.PeerCertificates[1:] {
					opts.Intermediates.AddCert(cert)
				}
				if _, err := cs.PeerCertificates[0].Verify(opts); err != nil {
					return fmt.Errorf("phone certificate chain: %w", err)
				}
			}
			gotCN, err := normalizedMAC(cs.PeerCertificates[0].Subject.CommonName)
			if err != nil || gotCN != wantCN {
				return errors.New("phone certificate identity does not match expected device MAC")
			}
			return nil
		},
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &Client{baseURL: baseURL, username: cfg.Username, password: cfg.Password, http: &http.Client{Transport: transport, Timeout: cfg.Timeout}}, nil
}

func (c *Client) Probe(ctx context.Context) (State, error) {
	response, err := c.request(ctx, http.MethodGet, "device/info", nil, false)
	if err != nil {
		return StateUnreachable, err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNotFound:
		return StateAPIDisabled, nil
	case http.StatusUnauthorized:
		challenge := response.Header.Get("WWW-Authenticate")
		if challenge == "" {
			return StateAuthFailed, nil
		}
		response, err = c.request(ctx, http.MethodGet, "device/info", nil, true)
		if err != nil {
			return StateUnreachable, err
		}
		defer response.Body.Close()
		if response.StatusCode == http.StatusUnauthorized {
			return StateAuthFailed, nil
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return StateOK, nil
		}
		return StateUnreachable, fmt.Errorf("phone probe: unexpected HTTP status %d", response.StatusCode)
	case 200, 201, 202, 204:
		return StateOK, nil
	default:
		return StateUnreachable, fmt.Errorf("phone probe: unexpected HTTP status %d", response.StatusCode)
	}
}

func (c *Client) DeviceInfo(ctx context.Context) (DeviceInfo, error) {
	var result DeviceInfo
	return result, c.read(ctx, "device/info", &result)
}

func (c *Client) LineInfo(ctx context.Context) ([]Line, error) {
	var result []Line
	return result, c.read(ctx, "lineInfo", &result)
}

func (c *Client) NetworkStats(ctx context.Context) (NetworkStats, error) {
	var result NetworkStats
	return result, c.read(ctx, "network/stats", &result)
}

func (c *Client) NetworkInfo(ctx context.Context) (NetworkInfo, error) {
	var result NetworkInfo
	return result, c.read(ctx, "network/info", &result)
}

func (c *Client) CallLogs(ctx context.Context) (CallLogs, error) {
	var result CallLogs
	return result, c.read(ctx, "callLogs", &result)
}

// ConfigGet is the sole POST exposed by this package. The phone only supports
// this method for management reads; there is intentionally no generic POST API.
func (c *Client) ConfigGet(ctx context.Context, params []string) (map[string]ConfigParam, []string, error) {
	body, err := json.Marshal(struct {
		Data []string `json:"data"`
	}{Data: params})
	if err != nil {
		return nil, nil, fmt.Errorf("encode config/get request: %w", err)
	}
	response, err := c.authenticatedRequest(ctx, http.MethodPost, "config/get", body)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("phone management request: HTTP status %d", response.StatusCode)
	}
	var result struct {
		Data          map[string]ConfigParam `json:"data"`
		InvalidParams []string               `json:"InvalidParams"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("decode config/get response: %w", err)
	}
	return result.Data, result.InvalidParams, nil
}

func (c *Client) read(ctx context.Context, endpoint string, into any) error {
	return c.authenticatedJSON(ctx, http.MethodGet, endpoint, nil, into)
}

func (c *Client) authenticatedJSON(ctx context.Context, method, endpoint string, body []byte, into any) error {
	response, err := c.authenticatedRequest(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return decodeResponse(response, into)
}

func (c *Client) authenticatedRequest(ctx context.Context, method, endpoint string, body []byte) (*http.Response, error) {
	response, err := c.request(ctx, method, endpoint, body, false)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusUnauthorized {
		return response, nil
	}
	if err := response.Body.Close(); err != nil {
		return nil, fmt.Errorf("close unauthenticated phone response: %w", err)
	}
	return c.request(ctx, method, endpoint, body, true)
}

func (c *Client) request(ctx context.Context, method, endpoint string, body []byte, withDigest bool) (*http.Response, error) {
	url := c.baseURL.JoinPath(managementPath, endpoint)
	request, err := http.NewRequestWithContext(ctx, method, url.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create phone request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if !withDigest {
		response, err := c.http.Do(request)
		if err != nil {
			return nil, fmt.Errorf("phone request: %w", err)
		}
		return response, nil
	}
	challengeResponse, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("phone request: %w", err)
	}
	if challengeResponse.StatusCode != http.StatusUnauthorized {
		return challengeResponse, nil
	}
	challenge := challengeResponse.Header.Get("WWW-Authenticate")
	if err := challengeResponse.Body.Close(); err != nil {
		return nil, fmt.Errorf("close phone digest challenge response: %w", err)
	}
	authorization, err := digestAuthorization(challenge, request, c.username, c.password)
	if err != nil {
		return nil, err
	}
	request, err = http.NewRequestWithContext(ctx, method, url.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create phone request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", authorization)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("phone request: %w", err)
	}
	return response, nil
}

func decodeResponse(response *http.Response, into any) error {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("phone management request: HTTP status %d", response.StatusCode)
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Status string          `json:"Status"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("decode phone response: %w", err)
	}
	if err := json.Unmarshal(envelope.Data, into); err != nil {
		return fmt.Errorf("decode phone response data: %w", err)
	}
	return nil
}

func roots(cfg TLSConfig) (*x509.CertPool, error) {
	if !cfg.VerifyChain {
		return nil, nil
	}
	if cfg.CAFile == "" {
		return nil, errors.New("phone TLS CA file is required when chain verification is enabled")
	}
	pemBytes, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read phone TLS CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("parse phone TLS CA file")
	}
	return pool, nil
}

func normalizedMAC(value string) (string, error) {
	value = strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(value)
	if len(value) != 12 {
		return "", errors.New("must contain exactly 12 hexadecimal characters")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", errors.New("must contain exactly 12 hexadecimal characters")
	}
	return strings.ToUpper(value), nil
}

func randomHex(bytesLen int) (string, error) {
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
