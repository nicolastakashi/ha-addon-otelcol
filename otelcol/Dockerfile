# Stage 1: build our collector distribution (collector/builder-config.yaml) with OCB.
# Cross-compiles on the build machine (no QEMU): needs BuildKit / buildx.
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
ARG TARGETARCH
ARG OCB_VERSION=0.165.0
RUN go install go.opentelemetry.io/collector/cmd/builder@v${OCB_VERSION}
WORKDIR /src
COPY collector/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} builder --config builder-config.yaml \
 && ./_build/otelcol-ha --version 2>/dev/null || true

# Stage 2: runtime. systemd provides journalctl, which the journald receiver shells out to.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl jq systemd \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /src/_build/otelcol-ha /usr/local/bin/otelcol-ha
COPY rootfs /
RUN chmod +x /run.sh

CMD ["/run.sh"]
