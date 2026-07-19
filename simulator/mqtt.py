"""
MQTT Device Simulator — publishes periodic JSON telemetry directly to the
gateway's embedded MQTT broker.

Usage:
    python simulator/mqtt.py

Requires: paho-mqtt (pip install paho-mqtt)
The gateway's embedded broker must be running on localhost:1883.
"""

import json
import logging
import random
import time
from datetime import datetime
from pathlib import Path
from typing import Dict, Any

# ─── Configuration ─────────────────────────────────────────────────────────

BROKER = "localhost"
PORT   = 1883
TOPIC  = "device/test/data"
INTERVAL_SECONDS = 5

# ─── Logging ───────────────────────────────────────────────────────────────

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [MQTT-SIM] %(message)s",
)
log = logging.getLogger(__name__)


def generate_telemetry() -> Dict[str, Any]:
    """Generate a random telemetry payload matching the MQTT test profile."""
    return {
        "device_id": "mqtt-test-001",
        "device_name": "MQTT-Test-Device",
        "timestamp": datetime.utcnow().isoformat() + "Z",
        "data": {
            "Temperature": round(random.uniform(20.0, 35.0), 2),
            "Humidity":    round(random.uniform(30.0, 90.0), 2),
            "Status":      random.choice([0, 1]),
        },
    }


def load_device_configs() -> None:
    """Display loaded device/profiles for reference (optional)."""
    root = Path(__file__).resolve().parent.parent
    device_file = root / "res" / "devices" / "mqtt.test.device.json"
    profile_file = root / "res" / "profiles" / "mqtt.test.profile.json"

    if device_file.exists():
        with open(device_file, "r", encoding="utf-8") as f:
            devices = json.load(f)
            log.info("Loaded %d device(s) from %s", len(devices), device_file.name)
    else:
        log.warning("Device config not found: %s", device_file)

    if profile_file.exists():
        with open(profile_file, "r", encoding="utf-8") as f:
            profile = json.load(f)
            resources = profile.get("deviceResources", [])
            log.info(
                "Loaded profile '%s' with %d resource(s) from %s",
                profile.get("name"), len(resources), profile_file.name,
            )
    else:
        log.warning("Profile config not found: %s", profile_file)


def main() -> None:
    try:
        import paho.mqtt.client as mqtt  # type: ignore[import-untyped]
    except ImportError:
        log.error("paho-mqtt not installed. Run: pip install paho-mqtt")
        return

    load_device_configs()

    log.info("Connecting to gateway embedded broker at %s:%d ...", BROKER, PORT)
    client = mqtt.Client(client_id="mqtt-simulator")
    client.connect(BROKER, PORT, keepalive=60)
    client.loop_start()

    log.info("Publishing to topic '%s' every %ds", TOPIC, INTERVAL_SECONDS)

    try:
        while True:
            payload = generate_telemetry()
            payload_bytes = json.dumps(payload)
            result = client.publish(TOPIC, payload_bytes, qos=1)
            result.wait_for_publish()

            temp = payload["data"]["Temperature"]
            hum  = payload["data"]["Humidity"]
            log.info("Published → temp=%.1f°C, humidity=%.1f%%, status=%d",
                     temp, hum, payload["data"]["Status"])

            time.sleep(INTERVAL_SECONDS)
    except KeyboardInterrupt:
        log.info("Shutting down...")
    finally:
        client.loop_stop()
        client.disconnect()
        log.info("Disconnected.")


if __name__ == "__main__":
    main()
