package adapter

import (
	"better-iot-edge/pkg/protocol"
	"time"
)

// Config represents the unified configuration for any protocol.
// This is usually parsed from config.toml or JSON.
type Config struct {
	protocol protocol.ProType // e.g., "modbus-tcp", "modbus-rtu", "opcua", "http"
	Endpoint string           // e.g., "tcp://127.0.0.1:502" or "/dev/ttyUSB0"
	Timeout  time.Duration    // Connection and read timeout
	// You can add more protocol-specific settings here if needed
}

// NewAdapter is the Factory function.
// It hides all protocol creation details from the main program.
//func NewAdapter(cfg Config) (ProtocolAdapter, error) {
//	switch cfg.Protocol {
//	case "modbus-tcp", "modbus-rtu":
//		return &modbusAdapter{config: cfg}, nil
//	case "opcua":
//		// return newOpcuaAdapter(cfg), nil
//		return nil, fmt.Errorf("opcua not implemented yet")
//	case "http":
//		// return newHttpAdapter(cfg), nil
//		return nil, fmt.Errorf("http not implemented yet")
//	default:
//		return nil, fmt.Errorf("unsupported protocol: %s", cfg.Protocol)
//	}
//}
