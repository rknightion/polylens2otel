# Security

## Read-only boundaries

Lens mutations are rejected inside `internal/lensclient` before token minting or transport use. The phone client exposes named readers only: GET endpoints and the POST-only `config/get` reader. There is no generic request method.

The exporter never scans a network. A phone target must come from that device's Lens inventory record or from `phone.targets`.

## Phone identity

Poly phones use a private issuing CA and are commonly addressed by IP. Chain verification is therefore off by default, with `phone.tls.ca_file` available when the CA is installed. Certificate identity checking is separate and cannot be disabled: the certificate CN must equal the device MAC from Lens before Digest credentials are sent.

## Credentials

Keep secrets in environment injection or a protected environment file. Don't put them in YAML, Helm `--set` flags, shell history or repository fixtures.

The phone collector needs a local administrator credential. Set a real password; a short factory-style password makes the collector work but leaves the web interface weak.

Policy-derived passwords are redacted under all formatting verbs and excluded from errors, logs and signal attributes.
