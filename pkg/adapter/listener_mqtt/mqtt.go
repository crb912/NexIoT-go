package listener_mqtt

import (
	"devices-iot-go/pkg/model"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

const (
	maxDataChannelSize = 256
	tcpListenerID      = "mqtt-tcp"
)

// messageHook intercepts published MQTT messages via the HookBase pattern.
type messageHook struct {
	mqtt.HookBase
	ch chan<- model.ReceiveEvent
}

func (h *messageHook) ID() string { return "mqtt-listener-message-hook" }

func (h *messageHook) Provides(b byte) bool {
	return b == mqtt.OnPublish
}

func (h *messageHook) OnPublish(_ *mqtt.Client, pk packets.Packet) (packets.Packet, error) {
	event := model.ReceiveEvent{
		Source:    "mqtt",
		EventName: pk.TopicName,
		EventTime: time.Now(),
		EventData: pk.Payload,
	}
	select {
	case h.ch <- event:
	default:
		// channel full — drop silently
	}
	return pk, nil
}

// MqttReceiver runs an embedded MQTT broker on Host:Port.
// Devices publish directly to this broker; all incoming messages are
// intercepted via the OnPublish hook and pushed into AsyncData.
type MqttReceiver struct {
	Host      string
	Port      uint16
	AsyncData chan model.ReceiveEvent

	mu  sync.Mutex
	srv *mqtt.Server
}

// NewMqttReceiver creates an MqttReceiver. Call Start() to launch the
// embedded broker and begin intercepting messages.
func NewMqttReceiver(host string, port uint16) *MqttReceiver {
	if port == 0 {
		port = 1883
	}
	return &MqttReceiver{
		Host:      host,
		Port:      port,
		AsyncData: make(chan model.ReceiveEvent, maxDataChannelSize),
	}
}

// Start launches the embedded MQTT broker and blocks until the listener
// stops or an irrecoverable error occurs.
func (r *MqttReceiver) Start() error {
	r.mu.Lock()
	r.srv = mqtt.New(nil)
	r.mu.Unlock()

	// Allow anonymous connections — devices don't need credentials.
	if err := r.srv.AddHook(new(auth.AllowHook), nil); err != nil {
		return fmt.Errorf("mqtt broker add auth hook: %w", err)
	}

	// Intercept every published message.
	if err := r.srv.AddHook(&messageHook{ch: r.AsyncData}, nil); err != nil {
		return fmt.Errorf("mqtt broker add message hook: %w", err)
	}

	addr := fmt.Sprintf("%s:%d", r.Host, r.Port)
	tcp := listeners.NewTCP(listeners.Config{
		ID:      tcpListenerID,
		Address: addr,
	})
	if err := r.srv.AddListener(tcp); err != nil {
		return fmt.Errorf("mqtt broker add listener %s: %w", addr, err)
	}

	// Serve blocks; returns when the listener is closed.
	return r.srv.Serve()
}

// Stop shuts down the embedded broker.
func (r *MqttReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.srv == nil {
		return nil
	}

	// Close closes all listeners and stops the event loop.
	if err := r.srv.Close(); err != nil {
		return fmt.Errorf("mqtt broker close: %w", err)
	}
	r.srv = nil
	return nil
}
