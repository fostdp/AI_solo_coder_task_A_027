package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type DB struct {
	*sql.DB
}

type Station struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	IsBaseline bool    `json:"is_baseline"`
}

type WeatherData struct {
	Time           time.Time `json:"time"`
	StationID      string    `json:"station_id"`
	Temperature    float64   `json:"temperature"`
	Humidity       float64   `json:"humidity"`
	WindSpeed      float64   `json:"wind_speed"`
	SolarRadiation float64   `json:"solar_radiation"`
}

type HeatIslandIntensity struct {
	Time            time.Time `json:"time"`
	UrbanAvgTemp    float64   `json:"urban_avg_temp"`
	BaselineAvgTemp float64   `json:"baseline_avg_temp"`
	Intensity       float64   `json:"intensity"`
}

type StationHeatIndex struct {
	Time       time.Time `json:"time"`
	StationID  string    `json:"station_id"`
	HeatIndex  float64   `json:"heat_index"`
}

type Alert struct {
	ID        int       `json:"id"`
	Time      time.Time `json:"time"`
	AlertType string    `json:"alert_type"`
	Level     string    `json:"level"`
	StationID string    `json:"station_id"`
	Message   string    `json:"message"`
	Notified  bool      `json:"notified"`
}

func NewDB(host string, port int, user, password, dbname string) (*DB, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Connected to TimescaleDB successfully")
	return &DB{db}, nil
}

func (d *DB) InsertWeatherData(data *WeatherData) error {
	_, err := d.Exec(`
		INSERT INTO weather_data (time, station_id, temperature, humidity, wind_speed, solar_radiation)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		data.Time, data.StationID, data.Temperature, data.Humidity, data.WindSpeed, data.SolarRadiation)
	return err
}

func (d *DB) BatchInsertWeatherData(batch []*WeatherData) error {
	if len(batch) == 0 {
		return nil
	}

	valueStr := strings.Builder{}
	args := make([]interface{}, 0, len(batch)*6)

	for i, data := range batch {
		if i > 0 {
			valueStr.WriteString(",")
		}
		base := i * 6
		fmt.Fprintf(&valueStr, "($%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6)
		args = append(args, data.Time, data.StationID, data.Temperature,
			data.Humidity, data.WindSpeed, data.SolarRadiation)
	}

	query := fmt.Sprintf(`
		INSERT INTO weather_data (time, station_id, temperature, humidity, wind_speed, solar_radiation)
		VALUES %s`, valueStr.String())

	_, err := d.Exec(query, args...)
	return err
}

func (d *DB) GetAllStations() ([]Station, error) {
	rows, err := d.Query("SELECT id, name, latitude, longitude, is_baseline FROM stations ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stations []Station
	for rows.Next() {
		var s Station
		if err := rows.Scan(&s.ID, &s.Name, &s.Latitude, &s.Longitude, &s.IsBaseline); err != nil {
			return nil, err
		}
		stations = append(stations, s)
	}
	return stations, nil
}

func (d *DB) GetBaselineStations() ([]Station, error) {
	rows, err := d.Query("SELECT id, name, latitude, longitude, is_baseline FROM stations WHERE is_baseline = TRUE")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stations []Station
	for rows.Next() {
		var s Station
		if err := rows.Scan(&s.ID, &s.Name, &s.Latitude, &s.Longitude, &s.IsBaseline); err != nil {
			return nil, err
		}
		stations = append(stations, s)
	}
	return stations, nil
}

func (d *DB) GetLatestWeatherForStation(stationID string) (*WeatherData, error) {
	var data WeatherData
	err := d.QueryRow(`
		SELECT time, station_id, temperature, humidity, wind_speed, solar_radiation
		FROM weather_data WHERE station_id = $1 ORDER BY time DESC LIMIT 1`,
		stationID).Scan(&data.Time, &data.StationID, &data.Temperature, &data.Humidity, &data.WindSpeed, &data.SolarRadiation)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func (d *DB) GetLatestWeatherAllStations() (map[string]*WeatherData, error) {
	rows, err := d.Query(`
		SELECT DISTINCT ON (station_id) time, station_id, temperature, humidity, wind_speed, solar_radiation
		FROM weather_data ORDER BY station_id, time DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*WeatherData)
	for rows.Next() {
		var data WeatherData
		if err := rows.Scan(&data.Time, &data.StationID, &data.Temperature, &data.Humidity, &data.WindSpeed, &data.SolarRadiation); err != nil {
			return nil, err
		}
		result[data.StationID] = &data
	}
	return result, nil
}

