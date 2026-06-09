package alarm_router

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/mqtt"
	"gas-monitoring-system/backend/services"
)

type AlarmRouter struct {
	cfg           *config.AlarmRouterConfig
	running       bool
	mu            sync.RWMutex

	dataChan      <-chan models.ValidatedData
	alarmChan     chan<- *models.Alarm

	activeAlarms  map[string]*models.Alarm
	alarmHistory  map[string][]time.Time
	alarmsMu      sync.RWMutex

	smsService    *services.SMSService
}

func NewAlarmRouter(cfg *config.AlarmRouterConfig) *AlarmRouter {
	return &AlarmRouter{
		cfg:          cfg,
		activeAlarms: make(map[string]*models.Alarm),
		alarmHistory: make(map[string][]time.Time),
	}
}

func (ar *AlarmRouter) SetChannels(dataChan <-chan models.ValidatedData, alarmChan chan<- *models.Alarm) {
	ar.dataChan = dataChan
	ar.alarmChan = alarmChan
}

func (ar *AlarmRouter) SetSMSService(sms *services.SMSService) {
	ar.smsService = sms
}

func (ar *AlarmRouter) Start() {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	if ar.running {
		return
	}
	ar.running = true

	go ar.dataReceiver()

	log.Println("[AlarmRouter] 告警路由模块启动")
}

func (ar *AlarmRouter) Stop() {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	ar.running = false

	log.Println("[AlarmRouter] 告警路由模块停止")
}

