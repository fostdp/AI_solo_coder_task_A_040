package emergency_controller

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/mqtt"
	"gas-monitoring-system/backend/services"
)

type EmergencyCommand struct {
	Type        CommandType
	FireZone    string
	Alarm       *models.Alarm
	LeakSource  *models.LeakSource
	Reason      string
}

type CommandType string

const (
	CommandTypeCloseValves    CommandType = "close_valves"
	CommandTypeStartFans      CommandType = "start_fans"
	CommandTypeEvacuation     CommandType = "evacuation"
	CommandTypeResetZone      CommandType = "reset_zone"
)

type EmergencyController struct {
	cfg             *config.EmergencyControllerConfig
	running         bool
	mu              sync.RWMutex

	alarmChan       <-chan *models.Alarm
	leakChan        <-chan *models.LeakSource
	commandChan     chan EmergencyCommand

	activeControls  map[string]time.Time
	controlsMu      sync.Mutex
}

func NewEmergencyController(cfg *config.EmergencyControllerConfig) *EmergencyController {
	return &EmergencyController{
		cfg:            cfg,
		activeControls: make(map[string]time.Time),
		commandChan:    make(chan EmergencyCommand, 100),
	}
}

func (ec *EmergencyController) SetChannels(alarmChan <-chan *models.Alarm, leakChan <-chan *models.LeakSource) {
	ec.alarmChan = alarmChan
	ec.leakChan = leakChan
}

func (ec *EmergencyController) Start() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if ec.running {
		return
	}
	ec.running = true

	go ec.alarmReceiver()
	go ec.leakReceiver()
	go ec.commandProcessor()

	log.Println("[EmergencyController] 应急联动控制模块启动")
}

func (ec *EmergencyController) Stop() {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.running = false
	close(ec.commandChan)

	log.Println("[EmergencyController] 应急联动控制模块停止")
}

