# Collector self-metrics (Prometheus text) -> "exported dropped queue_percent queue_bytes".
/^otelcol_exporter_sent_/        { sent += $NF }
/^otelcol_exporter_send_failed_/ { failed += $NF }
/^otelcol_exporter_queue_(size|capacity)/ {
  match($0, /data_type="[^"]*"/); key = substr($0, RSTART, RLENGTH)
  if ($0 ~ /^otelcol_exporter_queue_size/) size[key] = $NF; else cap[key] = $NF
}
END {
  q = 0; total = 0
  for (k in size) { total += size[k]; if (cap[k] > 0 && size[k] / cap[k] > q) q = size[k] / cap[k] }
  printf "%.0f %.0f %.1f %.0f\n", sent, failed, q * 100, total
}
