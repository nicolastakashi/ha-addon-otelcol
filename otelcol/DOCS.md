# OTel Collector (community)

Runs a purpose-built OpenTelemetry Collector (built with
[OCB](https://opentelemetry.io/docs/collector/extend/ocb/) from upstream components plus
our Home Assistant receiver) inside Home Assistant OS and ships telemetry to any OTLP/HTTP backend
(`http/protobuf`).

## What it collects

| Option | Signal | Source |
|---|---|---|
| `logs` | Logs | HAOS system journal: host services, kernel, Supervisor, Core and every add-on |
| `ha_traces` | Traces | Every automation and script run: a root span per run, a child span per step |
| `ha_metrics` | Metrics | Core `/api/prometheus` through the Supervisor proxy (no long-lived token) |
| `host_metrics` | Metrics | `system.cpu.*`, `system.memory.*`, `system.cpu.load_average.*`, `system.paging.*`, `system.network.*`, `system.uptime` |

## Configuration

```yaml
otlp_endpoint: http://192.0.2.10:4318
otlp_headers: []
service_name: homeassistant
service_instance_id: homeassistant.local:8123   # optional
deployment_environment: home
resource_attributes: []          # extra resource attributes, name/value
logs: true
logs_min_priority: info          # journald pre-filter, keep at info
logs_min_severity: warn          # final filter on the parsed severity
logs_parse: false                # JSON and Python log lines -> attributes
ha_metrics: true
ha_traces: true
host_metrics: true
profiles: false                  # experimental
scrape_interval: 60s
```

- `otlp_endpoint`: base OTLP/HTTP URL. Signal paths (`/v1/logs`, `/v1/metrics`) are appended.
  Credentials embedded in the URL are masked in the add-on log.
- `otlp_headers`: extra headers, typically the backend auth token. Values are hidden in the
  UI and never printed, but like every add-on option they are stored in plain text in the
  add-on's `options.json` and therefore end up in backups.
- `logs_min_priority`: journald priority read from the journal. **Keep it at `info`.** Docker
  writes every container line with priority 6 (stdout) or 3 (stderr), so a higher value drops
  whole containers regardless of what they log. Use `logs_min_severity` to cut verbosity.
- `logs_min_severity`: applied after parsing (`debug`, `info`, `warn`, `error`, `fatal`).
- `ha_metrics` needs the [Prometheus integration](https://www.home-assistant.io/integrations/prometheus/)
  (`prometheus:` in `configuration.yaml`). Without it the add-on log shows a scrape warning
  (HTTP 404) every interval.
- A `$` in a header or attribute value is passed through literally.

## Authenticated backends

Any OTLP/HTTP backend that authenticates with a header works through `otlp_headers`.

```yaml
# Dash0
otlp_endpoint: https://ingress.eu-west-1.aws.dash0.com
otlp_headers:
  - name: Authorization
    value: Bearer auth_xxxxxxxx
  - name: Dash0-Dataset          # optional
    value: default

# Grafana Cloud OTLP gateway (basic auth: base64 of "<instance id>:<token>")
otlp_endpoint: https://otlp-gateway-prod-eu-west-2.grafana.net/otlp
otlp_headers:
  - name: Authorization
    value: Basic <base64>

# Honeycomb
otlp_endpoint: https://api.honeycomb.io
otlp_headers:
  - name: x-honeycomb-team
    value: <api key>
```

## Logs

- Body: the journal `MESSAGE`, with ANSI colours removed. Records without a body are dropped,
  and so are the collector's own logs.
- Severity: for containers it is read from the text (`2026-01-01 12:00:00.000 WARNING (...)`
  and Python's default `WARNING:...:logger:...`); container lines without a level are INFO.
  For host services and the kernel it comes from the journal priority.
- Attributes: `systemd.unit`, `syslog.identifier`, `haos.container`, `journald.boot_id`.
- With `logs_parse: true`: JSON lines become attributes (`msg`/`message` as body, `level`
  as severity); Python lines add `thread.name` and `code.namespace` (logger) and keep only
  the message as body.

## Automation traces

`ha_traces` reads Home Assistant's own trace store (the one behind Settings → Automations →
Traces) over the WebSocket API and turns each finished run into a trace:

- Root span `automation <alias>` / `script <alias>` with the run's real start and end,
  `homeassistant.trigger`, `homeassistant.context.*` and error status.
- One span per step (trigger, condition, action...), nested like the automation (a condition
  sits under its `if`/`choose`, under its action) and named from the config, e.g.
  `action 2: Ligar resistência` or `conditions 0: state`. Home Assistant records only when a
  step starts, so a step ends when the next step outside it starts.
- The trace id is the run's context id (a ULID), so it can be correlated with HA logs.
- Runs already stored when the add-on starts are not replayed.
- Home Assistant keeps the last 5 runs per automation (`stored_traces`); more than 5 runs of
  one automation within 10 s lose the oldest.

## Profiles (experimental)

With `profiles: true` the add-on sends profiles of Home Assistant itself as
[OTel Profiles](https://opentelemetry.io/blog/2026/profiles-alpha/):

1. Turn on `profiles` and start the add-on. It installs the bundled **OTel Profiler**
   integration into `custom_components/otel_profiler` and shows a notification.
2. Restart Home Assistant. The add-on sets the integration up by itself; nothing to configure.

The integration samples every Python thread of Home Assistant 19 times per second
(`sys._current_frames()`, standard library only) and pushes a pprof profile to the add-on
every 10 s. It is a **wall-clock** profile: waiting threads (the event loop in `select`,
idle executor workers) appear too, labelled with `thread.name`. Only Python frames are
visible; time spent in C code is attributed to the Python function that called it.
Add-on updates update the integration too (restart Home Assistant when notified).

To stop: set `profiles: false` and remove the OTel Profiler integration (Settings → Devices &
services), then delete `custom_components/otel_profiler`.

The add-on also accepts profiles from anything else on the host:

- pprof: `POST http://127.0.0.1:4040/v1/pprof` with a serialized `profile.proto`
  (`Content-Type: application/octet-stream`, gzip allowed).
- OTLP/HTTP: `http://127.0.0.1:4318` (profiles; traces and logs are accepted too).

Profiles are alpha in the protocol, in the collector (`service.profilesSupport` feature gate,
enabled automatically) and in most backends; expect breaking changes. Check that your backend
accepts OTLP profiles.

## Resource attributes

`service.name`, `deployment.environment.name` and `host.name` (the HAOS host name, read from
the Supervisor) on every signal, plus `service.instance.id` and `resource_attributes` when set.

## Buffering

The journald cursor and the export queue live in `/data`. While the backend is unreachable,
telemetry queues on disk (up to 200 MB, retried forever) and is sent when it comes back; past
that cap new data is dropped. A crash can still lose the last few seconds of logs, and entries
are lost if the journal rotates past the saved cursor while the add-on is stopped.

## Known limitations

- OTLP/HTTP only (no gRPC, no custom CA yet).
- Multi-line Python tracebacks arrive as one record per line.
- Filesystem metrics are not collected: inside the add-on container the hostmetrics
  filesystem scraper sees no host partitions.
