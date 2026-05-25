An IoT Edge Gateway based on EdgeX device-sdk-go v2.

Under Active Development!
<!-- Project status badge -->
![Status](https://img.shields.io/badge/status-Work_in_Progress-orange)

## Quick Start

### 1. Prerequisites

- Go 1.21+
- Docker & Docker Compose

### 2. Start EdgeX Core Services
Launch the minimal required EdgeX infrastructure:
```bash
docker-compose up -d consul redis core-metadata core-data core-command
```

### 2. Start EdgeX Core Services

```shell
git clone git@github.com:crb912/hermes-edge.git
```
### 3. Configure Device Addresses

Edit res/devices/device-list.yaml and update the IP addresses to match your physical or simulated hardware
## System Architecture
```Plaintext
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                                 UPSTREAM (EdgeX / Edge-sdk)                                 │
└──────────────────────────────────────────────┌──────────────────────────────────────────────┘
                                               ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                                    1. DRIVER CORE LAYER                                     │
│                                 (internal/driver/drive.go)                                  │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│ - Core driver logic to serve upstream Edge-sdk.                                             │
│ - Implements Init, Start, Stop, Device Commands, and Device Events.                         │
│ - Injects EdgeX async data channel into lower layers to push data upward.                   │
└──────────────────────────────────────────────┌──────────────────────────────────────────────┘
                                               ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                                2. TRANSPORT CONNECTION LAYER                                │
│                           (Manage Active & Passive Connection Pools)                        │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│                       ┌──────────────────────────────────────┐                              │
│                       │      Transport (Interface)           │                              │  
│                       │ - Manages client connection pool.    │                              │ 
│                       │ - Defines protocol interface:        │                              │ 
│                       │   Connect, Disconnect, Read, Write.  │                              │ 
│                       │ - Shared behavior across adapters.   │                              │ 
│                       └──────────────────────────────────────┘                              │ 
└──────────────────────────────────────────────┌──────────────────────────────────────────────┘ 
                                               ▼
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│                                 3. PROTOCOL ADAPTER LAYER                                   │
│                        (Uniform Connection Interface & Payload Parsing)                     │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│    ┌──────────────────────────────────────┐     ┌──────────────────────────────────────┐    │
│    │           POLLER (Active)            │     │          RECEIVER (Passive)          │    │
│    │                                      │     │                                      │    │
│    │ Handles active protocols:            │     │ Handles passive protocols:           │    │
│    │ - Modbus RTU/TCP, OPC UA             │     │ - HTTP Webhook, MQTT Sub, UDP        │    │
│    │ - Actively pulls data from devices.  │     │ - Starts listener to receive data.   │    │
│    └──────────────────────────────────────┘     └──────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

**Architecture Design Highlights:**

- **High Cohesion & Low Coupling**: The architecture strictly separates connection management (Transport layer) from data parsing and protocol behavior (Adapter layer). This provides a highly maintainable and standardized layered design.
- **Maximum Reusability**: By isolating pkg/parser as an independent logic package, both the payloads actively pulled by the poller and the messages passively received by the receiver share the exact same parsing logic. This completely eliminates code dupcliation.
- **Asynchronous Decoupling**: The Core Driver layer injects EdgeX's asynchronous data channels into the lower layers. As a result, the underlying Poller and Receiver only focus on processing and sending data without needing to know the upstream state. This aligns perfectly with Go's channel-based concurrency philosophy.

## Study

edge-sdk-go interface: https://pkg.go.dev/github.com/edgexfoundry/device-sdk-go/v2/pkg/interfaces

对于非标准格式的配置，如何接入



## Quick Start


- 



### :

```yaml
# Modbus TCP Temperature Sensor
Address: "192.168.1.10:502"

# HTTP Humidity Sensor
BaseURL: "http://192.168.1.20:8080"
```

### 3. Run Locally

```bash
make run
```

### 4. Verify Data Acquisition
Check the latest events:

```bash
# Check the latest events:
curl http://localhost:59880/api/v2/event/device/name/temperature-sensor-01?limit=5

# Trigger an on-demand read command:
curl http://localhost:59882/api/v2/device/name/temperature-sensor-01/command/readTemperature
```

## Configuration Guide

### Adding a New Modbus Device

Append the following to `res/devices/device-list.yaml`：

```yaml
- name: "temperature-sensor-02"
  profileName: "temperature-sensor"
  autoEvents:
    - interval: "5s"
      onChange: false
      sourceName: "readTemperature"
  protocols:
    modbus:
      Address: "192.168.1.11:502"
      SlaveID: "2"
```

## Protocol Driver Specifications

### Modbus Driver Attributes

| Protocol Attribute          | Description                              | Example              |
|-------------|-----------------------------------|-------------------|
| `Address`   | Modbus TCP Server Address:Port         | `192.168.1.10:502`|
| `SlaveID`   | Slave Unit Identifier (1–247)                  | `1`               |

### Modbus Resource Attribute

| Resource Attribute         | Resource Attribute    | Default     |
|-------------------|-----------------------|-----------|
| `modbusFunction`  | `holding` / `input`   | `holding` |
| `modbusAddress`   | Register offset (Decimal)） | `0`        |
| `modbusDataType`  | `float32/int16/uint16/int32/uint32` | `float32`|
| `scale`           | Value scaling factor           | `1.0`      |

### HTTP Driver Attributes

| Protocol Attribute      | Description                     | Example                   |
|---------|-----------------------------|-----------------------------|
| `BaseURL` | Root URL of the device HTTP service       | `http://192.168.1.20:8080` |

### Http Resource Attribute

| Resource Attribute        | Description                | Example         |
|-------------|--------------------|-----------------|
| `httpMethod`  | HTTP Verb          | `GET`           |
| `httpPath`    | API Request Path               | `/api/humidity` |
| `jsonPath`    | Dot notation to extract value from JSON | `data.humidity` |
| `scale`       | Value scaling factor            | `1.0`           |

## Testing

```bash
# Run unit tests
make test
# Generate HTML coverage report
make test-cover
```

## Production Deployment

```bash
# Build the Docker image
make docker
# Tag and push to your registry
docker tag edge-gateway:dev registry.example.com/edge-gateway:1.0.0
docker push registry.example.com/edge-gateway:1.0.0
```


Bootstrap() 调用
│
├── 1. 解析命令行参数（-p, -cd, -cf, -i, -r, -cp 等）
│
├── 2. 加载配置文件（configuration.yaml）或从 Consul 拉取配置
│
├── 3. 向 Registry（Consul）注册本服务
│
├── 4. 连接 EdgeX 核心服务
│        ├── core-metadata（设备、Profile、ProvisionWatcher）
│        └── core-data / Message Bus（事件上报）
│
├── 5. 初始化缓存（设备、Profile、ProvisionWatcher 等）
│
├── 6. 调用 driver.Initialize(sdk) ← 用户自定义初始化逻辑
│
├── 7. 启动 REST API HTTP Server（设备命令路由）
│
├── 8. 启动 AutoEvents（自动采集事件）
│
├── 9. 调用 driver.Start() ← 用户初始化后置逻辑（新版本）
│
└── 10. 阻塞等待关闭信号（SIGTERM/SIGINT），触发 driver.Stop()