`devices-iot-go-go`(IoT Edge) is a flexible, multi-protocol IoT edge gateway built on [edgexfoundry/edgex-go](https://github.com/edgexfoundry/edgex-go). It enables bi-directional communication (read/write) for southbound devices and supports both active polling and passive data ingestion.

Key Features:
- Multi-Protocol: Out-of-the-box support for Modbus, SNMP, OPC, HTTP, and MQTT.
- Bi-Directional Operations: Supports both reading device resources and writing control commands.
- Dual Ingestion Modes: Supports active scheduling/polling and passive data pushing from devices.
- Configuration-Driven: Flexible read/write operations fully managed via configuration files.
- Highly Extensible: Designed for easy integration of additional standard or proprietary protocols.

## Table of Contents

- [Quick Start](#quick-start)
  - [Prerequisites: Start EdgeX Core Services](#prerequisites-start-edgex-core-services)
  - [Start Device Services](#start-device-services)
  - [Verification and Test Commands](#verification-and-test-commands)
- [System Architecture](#system-architecture)
- [Configuration Guide](#configuration-guide)
- [Documentation](#documentation)
  - [Developer Wiki (English)](docs/wiki-en.md)
  - [开发者 Wiki (中文)](docs/wiki-zh.md)


## Quick Start

### Prerequisites: Start EdgeX Core Services

```bash
# Download docker-compose.yml
curl -o docker-compose.yml https://raw.githubusercontent.com/edgexfoundry/edgex-compose/kamakura/docker-compose-no-secty.yml
#  or, use built-in docker-compose.yml for China Developer

docker compose pull
docker compose up -d
```

Check Edge-X service by`docker stats` command:

```text
edgex-core-data         0.03%     10.59MiB / 31.13GiB   0.03%     369kB / 476kB
edgex-core-command      0.02%     7.754MiB / 31.13GiB   0.02%     73.4kB / 50.3kB
edgex-core-metadata     0.03%     8.945MiB / 31.13GiB   0.03%     172kB / 172kB
edgex-redis             0.20%     2.984MiB / 31.13GiB   0.01%     913kB / 565kB
...
```

Port definitions:

- Port 59880 (Core Data，`edgex-core-data`: Collects, stores, and routes the actual sensor readings coming UP from the devices.
- Port 59881 (Core Metadata, `edgex-core-metadata`): Used only for managing metadata (e.g., creating/updating profiles, adding devices). It does NOT read actual device data.
- Port 59882 (Core Command, `edgex-core-command`): The core microservice port used to send actual Read and Write commands to the devices.

Consul (Service register and configuration):  http://localhost:8500
EdgeX UI: http://localhost:4000

### Start Device Services

```shell
git clone git@github.com:crb912/devices-iot-go.git
cd devices-iot-go
go mod tidy

# Run with dev mode
make dev
```

### Verification and Test Commands

Check the device service has loaded the default test (Modbus) device.

| **Description** | **Command** |
|:---|:---|
| Verify the backend EdgeX service API | `curl http://localhost:59880/api/v2/ping`|
| Verify pre-defined devices<br>*(Note: You can replace `*-test-device` with your actual device)* | `curlhttp://localhost:59881/api/v2/device/name/Modbus-TCP-RTU-test-device`|
| View pre-defined profile | `curl http://localhost:59881/api/v2/deviceprofile/name/Test-Device-Modbus-Profile`|
| View pre-defined device resource<br>*(Note: `isHidden: false`)* | `curl http://localhost:59882/api/v2/device/name/Modbus-TCP-RTU-test-device/ip_address`|
| View log (last 20 lines) | `docker logs edgex-core-command --tail 20`|
| Check the latest events  | `curl http://localhost:59880/api/v2/event/device/name/Modbus-TCP-RTU-test-device?limit=5` |
| Trigger a read command  <br> Assuming the device simulator is already running (python3 ./simulator/modbus.py). | `curl http://localhost:59882/api/v2/device/name/Modbus-TCP-RTU-test-device/Battery-Config`<br>`curl http://localhost:59882/api/v2/device/name/Modbus-TCP-RTU-test-device/System-Time` <br> or `python3 ./scripts/resource_read.py` |
| Trigger a write command  | `python3 ./scripts/resource_write.py |

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

**Architecture Design Highlights:**

- **High Cohesion & Low Coupling**: The architecture strictly separates connection management (Conection layer) from data parsing and protocol behavior (Adapter layer). This provides a highly maintainable and standardized layered design.
- **Maximum Reusability**: By isolating pkg/parser as an independent logic package, both the payloads actively pulled by the poller and the messages passively received by the receiver share the exact same parsing logic. This completely eliminates code dupcliation.
- **Asynchronous Decoupling**: The Core Driver layer injects EdgeX's asynchronous data channels into the lower layers. As a result, the underlying Poller and Receiver only focus on processing and sending data without needing to know the upstream state. This aligns perfectly with Go's channel-based concurrency philosophy.

## Configuration Guide

### How to Configure Device and Profiles

**NOTE: Each device must have two configuration files**.
- One defines the basic properties of the device, such as name, protocol, and data collection interval.
- One defines the device resources, such as specific resource attributes (temperature, pressure, humidity), data types, physical meanings, and mapping rules.

Best Practice for Configuration Files:
```text
/res/devices/
         |------ modbus.test.devices.json
         |------ mqtt.test.device.json
res/profiles/
         |------ modbus.test.profile.json
         |------ mqtt.test.profile.json
res/custom/
         // Some custom configuration files (which you may need to parse manually).
         |------ modbus.test.profile.csv
         |------ mqtt.test.profile.xlsx
```

You can put all devices into a single `JSON/YAML` file. You can also separate them into different JSON/YAML files by protocol or device name.
My recommended naming format is: protocol.device_name.devices.json. This makes it easy to maintain the devices in the future.
It is recommended to use **JSON** for both devices and profiles to keep the configuration format consistent across the project.

**Device Configuration** (`res/devices/`)

[device-sdk-go](https://github.com/edgexfoundry/device-sdk-go) v2 only supports **TOML** or **JSON** format for device configuration files.

> Reference: [device-sdk-go v2.3.0 example devices](https://github.com/edgexfoundry/device-sdk-go/tree/v2.3.0/example/cmd/device-simple/res/devices)

**Profile Configuration** (`res/profiles/`)

v2 only supports **YAML** or **JSON** format for device profile configuration files.

> Reference: [device-sdk-go v2.3.0 example profiles](https://github.com/edgexfoundry/device-sdk-go/tree/v2.3.0/example/cmd/device-simple/res/profiles)

**Custom Configuration** (`res/custom/`)

To make configuration files easier to read and deploy, **using custom XLSX or CSV formats is a very good choice**. Although you can implement custom parsing logic directly in the project code, I strongly advise against it. Instead, you should convert the custom formats (like XLSX or CSV) into project-compatible JSON or YAML formats using a Python script or a standalone Go binary. **Pre-compiling configurations before the program starts is much better than parsing them at runtime**. This approach minimizes project dependencies and keeps the project code simple.

### Update Config

```bash
cd scripts
python profiles_update.py
# Scanning folder: ./res/profiles
# Updating: modbus.test.profile.json
# Status: 207
# Response: [{"apiVersion":"v2","statusCode":200}]
```

## Documentation

For developers who want to understand the internals or write custom protocol adapters:

- **[Developer Wiki (English)](docs/wiki-en.md)** — ProtocolDriver interface deep-dive: Initialize/Stop lifecycle, HandleReadCommands/HandleWriteCommands, AddDevice/UpdateDevice/RemoveDevice patterns.
- **[开发者 Wiki (中文)](docs/wiki-zh.md)** — 中文版，内容相同。





