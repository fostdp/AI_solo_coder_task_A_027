package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/eclipse/paho.mqtt.golang"
)

type Station struct {
	ID         string
	Name       string
	IsBaseline bool
}

type WeatherPayload struct {
	StationID      string  `json:"station_id"`
	Temperature    float64 `json:"temperature"`
	Humidity       float64 `json:"humidity"`
	WindSpeed      float64 `json:"wind_speed"`
	SolarRadiation float64 `json:"solar_radiation"`
	Timestamp      int64   `json:"timestamp"`
}

type StationSimulator struct {
	Station         Station
	BaseTemp        float64
	CurrentTemp     float64
	CurrentHumidity float64
	CurrentWind     float64
	CurrentSolar    float64
	HeatBias        float64
	mu              sync.Mutex
}

func NewStationSimulator(station Station) *StationSimulator {
	s := &StationSimulator{
		Station: station,
	}

	if station.IsBaseline {
		s.BaseTemp = 28 + rand.Float64()*4
		s.HeatBias = 0
	} else {
		s.BaseTemp = 30 + rand.Float64()*6
		s.HeatBias = 1 + rand.Float64()*5
	}

	s.CurrentTemp = s.BaseTemp
	s.CurrentHumidity = 50 + rand.Float64()*30
	s.CurrentWind = 0.5 + rand.Float64()*4
	s.CurrentSolar = 100 + rand.Float64()*800

	return s
}

func (s *StationSimulator) Update(hour float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	diurnalTemp := math.Sin((hour-6)/24*2*math.Pi) * 5
	if hour < 6 || hour > 18 {
		diurnalTemp = -3 + math.Sin((hour-6)/24*2*math.Pi)*3
	}

	solarFactor := 0.0
	if hour >= 6 && hour <= 18 {
		solarFactor = math.Sin((hour-6)/12*math.Pi) * 8
	}

	targetTemp := s.BaseTemp + diurnalTemp + solarFactor*0.5 + s.HeatBias
	s.CurrentTemp += (targetTemp - s.CurrentTemp) * 0.1
	s.CurrentTemp += (rand.Float64() - 0.5) * 0.8

	targetHumidity := 65 - diurnalTemp*2 - s.HeatBias*2
	s.CurrentHumidity += (targetHumidity - s.CurrentHumidity) * 0.1
	s.CurrentHumidity += (rand.Float64() - 0.5) * 3
	s.CurrentHumidity = math.Max(20, math.Min(95, s.CurrentHumidity))

	targetWind := 1.5 + math.Sin(hour/12*math.Pi)*2 + rand.Float64()*2
	s.CurrentWind += (targetWind - s.CurrentWind) * 0.2
	s.CurrentWind = math.Max(0, s.CurrentWind)

	if hour >= 6 && hour <= 18 {
		s.CurrentSolar = math.Sin((hour-6)/12*math.Pi) * (600 + rand.Float64()*400)
	} else {
		s.CurrentSolar = rand.Float64() * 10
	}
	s.CurrentSolar = math.Max(0, s.CurrentSolar)
}

func (s *StationSimulator) Payload() WeatherPayload {
	s.mu.Lock()
	defer s.mu.Unlock()

	return WeatherPayload{
		StationID:      s.Station.ID,
		Temperature:    round2(s.CurrentTemp),
		Humidity:       round2(s.CurrentHumidity),
		WindSpeed:      round2(s.CurrentWind),
		SolarRadiation: round2(s.CurrentSolar),
		Timestamp:      time.Now().Unix(),
	}
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}

func main() {
	broker := getEnv("MQTT_BROKER", "tcp://localhost:1883")
	intervalStr := getEnv("REPORT_INTERVAL", "300")
	intervalSec := 300
	if v, err := strconv.Atoi(intervalStr); err == nil && v > 0 {
		intervalSec = v
	}

	rand.Seed(time.Now().UnixNano())

	urbanStations := make([]Station, 50)
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("U%03d", i+1)
		urbanStations[i] = Station{ID: id, Name: fmt.Sprintf("城区站%d", i+1), IsBaseline: false}
	}

	baselineStations := []Station{
		{ID: "B001", Name: "郊区基准站-东", IsBaseline: true},
		{ID: "B002", Name: "郊区基准站-南", IsBaseline: true},
		{ID: "B003", Name: "郊区基准站-西", IsBaseline: true},
	}

	allStations := append(baselineStations, urbanStations...)
	simulators := make([]*StationSimulator, len(allStations))
	for i, s := range allStations {
		simulators[i] = NewStationSimulator(s)
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID("weather-simulator-" + fmt.Sprintf("%d", rand.Intn(10000)))
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", token.Error())
	}
	defer client.Disconnect(1000)

	log.Printf("Weather simulator started, reporting every %d seconds to %s", intervalSec, broker)

	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		hour := float64(time.Now().Hour()) + float64(time.Now().Minute())/60.0

		for _, sim := range simulators {
			sim.Update(hour)
			payload := sim.Payload()

			data, err := json.Marshal(payload)
			if err != nil {
				log.Printf("Failed to marshal payload for station %s: %v", payload.StationID, err)
				continue
			}

			topic := "weather/stations/" + payload.StationID
			token := client.Publish(topic, 1, false, data)
			token.Wait()
			if token.Error() != nil {
				log.Printf("Failed to publish for station %s: %v", payload.StationID, token.Error())
			}
		}

		log.Printf("Published weather data for %d stations at %s", len(simulators), time.Now().Format("15:04:05"))
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
