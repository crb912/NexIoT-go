#!/usr/bin/env python3
"""
EdgeX Device Service - Read Command
Usage:
    python resource_read.py                                    # read all default commands
    python resource_read.py --command Battery-Config           # read specific command
    python resource_read.py --command System-Time
    python resource_read.py --command day                      # read single resource
    python resource_read.py --host 192.168.1.10 --port 59882
"""

import argparse
import json
import sys
from datetime import datetime

try:
    import requests
except ImportError:
    print("Missing dependency: pip install requests")
    sys.exit(1)

DEFAULT_HOST = "localhost"
DEFAULT_PORT = 59882
DEFAULT_DEVICE = "Modbus-TCP-RTU-test-device"
DEFAULT_COMMANDS = ["Battery-Config", "System-Time"]


def read_command(base_url: str, device: str, command: str) -> dict | None:
    url = f"{base_url}/api/v2/device/name/{device}/{command}"
    print(f"\n→ GET {url}")
    try:
        resp = requests.get(url, timeout=10)
    except requests.exceptions.ConnectionError:
        print(f"  ✗ Connection refused — is the device service running at {base_url}?")
        return None

    print(f"  Status: {resp.status_code}")

    if resp.status_code != 200:
        print(f"  ✗ Error response: {resp.text}")
        return None

    data = resp.json()

    # Pretty-print readings
    event = data.get("event", {})
    readings = event.get("readings", [])
    if readings:
        print(f"  ✓ {len(readings)} reading(s) from command '{command}':")
        col_w = max(len(r.get("resourceName", "")) for r in readings) + 2
        for r in readings:
            name = r.get("resourceName", "?")
            value = r.get("value", "?")
            vtype = r.get("valueType", "")
            ts = r.get("origin", 0)
            ts_str = datetime.fromtimestamp(ts / 1e9).strftime("%H:%M:%S") if ts else ""
            print(f"    {name:<{col_w}} = {value!s:<15}  ({vtype}) {ts_str}")
    else:
        print("  ✓ Response (no readings):")
        print(f"    {json.dumps(data, indent=4)}")

    return data


def main():
    parser = argparse.ArgumentParser(description="EdgeX on-demand read command")
    parser.add_argument("--host", default=DEFAULT_HOST)
    parser.add_argument("--port", type=int, default=DEFAULT_PORT)
    parser.add_argument("--device", default=DEFAULT_DEVICE)
    parser.add_argument(
        "--command",
        help="DeviceCommand or DeviceResource name to read (default: runs Battery-Config and System-Time)",
    )
    args = parser.parse_args()

    base_url = f"http://{args.host}:{args.port}"
    commands = [args.command] if args.command else DEFAULT_COMMANDS

    print(f"EdgeX Read — device: {args.device}  service: {base_url}")
    print("=" * 60)

    for cmd in commands:
        read_command(base_url, args.device, cmd)

    print("\nDone.")


if __name__ == "__main__":
    main()