func (ar *AlarmRouter) dataReceiver() {
	for {
		ar.mu.RLock()
		running := ar.running
		ar.mu.RUnlock()

		if !running {
			return
		}

		select {
		case data, ok := <-ar.dataChan:
			if !ok {
				return
			}
			ar.checkAlarm(data.DeviceID, data.Concentration)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (ar *AlarmRouter) checkAlarm(deviceID string, concentration float64) {
	level, threshold := ar.getAlarmLevel(concentration)

	if level == 0 {
		ar.resolveAlarm(deviceID)
		return
	}

	ar.alarmsMu.RLock()
	existingAlarm, exists := ar.activeAlarms[deviceID]
	ar.alarmsMu.RUnlock()

	if !exists || existingAlarm.Level != level {
		alarm := ar.createAlarm(deviceID, level, concentration, threshold)
		ar.triggerAlarm(alarm)
	}
}

func (ar *AlarmRouter) getAlarmLevel(concentration float64) (int, float64) {
	switch {
	case concentration >= ar.cfg.Level3Threshold:
		return 3, ar.cfg.Level3Threshold
	case concentration >= ar.cfg.Level2Threshold:
		return 2, ar.cfg.Level2Threshold
	case concentration >= ar.cfg.Level1Threshold:
		return 1, ar.cfg.Level1Threshold
	default:
		return 0, 0
	}
}

func (ar *AlarmRouter) createAlarm(deviceID string, level int, concentration, threshold float64) *models.Alarm {
	levelNames := map[int]string{
		1: "一级预警",
		2: "二级报警",
		3: "三级紧急关断",
	}

	messages := map[int]string{
		1: fmt.Sprintf("甲烷浓度超过预警值，当前浓度: %.2f%%LEL，阈值: %.1f%%LEL", concentration, threshold),
		2: fmt.Sprintf("甲烷浓度超标报警，当前浓度: %.2f%%LEL，阈值: %.1f%%LEL，请注意安全", concentration, threshold),
		3: fmt.Sprintf("紧急！甲烷浓度严重超标，当前浓度: %.2f%%LEL，已启动紧急关断程序", concentration),
	}

	alarm := &models.Alarm{
		ID:            uuid.New(),
		DeviceID:      deviceID,
		Level:         level,
		LevelName:     levelNames[level],
		Concentration: concentration,
		Threshold:     threshold,
		Message:       messages[level],
		Timestamp:     time.Now(),
		Acknowledged:  false,
		Resolved:      false,
	}

	ar.alarmsMu.Lock()
	ar.activeAlarms[deviceID] = alarm
	ar.alarmHistory[deviceID] = append(ar.alarmHistory[deviceID], alarm.Timestamp)
	ar.alarmsMu.Unlock()

	ar.saveAlarmToDB(alarm)

	return alarm
}

func (ar *AlarmRouter) triggerAlarm(alarm *models.Alarm) {
	log.Printf("[AlarmRouter] %s - Level %d: %s", alarm.DeviceID, alarm.Level, alarm.Message)

	go ar.publishMQTTAlarm(alarm)

	go ar.sendAlarmSMS(alarm)

	if ar.alarmChan != nil {
		select {
		case ar.alarmChan <- alarm:
		default:
			log.Printf("[AlarmRouter] 告警通道已满，丢弃告警: %s", alarm.DeviceID)
		}
	}

	go services.BroadcastAlarm(alarm)
}

func (ar *AlarmRouter) publishMQTTAlarm(alarm *models.Alarm) {
	if mqtt.MQTT == nil {
		return
	}

	mqtt.MQTT.PublishAlarm(alarm)
}

func (ar *AlarmRouter) sendAlarmSMS(alarm *models.Alarm) {
	if ar.smsService == nil {
		return
	}

	if !ar.shouldSendSMS(alarm) {
		return
	}

	ar.smsService.SendAlarmSMS(alarm)
}

func (ar *AlarmRouter) shouldSendSMS(alarm *models.Alarm) bool {
	if alarm.Level < ar.cfg.SMSMinLevel {
		return false
	}

	ar.alarmsMu.RLock()
	history := ar.alarmHistory[alarm.DeviceID]
	ar.alarmsMu.RUnlock()

	if len(history) < 2 {
		return true
	}

	recentCount := 0
	cutoff := time.Now().Add(-ar.cfg.SMSInterval)
	for _, t := range history {
		if t.After(cutoff) {
			recentCount++
		}
	}

	return recentCount <= ar.cfg.SMSMaxPerInterval
}

func (ar *AlarmRouter) resolveAlarm(deviceID string) {
	ar.alarmsMu.Lock()
	alarm, exists := ar.activeAlarms[deviceID]
	if exists {
		alarm.Resolved = true
		alarm.ResolvedAt = time.Now()
		delete(ar.activeAlarms, deviceID)
	}
	ar.alarmsMu.Unlock()

	if exists {
		if services.DB != nil {
			query := `
				UPDATE alarms
				SET resolved = TRUE, resolved_at = NOW()
				WHERE id = $1
			`
			services.DB.PG().Exec(context.Background(), query, alarm.ID)
		}

		log.Printf("[AlarmRouter] %s - 告警解除", deviceID)
	}
}

func (ar *AlarmRouter) saveAlarmToDB(alarm *models.Alarm) {
	if services.DB == nil {
		return
	}

	query := `
		INSERT INTO alarms (id, device_id, level, level_name, concentration, threshold, message, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := services.DB.PG().Exec(context.Background(), query,
		alarm.ID, alarm.DeviceID, alarm.Level, alarm.LevelName,
		alarm.Concentration, alarm.Threshold, alarm.Message, alarm.Timestamp)
	if err != nil {
		log.Printf("[AlarmRouter] 保存告警到数据库失败: %v", err)
	}
}

func (ar *AlarmRouter) GetActiveAlarms() []*models.Alarm {
	ar.alarmsMu.RLock()
	defer ar.alarmsMu.RUnlock()

	alarms := make([]*models.Alarm, 0, len(ar.activeAlarms))
	for _, alarm := range ar.activeAlarms {
		alarms = append(alarms, alarm)
	}
	return alarms
}

func (ar *AlarmRouter) AcknowledgeAlarm(alarmID uuid.UUID, acknowledgedBy string) error {
	ar.alarmsMu.Lock()
	defer ar.alarmsMu.Unlock()

	for _, alarm := range ar.activeAlarms {
		if alarm.ID == alarmID {
			alarm.Acknowledged = true
			alarm.AcknowledgedAt = time.Now()
			alarm.AcknowledgedBy = acknowledgedBy

			if services.DB != nil {
				query := `
					UPDATE alarms
					SET acknowledged = TRUE, acknowledged_at = $1, acknowledged_by = $2
					WHERE id = $3
				`
				_, err := services.DB.PG().Exec(context.Background(), query, alarm.AcknowledgedAt, acknowledgedBy, alarmID)
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("alarm not found")
}

func (ar *AlarmRouter) GetAlarmHistory(deviceID string, limit int) ([]models.Alarm, error) {
	if services.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
		SELECT id, device_id, level, level_name, concentration, threshold, message, timestamp, acknowledged, resolved
		FROM alarms
		WHERE device_id = $1
		ORDER BY timestamp DESC
		LIMIT $2
	`

	rows, err := services.DB.PG().Query(context.Background(), query, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alarms []models.Alarm
	for rows.Next() {
		var a models.Alarm
		err := rows.Scan(&a.ID, &a.DeviceID, &a.Level, &a.LevelName, &a.Concentration,
			&a.Threshold, &a.Message, &a.Timestamp, &a.Acknowledged, &a.Resolved)
		if err != nil {
			return nil, err
		}
		alarms = append(alarms, a)
	}

	return alarms, rows.Err()
}

func (ar *AlarmRouter) IsRunning() bool {
	ar.mu.RLock()
	defer ar.mu.RUnlock()
	return ar.running
}

func (ar *AlarmRouter) GetConfig() *config.AlarmRouterConfig {
	return ar.cfg
}
