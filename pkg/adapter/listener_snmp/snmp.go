// Package listener_snmp provides an SNMP trap listener implementing the Listener interface.
// It listens on UDP for incoming SNMP v1/v2c/v3 traps (informs) from devices and
// converts them into JSON ReceiveEvent payloads for the EdgeX pipeline.
package listener_snmp

import (
	"next-iot-go/pkg/model"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

const (
	maxDataChannelSize = 256
	defaultPort        = 162
)

// SnmpReceiver listens for incoming SNMP traps and pushes them into AsyncData.
type SnmpReceiver struct {
	Host      string
	Port      uint16
	Community string
	AsyncData chan model.ReceiveEvent

	mu       sync.Mutex
	listener *gosnmp.TrapListener
}

// NewSnmpReceiver creates an SnmpReceiver. Call Start() to begin listening.
func NewSnmpReceiver(host string, port uint16, community string) *SnmpReceiver {
	if port == 0 {
		port = defaultPort
	}
	if community == "" {
		community = "public"
	}
	return &SnmpReceiver{
		Host:      host,
		Port:      port,
		Community: community,
		AsyncData: make(chan model.ReceiveEvent, maxDataChannelSize),
	}
}

// trapPayload represents the JSON envelope sent to processReceiveData.
type trapPayload struct {
	SourceAddr string                 `json:"source_addr"`
	Enterprise string                 `json:"enterprise"`
	Generic    int                    `json:"generic"`
	Specific   int                    `json:"specific"`
	Timestamp  uint                   `json:"timestamp"`
	Data       map[string]interface{} `json:"data"`
}

// Start begins listening for SNMP traps. Blocks until Stop() is called.
func (r *SnmpReceiver) Start() error {
	r.mu.Lock()
	r.listener = gosnmp.NewTrapListener()
	r.listener.Params = &gosnmp.GoSNMP{
		Port:      r.Port,
		Transport: "udp",
		Version:   gosnmp.Version2c,
		Community: r.Community,
	}
	r.listener.OnNewTrap = r.onTrap
	r.mu.Unlock()

	addr := fmt.Sprintf("%s:%d", r.Host, r.Port)
	if err := r.listener.Listen(addr); err != nil {
		return fmt.Errorf("snmp trap listener on %s: %w", addr, err)
	}
	return nil
}

// Stop closes the trap listener.
func (r *SnmpReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.listener == nil {
		return nil
	}

	r.listener.Close()
	r.listener = nil
	return nil
}

// onTrap is called by gosnmp when a trap is received.
func (r *SnmpReceiver) onTrap(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) {
	source := addr.String()
	data := make(map[string]interface{}, len(packet.Variables))
	for _, pdu := range packet.Variables {
		// gosnmp returns OIDs with dots (e.g. ".1.3.6.1.2"); EdgeX resource names
		// disallow dots, so replace them with hyphens and strip the leading dot.
		oid := strings.ReplaceAll(strings.TrimPrefix(pdu.Name, "."), ".", "-")
		data[oid] = convertSnmpValue(pdu.Value)
	}

	payload := trapPayload{
		SourceAddr: source,
		Enterprise: packet.Enterprise,
		Generic:    packet.GenericTrap,
		Specific:   packet.SpecificTrap,
		Timestamp:  packet.Timestamp,
		Data:       data,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}

	event := model.ReceiveEvent{
		Source:    "snmp",
		EventName: "snmp_trap",
		EventTime: time.Now(),
		EventData: payloadBytes,
	}

	select {
	case r.AsyncData <- event:
	default:
	}
}

// convertSnmpValue converts gosnmp value types to JSON-compatible Go types.
func convertSnmpValue(v interface{}) interface{} {
	switch val := v.(type) {
	case []byte:
		return string(val)
	case net.IP:
		return val.String()
	case nil:
		return nil
	default:
		return val
	}
}
