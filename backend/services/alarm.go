package services

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
)

type AlarmEngineService struct {
	cfg              *config.AlarmConfig
	activeAlarms     map[string]*models.Alarm
	alarmHistory     map[string][]time.Time
	mu               sync.RWMutex
	smsService       *SMSService
	emergencyControl *EmergencyControlService
}

var AlarmEngine *AlarmEngineService

func InitAlarmEngine(cfg *config.AlarmConfig, smsCfg *config.SMSConfig) {
	AlarmEngine = &AlarmEngineService{
		cfg:          cfg,
		activeAlarms: make(map[string]*models.Alarm),
		alarmHistory: make(map[string][]time.Time),
		smsService:   NewSMSService(smsCfg),
	}
	AlarmEngine.emergencyControl = NewEmergencyControlService()
}

func (a *AlarmEngineService) CheckAlarm(deviceID string, concentration float64) {
	level, threshold := a.getAlarmLevel(concentration)

	if level == 0 {
		a.resolveAlarm(deviceID)
		return
	}

	a.mu.RLock()
	existingAlarm, exists := a.activeAlarms[deviceID]
	a.mu.RUnlock()

	if !exists || existingAlarm.Level != level {
		alarm := a.createAlarm(deviceID, level, concentration, threshold)
		a.triggerAlarm(alarm)
	}
}

func (a *AlarmEngineService) getAlarmLevel(concentration float64) (int, float64) {
	switch {
	case concentration >= a.cfg.Level3Threshold:
		return 3, a.cfg.Level3Threshold
	case concentration >= a.cfg.Level2Threshold:
		return 2, a.cfg.Level2Threshold
	case concentration >= a.cfg.Level1Threshold:
		return 1, a.cfg.Level1Threshold
	default:
		return 0, 0
	}
}

func (a *AlarmEngineService) createAlarm(deviceID string, level int, concentration, threshold float64) *models.Alarm {
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

	a.mu.Lock()
	a.activeAlarms[deviceID] = alarm
	a.alarmHistory[deviceID] = append(a.alarmHistory[deviceID], alarm.Timestamp)
	a.mu.Unlock()

	a.saveAlarmToDB(alarm)

	return alarm
}

func (a *AlarmEngineService) triggerAlarm(alarm *models.Alarm) {
	log.Printf("[ALARM] %s - Level %d: %s", alarm.DeviceID, alarm.Level, alarm.Message)

	go mqtt.MQTT.PublishAlarm(alarm)

	go a.smsService.SendAlarmSMS(alarm)

	if alarm.Level >= 2 {
		go a.emergencyControl.HandleEmergency(alarm)
	}

	go BroadcastAlarm(alarm)
}

func (a *AlarmEngineService) resolveAlarm(deviceID string) {
	a.mu.Lock()
	alarm, exists := a.activeAlarms[deviceID]
	if exists {
		alarm.Resolved = true
		alarm.ResolvedAt = time.Time{}
		delete(a.activeAlarms, deviceID)
	}
	a.mu.Unlock()

	if exists {
		query := `
			UPDATE alarms
			SET resolved = TRUE, resolved_at = NOW()
			WHERE id = $1
		`
		DB.PG().Exec(context.Background(), query, alarm.ID)

		log.Printf("[ALARM] %s - Alarm resolved", deviceID)
	}
}

func (a *AlarmEngineService) saveAlarmToDB(alarm *models.Alarm) {
	query := `
		INSERT INTO alarms (id, device_id, level, level_name, concentration, threshold, message, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := DB.PG().Exec(context.Background(), query,
		alarm.ID, alarm.DeviceID, alarm.Level, alarm.LevelName,
		alarm.Concentration, alarm.Threshold, alarm.Message, alarm.Timestamp)
	if err != nil {
		log.Printf("Failed to save alarm to DB: %v", err)
	}
}

func (a *AlarmEngineService) GetActiveAlarms() []*models.Alarm {
	a.mu.RLock()
	defer a.mu.RUnlock()

	alarms := make([]*models.Alarm, 0, len(a.activeAlarms))
	for _, alarm := range a.activeAlarms {
		alarms = append(alarms, alarm)
	}
	return alarms
}

func (a *AlarmEngineService) AcknowledgeAlarm(alarmID uuid.UUID, acknowledgedBy string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, alarm := range a.activeAlarms {
		if alarm.ID == alarmID {
			alarm.Acknowledged = true
			alarm.AcknowledgedAt = time.Now()
			alarm.AcknowledgedBy = acknowledgedBy

			query := `
				UPDATE alarms
				SET acknowledged = TRUE, acknowledged_at = $1, acknowledged_by = $2
				WHERE id = $3
			`
			_, err := DB.PG().Exec(context.Background(), query, alarm.AcknowledgedAt, acknowledgedBy, alarmID)
			return err
		}
	}

	return fmt.Errorf("alarm not found")
}

func (a *AlarmEngineService) GetAlarmHistory(deviceID string, limit int) ([]models.Alarm, error) {
	query := `
		SELECT id, device_id, level, level_name, concentration, threshold, message, timestamp, acknowledged, resolved
		FROM alarms
		WHERE device_id = $1
		ORDER BY timestamp DESC
		LIMIT $2
	`

	rows, err := DB.PG().Query(context.Background(), query, deviceID, limit)
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
