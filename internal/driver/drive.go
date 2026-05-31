// Package driver internal/driver/composite.go
package driver

import (
	"context"
	"errors"
	"fmt"
	"hermes-edge/pkg/adapter"
	"hermes-edge/pkg/transport"
	"reflect"
	"strconv"
	"time"

	"github.com/edgexfoundry/device-sdk-go/v2/example/config"
	"github.com/edgexfoundry/device-sdk-go/v2/pkg/interfaces"
	sdkModels "github.com/edgexfoundry/device-sdk-go/v2/pkg/models"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/clients/logger"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/common"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/models"
	gometrics "github.com/rcrowley/go-metrics"
)

const readCommandsExecutedName = "ReadCommandsExecuted"

const (
	ProtocolKeyModbus = "modbus"
	ProtocolKeyHTTP   = "http"
)

// CompositeDriver 实现 EdgeX ProtocolDriver 接口。
type CompositeDriver struct {
	lc                   logger.LoggingClient
	asyncCh              chan<- *sdkModels.AsyncValues       // send adta
	dataCh               chan *adapter.AsyncData             // Bridge channel between Receivers and EdgeX
	deviceCh             chan<- []sdkModels.DiscoveredDevice // add device
	ctx                  context.Context
	cancel               context.CancelFunc
	counter              interface{}
	stringArray          []string
	readCommandsExecuted gometrics.Counter
	serviceConfig        *config.ServiceConfig // user defined config
	polls                transport.Polls
	receivers            *transport.Receivers
}

func (cd *CompositeDriver) Initialize(lc logger.LoggingClient, asyncCh chan<- *sdkModels.AsyncValues, deviceCh chan<- []sdkModels.DiscoveredDevice) error {
	cd.lc = lc
	cd.asyncCh = asyncCh
	cd.deviceCh = deviceCh
	cd.ctx, cd.cancel = context.WithCancel(context.Background())

	// Initialize channel with buffer size based on expected concurrency
	cd.dataCh = make(chan *adapter.AsyncData, 100)

	ds := interfaces.Service()
	// Log the service version
	cd.lc.Infof("Starting %s: Version %s", ds.Name(), ds.Version())

	cd.serviceConfig = &config.ServiceConfig{}
	cd.counter = map[string]interface{}{
		"f1": "ABC",
		"f2": 123,
	}
	cd.stringArray = []string{"foo", "bar"}

	if err := ds.LoadCustomConfig(cd.serviceConfig, "SimpleCustom"); err != nil {
		return fmt.Errorf("unable to load 'SimpleCustom' custom configuration: %s", err.Error())
	}
	lc.Infof("Custom config is: %v", cd.serviceConfig.SimpleCustom)

	if err := cd.serviceConfig.SimpleCustom.Validate(); err != nil {
		return fmt.Errorf("'SimpleCustom' custom configuration validation failed: %s", err.Error())
	}
	// dynamic configuration hot update
	if err := ds.ListenForCustomConfigChanges(
		&cd.serviceConfig.SimpleCustom.Writable,
		"SimpleCustom/Writable", cd.ProcessCustomConfigChanges); err != nil {
		return fmt.Errorf("unable to listen for changes for 'SimpleCustom.Writable' custom configuration: %s", err.Error())
	}
	// Setup metrics
	if err := cd.initMetrics(ds); err != nil {
		cd.lc.Errorf("Failed to initialize metrics: %v", err)
	}
	cd.lc.Info("Driver initialized")
	return nil

}

// Initialize all observability metrics for the driver
// 初始化轻量的可观测性系统，观测边缘微服务的健康状态和运行性能
func (cd *CompositeDriver) initMetrics(sdk interfaces.DeviceServiceSDK) error {
	cd.readCommandsExecuted = gometrics.NewCounter()

	var err error
	metricsManger := sdk.GetMetricsManager()
	if metricsManger != nil {
		// Register the counter metric for read commands
		err = metricsManger.Register(readCommandsExecutedName, cd.readCommandsExecuted, nil)
	} else {
		err = errors.New("metrics manager not available")
	}

	if err != nil {
		return fmt.Errorf("unable to register metric %s: %s", readCommandsExecutedName, err.Error())
	}
	cd.lc.Infof("Registered %s metric for collection when enabled", readCommandsExecutedName)
	return nil
}

