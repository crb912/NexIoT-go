// Package driver internal/driver/composite.go
package driver

import (
	"context"
	"errors"
	"fmt"
	"octopus-edge/pkg/adapter"
	"octopus-edge/pkg/connector"
	"octopus-edge/pkg/parser"
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

type CompositeDriver struct {
	lc                   logger.LoggingClient
	asyncCh              chan<- *sdkModels.AsyncValues       // send adta
	dataCh               chan *adapter.AsyncData             // Bridge channel between Receivers and EdgeX
	deviceCh             chan<- []sdkModels.DiscoveredDevice // add device
	ctx                  context.Context
	cancel               context.CancelFunc
	readCommandsExecuted gometrics.Counter
	serviceConfig        *config.ServiceConfig // user defined config
	polls                *connector.Polls
	receivers            *connector.Receivers
}

// Initialize performs protocol-independent initialization for the device service.
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

	return cd.Start()
}

// Start initializes polls and receivers, the former actively collects device data,
// the latter accepts data push from devices.
// NOTE: Starting from device-sdk-go v3.0, the SDK automatically calls the Start method.
// For earlier versions like v2.3, users must manually call this method.
func (cd *CompositeDriver) Start() (err error) {
	cd.lc.Info("Driver Start")

	cd.polls = connector.NewPolls(
		connector.WithMaxCounts(30),
		connector.WithTimeout(5*time.Second))
	cd.lc.Info("Polls Started")

	//  Start the internal consumer goroutine
	go cd.processReceiveData()
	cd.receivers = connector.NewReceivers(":8080")

	// Start all receivers, passing the internal channel to them
	if err = cd.receivers.StartAll(cd.ctx, cd.dataCh); err != nil {
		cd.lc.Errorf("Failed to start the receiver: %v", err)
		return err
	}

	cd.lc.Infof("Receivers Started, Receivers nums: %d", len(cd.receivers.Servers))

	return nil
}

// Stop the protocol-independent DS code to shut down gracefully, or
// if the force parameter is 'true', immediately. The driver is responsible
// for closing any in-use channels, including the channel used to send async
// readings (if supported).
func (cd *CompositeDriver) Stop(force bool) error {
	if cd.lc != nil {
		cd.lc.Infof("Driver Stop called: force=%v", force)
	}

	err := cd.receivers.StopAll()
	if err != nil {
		cd.lc.Errorf("Driver Stop err: %v", err)
	}

	// Notify all background goroutines to exit
	cd.cancel()
	// Stop receivers one by one
	return nil
}

// HandleReadCommands triggers a Read operation for the specified device.
func (cd *CompositeDriver) HandleReadCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []sdkModels.CommandRequest) (res []*sdkModels.CommandValue, err error) {
	for protocolName, protocolProperties := range protocols {
		if isDisabled(protocolProperties) {
			cd.lc.Debugf("Skip device: %s protocol %s (disabled)", deviceName, protocolName)
			continue
		}

		pc := parser.NewProtocolConfig(protocolName, &protocolProperties)
		reader, err := cd.polls.GetReader(pc)
		if err != nil {
			cd.lc.Error("No reader for protocol: %s, device: %s, err: %v", protocolName, deviceName, err)
			continue
		}

		var createErr error
		var cv *sdkModels.CommandValue

		if len(reqs) == 1 {
			resIDInterface := reqs[0].Attributes["startingAddress"]
			resID := fmt.Sprintf("%v", resIDInterface)
			resData, err := reader.ReadSingle(resID)
			if err != nil {
				cd.lc.Errorf("ReadSingle failed for res: %s, device: %s, err: %v", reqs[0].DeviceResourceName, deviceName, err)
				continue
			}
			cv, createErr = sdkModels.NewCommandValue(reqs[0].DeviceResourceName, reqs[0].Type, resData.RawValue)
			if createErr != nil {
				cd.lc.Errorf("Failed to create CommandValue for %s, device: %s, err: %v", reqs[0].DeviceResourceName, deviceName, createErr)
				continue
			}
			res[0] = cv
			cd.readCommandsExecuted.Inc(1)
			continue
		}

		// TODO:这里的res ID 应该是空接口， 是任意类型， readBatch 一般只是字符串类型。因此要让它适配任意的接口。参考edge-sdk实现。
		resIDs := func() []string {
			names := make([]string, 0, len(reqs))
			for _, r := range reqs {
				resIDInterface := r.Attributes["startingAddress"]
				resID := fmt.Sprintf("%v", resIDInterface)
				names = append(names, resID)
			}
			return names
		}()

		data, err := reader.ReadBatch(resIDs)
		if err != nil {
			cd.lc.Errorf("ReadBatch failed for res: , device: %s, err: %v", deviceName, err)
		}
		cd.lc.Debugf("### Read ok: %v", data)

		// TODO: object pool
		res = make([]*sdkModels.CommandValue, len(reqs))
		for i, r := range reqs {
			cv, createErr = sdkModels.NewCommandValue(r.DeviceResourceName, r.Type, "A default value for example")
			if createErr != nil {
				cd.lc.Errorf("Failed to create CommandValue for %s: %v", r.DeviceResourceName, createErr)
				continue
			}
			res[i] = cv
			cd.readCommandsExecuted.Inc(1)
		}
	}
	return
}

// HandleWriteCommands passes a slice of CommandRequest struct each representing
// a ResourceOperation for a specific device resource.
// Since the commands are actuation commands, params provide parameters for the individual
// command.
func (cd *CompositeDriver) HandleWriteCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []sdkModels.CommandRequest,
	params []*sdkModels.CommandValue) error {

	for index, r := range reqs {
		cd.lc.Debugf("Driver HandleWriteCommands: protocols: %v, resource: %v, parameters: %v, attributes: %v", protocols, reqs[index].DeviceResourceName, params[index], reqs[index].Attributes)
		cd.lc.Infof("Please write data value (%s) to resource (%s) device(%s)", params[index].Value, r.DeviceResourceName, deviceName)
	}
	return nil
}

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

// Discover triggers protocol-independent device discovery, which is an asynchronous operation.
// Devices found as part of this discovery operation are written to the channel devices.
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
	// 校验 modbus-tcp 协议
	tcpProtocol, ok := device.Protocols["modbus-tcp"]
	if !ok {
		return errors.New("missing 'modbus-tcp' protocols")
	}

	addr, ok := tcpProtocol["Address"]
	if !ok || addr == "" {
		return errors.New("missing or empty 'Address' in modbus-tcp")
	}

	port, ok := tcpProtocol["Port"]
	if !ok || port == "" {
		return errors.New("missing 'Port' in modbus-tcp")
	}

	return nil
}

// Init all observability metrics for the driver
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
	cd.lc.Infof("Registered %s metric", readCommandsExecutedName)
	return nil
}

func isDisabled(props models.ProtocolProperties) bool {
	v, ok := props["enabled"]
	return ok && v == "false"
}
