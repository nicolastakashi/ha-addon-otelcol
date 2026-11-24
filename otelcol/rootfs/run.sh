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

if [ -z "$features" ]; then
  echo "[otelcol] every feature is disabled, nothing to do." >&2
  exit 1
fi

# Never print credentials embedded in the URL (https://user:pass@host).
safe_endpoint="$(printf '%s' "$OTLP_ENDPOINT" | sed -E 's#(://)[^/@]*@#\1***@#')"
echo "[otelcol] $(otelcol-ha --version)"
echo "[otelcol] host.name=$HOST_NAME, exporting to $safe_endpoint:$features"
# shellcheck disable=SC2086
exec otelcol-ha $args
