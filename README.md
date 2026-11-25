# OTel Collector add-on for Home Assistant

Community Home Assistant add-on (app) with a purpose-built OpenTelemetry Collector
(OCB build of upstream components) that ships Home Assistant OS logs, automation traces,
Home Assistant metrics and host metrics to any OTLP backend. Runs on amd64 and aarch64
(Raspberry Pi).

| Signal | Source |
|---|---|
| Logs | HAOS journal: host, Supervisor, Core, every add-on |
| Traces | Automation and script runs ([Home Assistant receiver](otelcol/collector/receiver/homeassistantreceiver)) |
| Metrics | Home Assistant `/api/prometheus`, host CPU/memory/network/uptime |
| Profiles | Experimental: Home Assistant's Python threads, sampled by a bundled integration |

![Builder](https://github.com/nicolastakashi/ha-addon-otelcol/actions/workflows/builder.yaml/badge.svg)

## Install

[![Add repository](https://my.home-assistant.io/badges/supervisor_add_addon_repository.svg)](https://my.home-assistant.io/redirect/supervisor_add_addon_repository/?repository_url=https%3A%2F%2Fgithub.com%2Fnicolastakashi%2Fha-addon-otelcol)

Or: Settings → Apps (Add-ons) → store → ⋮ → Repositories → add
`https://github.com/nicolastakashi/ha-addon-otelcol`.

See [otelcol/DOCS.md](otelcol/DOCS.md) for configuration.

## Disclaimer

This is an independent community project. It is not affiliated with, endorsed
by or maintained by the OpenTelemetry project or the CNCF. "OpenTelemetry" and
"OTel" are trademarks of The Linux Foundation; they are used here only to
describe what the add-on runs.

## License

Apache-2.0
