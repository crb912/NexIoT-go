// bertter-iot-edge/config/config.go

package pkg

import (
	"fmt"
)

const baseDir = "res/devices"
const envFilename = "env.json"

// Protocol constants
type Protocol int

const (
	Modbus Protocol = iota
	SNMP
	OPC
	EPS
	MTQQ
)

// Env Define Environment type as a string
type Env string

const (
	EnvProd    Env = "prod"
	EnvTest    Env = "test"
	EnvVT      Env = "vt"
	EnvP14     Env = "p14"
	EnvDefault Env = "prod"
)

type EnvJson struct {
	Prod       string `json:"prod"`
	Test       string `json:"test"`
	CurrentEnv string `json:"current_env"`
}

// String method converts Protocol int to string for path generation
func (p Protocol) String() string {
	switch p {
	case Modbus:
		return "modbus"
	case SNMP:
		return "snmp"
	case OPC:
		return "opc"
	default:
		return "unknown"
	}
}

// Device defines a physical or virtual device in EdgeX
type Device struct {
	Id          string // the unique UUID of the device
	Name        string // the unique string identifier for the device
	Description string
	AdminState  bool     // AdminState shows if the device is LOCKED or UNLOCKED
	Protocols   Protocol // Protocols stores connection details for specific protocols (e.g., Modbus, SNMP)
	Labels      []string // Labels are tags used for searching or grouping devices
	// Location stores physical or logical placement of the device
	Location any
	// ServiceName is the name of the Device Service managing this device
	ServiceName string
	// ProfileName is the name of the Device Profile (data model) bound to this device
	ProfileName string
	// AutoEvents lists events that should be generated automatically at set intervals
	// AutoEvents []AutoEvent

	// Properties stores extra, custom settings for the device
	// Properties map[string]any
}

const (
	FormatJSON  Format = "json"
	FormatExcel Format = "excel"
)

const (
	modbusFilepath = "config.toml"
)

// DevicesList holds the device information.
type DevicesList struct {
	Devices []interface{} `json:"devices"`
}

// Devices holds data for a single register point.

// NewLoader creates a new loader based on the format.
func NewLoader(f Format) (Loader, error) {
	switch f {
	case FormatJSON:
		return &JSONLoader{}, nil
	case FormatExcel:
		return &ExcelLoader{}, nil
	default:
		return nil, fmt.Errorf("unsupported format: %s", f)
	}
}
