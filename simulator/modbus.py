import asyncio
import logging
import tomllib
from pymodbus.server import StartAsyncTcpServer  
from pymodbus.datastore import ModbusSequentialDataBlock
# ✅ 修复 1：使用全新的 ModbusDeviceContext 替代已被删除的 ModbusSlaveContext
from pymodbus.datastore import ModbusDeviceContext, ModbusServerContext 

logging.basicConfig(format='%(asctime)s - %(levelname)s - %(message)s', level=logging.INFO)


class LoggingDataBlock(ModbusSequentialDataBlock):
    def getValues(self, address, count=1):
        values = super().getValues(address, count)
        start_reg = address
        end_reg = start_reg + count - 1
        
        if count == 1:
            logging.info(f" [READ] 主机读取了寄存器 {start_reg}, 返回值: {values}")
        else:
            logging.info(f" [READ] 主机批量读取了寄存器 {start_reg} ~ {end_reg} (共 {count} 个字), 返回值: {values}")
            
        return values

    def setValues(self, address, values):
        start_reg = address + 1
        count = len(values)
        end_reg = start_reg + count - 1
        
        if count == 1:
            logging.info(f" [WRITE] 主机修改了寄存器 {start_reg}, 新值: {values}")
        else:
            logging.info(f"[WRITE] 主机批量修改了寄存器 {start_reg} ~ {end_reg}, 新值: {values}")
            
        super().setValues(address, values)

# ==========================================

def load_toml_config():
    file_path = r"modbus.toml"
    with open(file_path, "rb") as f:
        return tomllib.load(f)

def print_non_zero_items(lst):
    for index, value in enumerate(lst):
        pass 
	
def reset_register(reg_kv):
    """自定义预设一些寄存器用于模拟"""
    def set_reg(reg_id, value):
        reg_kv[reg_id] = value
        
    set_reg(8000, 0)
    set_reg(8001, 0)
    set_reg(8002, 0)
    set_reg(8003, 4)
    set_reg(8004, 10)
    set_reg(8005, 40)
    set_reg(8006, 30)
    set_reg(8007, 40)
    set_reg(8008, 1)     
    set_reg(8009, 0)
    set_reg(8010, 2)
    set_reg(8011, 6)     
    set_reg(8012, 6)     
    set_reg(8013, 7)   
    set_reg(8014, 5000)   
    set_reg(8015, 5001)  
    set_reg(8016, 0)
    set_reg(8017, 0)
    set_reg(8018, 0)
    set_reg(8019, 0)
    set_reg(8020, 0)
    
    # 1. MAC 地址 (8023~8025) -> F4:0E:11:30:00:00
    set_reg(8023, 0xF40E)
    set_reg(8024, 0x1130)
    set_reg(8025, 0x0000)

    # 2. 网络 IP (8026~8027) -> 192.168.0.30
    set_reg(8026, 0xC0A8)
    set_reg(8027, 0x001E)
    
    # 3. 网关 (8028~8029) -> 192.168.0.1
    set_reg(8028, 0xC0A8)
    set_reg(8029, 0x0001)
    
    # 4. DNS
    set_reg(8032, 0xC0A8)
    set_reg(8033, 0x0001)
    
    # 5. 子网掩码 (8030~8031) -> 255.255.255.0
    set_reg(8030, 0xFFFF)
    set_reg(8031, 0xFF00)
    
    # 6. 目标端口 (8034)
    set_reg(8034, 502)
    
    # 7. 目标 IP (8035~8036) -> 192.168.0.100
    set_reg(8035, 0xC0A8)
    set_reg(8036, 0x0064)
    
    set_reg(8299, 0x0000)
    set_reg(8300, 0x07EA)  
    set_reg(8301, 3)
    set_reg(8302, 5)
    set_reg(8303, 8)
    set_reg(8304, 8)
    set_reg(8305, 8)
    
    print_non_zero_items(reg_kv)

async def create_simulator():
    print("Starting Modbus TCP Slave Simulator...")
    config = load_toml_config()
    hr_values = [0] * 65536
             
    reset_register(hr_values)
    
    devices_dict = {}
    for slave in config.get('slaves', []):
        if slave.get('enabled', False):
            node_id = slave['node_id']
            # ✅ 修复 2：将 ModbusSlaveContext 替换为新版合规名称 ModbusDeviceContext
            store = ModbusDeviceContext(hr=LoggingDataBlock(0x00, list(hr_values)))
            devices_dict[node_id] = store
            logging.info(f"已装载从机节点: Node ID {node_id}")

    # ✅ 修复 3：官方去除了 slaves= 关键字参数，我们直接用位置参数传入字典！
    context = ModbusServerContext(devices_dict, single=False)

    port = config['protocol_interface_settings']['port']
    host = "0.0.0.0" 
    
    logging.info(f"Listening on {host}:{port}")
    
    await StartAsyncTcpServer(context=context, address=(host, port))

if __name__ == "__main__":
    try:
        asyncio.run(create_simulator())
    except KeyboardInterrupt:
        print("模拟器已关闭。")
