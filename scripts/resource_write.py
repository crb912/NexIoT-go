#!/usr/bin/env python3
"""
EdgeX Device Service - Write Command  (triggers HandleWriteCommands)
Targets the System-Time deviceCommand: year / month / day / hour / minute / second

Usage:
    # Write current system time to device
    python resource_write.py

    # Write a specific time
    python resource_write.py --year 2024 --month 6 --day 15 --hour 10 --minute 30 --second 0

    # Write only specific fields (single DeviceResource)
    python resource_write.py --resource day --value 20
    python resource_write.py --resource hour --value 9

    # Target a different host/device
    python resource_write.py --host 192.168.1.10 --port 59882 --device my-device
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

# Maps to the "System-Time" deviceCommand resourceOperations
SYSTEM_TIME_COMMAND = "System-Time"
SYSTEM_TIME_FIELDS = ["year", "month", "day", "hour", "minute", "second"]


def write_command(base_url: str, device: str, command: str, payload: dict) -> bool:
    """PUT to a deviceCommand endpoint (triggers HandleWriteCommands with multiple reqs)."""
    url = f"{base_url}/api/v2/device/name/{device}/{command}"
    print(f"\n→ PUT {url}")
    print(f"  Body: {json.dumps(payload, indent=4)}")

    try:
        resp = requests.put(url, json=payload, timeout=10)
    except requests.exceptions.ConnectionError:
        print(f"  ✗ Connection refused — is the device service running at {base_url}?")
        return False

    print(f"  Status: {resp.status_code}")

    if resp.status_code in (200, 201, 204):
        print(f"  ✓ Write successful")
        if resp.text:
            try:
                print(f"  Response: {json.dumps(resp.json(), indent=4)}")
            except Exception:
                print(f"  Response: {resp.text}")
        return True
    else:
        print(f"  ✗ Write failed: {resp.text}")
        return False


def write_resource(base_url: str, device: str, resource: str, value: str) -> bool:
    """PUT to a single DeviceResource endpoint."""
    url = f"{base_url}/api/v2/device/name/{device}/{resource}"
    payload = {resource: value}
    print(f"\n→ PUT {url}  (single resource)")
    print(f"  Body: {json.dumps(payload, indent=4)}")

    try:
        resp = requests.put(url, json=payload, timeout=10)
    except requests.exceptions.ConnectionError:
        print(f"  ✗ Connection refused — is the device service running at {base_url}?")
        return False

    print(f"  Status: {resp.status_code}")

    if resp.status_code in (200, 201, 204):
        print(f"  ✓ Write successful")
        return True
    else:
        print(f"  ✗ Write failed: {resp.text}")
        return False


def build_system_time_payload(args) -> dict:
    """Build payload from CLI args, falling back to current system time."""
    now = datetime.now()

    # Uint16 values must be passed as strings per EdgeX v2 API
    return {
        "year":   str(args.year   if args.year   is not None else now.year),
        "month":  str(args.month  if args.month  is not None else now.month),
        "day":    str(args.day    if args.day    is not None else now.day),
        "hour":   str(args.hour   if args.hour   is not None else now.hour),
        "minute": str(args.minute if args.minute is not None else now.minute),
        "second": str(args.second if args.second is not None else now.second),
    }

def build_system_time_payload_int(args) -> dict:
    """Build payload from CLI args, falling back to current system time."""
    now = datetime.now()

    # Use native integer numeric type instead of string
    return {
        "year": args.year if args.year is not None else now.year,
        "month": args.month if args.month is not None else now.month,
        "day": args.day if args.day is not None else now.day,
        "hour": args.hour if args.hour is not None else now.hour,
        "minute": args.minute if args.minute is not None else now.minute,
        "second": args.second if args.second is not None else now.second,
    }

def validate_ranges(payload: dict) -> list[str]:
    """Return list of validation errors."""
    # mirrors the profile constraints
    limits = {
        "year":   (2000, 2099),
        "month":  (1, 12),
        "day":    (1, 31),
        "hour":   (0, 23),
        "minute": (0, 59),
        "second": (0, 59),
    }
    errors = []
    for field, (lo, hi) in limits.items():
        if field in payload:
            try:
                v = int(payload[field])
                if not (lo <= v <= hi):
                    errors.append(f"{field}={v} out of range [{lo}, {hi}]")
            except ValueError:
                errors.append(f"{field}={payload[field]!r} is not an integer")
    return errors


def main():
    parser = argparse.ArgumentParser(description="EdgeX on-demand write command")
    parser.add_argument("--host", default=DEFAULT_HOST)
    parser.add_argument("--port", type=int, default=DEFAULT_PORT)
    parser.add_argument("--device", default=DEFAULT_DEVICE)

    # Single-resource mode
    parser.add_argument("--resource", help="Write a single DeviceResource by name (e.g. 'day')")
    parser.add_argument("--value",    help="Value for --resource (required when --resource is set)")

    # System-Time command fields (all optional; defaults to current time)
    time_group = parser.add_argument_group("System-Time fields (default: current system time)")
    time_group.add_argument("--year",   type=int)
    time_group.add_argument("--month",  type=int)
    time_group.add_argument("--day",    type=int)
    time_group.add_argument("--hour",   type=int)
    time_group.add_argument("--minute", type=int)
    time_group.add_argument("--second", type=int)

    args = parser.parse_args()

    base_url = f"http://{args.host}:{args.port}"
    print(f"EdgeX Write — device: {args.device}  service: {base_url}")
    print("=" * 60)

    # ── Single resource mode ──────────────────────────────────────
    if args.resource:
        if args.value is None:
            print("Error: --value is required when --resource is specified")
            sys.exit(1)
        success = write_resource(base_url, args.device, args.resource, args.value)
        sys.exit(0 if success else 1)

    # ── System-Time deviceCommand mode ───────────────────────────
    payload = build_system_time_payload(args)

    # Validate before sending
    errors = validate_ranges(payload)
    if errors:
        print("Validation errors:")
        for e in errors:
            print(f"  ✗ {e}")
        sys.exit(1)

    now_used = datetime.now()
    print(f"Writing System-Time → {payload['year']}-{payload['month']:>02}-{payload['day']:>02}"
          f"  {payload['hour']:>02}:{payload['minute']:>02}:{payload['second']:>02}")

    success = write_command(base_url, args.device, SYSTEM_TIME_COMMAND, payload)
    print("\nDone." if success else "\nFailed.")
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()