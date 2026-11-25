#!/usr/bin/env python3
"""A fake OpenAI-compatible speech server for testing Stories (not a real voice).

POST /v1/audio/speech  -> a WAV of soft tones, about 55 ms per character of input
GET  /v1/audio/voices  -> {"voices": [...]}

Usage: scripts/fake-speech-server.py [port]   (binds to the loopback interface only)
"""
import json
import math
import struct
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

RATE = 16000
VOICES = ["fake_en", "fake_hi", "fake_ta"]


def wav(ms: int, seed: int) -> bytes:
    n = RATE * ms // 1000
    base = 180 + (seed % 7) * 25
    pcm = bytearray()
    for i in range(n):
        env = min(1.0, i / 400, (n - i) / 400)
        pulse = 0.5 + 0.5 * math.sin(i / RATE * 2 * math.pi * 3)
        v = int(4500 * env * pulse * math.sin(2 * math.pi * base * i / RATE))
        pcm += struct.pack("<h", v)
    hdr = (b"RIFF" + struct.pack("<I", 36 + len(pcm)) + b"WAVEfmt "
           + struct.pack("<IHHIIHH", 16, 1, 1, RATE, RATE * 2, 2, 16)
           + b"data" + struct.pack("<I", len(pcm)))
    return hdr + bytes(pcm)


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def _send(self, code, body, ctype):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path.endswith("/audio/voices"):
            self._send(200, json.dumps({"voices": VOICES}).encode(), "application/json")
        else:
            self._send(404, b"not found", "text/plain")

    def do_POST(self):
        if not self.path.endswith("/audio/speech"):
            return self._send(404, b"not found", "text/plain")
        body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))) or b"{}")
        text = body.get("input", "")
        speed = float(body.get("speed", 1) or 1)
        ms = max(300, int(len(text) * 55 / speed))
        self._send(200, wav(ms, len(body.get("voice", ""))), "audio/wav")


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8991
    HTTPServer(("0.0.0.0" if False else "127.0.0.1", port), H).serve_forever()
