// Package driver internal/driver/composite.go
package driver

import (
	"context"
	"errors"
	"fmt"
	"octopus-edge/pkg/cache"
	"octopus-edge/pkg/connector"
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
	receivedData         chan connector.ReceiveEvent         // Bridge channel between Receivers and EdgeX
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
	cd.receivedData = make(chan connector.ReceiveEvent, 256)

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

	cd.receivers = connector.NewReceivers(15, 16)
	//  Start the internal consumer goroutine
	go cd.processReceiveData()
	cd.receivers.RegisterHttpServer("127.0.0.1", 8000, "/alarm/push", cd.receivedData)

	// Start all receivers, passing the internal channel to them
	if err = cd.receivers.StartAll(cd.ctx); err != nil {
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
		protocolConfig := cache.ResolveProtocolConfig(deviceName, protocolName, protocolProperties)

		if protocolConfig.IsDisabled() {
			cd.lc.Debugf("Skip device: %s protocol %s (disabled)", deviceName, protocolName)
			continue
		}

		reader, err := cd.polls.GetHandler(protocolConfig)
		if err != nil {
			cd.lc.Errorf("No reader for protocol: %s, device: %s, err: %v", protocolName, deviceName, err)
			continue
		}

		if len(reqs) == 1 {
			cv, err := connector.HandleReadSingle(reader, reqs[0])
			if err != nil {
				cd.lc.Errorf("@read failed: dev %s, res %s, err %v", deviceName, reqs[0].DeviceResourceName, err)
				continue
			}
			res = append(res, cv)
			cd.readCommandsExecuted.Inc(1)
			cd.lc.Debugf("@read ok: dev %s, res %s, val %v ", deviceName, cv.DeviceResourceName, cv.Value)
		}

		cvList, err := connector.HandleReadBatch(reader, reqs)
		if err != nil {
			cd.lc.Errorf("@read batch failed: dev %s, err %v", deviceName, err)
		}
		res = append(res, cvList...)
		cd.readCommandsExecuted.Inc(1)
		cd.lc.Debugf("@read batch ok: dev %s ", deviceName)
	}
	return
}

// HandleWriteCommands triggers a Write operation for the specified device.
func (cd *CompositeDriver) HandleWriteCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []sdkModels.CommandRequest,
	params []*sdkModels.CommandValue) error {
	for protocolName, protocolProperties := range protocols {
		protocolConfig := cache.ResolveProtocolConfig(deviceName, protocolName, protocolProperties)

		if protocolConfig.IsDisabled() {
			cd.lc.Debugf("Skip write: device %s protocol %s (disabled)", deviceName, protocolName)
			continue
		}

		handler, err := cd.polls.GetHandler(protocolConfig)
		if err != nil {
			cd.lc.Errorf("No handler for protocol: %s, device: %s, err: %v", protocolName, deviceName, err)
			continue
		}

		writer, ok := handler.(connector.Writer)
		if !ok {
			cd.lc.Errorf("Protocol %s does not support write operations for device %s", protocolName, deviceName)
			continue
		}

		if len(reqs) == 1 {
			if err := connector.HandleWriteSingle(writer, reqs[0], params[0]); err != nil {
				cd.lc.Errorf("@write failed: dev %s, res %s, err %v", deviceName, reqs[0].DeviceResourceName, err)
				return err
			}
			cd.lc.Debugf("@write ok: dev %s, res %s, val %v", deviceName, reqs[0].DeviceResourceName, params[0].Value)
		} else {
			if err := connector.HandleWriteBatch(writer, reqs, params); err != nil {
				cd.lc.Errorf("@write batch failed: dev %s, err %v", deviceName, err)
				return err
			}
			cd.lc.Debugf("@write batch ok: dev %s", deviceName)
		}
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

// processReceiveData reads from receivedData and pushes to EdgeX
func (cd *CompositeDriver) processReceiveData() {
	for {
		select {
		case <-cd.ctx.Done():
			// Exit loop when Driver stops
			return
		case data := <-cd.receivedData:
			cd.lc.Debugf("@!Received data: %v", data)
			// Convert AsyncData to EdgeX CommandValue
			cv, err := sdkModels.NewCommandValue("", common.ValueTypeFloat64, 1.1)
			if err != nil {
				cd.lc.Errorf("Failed to create CommandValue: %v", err)
				continue
			}

			// Wrap into EdgeX AsyncValues structure
			asyncVal := &sdkModels.AsyncValues{
				DeviceName:    "Test Receiver",
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
