// Package opcua provides an OPC UA protocol adapter implementing the RWClient interface.
// It supports read/write operations on OPC UA server nodes identified by NodeID.
package opcua

import (
	"context"
	"devices-iot-go/pkg/conv"
	"devices-iot-go/pkg/model"
	"fmt"
	"sync"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

// OpcuaClient holds connection parameters and the underlying OPC UA session.
// It implements the protocol.RWClient interface (Session + Reader + Writer).
type OpcuaClient struct {
	EndPoint     string
	ProtocolType model.ProtocolType
	Timeout      time.Duration

	// Security settings parsed from args
	SecurityMode   ua.MessageSecurityMode
	SecurityPolicy string
	Username       string
	Password       string
	CertFile       string
	KeyFile        string

	mu        sync.Mutex
	connected bool
	client    *opcua.Client
}

// NewOpcuaClient constructs an OpcuaClient from the generic args map.
// Supported args keys:
//
//	SecurityMode   — "None" (default), "Sign", "SignAndEncrypt"
//	SecurityPolicy — "None" (default), or full policy URI
//	Username       — user name for authentication (empty = anonymous)
//	Password       — password for authentication
//	CertFile       — path to client certificate PEM file
//	KeyFile        — path to client private key PEM file
func NewOpcuaClient(endpoint string, pt model.ProtocolType, defaultTimeout time.Duration, args map[string]string) (*OpcuaClient, error) {
	c := &OpcuaClient{
		EndPoint:       endpoint,
		ProtocolType:   pt,
		Timeout:        defaultTimeout,
		SecurityMode:   ua.MessageSecurityModeNone,
		SecurityPolicy: "None",
	}
	if args == nil {
		return c, nil
	}

	if v, ok := args["SecurityMode"]; ok {
		switch v {
		case "Sign":
			c.SecurityMode = ua.MessageSecurityModeSign
		case "SignAndEncrypt":
			c.SecurityMode = ua.MessageSecurityModeSignAndEncrypt
		default:
			c.SecurityMode = ua.MessageSecurityModeNone
		}
	}
	if v, ok := args["SecurityPolicy"]; ok && v != "" {
		c.SecurityPolicy = v
	}
	if v, ok := args["Username"]; ok {
		c.Username = v
	}
	if v, ok := args["Password"]; ok {
		c.Password = v
	}
	if v, ok := args["CertFile"]; ok {
		c.CertFile = v
	}
	if v, ok := args["KeyFile"]; ok {
		c.KeyFile = v
	}
	return c, nil
}

// Connect establishes the OPC UA session.
func (o *OpcuaClient) Connect() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.client != nil {
		return nil
	}

	opts := []opcua.Option{
		opcua.SecurityMode(o.SecurityMode),
		opcua.SecurityPolicy(o.SecurityPolicy),
		opcua.RequestTimeout(o.Timeout),
	}

	if o.Username != "" {
		opts = append(opts, opcua.AuthUsername(o.Username, o.Password))
	} else {
		opts = append(opts, opcua.AuthAnonymous())
	}

	if o.CertFile != "" {
		opts = append(opts, opcua.CertificateFile(o.CertFile))
	}
	if o.KeyFile != "" {
		opts = append(opts, opcua.PrivateKeyFile(o.KeyFile))
	}

	ctx := context.Background()
	client, err := opcua.NewClient(o.EndPoint, opts...)
	if err != nil {
		return fmt.Errorf("failed to create OPC UA client: %w", err)
	}

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect OPC UA: %w", err)
	}

	o.client = client
	o.connected = true
	return nil
}

// Disconnect closes the OPC UA session.
func (o *OpcuaClient) Disconnect() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.client == nil {
		o.connected = false
		return nil
	}

	err := o.client.Close(context.Background())
	o.connected = false
	o.client = nil
	if err != nil {
		return fmt.Errorf("failed to disconnect OPC UA: %w", err)
	}
	return nil
}

// ReadSingle reads a single OPC UA node.
// res.Address must be a string like "ns=3;i=1001" or "ns=2;s=Temperature".
func (o *OpcuaClient) ReadSingle(res *model.Resource) error {
	nodeID, err := parseNodeID(res.Address)
	if err != nil {
		return fmt.Errorf("ReadSingle parse NodeID: %w", err)
	}

	val, err := o.client.Node(nodeID).Value(context.Background())
	if err != nil {
		return fmt.Errorf("ReadSingle node %v: %w", res.Address, err)
	}

	res.Value = val.Value()
	return nil
}