func (ec *EmergencyController) alarmReceiver() {
	for {
		ec.mu.RLock()
		running := ec.running
		ec.mu.RUnlock()

		if !running {
			return
		}

		select {
		case alarm, ok := <-ec.alarmChan:
			if !ok {
				return
			}
			ec.handleAlarm(alarm)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (ec *EmergencyController) leakReceiver() {
	for {
		ec.mu.RLock()
		running := ec.running
		ec.mu.RUnlock()

		if !running {
			return
		}

		select {
		case leak, ok := <-ec.leakChan:
			if !ok {
				return
			}
			ec.handleLeakSource(leak)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (ec *EmergencyController) commandProcessor() {
	for cmd := range ec.commandChan {
		ec.processCommand(cmd)
	}
}

func (ec *EmergencyController) handleAlarm(alarm *models.Alarm) {
	if alarm == nil {
		return
	}

	fireZone := getFireZoneFromDeviceID(alarm.DeviceID)

	log.Printf("[EmergencyController] 接收告警: %s, 级别: %d, 分区: %s", alarm.DeviceID, alarm.Level, fireZone)

	if alarm.Level >= ec.cfg.ValveControlLevel {
		ec.commandChan <- EmergencyCommand{
			Type:     CommandTypeCloseValves,
			FireZone: fireZone,
			Alarm:    alarm,
			Reason:   fmt.Sprintf("浓度达到%.2f%%LEL，触发%d级以上报警", alarm.Concentration, ec.cfg.ValveControlLevel),
		}

		ec.commandChan <- EmergencyCommand{
			Type:     CommandTypeStartFans,
			FireZone: fireZone,
			Alarm:    alarm,
			Reason:   fmt.Sprintf("浓度达到%.2f%%LEL，启动通风排风", alarm.Concentration),
		}
	}

	if alarm.Level >= ec.cfg.EvacuationLevel {
		ec.commandChan <- EmergencyCommand{
			Type:     CommandTypeEvacuation,
			FireZone: fireZone,
			Alarm:    alarm,
		}
	}
}

func (ec *EmergencyController) handleLeakSource(leak *models.LeakSource) {
	log.Printf("[EmergencyController] 接收泄漏源: 位置%.1fm, 半径%.1fm", leak.Position, leak.DiffusionRadius)
}

func (ec *EmergencyController) processCommand(cmd EmergencyCommand) {
	switch cmd.Type {
	case CommandTypeCloseValves:
		ec.closeZoneValves(cmd.FireZone, cmd.Alarm, cmd.Reason)
	case CommandTypeStartFans:
		ec.startZoneFans(cmd.FireZone, cmd.Alarm, cmd.Reason)
	case CommandTypeEvacuation:
		ec.sendEvacuationNotification(cmd.FireZone, cmd.Alarm)
	case CommandTypeResetZone:
		ec.ResetZone(cmd.FireZone)
	}
}

func (ec *EmergencyController) closeZoneValves(fireZone string, alarm *models.Alarm, reason string) {
	valveIDs, err := ec.getValvesInZone(fireZone)
	if err != nil {
		log.Printf("[EmergencyController] 获取防火分区 %s 的阀门列表失败: %v", fireZone, err)
		return
	}

	for _, valveID := range valveIDs {
		if !ec.canControl("valve", valveID) {
			continue
		}

		ec.markControlled("valve", valveID)

		go func(vID string) {
			log.Printf("[EmergencyController] 关闭阀门 %s，原因: %s", vID, reason)

			err := mqtt.MQTT.ControlValve(vID, "close", reason)
			success := err == nil

			if success && services.DB != nil {
				query := `UPDATE valves SET status = 'closed', last_action = NOW() WHERE valve_id = $1`
				services.DB.PG().Exec(context.Background(), query, vID)
			}

			ec.logValveControl(vID, "close", fireZone, alarm, success)

			if success {
				mqtt.MQTT.SendNotification(fmt.Sprintf("已关闭防火分区%s阀门 %s", fireZone, vID), alarm.Level)
			}
		}(valveID)
	}
}

func (ec *EmergencyController) startZoneFans(fireZone string, alarm *models.Alarm, reason string) {
	fanIDs, err := ec.getFansInZone(fireZone)
	if err != nil {
		log.Printf("[EmergencyController] 获取防火分区 %s 的风机列表失败: %v", fireZone, err)
		return
	}

	speed := ec.cfg.FanSpeedNormal
	if alarm.Level >= ec.cfg.FanSpeedHighLevel {
		speed = ec.cfg.FanSpeedHigh
	}

	for _, fanID := range fanIDs {
		if !ec.canControl("fan", fanID) {
			continue
		}

		ec.markControlled("fan", fanID)

		go func(fID string) {
			log.Printf("[EmergencyController] 启动风机 %s，转速: %d%%，原因: %s", fID, speed, reason)

			err := mqtt.MQTT.ControlFan(fID, "start", speed, reason)
			success := err == nil

			if success && services.DB != nil {
				query := `UPDATE fans SET status = 'running', speed = $1 WHERE fan_id = $2`
				services.DB.PG().Exec(context.Background(), query, speed, fID)
			}

			ec.logFanControl(fID, "start", speed, fireZone, alarm, success)

			if success {
				mqtt.MQTT.SendNotification(fmt.Sprintf("已启动防火分区%s风机 %s，转速%d%%", fireZone, fID, speed), alarm.Level)
			}
		}(fanID)
	}
}

func (ec *EmergencyController) sendEvacuationNotification(fireZone string, alarm *models.Alarm) {
	message := fmt.Sprintf("[紧急疏散通知] 防火分区%s检测到甲烷浓度严重超标(%.2f%%LEL)，请立即疏散！"+
		"已关闭相关阀门并启动强制排风。", fireZone, alarm.Concentration)

	log.Printf("[EmergencyController] 发送疏散通知: %s", message)

	mqtt.MQTT.SendNotification(message, 3)

	smsService := services.NewSMSService(&config.AppConfig.SMS)
	for _, receiver := range config.AppConfig.SMS.Receivers {
		go smsService.SendSMS(receiver, message)
	}
}

func (ec *EmergencyController) canControl(deviceType, deviceID string) bool {
	ec.controlsMu.Lock()
	defer ec.controlsMu.Unlock()

	controlKey := fmt.Sprintf("%s:%s", deviceType, deviceID)
	if lastControl, exists := ec.activeControls[controlKey]; exists {
		if time.Since(lastControl) < ec.cfg.ControlCooldown {
			return false
		}
	}
	return true
}

func (ec *EmergencyController) markControlled(deviceType, deviceID string) {
	ec.controlsMu.Lock()
	defer ec.controlsMu.Unlock()

	controlKey := fmt.Sprintf("%s:%s", deviceType, deviceID)
	ec.activeControls[controlKey] = time.Now()
}

func (ec *EmergencyController) getValvesInZone(fireZone string) ([]string, error) {
	if services.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT valve_id FROM valves WHERE fire_zone = $1`
	rows, err := services.DB.PG().Query(context.Background(), query, fireZone)
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

func (ec *EmergencyController) getFansInZone(fireZone string) ([]string, error) {
	if services.DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT fan_id FROM fans WHERE fire_zone = $1`
	rows, err := services.DB.PG().Query(context.Background(), query, fireZone)
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

func (ec *EmergencyController) logValveControl(valveID, action, fireZone string, alarm *models.Alarm, success bool) {
	if services.DB == nil {
		return
	}

	query := `
		INSERT INTO valve_control_logs (id, valve_id, action, fire_zone, triggered_by, reason, timestamp, success)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	reason := fmt.Sprintf("告警触发: 浓度%.2f%%LEL，级别%d", alarm.Concentration, alarm.Level)
	triggeredBy := "alarm-" + alarm.ID.String()
	_, err := services.DB.PG().Exec(context.Background(), query,
		uuid.New(), valveID, action, fireZone, triggeredBy, reason, time.Now(), success)
	if err != nil {
		log.Printf("[EmergencyController] 记录阀门控制日志失败: %v", err)
	}
}

func (ec *EmergencyController) logFanControl(fanID, action string, speed int, fireZone string, alarm *models.Alarm, success bool) {
	if services.DB == nil {
		return
	}

	query := `
		INSERT INTO fan_control_logs (id, fan_id, action, speed, fire_zone, triggered_by, reason, timestamp, success)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	reason := fmt.Sprintf("告警触发: 浓度%.2f%%LEL，级别%d", alarm.Concentration, alarm.Level)
	triggeredBy := "alarm-" + alarm.ID.String()
	_, err := services.DB.PG().Exec(context.Background(), query,
		uuid.New(), fanID, action, speed, fireZone, triggeredBy, reason, time.Now(), success)
	if err != nil {
		log.Printf("[EmergencyController] 记录风机控制日志失败: %v", err)
	}
}

func (ec *EmergencyController) ResetZone(fireZone string) error {
	log.Printf("[EmergencyController] 重置防火分区 %s", fireZone)

	if services.DB == nil {
		return fmt.Errorf("database not initialized")
	}

	tx, err := services.DB.PG().Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	valveQuery := `UPDATE valves SET status = 'open', last_action = NOW() WHERE fire_zone = $1`
	_, err = tx.Exec(context.Background(), valveQuery, fireZone)
	if err != nil {
		return err
	}

	fanQuery := `UPDATE fans SET status = 'stopped', speed = 0 WHERE fire_zone = $1`
	_, err = tx.Exec(context.Background(), fanQuery, fireZone)
	if err != nil {
		return err
	}

	if err := tx.Commit(context.Background()); err != nil {
		return err
	}

	valves, _ := ec.getValvesInZone(fireZone)
	for _, vID := range valves {
		mqtt.MQTT.ControlValve(vID, "open", "手动重置")
		ec.controlsMu.Lock()
		delete(ec.activeControls, fmt.Sprintf("valve:%s", vID))
		ec.controlsMu.Unlock()
	}

	fans, _ := ec.getFansInZone(fireZone)
	for _, fID := range fans {
		mqtt.MQTT.ControlFan(fID, "stop", 0, "手动重置")
		ec.controlsMu.Lock()
		delete(ec.activeControls, fmt.Sprintf("fan:%s", fID))
		ec.controlsMu.Unlock()
	}

	return nil
}

func getFireZoneFromDeviceID(deviceID string) string {
	if strings.HasPrefix(deviceID, "LASER-") {
		numStr := strings.TrimPrefix(deviceID, "LASER-")
		var num int
		for _, c := range numStr {
			if c >= '0' && c <= '9' {
				num = num*10 + int(c-'0')
			}
		}
		zoneNum := (num / 30) + 1
		return fmt.Sprintf("ZONE-%02d", zoneNum)
	}
	return "ZONE-01"
}

func (ec *EmergencyController) IsRunning() bool {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.running
}
