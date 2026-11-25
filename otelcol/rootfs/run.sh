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

# Real host name of the HAOS machine (the container hostname is the add-on slug), plus the
# Home Assistant, OS and architecture versions for the resource attributes.
# The token goes through a header file, so it never shows up in the process list.
host_name=""; ha_version=""; os_version=""; os_description=""; arch=""
if [ -n "$SUPERVISOR_TOKEN" ]; then
  umask 077
  printf 'Authorization: Bearer %s\n' "$SUPERVISOR_TOKEN" > /tmp/supervisor.hdr
  sup() { curl -fsS --max-time 5 -H @/tmp/supervisor.hdr "http://supervisor/$1" 2>/dev/null | jq -r "$2 // empty" || true; }
  host_name="$(sup host/info .data.hostname)"
  os_description="$(sup host/info .data.operating_system)"
  ha_version="$(sup core/info .data.version)"
  os_version="$(sup os/info .data.version)"
  arch="$(sup info .data.arch)"
  rm -f /tmp/supervisor.hdr
else
  echo "[otelcol] WARNING: no SUPERVISOR_TOKEN; host.name falls back to 'homeassistant' and ha_metrics will get 401." >&2
fi
HOST_NAME="${host_name:-homeassistant}"
export HOST_NAME
# Supervisor architecture names -> semconv host.arch values.
case "$arch" in
  aarch64) arch=arm64 ;; amd64) arch=amd64 ;; armv7|armhf) arch=arm32 ;; i386) arch=x86 ;;
esac

