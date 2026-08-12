#!/usr/bin/env bash
set -euo pipefail

version="1.42.3"
archive="syft_${version}_linux_amd64.tar.gz"
checksum="0d6be741479eddd2c8644a288990c04f3df0d609bbc1599a005532a9dff63509"
url="https://github.com/anchore/syft/releases/download/v${version}/${archive}"

: "${RUNNER_TEMP:?RUNNER_TEMP must be set by GitHub Actions}"
: "${GITHUB_PATH:?GITHUB_PATH must be set by GitHub Actions}"

download_dir="$(mktemp -d "${RUNNER_TEMP}/polylens2otel-syft.XXXXXX")"
trap 'rm -rf -- "$download_dir"' EXIT

curl --retry 5 --retry-all-errors --retry-delay 2 --fail --location \
  --output "${download_dir}/${archive}" "${url}"
actual_checksum="$(sha256sum "${download_dir}/${archive}" | awk '{print $1}')"
if [[ "${actual_checksum}" != "${checksum}" ]]; then
  printf 'Syft checksum mismatch: expected %s, got %s\n' "${checksum}" "${actual_checksum}" >&2
  exit 1
fi

install_dir="${RUNNER_TEMP}/polylens2otel-tools"
mkdir -p "${install_dir}"
tar --extract --gzip --file "${download_dir}/${archive}" --directory "${download_dir}" syft
install -m 0755 "${download_dir}/syft" "${install_dir}/syft"
printf '%s\n' "${install_dir}" >> "${GITHUB_PATH}"
