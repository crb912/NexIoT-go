package conv

import (
	"fmt"
	"strconv"

	"github.com/edgexfoundry/go-mod-core-contracts/v2/models"
)

// ToUint converts common numeric types stored in interface{} to uint.
func ToUint(v interface{}) (uint, bool) {
	switch val := v.(type) {
	case uint:
		return val, true
	case int:
		if val >= 0 {
			return uint(val), true
		}
	case uint64:
		return uint(val), true
	case int64:
		if val >= 0 {
			return uint(val), true
		}
	case float64:
		if val >= 0 {
			return uint(val), true
		}
	}
	return 0, false
}

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
