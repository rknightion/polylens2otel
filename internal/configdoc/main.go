// Command configdoc renders the public configuration reference from internal/config.
//
//go:generate go run .
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/rknightion/polylens2otel/internal/config"
)

type field struct {
	Key      string
	Type     string
	Default  string
	Secret   bool
	Required string
	Comment  string
	Value    reflect.Value
	IsMap    bool
}

var secrets = map[string]bool{
	"lens.client_secret":                      true,
	"phone.auth.password":                     true,
	"otlp.grafana_cloud.token":                true,
	"profiling.pyroscope.basic_auth_password": true,
}

var comments = map[string]string{
	"log.level":                               "Application log level.",
	"log.format":                              "Application log encoding.",
	"lens.token_url":                          "OAuth token endpoint for Poly Lens.",
	"lens.graphql_url":                        "Poly Lens GraphQL endpoint.",
	"lens.websocket_url":                      "Poly Lens GraphQL WebSocket endpoint for deviceStream.",
	"lens.client_id":                          "OAuth client identifier.",
	"lens.client_secret":                      "OAuth client secret; supply only through the environment.",
	"lens.tenants":                            "Lens tenant IDs to collect. An empty list discovers tenants through the read-only tenants query.",
	"lens.page_size":                          "Lens page size; validation permits 1 through 5000. Keep the value stable across follow-up pages.",
	"lens.request_timeout":                    "Timeout for an individual Lens request.",
	"lens.retry.max_attempts":                 "Maximum Lens request attempts, including the first attempt.",
	"lens.retry.min_backoff":                  "Initial Lens retry backoff.",
	"lens.retry.max_backoff":                  "Maximum Lens retry backoff.",
	"lens.stream.enabled":                     "Enable the named deviceStream GraphQL subscription as an edge-triggered supplement to polling.",
	"lens.stream.ack_timeout":                 "Time allowed for a deviceStream acknowledgement.",
	"lens.stream.min_backoff":                 "Initial deviceStream reconnect backoff.",
	"lens.stream.max_backoff":                 "Maximum deviceStream reconnect backoff.",
	"phone.enabled":                           "Enable phone REST collectors. When enabled, phone.auth.password is required.",
	"phone.targets":                           "Static per-device targets. Each <device-id> overrides that device's Lens internalIp; discovery never scans.",
	"phone.config_params":                     "Read-only phone configuration parameters requested by the phone config collector.",
	"phone.request_timeout":                   "Timeout for an individual phone REST request.",
	"phone.auth.username":                     "Digest authentication username; Polycom is the default.",
	"phone.auth.password":                     "Digest authentication password; supply only through the environment when phone collection is enabled.",
	"phone.auth.from_lens_policy":             "Use phone credentials from Lens policy where supported.",
	"phone.tls.verify_chain":                  "Verify the phone TLS certificate chain.",
	"phone.tls.ca_file":                       "Optional CA certificate file for phone TLS verification. The certificate CN must match the Lens MAC before credentials are sent.",
	"collectors.lens_devices":                 "Polling interval for Lens device inventory.",
	"collectors.lens_active_calls":            "Polling interval for Lens active calls.",
	"collectors.lens_firmware":                "Polling interval for Lens firmware inventory.",
	"collectors.lens_cdr":                     "Polling interval for Lens CDR logs.",
	"collectors.phone_status":                 "Polling interval for phone status.",
	"collectors.phone_lines":                  "Polling interval for phone lines.",
	"collectors.phone_config":                 "Polling interval for phone configuration.",
	"collectors.selfobs_internal":             "Emission interval for exporter self-observability.",
	"otlp.endpoint":                           "OTLP collector endpoint.",
	"otlp.protocol":                           "OTLP transport protocol; validation permits http or grpc.",
	"otlp.insecure":                           "Allow an insecure OTLP transport.",
	"otlp.export_interval":                    "OTLP metric export interval.",
	"otlp.grafana_cloud.instance_id":          "Grafana Cloud instance ID.",
	"otlp.grafana_cloud.token":                "Grafana Cloud access token; supply only through the environment.",
	"profiling.pyroscope.endpoint":            "Pyroscope endpoint. Empty leaves profiling endpoint unset.",
	"profiling.pyroscope.application":         "Pyroscope application name.",
	"profiling.pyroscope.basic_auth_user":     "Pyroscope basic-auth username.",
	"profiling.pyroscope.basic_auth_password": "Pyroscope basic-auth password; supply only through the environment.",
	"state.dir":                               "Directory for durable collector state; required and must be writable by the exporter.",
	"cardinality.max_devices":                 "Maximum devices represented by the exporter; must be positive.",
}

