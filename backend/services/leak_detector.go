package services

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"gas-monitoring-system/backend/algorithms"
	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
)

type LeakDetectorService struct {
	cfg             *config.LeakDetectionConfig
	readings        map[string][]algorithms.DetectorReading
	detectorInfo    map[string]*models.Detector
	currentLeaks    []*models.LeakSource
	windSpeed       float64
	windDir         float64
	temperature     float64
	mu              sync.RWMutex
	detectTicker    *time.Ticker
}

var LeakDetector *LeakDetectorService

func InitLeakDetector(cfg *config.LeakDetectionConfig) error {
	LeakDetector = &LeakDetectorService{
		cfg:          cfg,
		readings:     make(map[string][]algorithms.DetectorReading),
		detectorInfo: make(map[string]*models.Detector),
		currentLeaks: make([]*models.LeakSource, 0),
		windSpeed:    0.5,
		windDir:      90.0,
		temperature:  20.0,
	}

	if err := LeakDetector.loadDetectorInfo(); err != nil {
		log.Printf("Warning: failed to load detector info: %v", err)
	}

	LeakDetector.startDetectionLoop()

	return nil
}

func (l *LeakDetectorService) loadDetectorInfo() error {
	detectors, err := DB.GetAllDetectors()
	if err != nil {
		return err
	}

	for i := range detectors {
		l.detectorInfo[detectors[i].DeviceID] = &detectors[i]
	}

	log.Printf("Loaded %d detector info entries", len(l.detectorInfo))
	return nil
}

func (l *LeakDetectorService) AddReading(deviceID string, data *models.SensorData) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if data.WindSpeed > 0 {
		l.windSpeed = data.WindSpeed
	}
	if data.WindDir > 0 {
		l.windDir = data.WindDir
	}
	if data.Temperature > 0 {
		l.temperature = data.Temperature
	}

	detector, exists := l.detectorInfo[deviceID]
	if !exists {
		detector = &models.Detector{
			DeviceID: deviceID,
			Position: l.estimatePosition(deviceID),
			Latitude: 39.9042,
			Longitude: 116.4074,
		}
		l.detectorInfo[deviceID] = detector
	}

	reading := algorithms.DetectorReading{
		DeviceID:      deviceID,
		Position:      detector.Position,
		Latitude:      detector.Latitude,
		Longitude:     detector.Longitude,
		Concentration: data.Concentration,
		Timestamp:     data.Timestamp,
	}

	if _, exists := l.readings[deviceID]; !exists {
		l.readings[deviceID] = make([]algorithms.DetectorReading, 0, 60)
	}

	l.readings[deviceID] = append(l.readings[deviceID], reading)

	if len(l.readings[deviceID]) > 60 {
		l.readings[deviceID] = l.readings[deviceID][1:]
	}
}

func (l *LeakDetectorService) estimatePosition(deviceID string) float64 {
	numStr := deviceID[len(deviceID)-4:]
	var num int
	_, err := sscanf(numStr, "%d", &num)
	if err != nil {
		return 15000
	}
	return float64(num) * 100.0
}

func sscanf(str, format string, args ...interface{}) (int, error) {
	_, err := parseSscanf(str, format, args...)
	return len(args), err
}

func parseSscanf(str, format string, args ...interface{}) (string, error) {
	format = format[:len(format)-2]
	var num int
	for i, c := range str {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		} else {
			return str[i:], nil
		}
	}
	if len(args) > 0 {
		if p, ok := args[0].(*int); ok {
			*p = num
		}
	}
	return "", nil
}

func (l *LeakDetectorService) startDetectionLoop() {
	l.detectTicker = time.NewTicker(10 * time.Second)

	go func() {
		for range l.detectTicker.C {
			l.performDetection()
		}
	}()

	log.Println("Leak detection loop started")
}

