> 完整架构文档，供大模型下次会话加载。读取此文件即可获得全部上下文。

## System Architecture

```Plaintext
┌──────────────────────────────────────────────────────────────────────┐
│                         DRIVER CORE LAYER                            │
│  (internal/driver, implements EdgeX Device Service SDK interface     │
├──────────────────────────────────────────────────────────────────────┤
│ - Core driver logic to serve upstream EdgeX framework.               │
│ - Implements Init, Start, Stop, Device Commands, and Device Events.  │
│ - Forwards received data via EdgeX async channel.                    │
└─────────────────────────────────┌────────────────────────────────────┘
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│                      PROTOCOL INTERFACE LAYER                        │
│                (Manage Active & Passive Connection Pools)            │
├──────────────────────────────────────────────────────────────────────┤
│ - Defines protocol interface: Session, Reader, Writer.               │
│ - Manages connection pool lifecycle.                                 │
│ - Shared behavior across protocol adapters.                          │
└─────────────────────────────────┌────────────────────────────────────┘
                                  ▼
┌──────────────────────────────────────────────────────────────────────┐
│                     PROTOCOL ADAPTER LAYER                           │
│                    (Implements client Interface)                     │
├──────────────────────────────────────────────────────────────────────┤
│  POLLER   (Active — initiates outbound connection):                  │
│       - Modbus RTU/TCP, OPC UA...                                    │
│       - Actively read/write data from devices.                       │
│                                                                      │
│  LISTENER (Passive — binds and listens on local port):               │
│       - HTTP Webhook, MQTT Sub, UDP...                               │
│       - Listens and receives inbound data.                           │
└──────────────────────────────────────────────────────────────────────┘
```

## How to add a new protocol

当开发者要求新增一种协议时，LLM 应该自动遵循的规则:

1. 判断该协议的开源库哪个实现最好，添加到go mod，并且go get下载相关依赖。
2. 新协议如果是主动的轮询的协议，请实现参考项目内已实现的Modbus协议，参考modbus设备协议配置文件 `/res/devices/modbus.test.devices.json`, 增加新协议相关的设备协议配置的示例。参考modbus设备资源的配置文件示例 `res/profiles/modbus.test.profile.json`， 增加新协议相关的配置的文件示例。这些配置文件并非真实设备，但用于项目测试和模拟运行。
3. 新协议如果是主动的轮询的协议，必须实现的接口定义在文件: `pkg/protocol/connector.go`， type RWClient interface 必须实现。接口实现在目录: `pkg/adapter`
4. 主动协议的适配参考: `pkg/adapter/modbus`， 被动协议的适配参考 `pkg/adapter/listener_http`；如果有被动协议，或者类似snmp。主动协议应该尽可能实现Client的http复用。 trap的被动监听都应该以listener_开头，显示地区别它是一个被动监听的协议适配实现。
5. 所有主动轮询协议的读取设备指令调用都在 `/internal/driver/drive.go`的HandleReadCommands触发，设备写入则由 HandleWriteCommands触发，Edge-X 服务会根据res/devices中设备定义的AutoEvent自动事件周期性触发 `HandleReadCommands`。
6. 实现协议之后应该完成的工作，1. 应该增加该协议的单元测试文件。2. 应该增加该协议的设备模拟器 python编写，参考`simulator/modbus.py`，模拟器应该读取res目录设备配置和profile文件。



## TODO 

单元测试
