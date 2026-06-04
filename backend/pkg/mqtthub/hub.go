package mqtthub

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
)

type WeatherMessage struct {
	StationID      string    `json:"station_id"`
	Temperature    float64   `json:"temperature"`
	Humidity       float64   `json:"humidity"`
	WindSpeed      float64   `json:"wind_speed"`
	SolarRadiation float64   `json:"solar_radiation"`
	Timestamp      time.Time `json:"-"`
	RawTopic       string    `json:"-"`
}

type MessageHandler func(msg *WeatherMessage) error

type Hub struct {
	client       mqtt.Client
	broker       string
	clientID     string
	topic        string
	handlers     []MessageHandler
	msgCh        chan *WeatherMessage
	bufferSize   int
	wg           sync.WaitGroup
	quitCh       chan struct{}
	connected    bool
	mu           sync.RWMutex
}

type Config struct {
	Broker     string
	ClientID   string
	Topic      string
	BufferSize int
}

func NewHub(cfg Config) *Hub {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 5000
	}
	if cfg.Topic == "" {
		cfg.Topic = "weather/stations/#"
	}

	return &Hub{
		broker:     cfg.Broker,
		clientID:   cfg.ClientID,
		topic:      cfg.Topic,
		bufferSize: cfg.BufferSize,
		msgCh:      make(chan *WeatherMessage, cfg.BufferSize),
		quitCh:     make(chan struct{}),
		handlers:   make([]MessageHandler, 0),
	}
}

func (h *Hub) RegisterHandler(handler MessageHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers = append(h.handlers, handler)
}

func (h *Hub) Connect() error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(h.broker)
	opts.SetClientID(h.clientID)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetCleanSession(true)
	opts.SetMessageChannelDepth(2000)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Printf("[MQTTHub] Connected to %s, subscribing to %s", h.broker, h.topic)
		h.setConnected(true)
		token := c.Subscribe(h.topic, 1, h.handleMessage)
		token.Wait()
		if token.Error() != nil {
			log.Printf("[MQTTHub] Subscribe error: %v", token.Error())
		}
	})

	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		log.Printf("[MQTTHub] Connection lost: %v, pending: %d", err, len(h.msgCh))
		h.setConnected(false)
	})

	h.client = mqtt.NewClient(opts)

	h.wg.Add(1)
	go h.dispatchLoop()

	token := h.client.Connect()
	token.Wait()
	return token.Error()
}

func (h *Hub) Disconnect() {
	close(h.quitCh)
	h.wg.Wait()
	if h.client != nil && h.IsConnected() {
		h.client.Disconnect(1000)
	}
}

func (h *Hub) IsConnected() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.connected
}

func (h *Hub) setConnected(v bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connected = v
}

func (h *Hub) QueueDepth() int {
	return len(h.msgCh)
}

func (h *Hub) handleMessage(_ mqtt.Client, msg mqtt.Message) {
	var wm WeatherMessage
	if err := json.Unmarshal(msg.Payload(), &wm); err != nil {
		log.Printf("[MQTTHub] Failed to unmarshal: %v", err)
		return
	}

	wm.RawTopic = msg.Topic()
	if wm.Timestamp.IsZero() {
		wm.Timestamp = time.Now()
	}

	select {
	case h.msgCh <- &wm:
	default:
		log.Printf("[MQTTHub] Buffer full (len=%d), dropping: station=%s", len(h.msgCh), wm.StationID)
	}
}

func (h *Hub) dispatchLoop() {
	defer h.wg.Done()
	log.Printf("[MQTTHub] Dispatch loop started, handlers=%d", len(h.handlers))

	for {
		select {
		case msg := <-h.msgCh:
			h.dispatchToHandlers(msg)
		case <-h.quitCh:
			log.Println("[MQTTHub] Dispatch loop stopped")
			return
		}
	}
}

func (h *Hub) dispatchToHandlers(msg *WeatherMessage) {
	h.mu.RLock()
	handlers := make([]MessageHandler, len(h.handlers))
	copy(handlers, h.handlers)
	h.mu.RUnlock()

	for i, handler := range handlers {
		if err := handler(msg); err != nil {
			log.Printf("[MQTTHub] Handler %d error for station %s: %v", i, msg.StationID, err)
		}
	}
}
