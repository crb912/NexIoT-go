// This package provides a simple example of a device service.
package main

import (
	"better-iot-edge/internal/driver"
	"fmt"

	"github.com/edgexfoundry/device-sdk-go/v2/pkg/startup"
)

// Global version for device-sdk-go, can be replaced by Makefile or post-commit hook.
var serviceName = "better-iot-edge"
var serviceVersion = "dev-81b77be" // AUTO_GENERATED

func main() {
	composite := driver.CompositeDriver{}
	startup.Bootstrap(serviceName, serviceVersion, &composite)
	fmt.Println("IoT edge stopped")
}
