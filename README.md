# Edge Gateway

An IoT Edge Gateway based on EdgeX device-sdk-go v2. This service acts as a unified Device Service supporting both Modbus TCP Temperature Sensors and HTTP REST Humidity Sensors within a single instance.

## Project Structure

```
edge-gateway/
├── cmd/
│   └── main.go               # Entry point, invokes startup.Bootstrap
├── driver/
│   ├── composite.go          # Routes requests to sub-drivers based on protocol keys
│   ├── modbus/
│   │   └── driver.go         # Modbus ProtocolDriver implementation
│   └── http/
│       └── driver.go         # HTTP ProtocolDriver implementation
├── internal/
│   └── transform/
│       ├── transform.go      # Modbus byte ↔ value conversion / scaling logic
│       └── transform_test.go
├── res/
│   ├── profiles/
│   │   ├── temperature.yaml  # Device Profile for temperature sensors
│   │   └── humidity.yaml     # Device Profile for humidity sensors
│   ├── devices/
│   │   └── device-list.yaml  # Static device pre-provisioning & AutoEvent config
│   └── configuration.yaml    # Main service configuration
├── Makefile
├── Dockerfile
└── docker-compose.yml        # Complete EdgeX development environment
```

my:

```text
res /
		configuration.yaml
        devices / modbus 
				device.json   定义设备可采集的资源（资源名称，寄存器地址,，值的类型， 默认值，最大值，最小值，描述。）
										
				app-config.yaml  定义设备服务（采名称，描述，协议，采集周期，超时，地址。 等）    "name": "modbus-device1",
						参考： https://github.com/edgexfoundry/device-modbus-go/blob/main/cmd/res/devices/modbus.test.devices.yaml
						"protocol": "modbus",
							"address": "127.0.0.1",
							"port": 5020,
							"slave_id": 1,
							"timeout_ms": 1000,

				poll_interval_seconds: 5   # 采集(轮询)间隔      失败后重试次数
		
internal /
     drivers
                    / modbus        / 放协议
     collector   data_collector  采集的管理， 任务调度
  
pkg   
     models / 放数据模型
     logger
     utils 等
     
cmd      放采集逻辑

```


## Build

### Install the ZeroMQ Development Library

The edgexfoundry/device-sdk-go depends on the C library libzmq. ZeroMQ is a high-performance asynchronous messaging library.

```bash
# Install the ZeroMQ development files and pkg-config
sudo apt-get install libzmq3-dev pkg-config
# Note: If you happen to be using CentOS, RHEL, or Fedora, the command is:
sudo dnf install zeromq-devel pkgconf-pkg-config
```


## Quick Start

### Prerequisites

- Go 1.21+
- Docker & Docker Compose

### 1. Start EdgeX Core Services

Launch the minimal required EdgeX infrastructure:
```bash
docker-compose up -d consul redis core-metadata core-data core-command
```

### 2. Configure Device Addresses

Edit res/devices/device-list.yaml and update the IP addresses to match your physical or simulated hardware:

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
