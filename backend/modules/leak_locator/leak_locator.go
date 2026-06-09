package leak_locator

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"gas-monitoring-system/backend/algorithms"
	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/services"
)

type LeakLocator struct {
	cfg          *config.LeakLocatorConfig
	running      bool
	mu           sync.RWMutex

	dataChan     <-chan models.ValidatedData

	readings     map[string][]algorithms.DetectorReading
	detectorInfo map[string]*models.Detector
	currentLeaks []*models.LeakSource

	windSpeed    float64
	windDir      float64
	temperature  float64

	detectTicker *time.Ticker
	leakChan     chan<- *models.LeakSource
}

func NewLeakLocator(cfg *config.LeakLocatorConfig) *LeakLocator {
	return &LeakLocator{
		cfg:          cfg,
		readings:     make(map[string][]algorithms.DetectorReading),
		detectorInfo: make(map[string]*models.Detector),
		currentLeaks: make([]*models.LeakSource, 0),
		windSpeed:    0.5,
		windDir:      90.0,
		temperature:  20.0,
	}
}

func (ll *LeakLocator) SetChannels(dataChan <-chan models.ValidatedData, leakChan chan<- *models.LeakSource) {
	ll.dataChan = dataChan
	ll.leakChan = leakChan
}

func (ll *LeakLocator) Start() error {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	if ll.running {
		return nil
	}

	if err := ll.loadDetectorInfo(); err != nil {
		log.Printf("[LeakLocator] Warning: 加载检测器信息失败: %v", err)
	}

	ll.running = true
	ll.detectTicker = time.NewTicker(ll.cfg.DetectionInterval)

	go ll.dataReceiver()
	go ll.detectionLoop()

	log.Println("[LeakLocator] 泄漏源定位模块启动")
	return nil
}

func (ll *LeakLocator) Stop() {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	if !ll.running {
		return
	}

	ll.running = false
	if ll.detectTicker != nil {
		ll.detectTicker.Stop()
	}

	log.Println("[LeakLocator] 泄漏源定位模块停止")
}

func (ll *LeakLocator) loadDetectorInfo() error {
	if services.DB == nil {
		return nil
	}

	detectors, err := services.DB.GetAllDetectors()
	if err != nil {
		return err
	}

	for i := range detectors {
		ll.detectorInfo[detectors[i].DeviceID] = &detectors[i]
	}

	log.Printf("[LeakLocator] 已加载 %d 个检测器信息", len(ll.detectorInfo))
	return nil
}

