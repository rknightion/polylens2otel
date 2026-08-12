# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.26.5-bookworm@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/rknightion/polylens2otel/internal/version.Version=${VERSION}" \
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
