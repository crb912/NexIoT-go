阅读pkg/adapter/modbus/modbus.go的源码，它是modbus协议的适配器，其接口定义在 pkg/protocol/connector.go 。  参考我的Modbus适配器的代码，基于开源库 gopcua/opcua， 帮我实现opcua协议的适配器(代码实现在pkg/adapter/opcua)，实现opc的设备的RWClient接口，读数据和写入。

基本约束:
1. 所有代码注释采用简单的英文
2. 写代码之前，先确认方案，我同意后再开始执行。
3. 类型转换的代码，实现在: pkg/conv/conv.go
4. 读写接口的入口参数都需要, *model.Resource结构体，它定义在pkg/model/resource.go
5. /home/raybing/Desktop/github/hermes-edge/internal/driver/drive.go 的HandleReadCommands函数是读取设备的入口，从这里调用各协议ReadSingle和ReadBatch

工厂函数应该与Modbus的函数参数保持统一， 参考: func NewModbusClient(endpoint string, pt model.ProtocolType, defaultTimeout time.Duration, args map[string]string) (*ModbusClient, error) 。