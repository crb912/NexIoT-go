import asyncio
import logging
import tomllib
from pymodbus.server import StartAsyncTcpServer  
from pymodbus.datastore import (
    ModbusSequentialDataBlock,
    ModbusDeviceContext,
    ModbusServerContext
)

logging.basicConfig(format='%(asctime)s - %(levelname)s - %(message)s', level=logging.INFO)

class DynamicDataBlock(ModbusSequentialDataBlock):
    def __init__(self, is_bit=False):
        self.is_bit = is_bit
        # Init with a small list to pass parent checks
        super().__init__(0x00, [0])

    def validate(self, address, count=1):
        # Always return True to skip size checks
        return True

    def getValues(self, address, count=1):
        # pymodbus zero_mode=False makes 'address' 1-based by default
        logical_addr = address
        # Subtract 1 to get the actual wire address (0-based)
        physical_addr = address - 1

        if self.is_bit:
            # Rule: even physical address = True, odd = False
            values = [(addr % 2 == 0) for addr in range(physical_addr, physical_addr + count)]
            data_type = "BIT"
        else:
            # Rule: return the physical address value itself
            values = [addr for addr in range(physical_addr, physical_addr + count)]
            data_type = "REG"

        end_physical = physical_addr + count - 1
        end_logical = logical_addr + count - 1

        if count == 1:
            logging.info(f" [READ {data_type}] Device Reg (1-based): {logical_addr} ; Wire Addr (0-based): {physical_addr}  | Returns: {values[0]}")
        else:
            logging.info(f" [READ {data_type}] Device Reg: {logical_addr}~{end_logical} ; Wire Addr: {physical_addr}~{end_physical} ({count} items) | Returns: {values}")

        return values

    def setValues(self, address, values):
        logical_addr = address
        physical_addr = address - 1
        data_type = "BIT" if self.is_bit else "REG"

        count = len(values)
        end_physical = physical_addr + count - 1
        end_logical = logical_addr + count - 1

        if count == 1:
            logging.info(f" [WRITE {data_type}] Wire Addr (0-based): {physical_addr} -> Device Reg (1-based): {logical_addr} | Writes: {values[0]}")
        else:
            logging.info(f" [WRITE {data_type}] Wire Addr: {physical_addr}~{end_physical} ({count} items) -> Device Reg: {logical_addr}~{end_logical} | Writes: {values}")

# ==========================================

def load_toml_config():
    # 为了防止你本地没有 toml，这里做个简单的容错 mock
    try:
        file_path = r"modbus.toml"
        with open(file_path, "rb") as f:
            return tomllib.load(f)
    except FileNotFoundError:
        logging.warning("modbus.toml not found, using default configuration.")
        return {
            "protocol_interface_settings": {"port": 5020},
            "slaves": [{"enabled": True, "node_id": 1}]
        }

async def create_simulator():
    print("Starting Dynamic Modbus TCP Slave Simulator...")
    config = load_toml_config()

    devices_dict = {}
    for slave in config.get('slaves', []):
        if slave.get('enabled', False):
            node_id = slave['node_id']

            # 同时将自定义的数据块挂载到 4 种类型上
            store = ModbusDeviceContext(
                co=DynamicDataBlock(is_bit=True),  # Coils (读写位)
                di=DynamicDataBlock(is_bit=True),  # Discretes (只读位)
                hr=DynamicDataBlock(is_bit=False), # Holding Registers (读写字)
                ir=DynamicDataBlock(is_bit=False)  # Input Registers (只读字)
            )
            devices_dict[node_id] = store
            logging.info(f"Loaded dynamic slave node: Node ID {node_id}")

    context = ModbusServerContext(devices_dict, single=False)

    port = config['protocol_interface_settings']['port']
    host = "0.0.0.0"

    logging.info(f"Listening on {host}:{port}")
    await StartAsyncTcpServer(context=context, address=(host, port))

if __name__ == "__main__":
    try:
        asyncio.run(create_simulator())
    except KeyboardInterrupt:
        print("\nSimulator closed.")