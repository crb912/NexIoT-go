package model

import "errors"

type ProtocolType string

const (
	MQTT      ProtocolType = "mqtt"
	ModbusTCP ProtocolType = "modbus-tcp"
	ModbusRTU ProtocolType = "modbus-rtu"
	SNMP      ProtocolType = "snmp"
	OPCUA      ProtocolType = "opcua"
	HTTPClient ProtocolType = "http-client"
	Unknown    ProtocolType = "unknown"
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
	case "opcua":
		return OPCUA, nil
	case "http-client":
		return HTTPClient, nil
	default:
		return Unknown, errors.New("not support protocol type")
	}
}
