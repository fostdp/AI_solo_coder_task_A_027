package datacleaner

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"urban-climate-monitor/db"
	"urban-climate-monitor/pkg/mqtthub"
)

type ValidationRule func(msg *mqtthub.WeatherMessage) error

type Cleaner struct {
	db           *db.DB
	batchSize    int
	flushTimeout time.Duration
	msgCh        chan *mqtthub.WeatherMessage
	batch        []*db.WeatherData
	rules        []ValidationRule
	quitCh       chan struct{}
	wg           sync.WaitGroup
	stats        Stats
	mu           sync.Mutex
}

type Stats struct {
	Received    int64 `json:"received"`
	Validated   int64 `json:"validated"`
	Dropped     int64 `json:"dropped"`
	Inserted    int64 `json:"inserted"`
	BatchCount  int64 `json:"batch_count"`
	LastError   string `json:"last_error"`
}

func NewCleaner(database *db.DB, batchSize int, flushTimeout time.Duration) *Cleaner {
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushTimeout <= 0 {
		flushTimeout = 5 * time.Second
	}

	c := &Cleaner{
		db:           database,
		batchSize:    batchSize,
		flushTimeout: flushTimeout,
		msgCh:        make(chan *mqtthub.WeatherMessage, 2000),
		batch:        make([]*db.WeatherData, 0, batchSize),
		quitCh:       make(chan struct{}),
	}

	c.registerDefaultRules()
	return c
}

func (c *Cleaner) registerDefaultRules() {
	c.rules = append(c.rules,
		c.validateTemperature,
		c.validateHumidity,
		c.validateWindSpeed,
		c.validateSolarRadiation,
		c.validateStationID,
	)
}

func (c *Cleaner) validateTemperature(msg *mqtthub.WeatherMessage) error {
	if msg.Temperature < -50 || msg.Temperature > 70 {
		return fmt.Errorf("temperature out of range: %.2f", msg.Temperature)
	}
	return nil
}

func (c *Cleaner) validateHumidity(msg *mqtthub.WeatherMessage) error {
	if msg.Humidity < 0 || msg.Humidity > 100 {
		return fmt.Errorf("humidity out of range: %.2f", msg.Humidity)
	}
	return nil
}

func (c *Cleaner) validateWindSpeed(msg *mqtthub.WeatherMessage) error {
	if msg.WindSpeed < 0 || msg.WindSpeed > 200 {
		return fmt.Errorf("wind_speed out of range: %.2f", msg.WindSpeed)
	}
	return nil
}

func (c *Cleaner) validateSolarRadiation(msg *mqtthub.WeatherMessage) error {
	if msg.SolarRadiation < 0 || msg.SolarRadiation > 2000 {
		return fmt.Errorf("solar_radiation out of range: %.2f", msg.SolarRadiation)
	}
	return nil
}

func (c *Cleaner) validateStationID(msg *mqtthub.WeatherMessage) error {
	if msg.StationID == "" {
		return fmt.Errorf("empty station_id")
	}
	if len(msg.StationID) > 20 {
		return fmt.Errorf("station_id too long: %s", msg.StationID)
	}
	return nil
}

func (c *Cleaner) AddRule(rule ValidationRule) {
	c.rules = append(c.rules, rule)
}

func (c *Cleaner) HandleMessage(msg *mqtthub.WeatherMessage) error {
	c.mu.Lock()
	c.stats.Received++
	c.mu.Unlock()

	for _, rule := range c.rules {
		if err := rule(msg); err != nil {
			c.mu.Lock()
			c.stats.Dropped++
			c.stats.LastError = fmt.Sprintf("station=%s: %v", msg.StationID, err)
			c.mu.Unlock()
			log.Printf("[DataCleaner] Dropped message from %s: %v", msg.StationID, err)
			return nil
		}
	}

	select {
	case c.msgCh <- msg:
		c.mu.Lock()
		c.stats.Validated++
		c.mu.Unlock()
	default:
		c.mu.Lock()
		c.stats.Dropped++
		c.mu.Unlock()
		log.Printf("[DataCleaner] Input buffer full, dropping message from %s", msg.StationID)
	}
	return nil
}

func (c *Cleaner) Start() {
	c.wg.Add(1)
	go c.batchWriter()
	log.Printf("[DataCleaner] Started, batchSize=%d, flushTimeout=%v", c.batchSize, c.flushTimeout)
}

func (c *Cleaner) Stop() {
	close(c.quitCh)
	c.wg.Wait()
}

func (c *Cleaner) batchWriter() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.flushTimeout)
	defer ticker.Stop()

	for {
		select {
		case msg := <-c.msgCh:
			wd := &db.WeatherData{
				Time:           msg.Timestamp,
				StationID:      msg.StationID,
				Temperature:    msg.Temperature,
				Humidity:       msg.Humidity,
				WindSpeed:      msg.WindSpeed,
				SolarRadiation: msg.SolarRadiation,
			}
			c.batch = append(c.batch, wd)

			if len(c.batch) >= c.batchSize {
				c.flush()
			}
		case <-ticker.C:
			if len(c.batch) > 0 {
				c.flush()
			}
		case <-c.quitCh:
			if len(c.batch) > 0 {
				c.flush()
			}
			log.Println("[DataCleaner] Stopped")
			return
		}
	}
}

func (c *Cleaner) flush() {
	count := len(c.batch)
	if count == 0 {
		return
	}

	if err := c.db.BatchInsertWeatherData(c.batch); err != nil {
		log.Printf("[DataCleaner] Batch insert failed: %v, falling back to single insert", err)
		for _, wd := range c.batch {
			if err := c.db.InsertWeatherData(wd); err != nil {
				log.Printf("[DataCleaner] Single insert failed for %s: %v", wd.StationID, err)
			} else {
				c.mu.Lock()
				c.stats.Inserted++
				c.mu.Unlock()
			}
		}
	} else {
		c.mu.Lock()
		c.stats.Inserted += int64(count)
		c.stats.BatchCount++
		c.mu.Unlock()
	}

	c.batch = c.batch[:0]
}

func (c *Cleaner) GetStats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

func (c *Cleaner) GetMQTTHandler() mqtthub.MessageHandler {
	return func(msg *mqtthub.WeatherMessage) error {
		return c.HandleMessage(msg)
	}
}

func NormalizeStationID(id string) string {
	return strings.ToUpper(strings.TrimSpace(id))
}
