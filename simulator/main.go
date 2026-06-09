package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type SensorData struct {
	DeviceID      string  `json:"device_id"`
	Timestamp     string  `json:"timestamp"`
	Concentration float64 `json:"concentration"`
	Temperature   float64 `json:"temperature,omitempty"`
	Humidity      float64 `json:"humidity,omitempty"`
	Oxygen        float64 `json:"oxygen,omitempty"`
	WindSpeed     float64 `json:"wind_speed,omitempty"`
	WindDir       float64 `json:"wind_dir,omitempty"`
	Status        string  `json:"status"`
}

type SimulatorConfig struct {
	Broker        string
	ClientID      string
	Username      string
	Password      string
	TotalDetectors int
	TotalEnvSensors int
	IntervalMs    int
	LeakEnabled   bool
	LeakPosition  float64
	LeakRate      float64
	WindSpeed     float64
	WindDir       float64
}

type LaserDetector struct {
	DeviceID   string
	Position   float64
	Latitude   float64
	Longitude  float64
	FireZone   string
	BaseNoise  float64
}

type EnvSensor struct {
	DeviceID     string
	SensorType   string
	LocationType string
	Position     float64
	Latitude     float64
	Longitude    float64
	FireZone     string
}

var cfg SimulatorConfig
var client mqtt.Client
var detectors []*LaserDetector
var envSensors []*EnvSensor

func main() {
	flag.StringVar(&cfg.Broker, "broker", "tcp://localhost:1883", "MQTT broker address")
	flag.StringVar(&cfg.ClientID, "client-id", "sensor-simulator", "MQTT client ID")
	flag.StringVar(&cfg.Username, "username", "admin", "MQTT username")
	flag.StringVar(&cfg.Password, "password", "admin", "MQTT password")
	flag.IntVar(&cfg.TotalDetectors, "detectors", 300, "Total number of laser detectors")
	flag.IntVar(&cfg.TotalEnvSensors, "env-sensors", 50, "Total number of environment sensors")
	flag.IntVar(&cfg.IntervalMs, "interval", 1000, "Data sending interval in milliseconds")
	flag.BoolVar(&cfg.LeakEnabled, "leak", false, "Enable simulated gas leak")
	flag.Float64Var(&cfg.LeakPosition, "leak-pos", 15000, "Leak position in meters")
	flag.Float64Var(&cfg.LeakRate, "leak-rate", 1.0, "Leak rate in L/s")
	flag.Float64Var(&cfg.WindSpeed, "wind-speed", 1.5, "Wind speed in m/s")
	flag.Float64Var(&cfg.WindDir, "wind-dir", 90.0, "Wind direction in degrees")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	initSensors()
	initMQTT()
	defer client.Disconnect(250)

	log.Printf("Sensor simulator started")
	log.Printf("  Detectors: %d", cfg.TotalDetectors)
	log.Printf("  Environment sensors: %d", cfg.TotalEnvSensors)
	log.Printf("  Interval: %dms", cfg.IntervalMs)
	log.Printf("  Leak enabled: %v", cfg.LeakEnabled)
	if cfg.LeakEnabled {
		log.Printf("  Leak position: %.1fm, rate: %.2f L/s", cfg.LeakPosition, cfg.LeakRate)
	}
	log.Printf("  Wind: %.1f m/s, %.0f°", cfg.WindSpeed, cfg.WindDir)

	ticker := time.NewTicker(time.Duration(cfg.IntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		sendSensorData()
	}
}

func initSensors() {
	detectors = make([]*LaserDetector, cfg.TotalDetectors)
	for i := 0; i < cfg.TotalDetectors; i++ {
		position := float64(i) * 100.0
		detectors[i] = &LaserDetector{
			DeviceID:  fmt.Sprintf("LASER-%04d", i),
			Position:  position,
			Latitude:  39.9042 + (float64(i) * 0.0008) + (math.Sin(float64(i)*0.1) * 0.0002),
			Longitude: 116.4074 + (float64(i) * 0.001) + (math.Cos(float64(i)*0.05) * 0.0001),
			FireZone:  fmt.Sprintf("ZONE-%02d", (i/30)+1),
			BaseNoise: rand.Float64() * 0.5,
		}
	}

	envSensors = make([]*EnvSensor, cfg.TotalEnvSensors)
	for i := 0; i < cfg.TotalEnvSensors; i++ {
		sensorType := "temp_humidity"
		if i%2 == 0 {
			sensorType = "oxygen"
		}
		locationType := "vent"
		if i >= 25 {
			locationType = "valve"
		}

		position := float64(i) * 600.0
		envSensors[i] = &EnvSensor{
			DeviceID: func() string {
				if i%2 == 0 {
					return fmt.Sprintf("ENV-O2-%02d", (i/2)+1)
				}
				return fmt.Sprintf("ENV-TH-%02d", (i/2)+1)
			}(),
			SensorType:   sensorType,
			LocationType: locationType,
			Position:     position,
			Latitude:     39.9042 + (position * 0.0000008),
			Longitude:    116.4074 + (position * 0.000001),
			FireZone:     fmt.Sprintf("ZONE-%02d", (i*2)+1),
		}
	}

	log.Printf("Initialized %d detectors and %d environment sensors", len(detectors), len(envSensors))
}

func initMQTT() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID + "-" + fmt.Sprintf("%d", time.Now().Unix()))
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)

	client = mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", token.Error())
	}

	log.Println("Connected to MQTT broker")
}

