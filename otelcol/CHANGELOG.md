# Changelog

## 0.17.0

### Added

- Entity metrics: numeric state attributes as `homeassistant.<domain>.attribute.<name>`
  (`entity_metrics_attributes`, on by default) and text states as
  `homeassistant.entity.state` (1, with the state in the `homeassistant.entity.state` attribute)
- Automation traces: `homeassistant.step.variables.*`, `error.type=_OTHER` on failed runs
  and steps, and a span link from a step to the trace of the script or automation it started
- Go constants and `docs/attributes.md` generated from the Weaver registry
  (`scripts/generate.sh`), checked in CI

## 0.16.1

### Fixed

- Core logs carried no trace or span id: current Home Assistant stores traces in
  `TraceBuckets` (`.runs`), which the integration did not read. Records written in a run
  also get `homeassistant.step.path`

## 0.16.0

### Added

- `otlp_protocol` (`http` or `grpc`), `otlp_ca_file` (a CA certificate in the Home
  Assistant `ssl` folder) and `otlp_tls_insecure_skip_verify` options
- Core logs (`core_logs`) are linked to the exact step span of the automation or script
  run that wrote them, not only to the run's root span

## 0.15.0

### Added

- `health_sensors` option (on by default): `sensor.otel_collector_exported`, `_dropped`,
  `_queue` and `_status` in Home Assistant, from the collector's own metrics
  (`127.0.0.1:18888`). The status is `error` while the queue grows and nothing is sent

## 0.14.0

### Changed

- **Breaking, resources follow the semantic conventions:** `service.namespace` is the
  `service_name` option and each component is its own `service.name`. Home Assistant Core
  keeps `service_name` (`homeassistant`); the Supervisor is `supervisor`, an add-on its
  slug, the host (kernel, systemd, host metrics) `haos`. `service.version` is the Core
  version on Core resources and the OS version on the host. Backends that key resources on
  the service (for example Dash0) no longer merge attributes across containers. Queries
  that used `service.name=homeassistant` for host metrics or add-on and Supervisor logs
  should use `service.namespace=homeassistant`

## 0.13.0

### Changed

- `container.name` is a resource attribute, as the semantic conventions define it: one
  resource per container for journal logs and container metrics, and
  `container.name=homeassistant` on Core logs, event logs, entity metrics and automation
  traces. Backends no longer mix it across records that share a resource

## 0.12.1

### Fixed

- `ha_metrics` stopped scraping in 0.8.0: its pipeline had the same name as the entity and
  container metrics pipeline, so the last fragment replaced it. Pipelines now have unique
  names and CI fails on a collision
- `ha_metrics`: the scrape target (`server.address`, `server.port`, `url.scheme`,
  `service.instance.id`) is no longer put on the resource

## 0.12.0

### Added

- Entity metrics: `homeassistant.core.up` (1 when Core answered the scrape, 0 when not)
  and `homeassistant.entity.last_updated` (Unix seconds), the replacements for
  Prometheus' `up` and `homeassistant_last_updated_time_seconds`

## 0.11.1

### Fixed

- Bundled integrations: the add-on could report "set up" before Home Assistant was
  restarted and never create the config entry
- Logs: no "entry does not contain the combine_field" errors for journal entries without a
  message

## 0.11.0

### Added

- `core_logs` option (off by default): installs the bundled OTel Logs integration, which
  sends Home Assistant's log records structured (`code.*`, `exception.*`, logger as scope)
  with the trace and root span ids of the automation or script run that wrote them. The
  journal copy of Core's lines is dropped while it is on

## 0.10.1

### Changed

- Event logs follow the OpenTelemetry events conventions: event name
  `homeassistant.<event_type>` (was the bare event type), no body

## 0.10.0

### Added

- `event_logs` option (on by default): Home Assistant bus events as log records, with the
  trace id of the context that fired them, so an automation's `call_service` events sit in
  its trace. `event_logs_exclude` (default `[state_changed]`) leaves event types out

## 0.9.0

### Added

- `entity_metrics` option (on by default): every entity state as a gauge
  (`homeassistant.sensor.<device_class>`, `homeassistant.<domain>.value`,
  `homeassistant.<domain>.on`, `homeassistant.entity.available`) with entity, device, area
  and floor attributes, over the WebSocket API. No Prometheus integration needed.
  `entity_metrics_exclude` leaves entities out

### Changed

- `ha_metrics` (Prometheus scrape) is off by default for new installs; existing
  configurations keep their value

## 0.8.1

### Fixed

- Host metrics: disk usage of `/data` was never sent (the scraper skipped bind mounts)

## 0.8.0

### Added

- `container_metrics` option (on by default): CPU, memory, disk and network of Home
  Assistant Core, the Supervisor and every started add-on (`container.*`, labelled with
  `container.name`). The add-on now asks for the Supervisor `manager` role to read them
- `logs_multiline` option (on by default) to turn off joining Python tracebacks

### Fixed

- Logs: no more "entry does not contain the source_identifier" warnings for host entries

## 0.7.0

### Added

- Logs: a Python traceback from a container arrives as one record instead of one per line
- Resource attributes `service.version` (Home Assistant Core), `os.type`, `os.description`
  and `host.arch`
- Host metrics: disk usage of the data partition (`system.filesystem.*`, mount point `/data`)
- Automation traces: `homeassistant.last_step` on the run, `homeassistant.step.result.*` on
  steps

## 0.6.1

### Changed

- Docs: README, DOCS.md, option descriptions and receiver README rewritten for
  Home Assistant users new to OpenTelemetry; configuration reference, attribute
  reference and troubleshooting added

- Host metrics: removed the filesystem scraper, which reported nothing inside the add-on
  container

### Fixed

- Automation traces: runs still in Home Assistant's trace store were exported again every
  24 hours (automations that run less than once a day)
- Docs: the local OTLP endpoint (`profiles`) forwards profiles only, not traces or logs

## 0.6.0

- **Breaking, attribute names follow OpenTelemetry semantic conventions:**
  `haos.container` → `container.name`, `homeassistant.context.user_id` → `user.id`,
  `code.namespace` (deprecated) → `python.logger.name`. Update queries and dashboards
- Attributes without a semantic convention are defined in a Weaver registry (`model/`),
  checked in CI
- Step spans without an alias are named by their type: `action 0: choose`, `action 1: if`

## 0.5.1

- Automation traces: steps nest by path (an `if` condition sits under its `if`, under its
  action) and a step ends when the next step outside its subtree starts
- Step span names come from the config: `action 2: Ligar resistência` (alias),
  `action 3: switch.turn_on`, `conditions 0: state`, `entity_id 0: sensor.x`

## 0.5.0

- `profiles` now produces profiles of Home Assistant: the add-on installs a bundled
  OTel Profiler integration (wall-clock sampler of every Python thread, standard library
  only) and sets it up after a Home Assistant restart
- Profiles carry the add-on resource attributes (`service.name`, `host.name`...)
- The add-on maps the Home Assistant configuration folder (read-write) to install it

## 0.4.0

- Experimental `profiles` option: pprof (`127.0.0.1:4040/v1/pprof`) and OTLP (`127.0.0.1:4318`)
  intake forwarded as OTel Profiles (feature gate enabled automatically)

## 0.3.0

- Own collector distribution built with OCB (`otelcol-ha`): only the components the add-on
  uses, about a third of otelcol-contrib's size
- New `homeassistant` receiver and `ha_traces` option: automation and script runs as traces
- Cross-compiled per arch in CI (no QEMU), Go unit tests in CI

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
