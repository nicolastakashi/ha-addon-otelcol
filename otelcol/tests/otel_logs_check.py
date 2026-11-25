"""Self-check of the bundled otel_logs integration (stdlib only, no Home Assistant).

Trace and span ids must match what the homeassistant receiver (Go) gives a run, so a log
lands under the run's root span; expected values come from traceIDFor/spanIDFor.
"""

import importlib.util
import logging
import pathlib
import sys

path = pathlib.Path(__file__).parent.parent / "rootfs/usr/share/otelcol/otel_logs/exporter.py"
spec = importlib.util.spec_from_file_location("otel_logs_exporter", path)
exp = importlib.util.module_from_spec(spec)
spec.loader.exec_module(exp)

assert exp.ulid_hex("01KAV3RQ5Y2G3H4J5K6M7N8P9Q") == "019ab63c5cbe14071248b3350f545937"
assert exp.ulid_hex("not-a-ulid") is None
assert exp.root_span_hex("r2") == "b776e93913114424"
import hashlib
assert hashlib.sha256(b"r2").hexdigest()[:32] == "db77fd01af957221a4989b64b3770a83"  # Go fallback

import datetime
utc = datetime.timezone.utc
for ts, want in [
    (datetime.datetime(2026, 11, 24, 21, 5, 0, 150000, tzinfo=utc), "c746aac95ddaaed2"),
    (datetime.datetime(2026, 11, 24, 21, 5, 0, 0, tzinfo=utc), "42889a80d3434706"),
    (datetime.datetime(2026, 11, 24, 21, 5, 0, 123456, tzinfo=utc), "5e1eaeb9ef27a426"),
]:
    got = exp.step_span_hex("r2", "action/0", ts)
    assert got == want, (ts, got, want)  # spanIDFor("r2", "action/0#"+RFC3339Nano) in Go

try:
    raise ValueError("boom")
except ValueError:
    rec = logging.getLogger("homeassistant.components.x").makeRecord(
        "homeassistant.components.x", logging.ERROR, "/x.py", 7, "failed %s", ("job",), sys.exc_info())
out = exp.to_otlp(rec, ("019ab63c5cbe14071248b3350f545937", "b776e93913114424"))
attrs = {a["key"]: a["value"] for a in out["attributes"]}
assert out["severityNumber"] == 17 and out["body"]["stringValue"] == "failed job"
assert out["traceId"] == "019ab63c5cbe14071248b3350f545937" and out["spanId"] == "b776e93913114424"
assert attrs["exception.type"]["stringValue"] == "builtins.ValueError"
assert "Traceback" in attrs["exception.stacktrace"]["stringValue"]
assert attrs["code.line.number"]["intValue"] == "7"
print("otel_logs ok")
