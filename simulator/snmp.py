"""
SNMP Device emulator - Raw Socket UDP Server
"""

import os
import json
import socket
import threading
import time
import logging
import yaml
import tomllib
from collections import deque
from datetime import datetime
from typing import Dict, Any

# Configure simple logging
logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(message)s")

# ─── 1. GLOBAL VARIABLES ─────────────────────────────────────────────────────

MAX_LOG_ENTRIES = 5000
_snmp_logs = deque(maxlen=MAX_LOG_ENTRIES)

_clients_key = set()
_clients = deque(maxlen=2)
# OID → value map populated from profile YAML defaults.
# Each entry: {"1.3.6.1...": <int|str>}
default_value_map: Dict[str, Any] = {}

# Custom fallback values for OIDs that lack defaultValue in the profile YAML.
CUSTOM_DEFAULTS: Dict[str, Any] = {
    "1.3.6.1.4.1.28866.2.26.16.2.1.0": "AA:BB:CC:DD:EE:FF",  # MacAddress
    "1.3.6.1.4.1.28866.2.26.16.1.2.0": "v2.1.3",              # Firmware
    "1.3.6.1.4.1.28866.2.26.16.3.2.0": "192.168.1.100",       # IPV4Address
    "1.3.6.1.4.1.28866.2.26.16.3.3.0": "255.255.255.0",       # IPV4SubnetMask
    "1.3.6.1.4.1.28866.2.26.16.3.4.0": "192.168.1.1",         # IPV4GatewayAddress
}

_server_status = {
    "running": False,
    "host": "0.0.0.0",
    "port": 161,
}

# ─── 2. CONFIGURATION LOADERS ────────────────────────────────────────────────

def get_absolute_path(relative_path: str) -> str:
    """Convert a relative path to an absolute path based on script location."""
    script_dir = os.path.dirname(os.path.abspath(__file__))
    return os.path.join(script_dir, relative_path)

def load_toml_file(file_path: str) -> dict:
    """Read the device TOML configuration."""
    try:
        with open(file_path, "rb") as file:
            return tomllib.load(file)
    except Exception as e:
        logging.error(f"Failed to load TOML config: {e}")
        return {}

def load_yaml_file(file_path: str) -> dict:
    """Read the profile YAML configuration."""
    try:
        with open(file_path, "r", encoding="utf-8") as file:
            return yaml.safe_load(file)
    except Exception as e:
        logging.error(f"Failed to load YAML config: {e}")
        return {}

def get_default_value(val_type: str) -> Any:
    """Return a safe default value based on the type string."""
    if val_type in ("Int32", "Uint64", "Int16", "Uint16"):
        return 0
    return "Unknown"

def populate_oid_datastore(yaml_data: dict):
    """Extract OIDs and default values from YAML and store them.

    Priority: profile YAML defaultValue → CUSTOM_DEFAULTS → type-based fallback.
    """
    resources = yaml_data.get("deviceResources", [])

    for res in resources:
        attrs = res.get("attributes", {})
        raw_oid = attrs.get("oid", attrs.get("address", ""))
        clean_oid = raw_oid.strip(" .")

        if not clean_oid:
            continue

        props = res.get("properties", {})
        val_type = props.get("valueType", "String")

        # 1) Try profile YAML defaultValue
        default_val = props.get("defaultValue")

        # 2) Fall back to CUSTOM_DEFAULTS
        if default_val in (None, ""):
            default_val = CUSTOM_DEFAULTS.get(clean_oid)

        # 3) Ultimate fallback
        if default_val in (None, ""):
            default_val = get_default_value(val_type)

        # Convert numeric strings to actual integers
        if val_type in ("Int32", "Uint64") and isinstance(default_val, str):
            try:
                default_val = int(default_val)
            except ValueError:
                default_val = 0

        default_value_map[clean_oid] = default_val
        logging.info(f"Loaded OID: {clean_oid} -> {default_val} ({val_type})")

def _log_snmp(action: str, oid: str, value: Any):
    """Save the SNMP action to the in-memory logs."""
    val_str = str(value)
    if len(val_str) > 70:
        val_str = val_str[:67] + "..."

    entry = {
        "ts": datetime.now().strftime("%H:%M:%S.%f")[:-3],
        "action": action,
        "oid": oid,
        "value": val_str,
    }
    logging.info(f"[{action}] OID: {oid} | Value: {val_str}")
    _snmp_logs.append(entry)


# ─── 3. MINIMAL ASN.1 BER PARSER & ENCODER ───────────────────────────────────