func main() {
	check := flag.Bool("check", false, "fail if generated files differ")
	flag.Parse()
	root, err := repoRoot()
	if err != nil {
		fatal(err)
	}
	fields, err := fieldsFor(config.Default())
	if err != nil {
		fatal(err)
	}
	if err := validateMetadata(fields); err != nil {
		fatal(err)
	}
	outputs := map[string][]byte{
		"docs/env-vars.md":    renderDocs(fields),
		"config.example.yaml": renderExample(fields),
	}
	if _, err := yaml.Parser().Unmarshal(outputs["config.example.yaml"]); err != nil {
		fatal(fmt.Errorf("render config example: %w", err))
	}
	for name, want := range outputs {
		path := filepath.Join(root, name)
		got, err := os.ReadFile(path)
		if *check {
			if err != nil || !bytes.Equal(got, want) {
				fatal(fmt.Errorf("generated configuration drift: %s (run make regen)", name))
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(path, want, 0o644); err != nil {
			fatal(err)
		}
	}
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			return "", errors.New("could not find repository root")
		}
		dir = next
	}
}

func fieldsFor(cfg config.Config) ([]field, error) {
	var fields []field
	if err := walk(reflect.ValueOf(cfg), "", &fields); err != nil {
		return nil, err
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Key < fields[j].Key })
	return fields, nil
}

func walk(v reflect.Value, prefix string, fields *[]field) error {
	t := v.Type()
	if t == reflect.TypeFor[time.Duration]() {
		return addField(v, prefix, fields, false)
	}
	if v.Kind() != reflect.Struct {
		return addField(v, prefix, fields, v.Kind() == reflect.Map)
	}
	for i := range v.NumField() {
		sf := t.Field(i)
		name := strings.Split(sf.Tag.Get("yaml"), ",")[0]
		if name == "" || name == "-" {
			return fmt.Errorf("%s has no yaml tag", sf.Name)
		}
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		if err := walk(v.Field(i), key, fields); err != nil {
			return err
		}
	}
	return nil
}

func addField(v reflect.Value, key string, fields *[]field, isMap bool) error {
	if key == "" {
		return errors.New("empty configuration key")
	}
	*fields = append(*fields, field{Key: key, Type: typeName(v.Type()), Default: defaultValue(v), Secret: secrets[key], Required: required(key), Comment: comments[key], Value: v, IsMap: isMap})
	return nil
}

func typeName(t reflect.Type) string {
	if t == reflect.TypeFor[time.Duration]() {
		return "duration"
	}
	if t.Kind() == reflect.Map {
		return "map[string]string"
	}
	if t.Kind() == reflect.Slice {
		return "[]" + typeName(t.Elem())
	}
	return t.String()
}

