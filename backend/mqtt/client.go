package mqtt

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/services"
)

type MQTTService struct {
	client mqtt.Client
	cfg    *config.MQTTConfig
}

var MQTT *MQTTService

func InitMQTT(cfg *config.MQTTConfig) error {
	var err error
	MQTT, err = NewMQTTService(cfg)
	return err
}

func NewMQTTService(cfg *config.MQTTConfig) (*MQTTService, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID + "-" + fmt.Sprintf("%d", time.Now().Unix()))
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
	})
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("MQTT connected successfully")
		MQTT.subscribeTopics()
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("failed to connect MQTT broker: %w", token.Error())
	}

	return &MQTTService{
		client: client,
		cfg:    cfg,
	}, nil
}

func (m *MQTTService) subscribeTopics() {
	topics := m.cfg.Topics

	if topic, ok := topics["sensor_data"]; ok {
		token := m.client.Subscribe(topic, 1, m.handleSensorData)
		token.Wait()
		log.Printf("Subscribed to sensor data topic: %s", topic)
	}

	if topic, ok := topics["command"]; ok {
		token := m.client.Subscribe(topic, 1, m.handleCommand)
		token.Wait()
		log.Printf("Subscribed to command topic: %s", topic)
	}
}

func (m *MQTTService) handleSensorData(client mqtt.Client, msg mqtt.Message) {
	var data models.SensorData
	if err := json.Unmarshal(msg.Payload(), &data); err != nil {
		log.Printf("Failed to parse sensor data: %v", err)
		return
	}

	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}

	go m.processSensorData(&data)
}

func (m *MQTTService) processSensorData(data *models.SensorData) {
	isLaser := strings.HasPrefix(data.DeviceID, "LASER-")

	if isLaser {
		influxData := &models.InfluxSensorData{
			Measurement: "laser_methane",
			Tags: map[string]string{
				"device_id": data.DeviceID,
				"fire_zone": getFireZone(data.DeviceID),
				"status":    data.Status,
			},
			Fields: map[string]interface{}{
				"concentration":  data.Concentration,
				"temperature":    data.Temperature,
				"wind_speed":     data.WindSpeed,
				"wind_direction": data.WindDir,
			},
			Timestamp: data.Timestamp,
		}
		services.DB.WriteSensorData(influxData)

		go services.AlarmEngine.CheckAlarm(data.DeviceID, data.Concentration)
		go services.LeakDetector.AddReading(data.DeviceID, data)
	} else {
		influxData := &models.InfluxSensorData{
			Measurement: "environment",
			Tags: map[string]string{
				"device_id": data.DeviceID,
				"sensor_type": getSensorType(data.DeviceID),
				"location_type": getLocationType(data.DeviceID),
				"fire_zone": getFireZone(data.DeviceID),
			},
			Fields: map[string]interface{}{
				"temperature": data.Temperature,
				"humidity":    data.Humidity,
				"oxygen":      data.Oxygen,
				"wind_speed":  data.WindSpeed,
			},
			Timestamp: data.Timestamp,
		}
		services.DB.WriteSensorData(influxData)
	}
}

func (m *MQTTService) handleCommand(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received command on %s: %s", msg.Topic(), string(msg.Payload()))
}

func (m *MQTTService) Publish(topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	token := m.client.Publish(topic, 1, false, data)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (m *MQTTService) PublishAlarm(alarm *models.Alarm) error {
	topic := fmt.Sprintf("alarms/level%d/%s", alarm.Level, alarm.DeviceID)
	return m.Publish(topic, alarm)
}

func (m *MQTTService) ControlValve(valveID string, action string, reason string) error {
	topic := fmt.Sprintf("valves/%s/control", valveID)
	payload := map[string]interface{}{
		"valve_id": valveID,
		"action":   action,
		"reason":   reason,
		"timestamp": time.Now(),
	}
	return m.Publish(topic, payload)
}

func (m *MQTTService) ControlFan(fanID string, action string, speed int, reason string) error {
	topic := fmt.Sprintf("fans/%s/control", fanID)
	payload := map[string]interface{}{
		"fan_id":    fanID,
		"action":    action,
		"speed":     speed,
		"reason":    reason,
		"timestamp": time.Now(),
	}
	return m.Publish(topic, payload)
}

func (m *MQTTService) SendNotification(message string, level int) error {
	topic := fmt.Sprintf("notifications/level%d", level)
	payload := map[string]interface{}{
		"message":   message,
		"level":     level,
		"timestamp": time.Now(),
	}
	return m.Publish(topic, payload)
}

func (m *MQTTService) Close() {
	if m.client != nil {
		m.client.Disconnect(250)
		log.Println("MQTT client disconnected")
	}
}

func getFireZone(deviceID string) string {
	if strings.HasPrefix(deviceID, "LASER-") {
		numStr := strings.TrimPrefix(deviceID, "LASER-")
		var num int
		fmt.Sscanf(numStr, "%d", &num)
		zoneNum := (num / 30) + 1
		return fmt.Sprintf("ZONE-%02d", zoneNum)
	}
	return "ZONE-01"
}

func getSensorType(deviceID string) string {
	if strings.HasPrefix(deviceID, "ENV-O2-") {
		return "oxygen"
	}
	return "temp_humidity"
}

func getLocationType(deviceID string) string {
	if strings.Contains(deviceID, "VENT") {
		return "vent"
	}
	return "valve"
}
