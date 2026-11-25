# OTel Collector add-on for Home Assistant

![Builder](https://github.com/nicolastakashi/ha-addon-otelcol/actions/workflows/builder.yaml/badge.svg)

A community Home Assistant add-on that runs a custom build of the OpenTelemetry Collector on
Home Assistant OS and sends logs, traces, metrics and profiles to any [OTLP](https://opentelemetry.io/docs/specs/otlp/)/HTTP backend.

| Signal | Option | Source |
|---|---|---|
| Logs | `logs` | HAOS systemd journal: host services, kernel, Supervisor, Core and every add-on |
| Traces | `ha_traces` | Automation and script runs, one trace per run ([Home Assistant receiver](otelcol/collector/receiver/homeassistantreceiver)) |
| Metrics | `ha_metrics` | Home Assistant `/api/prometheus`, through the Supervisor proxy |
| Metrics | `host_metrics` | Host CPU, memory, load, paging, network and uptime (`system.*`) |
| Profiles | `profiles` | Experimental: Home Assistant's Python threads, sampled by a bundled integration |

Runs on `amd64` and `aarch64` (for example a Raspberry Pi running 64-bit Home Assistant OS).

## Install the add-on

[![Open your Home Assistant instance and show the add add-on repository dialog with this repository URL pre-filled.](https://my.home-assistant.io/badges/supervisor_add_addon_repository.svg)](https://my.home-assistant.io/redirect/supervisor_add_addon_repository/?repository_url=https%3A%2F%2Fgithub.com%2Fnicolastakashi%2Fha-addon-otelcol)

1. Add this repository: click the badge above, or go to **Settings → Add-ons → Add-on
   store → ⋮ → Repositories** and add `https://github.com/nicolastakashi/ha-addon-otelcol`.
   Recent Home Assistant releases label this menu **Apps**.
2. Install **OTel Collector (community)**.
3. On the **Configuration** tab, set `otlp_endpoint` and, if your backend needs it, an
   auth header in `otlp_headers`.
4. Start the add-on. The **Log** tab shows a line like
   `[otelcol] host.name=homeassistant, exporting to https://otlp.example.com: logs ha_metrics ha_traces host_metrics`.

For `ha_metrics`, also enable the
[Prometheus integration](https://www.home-assistant.io/integrations/prometheus/) in Home
Assistant (`prometheus:` in `configuration.yaml`).

## Configure the minimum

```yaml
otlp_endpoint: https://otlp.example.com
otlp_headers:
  - name: Authorization
    value: Bearer <your-token>
```

Every other option has a default. All signals except `profiles` are on by default.

## Choose a backend

Any OpenTelemetry-compatible backend works: any vendor or self-hosted tool that accepts
OTLP over HTTP (`http/protobuf`). Authentication goes in `otlp_headers`.

OTLP profiles are alpha. Check that your backend accepts them before you enable `profiles`.

## Read the full documentation

- [Docs](otelcol/DOCS.md)
- [CHANGELOG](otelcol/CHANGELOG.md)
- [Home Assistant receiver](otelcol/collector/receiver/homeassistantreceiver/README.md)

## How it works

```mermaid
flowchart LR
  subgraph HAOS["Home Assistant OS"]
    J[(systemd journal)]
    S[Supervisor proxy]
    C[Home Assistant Core]
    P[otel_profiler integration]
    subgraph A["otelcol add-on (otelcol-ha)"]
      RJ[journald receiver]
      RH[homeassistant receiver]
      RP[prometheus receiver]
      RM[hostmetrics receiver]
      RF[pprof receiver]
      Q[(persistent queue in /data)]
      E[otlp_http exporter]
    end
  end
  J --> RJ
  C -- WebSocket trace store --> S --> RH
  C -- /api/prometheus --> S --> RP
  H[(/proc and /sys)] --> RM
  P -- pprof 127.0.0.1:4040 --> RF
  RJ & RH & RP & RM & RF --> Q --> E
  E -- OTLP/HTTP --> B[(Your backend)]
```

In short: five receivers read the journal, Home Assistant (through the Supervisor), the
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

## Semantic conventions and Weaver

The add-on uses [OpenTelemetry semantic conventions](https://opentelemetry.io/docs/specs/semconv/)
wherever they exist (`service.*`, `host.name`, `container.name`, `thread.name`, `user.id`,
`system.*` metrics). Attributes without a convention (`homeassistant.*`, `systemd.unit`,
`syslog.identifier`, `journald.boot_id`, `python.logger.name`) are defined in a
[Weaver](https://github.com/open-telemetry/weaver) registry in [`model/`](model/). The
registry depends on semantic conventions v1.37.0.

> [!WARNING]
> Version 0.6.0 renamed three attributes: `haos.container` → `container.name`,
> `homeassistant.context.user_id` → `user.id`, `code.namespace` → `python.logger.name`.
> Update saved queries and dashboards.

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) to build and test.

## Disclaimer

This is an independent community project. It is not affiliated with, endorsed by or
maintained by the OpenTelemetry project, the CNCF, Home Assistant or the Open Home
Foundation. "OpenTelemetry" and "OTel" are trademarks of The Linux Foundation; they are
used here only to describe what the add-on runs.

## License

[Apache-2.0](LICENSE)
