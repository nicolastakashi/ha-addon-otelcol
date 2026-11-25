"""A logging handler that batches records and posts them as OTLP/HTTP JSON (stdlib only).

emit() only appends to a bounded queue; a daemon thread posts every 2 seconds, so the
event loop never waits on the network.
"""

import collections
import hashlib
import json
import logging
import threading
import time
import traceback
import urllib.request

_LOGGER = logging.getLogger(__package__)
_CROCKFORD = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
_SEVERITY = {logging.DEBUG: 5, logging.INFO: 9, logging.WARNING: 13, logging.ERROR: 17, logging.CRITICAL: 21}


def ulid_hex(ulid: str) -> str | None:
    """A ULID (Home Assistant context id) as the 32-hex-digit trace id the add-on uses."""
    if len(ulid) != 26:
        return None
    n = 0
    for c in ulid.upper():
        i = _CROCKFORD.find(c)
        if i < 0:
            return None
        n = n * 32 + i
    return f"{n:032x}" if n < 1 << 128 else None


def root_span_hex(run_id: str) -> str:
    """The span id the add-on gives a run's root span: sha256("<run_id>/root")[:8]."""
    return hashlib.sha256(f"{run_id}/root".encode()).hexdigest()[:16]


def trace_context(hass) -> tuple[str, str] | None:
    """(trace id, root span id) of the automation/script run this code runs in, if any."""
    try:
        from homeassistant.helpers.trace import trace_id_get

        key_run = trace_id_get()
        if not key_run:
            return None
        key, run_id = key_run
        run = hass.data.get("trace", {}).get(key, {}).get(run_id)
        context = getattr(run, "context", None)
        trace_id = ulid_hex(getattr(context, "id", "") or "")
        if trace_id is None:  # same fallback as the add-on: a hash of the run id
            trace_id = hashlib.sha256(run_id.encode()).hexdigest()[:32]
        return trace_id, root_span_hex(run_id)
    except Exception:  # noqa: BLE001 - private API; logging must never break
        return None


def _attr(key: str, value) -> dict:
    if isinstance(value, bool):
        v = {"boolValue": value}
    elif isinstance(value, int):
        v = {"intValue": str(value)}
    else:
        v = {"stringValue": str(value)}
    return {"key": key, "value": v}


def to_otlp(record: logging.LogRecord, ctx: tuple[str, str] | None) -> dict:
    attrs = [
        _attr("code.file.path", record.pathname),
        _attr("code.line.number", record.lineno),
        _attr("code.function.name", record.funcName),
        _attr("thread.name", record.threadName),
        _attr("thread.id", record.thread),
    ]
    if record.exc_info and record.exc_info[0] is not None:
        etype, evalue, tb = record.exc_info
        attrs += [
            _attr("exception.type", f"{etype.__module__}.{etype.__qualname__}"),
            _attr("exception.message", str(evalue)),
            _attr("exception.stacktrace", "".join(traceback.format_exception(etype, evalue, tb))),
        ]
    out = {
        "timeUnixNano": str(int(record.created * 1e9)),
        "observedTimeUnixNano": str(time.time_ns()),
        "severityNumber": _SEVERITY.get(record.levelno, 9),
        "severityText": record.levelname,
        "body": {"stringValue": record.getMessage()},
        "attributes": attrs,
    }
    if ctx:
        out["traceId"], out["spanId"] = ctx
    return out


class OtlpLogHandler(logging.Handler):
    def __init__(self, hass, endpoint: str, interval: float = 2.0, maxlen: int = 10000) -> None:
        super().__init__()
        self.hass, self.endpoint, self.interval = hass, endpoint, interval
        self.queue: collections.deque = collections.deque(maxlen=maxlen)
        self.stopped = threading.Event()
        self.thread = threading.Thread(target=self._run, name="otel_logs", daemon=True)
        self._warned = False

    def emit(self, record: logging.LogRecord) -> None:
        if record.name.startswith(__package__):
            return  # our own warnings would loop
        try:
            self.queue.append((record.name, to_otlp(record, trace_context(self.hass))))
        except Exception:  # noqa: BLE001
            self.handleError(record)

    def start(self) -> None:
        self.thread.start()

    def stop(self) -> None:
        self.stopped.set()

    def _run(self) -> None:
        # Built here, not in __init__: it loads CA certificates (blocking in the event loop).
        # No ProxyHandler entries: never route 127.0.0.1 through an HTTP(S)_PROXY.
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        while not self.stopped.wait(self.interval):
            batch = []
            while self.queue and len(batch) < 1000:
                batch.append(self.queue.popleft())
            if batch:
                self._post(opener, batch)

    def _post(self, opener, batch: list) -> None:
        scopes: dict = {}
        for name, rec in batch:
            scopes.setdefault(name, []).append(rec)
        body = {"resourceLogs": [{"resource": {}, "scopeLogs": [
            {"scope": {"name": name}, "logRecords": recs} for name, recs in scopes.items()]}]}
        req = urllib.request.Request(
            self.endpoint, json.dumps(body).encode(), {"Content-Type": "application/json"})
        try:
            opener.open(req, timeout=5).close()
            self._warned = False
        except OSError as err:  # add-on stopped or core_logs off: drop this batch
            if not self._warned:
                _LOGGER.warning("Cannot send logs to %s: %s", self.endpoint, err)
                self._warned = True
