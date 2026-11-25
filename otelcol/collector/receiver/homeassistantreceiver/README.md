# Home Assistant Receiver

| Status | |
|---|---|
| Stability | development: traces |
| Distributions | [otelcol-ha](../../builder-config.yaml) (Home Assistant add-on) |
| Code owners | @nicolastakashi |

Turns [Home Assistant](https://www.home-assistant.io/) automation and script runs into
traces. It reads Home Assistant's trace store, the same data shown in
Settings → Automations → Traces, over the
[WebSocket API](https://developers.home-assistant.io/docs/api/websocket/)
(`trace/list`, `trace/get`).

Each finished run becomes one trace:

| Span | Name | Timing | Attributes |
|---|---|---|---|
| Root | `automation <alias>` / `script <alias>` | run start → finish | `homeassistant.domain`, `.item_id`, `.run_id`, `.alias`, `.trigger`, `.script_execution`, `.context.id`, `.context.parent_id`, `user.id` |
| Step (nested by path) | last path part, index and what the config says, e.g. `action 2: Ligar resistência`, `conditions 0: state` | step start → start of the next step outside its subtree | `homeassistant.step.path`, `homeassistant.step.child` (called script run) |

- The trace id is the run's Home Assistant context id (a ULID, 128 bits), so it can be
  correlated with Home Assistant logs; it falls back to a hash of the run id.
- A run or step with an error gets `STATUS_CODE_ERROR` and the error message.
- Runs already stored when the receiver starts are not replayed.
- `user.id` follows OpenTelemetry semantic conventions. The `homeassistant.*` attributes are
  defined in a [Weaver](https://github.com/open-telemetry/weaver) registry, in
  [`model/homeassistant.yaml`](../../../../model/homeassistant.yaml).

## Configuration

```yaml
receivers:
  homeassistant:
    endpoint: ws://supervisor/core/websocket   # default; e.g. ws://homeassistant.local:8123/api/websocket
    token: ${env:HA_TOKEN}                     # admin token (SUPERVISOR_TOKEN in an add-on)
    poll_interval: 10s                         # default
    domains: [automation, script]              # default
```

| Setting | Default | Description |
|---|---|---|
| `endpoint` | `ws://supervisor/core/websocket` | Home Assistant WebSocket API URL. Required. |
| `token` | none | Access token of an admin user. Required. |
| `poll_interval` | `10s` | How often `trace/list` is polled. Must be positive. |
| `domains` | `[automation, script]` | Domains whose runs are collected. Must not be empty. |

On connection or polling errors the receiver reconnects with exponential backoff, from
1 s up to 1 min.

The trace store needs an admin token: in an add-on use `SUPERVISOR_TOKEN` with
`homeassistant_api: true`; elsewhere a long-lived access token of an admin user.

## Limitations

- Home Assistant keeps the last `stored_traces` runs per automation (default 5). More runs of
  one automation within a poll interval lose the oldest.
- Steps only have a start time; a step's end is inferred from the next step outside its
  subtree.
- A step is the child of the latest step recorded at its parent path
  (`action/1/if/condition/0` → `action/1/if` → `action/1`), else of the root span.
- The trace store format is not a stable Home Assistant API and may change between releases.
