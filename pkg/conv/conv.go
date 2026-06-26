package conv

import (
	"fmt"
	"reflect"
)

// ToBool only accept bool
func ToBool(v any) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("only support bool value")
	}
	return b, nil
}

// ToUint16 only accept uint16
func ToUint16(v any) (uint16, error) {
	u, ok := v.(uint16)
	if !ok {
		return 0, fmt.Errorf("only support uint16 value")
	}
	return u, nil
}

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

// ToBoolSlice converts input to []bool for modbus coil write
// support native bool or []bool input
func ToBoolSlice(v any) ([]bool, error) {
	if v == nil {
		return nil, fmt.Errorf("only bool, []bool supported, got nil")
	}
	switch val := v.(type) {
	case bool:
		return []bool{val}, nil
	case []bool:
		return val, nil
	default:
		return nil, fmt.Errorf("only bool, []bool supported, got %s", reflect.TypeOf(v))
	}
}

// ToUint16Slice convert input to []uint16 for modbus register write
// support uint16 or []uint16 input
func ToUint16Slice(v any) ([]uint16, error) {
	if v == nil {
		return nil, fmt.Errorf("only uint16, []uint16 supported, got nil")
	}
	switch val := v.(type) {
	case uint16:
		return []uint16{val}, nil
	case []uint16:
		return val, nil
	default:
		return nil, fmt.Errorf("only uint16, []uint16 or supported, got %s", reflect.TypeOf(v))
	}
}
