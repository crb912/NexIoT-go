// internal/driver/modbus/driver.go
package modbus

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/edgexfoundry/device-sdk-go/v2/pkg/interfaces"
	sdkModels "github.com/edgexfoundry/device-sdk-go/v2/pkg/models"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/common"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/models"
	"github.com/goburrow/modbus"
)

const defaultTimeout = 3 * time.Second

type Driver struct {
	sdk     interfaces.DeviceServiceSDK
	clients sync.Map // key: deviceName, value: *clientEntry
}

type clientEntry struct {
	client  modbus.Client
	handler *modbus.TCPClientHandler
	mu      sync.Mutex
}

func NewDriver() *Driver {
	return &Driver{}
}

// ---------- 生命周期 ----------

func (d *Driver) Initialize(sdk interfaces.DeviceServiceSDK) error {
	d.sdk = sdk
	sdk.GetLoggingClient().Info("ModbusDriver initialized")
	return nil
}

func (d *Driver) Start() error { return nil }

func (d *Driver) Stop(_ bool) error {
	d.clients.Range(func(key, value any) bool {
		entry := value.(*clientEntry)
		entry.mu.Lock()
		defer entry.mu.Unlock()
		if entry.handler != nil {
			_ = entry.handler.Close()
		}
		d.clients.Delete(key)
		return true
	})
	d.sdk.GetLoggingClient().Info("ModbusDriver stopped")
	return nil
}

// ---------- 读命令 ----------

func (d *Driver) HandleReadCommands(
	deviceName string,
	protocols map[string]models.ProtocolProperties,
	reqs []sdkModels.CommandRequest,
) ([]*sdkModels.CommandValue, error) {
	props, err := extractModbusProps(protocols)
	if err != nil {
		return nil, fmt.Errorf("device %s: %w", deviceName, err)
	}

	entry, err := d.getOrCreateClient(deviceName, props)
	if err != nil {
		return nil, fmt.Errorf("device %s: connect failed: %w", deviceName, err)
	}

	results := make([]*sdkModels.CommandValue, 0, len(reqs))
	for _, req := range reqs {
		cv, err := d.readResource(entry, req)
		if err != nil {
			// 失败重连一次
			d.clients.Delete(deviceName)
			entry, err2 := d.getOrCreateClient(deviceName, props)
			if err2 != nil {
				return nil, fmt.Errorf("device %s: reconnect failed: %w", deviceName, err2)
			}
			cv, err = d.readResource(entry, req)
			if err != nil {
				return nil, fmt.Errorf("device %s resource %s: %w", deviceName, req.DeviceResourceName, err)
			}
		}
		results = append(results, cv)
	}
	return results, nil
}

func (d *Driver) readResource(entry *clientEntry, req sdkModels.CommandRequest) (*sdkModels.CommandValue, error) {
	entry.mu.Lock()
	defer entry.mu.Unlock()

	attrs := req.Attributes
	regAddr, err := parseUint16(attrStr(attrs, "modbusAddress", "0"))
	if err != nil {
		return nil, fmt.Errorf("invalid modbusAddress: %w", err)
	}
	dataType := attrStr(attrs, "modbusDataType", "float32")
	fn := attrStr(attrs, "modbusFunction", "holding")
	scale := transform.ParseScale(attrStr(attrs, "scale", "1.0"))

	quantity := uint16(2)
	if dataType == "int16" || dataType == "uint16" {
		quantity = 1
	}

	var rawBytes []byte
	switch fn {
	case "holding":
		rawBytes, err = entry.client.ReadHoldingRegisters(regAddr, quantity)
	case "input":
		rawBytes, err = entry.client.ReadInputRegisters(regAddr, quantity)
	default:
		return nil, fmt.Errorf("unsupported modbusFunction: %s", fn)
	}
	if err != nil {
		return nil, fmt.Errorf("modbus read error: %w", err)
	}

	floatVal, err := transform.DecodeModbusBytes(rawBytes, dataType)
	if err != nil {
		return nil, err
	}
	floatVal *= scale

	// v2 用 sdkModels.NewCommandValue 构造 CommandValue
	cv, err := sdkModels.NewCommandValue(req.DeviceResourceName, common.ValueTypeFloat64, floatVal)
	if err != nil {
		return nil, err
	}
	return cv, nil
}

// ---------- 写命令 ----------

