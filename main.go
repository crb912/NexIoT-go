// cmd/main.go
// 程序入口，将两个协议驱动组合后交给 EdgeX device-sdk-go 的 Bootstrap 启动。
package main

import (
	"better-iot-edge/internal/driver"
	modbusdriver "better-iot-edge/internal/driver/modbus"

	"github.com/edgexfoundry/device-sdk-go/v2/pkg/startup"
)

const (
	serviceName    = "edge-gateway"
	serviceVersion = "1.0.0"
)

//func test() {
//	var devices []Device
//	err := config.Unmarshal(confg.Modbus, &devices, resCheck)
//	_ = config.Unmarshal(confg.SNMP, &devices)
//	err = config.UnmarshalFromfile(pathToMyFile, &devices, resCheck)
//	_ = config.Unmarshal(confg.Modbus, &devices)
//	err = config.MarshalToFile(pathToMyFile, &devices)
//
//}

func main() {
	modbusDrv := modbusdriver.NewDriver()

	composite := driver.NewCompositeDriver(modbusDrv)
	//fmt.Println(pkg.GetDeviceConfigPath(pkg.EnvProd, pkg.Modbus))
	startup.Bootstrap(serviceName, serviceVersion, composite)
}
