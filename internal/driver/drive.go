// Package driver internal/driver/composite.go
package driver

import (
	"errors"
	"fmt"
	"reflect"
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

// CompositeDriver 持有所有子驱动，实现 EdgeX ProtocolDriver 接口。
type CompositeDriver struct {
	lc                   logger.LoggingClient
	asyncCh              chan<- *sdkModels.AsyncValues       // send adta
	deviceCh             chan<- []sdkModels.DiscoveredDevice // add device
	counter              interface{}
	stringArray          []string
	readCommandsExecuted gometrics.Counter
	serviceConfig        *config.ServiceConfig // user defined config
}

// --------------------------------------------------------------------------
//  		| ProtocolDriver -- Lifespan management |
//          Initialize, Star, Stop
// --------------------------------------------------------------------------

func (c *CompositeDriver) Initialize(lc logger.LoggingClient, asyncCh chan<- *sdkModels.AsyncValues, deviceCh chan<- []sdkModels.DiscoveredDevice) error {
	c.lc = lc
	c.asyncCh = asyncCh
	c.deviceCh = deviceCh
	ds := interfaces.Service()

	// Log the service version
	c.lc.Infof("Starting %s: Version %s", ds.Name(), ds.Version())

	c.serviceConfig = &config.ServiceConfig{}
	c.counter = map[string]interface{}{
		"f1": "ABC",
		"f2": 123,
	}
	c.stringArray = []string{"foo", "bar"}

	if err := ds.LoadCustomConfig(c.serviceConfig, "SimpleCustom"); err != nil {
		return fmt.Errorf("unable to load 'SimpleCustom' custom configuration: %s", err.Error())
	}
	lc.Infof("Custom config is: %v", c.serviceConfig.SimpleCustom)

	if err := c.serviceConfig.SimpleCustom.Validate(); err != nil {
		return fmt.Errorf("'SimpleCustom' custom configuration validation failed: %s", err.Error())
	}

	// dynamic configuration hot update
	if err := ds.ListenForCustomConfigChanges(
		&c.serviceConfig.SimpleCustom.Writable,
		"SimpleCustom/Writable", c.ProcessCustomConfigChanges); err != nil {
		return fmt.Errorf("unable to listen for changes for 'SimpleCustom.Writable' custom configuration: %s", err.Error())
	}
	// Setup metrics
	if err := c.initMetrics(); err != nil {
		c.lc.Errorf("Failed to initialize metrics: %v", err)
	}
	c.lc.Info("Driver initialized")
	return nil

}

func (c *CompositeDriver) Start() error {
	c.lc.Info("Driver Started")
	return nil
}

// Stop the protocol-specific DS code to shut down gracefully, or
// if the force parameter is 'true', immediately. The driver is responsible
// for closing any in-use channels, including the channel used to send async
// readings (if supported).
func (c *CompositeDriver) Stop(force bool) error {
	if c.lc != nil {
		c.lc.Debugf("Driver.Stop called: force=%v", force)
	}
	return nil
}

// --------------------------------------------------------------------------
//  		| ProtocolDriver -- Handle Commands |
//          HandleReadCommands, HandleWriteCommands
// --------------------------------------------------------------------------

// HandleReadCommands triggers a protocol Read operation for the specified device.
// DeviceResourceName: 待读取的传感器/寄存器名称
func (c *CompositeDriver) HandleReadCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []sdkModels.CommandRequest) (res []*sdkModels.CommandValue, err error) {
	c.lc.Debugf("SimpleDriver.HandleReadCommands: protocols: %v resource: %v attributes: %v", protocols, reqs[0].DeviceResourceName, reqs[0].Attributes)

	if len(reqs) == 1 {
		res = make([]*sdkModels.CommandValue, 1)
		if reqs[0].DeviceResourceName == "SwitchButton" {
			cv, _ := sdkModels.NewCommandValue(reqs[0].DeviceResourceName, common.ValueTypeBool, false)
			res[0] = cv
		} else if reqs[0].DeviceResourceName == "Xrotation" {
			cv, _ := sdkModels.NewCommandValue(reqs[0].DeviceResourceName, common.ValueTypeInt32, 111)
			res[0] = cv
		}
	} else if len(reqs) == 2 {
		res = make([]*sdkModels.CommandValue, 2)
		for i, r := range reqs {
			var cv *sdkModels.CommandValue
			switch r.DeviceResourceName {
			case "Xrotation":
				cv, _ = sdkModels.NewCommandValue(r.DeviceResourceName, common.ValueTypeInt32, 111)
			case "Yrotation":
				cv, _ = sdkModels.NewCommandValue(r.DeviceResourceName, common.ValueTypeInt32, 111)
			}
			res[i] = cv
		}
	}

	c.readCommandsExecuted.Inc(1)

	return
}

