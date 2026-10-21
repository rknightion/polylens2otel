# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.27.3-bookworm@sha256:21ade014faf952365b9ecbd8dbdb87176fbacb82fac9fb132d9f2a7b8cc3f132 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/rknightion/polylens2otel/internal/version.Version=${VERSION} -X github.com/rknightion/polylens2otel/internal/version.Commit=${COMMIT} -X github.com/rknightion/polylens2otel/internal/version.BuildDate=${BUILD_DATE}" \
      -o /out/polylens2otel ./cmd/polylens2otel

# Docker copies this ownership into a new empty named volume on first use.
RUN install -d -m 0750 -o 65532 -g 65532 /out/state

# ---- runtime ----
# static-debian includes the system CA bundle required for Lens, phone, and OTLP
# HTTPS connections while retaining the distroless nonroot runtime identity.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:d62067879de890a1def9da0e7766b7d20810a5e5555cef8506b752437d7a3e88
COPY --from=build /out/polylens2otel /usr/local/bin/polylens2otel
COPY --from=build --chown=65532:65532 /out/state /var/lib/polylens2otel
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/polylens2otel"]
CMD []
