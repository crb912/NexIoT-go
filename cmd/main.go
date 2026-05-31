// This package provides a simple example of a device service.
package main

import (
	"hermes-edge/internal/driver"

	"github.com/edgexfoundry/device-sdk-go/v2/pkg/startup"
)

// serviceName is the unique identifier of the device service in the EdgeX system.
var serviceName = "hermes-iot-edge"

// Global version for device-sdk-go, can be replaced by Makefile
var serviceVersion = "dev-d9d13b7"

func main() {
	cd := driver.CompositeDriver{}
	startup.Bootstrap(serviceName, serviceVersion, &cd)
}