func (cd *CompositeDriver) Start() error {
	cd.lc.Info("Driver Start called")

	transport.NewPolls(
		transport.WithMaxCounts(30),
		transport.WithTimeout(5*time.Second))
	cd.lc.Info("Polls Started")

	//  Start the internal consumer goroutine
	go cd.processReceiveData()
	cd.receivers = transport.NewReceivers(":8080")

	// Start all receivers, passing the internal channel to them
	if err := cd.receivers.StartAll(cd.ctx, cd.dataCh); err != nil {
		cd.lc.Errorf("Failed to start the receiver: %v", err)
	}

	cd.lc.Info("Receivers Started")

	return nil
}

// Stop the protocol-specific DS code to shut down gracefully, or
// if the force parameter is 'true', immediately. The driver is responsible
// for closing any in-use channels, including the channel used to send async
// readings (if supported).
func (cd *CompositeDriver) Stop(force bool) error {
	if cd.lc != nil {
		cd.lc.Debugf("Driver Stop called: force=%v", force)
	}
	// Notify all background goroutines to exit
	cd.cancel()
	// Stop receivers one by one

	err := cd.receivers.StopAll()
	if err != nil {
		cd.lc.Errorf("Driver.Stop err: %v", err)
	}

	return nil
}

// --------------------------------------------------------------------------
//  Handle Commands:  HandleReadCommands, HandleWriteCommands
// --------------------------------------------------------------------------

// HandleReadCommands triggers a protocol Read operation for the specified device.
func (cd *CompositeDriver) HandleReadCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []sdkModels.CommandRequest) (res []*sdkModels.CommandValue, err error) {
	cd.lc.Debugf("SimpleDriver.HandleReadCommands: protocols: %v resource: %v attributes: %v", protocols, reqs[0].DeviceResourceName, reqs[0].Attributes)
	res = make([]*sdkModels.CommandValue, len(reqs))
	for i, r := range reqs {
		cd.lc.Infof("#### Read Commands Executed, Device: %v, Resource: %v", deviceName, r.DeviceResourceName)
		var cv *sdkModels.CommandValue
		switch r.DeviceResourceName {
		case "Xrotation":
			cv, _ = sdkModels.NewCommandValue(r.DeviceResourceName, common.ValueTypeInt32, 111)
		case "Yrotation":
			cv, _ = sdkModels.NewCommandValue(r.DeviceResourceName, common.ValueTypeInt32, 111)
		}
		res[i] = cv
	}

	cd.readCommandsExecuted.Inc(1)

	return
}

// HandleWriteCommands passes a slice of CommandRequest struct each representing
// a ResourceOperation for a specific device resource.
// Since the commands are actuation commands, params provide parameters for the individual
// command.
// HandleWriteCommands 传入一个 CommandRequest 结构体切片，
// 每个结构体对应一个特定设备资源的资源操作（ResourceOperation）。
// 由于这些命令属于执行/驱动类命令，params 为单个命令提供参数。
func (cd *CompositeDriver) HandleWriteCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []sdkModels.CommandRequest,
	params []*sdkModels.CommandValue) error {

	for index, r := range reqs {
		cd.lc.Debugf("Driver HandleWriteCommands: protocols: %v, resource: %v, parameters: %v, attributes: %v", protocols, reqs[index].DeviceResourceName, params[index], reqs[index].Attributes)
		cd.lc.Infof("Please write data value (%s) to resource (%s) device(%s)", params[index].Value, r.DeviceResourceName, deviceName)
	}
	return nil
}

//   Device Event: AddDevice, UpdateDevice, Discover

// AddDevice is a callback function that is invoked
// when a new Device associated with this Device Service is added
func (cd *CompositeDriver) AddDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error {
	cd.lc.Debugf("a new Device is added: %s", deviceName)
	return nil
}

// UpdateDevice is a callback function that is invoked
// when a Device associated with this Device Service is updated
func (cd *CompositeDriver) UpdateDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error {
	cd.lc.Debugf("Device %s is updated", deviceName)
	return nil
}

