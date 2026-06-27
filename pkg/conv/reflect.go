package conv

import "fmt"

// ValueToType converts a raw value to the exact Go type specified by
// an EdgeX ValueType constant (e.g. "Uint64", "String", "Float64").
//
// This is a generic, protocol-agnostic normalizer: any protocol adapter
// can call it after decoding a device value, to ensure the result matches
// the strict Go type that EdgeX expects for the given resource.
func ValueToType(rawValue interface{}, edgexType string) (interface{}, error) {
	switch edgexType {
	case "Uint8":
		return toUint8(rawValue)
	case "Uint16":
		return toUint16(rawValue)
	case "Uint32":
		return toUint32(rawValue)
	case "Uint64":
		return toUint64(rawValue)
	case "Int8":
		return toInt8(rawValue)
	case "Int16":
		return toInt16(rawValue)
	case "Int32":
		return toInt32(rawValue)
	case "Int64":
		return toInt64(rawValue)
	case "Float32":
		return toFloat32(rawValue)
	case "Float64":
		return toFloat64(rawValue)
	case "String":
		return toString(rawValue)
	case "Bool":
		return toBool(rawValue)
	case "Binary":
		return toBinary(rawValue)
	default:
		return rawValue, nil
	}
}

// ─── unsigned integers ──────────────────────────────────────────────────

func toUint8(v interface{}) (uint8, error) {
	u, err := toUint64(v)
	return uint8(u), err
}

func toUint16(v interface{}) (uint16, error) {
	u, err := toUint64(v)
	return uint16(u), err
}

func toUint32(v interface{}) (uint32, error) {
	u, err := toUint64(v)
	return uint32(u), err
}

func toUint64(v interface{}) (uint64, error) {
	switch val := v.(type) {
	case uint64:
		return val, nil
	case uint:
		return uint64(val), nil
	case uint32:
		return uint64(val), nil
	case uint16:
		return uint64(val), nil
	case uint8:
		return uint64(val), nil
	case int:
		if val < 0 {
			return 0, fmt.Errorf("cannot convert negative int %d to Uint64", val)
		}
		return uint64(val), nil
	case int64:
		if val < 0 {
			return 0, fmt.Errorf("cannot convert negative int64 %d to Uint64", val)
		}
		return uint64(val), nil
	case int32:
		if val < 0 {
			return 0, fmt.Errorf("cannot convert negative int32 %d to Uint64", val)
		}
		return uint64(val), nil
	case float64:
		if val < 0 {
			return 0, fmt.Errorf("cannot convert negative float64 %f to Uint64", val)
		}
		return uint64(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to Uint64", v)
	}
}

// ─── signed integers ────────────────────────────────────────────────────

func toInt8(v interface{}) (int8, error) {
	i, err := toInt64(v)
	return int8(i), err
}

func toInt16(v interface{}) (int16, error) {
	i, err := toInt64(v)
	return int16(i), err
}

func toInt32(v interface{}) (int32, error) {
	i, err := toInt64(v)
	return int32(i), err
}

func toInt64(v interface{}) (int64, error) {
	switch val := v.(type) {
	case int64:
		return val, nil
	case int:
		return int64(val), nil
	case int32:
		return int64(val), nil
	case int16:
		return int64(val), nil
	case int8:
		return int64(val), nil
	case uint64:
		return int64(val), nil
	case uint:
		return int64(val), nil
	case uint32:
		return int64(val), nil
	case float64:
		return int64(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to Int64", v)
	}
}

// ─── floats ─────────────────────────────────────────────────────────────

func toFloat32(v interface{}) (float32, error) {
	f, err := toFloat64(v)
	return float32(f), err
}

func toFloat64(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to Float64", v)
	}
}

// ─── string / bool / binary ─────────────────────────────────────────────

func toString(v interface{}) (string, error) {
	switch val := v.(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		return fmt.Sprintf("%v", val), nil
	}
}

func toBool(v interface{}) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case int:
		return val != 0, nil
	case int64:
		return val != 0, nil
	case uint:
		return val != 0, nil
	case uint64:
		return val != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to Bool", v)
	}
}

func toBinary(v interface{}) ([]byte, error) {
	switch val := v.(type) {
	case []byte:
		return val, nil
	case string:
		return []byte(val), nil
	default:
		return nil, fmt.Errorf("cannot convert %T to Binary", v)
	}
}