func (d *DB) GetWeather24h(stationID string) ([]WeatherData, error) {
	rows, err := d.Query(`
		SELECT time, station_id, temperature, humidity, wind_speed, solar_radiation
		FROM weather_data WHERE station_id = $1 AND time > NOW() - INTERVAL '24 hours'
		ORDER BY time ASC`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []WeatherData
	for rows.Next() {
		var d2 WeatherData
		if err := rows.Scan(&d2.Time, &d2.StationID, &d2.Temperature, &d2.Humidity, &d2.WindSpeed, &d2.SolarRadiation); err != nil {
			return nil, err
		}
		data = append(data, d2)
	}
	return data, nil
}

func (d *DB) GetAvgTempSince(stationIDs []string, since time.Time) (float64, error) {
	if len(stationIDs) == 0 {
		return 0, fmt.Errorf("GetAvgTempSince: empty station IDs list")
	}

	quoted := make([]string, len(stationIDs))
	for i, id := range stationIDs {
		quoted[i] = "'" + strings.ReplaceAll(id, "'", "''") + "'"
	}
	list := strings.Join(quoted, ",")
	query := fmt.Sprintf(`
		SELECT COALESCE(AVG(temperature), 0), COUNT(*) FROM weather_data
		WHERE station_id IN (%s) AND time > $1`, list)
	rows, err := d.Query(query, since)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var avg float64
	var count int
	if rows.Next() {
		rows.Scan(&avg, &count)
	}
	if count == 0 {
		return 0, fmt.Errorf("no data found for any of %d stations in time window", len(stationIDs))
	}
	return avg, nil
}

func (d *DB) CheckBaselineDataCoverage(since time.Time) (validIDs []string, missingIDs []string, err error) {
	stations, err := d.GetBaselineStations()
	if err != nil {
		return nil, nil, err
	}

	for _, s := range stations {
		var count int
		qErr := d.QueryRow(`
			SELECT COUNT(*) FROM weather_data
			WHERE station_id = $1 AND time > $2`, s.ID, since).Scan(&count)
		if qErr != nil {
			return nil, nil, qErr
		}
		if count > 0 {
			validIDs = append(validIDs, s.ID)
		} else {
			missingIDs = append(missingIDs, s.ID)
		}
	}
	return validIDs, missingIDs, nil
}

func (d *DB) InsertHeatIslandIntensity(h *HeatIslandIntensity) error {
	_, err := d.Exec(`
		INSERT INTO heat_island_intensity (time, urban_avg_temp, baseline_avg_temp, intensity)
		VALUES ($1, $2, $3, $4)`, h.Time, h.UrbanAvgTemp, h.BaselineAvgTemp, h.Intensity)
	return err
}

func (d *DB) InsertStationHeatIndex(h *StationHeatIndex) error {
	_, err := d.Exec(`
		INSERT INTO station_heat_index (time, station_id, heat_index)
		VALUES ($1, $2, $3)`, h.Time, h.StationID, h.HeatIndex)
	return err
}

func (d *DB) GetHeatIslandTrend7Days() ([]HeatIslandIntensity, error) {
	rows, err := d.Query(`
		SELECT time, urban_avg_temp, baseline_avg_temp, intensity
		FROM heat_island_intensity WHERE time > NOW() - INTERVAL '7 days'
		ORDER BY time ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []HeatIslandIntensity
	for rows.Next() {
		var h HeatIslandIntensity
		if err := rows.Scan(&h.Time, &h.UrbanAvgTemp, &h.BaselineAvgTemp, &h.Intensity); err != nil {
			return nil, err
		}
		data = append(data, h)
	}
	return data, nil
}

func (d *DB) GetStationHeatIndex(stationID string) (float64, error) {
	var idx float64
	err := d.QueryRow(`
		SELECT COALESCE(heat_index, 0) FROM station_heat_index
		WHERE station_id = $1 ORDER BY time DESC LIMIT 1`, stationID).Scan(&idx)
	if err != nil {
		return 0, err
	}
	return idx, nil
}

func (d *DB) GetStationsWithTempAbove(threshold float64, duration time.Duration) ([]string, error) {
	since := time.Now().Add(-duration)
	minReadings := int(duration.Minutes() / 5)
	rows, err := d.Query(`
		SELECT station_id FROM weather_data
		WHERE temperature > $1 AND time > $2
		GROUP BY station_id
		HAVING COUNT(*) >= $3`,
		threshold, since, minReadings)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, nil
}

func (d *DB) InsertAlert(a *Alert) error {
	var sid interface{}
	if a.StationID == "" {
		sid = nil
	} else {
		sid = a.StationID
	}
	_, err := d.Exec(`
		INSERT INTO alerts (time, alert_type, level, station_id, message, notified)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		a.Time, a.AlertType, a.Level, sid, a.Message, a.Notified)
	return err
}

func (d *DB) GetRecentAlerts(since time.Time) ([]Alert, error) {
	rows, err := d.Query(`
		SELECT id, time, alert_type, level, COALESCE(station_id, ''), message, notified
		FROM alerts WHERE time > $1 ORDER BY time DESC LIMIT 100`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.Time, &a.AlertType, &a.Level, &a.StationID, &a.Message, &a.Notified); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

func (d *DB) HasRecentAlert(alertType, stationID string, since time.Time) (bool, error) {
	var count int
	err := d.QueryRow(`
		SELECT COUNT(*) FROM alerts
		WHERE alert_type = $1 AND COALESCE(station_id, '') = $2 AND time > $3`,
		alertType, stationID, since).Scan(&count)
	return count > 0, err
}

func (d *DB) GetLatestHeatIslandIntensity() (*HeatIslandIntensity, error) {
	var h HeatIslandIntensity
	err := d.QueryRow(`
		SELECT time, urban_avg_temp, baseline_avg_temp, intensity
		FROM heat_island_intensity ORDER BY time DESC LIMIT 1`).
		Scan(&h.Time, &h.UrbanAvgTemp, &h.BaselineAvgTemp, &h.Intensity)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (d *DB) GetHeatIslandIntensitySince(since time.Time) ([]HeatIslandIntensity, error) {
	rows, err := d.Query(`
		SELECT time, urban_avg_temp, baseline_avg_temp, intensity
		FROM heat_island_intensity WHERE time > $1 ORDER BY time ASC`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []HeatIslandIntensity
	for rows.Next() {
		var h HeatIslandIntensity
		if err := rows.Scan(&h.Time, &h.UrbanAvgTemp, &h.BaselineAvgTemp, &h.Intensity); err != nil {
			return nil, err
		}
		data = append(data, h)
	}
	return data, nil
}

func (d *DB) GetAvgTempFromMaterializedView(stationIDs []string, since time.Time) (float64, error) {
	if len(stationIDs) == 0 {
		return 0, fmt.Errorf("empty station IDs list")
	}

	quoted := make([]string, len(stationIDs))
	for i, id := range stationIDs {
		quoted[i] = "'" + strings.ReplaceAll(id, "'", "''") + "'"
	}
	list := strings.Join(quoted, ",")
	query := fmt.Sprintf(`
		SELECT COALESCE(AVG(avg_temp), 0), COUNT(*) FROM weather_30min_agg
		WHERE station_id IN (%s) AND bucket_start > $1`, list)

	rows, err := d.Query(query, since)
	if err != nil {
		return 0, fmt.Errorf("materialized view query: %w", err)
	}
	defer rows.Close()

	var avg float64
	var count int
	if rows.Next() {
		if err := rows.Scan(&avg, &count); err != nil {
			return 0, err
		}
	}
	if count == 0 {
		return 0, fmt.Errorf("no data in materialized view for %d stations", len(stationIDs))
	}
	return avg, nil
}


