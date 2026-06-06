An IoT Edge Gateway based on EdgeX device-sdk-go v2.

Under Active Development!
<!-- Project status badge -->
![Status](https://img.shields.io/badge/status-Work_in_Progress-orange)

## Table of Contents

- [Quick Start](#quick-start)
  - [Prerequisites: Start EdgeX Core Services](#prerequisites-start-edgex-core-services) 
  - [Start Device Services](#start-device-services)
- [System Architecture](#system-architecture)
- [Configuration Guide](#configuration-guide)


## Quick Start

### Prerequisites: Start EdgeX Core Services

```bash
# Download or use built-in docker-compose.yml by `docker compose pull`
curl -o docker-compose.yml https://raw.githubusercontent.com/edgexfoundry/edgex-compose/kamakura/docker-compose-no-secty.yml
docker compose pull
docker compose up -d
```

Check Edge-X service by`docker stats` command: 

```text
edgex-core-data         0.03%     10.59MiB / 31.13GiB   0.03%     369kB / 476kB 
edgex-core-command      0.02%     7.754MiB / 31.13GiB   0.02%     73.4kB / 50.3kB 
edgex-core-metadata     0.03%     8.945MiB / 31.13GiB   0.03%     172kB / 172kB   
edgex-redis             0.20%     2.984MiB / 31.13GiB   0.01%     913kB / 565kB   
edgex-device-rest       0.04%     12.29MiB / 31.13GiB   0.04%     111kB / 82.8kB  
edgex-support-scheduler 0.07%     8.516MiB / 31.13GiB   0.03%     88.1kB / 66.3kB  
edgex-core-consul       0.81%     29.68MiB / 31.13GiB   0.09%     606kB / 574kB
edgex-ui-go             0.00%     4.316MiB / 31.13GiB   0.01%     25.6kB / 126B
```

The three backend services `edgex-core-data`, `edgex-core-command`, and `edgex-core-metadata` are essential. Other services depend on your configuration.

Check the backend EdgeX service API: `curl http://localhost:59880/api/v2/ping`
Check Consul (Service register and configuration):  http://localhost:8500
EdgeX UI: http://localhost:4000

### Start Device Services

```shell
git clone git@github.com:crb912/hermes-edge.git
cd hermes-edge
go mod tidy
go build ./cmd/main.go

# Run with insecure mode
export EDGEX_SECURITY_SECRET_STORE=false

# or ./main --overwrite 
./main -o
```

Check the device service has loaded the default Modbus test device:

```bash
# View pre-defined devices 
# you can replace `Modbus-TCP-RTU-test-device` with your actual device
curl http://localhost:59881/api/v2/device/name/Modbus-TCP-RTU-test-device

# View pre-defined profile
curl http://localhost:59881/api/v2/deviceprofile/name/Test-Device-Modbus-Profile

# View device resrouce
curl http://localhost:59881/api/v2/device/name/Test-Device-Modbus-Profile/StringA
```

## System Architecture

```Plaintext
┌───────────────────────────────────────────────────────────────────────────┐
│                            DRIVER CORE LAYER                              │
│        (internal/driver, register interface to UPSTREAM (EdgeX)           │
├───────────────────────────────────────────────────────────────────────────┤
│ - Core driver logic to serve upstream Edge-sdk.                           │
│ - Implements Init, Start, Stop, Device Commands, and Device Events.       │
│ - Injects EdgeX async data channel into lower layers to push data upward. │
└─────────────────────────────────────┌─────────────────────────────────────┘
                                      ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                               CONNECTION LAYER                            │
│                      (Manage Active & Passive Connection Pools)           │
├───────────────────────────────────────────────────────────────────────────┤
│                    ┌──────────────────────────────────────┐               │
│                    │      Connection (Interface)          │               │  
│                    │ - Manages client connection pool.    │               │ 
│                    │ - Defines protocol interface:        │               │ 
│                    │   Connect, Disconnect, Read, Write.  │               │ 
│                    │ - Shared behavior across adapters.   │               │ 
│                    └──────────────────────────────────────┘               │ 
└─────────────────────────────────────┌─────────────────────────────────────┘ 
                                      ▼
┌───────────────────────────────────────────────────────────────────────────┐
│                           PROTOCOL ADAPTER LAYER                          │
│                        (Uniform Protocol Abstraction Interface)           │
├───────────────────────────────────────────────────────────────────────────┤
│  ┌────────────────────────────┐     ┌─────────────────────────────────┐   │
│  │      POLLER (Active)       │     │       RECEIVER (Passive)        │   │
│  │                            │     │                                 │   │
│  │ Handles active protocols:  │     │ Handles passive protocols:      │   │
│  │ - Modbus RTU/TCP, OPC UA   │     │ - HTTP Webhook, MQTT Sub, UDP   │   │
│  │ - Actively pulls data      │     │ - Listener to receive data.     │   │
│  └────────────────────────────┘     └─────────────────────────────────┘   │
└───────────────────────────────────────────────────────────────────────────┘
```

**Architecture Design Highlights:**

- **High Cohesion & Low Coupling**: The architecture strictly separates connection management (Transport layer) from data parsing and protocol behavior (Adapter layer). This provides a highly maintainable and standardized layered design.
- **Maximum Reusability**: By isolating pkg/parser as an independent logic package, both the payloads actively pulled by the poller and the messages passively received by the receiver share the exact same parsing logic. This completely eliminates code dupcliation.
- **Asynchronous Decoupling**: The Core Driver layer injects EdgeX's asynchronous data channels into the lower layers. As a result, the underlying Poller and Receiver only focus on processing and sending data without needing to know the upstream state. This aligns perfectly with Go's channel-based concurrency philosophy.

## Configuration Guide

### How to Configure Device and Profiles

Edit res/devices/*.json and update the IP addresses to match your physical or simulated hardware

**Device Configuration** (`res/devices/`)

[device-sdk-go](https://github.com/edgexfoundry/device-sdk-go) v2 only supports **TOML** or **JSON** format for device configuration files.

> Reference: [device-sdk-go v2.3.0 example devices](https://github.com/edgexfoundry/device-sdk-go/tree/v2.3.0/example/cmd/device-simple/res/devices)

**Profile Configuration** (`res/profiles/`)

v2 only supports **YAML** or **JSON** format for device profile configuration files.

> Reference: [device-sdk-go v2.3.0 example profiles](https://github.com/edgexfoundry/device-sdk-go/tree/v2.3.0/example/cmd/device-simple/res/profiles)

**Recommendation**

It is recommended to use **JSON** for both devices and profiles to keep the configuration format consistent across the project.

## TODO

1. 自动发现
2. 版本更新与make
3. v2版本不支持这个字段，需要检查。 modbus.test.devices.yaml

### 配置文件的格式

每个设备都必须有两个配置文件。
- 一个定义设备的基本属性，比如：名称，使用的协议，事件采集间隔。
- 一个定义设备持有的资源，比如温度传感器，压力，湿度等具体的资源属性，以及这些资源的数据类型，物理意义,映射的map。

配置文件的规范:

```text
/res/devices/
         |------ modbus.test.devices.yaml
         |------ opc.test.device.yaml
res/profiles/
         |------ modbus.test.devices.yaml
```

你可以把所有设备放在同一个devices-list类型的yaml，也可以按协议的分类成不用yaml，也可以按工厂或设备的类型分类，甚至你可以每个设备单独用一个yaml

我推荐的命名方式是：  协议名.分类名.devices.yaml， 这样方便后续维护这些设备。

### 如何生成配置文件？

在设备数量庞大的时候（一个大型公司可能超过上千种设备），用 yaml 文件维护设备和资源的yaml非常不方便，excel却是对实施人员更友好的设计模式，因此
我实现了excel 转 yaml 生成的工具，该工具的代码不会编译进入主项目。您不需要手动调用该工具进行转换，建议在CI/CD 借用流水线调用该工具，将生成的yaml打包进入项目即可。或者部署在启用设备服务之前，先调用转换工具，生成配置文件。 同时是支持python或golang 编译的二进制两种方式。


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