package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	env "github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

const (
	EnvPrefix = "PL2O_"
	keyDelim  = "."
)

type Config struct {
	Log         LogConfig         `yaml:"log"`
	Lens        LensConfig        `yaml:"lens"`
	Phone       PhoneConfig       `yaml:"phone"`
	Collectors  CollectorConfig   `yaml:"collectors"`
	OTLP        OTLPConfig        `yaml:"otlp"`
	Profiling   ProfilingConfig   `yaml:"profiling"`
	State       StateConfig       `yaml:"state"`
	Cardinality CardinalityConfig `yaml:"cardinality"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type LensConfig struct {
	TokenURL       string        `yaml:"token_url"`
	GraphQLURL     string        `yaml:"graphql_url"`
	WebSocketURL   string        `yaml:"websocket_url"`
	ClientID       string        `yaml:"client_id"`
	ClientSecret   string        `yaml:"client_secret"`
	Tenants        []string      `yaml:"tenants"`
	PageSize       int           `yaml:"page_size"`
	RequestTimeout time.Duration `yaml:"request_timeout"`
	Retry          RetryConfig   `yaml:"retry"`
	Stream         StreamConfig  `yaml:"stream"`
}

type RetryConfig struct {
	MaxAttempts int           `yaml:"max_attempts"`
	MinBackoff  time.Duration `yaml:"min_backoff"`
	MaxBackoff  time.Duration `yaml:"max_backoff"`
}

type StreamConfig struct {
	Enabled    bool          `yaml:"enabled"`
	AckTimeout time.Duration `yaml:"ack_timeout"`
	MinBackoff time.Duration `yaml:"min_backoff"`
	MaxBackoff time.Duration `yaml:"max_backoff"`
}

type PhoneConfig struct {
	Enabled        bool              `yaml:"enabled"`
	Targets        map[string]string `yaml:"targets"`
	ConfigParams   []string          `yaml:"config_params"`
	RequestTimeout time.Duration     `yaml:"request_timeout"`
	Auth           PhoneAuthConfig   `yaml:"auth"`
	TLS            PhoneTLSConfig    `yaml:"tls"`
}

type PhoneAuthConfig struct {
	Username       string `yaml:"username"`
	Password       string `yaml:"password"`
	FromLensPolicy bool   `yaml:"from_lens_policy"`
}

type PhoneTLSConfig struct {
	VerifyChain bool   `yaml:"verify_chain"`
	CAFile      string `yaml:"ca_file"`
}

type CollectorConfig struct {
	LensDevices      time.Duration `yaml:"lens_devices"`
	LensActiveCalls  time.Duration `yaml:"lens_active_calls"`
	LensFirmware     time.Duration `yaml:"lens_firmware"`
	LensCDR          time.Duration `yaml:"lens_cdr"`
	PhoneStatus      time.Duration `yaml:"phone_status"`
	PhoneLines       time.Duration `yaml:"phone_lines"`
	PhoneConfig      time.Duration `yaml:"phone_config"`
	PhoneCallLogs    time.Duration `yaml:"phone_call_logs"`
	PhoneNetworkInfo time.Duration `yaml:"phone_network_info"`
	SelfObs          time.Duration `yaml:"selfobs_internal"`
}

type OTLPConfig struct {
	Endpoint       string             `yaml:"endpoint"`
	Protocol       string             `yaml:"protocol"`
	Insecure       bool               `yaml:"insecure"`
	ExportInterval time.Duration      `yaml:"export_interval"`
	GrafanaCloud   GrafanaCloudConfig `yaml:"grafana_cloud"`
}

type GrafanaCloudConfig struct {
	InstanceID string `yaml:"instance_id"`
	Token      string `yaml:"token"`
}

type ProfilingConfig struct {
	Pyroscope PyroscopeConfig `yaml:"pyroscope"`
}

type PyroscopeConfig struct {
	Endpoint          string `yaml:"endpoint"`
	Application       string `yaml:"application"`
	BasicAuthUser     string `yaml:"basic_auth_user"`
	BasicAuthPassword string `yaml:"basic_auth_password"`
}

type StateConfig struct {
	Dir string `yaml:"dir"`
}

type CardinalityConfig struct {
	MaxDevices int `yaml:"max_devices"`
}

func Default() Config {
	// ClientSecret and the other credential fields intentionally remain empty;
	// configuration only supplies their public endpoint defaults.
	//nolint:gosec // G101 mistakes the LensConfig field names for embedded credentials.
	return Config{
		Log: LogConfig{Level: "info", Format: "json"},
		Lens: LensConfig{
			TokenURL:     "https://login.lens.poly.com/oauth/token",
			GraphQLURL:   "https://api.silica-prod01.io.lens.poly.com/graphql",
			WebSocketURL: "wss://api.silica-prod01.io.lens.poly.com/graphql",
			PageSize:     10, RequestTimeout: 30 * time.Second,
			Retry:  RetryConfig{MaxAttempts: 4, MinBackoff: time.Second, MaxBackoff: 30 * time.Second},
			Stream: StreamConfig{Enabled: true, AckTimeout: 10 * time.Second, MinBackoff: time.Second, MaxBackoff: time.Minute},
		},
		Phone: PhoneConfig{
			Enabled: true, Targets: map[string]string{}, RequestTimeout: 15 * time.Second,
			ConfigParams: []string{"reg.1.address", "reg.2.address", "reg.1.label", "device.syslog.serverName", "tcpIpApp.sntp.address", "softkey.1.enable"},
			Auth:         PhoneAuthConfig{Username: "Polycom"},
		},
		Collectors: CollectorConfig{
			LensDevices: time.Minute, LensActiveCalls: time.Minute, LensFirmware: 24 * time.Hour,
			LensCDR: time.Hour, PhoneStatus: time.Minute, PhoneLines: time.Minute,
			PhoneConfig: 5 * time.Minute, PhoneCallLogs: 5 * time.Minute,
			PhoneNetworkInfo: 5 * time.Minute, SelfObs: time.Minute,
		},
		OTLP:        OTLPConfig{Protocol: "http", ExportInterval: 15 * time.Second},
		Profiling:   ProfilingConfig{Pyroscope: PyroscopeConfig{Application: "polylens2otel"}},
		State:       StateConfig{Dir: "/var/lib/polylens2otel"},
		Cardinality: CardinalityConfig{MaxDevices: 500},
	}
}

var yamlSecrets = []string{
	"lens.client_secret", "phone.auth.password", "otlp.grafana_cloud.token",
	"profiling.pyroscope.basic_auth_password",
}

func Load(path string) (*Config, error) {
	k := koanf.New(keyDelim)
	if err := k.Load(structs.Provider(Default(), "yaml"), nil); err != nil {
		return nil, fmt.Errorf("load defaults: %w", err)
	}
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("config file: %w", err)
		}
		fileOnly := koanf.New(keyDelim)
		if err := fileOnly.Load(file.Provider(path), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
		for _, key := range yamlSecrets {
			if fileOnly.Exists(key) {
				return nil, fmt.Errorf("%s is secret and must be supplied through %s environment variables", key, EnvPrefix)
			}
		}
		if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
	}
	if err := k.Load(env.Provider(keyDelim, env.Opt{Prefix: EnvPrefix, TransformFunc: envTransform}), nil); err != nil {
		return nil, fmt.Errorf("load environment: %w", err)
	}
	var cfg Config
	if err := k.UnmarshalWithConf("", &cfg, koanf.UnmarshalConf{Tag: "yaml", DecoderConfig: &mapstructure.DecoderConfig{
		Result: &cfg, WeaklyTypedInput: true, ErrorUnused: true,
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
	}}); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return &cfg, nil
}

func envTransform(key, value string) (string, any) {
	key = strings.ToLower(strings.TrimPrefix(key, EnvPrefix))
	return strings.ReplaceAll(key, "__", keyDelim), value
}

func (c Config) Validate() error {
	var missing []string
	for key, value := range map[string]string{
		"lens.client_id": c.Lens.ClientID, "lens.client_secret": c.Lens.ClientSecret,
		"otlp.endpoint": c.OTLP.Endpoint, "otlp.grafana_cloud.instance_id": c.OTLP.GrafanaCloud.InstanceID,
		"otlp.grafana_cloud.token": c.OTLP.GrafanaCloud.Token,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	if c.Phone.Enabled && strings.TrimSpace(c.Phone.Auth.Password) == "" {
		missing = append(missing, "phone.auth.password")
	}
	if len(missing) > 0 {
		return fmt.Errorf("required configuration missing: %s", strings.Join(missing, ", "))
	}
	if c.Lens.PageSize < 1 || c.Lens.PageSize > 5000 {
		return fmt.Errorf("lens.page_size must be between 1 and 5000")
	}
	if c.Cardinality.MaxDevices < 1 {
		return fmt.Errorf("cardinality.max_devices must be positive")
	}
	if c.State.Dir == "" {
		return fmt.Errorf("state.dir is required")
	}
	if c.OTLP.Protocol != "http" && c.OTLP.Protocol != "grpc" {
		return fmt.Errorf("otlp.protocol must be http or grpc")
	}
	return nil
}
