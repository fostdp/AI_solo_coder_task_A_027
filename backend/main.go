package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"urban-climate-monitor/api"
	"urban-climate-monitor/db"
	"urban-climate-monitor/pkg/alarmengine"
	"urban-climate-monitor/pkg/datacleaner"
	"urban-climate-monitor/pkg/heatisland"
	"urban-climate-monitor/pkg/mqtthub"
)

var (
	version   = "1.0.0"
	buildTime = "unknown"
)

type App struct {
	db         *db.DB
	mqttHub    *mqtthub.Hub
	cleaner    *datacleaner.Cleaner
	calculator *heatisland.Calculator
	alarmEng   *alarmengine.Engine
	router     *mux.Router
	server     *http.Server
	pprofSrv   *http.Server
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer *gzip.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.writer.Write(b)
}

func (w *gzipResponseWriter) Flush() {
	w.writer.Flush()
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, writer: gz}, r)
	})
}

func cacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else if strings.HasSuffix(r.URL.Path, ".png") ||
			strings.HasSuffix(r.URL.Path, ".jpg") ||
			strings.HasSuffix(r.URL.Path, ".ico") {
			w.Header().Set("Cache-Control", "public, max-age=604800")
		}
		next.ServeHTTP(w, r)
	})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"version":   version,
		"buildTime": buildTime,
		"time":      time.Now().Format(time.RFC3339),
	})
}

func main() {
	app := &App{}

	if err := app.init(); err != nil {
		log.Fatalf("Initialization failed: %v", err)
	}
	defer app.shutdown()

	app.start()

	app.waitForShutdown()
}

func (a *App) init() error {
	log.Println("=" * 60)
	log.Printf("  Urban Climate Monitor v%s (%s)", version, buildTime)
	log.Println("=" * 60)

	dbHost := getEnv("DB_HOST", "localhost")
	dbPort, _ := strconv.Atoi(getEnv("DB_PORT", "5432"))
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "postgres")
	dbName := getEnv("DB_NAME", "climate_monitor")

	mqttBroker := getEnv("MQTT_BROKER", "tcp://localhost:1883")
	mqttTopic := getEnv("MQTT_TOPIC", "weather/stations/#")
	mqttClientID := getEnv("MQTT_CLIENT_ID", "climate-backend")

	dingtalkURL := getEnv("DINGTALK_WEBHOOK_URL", "")
	serverPort := getEnv("SERVER_PORT", "8080")
	pprofEnabled := getEnv("PPROF_ENABLED", "true") == "true"

	var err error
	a.db, err = db.NewDB(dbHost, dbPort, dbUser, dbPassword, dbName)
	if err != nil {
		return err
	}
	log.Println("[OK] Database connected")

	a.mqttHub = mqtthub.NewHub(mqtthub.Config{
		Broker:     mqttBroker,
		ClientID:   mqttClientID,
		Topic:      mqttTopic,
		BufferSize: 5000,
	})
	log.Println("[OK] MQTT Hub initialized")

	a.cleaner = datacleaner.NewCleaner(a.db, 100, 5*time.Second)
	a.mqttHub.RegisterHandler(a.cleaner.GetMQTTHandler())
	log.Println("[OK] Data Cleaner initialized")

	a.calculator = heatisland.NewCalculator(a.db, heatisland.Config{
		Interval:            30 * time.Minute,
		MinBaselineStations: 2,
		WindowMinutes:       30,
	})
	log.Println("[OK] Heat Island Calculator initialized")

	a.alarmEng = alarmengine.NewEngine(a.db, 5*time.Minute)
	a.alarmEng.AddDingTalkNotifier(dingtalkURL)
	log.Println("[OK] Alarm Engine initialized")

	a.router = mux.NewRouter()
	a.router.HandleFunc("/health", healthHandler).Methods("GET")
	handler := api.NewHandler(a.db)
	handler.RegisterRoutes(a.router)

	fs := http.FileServer(http.Dir("./static"))
	a.router.PathPrefix("/").Handler(cacheMiddleware(gzipMiddleware(fs)))
	log.Println("[OK] API Router initialized")

	a.server = &http.Server{
		Addr:         ":" + serverPort,
		Handler:      corsMiddleware(a.router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if pprofEnabled {
		a.pprofSrv = &http.Server{
			Addr:         ":6060",
			Handler:      pprofHandler(),
			ReadTimeout:  60 * time.Second,
			WriteTimeout: 60 * time.Second,
		}
		go func() {
			log.Printf("[OK] pprof server listening on :6060")
			if err := a.pprofSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("pprof server error: %v", err)
			}
		}()
	}

	return nil
}

func pprofHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return mux
}

func (a *App) start() {
	if err := a.mqttHub.Connect(); err != nil {
		log.Fatalf("[FATAL] MQTT connect failed: %v", err)
	}
	log.Println("[OK] MQTT connected")

	a.cleaner.Start()
	log.Println("[OK] Data Cleaner started")

	a.calculator.Start()
	log.Println("[OK] Heat Island Calculator started")

	a.alarmEng.Start()
	log.Println("[OK] Alarm Engine started")

	go func() {
		log.Printf("[OK] HTTP server listening on %s", a.server.Addr)
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] HTTP server failed: %v", err)
		}
	}()

	log.Println("=" * 60)
	log.Println("  All components started successfully")
	log.Println("=" * 60)
}

func (a *App) shutdown() {
	log.Println("\nShutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.server.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	log.Println("[OK] HTTP server stopped")

	if a.pprofSrv != nil {
		if err := a.pprofSrv.Shutdown(ctx); err != nil {
			log.Printf("pprof server shutdown error: %v", err)
		}
		log.Println("[OK] pprof server stopped")
	}

	a.alarmEng.Stop()
	log.Println("[OK] Alarm Engine stopped")

	a.cleaner.Stop()
	log.Println("[OK] Data Cleaner stopped")

	a.mqttHub.Disconnect()
	log.Println("[OK] MQTT disconnected")

	a.db.Close()
	log.Println("[OK] Database disconnected")

	log.Println("=" * 60)
	log.Println("  System shutdown complete")
	log.Println("=" * 60)
}

func (a *App) waitForShutdown() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
