package connector

import (
	"fmt"
	"octopus-edge/pkg/parser"
	"octopus-edge/pkg/protocol"

	sdkModels "github.com/edgexfoundry/device-sdk-go/v2/pkg/models"
)

// Session manages the lifecycle of a connection.
type Session interface {
	Connect() error
	Disconnect() error
	IsConnected() bool
}

// Reader defines the standard read interface for all protocol plugins.
type Reader interface {
	ReadSingle(point *protocol.Resource) error
	ReadBatch(points []protocol.Resource) error
}

// Writer defines the standard write interface for all protocol plugins.
type Writer interface {
	WriteSingle(point *protocol.Resource) error
	WriteBatch(points []protocol.Resource) error // 连续写 n 个点
}

// ReadClient embeds the Reader interface with lifecycle management.
type ReadClient interface {
	Session
	Reader
}

// WriteClient embeds the Writer interface with lifecycle management.
type WriteClient interface {
	Session
	Writer
}

// RWClient embeds the Reader and Writer interfaces with lifecycle management.
type RWClient interface {
	Session
	Reader
	Writer
}

// ReceiverAdapter interface: all passive protocols must implement this
type ReceiverAdapter interface {
	Start() error
	Stop() error
}

// HandleReadSingle processes a single read command.
func HandleReadSingle(reader ReadClient, req sdkModels.CommandRequest) (*sdkModels.CommandValue, error) {
	res := protocol.NewResource(req)
	var err error
	if err = reader.ReadSingle(&res); err != nil {
		return nil, err
	}

	if res.Decoder != "" {
		res.Value, err = parser.DecodeRawData(res.Decoder, res.Value)
		if err != nil {
			return nil, err
		}
	}

	cv, err := sdkModels.NewCommandValue(req.DeviceResourceName, req.Type, res.Value)
	if err != nil {
		return nil, err
	}

	return cv, nil
}

// HandleReadBatch processes a batch read command.
func HandleReadBatch(reader ReadClient, req []sdkModels.CommandRequest) ([]*sdkModels.CommandValue, error) {
	resList := protocol.NewResourceN(req)
	var err error
	if err = reader.ReadBatch(resList); err != nil {
		return nil, err
	}

	cvList := make([]*sdkModels.CommandValue, 0, len(req))
	for i := range resList {
		if resList[i].Decoder != "" {
			resList[i].Value, err = parser.DecodeRawData(resList[i].Decoder, resList[i].Value)
			if err != nil {
				return cvList, fmt.Errorf("decode err, res: %s, decoder: %s, %v", resList[i].Name, resList[i].Decoder, err)
			}
		}

		cv, err := sdkModels.NewCommandValue(req[i].DeviceResourceName, req[i].Type, resList[i].Value)
		if err != nil {
			return cvList, fmt.Errorf("newCmd value err, res: %s, %v", resList[i].Name, err)
		}
		cvList = append(cvList, cv)
	}
	return cvList, nil
}

// HandleWriteSingle processes a single write command.
func HandleWriteSingle(writer Writer, req sdkModels.CommandRequest, param *sdkModels.CommandValue) error {
	res := protocol.NewResource(req)
	res.Value = param.Value

	if err := writer.WriteSingle(&res); err != nil {
		return fmt.Errorf("HandleWriteSingle: resource %s: %w", req.DeviceResourceName, err)
	}
	return nil
}

// HandleWriteBatch processes a batch write command.
func HandleWriteBatch(writer Writer, reqs []sdkModels.CommandRequest, params []*sdkModels.CommandValue) error {
	resList := protocol.NewResourceN(reqs)
	for i := range resList {
		if i < len(params) {
			resList[i].Value = params[i].Value
		}
	}

	if err := writer.WriteBatch(resList); err != nil {
		return fmt.Errorf("HandleWriteBatch: %w", err)
	}
	return nil
}
