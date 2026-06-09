package laser_receiver

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/services"
)



type LaserReceiver struct {
	cfg          *config.LaserReceiverConfig
	running      bool
	mu           sync.RWMutex

	alarmChan    chan<- models.ValidatedData
	leakChan     chan<- models.ValidatedData

	stats        ReceiverStats
	statsMu      sync.Mutex
}

type ReceiverStats struct {
	TotalReceived   int64
	ValidCount      int64
	InvalidCount    int64
	LastReceivedAt  time.Time
	ErrorCount      map[string]int64
}

func NewLaserReceiver(cfg *config.LaserReceiverConfig) *LaserReceiver {
	return &LaserReceiver{
		cfg:       cfg,
		stats: ReceiverStats{
			ErrorCount: make(map[string]int64),
		},
	}
}

func (lr *LaserReceiver) SetChannels(alarmChan chan<- models.ValidatedData, leakChan chan<- models.ValidatedData) {
	lr.alarmChan = alarmChan
	lr.leakChan = leakChan
}

func (lr *LaserReceiver) Start() {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	if lr.running {
		return
	}
	lr.running = true

	go lr.statsPrinter()
	log.Println("[LaserReceiver] 激光数据接收器启动")
}

func (lr *LaserReceiver) Stop() {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	lr.running = false
	log.Println("[LaserReceiver] 激光数据接收器停止")
}

func (lr *LaserReceiver) ProcessData(data *models.SensorData) {
	lr.mu.RLock()
	running := lr.running
	lr.mu.RUnlock()

	if !running {
		return
	}

	lr.statsMu.Lock()
	lr.stats.TotalReceived++
	lr.stats.LastReceivedAt = data.Timestamp
	lr.statsMu.Unlock()

	validated := lr.validate(data)

	lr.statsMu.Lock()
	if validated.IsValid {
		lr.stats.ValidCount++
	} else {
		lr.stats.InvalidCount++
		lr.stats.ErrorCount[validated.FailReason]++
	}
	lr.statsMu.Unlock()

	if validated.IsValid {
		if services.DB != nil {
			influxData := &models.InfluxSensorData{
				DeviceID:      data.DeviceID,
				Concentration: data.Concentration,
				Timestamp:     data.Timestamp,
				IsLaser:       strings.HasPrefix(data.DeviceID, "LASER-"),
				Temperature:   data.Temperature,
				Humidity:      data.Humidity,
				OxygenLevel:   data.OxygenLevel,
				WindSpeed:     data.WindSpeed,
				WindDir:       data.WindDir,
			}
			go services.DB.WriteSensorData(influxData)
		}

		if lr.alarmChan != nil && validated.Concentration > lr.cfg.AlarmForwardThreshold {
			select {
			case lr.alarmChan <- validated:
			default:
				log.Printf("[LaserReceiver] 告警通道已满，丢弃数据: %s", data.DeviceID)
			}
		}

		if lr.leakChan != nil {
			select {
			case lr.leakChan <- validated:
			default:
				log.Printf("[LaserReceiver] 泄漏定位通道已满，丢弃数据: %s", data.DeviceID)
			}
		}
	} else {
		log.Printf("[LaserReceiver] 数据校验失败 [%s]: %s", data.DeviceID, validated.FailReason)
	}
}

func (lr *LaserReceiver) validate(data *models.SensorData) models.ValidatedData {
	result := models.ValidatedData{
		DeviceID:      data.DeviceID,
		Concentration: data.Concentration,
		Timestamp:     data.Timestamp,
		RawData:       data,
		IsValid:       true,
	}

	if data.DeviceID == "" {
		result.IsValid = false
		result.FailReason = "empty_device_id"
		return result
	}

	if data.Concentration < 0 {
		result.IsValid = false
		result.FailReason = "negative_concentration"
		return result
	}

	if data.Concentration > lr.cfg.MaxValidConcentration {
		result.IsValid = false
		result.FailReason = "concentration_out_of_range"
		return result
	}

	if time.Since(data.Timestamp) > lr.cfg.MaxDataAge {
		result.IsValid = false
		result.FailReason = "data_too_old"
		return result
	}

	if time.Since(data.Timestamp) < -lr.cfg.MaxFutureTime {
		result.IsValid = false
		result.FailReason = "future_timestamp"
		return result
	}

	return result
}

func (lr *LaserReceiver) statsPrinter() {
	ticker := time.NewTicker(lr.cfg.StatsInterval)
	defer ticker.Stop()

	for range ticker.C {
		lr.mu.RLock()
		running := lr.running
		lr.mu.RUnlock()

		if !running {
			return
		}

		lr.statsMu.Lock()
		stats := lr.stats
		lr.statsMu.Unlock()

		validRate := float64(0)
		if stats.TotalReceived > 0 {
			validRate = float64(stats.ValidCount) / float64(stats.TotalReceived) * 100
		}

		log.Printf("[LaserReceiver] 统计 - 接收:%d, 有效:%d, 无效:%d, 有效率:%.1f%%, 最后接收:%v",
			stats.TotalReceived, stats.ValidCount, stats.InvalidCount,
			validRate, stats.LastReceivedAt.Format("15:04:05"))
	}
}

func (lr *LaserReceiver) GetStats() ReceiverStats {
	lr.statsMu.Lock()
	defer lr.statsMu.Unlock()

	stats := lr.stats
	stats.ErrorCount = make(map[string]int64)
	for k, v := range lr.stats.ErrorCount {
		stats.ErrorCount[k] = v
	}
	return stats
}

func (lr *LaserReceiver) IsRunning() bool {
	lr.mu.RLock()
	defer lr.mu.RUnlock()
	return lr.running
}

func (lr *LaserReceiver) ResetStats() {
	lr.statsMu.Lock()
	defer lr.statsMu.Unlock()

	lr.stats = ReceiverStats{
		ErrorCount: make(map[string]int64),
	}
}
