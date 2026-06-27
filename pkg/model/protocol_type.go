package model

import "errors"

type ProtocolType string

const (
	MQTT      ProtocolType = "mqtt"
	ModbusTCP ProtocolType = "modbus-tcp"
	ModbusRTU ProtocolType = "modbus-rtu"
	SNMP      ProtocolType = "snmp"
	Unknown   ProtocolType = "unknown"
)

func ValidateProtocol(protocolName string) (ProtocolType, error) {
	switch protocolName {
	case "modbus-tcp":
		return ModbusTCP, nil
	case "modbus-rtu":
		return ModbusRTU, nil
	case "mqtt":
		return MQTT, nil
	case "snmp":
		return SNMP, nil
	default:
		return Unknown, errors.New("not support protocol type")
	}
}
