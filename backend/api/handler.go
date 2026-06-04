package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"urban-climate-monitor/db"
)

type Handler struct {
	db *db.DB
}

func NewHandler(database *db.DB) *Handler {
	return &Handler{db: database}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/api/stations", h.GetStations).Methods("GET")
	r.HandleFunc("/api/stations/{id}/latest", h.GetStationLatest).Methods("GET")
	r.HandleFunc("/api/stations/{id}/24h", h.GetStation24h).Methods("GET")
	r.HandleFunc("/api/stations/{id}/heat-index", h.GetStationHeatIndex).Methods("GET")
	r.HandleFunc("/api/heatmap", h.GetHeatmap).Methods("GET")
	r.HandleFunc("/api/heat-island/current", h.GetHeatIslandCurrent).Methods("GET")
	r.HandleFunc("/api/heat-island/trend", h.GetHeatIslandTrend).Methods("GET")
	r.HandleFunc("/api/alerts", h.GetAlerts).Methods("GET")
	r.HandleFunc("/api/weather/latest", h.GetLatestWeatherAll).Methods("GET")
}

func (h *Handler) GetStations(w http.ResponseWriter, r *http.Request) {
	stations, err := h.db.GetAllStations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stations)
}

func (h *Handler) GetStationLatest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	stationID := vars["id"]

	data, err := h.db.GetLatestWeatherForStation(stationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) GetStation24h(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	stationID := vars["id"]

	data, err := h.db.GetWeather24h(stationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) GetStationHeatIndex(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	stationID := vars["id"]

	idx, err := h.db.GetStationHeatIndex(stationID)
	if err != nil {
		idx = 0
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"station_id":  stationID,
		"heat_index":  idx,
	})
}

type HeatmapPoint struct {
	StationID   string  `json:"station_id"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Temperature float64 `json:"temperature"`
	HeatIndex   float64 `json:"heat_index"`
	IsBaseline  bool    `json:"is_baseline"`
}

func (h *Handler) GetHeatmap(w http.ResponseWriter, r *http.Request) {
	stations, err := h.db.GetAllStations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	weatherMap, err := h.db.GetLatestWeatherAllStations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var points []HeatmapPoint
	for _, s := range stations {
		pt := HeatmapPoint{
			StationID:  s.ID,
			Latitude:   s.Latitude,
			Longitude:  s.Longitude,
			IsBaseline: s.IsBaseline,
		}
		if w, ok := weatherMap[s.ID]; ok {
			pt.Temperature = w.Temperature
			idx, _ := h.db.GetStationHeatIndex(s.ID)
			pt.HeatIndex = idx
		}
		points = append(points, pt)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}

func (h *Handler) GetHeatIslandCurrent(w http.ResponseWriter, r *http.Request) {
	intensity, err := h.db.GetLatestHeatIslandIntensity()
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(intensity)
}

func (h *Handler) GetHeatIslandTrend(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days := 7
	if daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 && d <= 30 {
			days = d
		}
	}

	since := time.Now().AddDate(0, 0, -days)
	data, err := h.db.GetHeatIslandIntensitySince(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) GetAlerts(w http.ResponseWriter, r *http.Request) {
	hoursStr := r.URL.Query().Get("hours")
	hours := 24
	if hoursStr != "" {
		if h2, err := strconv.Atoi(hoursStr); err == nil && h2 > 0 {
			hours = h2
		}
	}

	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	alerts, err := h.db.GetRecentAlerts(since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(alerts)
}

func (h *Handler) GetLatestWeatherAll(w http.ResponseWriter, r *http.Request) {
	data, err := h.db.GetLatestWeatherAllStations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
