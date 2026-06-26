package parser

import (
	"reflect"
	"strings"
	"testing"
)

// TestEncoderMapKeyMatchFuncName check every map key equals its function name
func TestEncoderMapKeyMatchFuncName(t *testing.T) {
	for mapKey, fn := range encoderMap {
		fnVal := reflect.ValueOf(fn)
		if fnVal.Kind() != reflect.Func {
			t.Fatalf("key [%s] value is not function", mapKey)
		}

		fullName := fnVal.Type().Name()
		var shortName string
		dotPos := strings.LastIndex(fullName, ".")
		if dotPos != -1 {
			shortName = fullName[dotPos+1:]
		} else {
			shortName = fullName
		}

		if mapKey != shortName {
			t.Errorf("name mismatch, key=%s, func=%s", mapKey, shortName)
		}
	}
}

// TestAllEncodeFuncRegistered check all encode functions exist in encoderMap
func TestAllEncodeFuncRegistered(t *testing.T) {
	registered := make(map[string]bool)
	for k := range encoderMap {
		registered[k] = true
	}

	// List all encode functions in this package
	allEncodeFuncs := []string{
		"encodeMACAddress",
		"encodeIPv4Address",
		"toUint16Slice",
	}

	for _, funcName := range allEncodeFuncs {
		if !registered[funcName] {
			t.Errorf("function %s not registered in encoderMap", funcName)
		}
	}
}