# Generated fragment: resource attributes and exporter headers.
# Lists are replaced (not merged) across --config files, so the processors are built here.
# JSON is valid YAML. "$" is doubled because the collector expands ${...} in config values.
#
# Resources follow the semantic conventions: service.namespace (the service_name option)
# groups everything; each component is its own service.name (Home Assistant Core keeps the
# service_name option, the Supervisor is "supervisor", an add-on its slug, the host "haos").
#   resource       every signal: namespace, environment, host, OS, user attributes
#   resource/core  pipelines that only carry Home Assistant Core data
#   resource/haos  host metrics
# Pipelines that mix components (journal logs, entity + container metrics) name each
# resource from container.name in transform/service (base.yaml).
HA_VERSION="$ha_version"; OS_VERSION="$os_version"
export HA_VERSION OS_VERSION
jq --arg host "$HOST_NAME" --arg ver "$ha_version" --arg osver "$os_version" \
   --arg os "$os_description" --arg arch "$arch" '
  def esc: tostring | gsub("\\$"; "$$");
  def attr(k; v): {key: k, value: (v | esc), action: "upsert"};
  def opt(k; v): if v != "" then [attr(k; v)] else [] end;
  {
    processors: {
      resource: {attributes: (
        [ attr("service.namespace"; .service_name),
          attr("deployment.environment.name"; .deployment_environment),
          attr("host.name"; $host),
          attr("os.type"; "linux") ]
        + opt("os.description"; $os)
        + opt("host.arch"; $arch)
        + opt("service.instance.id"; .service_instance_id // "")
        + ((.resource_attributes // []) | map(attr(.name; .value)))
      )},
      "resource/core": {attributes: (
        [ attr("service.name"; .service_name), attr("container.name"; "homeassistant") ]
        + opt("service.version"; $ver)
      )},
      "resource/haos": {attributes: ([attr("service.name"; "haos")] + opt("service.version"; $osver))}
    },
    exporters: {otlp_http: {headers: ((.otlp_headers // []) | map({(.name): (.value | esc)}) | add // {})}}
  }' "$OPTS" > /tmp/generated.yaml

args="--config $CONF/base.yaml --config /tmp/generated.yaml"
features=""
if [ "$(opt .logs)" = "true" ]; then
  args="$args --config $CONF/logs.yaml"; features="$features logs"
  if [ "$(opt 'if .logs_multiline == null then true else .logs_multiline end')" = "true" ]; then
    args="$args --config $CONF/logs_multiline.yaml"; features="$features logs_multiline"
  fi
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
if [ "$(opt 'if .entity_metrics == null then true else .entity_metrics end')" = "true" ]; then
  jq '{receivers: {homeassistant: {exclude_entities: (.entity_metrics_exclude // [])}}}' "$OPTS" > /tmp/entities.yaml
  args="$args --config $CONF/entity_metrics.yaml --config /tmp/entities.yaml"; features="$features entity_metrics"
fi
if [ "$(opt 'if .event_logs == null then true else .event_logs end')" = "true" ]; then
  jq '{receivers: {homeassistant: {exclude_event_types: (.event_logs_exclude // ["state_changed"])}}}' "$OPTS" > /tmp/events.yaml
  args="$args --config $CONF/event_logs.yaml --config /tmp/events.yaml"; features="$features event_logs"
fi
if [ "$(opt 'if .container_metrics == null then true else .container_metrics end')" = "true" ]; then
  args="$args --config $CONF/container_metrics.yaml"; features="$features container_metrics"
fi

# Profiles producer: the add-on ships the otel_profiler integration and installs it into
# Home Assistant. Copying needs a Core restart (we notify); the config entry is created in
# the background as soon as Core has loaded the integration.
core_post() {  # core_post <api path> <json>; token via a header file, never on the command line
  hdr="$(mktemp)"
  printf 'Authorization: Bearer %s\n' "$SUPERVISOR_TOKEN" > "$hdr"
  curl -fsS --max-time 10 -X POST -H @"$hdr" -H 'Content-Type: application/json' \
    -d "$2" "http://supervisor/core/api/$1"
  rc=$?; rm -f "$hdr"; return $rc
}
# install_integration <domain> <name> <option>: copy the bundled integration into
# custom_components when it differs (Core restart needed, we notify), then create its config
# entry in the background as soon as Core has loaded it.
install_integration() {
  src="/usr/share/otelcol/$1"; dst="/homeassistant/custom_components/$1"
  if ! diff -rq -x __pycache__ "$src" "$dst" >/dev/null 2>&1; then
    mkdir -p "$(dirname "$dst")"
    rm -rf "$dst" && cp -r "$src" "$dst"
    echo "[otelcol] installed the $2 integration: restart Home Assistant to load it."
    core_post services/persistent_notification/create "{\"notification_id\": \"otelcol_$1\",
      \"title\": \"OTel Collector add-on\",
      \"message\": \"The $2 integration was installed or updated ($3). Restart Home Assistant to load it.\"}" \
      >/dev/null 2>&1 || true
  fi
  # create_entry on the first success, abort "single_instance_allowed" afterwards. Until Core
  # has loaded the integration the flow fails or aborts for another reason; retry.
  ( until core_post config/config_entries/flow "{\"handler\": \"$1\"}" 2>/dev/null \
        | jq -e '.type == "create_entry" or (.type == "abort" and
                 (.reason == "single_instance_allowed" or .reason == "already_configured"))' \
        >/dev/null 2>&1; do sleep 30; done
    echo "[otelcol] $2 integration is set up." ) &
}
# remove_hint <domain> <name> <option>: the option is off but the integration is installed.
remove_hint() {
  if [ -d "/homeassistant/custom_components/$1" ]; then
    echo "[otelcol] $3 is off: remove the $2 integration from Home Assistant to stop it."
  fi
}

gates=""
if [ "$(opt '.profiles // false')" = "true" ]; then
  args="$args --config $CONF/profiles.yaml"; features="$features profiles(experimental)"
  gates="--feature-gates=service.profilesSupport"
  install_integration otel_profiler "OTel Profiler" profiles
else
  remove_hint otel_profiler "OTel Profiler" profiles
fi

# Structured Core logs: the bundled otel_logs integration posts Home Assistant's log records
# (with the trace context of the run that wrote them) to the local OTLP receiver; the
# journal copy of the homeassistant container is then dropped (CORE_LOGS_CONTAINER).
CORE_LOGS_CONTAINER="-"
if [ "$(opt '.core_logs // false')" = "true" ]; then
  args="$args --config $CONF/core_logs.yaml"; features="$features core_logs"
  CORE_LOGS_CONTAINER="homeassistant"
  install_integration otel_logs "OTel Logs" core_logs
else
  remove_hint otel_logs "OTel Logs" core_logs
fi
export CORE_LOGS_CONTAINER

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
