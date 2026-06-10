package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

type LeakSource struct {
	ID       string  `json:"id"`
	Position float64 `json:"position"`
	Rate     float64 `json:"rate"`
	Enabled  bool    `json:"enabled"`
}

type SimulatorConfig struct {
	Broker         string
	ClientID       string
	Username       string
	Password       string
	TotalDetectors int
	TotalEnvSensors int
	IntervalMs     int
	CorridorLength float64
	WindSpeed      float64
	WindDir        float64
	APIPort        string
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

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

var (
	cfg         SimulatorConfig
	client      mqtt.Client
	detectors   []*LaserDetector
	envSensors  []*EnvSensor
	leakSources []*LeakSource
	leakMutex   sync.RWMutex
	windMutex   sync.RWMutex

	messagesPublished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "simulator_messages_published_total",
			Help: "Total number of messages published",
		},
		[]string{"device_type"},
	)

	concentrationGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "simulator_concentration_percent_lel",
			Help: "Simulated concentration in %LEL",
		},
		[]string{"detector_id"},
	)

	activeLeaksGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "simulator_active_leaks",
			Help: "Number of active leak sources",
		},
	)

	windSpeedGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "simulator_wind_speed_mps",
			Help: "Simulated wind speed in m/s",
		},
	)

	windDirGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "simulator_wind_direction_deg",
			Help: "Simulated wind direction in degrees",
		},
	)

	simulatorUptime = promauto.NewCounterFunc(
		prometheus.CounterOpts{
			Name: "simulator_uptime_seconds",
			Help: "Simulator uptime in seconds",
		},
		func() float64 {
			return time.Since(startTime).Seconds()
		},
	)

	startTime = time.Now()
)

func main() {
	flag.StringVar(&cfg.Broker, "broker", getEnv("MQTT_BROKER", "tcp://localhost:1883"), "MQTT broker address")
	flag.StringVar(&cfg.ClientID, "client-id", getEnv("MQTT_CLIENT_ID", "sensor-simulator"), "MQTT client ID")
	flag.StringVar(&cfg.Username, "username", getEnv("MQTT_USER", "admin"), "MQTT username")
	flag.StringVar(&cfg.Password, "password", getEnv("MQTT_PASSWORD", "admin123"), "MQTT password")
	flag.IntVar(&cfg.TotalDetectors, "detectors", getEnvInt("SIMULATOR_DETECTORS", 300), "Total number of laser detectors")
	flag.IntVar(&cfg.TotalEnvSensors, "env-sensors", 50, "Total number of environment sensors")
	flag.IntVar(&cfg.IntervalMs, "interval", getEnvInt("SIMULATOR_INTERVAL", 1000), "Data sending interval in milliseconds")
	flag.Float64Var(&cfg.CorridorLength, "corridor-length", getEnvFloat("SIMULATOR_CORRIDOR_LENGTH", 30000), "Corridor length in meters")
	flag.Float64Var(&cfg.WindSpeed, "wind-speed", getEnvFloat("SIMULATOR_WIND_SPEED", 1.5), "Wind speed in m/s")
	flag.Float64Var(&cfg.WindDir, "wind-dir", getEnvFloat("SIMULATOR_WIND_DIR", 90.0), "Wind direction in degrees")
	flag.StringVar(&cfg.APIPort, "api-port", "8081", "HTTP API port")
	flag.Parse()

	if getEnvBool("SIMULATOR_LEAK_ENABLED", false) {
		leakSources = append(leakSources, &LeakSource{
			ID:       "default-leak",
			Position: getEnvFloat("SIMULATOR_LEAK_POSITION", 15000),
			Rate:     getEnvFloat("SIMULATOR_LEAK_RATE", 1.0),
			Enabled:  true,
		})
	}

	rand.Seed(time.Now().UnixNano())

	initSensors()
	initMQTT()
	defer client.Disconnect(250)

	go startHTTPServer()

	log.Printf("=== 激光检测器模拟器启动 ===")
	log.Printf("  管廊长度: %.0f 米 (%.1f 公里)", cfg.CorridorLength, cfg.CorridorLength/1000)
	log.Printf("  激光检测器: %d 台 (每 %.1f 米1台)", cfg.TotalDetectors, cfg.CorridorLength/float64(cfg.TotalDetectors))
	log.Printf("  环境传感器: %d 台", cfg.TotalEnvSensors)
	log.Printf("  上报间隔: %d 毫秒", cfg.IntervalMs)
	log.Printf("  初始风速: %.1f m/s, 风向: %.0f°", cfg.WindSpeed, cfg.WindDir)
	log.Printf("  HTTP API: :%s", cfg.APIPort)
	log.Printf("  MQTT Broker: %s", cfg.Broker)

	if len(leakSources) > 0 {
		log.Printf("  初始泄漏源:")
		for _, leak := range leakSources {
			if leak.Enabled {
				log.Printf("    - %s: 位置 %.0f 米, 速率 %.2f L/s", leak.ID, leak.Position, leak.Rate)
			}
		}
	}

	ticker := time.NewTicker(time.Duration(cfg.IntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		sendSensorData()
	}
}

func initSensors() {
	spacing := cfg.CorridorLength / float64(cfg.TotalDetectors)
	detectors = make([]*LaserDetector, cfg.TotalDetectors)
	for i := 0; i < cfg.TotalDetectors; i++ {
		position := float64(i) * spacing
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

		position := float64(i) * (cfg.CorridorLength / float64(cfg.TotalEnvSensors))
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

	log.Printf("初始化 %d 个激光检测器和 %d 个环境传感器", len(detectors), len(envSensors))
}

func initMQTT() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID(cfg.ClientID + "-" + fmt.Sprintf("%d", time.Now().Unix()))
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(30 * time.Second)
	opts.SetCleanSession(false)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetConnectionLostHandler(func(c mqtt.Client, err error) {
		log.Printf("MQTT连接断开: %v", err)
	})
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("MQTT连接成功，会话已恢复")
	})

	client = mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("连接MQTT Broker失败: %v", token.Error())
	}

	log.Println("MQTT Broker连接成功 (QoS 2, 持久会话)")
}

