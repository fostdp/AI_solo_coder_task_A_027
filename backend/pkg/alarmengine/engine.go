package alarmengine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"urban-climate-monitor/db"
)

type AlertType string

const (
	TypeHighTemp   AlertType = "high_temp"
	TypeHeatIsland AlertType = "heat_island"
)

type AlertLevel string

const (
	LevelWarning  AlertLevel = "warning"
	LevelCritical AlertLevel = "critical"
)

type AlertRule struct {
	Type            AlertType
	Level           AlertLevel
	Threshold       float64
	Duration        time.Duration
	DedupWindow     time.Duration
	Enabled         bool
}

type Notifier interface {
	Send(alert *db.Alert) error
}

type DingTalkNotifier struct {
	WebhookURL string
	Enabled    bool
	RetryCount int
	RetryDelay time.Duration
}

type Engine struct {
	db        *db.DB
	rules     []AlertRule
	notifiers []Notifier
	checkInterval time.Duration
	quitCh    chan struct{}
	wg        sync.WaitGroup
	stats     Stats
	mu        sync.Mutex
}

type Stats struct {
	ChecksPerformed int64 `json:"checks_performed"`
	AlertsTriggered int64 `json:"alerts_triggered"`
	NotifSent       int64 `json:"notifications_sent"`
	NotifFailed     int64 `json:"notifications_failed"`
}

func NewEngine(database *db.DB, checkInterval time.Duration) *Engine {
	if checkInterval <= 0 {
		checkInterval = 5 * time.Minute
	}

	e := &Engine{
		db:            database,
		checkInterval: checkInterval,
		quitCh:        make(chan struct{}),
		rules:         defaultRules(),
	}
	return e
}

func defaultRules() []AlertRule {
	return []AlertRule{
		{
			Type:        TypeHighTemp,
			Level:       LevelWarning,
			Threshold:   40.0,
			Duration:    30 * time.Minute,
			DedupWindow: 30 * time.Minute,
			Enabled:     true,
		},
		{
			Type:        TypeHeatIsland,
			Level:       LevelCritical,
			Threshold:   5.0,
			Duration:    1 * time.Hour,
			DedupWindow: 1 * time.Hour,
			Enabled:     true,
		},
	}
}

func (e *Engine) AddNotifier(n Notifier) {
	e.notifiers = append(e.notifiers, n)
}

func (e *Engine) AddDingTalkNotifier(webhookURL string) {
	if webhookURL == "" {
		return
	}
	e.notifiers = append(e.notifiers, &DingTalkNotifier{
		WebhookURL: webhookURL,
		Enabled:    true,
		RetryCount: 3,
		RetryDelay: 5 * time.Second,
	})
}

func (e *Engine) Start() {
	e.wg.Add(1)
	go e.runCheckLoop()
	log.Printf("[AlarmEngine] Started, checkInterval=%v, rules=%d, notifiers=%d",
		e.checkInterval, len(e.rules), len(e.notifiers))
}

func (e *Engine) Stop() {
	close(e.quitCh)
	e.wg.Wait()
}

func (e *Engine) runCheckLoop() {
	defer e.wg.Done()

	ticker := time.NewTicker(e.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.runChecks()
		case <-e.quitCh:
			log.Println("[AlarmEngine] Stopped")
			return
		}
	}
}

func (e *Engine) runChecks() {
	e.mu.Lock()
	e.stats.ChecksPerformed++
	e.mu.Unlock()

	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}
		e.checkRule(rule)
	}
}

func (e *Engine) checkRule(rule AlertRule) {
	switch rule.Type {
	case TypeHighTemp:
		e.checkHighTempRule(rule)
	case TypeHeatIsland:
		e.checkHeatIslandRule(rule)
	}
}

