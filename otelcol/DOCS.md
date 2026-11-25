# OTel Collector (community)

This add-on runs an OpenTelemetry Collector inside Home Assistant OS. It collects logs,
automation traces, metrics and, optionally, profiles, and sends them to an observability
backend over OTLP/HTTP.

This is a community add-on. It is not affiliated with the OpenTelemetry project or with
Home Assistant.

**Contents**

- Get started
- Send data to your backend
- Configuration reference
- Logs
- Automation traces
- Home Assistant metrics
- Host metrics
- Profiles (experimental)
- Resource attributes
- Attribute reference
- Buffering and outages
- Troubleshooting
- Privacy and security
- Known limitations
- FAQ

## Terms used here

| Term | Meaning |
|---|---|
| OpenTelemetry (OTel) | An open standard for logs, metrics, traces and profiles. |
| Collector | A program that receives, processes and exports telemetry. This add-on runs one, called `otelcol-ha`. |
| OTLP | The OpenTelemetry protocol. The add-on sends OTLP over HTTP (`http/protobuf`). |
| Backend | The service that stores and shows your data, for example Dash0, Grafana Cloud or Honeycomb. |
| Trace, span | A trace is one operation, such as one automation run. It is made of spans: one per step. |
| Resource attribute | A key/value pair that describes the source of the data, such as `host.name`. It is set on every signal. |
| Semantic conventions (semconv) | The standard OpenTelemetry attribute names, such as `container.name`. |

## Get started

1. Get the OTLP endpoint and an API token from your backend. See
   "Send data to your backend" below.
2. On the add-on's **Configuration** tab, set **OTLP endpoint** (`otlp_endpoint`).
3. Under **OTLP headers** (`otlp_headers`), add the auth header your backend needs.
4. Select **Save**, then go to the **Info** tab and select **Start**.
5. Open the **Log** tab. A working start ends with a line like this:

   ```text
   [otelcol] host.name=homeassistant, exporting to https://otlp.example.com: logs ha_metrics ha_traces host_metrics
   ```

6. Optional, for Home Assistant metrics: add the Prometheus integration to
   `configuration.yaml` and restart Home Assistant:

   ```yaml
   prometheus:
   ```

Logs start with new journal entries. The add-on does not send entries written before its
first start.

## Send data to your backend

Any backend that accepts OTLP/HTTP and authenticates with a request header works. Set the
base URL in `otlp_endpoint`. The exporter appends the signal path (`/v1/logs`,
`/v1/metrics`, `/v1/traces`) itself, so do not include it.

### Send data to Dash0

```yaml
otlp_endpoint: https://ingress.eu-west-1.aws.dash0.com
otlp_headers:
  - name: Authorization
    value: Bearer auth_xxxxxxxx
  - name: Dash0-Dataset
    value: default
```

`Dash0-Dataset` is optional. Use the ingress URL of your Dash0 region.

### Send data to Grafana Cloud

Grafana Cloud uses basic auth. The value is the base64 encoding of
`<instance-id>:<token>`.

```yaml
otlp_endpoint: https://otlp-gateway-prod-eu-west-2.grafana.net/otlp
otlp_headers:
  - name: Authorization
    value: Basic <base64-of-instance-id-and-token>
```

Use the OTLP gateway URL of your Grafana Cloud stack.

### Send data to Honeycomb

```yaml
otlp_endpoint: https://api.honeycomb.io
otlp_headers:
  - name: x-honeycomb-team
    value: <your-api-key>
```

### Send data to a collector on your network

```yaml
otlp_endpoint: http://192.0.2.10:4318
otlp_headers: []
```

## Configuration reference

The options as they appear in YAML mode, with their defaults:

```yaml
otlp_endpoint: ""
otlp_headers: []
service_name: homeassistant
deployment_environment: home
resource_attributes: []
logs: true
logs_min_priority: info
logs_min_severity: info
logs_parse: false
ha_metrics: true
ha_traces: true
host_metrics: true
profiles: false
scrape_interval: 60s
```

`service_instance_id` is optional and has no default.

### Export options

| Option | Type | Default | Description |
|---|---|---|---|
| `otlp_endpoint` | string | none (required) | Base OTLP/HTTP URL of your backend. The add-on does not start without it. Credentials in the URL (`https://user:pass@host`) are masked in the log. |
| `otlp_headers` | list of `name`/`value` | `[]` | Request headers, usually the auth token. Values are hidden in the UI and never printed in the log. |

