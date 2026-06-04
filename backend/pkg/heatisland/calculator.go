package heatisland

import (
	"fmt"
	"log"
	"math"
	"time"

	"urban-climate-monitor/db"
)

type Calculator struct {
	db              *db.DB
	interval        time.Duration
	minBaselineStations int
	windowMinutes   int
}

type Config struct {
	Interval             time.Duration
	MinBaselineStations  int
	WindowMinutes        int
}

type Result struct {
	Time            time.Time `json:"time"`
	UrbanAvgTemp    float64   `json:"urban_avg_temp"`
	BaselineAvgTemp float64   `json:"baseline_avg_temp"`
	Intensity       float64   `json:"intensity"`
	StationCount    int       `json:"station_count"`
	BaselineCount   int       `json:"baseline_count"`
	Method          string    `json:"method"`
}

type StationIndex struct {
	Time      time.Time `json:"time"`
	StationID string    `json:"station_id"`
	HeatIndex float64   `json:"heat_index"`
}

func NewCalculator(database *db.DB, cfg Config) *Calculator {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Minute
	}
	if cfg.MinBaselineStations <= 0 {
		cfg.MinBaselineStations = 2
	}
	if cfg.WindowMinutes <= 0 {
		cfg.WindowMinutes = 30
	}

	return &Calculator{
		db:                    database,
		interval:              cfg.Interval,
		minBaselineStations:   cfg.MinBaselineStations,
		windowMinutes:         cfg.WindowMinutes,
	}
}

func (c *Calculator) Calculate() (*Result, error) {
	since := time.Now().Add(-time.Duration(c.windowMinutes) * time.Minute)

	validBaselineIDs, missingBaselineIDs, err := c.db.CheckBaselineDataCoverage(since)
	if err != nil {
		return nil, fmt.Errorf("check baseline coverage: %w", err)
	}

	if len(missingBaselineIDs) > 0 {
		log.Printf("[HeatIsland] WARNING: Missing baseline data for: %v", missingBaselineIDs)
	}

	if len(validBaselineIDs) < c.minBaselineStations {
		return nil, fmt.Errorf("insufficient baseline data: got %d, need at least %d",
			len(validBaselineIDs), c.minBaselineStations)
	}

	stations, err := c.db.GetAllStations()
	if err != nil {
		return nil, fmt.Errorf("get stations: %w", err)
	}

	var urbanIDs []string
	for _, s := range stations {
		if !s.IsBaseline {
			urbanIDs = append(urbanIDs, s.ID)
		}
	}

	if len(urbanIDs) == 0 {
		return nil, fmt.Errorf("no urban stations found")
	}

	urbanAvg, err := c.db.GetAvgTempFromMaterializedView(urbanIDs, since)
	if err != nil {
		log.Printf("[HeatIsland] Materialized view unavailable, falling back to raw query: %v", err)
		urbanAvg, err = c.db.GetAvgTempSince(urbanIDs, since)
		if err != nil {
			return nil, fmt.Errorf("get urban avg temp: %w", err)
		}
	}

	baselineAvg, err := c.db.GetAvgTempFromMaterializedView(validBaselineIDs, since)
	if err != nil {
		log.Printf("[HeatIsland] Materialized view unavailable, falling back to raw query: %v", err)
		baselineAvg, err = c.db.GetAvgTempSince(validBaselineIDs, since)
		if err != nil {
			return nil, fmt.Errorf("get baseline avg temp: %w", err)
		}
	}

	if !isValidTemperature(urbanAvg) || !isValidTemperature(baselineAvg) {
		return nil, fmt.Errorf("abnormal temperature values: urban=%.2f, baseline=%.2f",
			urbanAvg, baselineAvg)
	}

	intensity := urbanAvg - baselineAvg

	result := &Result{
		Time:            time.Now(),
		UrbanAvgTemp:    round2(urbanAvg),
		BaselineAvgTemp: round2(baselineAvg),
		Intensity:       round2(intensity),
		StationCount:    len(urbanIDs),
		BaselineCount:   len(validBaselineIDs),
		Method:          "materialized_view",
	}

	hi := &db.HeatIslandIntensity{
		Time:            result.Time,
		UrbanAvgTemp:    result.UrbanAvgTemp,
		BaselineAvgTemp: result.BaselineAvgTemp,
		Intensity:       result.Intensity,
	}
	if err := c.db.InsertHeatIslandIntensity(hi); err != nil {
		return result, fmt.Errorf("insert heat island intensity: %w", err)
	}

	if err := c.calculateStationIndexes(baselineAvg); err != nil {
		log.Printf("[HeatIsland] Failed to calculate station indexes: %v", err)
	}

	log.Printf("[HeatIsland] Calculated: intensity=%.2f (urban=%.2f, baseline=%.2f) using %d/%d baselines",
		result.Intensity, result.UrbanAvgTemp, result.BaselineAvgTemp,
		result.BaselineCount, len(validBaselineIDs)+len(missingBaselineIDs))

	return result, nil
}

func (c *Calculator) calculateStationIndexes(baselineAvg float64) error {
	latestWeather, err := c.db.GetLatestWeatherAllStations()
	if err != nil {
		return fmt.Errorf("get latest weather: %w", err)
	}

	stations, err := c.db.GetAllStations()
	if err != nil {
		return fmt.Errorf("get stations: %w", err)
	}

	now := time.Now()
	for _, s := range stations {
		if s.IsBaseline {
			continue
		}
		if w, ok := latestWeather[s.ID]; ok {
			idx := &db.StationHeatIndex{
				Time:      now,
				StationID: s.ID,
				HeatIndex: round2(w.Temperature - baselineAvg),
			}
			if err := c.db.InsertStationHeatIndex(idx); err != nil {
				log.Printf("[HeatIsland] Failed to store index for %s: %v", s.ID, err)
			}
		}
	}
	return nil
}

func (c *Calculator) Start() {
	ticker := time.NewTicker(c.interval)
	go func() {
		log.Printf("[HeatIsland] Calculator started, interval=%v", c.interval)
		for range ticker.C {
			if _, err := c.Calculate(); err != nil {
				log.Printf("[HeatIsland] Calculation error: %v", err)
			}
		}
	}()
}

func (c *Calculator) RefreshMaterializedView() error {
	_, err := c.db.Exec("REFRESH MATERIALIZED VIEW CONCURRENTLY weather_30min_agg")
	if err != nil {
		_, err = c.db.Exec("REFRESH MATERIALIZED VIEW weather_30min_agg")
	}
	if err != nil {
		return fmt.Errorf("refresh materialized view: %w", err)
	}
	log.Println("[HeatIsland] Materialized view refreshed")
	return nil
}

func isValidTemperature(t float64) bool {
	return t >= -50 && t <= 70
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
