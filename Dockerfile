# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.27.0-bookworm@sha256:ded31c68586d2e49e760acc2e65a884b23d032e9bbbed0ae0c55abd3fcaf4452 AS build
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
FROM gcr.io/distroless/static-debian12:nonroot@sha256:f5b485ea962d9bd1186b2f6b3a061191539b905b82ec395de78cbfae51f20e35
COPY --from=build /out/polylens2otel /usr/local/bin/polylens2otel
COPY --from=build --chown=65532:65532 /out/state /var/lib/polylens2otel
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/polylens2otel"]
CMD []
