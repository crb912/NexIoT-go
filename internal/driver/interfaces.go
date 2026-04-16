package driver

import (
	"github.com/edgexfoundry/device-sdk-go/v2/pkg/interfaces"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/models"
)

// Lifecycle  定义了协议驱动生命周期的接口， 该接口必须实现。
type Lifecycle interface {
	Initialize(sdk interfaces.DeviceServiceSDK) error
	Start() error
	Stop(force bool) error
}

// CommandReader 可选接口：命令处理（只读设备可只实现 Reader）
type CommandReader interface {
	HandleReadCommands(
		deviceName string,
		protocols map[string]ProtocolProperties,
		reqs []CommandRequest,
	) ([]*CommandValue, error)
}

type CommandWriter interface {
	HandleWriteCommands(
		deviceName string,
		protocols map[string]ProtocolProperties,
		reqs []CommandRequest,
		params []*CommandValue,
	) error
}

// 职责三：设备事件（可选，用独立接口隔离）
type DeviceWatcher interface {
	AddDevice(deviceName string, protocols map[string]ProtocolProperties, adminState AdminState) error
	UpdateDevice(deviceName string, protocols map[string]ProtocolProperties, adminState AdminState) error
	RemoveDevice(deviceName string, protocols map[string]ProtocolProperties) error
}

// SDK 内部通过类型断言按需调用，而不是强制实现
type ProtocolDriver interface {
	Lifecycle
	CommandReader
	CommandWriter
}

// 接口示例

type ProtocolDriver interface {
	// SDK 完全初始化后调用，用于驱动的后置初始化逻辑
	Initialize(sdk interfaces.DeviceServiceSDK) error

	// SDK 完全初始化后调用（较新版本新增），放置初始化完成后的业务逻辑
	Start() error

	// 处理设备读取命令（GET），从物理设备采集数据
	HandleReadCommands(
		deviceName string,
		protocols map[string]models.ProtocolProperties,
		reqs []models.CommandRequest,
	) ([]*models.CommandValue, error)

	// 处理设备写入命令（PUT/SET），向物理设备下发指令
	HandleWriteCommands(
		deviceName string,
		protocols map[string]models.ProtocolProperties,
		reqs []models.CommandRequest,
		params []*models.CommandValue,
	) error

	// 有新设备被添加到该服务时触发
	AddDevice(
		deviceName string,
		protocols map[string]models.ProtocolProperties,
		adminState models.AdminState,
	) error

	// 设备信息被更新时触发
	UpdateDevice(
		deviceName string,
		protocols map[string]models.ProtocolProperties,
		adminState models.AdminState,
	) error

	// 设备被删除时触发，用于释放连接等资源
	RemoveDevice(deviceName string, protocols map[string]models.ProtocolProperties) error

	// 服务关闭时调用，用于清理资源
	Stop(force bool) error
}
