# Changelog

## 0.2.2

- Renamed to "OTel Collector (community)", no "official" wording, non-affiliation disclaimer
- New neutral icon; logo removed

## 0.2.1

- AppArmor: allow listing /proc
- Host metrics: `system.network.connections` disabled (would need to walk every host process)

## 0.2.0

- Collector binary from the release tarball, sha256-verified per arch; unknown arch fails the build
- Batching inside the persistent queue (bytes-sized, 200 MB cap) instead of an in-memory batch processor; queue compaction
- Collector self-metrics disabled (no :8888 bind on the host network)
- Supervisor call with a timeout, token kept out of argv, clear warning when the token is missing
- `$` in option values passed literally; credentials in the endpoint URL masked in the log
- `logs_parse`: JSON `level` mapped to severity
- `scrape_interval` must be > 0
- Store assets: icon, logo, option translations, AppArmor profile
- CI: add-on lint and multi-arch build published to GHCR
- Docs rewritten to match the actual behaviour

## 0.1.5

- Also drop the Supervisor build container logs (`app_builder_*_otelcol`)
- Docs: authenticated backends (Dash0, Grafana Cloud, Honeycomb)

## 0.1.4

- New `logs_min_severity` (debug|info|warn|error|fatal), applied to the parsed severity. Use it instead of `logs_min_priority` to cut verbosity: journald priority is 6 or 3 for every container line

## 0.1.3

- Severity and `logs_parse` also understand Python's default log format (`LEVEL:time:logger:text`)

## 0.1.2

- Drop log records without a body (no MESSAGE or blank line)
- Renamed deprecated `hostmetrics` alias (`host_metrics`)

## 0.1.1

- Core/Supervisor log bodies decoded (`convert_message_bytes`) and ANSI colours stripped
- Container log severity taken from the log text; journal PRIORITY only for host/kernel; `severity_text` set
- Collector no longer ships its own logs (export-error loop)
- Resource: real `host.name` from the Supervisor, optional `service_instance_id` and `resource_attributes`
- Container name is a log attribute (`haos.container`), so everything stays one resource
- New `logs_parse` flag: JSON lines and Python log lines become attributes
- Host metrics: all real filesystems and `system.uptime`
- Renamed deprecated component aliases (`otlp_http`)

## 0.1.0

- otelcol-contrib 0.165.0
- journald logs, Home Assistant metrics via Supervisor proxy, host metrics
- Persistent journald cursor and export queue in /data