func (ll *LeakLocator) dataReceiver() {
	for {
		ll.mu.RLock()
		running := ll.running
		ll.mu.RUnlock()

		if !running {
			return
		}

		select {
		case data, ok := <-ll.dataChan:
			if !ok {
				return
			}
			ll.addReading(data)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (ll *LeakLocator) addReading(data models.ValidatedData) {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	rawData := data.RawData
	if rawData != nil {
		if rawData.WindSpeed > 0 {
			ll.windSpeed = rawData.WindSpeed
		}
		if rawData.WindDir > 0 {
			ll.windDir = rawData.WindDir
		}
		if rawData.Temperature > 0 {
			ll.temperature = rawData.Temperature
		}
	}

	detector, exists := ll.detectorInfo[data.DeviceID]
	if !exists {
		detector = &models.Detector{
			DeviceID:  data.DeviceID,
			Position:  ll.estimatePosition(data.DeviceID),
			Latitude:  39.9042,
			Longitude: 116.4074,
		}
		ll.detectorInfo[data.DeviceID] = detector
	}

	reading := algorithms.DetectorReading{
		DeviceID:      data.DeviceID,
		Position:      detector.Position,
		Latitude:      detector.Latitude,
		Longitude:     detector.Longitude,
		Concentration: data.Concentration,
		Timestamp:     data.Timestamp,
	}

	if _, exists := ll.readings[data.DeviceID]; !exists {
		ll.readings[data.DeviceID] = make([]algorithms.DetectorReading, 0, ll.cfg.MaxReadingsPerDetector)
	}

	ll.readings[data.DeviceID] = append(ll.readings[data.DeviceID], reading)

	if len(ll.readings[data.DeviceID]) > ll.cfg.MaxReadingsPerDetector {
		ll.readings[data.DeviceID] = ll.readings[data.DeviceID][1:]
	}
}

func (ll *LeakLocator) estimatePosition(deviceID string) float64 {
	if len(deviceID) < 4 {
		return 15000
	}
	numStr := deviceID[len(deviceID)-4:]
	var num int
	for _, c := range numStr {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		}
	}
	return float64(num) * 100.0
}

func (ll *LeakLocator) detectionLoop() {
	for range ll.detectTicker.C {
		ll.mu.RLock()
		running := ll.running
		ll.mu.RUnlock()

		if !running {
			return
		}

		ll.performDetection()
	}
}

func (ll *LeakLocator) performDetection() {
	ll.mu.RLock()

	allReadings := make([]algorithms.DetectorReading, 0)
	highConcCount := 0
	threshold := ll.cfg.HighConcentrationThreshold

	for _, readings := range ll.readings {
		if len(readings) == 0 {
			continue
		}

		latest := readings[len(readings)-1]
		allReadings = append(allReadings, latest)

		if latest.Concentration > threshold {
			highConcCount++
		}
	}

	model := &algorithms.GaussianPlumeModel{
		WindSpeed:            ll.windSpeed,
		WindDir:              ll.windDir,
		Temperature:          ll.temperature,
		AtmosphericStability: ll.cfg.AtmosphericStability,
	}

	ll.mu.RUnlock()

	if highConcCount < ll.cfg.MinHighConcReadings {
		ll.mu.Lock()
		ll.currentLeaks = make([]*models.LeakSource, 0)
		ll.mu.Unlock()
		return
	}

	psoCfg := algorithms.PSOConfig{
		NumParticles:    ll.cfg.PSONumParticles,
		MaxIterations:   ll.cfg.PSOMaxIterations,
		InertiaWeight:   ll.cfg.PSOInertiaWeight,
		CognitiveWeight: ll.cfg.PSOCognitiveWeight,
		SocialWeight:    ll.cfg.PSOSocialWeight,
		SearchMinX:      ll.cfg.SearchMinX,
		SearchMaxX:      ll.cfg.SearchMaxX,
		SearchMinRate:   ll.cfg.SearchMinRate,
		SearchMaxRate:   ll.cfg.SearchMaxRate,
	}

	result, quality, err := algorithms.LocalizeLeakSourceWithQualityCheck(allReadings, model, psoCfg)
	if err != nil {
		log.Printf("[LeakLocator] 泄漏检测错误: %v", err)
		return
	}

	if quality.DegradedMode {
		log.Printf("[LeakLocator] 数据质量降级: %s, 质量分数: %.1f%%",
			quality.DegradationReason, quality.QualityScore)
	}

	minConfidence := ll.cfg.MinConfidence
	if quality.DegradedMode {
		minConfidence = ll.cfg.MinConfidenceDegraded
	}

	if result == nil || result.Confidence < minConfidence {
		ll.mu.Lock()
		ll.currentLeaks = make([]*models.LeakSource, 0)
		ll.mu.Unlock()
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

	ll.mu.Lock()

	newLeak := true
	for i, existing := range ll.currentLeaks {
		dist := abs(existing.Position - leakSource.Position)
		if dist < ll.cfg.LeakMergeDistance {
			ll.currentLeaks[i] = leakSource
			newLeak = false
			break
		}
	}

	if newLeak {
		ll.currentLeaks = append(ll.currentLeaks, leakSource)
		go ll.saveLeakSource(leakSource)
		go services.BroadcastLeakSource(leakSource)

		if ll.leakChan != nil {
			select {
			case ll.leakChan <- leakSource:
			default:
				log.Printf("[LeakLocator] 泄漏通道已满")
			}
		}
	}

	ll.mu.Unlock()

	log.Printf("[LeakLocator] 检测到泄漏: 位置%.1fm, 速率%.4f L/s, 置信度%.1f%%, 半径%.1fm, 质量:%.1f%%%s",
		leakSource.Position, leakSource.LeakRate, leakSource.Confidence, leakSource.DiffusionRadius,
		quality.QualityScore, map[bool]string{true: " (降级模式)", false: ""}[quality.DegradedMode])
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func (ll *LeakLocator) saveLeakSource(leak *models.LeakSource) {
	if services.DB == nil {
		return
	}

	query := `
		INSERT INTO leak_sources (id, position, latitude, longitude, leak_rate, confidence, diffusion_radius, detected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := services.DB.PG().Exec(context.Background(), query,
		leak.ID, leak.Position, leak.Latitude, leak.Longitude,
		leak.LeakRate, leak.Confidence, leak.DiffusionRadius, leak.DetectedAt)
	if err != nil {
		log.Printf("[LeakLocator] 保存泄漏源失败: %v", err)
	}
}

func (ll *LeakLocator) GetCurrentLeaks() []*models.LeakSource {
	ll.mu.RLock()
	defer ll.mu.RUnlock()

	leaks := make([]*models.LeakSource, len(ll.currentLeaks))
	copy(leaks, ll.currentLeaks)
	return leaks
}

func (ll *LeakLocator) ResolveLeak(leakID uuid.UUID) error {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	for i, leak := range ll.currentLeaks {
		if leak.ID == leakID {
			ll.currentLeaks = append(ll.currentLeaks[:i], ll.currentLeaks[i+1:]...)

			if services.DB != nil {
				query := `
					UPDATE leak_sources
					SET status = 'resolved', resolved_at = NOW()
					WHERE id = $1
				`
				_, err := services.DB.PG().Exec(context.Background(), query, leakID)
				return err
			}
			return nil
		}
	}

	return nil
}

func (ll *LeakLocator) GetWindData() (float64, float64, float64) {
	ll.mu.RLock()
	defer ll.mu.RUnlock()
	return ll.windSpeed, ll.windDir, ll.temperature
}

func (ll *LeakLocator) IsRunning() bool {
	ll.mu.RLock()
	defer ll.mu.RUnlock()
	return ll.running
}
