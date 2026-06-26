package parser

import (
	"reflect"
	"strings"
	"testing"
)

// TestDecoderMapKeyMatchFuncName check every map key equals its function name
func TestDecoderMapKeyMatchFuncName(t *testing.T) {
	for mapKey, fn := range decoderMap {
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

// TestAllDecodeFuncRegistered check all decode functions exist in decoderMap
func TestAllDecodeFuncRegistered(t *testing.T) {
	registered := make(map[string]bool)
	for k := range decoderMap {
		registered[k] = true
	}

	// List all decode functions in this package
	allDecodeFuncs := []string{
		"decodeMACAddress",
		"decodeIPv4Address",
		"toUnit32",
	}

	for _, funcName := range allDecodeFuncs {
		if !registered[funcName] {
			t.Errorf("function %s not registered in decoderMap", funcName)
		}
	}
}