### Resource options

| Option | Type | Default | Description |
|---|---|---|---|
| `service_name` | string | `homeassistant` | Sets `service.name` on every signal. |
| `service_instance_id` | string, optional | not set | Sets `service.instance.id`, for example `homeassistant.local:8123`. Omitted when empty. |
| `deployment_environment` | string | `home` | Sets `deployment.environment.name`. |
| `resource_attributes` | list of `name`/`value` | `[]` | Extra resource attributes. Advanced: see "Resource attributes". |

### Signal options

| Option | Type | Default | Description |
|---|---|---|---|
| `logs` | bool | `true` | Send the Home Assistant OS system journal. |
| `logs_min_priority` | `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug` | `info` | Journal pre-filter. Keep it at `info`. See the note below. |
| `logs_min_severity` | `debug`, `info`, `warn`, `error`, `fatal` | `info` | Drops log records below this severity. Applied after the severity is parsed. Use this to cut volume. |
| `logs_parse` | bool | `false` | Advanced. Turns JSON and Python log lines into attributes. See "Logs". |
| `ha_traces` | bool | `true` | Send every automation and script run as a trace. |
| `ha_metrics` | bool | `true` | Scrape Home Assistant's Prometheus endpoint. Needs the Prometheus integration. |
| `host_metrics` | bool | `true` | Send CPU, memory, load, paging, network and uptime metrics of the machine. |
| `profiles` | bool | `false` | Experimental. Send profiles of Home Assistant's Python threads. See "Profiles (experimental)". |
| `scrape_interval` | `<n>s` or `<n>m` | `60s` | How often `ha_metrics` and `host_metrics` collect. Must be greater than zero. Traces are polled every 10 s regardless. |

**Note:** Keep `logs_min_priority` at `info`. Docker writes every container line to the
journal with priority 6 (`info`, stdout) or 3 (`err`, stderr). A stricter value such as
`warning` drops all stdout lines of Home Assistant Core, the Supervisor and every add-on,
whatever their real level. To reduce volume, raise `logs_min_severity` instead.

A `$` in a header or attribute value is sent as is. You do not need to escape it.

If you turn off every signal, the add-on stops with
`every feature is disabled, nothing to do`.

## Logs

The `logs` option reads the Home Assistant OS systemd journal. It covers host services,
the kernel, the Supervisor, Home Assistant Core and every add-on container.

### What a log record contains

- **Body**: the journal `MESSAGE`, with ANSI colour codes removed.
- **Severity for containers**: parsed from the text. Two formats are recognised:
  - Home Assistant style: `2026-01-01 12:00:00.000 WARNING (MainThread) [logger] text`
  - Python default: `WARNING:2026-01-01 12:00:00,000:logger:text`

  Container lines without a recognised level are `INFO`.
- **Severity for host services and the kernel**: taken from the journal priority.

| Journal priority | Severity |
|---|---|
| 0 to 2 (`emerg`, `alert`, `crit`) | `FATAL` |
| 3 (`err`) | `ERROR` |
| 4 (`warning`) | `WARN` |
| 5 to 6 (`notice`, `info`) | `INFO` |
| 7 (`debug`) | `DEBUG` |

### Log attributes

| Attribute | When | Example |
|---|---|---|
| `container.name` | The entry comes from a container | `homeassistant`, `hassio_supervisor`, `addon_core_ssh` |
| `systemd.unit` | The journal has `_SYSTEMD_UNIT` | `docker.service` |
| `syslog.identifier` | The journal has `SYSLOG_IDENTIFIER` | `kernel` |
| `journald.boot_id` | The journal has `_BOOT_ID` | `3e1c0f6b2a9d4e8f9b7c6d5e4f3a2b1c` |

`container.name` is a log attribute, not a resource attribute, so all logs share one
resource (`service.name`).

### What is dropped

- Records with an empty or missing body.
- The add-on's own logs. Otherwise each export error would be exported again.
- Records below `logs_min_severity`.

### Parse log lines (advanced)

With `logs_parse: true`:

- **JSON lines**: every top-level key becomes a log attribute. `message` or `msg` becomes
  the body. A `level` key (`trace`, `debug`, `info`, `warn`, `warning`, `error`, `fatal`,
  `panic`, `critical`) sets the severity.
- **Python lines**: the body keeps only the message. The thread and logger move to
  `thread.name` (Home Assistant style only) and `python.logger.name`.

