// Package conv provides type conversion helpers used across protocol adapters.
package conv

import (
	"fmt"
	"reflect"
	"strconv"
)

// ToBool only accept bool value.
func ToBool(v any) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("only support bool value")
	}
	return b, nil
}

// ToUint16 only accept uint16 value.
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
	case string:
		u, err := strconv.ParseUint(val, 10, 64)
		if err == nil {
			return uint(u), true
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

// ToBoolSlice converts input to []bool for modbus coil write.
// Supports native bool or []bool input.
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

// ToUint16Slice converts input to []uint16 for modbus register write.
// Supports uint16 or []uint16 input.
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
		return nil, fmt.Errorf("only uint16, []uint16 supported, got %s", reflect.TypeOf(v))
	}
}

// ToFloat64 converts common numeric types to float64 for OPC UA write.
func ToFloat64(v any) (float64, error) {
	if v == nil {
		return 0, fmt.Errorf("cannot convert nil to float64")
	}
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case uint32:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// ToInt32 converts common numeric types to int32 for OPC UA write.
func ToInt32(v any) (int32, error) {
	if v == nil {
		return 0, fmt.Errorf("cannot convert nil to int32")
	}
	switch val := v.(type) {
	case int32:
		return val, nil
	case int:
		return int32(val), nil
	case int64:
		return int32(val), nil
	case float64:
		return int32(val), nil
	case float32:
		return int32(val), nil
	case uint:
		return int32(val), nil
	case uint32:
		return int32(val), nil
	case uint64:
		return int32(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int32", v)
	}
}

// ToFloat64Slice converts input to []float64 for OPC UA batch write.
func ToFloat64Slice(v any) ([]float64, error) {
	if v == nil {
		return nil, fmt.Errorf("only float64, []float64 supported, got nil")
	}
	switch val := v.(type) {
	case float64:
		return []float64{val}, nil
	case []float64:
		return val, nil
	default:
		return nil, fmt.Errorf("only float64, []float64 supported, got %s", reflect.TypeOf(v))
	}
}

// ToInt32Slice converts input to []int32 for OPC UA batch write.
func ToInt32Slice(v any) ([]int32, error) {
	if v == nil {
		return nil, fmt.Errorf("only int32, []int32 supported, got nil")
	}
	switch val := v.(type) {
	case int32:
		return []int32{val}, nil
	case []int32:
		return val, nil
	default:
		return nil, fmt.Errorf("only int32, []int32 supported, got %s", reflect.TypeOf(v))
	}
}
