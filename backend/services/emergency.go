package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/mqtt"
)

type SMSService struct {
	cfg *config.SMSConfig
}

func NewSMSService(cfg *config.SMSConfig) *SMSService {
	return &SMSService{cfg: cfg}
}

func (s *SMSService) SendAlarmSMS(alarm *models.Alarm) {
	message := fmt.Sprintf("[燃气监测告警] %s 设备%s检测到甲烷浓度%.2f%%LEL，%s",
		alarm.LevelName, alarm.DeviceID, alarm.Concentration, alarm.Message)

	for _, receiver := range s.cfg.Receivers {
		go s.sendSMS(receiver, message, alarm.ID)
	}
}

func (s *SMSService) SendSMS(receiver, message string) error {
	return s.sendSMS(receiver, message, uuid.Nil)
}

func (s *SMSService) sendSMS(receiver, message string, alarmID uuid.UUID) error {
	log.Printf("[SMS] 发送到 %s: %s", receiver, message)

	payload := map[string]interface{}{
		"mobile":  receiver,
		"content": message,
	}

	data, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", s.cfg.APIURL, bytes.NewBuffer(data))
	if err != nil {
		s.logSMSResult(receiver, message, alarmID, false, err.Error())
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.logSMSResult(receiver, message, alarmID, false, err.Error())
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	success := resp.StatusCode == 200

	s.logSMSResult(receiver, message, alarmID, success, string(body))

	if !success {
		return fmt.Errorf("SMS API returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (s *SMSService) logSMSResult(receiver, message string, alarmID uuid.UUID, success bool, response string) {
	query := `
		INSERT INTO sms_logs (id, alarm_id, receiver, message, sent_at, success, response)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	id := uuid.New()
	DB.PG().Exec(context.Background(), query, id, alarmID, receiver, message, time.Now(), success, response)
}

type EmergencyControlService struct {
	activeControls map[string]time.Time
}

func NewEmergencyControlService() *EmergencyControlService {
	return &EmergencyControlService{
		activeControls: make(map[string]time.Time),
	}
}

func (e *EmergencyControlService) HandleEmergency(alarm *models.Alarm) {
	fireZone := getFireZoneFromDeviceID(alarm.DeviceID)

	log.Printf("[EMERGENCY] %s 触发应急联动，防火分区: %s", alarm.DeviceID, fireZone)

	if alarm.Level >= 2 {
		go e.closeZoneValves(fireZone, alarm)
		go e.startZoneFans(fireZone, alarm)
	}

	if alarm.Level >= 3 {
		go e.sendEvacuationNotification(fireZone, alarm)
	}
}

func (e *EmergencyControlService) closeZoneValves(fireZone string, alarm *models.Alarm) {
	valveIDs, err := e.getValvesInZone(fireZone)
	if err != nil {
		log.Printf("[EMERGENCY] 获取防火分区 %s 的阀门列表失败: %v", fireZone, err)
		return
	}

	reason := fmt.Sprintf("浓度达到%.2f%%LEL，触发二级以上报警", alarm.Concentration)

	for _, valveID := range valveIDs {
		controlKey := fmt.Sprintf("valve:%s", valveID)
		if lastControl, exists := e.activeControls[controlKey]; exists {
			if time.Since(lastControl) < 5*time.Minute {
				continue
			}
		}

		e.activeControls[controlKey] = time.Now()

		go func(vID string) {
			log.Printf("[VALVE] 关闭阀门 %s，原因: %s", vID, reason)

			err := mqtt.MQTT.ControlValve(vID, "close", reason)
			success := err == nil

			if success {
				query := `UPDATE valves SET status = 'closed', last_action = NOW() WHERE valve_id = $1`
				DB.PG().Exec(context.Background(), query, vID)
			}

			e.logValveControl(vID, "close", fireZone, alarm, success)

			if success {
				mqtt.MQTT.SendNotification(fmt.Sprintf("已关闭防火分区%s阀门 %s", fireZone, vID), alarm.Level)
			}
		}(valveID)
	}
}

func (e *EmergencyControlService) startZoneFans(fireZone string, alarm *models.Alarm) {
	fanIDs, err := e.getFansInZone(fireZone)
	if err != nil {
		log.Printf("[EMERGENCY] 获取防火分区 %s 的风机列表失败: %v", fireZone, err)
		return
	}

	speed := 100
	if alarm.Level >= 3 {
		speed = 100
	} else {
		speed = 75
	}

	reason := fmt.Sprintf("浓度达到%.2f%%LEL，启动通风排风", alarm.Concentration)

	for _, fanID := range fanIDs {
		controlKey := fmt.Sprintf("fan:%s", fanID)
		if lastControl, exists := e.activeControls[controlKey]; exists {
			if time.Since(lastControl) < 5*time.Minute {
				continue
			}
		}

		e.activeControls[controlKey] = time.Now()

		go func(fID string) {
			log.Printf("[FAN] 启动风机 %s，转速: %d%%，原因: %s", fID, speed, reason)

			err := mqtt.MQTT.ControlFan(fID, "start", speed, reason)
			success := err == nil

			if success {
				query := `UPDATE fans SET status = 'running', speed = $1 WHERE fan_id = $2`
				DB.PG().Exec(context.Background(), query, speed, fID)
			}

			e.logFanControl(fID, "start", speed, fireZone, alarm, success)

			if success {
				mqtt.MQTT.SendNotification(fmt.Sprintf("已启动防火分区%s风机 %s，转速%d%%", fireZone, fID, speed), alarm.Level)
			}
		}(fanID)
	}
}

func (e *EmergencyControlService) sendEvacuationNotification(fireZone string, alarm *models.Alarm) {
	message := fmt.Sprintf("[紧急疏散通知] 防火分区%s检测到甲烷浓度严重超标(%.2f%%LEL)，请立即疏散！" +
		"已关闭相关阀门并启动强制排风。", fireZone, alarm.Concentration)

	log.Printf("[EVACUATION] %s", message)

	mqtt.MQTT.SendNotification(message, 3)

	smsService := NewSMSService(config.AppConfig.SMS)
	for _, receiver := range config.AppConfig.SMS.Receivers {
		go smsService.SendSMS(receiver, message)
	}
}

func (e *EmergencyControlService) getValvesInZone(fireZone string) ([]string, error) {
	query := `SELECT valve_id FROM valves WHERE fire_zone = $1`
	rows, err := DB.PG().Query(context.Background(), query, fireZone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var valveIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		valveIDs = append(valveIDs, id)
	}

	return valveIDs, rows.Err()
}

func (e *EmergencyControlService) getFansInZone(fireZone string) ([]string, error) {
	query := `SELECT fan_id FROM fans WHERE fire_zone = $1`
	rows, err := DB.PG().Query(context.Background(), query, fireZone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fanIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		fanIDs = append(fanIDs, id)
	}

	return fanIDs, rows.Err()
}

func (e *EmergencyControlService) logValveControl(valveID, action, fireZone string, alarm *models.Alarm, success bool) {
	query := `
		INSERT INTO valve_control_logs (id, valve_id, action, fire_zone, triggered_by, reason, timestamp, success)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	reason := fmt.Sprintf("告警触发: 浓度%.2f%%LEL，级别%d", alarm.Concentration, alarm.Level)
	_, err := DB.PG().Exec(context.Background(), query,
		uuid.New(), valveID, action, fireZone, "alarm-"+alarm.ID.String(), reason, time.Now(), success)
	if err != nil {
		log.Printf("Failed to log valve control: %v", err)
	}
}

func (e *EmergencyControlService) logFanControl(fanID, action string, speed int, fireZone string, alarm *models.Alarm, success bool) {
	query := `
		INSERT INTO fan_control_logs (id, fan_id, action, speed, fire_zone, triggered_by, reason, timestamp, success)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	reason := fmt.Sprintf("告警触发: 浓度%.2f%%LEL，级别%d", alarm.Concentration, alarm.Level)
	_, err := DB.PG().Exec(context.Background(), query,
		uuid.New(), fanID, action, speed, fireZone, "alarm-"+alarm.ID.String(), reason, time.Now(), success)
	if err != nil {
		log.Printf("Failed to log fan control: %v", err)
	}
}

func getFireZoneFromDeviceID(deviceID string) string {
	if strings.HasPrefix(deviceID, "LASER-") {
		numStr := strings.TrimPrefix(deviceID, "LASER-")
		var num int
		fmt.Sscanf(numStr, "%d", &num)
		zoneNum := (num / 30) + 1
		return fmt.Sprintf("ZONE-%02d", zoneNum)
	}
	return "ZONE-01"
}

func (e *EmergencyControlService) ResetZone(fireZone string) error {
	log.Printf("[EMERGENCY] 重置防火分区 %s", fireZone)

	valveQuery := `UPDATE valves SET status = 'open', last_action = NOW() WHERE fire_zone = $1`
	_, err := DB.PG().Exec(context.Background(), valveQuery, fireZone)
	if err != nil {
		return err
	}

	fanQuery := `UPDATE fans SET status = 'stopped', speed = 0 WHERE fire_zone = $1`
	_, err = DB.PG().Exec(context.Background(), fanQuery, fireZone)
	if err != nil {
		return err
	}

	valves, _ := e.getValvesInZone(fireZone)
	for _, vID := range valves {
		mqtt.MQTT.ControlValve(vID, "open", "手动重置")
		delete(e.activeControls, fmt.Sprintf("valve:%s", vID))
	}

	fans, _ := e.getFansInZone(fireZone)
	for _, fID := range fans {
		mqtt.MQTT.ControlFan(fID, "stop", 0, "手动重置")
		delete(e.activeControls, fmt.Sprintf("fan:%s", fID))
	}

	return nil
}
