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
        # Dictionary to store register values based on physical wire address (0-based)
        self.memory = memory_store
        # Init with a dummy list to pass parent class checks
        super().__init__(0x00, [0])

    def validate(self, address, count=1):
        # Always return True to skip strict boundary checks
        return True

    def getValues(self, address, count=1):
        logical_addr = address
        # Subtract 1 to get the actual wire address (0-based)
        physical_addr = address - 1
        values = []

        for i in range(count):
            current_addr = physical_addr + i

            # 1. Use the preset value if it exists in the configuration memory
            if current_addr in self.memory:
                val = self.memory[current_addr]
            # 2. If no preset value, dynamically generate the fallback value
            else:
                if self.is_bit:
                    # Rule: even physical address = True, odd = False
                    val = (current_addr % 2 == 0)
                else:
                    # Rule: return the physical address value itself
                    val = current_addr

            if self.is_bit:
                values.append(bool(val))
            else:
                values.append(int(val))

        data_type = "BIT" if self.is_bit else "REG"
        end_physical = physical_addr + count - 1
        end_logical = logical_addr + count - 1

        if count == 1:
            logging.info(f" [READ {data_type}] Device Reg: {logical_addr} ; Wire Addr: {physical_addr} | Returns: {values[0]}")
        else:
            logging.info(f" [READ {data_type}] Device Reg: {logical_addr}~{end_logical} ; Wire Addr: {physical_addr}~{end_physical} | Returns: {values}")

        return values

    def setValues(self, address, values):
        logical_addr = address
        physical_addr = address - 1
        data_type = "BIT" if self.is_bit else "REG"

        # Save written values into our dictionary to persist changes
        for i, val in enumerate(values):
            self.memory[physical_addr + i] = val

        count = len(values)
        end_physical = physical_addr + count - 1
        end_logical = logical_addr + count - 1

        if count == 1:
            logging.info(f" [WRITE {data_type}] Wire Addr: {physical_addr} -> Device Reg: {logical_addr} | Writes: {values[0]}")
        else:
            logging.info(f" [WRITE {data_type}] Wire Addr: {physical_addr}~{end_physical} -> Device Reg: {logical_addr}~{end_logical} | Writes: {values}")

# ==========================================

def load_json_profile(file_path):
    memory = {}
    try:
        with open(file_path, "r", encoding="utf-8") as f:
            profile = json.load(f)
    except FileNotFoundError:
        logging.error(f"Configuration file not found at: {file_path}")
        return memory
    except json.JSONDecodeError:
        logging.error("Failed to parse the JSON file.")
        return memory

    for resource in profile.get("deviceResources", []):
        attrs = resource.get("attributes", {})
        props = resource.get("properties", {})
        name = resource.get("name", "")

        if "address" not in attrs:
            continue

        logical_addr = attrs["address"]
        wire_addr = logical_addr - 1
        length = attrs.get("length", 1)
        default_val = props.get("defaultValue")

        if name in set_val:
            default_val = set_val[name]

        if default_val is not None:
            decode_func = attrs.get("decodefunc", "")

            # MAC 地址（3个寄存器）
            if "decodeMACAddress" in decode_func or name == "mac_address":
                try:
                    clean_mac = str(default_val).replace(":", "")
                    mac_bytes = bytes.fromhex(clean_mac)
                    if len(mac_bytes) == 6:
                        regs = struct.unpack(">HHH", mac_bytes)
                        memory[wire_addr] = regs[0]
                        memory[wire_addr + 1] = regs[1]
                        memory[wire_addr + 2] = regs[2]
                except ValueError:
                    logging.warning(f"Invalid MAC address format: {default_val}")

            # IPv4 地址（2个寄存器，大端）
            elif "decodeIPv4Address" in decode_func or name in ["ip_address", "subnet_mask", "gateway", "dns", "target_ip_address"]:
                try:
                    packed_ip = socket.inet_aton(str(default_val))
                    regs = struct.unpack(">HH", packed_ip)
                    memory[wire_addr] = regs[0]
                    if length > 1:
                        memory[wire_addr + 1] = regs[1]
                except socket.error:
                    logging.warning(f"Invalid IP address format: {default_val}")

            # 32位整数（2个寄存器，大端字序，和IP保持一致）
            elif length == 2 and isinstance(default_val, int):
                try:
                    # 大端打包 uint32，再拆成两个 uint16
                    packed_u32 = struct.pack(">I", default_val)
                    regs = struct.unpack(">HH", packed_u32)
                    memory[wire_addr] = regs[0]      # 低地址 = 高字
                    memory[wire_addr + 1] = regs[1]  # 高地址 = 低字
                except struct.error:
                    logging.warning(f"Invalid uint32 value: {default_val}")

            # 普通 16位整数
            else:
                try:
                    memory[wire_addr] = int(default_val)
                except ValueError:
                    pass

    # 移除硬编码的 memory[8300] = 0，由上面的 length=2 逻辑自动处理
    logging.info(f"Loaded {len(memory)} predefined registers from JSON profile.")
    return memory

async def create_simulator():
    print("Starting JSON-Driven Modbus TCP Slave Simulator...")

    port = 5020
    host = "0.0.0.0"
    node_id = 1

    # Use pathlib to get the absolute path based on the current script location
    script_dir = Path(__file__).parent.parent
    json_path = script_dir / "res" / "profiles" / "modbus.test.profile.json"

    # Load initial memory from the JSON file
    initial_memory = load_json_profile(str(json_path))

    store = ModbusDeviceContext(
        co=JsonDataBlock(memory_store={}, is_bit=True),
        di=JsonDataBlock(memory_store={}, is_bit=True),
        hr=JsonDataBlock(memory_store=initial_memory, is_bit=False),
        ir=JsonDataBlock(memory_store=initial_memory, is_bit=False)
    )

    devices_dict = {node_id: store}
    logging.info(f"Loaded slave node: Node ID {node_id}")

    context = ModbusServerContext(devices_dict, single=False)

    logging.info(f"Listening on {host}:{port}")
    await StartAsyncTcpServer(context=context, address=(host, port))

if __name__ == "__main__":
    try:
        asyncio.run(create_simulator())
    except KeyboardInterrupt:
        print("\nSimulator closed.")