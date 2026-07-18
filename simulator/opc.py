import json
import asyncio
import logging
import random
from pathlib import Path
from asyncua import Server, ua

# Configure basic logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("OPC-Simulator")

# Suppress noisy internal address space logs from asyncua
logging.getLogger("asyncua.server.address_space").setLevel(logging.WARNING)

class ConfigLoader:
    """Class to handle reading and parsing configuration files."""
    
    def __init__(self):
        # Get the project root directory (one level up from 'simulator' folder)
        self.project_root = Path(__file__).resolve().parent.parent

    def load_json(self, relative_path: str):
        """Read and return JSON data from a file."""
        file_path = self.project_root / relative_path
        
        try:
            with open(file_path, 'r', encoding='utf-8') as file:
                return json.load(file)
        except FileNotFoundError:
            logger.error(f"Config file not found: {file_path}")
            raise

class OpcNodeManager:
    """Class to manage the creation of OPC UA nodes based on profiles."""
    
    @staticmethod
    def parse_node_id(address: str) -> ua.NodeId:
        """Convert string address (e.g., 'ns=3;i=1001') to ua.NodeId."""
        parts = address.split(';')
        ns = int(parts[0].split('=')[1])
        i = int(parts[1].split('=')[1])
        # Use positional arguments: identifier first, then namespace index
        return ua.NodeId(i, ns)

    @staticmethod
    def get_variant_type(type_string: str) -> ua.VariantType:
        """Map profile value types to OPC UA variant types."""
        type_map = {
            "Float64": ua.VariantType.Double,
            "Int32": ua.VariantType.Int32
        }
        return type_map.get(type_string, ua.VariantType.String)

    @staticmethod
    def _get_initial_value(props: dict, variant_type: ua.VariantType):
        """Extract and convert defaultValue from deviceResource properties.
        
        The defaultValue is stored as a string in JSON (e.g. "2.56", "0").
        Convert to the correct numeric type based on variant_type.
        Falls back to zero if defaultValue is missing or unparseable.
        """
        raw = props.get("defaultValue")
        if raw is None:
            return 0.0 if variant_type == ua.VariantType.Double else 0
        try:
            if variant_type == ua.VariantType.Double:
                return float(raw)
            if variant_type == ua.VariantType.Int32:
                return int(float(raw))  # float() first to handle "2.56" → 2
            return 0
        except (ValueError, TypeError):
            return 0.0 if variant_type == ua.VariantType.Double else 0

    @staticmethod
    async def create_nodes(server: Server, resources: list, device_commands: list) -> dict:
        """Create variables in the OPC UA server and return a dictionary of node objects."""
        nodes = {}
        objects_node = server.nodes.objects
        
        for resource in resources:
            name = resource["name"]
            address = resource["attributes"]["address"]
            props = resource["properties"]
            value_type_str = props["valueType"]
            is_writable = props.get("readWrite") == "RW"
            
            # Setup node parameters
            node_id = OpcNodeManager.parse_node_id(address)
            variant_type = OpcNodeManager.get_variant_type(value_type_str)

            # Use defaultValue from deviceResource properties if present, fall back to zero
            initial_value = OpcNodeManager._get_initial_value(props, variant_type)
            
            # Create the variable
            var_node = await objects_node.add_variable(
                nodeid=node_id, 
                bname=name, 
                val=initial_value, 
                varianttype=variant_type
            )
            
            if is_writable:
                await var_node.set_writable()
                
            nodes[name] = var_node
            logger.info(f"Created Node: {name} | Address: {address} | Writable: {is_writable}")
            
        return nodes

class SimulationTask:
    """Class to handle background data updates to simulate real device behavior."""
    
    def __init__(self, nodes: dict):
        self.nodes = nodes
        self.counter = 0

    async def run(self):
        """Run continuous loop to update dynamic variables."""
        logger.info("Starting simulation data loop...")
        
        while True:
            # Update Counter node
            if "Counter" in self.nodes:
                self.counter += 1
                await self.nodes["Counter"].write_value(
                    ua.DataValue(ua.Variant(self.counter, ua.VariantType.Int32))
                )
                
            # Update Random node
            if "Random" in self.nodes:
                random_val = random.uniform(0.0, 100.0)
                await self.nodes["Random"].write_value(
                    ua.DataValue(ua.Variant(random_val, ua.VariantType.Double))
                )
                
            # Wait for 1 second before next update
            await asyncio.sleep(1)

async def main():
    """Main application flow."""
    # 1. Load configurations
    config_loader = ConfigLoader()
    devices = config_loader.load_json("res/devices/opc.test.device.json")
    profile = config_loader.load_json("res/profiles/opc.test.profile.json")
    
    # Get the active device config
    device_cfg = devices[0]
    endpoint = device_cfg["protocols"]["opcua"]["Endpoint"]
    
    # 2. Setup OPC UA Server
    server = Server()
    await server.init()
    server.set_endpoint(endpoint)
    server.set_server_name(profile["description"])
    
    # 3. Create Nodes
    resources = profile["deviceResources"]
    device_commands = profile.get("deviceCommands", [])
    created_nodes = await OpcNodeManager.create_nodes(server, resources, device_commands)
    
    # 4. Start Server
    async with server:
        logger.info(f"OPC UA Server started at {endpoint}")
        
        # 5. Start Background Simulation
        simulation = SimulationTask(created_nodes)
        await simulation.run()

if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logger.info("Server stopped by user.")