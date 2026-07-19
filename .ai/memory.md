# NexIoT — Project Memory

> Full context for future LLM sessions. Read this file to restore all project knowledge.

## System Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                   DRIVER CORE LAYER                          │
│  (internal/driver — implements EdgeX Device Service SDK)     │
├──────────────────────────────────────────────────────────────┤
│ - Init, Start, Stop, Device Commands, Device Events          │
│ - processReceiveData: Source-based routing + Profile matching│
│ - loadListeners: config-driven listener registration         │
│ - Forwards data via EdgeX async channel                      │
└────────────────────────────┬─────────────────────────────────┘
                             ▼
┌──────────────────────────────────────────────────────────────┐
│                PROTOCOL INTERFACE LAYER                      │
│          (Manages Active & Passive Connection Pools)         │
├──────────────────────────────────────────────────────────────┤
│ - Interface: Session, Reader, Writer (pkg/protocol/connector)│
│ - Connection pool lifecycle via pkg/protocol/poller.go       │
│ - Listener interface: Start()/Stop() for passive protocols   │
│ - Receivers: manages multiple listeners, shared channel      │
└────────────────────────────┬─────────────────────────────────┘
                             ▼
┌──────────────────────────────────────────────────────────────┐
│               PROTOCOL ADAPTER LAYER                         │
│              (Implements client interfaces)                  │
├──────────────────────────────────────────────────────────────┤
│  POLLER  (Active — outbound connection):                     │
│    Modbus RTU/TCP, SNMP, OPC UA                              │
│  LISTENER (Passive — binds local port / receives push):      │
│    HTTP Webhook, MQTT Broker (embedded), SNMP Traps          │
└──────────────────────────────────────────────────────────────┘
```

## Key File Map

| Path | Purpose |
|---|---|
| `internal/driver/drive.go` | Core: Start/Stop, `processReceiveData` (route), `loadListeners` (config), `lookupProfile`, `lookupDeviceByAddr`, `decodeMqttPayload`, `decodeSnmpTrapPayload`, `buildCommandValues`, `ensureDevice` |
| `pkg/protocol/connector.go` | Interfaces: `Session`, `Reader`, `Writer`, `RWClient`, `Listener` |
| `pkg/protocol/poller.go` | Connection pool + protocol factory (`newClient` switch) |
| `pkg/protocol/listener.go` | Passive listener pool + `RegisterHttpServer`/`RegisterMqttServer`/`RegisterSnmpTrapServer` |
| `pkg/model/protocol_type.go` | `ProtocolType` enum + `ValidateProtocol()` |
| `pkg/model/resource.go` | `Resource`, `ReceiveEvent` (Source/EventName/EventTime/EventData), `ListenerConfig`, `ListenerItem` |
| `pkg/model/protocol_config.go` | `ProtocolConfig` — wraps EdgeX `ProtocolProperties` |
| `pkg/conv/conv.go` | Type conversion helpers (shared across adapters) |
| `pkg/parser/decoder.go` | Custom data decode functions |
| `pkg/parser/encoder.go` | Custom data encode functions |
| `res/custom/listener.json` | **Config-driven listener registration** |
| `res/configuration.toml` | `[SimpleCustom].ListenerConfigPath` |

## Implemented Protocols

| Protocol | Type | Adapter | Simulator | Status |
|---|---|---|---|---|
| Modbus TCP/RTU | Poller | `pkg/adapter/modbus/` | `simulator/modbus.py` | ✅ |
| SNMP v1/v2c/v3 | Poller | `pkg/adapter/snmp/` | `simulator/snmp.py` | ✅ |
| OPC UA | Poller | `pkg/adapter/opcua/` | `simulator/opc.py` | ✅ |
| HTTP Listener | Listener | `pkg/adapter/listener_http/` | `simulator/http_listener.py` | ✅ |
| MQTT Broker | Listener | `pkg/adapter/listener_mqtt/` | `simulator/mqtt.py` | ✅ |
| SNMP Trap | Listener | `pkg/adapter/listener_snmp/` | `simulator/snmp_trap.py` | ✅ |

## RWClient Interface (All Active Protocols Must Implement)

```go
// pkg/protocol/connector.go
type Session interface {
    Connect() error
    Disconnect() error
}
type Reader interface {
    ReadSingle(point *model.Resource) error
    ReadBatch(points []model.Resource) error
}
type Writer interface {
    WriteSingle(point *model.Resource) error
    WriteBatch(points []model.Resource) error
}
type RWClient interface { Session; Reader; Writer }
```

## Listener Interface (All Passive Protocols Must Implement)

```go
// pkg/protocol/connector.go
type Listener interface {
    Start() error
    Stop() error
}
```

## ReceiveEvent (Bridge Between Listeners and Driver)

```go
// pkg/model/resource.go
type ReceiveEvent struct {
    Source    string    // "mqtt" | "http" | "snmp" — used for Source-based routing
    EventName string    // MQTT topic, SNMP "snmp_trap", HTTP event name
    EventTime time.Time
    EventData []byte    // raw payload (JSON for MQTT/SNMP, raw for HTTP)
}
```

## Listener Registration Pattern (Config-Driven)

```jsonc
// res/custom/listener.json — path from configuration.toml [SimpleCustom].ListenerConfigPath
{
  "listeners": [
    { "protocol": "http",  "enabled": true, "host": "0.0.0.0", "port": 8000, "push_url": "/alarm/push" },
    { "protocol": "mqtt",  "enabled": true, "host": "0.0.0.0", "port": 1883 },
    { "protocol": "snmp",  "enabled": true, "host": "0.0.0.0", "port": 1620, "community": "public" }
  ]
}
```

`drive.go:loadListeners()` reads this JSON and calls `RegisterXxxServer()` in a switch.

## Data Flow: Passive Listeners → EdgeX

```
Device push → Listener adapter (Start/Stop) 
    → ReceiveEvent{Source, EventData} 
    → shared channel (cd.receivedData) 
    → processReceiveData() 
    → cd.decodeReceiveEvent() ← Source-based routing
        ├─ "mqtt"  → cd.decodeMqttPayload()
        └─ "snmp"  → cd.decodeSnmpTrapPayload()
            │
            ├─ cd.lookupProfile(deviceName)     ← GetDeviceByName → GetProfileByName
            │  or cd.lookupDeviceByAddr(addr)   ← Devices() iterate + Address match
            │
            ├─ cd.buildCommandValues(profile, data)
            │   └─ resolveFieldType() ← Profile.valueType first, JSON infer fallback
            │       └─ match by name || match by Attributes["address"] (dots↔hyphens)
            │
            └─ cd.ensureDevice() ← dynamic DiscoveredDevice via deviceCh