func sendSensorData() {
	now := time.Now()

	for _, detector := range detectors {
		data := generateLaserData(detector, now)
		publishData(fmt.Sprintf("sensors/%s/data", detector.DeviceID), data)
	}

	for _, sensor := range envSensors {
		data := generateEnvData(sensor, now)
		publishData(fmt.Sprintf("sensors/%s/data", sensor.DeviceID), data)
	}
}

func generateLaserData(detector *LaserDetector, now time.Time) *SensorData {
	concentration := detector.BaseNoise + rand.Float64()*0.3

	if cfg.LeakEnabled {
		concentration += calculateGaussianPlume(detector.Position)
	}

	if concentration < 0 {
		concentration = 0
	}

	status := "normal"
	if concentration > 50 {
		status = "fault"
	} else if concentration > 20 {
		status = "alarm"
	}

	return &SensorData{
		DeviceID:      detector.DeviceID,
		Timestamp:     now.Format(time.RFC3339Nano),
		Concentration: concentration,
		Temperature:   25.0 + rand.Float64()*5.0,
		WindSpeed:     cfg.WindSpeed + rand.Float64()*0.5,
		WindDir:       cfg.WindDir + rand.Float64()*10.0,
		Status:        status,
	}
}

func generateEnvData(sensor *EnvSensor, now time.Time) *SensorData {
	data := &SensorData{
		DeviceID:  sensor.DeviceID,
		Timestamp: now.Format(time.RFC3339Nano),
		Status:    "normal",
	}

	if sensor.SensorType == "oxygen" {
		data.Oxygen = 20.5 + rand.Float64()*0.5
		if cfg.LeakEnabled {
			dist := math.Abs(sensor.Position - cfg.LeakPosition)
			data.Oxygen -= calculateGaussianPlume(sensor.Position) * 0.02
			if data.Oxygen < 19.5 {
				data.Status = "alarm"
			}
		}
	} else {
		data.Temperature = 20.0 + rand.Float64()*10.0
		data.Humidity = 50.0 + rand.Float64()*20.0
	}

	data.WindSpeed = cfg.WindSpeed + rand.Float64()*0.3
	data.WindDir = cfg.WindDir + rand.Float64()*5.0

	return data
}

func calculateGaussianPlume(position float64) float64 {
	distance := position - cfg.LeakPosition
	absDistance := math.Abs(distance)

	if absDistance < 1.0 {
		return cfg.LeakRate * 50.0
	}

	sigmaY := 0.22 * absDistance / math.Pow(1+0.0001*absDistance, 0.5)
	sigmaZ := 0.2 * absDistance

	windFactor := 1.0
	if cfg.WindSpeed > 0.1 {
		windFactor = 1.0 / (cfg.WindSpeed * math.Sqrt(2*math.Pi))
	}

	concentration := cfg.LeakRate * windFactor *
		math.Exp(-0.5*math.Pow(distance/sigmaY, 2)) *
		math.Exp(-0.5*math.Pow(1.5/sigmaZ, 2)) / (sigmaY * sigmaZ * math.Sqrt(2*math.Pi))

	return concentration * 1000
}

func publishData(topic string, data *SensorData) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal sensor data: %v", err)
		return
	}

	token := client.Publish(topic, 1, false, payload)
	go func() {
		if token.Wait() && token.Error() != nil {
			log.Printf("Failed to publish to %s: %v", topic, token.Error())
		}
	}()
}
