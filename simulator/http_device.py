#!/usr/bin/env python3
"""
HTTP Poller Simulator — simulates an HTTP REST API device.

Provides endpoints:
  POST /api/v1/login    — authenticate and receive a Bearer token
  POST /api/v1/logout   — invalidate the token
  GET  /api/v1/temperature — returns current temperature
  GET  /api/v1/humidity    — returns current humidity
  GET  /api/v1/status      — returns device status
  POST /api/v1/setpoint    — set target temperature setpoint

Usage:
  python3 simulator/http_device.py
  (listens on http://0.0.0.0:9000 by default)
"""

from pathlib import Path
from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import random
import threading
import time
import uuid

PORT = 9000
HOST = "0.0.0.0"

# In-memory device state
class DeviceState:
    def __init__(self):
        self.temperature = 25.0
        self.humidity = 60.0
        self.setpoint = 22.0
        self.status = "ok"
        self.tokens = {}  # token -> expiry timestamp

    def update(self):
        """Simulate sensor drift."""
        self.temperature += random.uniform(-0.5, 0.5)
        self.temperature = round(self.temperature, 1)
        self.humidity += random.uniform(-1.0, 1.0)
        self.humidity = round(max(0, min(100, self.humidity)), 1)

state = DeviceState()


class HTTPHandler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        """Log incoming requests."""
        print(f"[{self.address_string()}] {self.command} {self.path} → {format % args if args else format}")

    def _send_json(self, code, data):
        body = json.dumps(data).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _read_body(self):
        length = int(self.headers.get("Content-Length", 0))
        if length == 0:
            return {}
        return json.loads(self.rfile.read(length))

    def _check_auth(self):
        """Check Bearer token in Authorization header."""
        auth = self.headers.get("Authorization", "")
        if auth.startswith("Bearer "):
            token = auth[7:]
            if token in state.tokens:
                return True
        return False

    def do_GET(self):
        if not self._check_auth():
            self._send_json(401, {"error": "unauthorized"})
            return

        if self.path == "/api/v1/temperature":
            self._send_json(200, {"temperature": state.temperature})
        elif self.path == "/api/v1/humidity":
            self._send_json(200, {"humidity": state.humidity})
        elif self.path == "/api/v1/status":
            self._send_json(200, {"status": state.status})
        else:
            self._send_json(404, {"error": "not found"})

    def do_POST(self):
        if self.path == "/api/v1/login":
            body = self._read_body()
            username = body.get("username", "")
            password = body.get("password", "")
            if username == "admin" and password == "admin123":
                token = str(uuid.uuid4())
                state.tokens[token] = time.time() + 3600
                self._send_json(200, {"token": token})
            else:
                self._send_json(401, {"error": "invalid credentials"})
            return

        if self.path == "/api/v1/logout":
            auth = self.headers.get("Authorization", "")
            if auth.startswith("Bearer "):
                token = auth[7:]
                state.tokens.pop(token, None)
            self._send_json(200, {"message": "logged out"})
            return

        if not self._check_auth():
            self._send_json(401, {"error": "unauthorized"})
            return

        if self.path == "/api/v1/setpoint":
            body = self._read_body()
            if isinstance(body, dict):
                sp = body.get("setpoint", body)
                if isinstance(sp, (int, float)):
                    state.setpoint = float(sp)
                    self._send_json(200, {"setpoint": state.setpoint})
                    return
            self._send_json(400, {"error": "invalid setpoint value"})
        else:
            self._send_json(404, {"error": "not found"})


def update_loop():
    """Background thread to update simulated sensor values."""
    while True:
        time.sleep(5)
        state.update()


def main():
    server = HTTPServer((HOST, PORT), HTTPHandler)
    print(f"HTTP poller simulator running on http://{HOST}:{PORT}")

    # Start sensor update thread
    updater = threading.Thread(target=update_loop, daemon=True)
    updater.start()

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down...")
        server.shutdown()


if __name__ == "__main__":
    main()
