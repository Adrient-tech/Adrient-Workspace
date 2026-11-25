#!/usr/bin/env python3
"""Times API calls against a scale-seeded scratch stack: probe.py PORT PATH [PATH...]"""
import sys, time, urllib.request
port, paths = sys.argv[1], sys.argv[2:]
for p in paths:
    best = None
    for _ in range(3):
        req = urllib.request.Request(f"http://127.0.0.1:{port}{p}", headers={"Cookie": "adrient_session=scaletesttoken0123456789", "Host": f"workspace.localtest.me:{port}"})
        t = time.time()
        try:
            body = urllib.request.urlopen(req, timeout=60).read()
            code = 200
        except Exception as e:
            body, code = str(e).encode(), getattr(e, "code", 0)
        ms = (time.time() - t) * 1000
        best = ms if best is None else min(best, ms)
    print(f"{best:8.1f} ms  {code}  {len(body):7d} B  {p}")
