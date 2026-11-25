"""Self-check for the otel_profiler sampler/encoder (no Home Assistant needed).

    python3 -I otelcol/tests/profiler_check.py /tmp/profile.pb.gz
    go tool pprof -traces /tmp/profile.pb.gz | grep busy_marker
"""

import gzip
import importlib.util
import pathlib
import sys
import threading
import time

src = pathlib.Path(__file__).parent.parent / "rootfs/usr/share/otelcol/otel_profiler/profiler.py"
spec = importlib.util.spec_from_file_location("profiler", src)
profiler = importlib.util.module_from_spec(spec)
spec.loader.exec_module(profiler)

assert profiler._varint(300) == b"\xac\x02"

stop = threading.Event()


def busy_marker():
    while not stop.is_set():
        sum(range(1000))


threading.Thread(target=busy_marker, name="busy", daemon=True).start()
counts: dict = {}
start = time.time_ns()
for _ in range(20):
    for key in profiler.sample(threading.get_ident()):
        counts[key] = counts.get(key, 0) + 1
    time.sleep(0.01)
stop.set()

assert any(t == "busy" and s[0][0] == "busy_marker" for t, s in counts), counts.keys()
data = profiler.encode(counts, start, time.time_ns() - start, 10_000_000)
pathlib.Path(sys.argv[1]).write_bytes(gzip.compress(data))
print(f"ok: {len(counts)} stacks, {len(data)} bytes")
