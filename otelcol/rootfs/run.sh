#!/bin/sh
# Builds the collector command line from the add-on options.
# Each feature is a config fragment; otelcol deep-merges every --config it gets
# (maps merge, lists are replaced).
set -eu

OPTS=/data/options.json
CONF=/etc/otelcol
opt() { jq -r "$1" "$OPTS"; }

endpoint="$(opt '.otlp_endpoint // ""')"
if [ -z "$endpoint" ]; then
  echo "[otelcol] otlp_endpoint is empty: set it in the add-on Configuration tab." >&2
  exit 1
fi

# Assign, then export: `export X="$(cmd)"` would hide a jq failure.
OTLP_ENDPOINT="$endpoint"
SERVICE_NAME="$(opt .service_name)"
DEPLOYMENT_ENVIRONMENT="$(opt .deployment_environment)"
SCRAPE_INTERVAL="$(opt .scrape_interval)"
LOGS_MIN_PRIORITY="$(opt .logs_min_priority)"
# OTel severity numbers: DEBUG 5, INFO 9, WARN 13, ERROR 17, FATAL 21.
case "$(opt '.logs_min_severity // "info"')" in
  debug) LOGS_MIN_SEVERITY_NUMBER=5 ;;
  warn)  LOGS_MIN_SEVERITY_NUMBER=13 ;;
  error) LOGS_MIN_SEVERITY_NUMBER=17 ;;
  fatal) LOGS_MIN_SEVERITY_NUMBER=21 ;;
  *)     LOGS_MIN_SEVERITY_NUMBER=9 ;;
esac
SUPERVISOR_TOKEN="${SUPERVISOR_TOKEN:-}"
export OTLP_ENDPOINT SERVICE_NAME DEPLOYMENT_ENVIRONMENT SCRAPE_INTERVAL \
       LOGS_MIN_PRIORITY LOGS_MIN_SEVERITY_NUMBER SUPERVISOR_TOKEN

mkdir -p /data/storage

# Real host name of the HAOS machine (the container hostname is the add-on slug).
# The token goes through a header file, so it never shows up in the process list.
host_name=""
if [ -n "$SUPERVISOR_TOKEN" ]; then
  umask 077
  printf 'Authorization: Bearer %s\n' "$SUPERVISOR_TOKEN" > /tmp/supervisor.hdr
  host_name="$(curl -fsS --max-time 5 -H @/tmp/supervisor.hdr http://supervisor/host/info 2>/dev/null \
    | jq -r '.data.hostname // empty' || true)"
  rm -f /tmp/supervisor.hdr
else
  echo "[otelcol] WARNING: no SUPERVISOR_TOKEN; host.name falls back to 'homeassistant' and ha_metrics will get 401." >&2
fi
HOST_NAME="${host_name:-homeassistant}"
export HOST_NAME

# Generated fragment: resource attributes and exporter headers.
# Lists are replaced (not merged) across --config files, so the whole
# resource processor is built here. JSON is valid YAML.
# "$" is doubled because the collector expands ${...} in config values.
jq --arg host "$HOST_NAME" '
  def esc: tostring | gsub("\\$"; "$$");
  def attr(k; v): {key: k, value: (v | esc), action: "upsert"};
  {
    processors: {resource: {attributes: (
      [ attr("service.name"; .service_name),
        attr("deployment.environment.name"; .deployment_environment),
        attr("host.name"; $host) ]
      + (if (.service_instance_id // "") != "" then [attr("service.instance.id"; .service_instance_id)] else [] end)
      + ((.resource_attributes // []) | map(attr(.name; .value)))
    )}},
    exporters: {otlp_http: {headers: ((.otlp_headers // []) | map({(.name): (.value | esc)}) | add // {})}}
  }' "$OPTS" > /tmp/generated.yaml

args="--config $CONF/base.yaml --config /tmp/generated.yaml"
features=""
if [ "$(opt .logs)" = "true" ]; then
  args="$args --config $CONF/logs.yaml"; features="$features logs"
  if [ "$(opt .logs_parse)" = "true" ]; then
    args="$args --config $CONF/logs_parse.yaml"; features="$features logs_parse"
  fi
fi
if [ "$(opt .ha_metrics)" = "true" ]; then
  args="$args --config $CONF/ha_metrics.yaml"; features="$features ha_metrics"
fi
if [ "$(opt 'if .ha_traces == null then true else .ha_traces end')" = "true" ]; then
  args="$args --config $CONF/ha_traces.yaml"; features="$features ha_traces"
fi
if [ "$(opt .host_metrics)" = "true" ]; then
  args="$args --config $CONF/host_metrics.yaml"; features="$features host_metrics"
fi

# Profiles producer: the add-on ships the otel_profiler integration and installs it into
# Home Assistant. Copying needs a Core restart (we notify); the config entry is created in
# the background as soon as Core has loaded the integration.
PROFILER_SRC=/usr/share/otelcol/otel_profiler
PROFILER_DST=/homeassistant/custom_components/otel_profiler
core_post() {  # core_post <api path> <json>; token via a header file, never on the command line
  hdr="$(mktemp)"
  printf 'Authorization: Bearer %s\n' "$SUPERVISOR_TOKEN" > "$hdr"
  curl -fsS --max-time 10 -X POST -H @"$hdr" -H 'Content-Type: application/json' \
    -d "$2" "http://supervisor/core/api/$1"
  rc=$?; rm -f "$hdr"; return $rc
}
setup_profiler() {
  if ! diff -rq -x __pycache__ "$PROFILER_SRC" "$PROFILER_DST" >/dev/null 2>&1; then
    mkdir -p "$(dirname "$PROFILER_DST")"
    rm -rf "$PROFILER_DST" && cp -r "$PROFILER_SRC" "$PROFILER_DST"
    echo "[otelcol] installed the OTel Profiler integration: restart Home Assistant to load it."
    core_post services/persistent_notification/create '{"notification_id": "otelcol_profiler",
      "title": "OTel Collector add-on",
      "message": "The OTel Profiler integration was installed or updated. Restart Home Assistant to start sending profiles."}' \
      >/dev/null 2>&1 || true
  fi
  # create_entry on the first success, abort ("already configured") afterwards; until Core
  # has loaded the integration the flow fails and we retry.
  ( until core_post config/config_entries/flow '{"handler": "otel_profiler"}' 2>/dev/null \
        | jq -e '.type == "create_entry" or .type == "abort"' >/dev/null 2>&1; do sleep 30; done
    echo "[otelcol] OTel Profiler integration is set up." ) &
}

gates=""
if [ "$(opt '.profiles // false')" = "true" ]; then
  args="$args --config $CONF/profiles.yaml"; features="$features profiles(experimental)"
  gates="--feature-gates=service.profilesSupport"
  setup_profiler
elif [ -d "$PROFILER_DST" ]; then
  echo "[otelcol] profiles is off: remove the OTel Profiler integration from Home Assistant to stop sampling."
fi

if [ -z "$features" ]; then
  echo "[otelcol] every feature is disabled, nothing to do." >&2
  exit 1
fi

# Never print credentials embedded in the URL (https://user:pass@host).
safe_endpoint="$(printf '%s' "$OTLP_ENDPOINT" | sed -E 's#(://)[^/@]*@#\1***@#')"
echo "[otelcol] $(otelcol-ha --version)"
echo "[otelcol] host.name=$HOST_NAME, exporting to $safe_endpoint:$features"
# shellcheck disable=SC2086
exec otelcol-ha $gates $args
