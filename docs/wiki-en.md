## Project Bootstrap Logic

```text
Bootstrap() Core Flow
│
├── 1. Parse command-line arguments (-p, -cd, -cf, -i, -r, -cp, etc.)
│
├── 2. Load local configuration (configuration.yaml) or fetch from Registry (Consul)
│
├── 3. Register service with Registry (Consul)
│
├── 4. Connect to EdgeX Core Services
│        ├── core-metadata (Devices, Profiles, ProvisionWatchers)
│        └── core-data / Message Bus (Event ingestion & publishing)
│
├── 5. Initialize internal caches (Devices, Profiles, ProvisionWatchers, etc.)
│
├── 6. Invoke driver.Initialize(sdk) ← Custom driver initialization logic
│
├── 7. Start REST API HTTP Server (Device command routing)
│
├── 8. Start AutoEvents engine (Automated scheduling/data collection)
│
├── 9. Invoke driver.Start() ← Custom driver post-initialization logic (New Version)
│
└── 10. Block and wait for shutdown signals (SIGTERM/SIGINT), then trigger driver.Stop()

-------------------------------------------------------------------------

## Introduction to the `ProtocolDriver` Interface
```go
type ProtocolDriver interface {
    Initialize(lc logger.LoggingClient, asyncCh chan<- *AsyncValues, deviceCh chan<- []DiscoveredDevice) error
    Stop(force bool) error

    HandleReadCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []CommandRequest) ([]*CommandValue, error)
    HandleWriteCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []CommandRequest, params []*CommandValue) error

    AddDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error
    UpdateDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error
    RemoveDevice(deviceName string, protocols map[string]models.ProtocolProperties) error
}
```

### Device Initialize/Stop

`Initialize` and `Stop` represent the beginning and end of the Device Service lifecycle. These two callbacks are triggered by the service's own start and stop events. They are scheduled through the underlying `go-mod-bootstrap` framework.

#### Layer 1: External Trigger Sources (Lifecycle Events)

- Startup Phase (Start): When starting the Device Service process via command line or Docker, the service performs a series of initialization operations (loading configurations, connecting to Consul/Redis, synchronizing Core Metadata). After basic dependencies are ready, but before the service officially opens HTTP/MessageBus ports to the outside, `Initialize` is triggered.
- Shutdown Phase (Shutdown): When the process receives an OS interrupt signal (such as SIGINT (Ctrl+C) or SIGTERM (Docker stop)), it triggers a graceful shutdown process, at which point `Stop` is called.

#### Layer 2: Bootstrap and Shutdown Mechanism

EdgeX services use `go-mod-bootstrap` to manage lifecycles. In `device-sdk-go`, the lifecycle mounting points are usually in `pkg/service/init.go` or internal bootstrap handlers:
- Initialization Route: The Bootstrap framework executes a series of `BootstrapHandlers` in order. In the specific handler for the Device Service, it retrieves the ProtocolDriver from the DI (Dependency Injection) container and calls its `Initialize` method.
- Shutdown Route: The Bootstrap framework listens for OS signals in the background. Upon detecting a shutdown signal, it traverses the registered `wg.Wait()` and `ctx.Done()` in reverse order, calling `driver.Stop()`.

#### Layer 3: Application Layer — Initialize Interface
`Initialize` marks that the SDK is ready and hands control to the user-written Driver for low-level protocol initialization.

```go
Initialize(lc logger.LoggingClient, asyncCh chan<- *AsyncValues, deviceCh chan<- []DiscoveredDevice) error
````

This is the most important entry point when writing a Driver. The roles of these three parameters are as follows:

1.`lc logger.LoggingClient`:
- Role: The logging client passed from the SDK to the Driver.
- Implementation Suggestion: The Driver should save this lc into its own struct. All subsequent log outputs (like  `lc.Debugf(...))` should use it. This ensures a unified log format and allows dynamic control by the log level in Consul.

2.`asyncCh chan<- *AsyncValues` (Asynchronous Read Channel):
- Role: Used to handle data actively reported by devices. Examples include messages received from MQTT subscriptions, device heartbeats received by a TCP Server, or Notify alerts from BLE devices.
- Implementation Suggestion: If your protocol is asynchronous, you should start a Goroutine within Initialize to maintain a long-lived connection listening for device data. Upon receiving data, package it into AsyncValues and push it to this asyncCh. The SDK's core engine will consume this channel on the other end and send the data to Core Data.

3.`deviceCh chan<- []DiscoveredDevice` (Device Discovery Channel):
- Role: Used for the Auto-Discovery mechanism.
- Implementation Suggestion: If the service is configured with `Device.Discovery.Enabled = true`, you can push unknown devices found via network scanning to this channel. The SDK will automatically register these devices in Core Metadata upon receipt. If auto-discovery is not used, this channel can be safely ignored.

#### Key Points to Note
1. Blocking Issues: `Initialize `must never block. If you need to establish a potentially time-consuming network connection or start an infinite loop listening service (like TCP Listen) at startup, you must use the go keyword to start a new Goroutine to run in the background. If Initialize blocks, the entire Device Service startup process will freeze.
2. Error Handling: If `Initialize` returns an `error`, `go-mod-bootstrap` assumes initialization failed and directly triggers `os.Exit` to terminate the entire Device Service process. Therefore, you should only return an error when a fatal, unrecoverable error occurs (like a missing critical certificate). If it is a temporary network disconnection, it is recommended to implement retry logic in a background Goroutine, while `Initialize` itself returns `nil`.
3. Lifecycle Sequence: When the service starts, the SDK will first build local device and Profile caches before calling `Initialize`. Thus, at the moment `Initialize` runs, the Driver can use SDK APIs to query the existing device list (this is very useful for restoring previous connection states at the protocol layer).

--------------------------------------

### Device Service Read & Write Command Processing Mechanism

`HandleReadCommands` and `HandleWriteCommands` are the core components of the Device Service **Data Plane**. Unlike lifecycle callbacks responsible for device lifecycle management, these two interfaces are exclusively used for synchronous sensor data collection (**GET**) and device control operations (**PUT**).

#### Tier 1: External Trigger Sources (Two Primary Paths)
Read/write commands are generally not initiated by the Device Service itself, but originate from the two sources below:

- **Path A — Initiated by External Microservices / Clients (On-Demand Read & Write)**
  External applications (such as application services, UI dashboards or other clients) send GET/PUT requests by calling REST APIs of the `core-command` microservice. After locating the matching Device Service, `core-command` forwards the requests to the command APIs of the target Device Service:

```text
GET /api/v2/device/name/{name}/{command}  → Trigger HandleReadCommands
PUT /api/v2/device/name/{name}/{command}  → Trigger HandleWriteCommands
```

- **Path B — Initiated by Internal Scheduled Tasks (AutoEvents)**
  This relies on the `AutoEvents` mechanism defined in Device Profile configurations. The internal scheduler of Device Service periodically triggers read operations targeting specific resources at pre-configured intervals (e.g., `Interval: "10s"`), which will eventually invoke `HandleReadCommands`.

#### Tier 2: HTTP Routing & SDK Core Processing Layer

In `internal/controller/http/command.go`, the SDK listens to the HTTP requests mentioned above. Once a request is received, the logic flows into `internal/application/command.go` (the Application layer).

This layer undertakes heavy preprocessing work:
1. Resource Validation: Check whether the requested `deviceName` and `command` exist in cached Device Profile data.
2. Protocol Parameter Assembly: Extract device connection information (`protocols`), such as IP address, port number, etc.
3. Attribute Mapping: Package `deviceResource` defined in the Profile and underlying protocol-specific `attributes` (e.g., Modbus register addresses, MQTT Topics) into an array of `CommandRequest`.

Only after preprocessing finishes will the SDK pass the structured data to the underlying protocol interface.

#### Tier 3: Application Layer — `HandleReadCommands` Interface

The SDK calls this interface upon receiving a request to read device data.

```go
HandleReadCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []models.CommandRequest) ([]*models.CommandValue, error)
```
Input Parameter Explanation:
- `deviceName`: Name of the target device.
- `protocols`: Physical connection information of the device (e.g., IP address, baud rate). This data is usually processed during the Initialize phase for persistent connection protocols; it is mainly used for short-connection protocols like HTTP REST devices.
- `reqs []models.CommandRequest`: An array type. EdgeX allows combining multiple deviceResources into one logical `deviceCommand`. This array records which specific sensors to read, corresponding data types, and low-level device attributes (`req.Attributes` defined by users in Profile, for example `{ "primaryTable": "HOLDING_REGISTERS", "startingAddress": "10" }`).

Return Value:
An array `[]*models.CommandValue` with the exact same length as input `reqs` must be returned. Each element encapsulates the retrieved raw data, together with timestamp and data type labels.

Code Example (Driver Implementation Logic):

```go
func (d *MyDriver) HandleReadCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []models.CommandRequest) ([]*models.CommandValue, error) {
    // Create an array to hold the responses
    responses := make([]*models.CommandValue, len(reqs))
    
    // Process each read request in the batch
    for i, req := range reqs {
        // 1. Get protocol specific attributes (e.g., register address)
        address := req.Attributes["address"].(string)
        
        // 2. Read data from the physical device
        rawData, err := d.readFromDevice(address)
        if err != nil {
             // Return error if hardware read fails
            return nil, err
        }
        
        // 3. Wrap the raw data into an EdgeX CommandValue based on expected value type
        cv, err := models.NewCommandValue(req.DeviceResourceName, req.Type, rawData)
        if err != nil {
            return nil, err
        }
        
        // 4. Store the result
        responses[i] = cv
    }
    
    return responses, nil
}
```

#### Key Points to Note

- **Synchronous & Blocking Execution**: These two interfaces are invoked synchronously. The HTTP request will remain blocked until the functions return. Time-consuming retry or sleep logic implemented here will lead to client request timeouts (the default SDK timeout is generally 15–20 seconds).
- **Concurrent Thread Safety**: If multiple clients send read/write requests to the same device simultaneously, the SDK will concurrently invoke your `HandleReadCommands` or `HandleWriteCommands`. If your underlying communication library (e.g., certain serial port libraries) does not support concurrent operations, you must add locks with sync.Mutex inside the Driver to avoid data interference.
- **Batch Processing Requirements**: EdgeX strongly recommends Drivers to merge requests as much as possible. For example, if `reqs` contains three consecutive register read requests, a well-designed Driver implementation will not send three separate physical network requests. Instead, it merges them into one bulk read request, parses the returned data and fills each entry into `responses` separately, which can greatly boost overall performance.

--------------------------------------

### Analysis of the Device Add/Update/Remove Callback Mechanism

In v2.3.0, these three `driver` callbacks are triggered by `core-metadata` sending HTTP Callbacks to the Device Service, rather than the Device Service actively polling.

#### Layer 1: External Trigger Sources (Two Paths)

**Path A — Runtime Operations**: When external entities (UI, API clients, other services) add, delete, or modify devices by calling the `core-metadata` REST API, `core-metadata` finds the Device Service that the device belongs to and sends an HTTP callback to it:

```Plaintext
POST   /api/v2/callback/device                → AddDevice
PUT    /api/v2/callback/device                → UpdateDevice
DELETE /api/v2/callback/device/name/{name}    → DeleteDevice
```

**Path B — Provisioning Devices at Startup**: The `BootstrapHandler` calls `provision.LoadDevices()`. For predefined devices that do not yet exist in metadata, it calls `dc.Add(ctx, addDevicesReq)` to register the device with `core-metadata`. The latter then triggers an HTTP callback to the Device Service, ultimately following the same path.

#### Layer 2: HTTP Route Registration

The `BootstrapHandler()` in `pkg/service/init.go` calls `ds.controller.InitRestRoutes()`. This function, located in internal/controller/http/restrouter.go, binds HTTP methods and paths to three handlers in the `RestController`:

```go
c.addReservedRoute(common.ApiDeviceCallbackRoute, c.AddDevice).Methods(http.MethodPost)
c.addReservedRoute(common.ApiDeviceCallbackRoute, c.UpdateDevice).Methods(http.MethodPut)
c.addReservedRoute(common.ApiDeviceCallbackNameRoute, c.DeleteDevice).Methods(http.MethodDelete)
```

#### Layer 3: HTTP Handler (`internal/controller/http/callback.go`)

This layer is only responsible for I/O: decoding the JSON Body, passing the DTO to the application layer, and then serializing the result into an HTTP response. It contains no business logic.

#### Layer 4: Application Layer (`internal/application/callback.go`) — The Core
This is where the real business logic happens. Taking `AddDevice` as an example (source code lines 42–70):

```go
func AddDevice(addDeviceRequest requests.AddDeviceRequest, dic *di.Container) errors.EdgeX {
device := dtos.ToDeviceModel(addDeviceRequest.Device)

    // 1. Fetch from metadata and update the profile cache
    edgexErr = updateAssociatedProfile(device.ProfileName, dic)
    
    // 2. Write to the local device cache
    edgexErr = cache.Devices().Add(device)
    
    // 3. Retrieve the driver from the DI container and call the callback
    driver := container.ProtocolDriverFrom(dic.Get)
    err := driver.AddDevice(device.Name, device.Protocols, device.AdminState)
    
    // 4. Restart AutoEvents for this device
    container.ManagerFrom(dic.Get).RestartForDevice(device.Name)
}
```

`UpdateDevice` and `DeleteDevice` have perfectly symmetrical structures, except that step 3 calls `driver.UpdateDevice()` and `driver.RemoveDevice()` respectively.

#### Layer 5: DI Container + ProtocolDriver Interface
`container.ProtocolDriverFrom(dic.Get)` retrieves the user-registered `ProtocolDriver` instance from the dependency injection container. The interface is defined in `pkg/models/protocoldriver.go`:

```Go
type ProtocolDriver interface {
AddDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error
UpdateDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error
RemoveDevice(deviceName string, protocols map[string]models.ProtocolProperties) error
// ... Initialize, HandleReadCommands, HandleWriteCommands, Stop
}

```
The user (e.g., `device-mqtt-go`) implements this interface. Inside `AddDevice`, it subscribes to MQTT topics and establishes connections; inside `RemoveDevice`, it disconnects and cleans up resources.

#### Key Points to Note

When the service starts, `cache.InitCache()` pulls all existing devices under this service from `core-metadata` and puts them into the cache. This process does not call `driver.AddDevice()`. Only actual add, update, or delete device operations (through the HTTP callback chain described above) will trigger these three callbacks.

--------------------------------------