func defaultValue(v reflect.Value) string {
	if v.Type() == reflect.TypeFor[time.Duration]() {
		return v.Interface().(time.Duration).String()
	}
	if v.Kind() == reflect.String {
		return v.String()
	}
	if v.Kind() == reflect.Bool {
		return fmt.Sprintf("%t", v.Bool())
	}
	if v.Kind() == reflect.Int {
		return fmt.Sprintf("%d", v.Int())
	}
	if v.Kind() == reflect.Map {
		return "{}"
	}
	if v.Kind() == reflect.Slice {
		items := make([]string, v.Len())
		for i := range v.Len() {
			items[i] = fmt.Sprint(v.Index(i).Interface())
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	return fmt.Sprint(v.Interface())
}

func required(key string) string {
	switch key {
	case "lens.client_id", "lens.client_secret", "otlp.endpoint", "otlp.grafana_cloud.instance_id", "otlp.grafana_cloud.token", "state.dir":
		return "Required"
	case "phone.auth.password":
		return "Required when phone.enabled is true"
	default:
		return "Optional"
	}
}

func validateMetadata(fields []field) error {
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		seen[f.Key] = true
		if f.Comment == "" {
			return fmt.Errorf("missing documentation metadata for %s", f.Key)
		}
		if f.Secret != secrets[f.Key] {
			return fmt.Errorf("secret metadata mismatch for %s", f.Key)
		}
	}
	for key := range comments {
		if !seen[key] {
			return fmt.Errorf("documentation metadata refers to absent config key %s", key)
		}
	}
	for key := range secrets {
		if !seen[key] {
			return fmt.Errorf("secret metadata refers to absent config key %s", key)
		}
	}
	return nil
}

func envName(key string) string {
	return config.EnvPrefix + strings.ToUpper(strings.ReplaceAll(key, ".", "__"))
}

func renderDocs(fields []field) []byte {
	var b strings.Builder
	b.WriteString("<!-- Code generated by internal/configdoc; DO NOT EDIT. Run `make regen`. -->\n\n# Configuration and environment reference\n\nConfiguration precedence is defaults, YAML, then `PL2O_` environment variables. Nested YAML dots become double underscores in environment variables. Secrets are environment-only: placing a secret key in YAML is rejected before the exporter starts.\n\n| Key | Type | Default | Environment | Secret | Required | Description |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, f := range fields {
		secret := "No"
		if f.Secret {
			secret = "Yes (environment only)"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | `%s` | %s | %s | %s |\n", f.Key, f.Type, escapeTable(f.Default), envName(f.Key), secret, f.Required, f.Comment)
	}
	b.WriteString("\n## Static phone targets\n\n`phone.targets` is a YAML map of device IDs to DNS names or addresses. It is an explicit override for Lens `internalIp`; no LAN discovery or scanning occurs. Use a device ID that identifies the Lens device, and ensure the phone certificate CN matches that device's Lens MAC before credentials are sent.\n\n```yaml\nphone:\n  targets:\n    <device-id>: phone.example.invalid\n```\n\n## Secret environment variables\n\nSet required secrets through the process environment, never in YAML: `PL2O_LENS__CLIENT_SECRET`, `PL2O_PHONE__AUTH__PASSWORD` (when phones are enabled), `PL2O_OTLP__GRAFANA_CLOUD__TOKEN`, and `PL2O_PROFILING__PYROSCOPE__BASIC_AUTH_PASSWORD` when Pyroscope authentication is used.\n")
	return []byte(b.String())
}

func escapeTable(s string) string { return strings.ReplaceAll(s, "|", "\\|") }

func renderExample(fields []field) []byte {
	byKey := make(map[string]field, len(fields))
	for _, f := range fields {
		byKey[f.Key] = f
	}
	var b strings.Builder
	b.WriteString("# Code generated by internal/configdoc; DO NOT EDIT. Run `make regen`.\n# Defaults < YAML < PL2O_ environment variables. Secrets are intentionally omitted:\n# use their listed PL2O_ variables, because the loader rejects secrets in YAML.\n")
	writeExample(&b, reflect.ValueOf(config.Default()), "", 0, byKey)
	return []byte(b.String())
}

func writeExample(b *strings.Builder, v reflect.Value, prefix string, depth int, fields map[string]field) {
	indent := strings.Repeat("  ", depth)
	if v.Type() == reflect.TypeFor[time.Duration]() {
		return
	}
	t := v.Type()
	for i := range v.NumField() {
		sf, value := t.Field(i), v.Field(i)
		name := strings.Split(sf.Tag.Get("yaml"), ",")[0]
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		if value.Kind() == reflect.Struct && value.Type() != reflect.TypeFor[time.Duration]() {
			fmt.Fprintf(b, "%s%s:\n", indent, name)
			writeExample(b, value, key, depth+1, fields)
			continue
		}
		f := fields[key]
		if f.Secret {
			fmt.Fprintf(b, "%s# %s is secret; set %s in the environment.\n", indent, key, envName(key))
			continue
		}
		fmt.Fprintf(b, "%s# %s\n", indent, f.Comment)
		if f.IsMap {
			fmt.Fprintf(b, "%s%s:\n%s  <device-id>: phone.example.invalid\n", indent, name, indent)
			continue
		}
		fmt.Fprintf(b, "%s%s: %s\n", indent, name, yamlValue(value))
	}
}

func yamlValue(v reflect.Value) string {
	if v.Type() == reflect.TypeFor[time.Duration]() {
		return v.Interface().(time.Duration).String()
	}
	if v.Kind() == reflect.String {
		return fmt.Sprintf("%q", v.String())
	}
	if v.Kind() == reflect.Bool || v.Kind() == reflect.Int {
		return defaultValue(v)
	}
	if v.Kind() == reflect.Slice {
		items := make([]string, v.Len())
		for i := range v.Len() {
			items[i] = fmt.Sprintf("%q", fmt.Sprint(v.Index(i).Interface()))
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	return fmt.Sprint(v.Interface())
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