def parse_tlv(data: bytes, offset: int = 0):
    """Parse Type, Length, and Value from byte data."""
    if offset >= len(data):
        return None, None, offset

    tag = data[offset]
    offset += 1
    length = data[offset]
    offset += 1

    # Handle lengths greater than 127 bytes
    if length > 127:
        num_bytes = length & 0x7F
        length = int.from_bytes(data[offset:offset + num_bytes], 'big')
        offset += num_bytes

    value = data[offset:offset + length]
    return tag, value, offset + length

def encode_len(length: int) -> bytes:
    """Encode the length field for ASN.1 BER."""
    if length < 128:
        return bytes([length])
    if length < 256:
        return bytes([0x81, length])
    return bytes([0x82, length >> 8, length & 0xFF])

def decode_oid(value_bytes: bytes) -> str:
    """Convert byte data into an OID string."""
    if not value_bytes:
        return ""

    oid = [str(value_bytes[0] // 40), str(value_bytes[0] % 40)]
    val = 0
    for b in value_bytes[1:]:
        val = (val << 7) | (b & 0x7F)
        if not (b & 0x80):
            oid.append(str(val))
            val = 0
    return ".".join(oid)

def encode_oid(oid_str: str) -> bytes:
    """Convert an OID string into byte data."""
    parts = [int(x) for x in oid_str.strip(".").split(".")]
    if len(parts) < 2:
        return bytes([0x06, 0x01, 0x00])

    enc = [parts[0] * 40 + parts[1]]
    for p in parts[2:]:
        if p == 0:
            enc.append(0)
            continue
        p_bytes = []
        while p > 0:
            p_bytes.insert(0, (p & 0x7F) | 0x80)
            p >>= 7
        p_bytes[-1] &= 0x7F
        enc.extend(p_bytes)

    length_bytes = encode_len(len(enc))
    return bytes([0x06]) + length_bytes + bytes(enc)


# ─── 4. SNMP PACKET PROCESSING ───────────────────────────────────────────────

def unpack_snmp_request(data: bytes):
    """Unpack SNMP sequence to get PDU details."""
    tag, seq_val, _ = parse_tlv(data)
    if tag != 0x30:
        return None  # Not an ASN.1 Sequence

    _, v_val, offset = parse_tlv(seq_val)
    _, c_val, offset = parse_tlv(seq_val, offset)
    pdu_tag, pdu_val, _ = parse_tlv(seq_val, offset)

    # Only handle GET (0xa0), GETNEXT (0xa1), or SET (0xa3)
    if pdu_tag not in (0xa0, 0xa1, 0xa3):
        return None

    _, req_val, offset2 = parse_tlv(pdu_val)

    # Skip Error Status and Error Index
    _, _, offset2 = parse_tlv(pdu_val, offset2)
    _, _, offset2 = parse_tlv(pdu_val, offset2)

    _, vbl_val, offset2 = parse_tlv(pdu_val, offset2)
    _, vb_val, _ = parse_tlv(vbl_val)
    _, oid_val, offset3 = parse_tlv(vb_val)

    oid_str = decode_oid(oid_val)
    val_tag, val_data, _ = parse_tlv(vb_val, offset3)

    return v_val, c_val, pdu_tag, req_val, oid_str, val_tag, val_data

def process_set_request(oid_str: str, val_tag: int, val_data: bytes):
    """Process a SET request and return the response value."""
    if val_tag == 0x02:  # Integer
        val_to_save = int.from_bytes(val_data, 'big')
    else:  # String or other
        val_to_save = val_data.decode('utf-8', errors='ignore')

    default_value_map[oid_str] = val_to_save
    _log_snmp("WRITE", oid_str, val_to_save)

    # Return the same value for the response
    return val_tag, val_data

def process_get_request(oid_str: str):
    """Process a GET request and return the response value."""
    # Handle the `.0` suffix that snmpget often appends
    lookup_oid = oid_str[:-2] if oid_str.endswith(".0") else oid_str

    # Fetch from datastore, default to string "No Such Object" if missing
    val = default_value_map.get(oid_str, default_value_map.get(lookup_oid, "No_Data"))

    _log_snmp("READ", oid_str, val)

    if isinstance(val, int):
        length = max((val.bit_length() + 8) // 8, 1)
        val_bytes = val.to_bytes(length, 'big', signed=True)
        return 0x02, val_bytes  # 0x02 is Integer tag
    else:
        str_bytes = str(val).encode('utf-8')
        return 0x04, str_bytes  # 0x04 is OctetString tag

def build_response_packet(v_val, c_val, req_id, oid_str, res_tag, res_data):
    """Build the final SNMP response packet bytes."""
    res_val_tlv = bytes([res_tag]) + encode_len(len(res_data)) + res_data
    encoded_oid = encode_oid(oid_str)

    varbind_inner = encoded_oid + res_val_tlv
    varbind = bytes([0x30]) + encode_len(len(varbind_inner)) + varbind_inner
    varbind_list = bytes([0x30]) + encode_len(len(varbind)) + varbind

    req_id_tlv = bytes([0x02]) + encode_len(len(req_id)) + req_id
    err_tlv = bytes([0x02, 0x01, 0x00])  # Error status 0
    idx_tlv = bytes([0x02, 0x01, 0x00])  # Error index 0

    pdu_payload = req_id_tlv + err_tlv + idx_tlv + varbind_list
    pdu = bytes([0xa2]) + encode_len(len(pdu_payload)) + pdu_payload

    v_tlv = bytes([0x02]) + encode_len(len(v_val)) + v_val
    c_tlv = bytes([0x04]) + encode_len(len(c_val)) + c_val

    final_payload = v_tlv + c_tlv + pdu
    return bytes([0x30]) + encode_len(len(final_payload)) + final_payload

def handle_snmp_packet(data: bytes, addr: tuple, sock: socket.socket):
    """Coordinate parsing, processing, and replying to an SNMP packet."""
    try:
        parsed = unpack_snmp_request(data)
        if not parsed:
            return

        v_val, c_val, pdu_tag, req_id, oid_str, val_tag, val_data = parsed

        if pdu_tag == 0xa3:
            res_tag, res_data = process_set_request(oid_str, val_tag, val_data)
        else:
            res_tag, res_data = process_get_request(oid_str)

        response_packet = build_response_packet(
            v_val, c_val, req_id, oid_str, res_tag, res_data
        )
        sock.sendto(response_packet, addr)

    except Exception as e:
        logging.error(f"Error parsing SNMP packet: {e}")


# ─── 5. RAW UDP SNMP SERVER ──────────────────────────────────────────────────

def record_client(ip: str, port: int):
    """Record a new client connection."""
    client_key = f"{ip}:{port}"

    if client_key not in _clients_key:
        create_at = time.time()
        _clients.append({
            "token": "SNMP",
            "ip": ip,
            "port": str(port),
            "username": "SNMP",
            "online_sub": False,
            "alarm_sub": False,
            "created_at": create_at,
        })
    _clients_key.add(client_key)

def run_udp_server(host: str, port: int):
    """Run a standard UDP server socket."""
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)

    try:
        sock.bind((host, port))
    except PermissionError:
        logging.error(f"Failed to bind port {port}. Use root if port < 1024.")
        return
    except Exception as e:
        logging.error(f"Failed to start server: {e}")
        return

    sock.settimeout(1.0)
    _server_status["running"] = True
    logging.info(f"SNMP Simulator is running on UDP {host}:{port}...")

    while _server_status["running"]:
        try:
            data, addr = sock.recvfrom(4096)
            client_ip, client_port = addr

            record_client(client_ip, client_port)
            handle_snmp_packet(data, addr, sock)

        except socket.timeout:
            continue
        except Exception as e:
            logging.error(f"Server error: {e}")

    sock.close()
    logging.info("SNMP server stopped.")


# ─── 6. MAIN EXECUTION FLOW ──────────────────────────────────────────────────

def start_snmp_server():
    """Load configuration and start the UDP SNMP server."""

    # 1. Resolve paths dynamically
    toml_path = get_absolute_path("../res/devices/snmp.test.device.toml")
    yaml_path = get_absolute_path("../res/profiles/snmp.test.profile.yaml")

    # 2. Load configurations
    device_config = load_toml_file(toml_path)
    profile_config = load_yaml_file(yaml_path)

    if not device_config or not profile_config:
        logging.error("Missing required configuration files. Exiting.")
        return

    # 3. Extract Host and Port from TOML
    try:
        snmp_config = device_config["DeviceList"][0]["Protocols"]["snmp"]
        host = snmp_config["Address"]
        port = int(snmp_config["Port"])
    except KeyError:
        logging.error("Invalid TOML format. Using default 0.0.0.0:1610")
        host = "0.0.0.0"
        port = 1610

    _server_status.update({
        "host": host,
        "port": port
    })

    # 4. Populate Datastore from YAML
    populate_oid_datastore(profile_config)

    # 5. Start the server thread
    snmp_thread = threading.Thread(
        target=run_udp_server,
        args=(host, port),
        daemon=True
    )
    snmp_thread.start()
    logging.info("SNMP server initialized in background thread.")

if __name__ == "__main__":
    start_snmp_server()

    try:
        # Keep the main thread alive while the daemon thread runs
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        _server_status["running"] = False
        logging.info("Exiting program...")