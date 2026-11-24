FROM debian:bookworm-slim

# To upgrade the collector: bump the version, both sha256 (from the
# otelcol-contrib_<v>_linux_<arch>.tar.gz.sha256 release assets) and
# "version" in config.yaml.
ARG OTELCOL_VERSION=0.165.0
ARG OTELCOL_SHA256_amd64=bd6e73d94bf3359ad6fac25529a8ee308adb9b0b8a0e8af034ed3891fbe5aa33
ARG OTELCOL_SHA256_arm64=376420729c862230807bc89811d07d10031eaeca3f6481fe371a2bdcefd2c8fa
ARG BUILD_ARCH=amd64

# systemd provides journalctl, which the journald receiver shells out to.
# The release tarball is used instead of the .deb: just the binary, no unit/user.
RUN set -eux; \
    case "${BUILD_ARCH}" in \
      amd64)   arch=amd64; sha="${OTELCOL_SHA256_amd64}" ;; \
      aarch64) arch=arm64; sha="${OTELCOL_SHA256_arm64}" ;; \
      *) echo "unsupported architecture: ${BUILD_ARCH}" >&2; exit 1 ;; \
    esac; \
    apt-get update; \
    apt-get install -y --no-install-recommends ca-certificates curl jq systemd; \
    curl -fsSL -o /tmp/otelcol.tar.gz \
      "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v${OTELCOL_VERSION}/otelcol-contrib_${OTELCOL_VERSION}_linux_${arch}.tar.gz"; \
    echo "${sha}  /tmp/otelcol.tar.gz" | sha256sum -c -; \
    tar -xzf /tmp/otelcol.tar.gz -C /usr/local/bin otelcol-contrib; \
    rm -rf /tmp/otelcol.tar.gz /var/lib/apt/lists/*; \
    otelcol-contrib --version

COPY rootfs /
RUN chmod +x /run.sh

CMD ["/run.sh"]
