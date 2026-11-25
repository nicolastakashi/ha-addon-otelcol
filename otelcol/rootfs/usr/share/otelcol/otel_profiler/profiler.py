"""Wall-clock sampler of every Python thread, pushed as pprof. Standard library only.

sys._current_frames() at a fixed rate, stacks aggregated per thread, encoded as
profile.proto by hand and POSTed gzipped every interval. It samples wall time: a thread
waiting (e.g. the event loop in select) shows up too, with its waiting frame on top.
"""

from __future__ import annotations

import gzip
import logging
import sys
import threading
import time
import urllib.request

_LOGGER = logging.getLogger(__package__)

Frame = tuple[str, str, int]  # function, file, line


def _varint(n: int) -> bytes:
    out = bytearray()
    while True:
        b, n = n & 0x7F, n >> 7
        if n:
            out.append(b | 0x80)
        else:
            out.append(b)
            return bytes(out)


def _field(num: int, value: int | bytes) -> bytes:
    if isinstance(value, int):
        return _varint(num << 3) + _varint(value)
    return _varint(num << 3 | 2) + _varint(len(value)) + value


def _packed(num: int, values: list[int]) -> bytes:
    return _field(num, b"".join(_varint(v) for v in values))


def encode(
    counts: dict[tuple[str, tuple[Frame, ...]], int],
    start_ns: int,
    duration_ns: int,
    period_ns: int,
) -> bytes:
    """profile.proto with samples/count and wall/nanoseconds; stacks are leaf first."""
    strings: dict[str, int] = {"": 0}

    def s(x: str) -> int:
        return strings.setdefault(x, len(strings))

    functions: dict[tuple[str, str], int] = {}
    locations: dict[tuple[int, int], int] = {}
    out = [
        _field(1, _field(1, s("samples")) + _field(2, s("count"))),
        _field(1, _field(1, s("wall")) + _field(2, s("nanoseconds"))),
    ]
    for (thread, stack), n in counts.items():
        loc_ids = []
        for name, filename, line in stack:
            fid = functions.setdefault((name, filename), len(functions) + 1)
            loc_ids.append(locations.setdefault((fid, line), len(locations) + 1))
        label = _field(1, s("thread.name")) + _field(2, s(thread))
        out.append(
            _field(2, _packed(1, loc_ids) + _packed(2, [n, n * period_ns]) + _field(3, label))
        )
    for (fid, line), lid in locations.items():
        out.append(_field(4, _field(1, lid) + _field(4, _field(1, fid) + _field(2, line))))
    for (name, filename), fid in functions.items():
        out.append(
            _field(5, _field(1, fid) + _field(2, s(name)) + _field(3, s(name)) + _field(4, s(filename)))
        )
    out.append(_field(9, start_ns))
    out.append(_field(10, duration_ns))
    out.append(_field(11, _field(1, s("wall")) + _field(2, s("nanoseconds"))))
    out.append(_field(12, period_ns))
    out += [_field(6, x.encode()) for x in strings]  # insertion order == index
    return b"".join(out)


def sample(skip: int) -> list[tuple[str, tuple[Frame, ...]]]:
    """One stack per thread, except the thread `skip`."""
    names = {t.ident: t.name for t in threading.enumerate()}
    stacks = []
    for tid, frame in sys._current_frames().items():
        if tid == skip:
            continue
        stack = []
        while frame is not None:
            code = frame.f_code
            stack.append((code.co_qualname, code.co_filename, frame.f_lineno or 0))
            frame = frame.f_back
        stacks.append((names.get(tid, str(tid)), tuple(stack)))
    return stacks


class Sampler(threading.Thread):
    def __init__(self, endpoint: str, hz: int = 19, interval: float = 10.0) -> None:
        super().__init__(name="otel_profiler", daemon=True)
        self.endpoint, self.period, self.interval = endpoint, 1 / hz, interval
        self.stopped = threading.Event()
        self._warned = False

    def stop(self) -> None:
        self.stopped.set()

    def run(self) -> None:
        me, period_ns = threading.get_ident(), int(self.period * 1e9)
        # Built here, not in __init__: it loads CA certificates (blocking in the event loop).
        # No ProxyHandler entries: never route 127.0.0.1 through an HTTP(S)_PROXY.
        self._opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        counts: dict = {}
        start, next_push = time.time_ns(), time.monotonic() + self.interval
        while not self.stopped.wait(self.period):
            for key in sample(me):
                counts[key] = counts.get(key, 0) + 1
            if time.monotonic() >= next_push:
                now = time.time_ns()
                self._push(encode(counts, start, now - start, period_ns))
                counts, start, next_push = {}, now, time.monotonic() + self.interval

    def _push(self, data: bytes) -> None:
        req = urllib.request.Request(
            self.endpoint,
            gzip.compress(data),
            {"Content-Type": "application/octet-stream", "Content-Encoding": "gzip"},
        )
        try:
            self._opener.open(req, timeout=5).close()
            self._warned = False
        except OSError as err:  # add-on stopped or profiles off: keep sampling, drop this batch
            if not self._warned:
                _LOGGER.warning("Cannot push profiles to %s: %s", self.endpoint, err)
                self._warned = True
