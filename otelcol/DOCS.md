# OTel Collector (community)

This add-on runs an OpenTelemetry Collector inside Home Assistant OS. It collects logs,
automation traces, metrics and, optionally, profiles, and sends them to an observability
backend over OTLP/HTTP.

This is a community add-on. It is not affiliated with the OpenTelemetry project or with
Home Assistant.

**Contents**

- Concepts
- Get started
- Send data to your backend
- Configuration reference
- Logs
- Structured Core logs
- Event logs
- Automation traces
- Entity metrics
- Home Assistant metrics (Prometheus)
- Host metrics
- Profiles (experimental)
- Resource attributes
- Attribute reference
- Health sensors
- Buffering and outages
- Troubleshooting
- Privacy and security
- Known limitations
- FAQ

## Concepts

New to OpenTelemetry? These pages explain the terms used here:
[OpenTelemetry](https://opentelemetry.io/docs/what-is-opentelemetry/),
[Collector](https://opentelemetry.io/docs/collector/),
[OTLP](https://opentelemetry.io/docs/specs/otlp/),
[signals](https://opentelemetry.io/docs/concepts/signals/) (traces, metrics, logs,
profiles), [resources](https://opentelemetry.io/docs/concepts/resources/) and
[semantic conventions](https://opentelemetry.io/docs/concepts/semantic-conventions/).

## Get started

1. Get the OTLP endpoint and an API token from your backend. See
   "Send data to your backend" below.
2. On the add-on's **Configuration** tab, set **OTLP endpoint** (`otlp_endpoint`).
3. Under **OTLP headers** (`otlp_headers`), add the auth header your backend needs.
4. Select **Save**, then go to the **Info** tab and select **Start**.
5. Open the **Log** tab. A working start ends with a line like this:

   ```text
   [otelcol] host.name=homeassistant, exporting to https://otlp.example.com: logs logs_multiline ha_traces host_metrics entity_metrics container_metrics
   ```

Logs start with new journal entries. The add-on does not send entries written before its
first start.

## Send data to your backend

Any backend that accepts OTLP/HTTP and authenticates with a request header works. Set the
base URL in `otlp_endpoint`. The exporter appends the signal path (`/v1/logs`,
`/v1/metrics`, `/v1/traces`) itself, so do not include it.

### Send data to a vendor

Your vendor's OTLP documentation gives the endpoint and the auth header. Most use a bearer
token or an API key header:

```yaml
otlp_endpoint: https://otlp.example.com
otlp_headers:
  - name: Authorization
    value: Bearer <your-token>
```

Add more headers the same way if your vendor needs them, for example a dataset or tenant
header.

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
logs_multiline: true
logs_parse: false
ha_metrics: false
ha_traces: true
host_metrics: true
container_metrics: true
entity_metrics: true
entity_metrics_exclude: []
event_logs: true
event_logs_exclude: [state_changed]
core_logs: false
health_sensors: true
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
| `service_name` | string | `homeassistant` | `service.name` of Home Assistant Core and `service.namespace` of every signal. |
| `service_instance_id` | string, optional | not set | Sets `service.instance.id`, for example `homeassistant.local:8123`. Omitted when empty. |
| `deployment_environment` | string | `home` | Sets `deployment.environment.name`. |
| `resource_attributes` | list of `name`/`value` | `[]` | Extra resource attributes. Advanced: see "Resource attributes". |

### Signal options

| Option | Type | Default | Description |
|---|---|---|---|
| `logs` | bool | `true` | Send the Home Assistant OS system journal. |
| `logs_min_priority` | `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug` | `info` | Journal pre-filter. Keep it at `info`. See the note below. |
| `logs_min_severity` | `debug`, `info`, `warn`, `error`, `fatal` | `info` | Drops log records below this severity. Applied after the severity is parsed. Use this to cut volume. |
| `logs_multiline` | bool | `true` | Send a Python traceback from a container as one log record instead of one per line. |
| `logs_parse` | bool | `false` | Advanced. Turns JSON and Python log lines into attributes. See "Logs". |
| `ha_traces` | bool | `true` | Send every automation and script run as a trace. |
| `entity_metrics` | bool | `true` | Send every entity state as a metric, with device, area and floor. See "Entity metrics". |
| `entity_metrics_exclude` | list of entity ids | `[]` | Entities left out of entity metrics. Wildcards work, for example `sensor.*_linkquality`. |
| `event_logs` | bool | `true` | Send Home Assistant events as logs, linked to the automation trace that caused them. See "Event logs". |
| `event_logs_exclude` | list of event types | `[state_changed]` | Event types not sent. |
| `core_logs` | bool | `false` | Install the bundled OTel Logs integration: Home Assistant's logs arrive structured and linked to automation traces. See "Structured Core logs". |
| `health_sensors` | bool | `true` | Publish the add-on's export health as sensors in Home Assistant. See "Health sensors". |
| `ha_metrics` | bool | `false` | Scrape Home Assistant's Prometheus endpoint instead. Needs the Prometheus integration. |
| `host_metrics` | bool | `true` | Send CPU, memory, load, paging, network, data disk and uptime metrics of the machine. |
| `container_metrics` | bool | `true` | Send CPU, memory, disk and network of Home Assistant Core, the Supervisor and every started add-on. |
| `profiles` | bool | `false` | Experimental. Send profiles of Home Assistant's Python threads. See "Profiles (experimental)". |
| `scrape_interval` | `<n>s` or `<n>m` | `60s` | How often `entity_metrics`, `ha_metrics`, `host_metrics` and `container_metrics` collect. Must be greater than zero. Traces are polled every 10 s regardless. |

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

The body is the journal `MESSAGE`, with ANSI colour codes removed. With `logs_multiline` (on by
default), a Python traceback from a container arrives as one record: its lines are joined
to the line that started it.

For containers, the add-on reads the severity from the text. It recognises two formats:

- Home Assistant style: `2026-01-01 12:00:00.000 WARNING (MainThread) [logger] text`
- Python default: `WARNING:2026-01-01 12:00:00,000:logger:text`

Container lines without a recognised level are `INFO`. For host services and the kernel,
the severity comes from the journal priority:

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
| `systemd.unit` | The journal has `_SYSTEMD_UNIT` | `docker.service` |
| `syslog.identifier` | The journal has `SYSLOG_IDENTIFIER` | `kernel` |
| `journald.boot_id` | The journal has `_BOOT_ID` | `3e1c0f6b2a9d4e8f9b7c6d5e4f3a2b1c` |

An entry from a container gets its own resource with `container.name` (`homeassistant`,
`hassio_supervisor`, `addon_core_ssh`...), as the semantic conventions define it. Host
services and the kernel get the host resource (`service.name=haos`). See "Resource
attributes".

### What is dropped

- Records with an empty or missing body.
- The add-on's own logs. Otherwise each export error would be exported again.
- Records below `logs_min_severity`.

### Parse log lines (advanced)

With `logs_parse: true`:

- In JSON lines, every top-level key becomes a log attribute and `message` or `msg`
  becomes the body. A `level` key (`trace`, `debug`, `info`, `warn`, `warning`, `error`,
  `fatal`, `panic`, `critical`) sets the severity.
- In Python lines, the body keeps only the message. The thread goes to `thread.name`
  (Home Assistant style only) and the logger to `python.logger.name`.

`logs_min_severity` is applied after parsing, so a JSON `level` counts.

## Structured Core logs

With `core_logs: true`, the add-on installs the bundled **OTel Logs** integration, the
same way `profiles` installs the profiler. Restart Home Assistant once; the add-on then
sets the integration up by itself.

The integration hands every log record Home Assistant writes to the add-on, instead of
the journal line:

- The logger name is the instrumentation scope, for example
  `homeassistant.components.mqtt`.
- `code.file.path`, `code.line.number`, `code.function.name`, `thread.name`, `thread.id`.
- An exception becomes `exception.type`, `exception.message` and `exception.stacktrace` on
  the same record.
- A record written while an automation or script runs carries that run's trace id and the
  id of its root span, so your backend shows the log inside the run's trace.

The journal copy of Home Assistant Core's lines is dropped while `core_logs` is on, so
nothing arrives twice. Records follow your `logger:` levels and `logs_min_severity`.

To stop: set `core_logs: false`, delete the **OTel Logs** integration in **Settings →
Devices & services**, delete `/config/custom_components/otel_logs` and restart Home
Assistant.

## Event logs

The `event_logs` option sends every Home Assistant bus event as a log record, as it
happens: `call_service`, `automation_triggered`, `script_started`, `homeassistant_start`
and so on. `state_changed` is left out by default because entity metrics already cover
states; remove it from `event_logs_exclude` if you want every change.

- The record is an [OpenTelemetry event](https://opentelemetry.io/docs/specs/semconv/general/events/)
  named `homeassistant.<event_type>`, for example `homeassistant.call_service`. The
  payload is in the attributes; there is no body.
- Its trace id comes from the event's context id, the same way a run's trace id does. The
  events an automation causes, such as its `call_service` calls, appear in that run's
  trace.
- Attributes: `homeassistant.event.type`, `homeassistant.event.origin`,
  `homeassistant.context.id`, `homeassistant.context.parent_id`, `user.id`, and one
  `homeassistant.event.data.<key>` per data key (nested values as JSON).

## Automation traces

The `ha_traces` option turns each finished automation or script run into a trace. It
reads the same data as the **Traces** view of an automation in **Settings → Automations &
scenes**, over the Home Assistant WebSocket API. You do not need a token: the add-on uses
its Supervisor token.

### How a run maps to a trace

The trace id is the run's Home Assistant context id (a ULID), so you can search for the
same id in Home Assistant logs. If the context id is not a ULID, the trace id is a hash of
the run id.

The root span is named `automation <alias>` or `script <alias>` (the item id when there is
no alias) and lasts from the run's real start to its end. Under it there is one span per
trigger, condition and action, nested like the configuration: a condition of a `choose`
sits under its `choose` action.

Step names come from the configuration: the path part, the index, and the alias or type,
for example `action 2: Ligar resistência`, `action 3: switch.turn_on`, `action 0: choose`
or `conditions 0: state`. A run or step with an error gets status `ERROR` and the error
message.

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
| `homeassistant.last_step` | root | Path of the last step the run reached |
| `user.id` | root | Home Assistant user who started the run, when there is one |
| `homeassistant.step.path` | step | Path of the step in the configuration, for example `action/1/if/condition/0` |
| `homeassistant.step.result.<key>` | step | Scalar values of the step result, for example `homeassistant.step.result.result` of a condition |
| `homeassistant.step.child` | step | Run started by the step, as `domain.item_id/run_id` |

### Trace limitations

- Step durations are inferred. Home Assistant records only when a step starts, so a step
  ends when the next step outside it starts, or when the run ends.
- Runs already in the trace store when the add-on starts are skipped.
- Home Assistant keeps the last 5 runs per automation by default (`stored_traces`). The
  add-on polls every 10 s, so more than 5 runs of one automation within 10 s lose the
  oldest. Raise `stored_traces` in the automation's `trace:` settings if this matters.
- The trace store is not a stable API. A Home Assistant release can change its format.

## Entity metrics

The `entity_metrics` option reads every entity state over the WebSocket API, every
`scrape_interval`, and sends it as a gauge. It needs no integration and no token.

| Metric | Value |
|---|---|
| `homeassistant.sensor.<device_class>` | Numeric state of a sensor with a device class, for example `homeassistant.sensor.temperature`. The unit comes from the entity. |
| `homeassistant.<domain>.value` | Numeric state of other sensors, `number`, `input_number` and `counter` entities |
| `homeassistant.<domain>.on` | `1` or `0` for entities whose state is `on` or `off`, for example `homeassistant.switch.on` |
| `homeassistant.entity.available` | `0` when the entity is `unavailable`, else `1` |
| `homeassistant.entity.last_updated` | Unix time (seconds) of the entity's last state update |
| `homeassistant.core.up` | `1` when Home Assistant Core answered at this scrape, `0` when it did not. No entity attributes. Alert on it for "Home Assistant down". |

Every data point carries the entity and where it is: `homeassistant.entity.id`,
`.domain`, `.name`, `.device_class`, `.platform`, `homeassistant.device.name`,
`.manufacturer`, `.model`, `homeassistant.area.name` and `homeassistant.floor.name`. Use
`entity_metrics_exclude` to leave out noisy entities.

## Home Assistant metrics (Prometheus)

The `ha_metrics` option, off by default, scrapes Home Assistant's `/api/prometheus` endpoint through the
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
| Disk | `system.filesystem.*` of the data partition (mount point `/data`) |
| Containers (`container_metrics`) | `container.cpu.usage`, `container.memory.usage`, `container.disk.io`, `container.network.io` of Home Assistant Core, the Supervisor and every started add-on, one resource per container with `container.name` |

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

The profiler takes wall-clock samples of every Python thread of Home Assistant, 19 times
per second, so waiting threads (the event loop in `select`, idle executor workers) appear
too. Each sample carries the thread name as the `thread.name` label. Only Python frames are
visible: time spent in C code counts toward the Python function that called it.

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

Resources follow the [semantic conventions](https://opentelemetry.io/docs/specs/semconv/resource/):
each component is its own service, and `service.namespace` groups them all.

| Component | `service.name` | `container.name` | `service.version` |
|---|---|---|---|
| Home Assistant Core: entity metrics, event logs, traces, Core logs, `ha_metrics`, profiles, its journal lines and container metrics | `service_name` option (`homeassistant`) | `homeassistant` | Core version |
| Supervisor | `supervisor` | `hassio_supervisor` | |
| An add-on | its slug, for example `core_ssh` | `addon_<slug>` | |
| Other Supervisor containers | the name without `hassio_`, for example `dns` | `hassio_<name>` | |
| The host: kernel and systemd journal lines, host metrics | `haos` | | Home Assistant OS version |

Every resource also carries:

| Attribute | Source |
|---|---|
| `service.namespace` | `service_name` option |
| `deployment.environment.name` | `deployment_environment` option |
| `host.name` | Host name of the Home Assistant OS machine, read from the Supervisor. Falls back to `homeassistant`. |
| `service.instance.id` | `service_instance_id` option, when set |
| `os.type`, `os.description` | `linux` and the Home Assistant OS version, for example `Home Assistant OS 18.4` |
| `host.arch` | Machine architecture, for example `arm64` |

To see everything from this add-on, filter on `service.namespace`; to see Home Assistant
Core only, on `service.name`.

**Note:** Prometheus-compatible backends build the `job` label from
`service.namespace/service.name`, so Home Assistant's `up` series has
`job="homeassistant/homeassistant"`. Filter on `service_name` instead of `job`.

### Add your own resource attributes (advanced)

Use `resource_attributes` for backend-specific attributes:

```yaml
resource_attributes:
  - name: location
    value: lisbon
```

The add-on applies these last, on every signal: a name that is already set, such as
`service.namespace`, is overwritten everywhere.

## Attribute reference

Every attribute the add-on emits. "Semconv" means an [OpenTelemetry semantic convention](https://opentelemetry.io/docs/specs/semconv/).
"Custom" means defined in the add-on's Weaver registry, in the
[`model/` folder of the repository](https://github.com/nicolastakashi/ha-addon-otelcol/tree/main/model).
The registry builds on semantic conventions v1.37.0.

| Attribute | Level | Signal | Defined by |
|---|---|---|---|
| `service.name`, `service.namespace` | resource | all | Semconv |
| `service.instance.id` | resource | all | Semconv |
| `deployment.environment.name` | resource | all | Semconv |
| `host.name` | resource | all | Semconv |
| `service.version`, `os.type`, `os.description`, `host.arch` | resource | all | Semconv |
| `container.name` | resource | container logs and metrics; `homeassistant` on Core logs, events, entity metrics and traces | Semconv |
| `systemd.unit` | log | logs | Custom |
| `syslog.identifier` | log | logs | Custom |
| `journald.boot_id` | log | logs | Custom |
| `thread.name` | log | logs (`logs_parse`) | Semconv |
| `python.logger.name` | log | logs (`logs_parse`) | Custom |
| JSON keys | log | logs (`logs_parse`) | From the log line |
| `homeassistant.*` | span, metric, log | traces, entity metrics, event logs | Custom |
| `user.id` | span, log | traces, event logs | Semconv |
| `thread.name` | sample label | profiles | Semconv |
| `system.*` | metric names | host metrics | Semconv |

Metric names from `ha_metrics` come from Home Assistant, not from semantic conventions.

## Health sensors

With `health_sensors` (on by default), the add-on publishes its export health in Home
Assistant every `scrape_interval`:

| Entity | State |
|---|---|
| `sensor.otel_collector_exported` | Log records, spans and metric points sent since the add-on started |
| `sensor.otel_collector_dropped` | Items dropped because the queue was full |
| `sensor.otel_collector_queue` | Fullest export queue, in percent of its 200 MB |
| `sensor.otel_collector_status` | `error` while the queue grows and nothing is sent (backend unreachable or rejecting data), else `ok` |

Use them in a dashboard or in an automation that notifies you when the status is `error`.
They are plain states: after a Home Assistant restart they come back at the next round.

## Buffering and outages

The add-on stores data on disk while your backend is unreachable, and sends it when the
backend comes back.

The queue holds up to 200 MB in the add-on's `/data` folder and retries with no time
limit. Past 200 MB, new data is dropped. The add-on also saves its place in the journal in
`/data`, so after a restart it continues from there.

You can still lose the last few seconds of logs after a crash, and journal entries that
rotate out while the add-on is stopped.

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

- Restart Home Assistant. The integration loads only after a restart; until then the
  add-on retries the setup every 30 seconds. When it succeeds, the log shows
  `OTel Profiler integration is set up.`
- Check that your backend accepts OTLP profiles.
- If the Home Assistant log shows `Cannot push profiles to http://127.0.0.1:4040/v1/pprof`,
  the add-on is stopped or `profiles` is off.

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

The add-on sends data only to `otlp_endpoint`. Logs and traces can contain personal data,
such as entity names, automation aliases, user ids and anything add-ons log.

Header values are hidden in the UI and never printed, but they are not encrypted at rest.
Like every add-on option, they are stored in plain text in the add-on's `options.json`, so
they end up in backups. Use a token with write-only (ingest) scope.

With `profiles` or `core_logs` on, the local endpoints `127.0.0.1:4040` and `127.0.0.1:4318`
listen on the loopback interface of the host network. Other processes on the host, including add-ons
with host networking, can send to them.

The add-on needs the system journal (read-only), the Home Assistant and Supervisor APIs,
host networking, the Supervisor `manager` role to read the resource usage of Core and the
add-ons, and read-write access to the Home Assistant configuration folder to install the
profiler and logs integrations. Its own AppArmor profile allows, besides its image
files, reads of the journal, `/proc` and `/sys`, and writes only to `/data`, `/tmp` and
`custom_components/otel_profiler`.

## Known limitations

- OTLP/HTTP only. No gRPC, no custom CA certificates.
- Profiles are experimental and cover Python frames only.

## FAQ

### Why only OTLP/HTTP?

The collector includes one exporter, OTLP/HTTP. Most OTLP backends accept it, and one
exporter keeps the build and the configuration small. gRPC is not included.

### Why not eBPF profiling?

eBPF profilers need the `CAP_SYSLOG` capability to read kernel symbols
(`/proc/kallsyms`). The Supervisor does not grant it to add-ons.

### Does the add-on need a long-lived access token?

No. It uses the Supervisor token that every add-on gets.

### Is this an official OpenTelemetry or Home Assistant add-on?

No. It is an independent community project. "OpenTelemetry" and "OTel" are trademarks
of The Linux Foundation and are used only to describe what the add-on runs.