// HandleWriteCommands passes a slice of CommandRequest struct each representing
// a ResourceOperation for a specific device resource.
// Since the commands are actuation commands, params provide parameters for the individual
// command.
// HandleWriteCommands 传入一个 CommandRequest 结构体切片，
// 每个结构体对应一个特定设备资源的资源操作（ResourceOperation）。
// 由于这些命令属于执行/驱动类命令，params 为单个命令提供参数。
func (c *CompositeDriver) HandleWriteCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []sdkModels.CommandRequest,
	params []*sdkModels.CommandValue) error {

	for index, r := range reqs {
		c.lc.Debugf("Driver HandleWriteCommands: protocols: %v, resource: %v, parameters: %v, attributes: %v", protocols, reqs[index].DeviceResourceName, params[index], reqs[index].Attributes)
		c.lc.Infof("Please write data value (%s) to resource (%s) ", params[index].Value, r.DeviceResourceName)

	}
	return nil
}

// --------------------------------------------------------------------------
//             | ProtocolDriver -- Device Event |
// 				AddDevice, UpdateDevice, Discover
// --------------------------------------------------------------------------

// AddDevice is a callback function that is invoked
// when a new Device associated with this Device Service is added
func (c *CompositeDriver) AddDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error {
	c.lc.Debugf("a new Device is added: %s", deviceName)

	drv, err := c.route(protocols)
	if err != nil {
		return err
	}
	return drv.AddDevice(deviceName, protocols, adminState)
}

// UpdateDevice is a callback function that is invoked
// when a Device associated with this Device Service is updated
func (c *CompositeDriver) UpdateDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error {
	c.lc.Debugf("Device %s is updated", deviceName)

	drv, err := c.route(protocols)
	if err != nil {
		return err
	}
	return drv.UpdateDevice(deviceName, protocols, adminState)
}

// RemoveDevice is a callback function that is invoked
// when a Device associated with this Device Service is removed
func (c *CompositeDriver) RemoveDevice(deviceName string, protocols map[string]models.ProtocolProperties) error {
	c.lc.Debugf("Device %s is removed", deviceName)

	drv, err := c.route(protocols)
	if err != nil {
		return err
	}
	return drv.RemoveDevice(deviceName, protocols)
}

// Discover triggers protocol specific device discovery, which is an asynchronous operation.
// Devices found as part of this discovery operation are written to the channel devices.
// Discover 触发协议相关的设备发现，这是一个异步操作。
// 本次发现过程中找到的设备，会写入到 devices 通道中。
func (c *CompositeDriver) Discover() {
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

	time.Sleep(time.Duration(c.serviceConfig.SimpleCustom.Writable.DiscoverSleepDurationSecs) * time.Second)
	c.deviceCh <- res
}

func (c *CompositeDriver) ValidateDevice(device models.Device) error {
	drv, err := c.route(device.Protocols)
	if err != nil {
		return err
	}
	return drv.ValidateDevice(device)
}

// ---------- 私有路由方法 ----------

func (c *CompositeDriver) route(protocols map[string]models.ProtocolProperties) (SubDriver, error) {
	if _, ok := protocols[ProtocolKeyModbus]; ok {
		return nil, nil
	}
	if _, ok := protocols[ProtocolKeyHTTP]; ok {
		return nil, nil
	}
	return nil, fmt.Errorf("no supported protocol found in device protocols: %v", protocols)
}

// Initialize all observability metrics for the driver
// 初始化轻量的可观测性系统，观测边缘微服务的健康状态和运行性能
func (c *CompositeDriver) initMetrics() error {
	c.readCommandsExecuted = gometrics.NewCounter()
	ds := interfaces.Service()
	metricsManager := ds.GetMetricsManager()
	// Check if metrics manager is available
	if metricsManager == nil {
		return errors.New("metrics manager not available")
	}

	// Register the counter metric for read commands
	err := metricsManager.Register(readCommandsExecutedName, c.readCommandsExecuted, nil)
	if err != nil {
		return fmt.Errorf("unable to register metric %s: %s", readCommandsExecutedName, err.Error())
	}
	c.lc.Infof("Registered %s metric for collection when enabled", readCommandsExecutedName)
	return nil
}

// ProcessCustomConfigChanges ...hot-reload configuration
// 配置热更新，不重启加载配置。
func (c *CompositeDriver) ProcessCustomConfigChanges(rawWritableConfig interface{}) {
	updated, ok := rawWritableConfig.(*config.SimpleWritable)
	if !ok {
		c.lc.Error("unable to process custom config updates: Can not cast raw config to type 'SimpleWritable'")
		return
	}

	c.lc.Info("Received configuration updates for 'SimpleCustom.Writable' section")

	previous := c.serviceConfig.SimpleCustom.Writable
	c.serviceConfig.SimpleCustom.Writable = *updated

	if reflect.DeepEqual(previous, *updated) {
		c.lc.Info("No changes detected")
		return
	}

	// Now check to determine what changed.
	// In this example we only have the one writable setting,
	// so the check is not really need but left here as an example.
	// Since this setting is pulled from configuration each time it is need, no extra processing is required.
	// This may not be true for all settings, such as external host connection info, which
	// may require re-establishing the connection to the external host for example.
	if previous.DiscoverSleepDurationSecs != updated.DiscoverSleepDurationSecs {
		c.lc.Infof("DiscoverSleepDurationSecs changed to: %d", updated.DiscoverSleepDurationSecs)
	}
}