```

## Important Design Rules

### 1. DO NOT close shared channel in listener Start()
All listeners share `cd.receivedData`. Closing it in one listener breaks all others.
The channel is owned by `CompositeDriver`; goroutine exits via `ctx.Done()`.

### 2. MQTT uses embedded broker (mochi-mqtt), not external broker
- Library: `github.com/mochi-mqtt/server/v2` (v2.7.9)
- OnPublish hook via `HookBase` + `Provides(b byte)` + `OnPublish()`
- No `Events` field in v2.7.x
- Port >1024 required on Linux (non-root)

### 3. SNMP Trap OIDs use hyphen format for EdgeX compatibility
- gosnmp returns `.1.3.6.1...` → strip leading dot + replace dots → `1-3-6-1...`
- Profile `address` stores dots; `resolveFieldType` handles both formats

### 4. Profile field matching order
1. `dr.Name == fieldName` (exact name match)
2. `dr.Attributes["address"]` matches `fieldName` or `strings.ReplaceAll(address, ".", "-")`

### 5. Type conversion: Profile type overrides JSON inference
`resolveFieldType()` checks profile first; falls back to JSON type (float64→Float64, string→String, bool→Bool).
`convertValue()` handles JSON number→Int32, JSON number→Float64, etc.

## Programming Patterns

### 1. Protocol Factory Signature (MUST be identical)

```go
func NewXxxClient(
    endpoint string,
    pt model.ProtocolType,
    defaultTimeout time.Duration,
    args map[string]string,
) (*XxxClient, error)
```

`args` is `ProtocolProperties` from EdgeX device config (`res/devices/*.json`).

### 2. Thread-Safe Connection

All adapters use `sync.Mutex` + `connected bool` pattern. See Modbus or SNMP for reference.

### 3. Registration Checklist (New Active Protocol)

To add a new active protocol:
1. Add constant to `pkg/model/protocol_type.go`
2. Register in `ValidateProtocol()` switch
3. Create adapter in `pkg/adapter/<name>/` implementing `RWClient`
4. Register factory in `pkg/protocol/poller.go` → `newClient()` switch
5. Add conversion helpers to `pkg/conv/conv.go` if needed
6. Create device config example: `res/devices/<name>.test.device.json`
7. Create profile example: `res/profiles/<name>.test.profile.json`
8. Create Python simulator: `simulator/<name>.py` (reads from res/)
9. Write unit tests: `pkg/adapter/<name>/<name>_test.go`

### 4. Registration Checklist (New Passive Listener)

To add a new passive listener:
1. Create adapter in `pkg/adapter/listener_<name>/` implementing `Listener`
2. Add `RegisterXxxServer()` to `pkg/protocol/listener.go`
3. Add logger entry to `res/custom/listener.json`
4. Add `case "xxx"` to `loadListeners()` switch in `drive.go`
5. Add `case "xxx"` to `decodeReceiveEvent()` switch in `drive.go`
6. Add `decodeXxxPayload()` decoder with profile matching
7. Mark `Source: "xxx"` in listener's ReceiveEvent creation
8. Create device config: `res/devices/<name>.test.device.json`
9. Create Python simulator: `simulator/<name>.py`
10. Add unit tests

### 5. Device Config Convention (`res/devices/*.json`)

```json
{
  "protocols": {
    "<protocolName>": {
      "Enabled": "true",
      "Endpoint": "<connection URL>",
      "Timeout": "5",
      "<protocol-specific fields>": ""
    }
  }
}
```

### 6. Profile Convention (`res/profiles/*.json`)

```json
{
  "deviceResources": [{
    "name": "ResourceName",
    "properties": { "valueType": "Float64", "readWrite": "RW", "defaultValue": "0.0" },
    "attributes": { "address": "<protocol-specific address>" }
  }]
}
```

`defaultValue` is a **string** in JSON — simulators must convert it to the correct numeric type.

### 7. Simulator Pattern (Python)

All simulators in `simulator/`:
- Read device config and profile from `res/`
- Use `Path(__file__).resolve().parent.parent` to find project root
- Implement a background update loop for dynamic values
- Use proto-specific Python library or raw sockets

## MQTT Listener Specifics

- Library: `github.com/mochi-mqtt/server/v2` (v2.7.9 — embedded broker)
- Pattern: `HookBase` + `OnPublish` hook for message interception
- Port: 1883 (non-privileged)
- Device config: `res/devices/mqtt.test.device.json` (minimal: just `Enabled: true`)
- Profile: `res/profiles/mqtt.test.profile.json` (Temperature/Humidity/Status)
- Simulator: `simulator/mqtt.py` (paho-mqtt client, publishes to embedded broker)
- Payload format: `{"device_name":"...", "data":{"Temperature":25.1,...}}`
- Field names MUST match profile deviceResource names (case-sensitive, e.g., "Temperature")

## SNMP Trap Listener Specifics

- Library: `github.com/gosnmp/gosnmp` v1.38.0 (TrapListener)
- Port: 1620 (non-privileged; default 162 requires root)
- OID format: dots replaced with hyphens for EdgeX resource name compatibility
- Device matching: by source IP address via `lookupDeviceByAddr()`
- Simulator: `simulator/snmp_trap.py` (raw ASN.1 BER, no pysnmp dependency)
- Uses pre-defined profile `res/profiles/snmp.test.profile.yaml`

## OPC UA Specifics

- Library: `github.com/gopcua/opcua v0.9.0`
- Address format: `"ns=3;i=1001"` (NodeID string)
- `ReadBatch` aggregates all NodeIDs into one `ua.ReadRequest`
- Write uses `client.Write()` with `*ua.WriteValue` (not `Node.Write()` — absent in v0.9.0)
- `Node.Value()` requires `context.Context` argument in v0.9.0
- `CertificateFile` takes single arg; `PrivateKeyFile` is separate
- Simulator uses `asyncua` library to create an OPC UA server

## TODO

- Unit tests for `pkg/adapter/opcua/`
- Unit tests for `pkg/adapter/snmp/`
- Unit tests for `pkg/adapter/listener_mqtt/`
- Unit tests for `pkg/adapter/listener_snmp/`
- HTTP listener decoder (`case "http"` in `decodeReceiveEvent`)
- Profile-driven decoder (use `receive` config in profile for payload structure)
