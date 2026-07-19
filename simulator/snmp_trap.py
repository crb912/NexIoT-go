"""
SNMP Trap Simulator — sends periodic v2c trap packets to the gateway.

Usage:
    python simulator/snmp_trap.py

Requires: pysnmp (pip install pysnmp)
The gateway's SNMP trap listener must be running on localhost:162.
"""

import logging
import random
import time
from pathlib import Path

# ─── Configuration ─────────────────────────────────────────────────────────

HOST   = "127.0.0.1"
PORT   = 1620
INTERVAL_SECONDS = 10

# ─── Logging ───────────────────────────────────────────────────────────────

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [SNMP-TRAP] %(message)s",
)
log = logging.getLogger(__name__)


def load_configs() -> None:
    """Display loaded configs for reference."""
    import json
    root = Path(__file__).resolve().parent.parent
    for fname in ["snmp_trap.test.device.json", "snmp_trap.test.profile.json"]:
        path = root / "res" / "devices" / fname if "device" in fname else root / "res" / "profiles" / fname
        if path.exists():
            with open(path, "r", encoding="utf-8") as f:
                json.load(f)
                log.info("Loaded %s", path.name)
        else:
            log.warning("Not found: %s", path)


def send_trap_v2c(uptime: int) -> None:
    """Send an SNMP v2c trap using raw ASN.1 BER encoding over UDP."""
    import socket
    import struct

    # --- ASN.1 BER helpers ---
    def encode_length(n: int) -> bytes:
        if n < 128:
            return bytes([n])
        if n < 256:
            return bytes([0x81, n])
        return bytes([0x82, (n >> 8) & 0xFF, n & 0xFF])

    def encode_integer(tag: int, value: int) -> bytes:
        if value == 0:
            return bytes([tag, 1, 0])
        b = []
        v = value
        while v > 0:
            b.insert(0, v & 0xFF)
            v >>= 8
        return bytes([tag]) + encode_length(len(b)) + bytes(b)

    def encode_oid(oid_str: str) -> bytes:
        parts = [int(x) for x in oid_str.strip(".").split(".")]
        enc = [parts[0] * 40 + parts[1]]
        for p in parts[2:]:
            if p == 0:
                enc.append(0)
                continue
            sub = []
            while p:
                sub.insert(0, (p & 0x7F) | 0x80)
                p >>= 7
            sub[-1] &= 0x7F
            enc.extend(sub)
        length = encode_length(len(enc))
        return bytes([0x06]) + length + bytes(enc)

    def encode_octet_string(value: bytes) -> bytes:
        return bytes([0x04]) + encode_length(len(value)) + value

    def encode_timeticks(value: int) -> bytes:
        return encode_integer(0x43, value)

    def encode_null() -> bytes:
        return bytes([0x05, 0x00])

    def encode_varbind(oid: str, value_enc: bytes) -> bytes:
        oid_enc = encode_oid(oid)
        return encode_sequence(oid_enc + value_enc)

    def encode_sequence(content: bytes) -> bytes:
        return bytes([0x30]) + encode_length(len(content)) + content

    # --- Build PDU ---
    request_id = random.randint(1, 2147483647)

    # SNMPv2-MIB::snmpTrapOID.0 = 1.3.6.1.6.3.1.1.4.1.0
    trap_oid = "1.3.6.1.6.3.1.1.4.1.0"
    enterprise_oid = "1.3.6.1.6.3.1.1.5.1"  # linkUp trap

    # sysUpTime.0
    vb1 = encode_varbind("1.3.6.1.2.1.1.3.0", encode_timeticks(uptime))
    # snmpTrapOID.0
    vb2 = encode_varbind(trap_oid, encode_oid(enterprise_oid))
    # UpTimeCurrentState (matches pre-defined profile)
    vb3 = encode_varbind("1.3.6.1.4.1.28866.2.26.16.1.1.0", encode_timeticks(uptime))
    # MacAddressCurrentState
    mac = f"{random.randint(0,255):02X}:{random.randint(0,255):02X}:{random.randint(0,255):02X}"
    vb4 = encode_varbind("1.3.6.1.4.1.28866.2.26.16.2.1.0", encode_octet_string(mac.encode()))
    # FirmwareCurrentState
    fw = f"v{random.randint(1,5)}.{random.randint(0,9)}.{random.randint(0,99)}"
    vb5 = encode_varbind("1.3.6.1.4.1.28866.2.26.16.1.2.0", encode_octet_string(fw.encode()))

    varbinds = vb1 + vb2 + vb3 + vb4 + vb5

    pdu = encode_sequence(
        encode_integer(0x02, request_id) +   # request-id
        encode_integer(0x02, 0) +             # error-status
        encode_integer(0x02, 0) +             # error-index
        encode_sequence(varbinds)             # variable-bindings
    )

    # SNMP v2c trap: version=1 (v2c), community, pdu type=a7
    version = encode_integer(0x02, 1)  # SNMP v2c = 1
    community = encode_octet_string(b"public")
    trap_pdu = bytes([0xA7]) + encode_length(len(pdu)) + pdu

    message = encode_sequence(version + community + trap_pdu)

    # --- Send via UDP ---
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        sock.sendto(message, (HOST, PORT))
        log.info("Trap sent: uptime=%d", uptime)
    finally:
        sock.close()


def main() -> None:
    load_configs()

    uptime = 0
    log.info("Sending SNMP v2c traps to %s:%d every %ds", HOST, PORT, INTERVAL_SECONDS)

    try:
        while True:
            uptime += INTERVAL_SECONDS * 100  # centiseconds

            send_trap_v2c(uptime)
            time.sleep(INTERVAL_SECONDS)
    except KeyboardInterrupt:
        log.info("Shutting down...")


if __name__ == "__main__":
    main()