// ReadBatch reads multiple OPC UA nodes in a single Read request.
// It aggregates all point addresses into one ReadRequest for efficiency.
func (o *OpcuaClient) ReadBatch(points []model.Resource) error {
	if len(points) == 0 {
		return nil
	}

	readIDs := make([]*ua.ReadValueID, len(points))
	for i, p := range points {
		nodeID, err := parseNodeID(p.Address)
		if err != nil {
			return fmt.Errorf("ReadBatch parse NodeID at index %d: %w", i, err)
		}
		readIDs[i] = &ua.ReadValueID{
			NodeID:      nodeID,
			AttributeID: ua.AttributeIDValue,
		}
	}

	req := &ua.ReadRequest{
		NodesToRead: readIDs,
	}

	resp, err := o.client.Read(context.Background(), req)
	if err != nil {
		return fmt.Errorf("ReadBatch: %w", err)
	}
	if resp == nil {
		return fmt.Errorf("ReadBatch: nil response from server")
	}

	for i, dv := range resp.Results {
		if dv == nil || dv.Status != ua.StatusOK {
			status := ua.StatusBadUnexpectedError
			if dv != nil {
				status = dv.Status
			}
			return fmt.Errorf("ReadBatch point %q: status %v", points[i].Name, status)
		}
		if dv.Value != nil {
			points[i].Value = dv.Value.Value()
		}
	}
	return nil
}

// WriteSingle writes a single value to an OPC UA node.
func (o *OpcuaClient) WriteSingle(res *model.Resource) error {
	nodeID, err := parseNodeID(res.Address)
	if err != nil {
		return fmt.Errorf("WriteSingle parse NodeID: %w", err)
	}

	value, err := convertForWrite(res.Value, res.Type)
	if err != nil {
		return fmt.Errorf("WriteSingle: %w", err)
	}

	return o.writeNode(nodeID, value, res.Name)
}

// WriteBatch writes multiple values to OPC UA nodes sequentially.
func (o *OpcuaClient) WriteBatch(points []model.Resource) error {
	if len(points) == 0 {
		return nil
	}

	for _, p := range points {
		nodeID, err := parseNodeID(p.Address)
		if err != nil {
			return fmt.Errorf("WriteBatch point %q parse NodeID: %w", p.Name, err)
		}

		value, err := convertForWrite(p.Value, p.Type)
		if err != nil {
			return fmt.Errorf("WriteBatch point %q: %w", p.Name, err)
		}

		if err := o.writeNode(nodeID, value, p.Name); err != nil {
			return err
		}
	}
	return nil
}

// writeNode writes a single value to an OPC UA node using the Write service.
func (o *OpcuaClient) writeNode(nodeID *ua.NodeID, value any, name string) error {
	req := &ua.WriteRequest{
		NodesToWrite: []*ua.WriteValue{
			{
				NodeID:      nodeID,
				AttributeID: ua.AttributeIDValue,
				Value:       &ua.DataValue{Value: ua.MustVariant(value)},
			},
		},
	}
	resp, err := o.client.Write(context.Background(), req)
	if err != nil {
		return fmt.Errorf("write node %v: %w", nodeID, err)
	}
	for _, r := range resp.Results {
		if r != ua.StatusOK {
			return fmt.Errorf("write node %q status: %v", name, r)
		}
	}
	return nil
}

// parseNodeID converts the resource address (string) to a *ua.NodeID.
// Supported formats: "ns=3;i=1001" (numeric), "ns=2;s=Temperature" (string).
func parseNodeID(address any) (*ua.NodeID, error) {
	s, ok := address.(string)
	if !ok {
		return nil, fmt.Errorf("address must be a string, got %T", address)
	}
	nodeID, err := ua.ParseNodeID(s)
	if err != nil {
		return nil, fmt.Errorf("invalid NodeID %q: %w", s, err)
	}
	return nodeID, nil
}

// convertForWrite converts the raw value to the target OPC UA type.
func convertForWrite(value any, resType string) (any, error) {
	switch resType {
	case "Float64":
		return conv.ToFloat64(value)
	case "Int32":
		return conv.ToInt32(value)
	default:
		return nil, fmt.Errorf("unsupported type for write: %s", resType)
	}
}