func sendSensorData() {
	now := time.Now()
	windSpeed, windDir := getCurrentWind()

	activeLeaks := 0
	for _, detector := range detectors {
		data := generateLaserData(detector, now, windSpeed, windDir)
		publishData(fmt.Sprintf("sensors/%s/data", detector.DeviceID), data)
		concentrationGauge.WithLabelValues(detector.DeviceID).Set(data.Concentration)
	}
	messagesPublished.WithLabelValues("laser").Add(float64(len(detectors)))

	for _, sensor := range envSensors {
		data := generateEnvData(sensor, now, windSpeed, windDir)
		publishData(fmt.Sprintf("sensors/%s/data", sensor.DeviceID), data)
	}
	messagesPublished.WithLabelValues("environment").Add(float64(len(envSensors)))

	leakMutex.RLock()
	for _, leak := range leakSources {
		if leak.Enabled {
			activeLeaks++
		}
	}
	leakMutex.RUnlock()
	activeLeaksGauge.Set(float64(activeLeaks))

	windSpeedGauge.Set(windSpeed)
	windDirGauge.Set(windDir)
}

func generateLaserData(detector *LaserDetector, now time.Time, windSpeed, windDir float64) *SensorData {
	concentration := detector.BaseNoise + rand.Float64()*0.3

	leakMutex.RLock()
	for _, leak := range leakSources {
		if leak.Enabled {
			concentration += calculateGaussianPlume(detector.Position, leak.Position, leak.Rate, windSpeed)
		}
	}
	leakMutex.RUnlock()

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
		WindSpeed:     windSpeed + rand.Float64()*0.5,
		WindDir:       windDir + rand.Float64()*10.0,
		Status:        status,
	}
}

func generateEnvData(sensor *EnvSensor, now time.Time, windSpeed, windDir float64) *SensorData {
	data := &SensorData{
		DeviceID:  sensor.DeviceID,
		Timestamp: now.Format(time.RFC3339Nano),
		Status:    "normal",
	}

	if sensor.SensorType == "oxygen" {
		data.Oxygen = 20.5 + rand.Float64()*0.5
		leakMutex.RLock()
		for _, leak := range leakSources {
			if leak.Enabled {
				dist := math.Abs(sensor.Position - leak.Position)
				data.Oxygen -= calculateGaussianPlume(sensor.Position, leak.Position, leak.Rate, windSpeed) * 0.02
			}
		}
		leakMutex.RUnlock()
		if data.Oxygen < 19.5 {
			data.Status = "alarm"
		}
	} else {
		data.Temperature = 20.0 + rand.Float64()*10.0
		data.Humidity = 50.0 + rand.Float64()*20.0
	}

	data.WindSpeed = windSpeed + rand.Float64()*0.3
	data.WindDir = windDir + rand.Float64()*5.0

	return data
}

func calculateGaussianPlume(position, leakPosition, leakRate, windSpeed float64) float64 {
	distance := position - leakPosition
	absDistance := math.Abs(distance)

	if absDistance < 1.0 {
		return leakRate * 50.0
	}

	sigmaY := 0.22 * absDistance / math.Pow(1+0.0001*absDistance, 0.5)
	sigmaZ := 0.2 * absDistance

	windFactor := 1.0
	if windSpeed > 0.1 {
		windFactor = 1.0 / (windSpeed * math.Sqrt(2*math.Pi))
	}

	concentration := leakRate * windFactor *
		math.Exp(-0.5*math.Pow(distance/sigmaY, 2)) *
		math.Exp(-0.5*math.Pow(1.5/sigmaZ, 2)) / (sigmaY * sigmaZ * math.Sqrt(2*math.Pi))

	return concentration * 1000
}

