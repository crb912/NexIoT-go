package parser

import (
	"strconv"
	"time"

	"github.com/edgexfoundry/go-mod-core-contracts/v2/models"
)

// ProtocolConfigAdapter implements the ProtocolConfig interface.
// Added protocolName to store the isolated variable.
type ProtocolConfigAdapter struct {
	props        *models.ProtocolProperties // Explicitly holding the third-party object
	protocolName string                     // Separate variable for protocol name
}

// NewProtocolConfig is a constructor to initialize the adapter.
func NewProtocolConfig(name string, props *models.ProtocolProperties) *ProtocolConfigAdapter {
	return &ProtocolConfigAdapter{
		props:        props,
		protocolName: name,
	}
}

// GetEndpoint returns the Endpoint string from the map.
func (a *ProtocolConfigAdapter) GetEndpoint() string {
	if a.props == nil {
		return ""
	}
	// Dereference the pointer to access the map
	return (*a.props)["Endpoint"]
}

// GetProtocolName returns the protocol name variable.
func (a *ProtocolConfigAdapter) GetProtocolName() string {
	return a.protocolName
}

// GetTimeout parses the timeout string and returns it as time.Duration.
func (a *ProtocolConfigAdapter) GetTimeout() time.Duration {
	if a.props == nil {
		return 0
	}

	// Get the timeout string from the map
	timeoutStr, exists := (*a.props)["Timeout"]
	if !exists {
		return 0
	}

	// Convert string to integer
	timeoutInt, err := strconv.Atoi(timeoutStr)
	if err != nil {
		// Return default 0 if conversion fails
		return 0
	}

	// Assume the value "5" means 5 seconds.
	// Change time.Second to time.Millisecond if needed.
	return time.Duration(timeoutInt) * time.Second
}

// GetAll returns the entire map.
func (a *ProtocolConfigAdapter) GetAll() map[string]string {
	if a.props == nil {
		return nil
	}
	// Return the underlying map
	return *a.props
}
