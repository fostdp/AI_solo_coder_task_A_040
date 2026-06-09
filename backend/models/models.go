package models

import (
	"time"

	"github.com/google/uuid"
)

type Detector struct {
	ID          string    `json:"id" db:"id"`
	DeviceID    string    `json:"device_id" db:"device_id"`
	Name        string    `json:"name" db:"name"`
	Position    float64   `json:"position" db:"position"`
	Latitude    float64   `json:"latitude" db:"latitude"`
	Longitude   float64   `json:"longitude" db:"longitude"`
	FireZone    string    `json:"fire_zone" db:"fire_zone"`
	Status      string    `json:"status" db:"status"`
	Health      float64   `json:"health" db:"health"`
	InstallDate time.Time `json:"install_date" db:"install_date"`
	LastCalib   time.Time `json:"last_calib" db:"last_calib"`
}

type SensorData struct {
	DeviceID   string    `json:"device_id"`
	Timestamp  time.Time `json:"timestamp"`
	Concentration float64 `json:"concentration"`
	Temperature float64   `json:"temperature,omitempty"`
	Humidity    float64   `json:"humidity,omitempty"`
	Oxygen      float64   `json:"oxygen,omitempty"`
	WindSpeed   float64   `json:"wind_speed,omitempty"`
	WindDir     float64   `json:"wind_dir,omitempty"`
	Status      string    `json:"status"`
}

type InfluxSensorData struct {
	Measurement string
	Tags        map[string]string
	Fields      map[string]interface{}
	Timestamp   time.Time
}

type Alarm struct {
	ID          uuid.UUID `json:"id" db:"id"`
	DeviceID    string    `json:"device_id" db:"device_id"`
	Level       int       `json:"level" db:"level"`
	LevelName   string    `json:"level_name" db:"level_name"`
	Concentration float64 `json:"concentration" db:"concentration"`
	Threshold   float64   `json:"threshold" db:"threshold"`
	Message     string    `json:"message" db:"message"`
	Timestamp   time.Time `json:"timestamp" db:"timestamp"`
	Acknowledged bool     `json:"acknowledged" db:"acknowledged"`
	Resolved    bool      `json:"resolved" db:"resolved"`
}

type LeakSource struct {
	ID          uuid.UUID `json:"id"`
	Position    float64   `json:"position"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	LeakRate    float64   `json:"leak_rate"`
	Confidence  float64   `json:"confidence"`
	DiffusionRadius float64 `json:"diffusion_radius"`
	DetectedAt  time.Time `json:"detected_at"`
}

type ValveControl struct {
	ID          uuid.UUID `json:"id" db:"id"`
	ValveID     string    `json:"valve_id" db:"valve_id"`
	Action      string    `json:"action" db:"action"`
	FireZone    string    `json:"fire_zone" db:"fire_zone"`
	TriggeredBy string    `json:"triggered_by" db:"triggered_by"`
	Timestamp   time.Time `json:"timestamp" db:"timestamp"`
	Success     bool      `json:"success" db:"success"`
}

type FanControl struct {
	ID         uuid.UUID `json:"id" db:"id"`
	FanID      string    `json:"fan_id" db:"fan_id"`
	Action     string    `json:"action" db:"action"`
	Speed      int       `json:"speed" db:"speed"`
	FireZone   string    `json:"fire_zone" db:"fire_zone"`
	Timestamp  time.Time `json:"timestamp" db:"timestamp"`
}

type DetectorHistoryPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	Concentration float64   `json:"concentration"`
}

type PipeCorridorPoint struct {
	Position  float64 `json:"position"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type HealthStatus struct {
	DeviceID    string  `json:"device_id"`
	Status      string  `json:"status"`
	Health      float64 `json:"health"`
	LastUpdate  time.Time `json:"last_update"`
	Temperature float64 `json:"temperature"`
	Voltage     float64 `json:"voltage"`
	Signal      float64 `json:"signal_strength"`
}
