package pkg

import (
	"testing"
)

func TestGetGetDeviceConfigPath(t *testing.T) {
	t.Run("Test GetGetDeviceConfigPath", func(t *testing.T) {
		actual := GetDeviceConfigPath(EnvProd, Modbus)
		if actual != "res/devices/prod/modbus/config.json" {
			t.Errorf("expect: res/devices/prod/modbus/config.json，but: %s", actual)
		}
		actual = GetDeviceConfigPath(EnvProd, SNMP)
		if actual != "res/devices/prod/snmp/config.json" {
			t.Errorf("expect: res/devices/test/mtqq/config.json，but: %s", actual)
		}
	})
}