`logs_min_severity` is applied after parsing, so a JSON `level` counts.

## Automation traces

The `ha_traces` option turns each finished automation or script run into a trace. It
reads the same data as the **Traces** view of an automation in **Settings → Automations &
scenes**, over the Home Assistant WebSocket API. It needs no token: the add-on uses its Supervisor token.

### How a run maps to a trace

- **Trace id**: the run's Home Assistant context id (a ULID). Search for the same id in Home
  Assistant logs to correlate. If the context id is not a ULID, the trace id is a hash of
  the run id.
- **Root span**: `automation <alias>` or `script <alias>`, from the run's real start to its
  end. Without an alias, the item id replaces it.
- **Step spans**: one per trigger, condition and action, nested like the configuration. A
  condition of a `choose` sits under its `choose` action.
- **Step names** come from the configuration: the path part, the index, and the alias or
  type. Examples: `action 2: Ligar resistência`, `action 3: switch.turn_on`,
  `action 0: choose`, `conditions 0: state`.
- **Errors**: a run or step with an error gets status `ERROR` and the error message.

### Span attributes

| Attribute | Span | Description |
|---|---|---|
| `homeassistant.domain` | root | `automation` or `script` |
| `homeassistant.item_id` | root | Id of the automation or script |
| `homeassistant.run_id` | root | Id of the run in the trace store |
| `homeassistant.alias` | root | Name of the automation or script |
| `homeassistant.trigger` | root | What triggered the run, for example `time pattern` |
| `homeassistant.script_execution` | root | How the run ended: `finished`, `failed_conditions`, `error`, `cancelled`... |
| `homeassistant.context.id` | root | Context id of the run |
| `homeassistant.context.parent_id` | root | Context id of the event or run that caused it |
| `user.id` | root | Home Assistant user who started the run, when there is one |
| `homeassistant.step.path` | step | Path of the step in the configuration, for example `action/1/if/condition/0` |
| `homeassistant.step.child` | step | Run started by the step, as `domain.item_id/run_id` |

### Trace limitations

- **Step durations are inferred.** Home Assistant records only when a step starts. A step
  ends when the next step outside it starts, or when the run ends.
- **Old runs are not replayed.** Runs already in the trace store when the add-on starts
  are skipped.
- **Bursts can lose runs.** Home Assistant keeps the last 5 runs per automation by default
  (`stored_traces`). The add-on polls every 10 s, so more than 5 runs of one automation
  within 10 s lose the oldest. Raise `stored_traces` in the automation's `trace:` settings
  if this matters.
- **The trace store is not a stable API.** A Home Assistant release can change its format.

## Home Assistant metrics

The `ha_metrics` option scrapes Home Assistant's `/api/prometheus` endpoint through the
Supervisor proxy, every `scrape_interval`. You do not need a long-lived access token.