func publishData(topic string, data *SensorData) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("序列化传感器数据失败: %v", err)
		return
	}

	token := client.Publish(topic, 2, true, payload)
	go func() {
		if token.Wait() && token.Error() != nil {
			log.Printf("发布到 %s 失败: %v", topic, token.Error())
		}
	}()
}

func getCurrentWind() (float64, float64) {
	windMutex.RLock()
	defer windMutex.RUnlock()
	return cfg.WindSpeed, cfg.WindDir
}

func startHTTPServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/config", handleGetConfig)
	mux.HandleFunc("/api/wind", handleWind)
	mux.HandleFunc("/api/leaks", handleLeaks)
	mux.HandleFunc("/api/leaks/add", handleAddLeak)
	mux.HandleFunc("/api/leaks/remove", handleRemoveLeak)
	mux.HandleFunc("/api/leaks/toggle", handleToggleLeak)
	mux.HandleFunc("/api/reset", handleReset)
	mux.Handle("/metrics", promhttp.Handler())

	log.Printf("HTTP API服务器启动在 :%s", cfg.APIPort)
	if err := http.ListenAndServe(":"+cfg.APIPort, mux); err != nil {
		log.Printf("HTTP服务器错误: %v", err)
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"status":    "running",
			"uptime":    time.Since(startTime).Seconds(),
			"detectors": len(detectors),
			"sensors":   len(envSensors),
		},
	})
}

func handleGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"corridor_length": cfg.CorridorLength,
			"detectors":       cfg.TotalDetectors,
			"env_sensors":     cfg.TotalEnvSensors,
			"interval_ms":     cfg.IntervalMs,
			"broker":          cfg.Broker,
		},
	})
}

func handleWind(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		windSpeed, windDir := getCurrentWind()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(APIResponse{
			Success: true,
			Data: map[string]interface{}{
				"wind_speed": windSpeed,
				"wind_dir":   windDir,
			},
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			WindSpeed *float64 `json:"wind_speed"`
			WindDir   *float64 `json:"wind_dir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
			return
		}

		windMutex.Lock()
		if req.WindSpeed != nil {
			cfg.WindSpeed = *req.WindSpeed
		}
		if req.WindDir != nil {
			cfg.WindDir = *req.WindDir
		}
		windMutex.Unlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(APIResponse{
			Success: true,
			Message: "风速风向已更新",
			Data: map[string]interface{}{
				"wind_speed": cfg.WindSpeed,
				"wind_dir":   cfg.WindDir,
			},
		})
	}
}

func handleLeaks(w http.ResponseWriter, r *http.Request) {
	leakMutex.RLock()
	defer leakMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    leakSources,
	})
}

func handleAddLeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req LeakSource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	if req.ID == "" {
		req.ID = fmt.Sprintf("leak-%d", time.Now().Unix())
	}
	if req.Position < 0 || req.Position > cfg.CorridorLength {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: fmt.Sprintf("泄漏位置必须在0-%.0f米之间", cfg.CorridorLength)})
		return
	}
	if req.Rate <= 0 {
		req.Rate = 1.0
	}
	req.Enabled = true

	leakMutex.Lock()
	leakSources = append(leakSources, &req)
	leakMutex.Unlock()

	log.Printf("新增泄漏源: %s, 位置: %.0f米, 速率: %.2f L/s", req.ID, req.Position, req.Rate)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "泄漏源已添加",
		Data:    req,
	})
}

func handleRemoveLeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	leakMutex.Lock()
	deleted := false
	for i, leak := range leakSources {
		if leak.ID == req.ID {
			leakSources = append(leakSources[:i], leakSources[i+1:]...)
			deleted = true
			break
		}
	}
	leakMutex.Unlock()

	if !deleted {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "泄漏源不存在"})
		return
	}

	log.Printf("移除泄漏源: %s", req.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "泄漏源已移除",
	})
}

func handleToggleLeak(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID      string `json:"id"`
		Enabled *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "无效的请求体"})
		return
	}

	leakMutex.Lock()
	found := false
	for _, leak := range leakSources {
		if leak.ID == req.ID {
			if req.Enabled != nil {
				leak.Enabled = *req.Enabled
			} else {
				leak.Enabled = !leak.Enabled
			}
			found = true
			log.Printf("泄漏源 %s 状态: %v", req.ID, leak.Enabled)
			break
		}
	}
	leakMutex.Unlock()

	if !found {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "泄漏源不存在"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "泄漏源状态已更新",
	})
}

func handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	leakMutex.Lock()
	leakSources = make([]*LeakSource, 0)
	leakMutex.Unlock()

	windMutex.Lock()
	cfg.WindSpeed = 1.5
	cfg.WindDir = 90.0
	windMutex.Unlock()

	log.Println("模拟器已重置: 清除所有泄漏源，风速风向恢复默认")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "模拟器已重置",
	})
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		var floatValue float64
		if _, err := fmt.Sscanf(value, "%f", &floatValue); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}
