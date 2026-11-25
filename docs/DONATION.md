# Donating the Home Assistant receiver to opentelemetry-collector-contrib

Process: [new-components.md](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/docs/new-components.md).
Contrib accepts donations of components that already have real usage outside contrib.
A sponsor (approver or maintainer from a different company) and at least three code owners
(one of them an approver/maintainer) are required.

## Checklist

- [x] Component lives outside contrib with a public repo and CI (`otelcol/collector/receiver/homeassistantreceiver`)
- [x] README with configuration, mapping and limitations
- [x] `metadata.yaml` (mdatagen style)
- [x] Unit tests with a fake Home Assistant WebSocket server (≈80% coverage)
- [ ] Some months of real usage and feedback (issues, users of the add-on)
- [ ] Listed in the [OpenTelemetry registry](https://opentelemetry.io/ecosystem/registry/adding/)
- [ ] Two more code owners (Home Assistant + OTel users)
- [ ] Sponsor found (#otel-collector-dev on CNCF Slack, Collector SIG meeting)
- [ ] Generated code via `mdatagen` (config docs, telemetry), `go.opentelemetry.io/collector` APIs only
- [ ] Follow contrib conventions: `internal/metadata`, `generated_component_test.go`, `testdata/config.yaml`

## Draft "Component donation" issue

> **The purpose and use-cases of the new component**
>
> Home Assistant is an open-source home automation platform. Its automations and
> scripts record a run trace (trigger, conditions, actions, timings, errors) in an
> internal trace store. This receiver turns each finished run into an OpenTelemetry trace,
> so users can see the latency and errors of their automations in any OTLP backend.
>
> **Example configuration for the component**
>
> ```yaml
> receivers:
>   homeassistant:
>     endpoint: ws://homeassistant.local:8123/api/websocket
>     token: ${env:HA_TOKEN}
>     poll_interval: 10s
>     domains: [automation, script]
> ```
>
> **Telemetry data types supported**: traces
>
> **Code owner(s)**: @nicolastakashi (+ two more)
>
> **Sponsor (optional)**: _to be found_
>
> **Additional context**: running in production as part of a community Home Assistant add-on
> (<https://github.com/nicolastakashi/ha-addon-otelcol>); prior art covers only in-process
> HACS integrations that emit zero-duration spans.

## OTel registry entry (draft)

`data/registry/collector-receiver-homeassistant.yml` in opentelemetry.io:

```yaml
title: Home Assistant Receiver
registryType: receiver
language: collector
tags: [go, receiver, collector, homeassistant, traces]
license: Apache 2.0
description: Turns Home Assistant automation and script runs into traces.
authors:
  - name: Nicolas Takashi
    url: https://github.com/nicolastakashi
urls:
  repo: https://github.com/nicolastakashi/ha-addon-otelcol/tree/main/otelcol/collector/receiver/homeassistantreceiver
createdAt: 2026-11-24
isFirstParty: false
```