// RemoveDevice is a callback function that is invoked
// when a Device associated with this Device Service is removed
func (cd *CompositeDriver) RemoveDevice(deviceName string, protocols map[string]models.ProtocolProperties) error {
	cd.lc.Debugf("Device %s is removed", deviceName)
	return nil
}

// Discover triggers protocol specific device discovery, which is an asynchronous operation.
// Devices found as part of this discovery operation are written to the channel devices.
// Discover 触发协议相关的设备发现，这是一个异步操作。
// 本次发现过程中找到的设备，会写入到 devices 通道中。
func (cd *CompositeDriver) Discover() {
	proto := make(map[string]models.ProtocolProperties)
	proto["other"] = map[string]string{"Address": "simple02", "Port": "301"}

	device2 := sdkModels.DiscoveredDevice{
		Name:        "Simple-Device-for-test",
		Protocols:   proto,
		Description: "found by discovery",
		Labels:      []string{"auto-discovery"},
	}

	proto = make(map[string]models.ProtocolProperties)
	proto["other"] = map[string]string{"Address": "simple03", "Port": "399"}

	device3 := sdkModels.DiscoveredDevice{
		Name:        "Simple-Device03",
		Protocols:   proto,
		Description: "found by discovery",
		Labels:      []string{"auto-discovery"},
	}

	res := []sdkModels.DiscoveredDevice{device2, device3}

	time.Sleep(time.Duration(cd.serviceConfig.SimpleCustom.Writable.DiscoverSleepDurationSecs) * time.Second)
	cd.deviceCh <- res
}

// processReceiveData reads from dataCh and pushes to EdgeX
func (cd *CompositeDriver) processReceiveData() {
	for {
		select {
		case <-cd.ctx.Done():
			// Exit loop when Driver stops
			return
		case data := <-cd.dataCh:
			// Convert AsyncData to EdgeX CommandValue
			cv, err := sdkModels.NewCommandValue(data.ResourceName, common.ValueTypeFloat64, data.Value)
			if err != nil {
				cd.lc.Errorf("Failed to create CommandValue: %v", err)
				continue
			}

			// Wrap into EdgeX AsyncValues structure
			asyncVal := &sdkModels.AsyncValues{
				DeviceName:    data.DeviceName,
				CommandValues: []*sdkModels.CommandValue{cv},
			}

			// Push to EdgeX core
			cd.asyncCh <- asyncVal
		}
	}
}

// ProcessCustomConfigChanges ...hot-reload configuration
// 配置热更新，不重启加载配置。
func (cd *CompositeDriver) ProcessCustomConfigChanges(rawWritableConfig interface{}) {
	updated, ok := rawWritableConfig.(*config.SimpleWritable)
	if !ok {
		cd.lc.Error("unable to process custom config updates: Can not cast raw config to type 'SimpleWritable'")
		return
	}

	cd.lc.Info("Received configuration updates for 'SimpleCustom.Writable' section")

	previous := cd.serviceConfig.SimpleCustom.Writable
	cd.serviceConfig.SimpleCustom.Writable = *updated

	if reflect.DeepEqual(previous, *updated) {
		cd.lc.Info("No changes detected")
		return
	}

	// Now check to determine what changed.
	// In this example we only have the one writable setting,
	// so the check is not really need but left here as an example.
	// Since this setting is pulled from configuration each time it is need, no extra processing is required.
	// This may not be true for all settings, such as external host connection info, which
	// may require re-establishing the connection to the external host for example.
	if previous.DiscoverSleepDurationSecs != updated.DiscoverSleepDurationSecs {
		cd.lc.Infof("DiscoverSleepDurationSecs changed to: %d", updated.DiscoverSleepDurationSecs)
	}
}

func (cd *CompositeDriver) ValidateDevice(device models.Device) error {
	protocol, ok := device.Protocols["other"]
	if !ok {
		return errors.New("missing 'other' protocols")
	}

	addr, ok := protocol["Address"]
	if !ok {
		return errors.New("missing 'Address' information")
	} else if addr == "" {
		return errors.New("address must not empty")
	}

	port, ok := protocol["Port"]
	if !ok {
		return errors.New("missing 'Port' information")
	} else if _, err := strconv.Atoi(port); err != nil {
		return errors.New("port must be a number")
	}

	return nil
}
