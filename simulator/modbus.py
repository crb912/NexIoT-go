import asyncio
import logging
import json
import socket
import struct
from pathlib import Path
from pymodbus.server import StartAsyncTcpServer
from pymodbus.datastore import (
    ModbusSequentialDataBlock,
    ModbusDeviceContext,
    ModbusServerContext
)

# Configure basic logging
logging.basicConfig(format='%(asctime)s - %(levelname)s - %(message)s', level=logging.INFO)

set_val = {
    "subnet_mask": "255.255.255.0",
    "ip_address": "192.168.1.1",
    "gateway": "192.168.1.254",
    "mac_addres": "11:AA:22:BB",
    "dns": "8.8.8.8",
    "year": 2026,
    "month": 6,
    "day": 12,
    "hour": 20,
    "minute": 1,
    "second": 59
}

class JsonDataBlock(ModbusSequentialDataBlock):
    def __init__(self, memory_store, is_bit=False):
        self.is_bit = is_bit
        self.memory = memory_store
        super().__init__(0x00, [0]) # Dummy init to pass parent checks

    def validate(self, address, count=1):
        # Skip boundary checks
        return True

    def getValues(self, address, count=1):
        # Handle read requests
        physical_addr = address - 1
        values = []

        for i in range(count):
            curr_addr = physical_addr + i
            if curr_addr in self.memory:
                val = self.memory[curr_addr]
            else:
                # Fallback data generation
                val = (curr_addr % 2 == 0) if self.is_bit else curr_addr

            values.append(bool(val) if self.is_bit else int(val))

        logging.info(f"[READ] Addr: {physical_addr} | Count: {count} | Values: {values}")
        return values

    def setValues(self, address, values):
        # Handle write requests and save to memory store
        physical_addr = address - 1
        for i, val in enumerate(values):
            self.memory[physical_addr + i] = val

        logging.info(f"[WRITE] Addr: {physical_addr} | Count: {len(values)} | Written: {values}")


# ==========================================
# SRP Refactored Configuration Parsers
# ==========================================

def load_json_file(file_path):
    # Read and parse the JSON file safely
    try:
        with open(file_path, "r", encoding="utf-8") as f:
            return json.load(f)
    except (FileNotFoundError, json.JSONDecodeError) as e:
        logging.error(f"Failed to load JSON: {e}")
        return {}

def decode_mac(mac_str):
    # Convert MAC string to three uint16 registers
    clean_mac = str(mac_str).replace(":", "")
    mac_bytes = bytes.fromhex(clean_mac)
    return struct.unpack(">HHH", mac_bytes) if len(mac_bytes) == 6 else []

def decode_ipv4(ip_str):
    # Convert IPv4 string to two uint16 registers
    packed_ip = socket.inet_aton(str(ip_str))
    return struct.unpack(">HH", packed_ip)

def decode_uint32(val):
    # Convert uint32 integer to two uint16 registers
    packed_u32 = struct.pack(">I", int(val))
    return struct.unpack(">HH", packed_u32)

def populate_memory(profile_data, memory, overrides):
    # Map decoded register values to physical wire addresses
    for resource in profile_data.get("deviceResources", []):
        attrs = resource.get("attributes", {})
        name = resource.get("name", "")

        if "address" not in attrs:
            continue

        wire_addr = attrs["address"] - 1
        default_val = overrides.get(name, resource.get("properties", {}).get("defaultValue"))
        decode_func = attrs.get("decodeFunc", "")
        length = attrs.get("length", 1)

        if default_val is None:
            continue

        try:
            if "decodeMACAddress" in decode_func or "mac" in name.lower():
                regs = decode_mac(default_val)
                for i, r in enumerate(regs): memory[wire_addr + i] = r

            elif "decodeIPv4Address" in decode_func or name in ["ip_address", "subnet_mask", "gateway", "dns"]:
                regs = decode_ipv4(default_val)
                for i, r in enumerate(regs[:length]): memory[wire_addr + i] = r

            elif length == 2 and isinstance(default_val, int):
                regs = decode_uint32(default_val)
                for i, r in enumerate(regs): memory[wire_addr + i] = r

            else:
                memory[wire_addr] = int(default_val)

        except Exception as e:
            logging.warning(f"Error parsing {name} with value {default_val}: {e}")

    return memory

def build_initial_memory(file_path):
    # Master builder function for memory store
    profile_data = load_json_file(file_path)
    memory = populate_memory(profile_data, {}, set_val)
    logging.info(f"Loaded {len(memory)} registers into memory.")
    return memory


# ==========================================
# Server Startup Logic
# ==========================================

async def create_simulator():
    print("Starting JSON-Driven Modbus TCP Simulator...")

    script_dir = Path(__file__).parent.parent
    json_path = script_dir / "res" / "profiles" / "modbus.test.profile.json"

    # Initialize the memory dictionary
    initial_memory = build_initial_memory(str(json_path))

    # Apply the same memory dict to Holding (hr) and Input (ir) registers
    store = ModbusDeviceContext(
        co=JsonDataBlock(memory_store={}, is_bit=True),
        di=JsonDataBlock(memory_store={}, is_bit=True),
        hr=JsonDataBlock(memory_store=initial_memory, is_bit=False),
        ir=JsonDataBlock(memory_store=initial_memory, is_bit=False)
    )

    context = ModbusServerContext({1: store}, single=False)

    logging.info("Listening on 0.0.0.0:5020")
    await StartAsyncTcpServer(context=context, address=("0.0.0.0", 5020))

if __name__ == "__main__":
    try:
        asyncio.run(create_simulator())
    except KeyboardInterrupt:
        print("\nSimulator closed safely.")