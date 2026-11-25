#!/bin/sh
# Regenerates code and docs from the Weaver registry (model/), like the OpenTelemetry
# semantic-conventions repo does. CI fails if the committed output is out of date.
set -eu
cd "$(dirname "$0")/.."
WEAVER="${WEAVER:-weaver}"
"$WEAVER" registry generate -r model --future -t templates go otelcol/collector/receiver/homeassistantreceiver
"$WEAVER" registry generate -r model --future -t templates markdown docs
gofmt -w otelcol/collector/receiver/homeassistantreceiver/semconv_gen.go