It requires the [Prometheus integration](https://www.home-assistant.io/integrations/prometheus/).
Add `prometheus:` to `configuration.yaml` and restart Home Assistant. Use the
integration's `filter:` to choose which entities are exported.

Metric names are the ones Home Assistant exposes, for example
`homeassistant_sensor_temperature_celsius`. They do not follow OpenTelemetry semantic
conventions.

## Host metrics

The `host_metrics` option sends metrics of the machine that runs Home Assistant OS, every
`scrape_interval`.

| Group | Metrics |
|---|---|
| CPU | `system.cpu.*` |
| Load | `system.cpu.load_average.*` |
| Memory | `system.memory.*` |
| Paging | `system.paging.*` |
| Network | `system.network.*`, except `system.network.connections` |
| Uptime | `system.uptime` |

Host disk usage is not collected: inside the add-on container the host partitions are not
visible. Use Home Assistant's System Monitor integration for disk usage.

## Profiles (experimental)

A profile shows where Home Assistant spends its time, down to the Python function. With
`profiles: true`, the add-on sends profiles of Home Assistant Core as
[OTel Profiles](https://opentelemetry.io/blog/2026/profiles-alpha/).

**Note:** OTel Profiles are alpha in the protocol, in the collector and in most backends.
Expect breaking changes. Check that your backend accepts OTLP profiles first.

### Turn on profiles

1. Set `profiles: true` and restart the add-on.
   The add-on copies the bundled **OTel Profiler** integration to
   `/config/custom_components/otel_profiler` and shows a notification.
2. Restart Home Assistant.
   The add-on then creates the integration's config entry by itself. The add-on log shows
   `OTel Profiler integration is set up.` You do not need to configure anything.

### What the profiler samples

- **Wall-clock samples** of every Python thread of Home Assistant, 19 times per second.
  Waiting threads, such as the event loop in `select` or idle executor workers, appear too.
- **Python frames only.** Time spent in C code is attributed to the Python function that
  called it.
- **Thread name** is a sample label, `thread.name`.

The integration uses only the Python standard library (`sys._current_frames()`). It pushes
a pprof profile to the add-on at `127.0.0.1:4040` every 10 seconds.

When you update the add-on, it updates the integration too. Restart Home Assistant when the
notification asks you to.

### Turn off profiles

1. Set `profiles: false` and restart the add-on.
2. Go to **Settings → Devices & services** and delete the **OTel Profiler** integration.
3. Delete the folder `/config/custom_components/otel_profiler`.
4. Restart Home Assistant.

Until you remove the integration, it keeps sampling and logs one warning that it cannot
reach the add-on.

### Send profiles from other programs

While `profiles` is on, the add-on also accepts profiles from other processes on the host:

| Protocol | Endpoint | Format |
|---|---|---|
| pprof | `POST http://127.0.0.1:4040/v1/pprof` | Serialized `profile.proto`, `Content-Type: application/octet-stream`, gzip allowed |
| OTLP/HTTP | `http://127.0.0.1:4318` | OTLP profiles |

Only profiles are forwarded from these endpoints. Traces, logs and metrics sent to port
4318 are not exported.

eBPF profiling, which would cover native code too, is not possible on Home Assistant OS.
It needs the `CAP_SYSLOG` capability to read kernel symbols, and the Supervisor does not
grant it to add-ons.

## Resource attributes

Every signal carries these resource attributes:

| Attribute | Source |
|---|---|
| `service.name` | `service_name` option |
| `deployment.environment.name` | `deployment_environment` option |
| `host.name` | Host name of the Home Assistant OS machine, read from the Supervisor. Falls back to `homeassistant`. |
| `service.instance.id` | `service_instance_id` option, when set |

### Add your own resource attributes (advanced)

Use `resource_attributes` for backend-specific attributes:

```yaml
resource_attributes:
  - name: service.namespace
    value: home
  - name: location
    value: lisbon
```

The add-on applies these after the built-in ones. A name that is already set, such as
`service.name`, is overwritten.

## Attribute reference

Every attribute the add-on emits. "Semconv" means an OpenTelemetry semantic convention.
"Custom" means defined in the add-on's Weaver registry, in the
[`model/` folder of the repository](https://github.com/nicolastakashi/ha-addon-otelcol/tree/main/model).
The registry builds on semantic conventions v1.37.0.

| Attribute | Level | Signal | Defined by |
|---|---|---|---|
| `service.name` | resource | all | Semconv |
| `service.instance.id` | resource | all | Semconv |
| `deployment.environment.name` | resource | all | Semconv |
| `host.name` | resource | all | Semconv |
| `container.name` | log | logs | Semconv |
| `systemd.unit` | log | logs | Custom |
| `syslog.identifier` | log | logs | Custom |
| `journald.boot_id` | log | logs | Custom |
| `thread.name` | log | logs (`logs_parse`) | Semconv |
| `python.logger.name` | log | logs (`logs_parse`) | Custom |
| JSON keys | log | logs (`logs_parse`) | From the log line |
| `homeassistant.*` | span | traces | Custom |
| `user.id` | span | traces | Semconv |
| `thread.name` | sample label | profiles | Semconv |
| `system.*` | metric names | host metrics | Semconv |

Metric names from `ha_metrics` come from Home Assistant, not from semantic conventions.

**Note:** Version 0.6.0 renamed three attributes. Update saved queries and dashboards:

| Before 0.6.0 | From 0.6.0 |
|---|---|
| `haos.container` | `container.name` |
| `homeassistant.context.user_id` | `user.id` |
| `code.namespace` | `python.logger.name` |

## Buffering and outages

The add-on stores data on disk while your backend is unreachable, and sends it when the
backend comes back.

- **Queue**: up to 200 MB in the add-on's `/data` folder, retried with no time limit. Past
  200 MB, new data is dropped.
- **Journal position**: the add-on saves its place in the journal in `/data`. After a
  restart it continues from there.
- **What can still be lost**: the last few seconds of logs after a crash, and journal
  entries that rotate out while the add-on is stopped.

## Troubleshooting

### Read the add-on log

Open **Settings → Add-ons → OTel Collector (community) → Log**. Lines from the start
script begin with `[otelcol]`. The line `exporting to <endpoint>: <signals>` lists the
signals that are on. Collector messages follow it.

### The add-on stops right after it starts

| Log message | Fix |
|---|---|
| `otlp_endpoint is empty` | Set **OTLP endpoint** on the **Configuration** tab. |
| `every feature is disabled, nothing to do` | Turn on at least one signal. |

### Home Assistant metrics show HTTP 404

The Prometheus integration is not set up. The log shows a scrape warning with status 404
every `scrape_interval`. Add `prometheus:` to `configuration.yaml` and restart Home
Assistant, or set `ha_metrics: false`.

### Metrics show HTTP 401, or host.name is homeassistant

The log shows `no SUPERVISOR_TOKEN`. The add-on did not get its Supervisor token. Restart
the add-on.

### Logs from containers are missing

Check `logs_min_priority`. Any value stricter than `info` drops the stdout of every
container. Set it back to `info` and use `logs_min_severity` to filter.

Also check `logs_min_severity`. At `warn`, `INFO` lines are dropped, including container
lines with no recognised level.

### Profiles do not arrive

- **Home Assistant not restarted.** The integration loads only after a restart. Until then
  the add-on retries the setup every 30 seconds. When it succeeds, the log shows
  `OTel Profiler integration is set up.`
- **Backend does not accept profiles.** Check your backend's OTLP profiles support.
- **Home Assistant log shows `Cannot push profiles to http://127.0.0.1:4040/v1/pprof`.**
  The add-on is stopped or `profiles` is off.

### No automation traces

- Only runs that finish after the add-on starts are sent. Trigger an automation to test.
- `Home Assistant connection failed, retrying` or `Polling Home Assistant traces failed,
  reconnecting` in the log means the WebSocket connection failed. The receiver retries,
  backing off up to one minute. It recovers when Home Assistant Core is back.

### The backend is unreachable or rejects data

The collector logs export errors and keeps the data in the queue (see "Buffering and
outages"). Check:

- `otlp_endpoint` has the right scheme (`https://` or `http://`) and no signal path.
- The auth header name and value. A 401 or 403 from the backend means the token is wrong.
- The backend's OTLP/HTTP port. A self-hosted collector usually listens on 4318.

## Privacy and security

- **Where data goes**: only to `otlp_endpoint`. Logs and traces can contain personal data,
  such as entity names, automation aliases, user ids and anything add-ons log.
- **Header values are not encrypted at rest.** They are hidden in the UI and never printed,
  but like every add-on option they are stored in plain text in the add-on's
  `options.json`, so they end up in backups. Use a token with write-only (ingest) scope.
- **Local endpoints** (`127.0.0.1:4040` and `127.0.0.1:4318`, `profiles` only) listen on
  the loopback interface of the host network. Other processes on the host, including
  add-ons with host networking, can send to them.
- **Access the add-on needs**: the system journal (read-only), the Home Assistant and
  Supervisor APIs, host networking, and read-write access to the Home Assistant
  configuration folder to install the profiler integration.
- **AppArmor**: the add-on ships its own AppArmor profile. Besides its own image files, it
  allows reads of the journal, `/proc` and `/sys`, and writes only to `/data`, `/tmp` and
  `custom_components/otel_profiler`.

## Known limitations

- OTLP/HTTP only. No gRPC, no custom CA certificates.
- Multi-line Python tracebacks arrive as one log record per line.
- No filesystem metrics from the host.
- Profiles are experimental and cover Python frames only.

## FAQ

### Why only OTLP/HTTP?

The collector includes one exporter, OTLP/HTTP. All the backends in this guide accept it,
and one exporter keeps the build and the configuration small. gRPC is not included.

### Why not eBPF profiling?

eBPF profilers need the `CAP_SYSLOG` capability to read kernel symbols
(`/proc/kallsyms`). The Supervisor does not grant it to add-ons.

### Does the add-on need a long-lived access token?

No. It uses the Supervisor token that every add-on gets.

### Is this an official OpenTelemetry or Home Assistant add-on?

No. It is an independent community project. "OpenTelemetry" and "OTel" are trademarks
of The Linux Foundation and are used only to describe what the add-on runs.
