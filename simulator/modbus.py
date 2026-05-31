# pip install pymodbus

import logging
# Import necessary modules from pymodbus library
from pymodbus.server import StartTcpServer
from pymodbus.datastore import ModbusSequentialDataBlock, ModbusSlaveContext, ModbusServerContext

# Setup logging to see EdgeX reading requests in the terminal
logging.basicConfig(format='%(asctime)s - %(message)s', level=logging.DEBUG)

def start_simulator():
    # Create a block of Holding Registers.
    # Starting at address 0, create 100 registers.
    # Set the default value of all registers to 25.
    data_block = ModbusSequentialDataBlock(0, [25] * 100)

    # Create the slave context (store) with our holding registers (hr)
    store = ModbusSlaveContext(hr=data_block)

    # Create the server context. single=True means all slave IDs map to the same store.
    context = ModbusServerContext(slaves=store, single=True)

    print("=============================================")
    print("Starting Modbus TCP Slave Simulator...")
    print("Listening on 0.0.0.0, Port 502")
    print("Waiting for EdgeX to connect and read data...")
    print("=============================================")

    # Start the TCP server on port 502
    # Note: Port 502 requires 'sudo' or root privileges on Linux systems
    StartTcpServer(context=context, address=("0.0.0.0", 502))

if __name__ == "__main__":
    start_simulator()