func (l *LeakDetectorService) performDetection() {
	l.mu.RLock()

	allReadings := make([]algorithms.DetectorReading, 0)
	highConcCount := 0

	for _, readings := range l.readings {
		if len(readings) == 0 {
			continue
		}

		latest := readings[len(readings)-1]
		allReadings = append(allReadings, latest)

		if latest.Concentration > config.AppConfig.Alarm.Level1Threshold {
			highConcCount++
		}
	}

	model := &algorithms.GaussianPlumeModel{
		WindSpeed:            l.windSpeed,
		WindDir:              l.windDir,
		Temperature:          l.temperature,
		AtmosphericStability: 0.5,
	}

	l.mu.RUnlock()

	if highConcCount < 3 {
		l.mu.Lock()
		l.currentLeaks = make([]*models.LeakSource, 0)
		l.mu.Unlock()
		return
	}

	psoCfg := algorithms.DefaultPSOConfig()
	psoCfg.LoadFromConfig(l.cfg)

	result, err := algorithms.LocalizeLeakSource(allReadings, model, psoCfg)
	if err != nil {
		log.Printf("Leak detection error: %v", err)
		return
	}

	if result == nil || result.Confidence < 50 {
		l.mu.Lock()
		l.currentLeaks = make([]*models.LeakSource, 0)
		l.mu.Unlock()
		return
	}

	leakSource := &models.LeakSource{
		ID:              uuid.New(),
		Position:        result.Position,
		Latitude:        result.Latitude,
		Longitude:       result.Longitude,
		LeakRate:        result.LeakRate,
		Confidence:      result.Confidence,
		DiffusionRadius: result.DiffusionRadius,
		DetectedAt:      time.Now(),
	}

	l.mu.Lock()

	newLeak := true
	for i, existing := range l.currentLeaks {
		dist := abs(existing.Position - leakSource.Position)
		if dist < 500 {
			l.currentLeaks[i] = leakSource
			newLeak = false
			break
		}
	}

	if newLeak {
		l.currentLeaks = append(l.currentLeaks, leakSource)
		go l.saveLeakSource(leakSource)
		go BroadcastLeakSource(leakSource)
	}

	l.mu.Unlock()

	log.Printf("[LEAK] Detected leak at position %.1f, rate %.4f L/s, confidence %.1f%%, radius %.1fm",
		leakSource.Position, leakSource.LeakRate, leakSource.Confidence, leakSource.DiffusionRadius)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func (l *LeakDetectorService) saveLeakSource(leak *models.LeakSource) {
	query := `
		INSERT INTO leak_sources (id, position, latitude, longitude, leak_rate, confidence, diffusion_radius, detected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := DB.PG().Exec(context.Background(), query,
		leak.ID, leak.Position, leak.Latitude, leak.Longitude,
		leak.LeakRate, leak.Confidence, leak.DiffusionRadius, leak.DetectedAt)
	if err != nil {
		log.Printf("Failed to save leak source: %v", err)
	}
}

func (l *LeakDetectorService) GetCurrentLeaks() []*models.LeakSource {
	l.mu.RLock()
	defer l.mu.RUnlock()

	leaks := make([]*models.LeakSource, len(l.currentLeaks))
	copy(leaks, l.currentLeaks)
	return leaks
}

func (l *LeakDetectorService) ResolveLeak(leakID uuid.UUID) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for i, leak := range l.currentLeaks {
		if leak.ID == leakID {
			l.currentLeaks = append(l.currentLeaks[:i], l.currentLeaks[i+1:]...)

			query := `
				UPDATE leak_sources
				SET status = 'resolved', resolved_at = NOW()
				WHERE id = $1
			`
			_, err := DB.PG().Exec(context.Background(), query, leakID)
			return err
		}
	}

	return nil
}

func (l *LeakDetectorService) GetWindData() (float64, float64, float64) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.windSpeed, l.windDir, l.temperature
}

func (l *LeakDetectorService) Close() {
	if l.detectTicker != nil {
		l.detectTicker.Stop()
		log.Println("Leak detection loop stopped")
	}
}
