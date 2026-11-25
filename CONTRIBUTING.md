# Contributing

Issues and pull requests are welcome.

## Architecture

See the diagram in the [README](README.md#how-it-works).

Five receivers read the journal, Home Assistant (through the Supervisor), the
host and the profiler integration; everything passes through a disk-backed queue to one
OTLP/HTTP exporter.

- `run.sh` reads the add-on options and builds the collector command line from config
  fragments in [`otelcol/rootfs/etc/otelcol/`](otelcol/rootfs/etc/otelcol/), one per option.
- The collector is `otelcol-ha`, built with the
  [OpenTelemetry Collector Builder](https://opentelemetry.io/docs/collector/extend/ocb/)
  (OCB). It has only the components the add-on uses: upstream contrib receivers,
  processors and the OTLP/HTTP exporter, plus the custom `homeassistant` receiver. See
  [`builder-config.yaml`](otelcol/collector/builder-config.yaml).
- Every signal goes through one resource processor and one exporter. The export queue
  lives on disk in `/data`, so data survives restarts and backend outages.

## Repository layout

| Path | What |
|---|---|
| `otelcol/config.yaml` | Add-on manifest and options schema |
| `otelcol/rootfs/run.sh` | Builds the collector command line from the options |
| `otelcol/rootfs/etc/otelcol/` | Collector config fragments, one per option |
| `otelcol/collector/builder-config.yaml` | OCB manifest of the `otelcol-ha` distribution |
| `otelcol/collector/receiver/homeassistantreceiver/` | The custom receiver (Go) |
| `otelcol/rootfs/usr/share/otelcol/otel_profiler/` | The bundled profiler integration (Python, stdlib only) |
| `model/` | Weaver registry of custom attributes |

## Build

Build the collector with OCB (the Dockerfile pins the version, currently 0.165.0):

```sh
go install go.opentelemetry.io/collector/cmd/builder@v0.165.0
cd otelcol/collector
builder --config builder-config.yaml   # writes ./_build/otelcol-ha
```

Build the add-on image (BuildKit cross-compiles, no QEMU needed for the Go build):

```sh
docker buildx build --platform linux/amd64 otelcol
```

To install a local build in Home Assistant, remove the `image:` line from
`otelcol/config.yaml` so the Supervisor builds the add-on itself.

## Test

```sh
cd otelcol/collector/receiver/homeassistantreceiver
go vet ./... && go test -race -cover ./...
cd -
python3 -I otelcol/tests/profiler_check.py /tmp/p.pb.gz
go tool pprof -traces /tmp/p.pb.gz | grep busy_marker
```

CI ([`builder.yaml`](.github/workflows/builder.yaml)) runs on every push and pull request
to `main`: the add-on linter, the Weaver registry check, the Go tests, the profiler
self-check, and a build for each architecture. Pushes to `main` publish the images to
GHCR as `ghcr.io/nicolastakashi/{arch}-addon-otelcol`.

## Weaver registry

Attributes without an OpenTelemetry semantic convention go in the Weaver registry in
[`model/`](model/). CI checks it:

```sh
weaver registry check -r model --future
```

To package it for a backend that accepts Weaver registries:

```sh
weaver registry package -r model --future --v2 -o <dir> --resolved-schema-uri <uri>
```

The registry is also the source of generated code and docs, the way the OpenTelemetry
semantic-conventions repo works: Go constants for the receiver
(`semconv_gen.go`) and the telemetry reference ([`docs/attributes.md`](docs/attributes.md)),
from the templates in [`templates/registry/`](templates/registry/). After changing `model/`:

```sh
./scripts/generate.sh
```

CI fails if the committed output is out of date.
