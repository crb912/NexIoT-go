package model

import (
	"strconv"
	"sync"
	"time"

	"github.com/edgexfoundry/go-mod-core-contracts/v2/models"
)

// cacheKey is used as the key for the global cache map.
// It is allocated on the stack, avoiding heap allocation.
type cacheKey struct {
	deviceName   string
	protocolName string
}

// globalProtocolCache stores the protocol configs.
var globalProtocolCache sync.Map

// ProtocolConfig holds the protocol information for a device.
type ProtocolConfig struct {
	Name          string
	rawProperties models.ProtocolProperties
}

// ResolveProtocolConfig gets an existing config or creates a new one.
func ResolveProtocolConfig(deviceName string, protocolName string, props models.ProtocolProperties) *ProtocolConfig {
	// Create a struct key on the stack. Zero heap allocation!
	key := cacheKey{
		deviceName:   deviceName,
		protocolName: protocolName,
	}

	// Fast path: load the configuration without locks.
	if val, ok := globalProtocolCache.Load(key); ok {
		return val.(*ProtocolConfig)
	}

	// Slow path: create a new configuration instance.
	newConfig := &ProtocolConfig{
		rawProperties: props,
		Name:          protocolName,
	}

	// Store the new config safely to prevent data race.
	actual, _ := globalProtocolCache.LoadOrStore(key, newConfig)
	return actual.(*ProtocolConfig)
}

// UpdateProtocolConfig safely replaces an existing config with a new one.
func UpdateProtocolConfig(deviceName string, protocolName string, newProps models.ProtocolProperties) *ProtocolConfig {
	key := cacheKey{
		deviceName:   deviceName,
		protocolName: protocolName,
	}

	updatedConfig := &ProtocolConfig{
		rawProperties: newProps,
		Name:          protocolName,
	}

	globalProtocolCache.Store(key, updatedConfig)
	return updatedConfig
}

// RemoveProtocolConfig deletes a config from the cache.
func RemoveProtocolConfig(deviceName string, protocolName string) {
	key := cacheKey{
		deviceName:   deviceName,
		protocolName: protocolName,
	}
	globalProtocolCache.Delete(key)
}

// IsDisabled checks if the protocol is disabled.
func (a *ProtocolConfig) IsDisabled() bool {
	v, ok := a.rawProperties["Enabled"]
	return ok && v == "false"
}

// GetEndpoint returns the Endpoint string.
func (a *ProtocolConfig) GetEndpoint() string {
	if a.rawProperties == nil {
		return ""
	}
	return a.rawProperties["Endpoint"]
}

// GetTimeout parses the timeout string and returns time.Duration.
func (a *ProtocolConfig) GetTimeout() time.Duration {
	if a.rawProperties == nil {
		return 0
	}

	timeoutStr, exists := a.rawProperties["Timeout"]
	if !exists {
		return 0
	}

	timeoutInt, err := strconv.Atoi(timeoutStr)
	if err != nil {
		return 0
	}

	return time.Duration(timeoutInt) * time.Second
}

// GetRawProtocolProperties returns the entire map.
func (a *ProtocolConfig) GetRawProtocolProperties() map[string]string {
	if a.rawProperties == nil {
		return nil
	}
	return a.rawProperties
}
