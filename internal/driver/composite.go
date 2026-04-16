// Package driver internal/driver/composite.go
package driver

import (
	"fmt"
	"net/http"

	"github.com/edgexfoundry/device-sdk-go/v2/pkg/interfaces"
	sdkModels "github.com/edgexfoundry/device-sdk-go/v2/pkg/models"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/models"

	"better-iot-edge/internal/handler"
)

const (
	ProtocolKeyModbus = "modbus"
	ProtocolKeyHTTP   = "http"
)

// SubDriver 定义子驱动必须满足的接口。
type SubDriver interface {
	Initialize(sdk interfaces.DeviceServiceSDK) error
	Start() error
	Stop(force bool) error
	HandleReadCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []sdkModels.CommandRequest) ([]*sdkModels.CommandValue, error)
	HandleWriteCommands(deviceName string, protocols map[string]models.ProtocolProperties, reqs []sdkModels.CommandRequest, params []*sdkModels.CommandValue) error
	AddDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error
	UpdateDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error
	RemoveDevice(deviceName string, protocols map[string]models.ProtocolProperties) error
	Discover() error
	ValidateDevice(device models.Device) error
}

// CompositeDriver 持有所有子驱动，实现 EdgeX ProtocolDriver 接口。
type CompositeDriver struct {
	modbus SubDriver
	http   SubDriver
}

func NewCompositeDriver(modbus SubDriver) *CompositeDriver {
	return &CompositeDriver{modbus: modbus}
}

// ---------- 生命周期 ----------

func (c *CompositeDriver) Initialize(sdk interfaces.DeviceServiceSDK) error {
	if err := c.modbus.Initialize(sdk); err != nil {
		return fmt.Errorf("modbus driver Initialize: %w", err)
	}
	if err := c.http.Initialize(sdk); err != nil {
		return fmt.Errorf("http driver Initialize: %w", err)
	}

	// v2 的 AddCustomRoute 签名：(route string, handler http.HandlerFunc, methods ...string)
	// 没有 v3 的 interfaces.Authenticated 参数
	alarmHandler := handler.NewAlarmHandler(sdk.GetLoggingClient())
	if err := sdk.AddRoute("/api/alarm", alarmHandler.HandleAlarm, http.MethodPost); err != nil {
		return fmt.Errorf("register /api/alarm route: %w", err)
	}

	sdk.GetLoggingClient().Info("custom route registered: POST /api/alarm")
	return nil
}

func (c *CompositeDriver) Start() error {
	if err := c.modbus.Start(); err != nil {
		return err
	}
	return c.http.Start()
}

func (c *CompositeDriver) Stop(force bool) error {
	_ = c.modbus.Stop(force)
	_ = c.http.Stop(force)
	return nil
}

// ---------- 读写命令路由 ----------

func (c *CompositeDriver) HandleReadCommands(
	deviceName string,
	protocols map[string]models.ProtocolProperties,
	reqs []sdkModels.CommandRequest,
) ([]*sdkModels.CommandValue, error) {
	drv, err := c.route(protocols)
	if err != nil {
		return nil, err
	}
	return drv.HandleReadCommands(deviceName, protocols, reqs)
}

func (c *CompositeDriver) HandleWriteCommands(
	deviceName string,
	protocols map[string]models.ProtocolProperties,
	reqs []sdkModels.CommandRequest,
	params []*sdkModels.CommandValue,
) error {
	drv, err := c.route(protocols)
	if err != nil {
		return err
	}
	return drv.HandleWriteCommands(deviceName, protocols, reqs, params)
}

// ---------- 设备生命周期回调 ----------

func (c *CompositeDriver) AddDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error {
	drv, err := c.route(protocols)
	if err != nil {
		return err
	}
	return drv.AddDevice(deviceName, protocols, adminState)
}

func (c *CompositeDriver) UpdateDevice(deviceName string, protocols map[string]models.ProtocolProperties, adminState models.AdminState) error {
	drv, err := c.route(protocols)
	if err != nil {
		return err
	}
	return drv.UpdateDevice(deviceName, protocols, adminState)
}

func (c *CompositeDriver) RemoveDevice(deviceName string, protocols map[string]models.ProtocolProperties) error {
	drv, err := c.route(protocols)
	if err != nil {
		return err
	}
	return drv.RemoveDevice(deviceName, protocols)
}

// ---------- 设备发现 ----------

func (c *CompositeDriver) Discover() error {
	_ = c.modbus.Discover()
	_ = c.http.Discover()
	return nil
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
		return c.modbus, nil
	}
	if _, ok := protocols[ProtocolKeyHTTP]; ok {
		return c.http, nil
	}
	return nil, fmt.Errorf("no supported protocol found in device protocols: %v", protocols)
}