func (d *Driver) HandleWriteCommands(
	deviceName string,
	protocols map[string]models.ProtocolProperties,
	reqs []sdkModels.CommandRequest,
	params []*sdkModels.CommandValue,
) error {
	props, err := extractModbusProps(protocols)
	if err != nil {
		return fmt.Errorf("device %s: %w", deviceName, err)
	}

	entry, err := d.getOrCreateClient(deviceName, props)
	if err != nil {
		return fmt.Errorf("device %s: connect failed: %w", deviceName, err)
	}

	for i, req := range reqs {
		if err := d.writeResource(entry, req, params[i]); err != nil {
			return fmt.Errorf("device %s resource %s: %w", deviceName, req.DeviceResourceName, err)
		}
	}
	return nil
}

func (d *Driver) writeResource(entry *clientEntry, req sdkModels.CommandRequest, cv *sdkModels.CommandValue) error {
	entry.mu.Lock()
	defer entry.mu.Unlock()

	attrs := req.Attributes
	regAddr, err := parseUint16(attrStr(attrs, "modbusAddress", "0"))
	if err != nil {
		return err
	}
	dataType := attrStr(attrs, "modbusDataType", "float32")

	// v2 用 cv.Float64Value() 等方法取值
	rawVal, err := cv.Float64Value()
	if err != nil {
		return err
	}

	rawBytes, err := transform.EncodeModbusBytes(rawVal, dataType)
	if err != nil {
		return err
	}

	_, err = entry.client.WriteMultipleRegisters(regAddr, uint16(len(rawBytes)/2), rawBytes)
	return err
}

// ---------- 设备回调 ----------

func (d *Driver) AddDevice(deviceName string, protocols map[string]models.ProtocolProperties, _ models.AdminState) error {
	d.sdk.GetLoggingClient().Infof("ModbusDriver: device added: %s", deviceName)
	props, err := extractModbusProps(protocols)
	if err != nil {
		return err
	}
	_, err = d.getOrCreateClient(deviceName, props)
	return err
}

func (d *Driver) UpdateDevice(deviceName string, protocols map[string]models.ProtocolProperties, _ models.AdminState) error {
	d.clients.Delete(deviceName)
	props, err := extractModbusProps(protocols)
	if err != nil {
		return err
	}
	_, err = d.getOrCreateClient(deviceName, props)
	return err
}

func (d *Driver) RemoveDevice(deviceName string, _ map[string]models.ProtocolProperties) error {
	if v, ok := d.clients.LoadAndDelete(deviceName); ok {
		entry := v.(*clientEntry)
		entry.mu.Lock()
		defer entry.mu.Unlock()
		if entry.handler != nil {
			_ = entry.handler.Close()
		}
	}
	return nil
}

func (d *Driver) Discover() error { return nil }

func (d *Driver) ValidateDevice(device models.Device) error {
	_, err := extractModbusProps(device.Protocols)
	return err
}

// ---------- 连接管理 ----------

type modbusProps struct {
	address string
	slaveID byte
}

func (d *Driver) getOrCreateClient(deviceName string, props modbusProps) (*clientEntry, error) {
	if v, ok := d.clients.Load(deviceName); ok {
		return v.(*clientEntry), nil
	}

	handler := modbus.NewTCPClientHandler(props.address)
	handler.Timeout = defaultTimeout
	handler.SlaveId = props.slaveID

	if err := handler.Connect(); err != nil {
		return nil, fmt.Errorf("TCP connect to %s: %w", props.address, err)
	}

	entry := &clientEntry{
		client:  modbus.NewClient(handler),
		handler: handler,
	}
	d.clients.Store(deviceName, entry)
	return entry, nil
}

// ---------- 工具函数 ----------

func extractModbusProps(protocols map[string]models.ProtocolProperties) (modbusProps, error) {
	p, ok := protocols["modbus"]
	if !ok {
		return modbusProps{}, fmt.Errorf("missing 'modbus' protocol section")
	}
	address, ok := p["Address"]
	if !ok || address == "" {
		return modbusProps{}, fmt.Errorf("modbus.Address is required")
	}
	slaveIDStr, ok := p["SlaveID"]
	if !ok {
		slaveIDStr = "1"
	}
	slaveID, err := strconv.ParseUint(slaveIDStr, 10, 8)
	if err != nil {
		return modbusProps{}, fmt.Errorf("invalid SlaveID %q: %w", slaveIDStr, err)
	}
	return modbusProps{address: address, slaveID: byte(slaveID)}, nil
}

func attrStr(attrs map[string]interface{}, key, def string) string {
	if v, ok := attrs[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func parseUint16(s string) (uint16, error) {
	v, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, err
	}
	return uint16(v), nil
}
