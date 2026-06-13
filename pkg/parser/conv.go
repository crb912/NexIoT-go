package parser

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