func (e *Engine) checkHighTempRule(rule AlertRule) {
	stationIDs, err := e.db.GetStationsWithTempAbove(rule.Threshold, rule.Duration)
	if err != nil {
		log.Printf("[AlarmEngine] Error checking high temp: %v", err)
		return
	}

	for _, sid := range stationIDs {
		if e.isDuplicate(rule.Type, sid, rule.DedupWindow) {
			continue
		}

		var stationName string
		e.db.QueryRow("SELECT name FROM stations WHERE id = $1", sid).Scan(&stationName)

		alert := &db.Alert{
			Time:      time.Now(),
			AlertType: string(rule.Type),
			Level:     string(rule.Level),
			StationID: sid,
			Message:   fmt.Sprintf("高温告警：站点 %s(%s) 温度超过 %.1f℃ 已持续 %d 分钟",
				sid, stationName, rule.Threshold, int(rule.Duration.Minutes())),
			Notified:  false,
		}

		e.processAlert(alert)
	}
}

func (e *Engine) checkHeatIslandRule(rule AlertRule) {
	latest, err := e.db.GetLatestHeatIslandIntensity()
	if err != nil {
		return
	}

	if latest.Intensity <= rule.Threshold {
		return
	}

	intensities, err := e.db.GetHeatIslandIntensitySince(time.Now().Add(-rule.Duration))
	if err != nil {
		log.Printf("[AlarmEngine] Error getting heat island history: %v", err)
		return
	}

	allAbove := true
	for _, i := range intensities {
		if i.Intensity <= rule.Threshold {
			allAbove = false
			break
		}
	}

	if !allAbove || len(intensities) == 0 {
		return
	}

	if e.isDuplicate(rule.Type, "", rule.DedupWindow) {
		return
	}

	alert := &db.Alert{
		Time:      time.Now(),
		AlertType: string(rule.Type),
		Level:     string(rule.Level),
		StationID: "",
		Message:   fmt.Sprintf("热岛告警：城区热岛强度 %.2f℃ 超过 %.1f℃ 已持续 %d 分钟",
			latest.Intensity, rule.Threshold, int(rule.Duration.Minutes())),
		Notified:  false,
	}

	e.processAlert(alert)
}

func (e *Engine) isDuplicate(alertType AlertType, stationID string, window time.Duration) bool {
	exists, err := e.db.HasRecentAlert(string(alertType), stationID, time.Now().Add(-window))
	if err != nil {
		log.Printf("[AlarmEngine] Error checking duplicate alert: %v", err)
		return false
	}
	return exists
}

func (e *Engine) processAlert(alert *db.Alert) {
	if err := e.db.InsertAlert(alert); err != nil {
		log.Printf("[AlarmEngine] Failed to insert alert: %v", err)
		return
	}

	e.mu.Lock()
	e.stats.AlertsTriggered++
	e.mu.Unlock()

	allSuccess := true
	for _, notifier := range e.notifiers {
		if err := notifier.Send(alert); err != nil {
			log.Printf("[AlarmEngine] Failed to send notification: %v", err)
			allSuccess = false
			e.mu.Lock()
			e.stats.NotifFailed++
			e.mu.Unlock()
		} else {
			e.mu.Lock()
			e.stats.NotifSent++
			e.mu.Unlock()
		}
	}

	if allSuccess && len(e.notifiers) > 0 {
		alert.Notified = true
	}

	log.Printf("[AlarmEngine] Alert triggered: type=%s, level=%s, msg=%s",
		alert.AlertType, alert.Level, alert.Message)
}

func (n *DingTalkNotifier) Send(alert *db.Alert) error {
	if !n.Enabled || n.WebhookURL == "" {
		return nil
	}

	msg := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": fmt.Sprintf("[%s] %s", alert.Level, alert.Message),
		},
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < n.RetryCount; attempt++ {
		if attempt > 0 {
			time.Sleep(n.RetryDelay)
		}

		resp, err := http.Post(n.WebhookURL, "application/json", bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return nil
		}
		lastErr = fmt.Errorf("status %d", resp.StatusCode)
	}

	return fmt.Errorf("failed after %d attempts: %w", n.RetryCount, lastErr)
}

func (e *Engine) GetStats() Stats {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stats
}
