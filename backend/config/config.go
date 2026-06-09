package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server             ServerConfig             `mapstructure:"server"`
	Database           DatabaseConfig           `mapstructure:"database"`
	MQTT               MQTTConfig               `mapstructure:"mqtt"`
	Alarm              AlarmConfig              `mapstructure:"alarm"`
	SMS                SMSConfig                `mapstructure:"sms"`
	LeakDetection      LeakDetectionConfig      `mapstructure:"leak_detection"`
	PipeCorridor       PipeCorridorConfig       `mapstructure:"pipe_corridor"`
	LaserReceiver      LaserReceiverConfig      `mapstructure:"laser_receiver"`
	LeakLocator        LeakLocatorConfig        `mapstructure:"leak_locator"`
	EmergencyController EmergencyControllerConfig `mapstructure:"emergency_controller"`
	AlarmRouter        AlarmRouterConfig        `mapstructure:"alarm_router"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

type DatabaseConfig struct {
	PostgreSQL PostgreSQLConfig `mapstructure:"postgresql"`
	InfluxDB   InfluxDBConfig   `mapstructure:"influxdb"`
}

type PostgreSQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

type InfluxDBConfig struct {
	Host           string `mapstructure:"host"`
	Token          string `mapstructure:"token"`
	Org            string `mapstructure:"org"`
	Bucket         string `mapstructure:"bucket"`
	BatchSize      int    `mapstructure:"batch_size"`
	FlushIntervalMs int   `mapstructure:"flush_interval_ms"`
}

type MQTTConfig struct {
	Broker           string            `mapstructure:"broker"`
	ClientID         string            `mapstructure:"client_id"`
	Username         string            `mapstructure:"username"`
	Password         string            `mapstructure:"password"`
	Topics           map[string]string `mapstructure:"topics"`
	CommandTimeoutSec int              `mapstructure:"command_timeout_sec"`
	MaxRetries       int               `mapstructure:"max_retries"`
}

type AlarmConfig struct {
	Level1Threshold float64 `mapstructure:"level1_threshold"`
	Level2Threshold float64 `mapstructure:"level2_threshold"`
	Level3Threshold float64 `mapstructure:"level3_threshold"`
	CheckInterval   int     `mapstructure:"check_interval"`
}

type SMSConfig struct {
	APIURL    string   `mapstructure:"api_url"`
	APIKey    string   `mapstructure:"api_key"`
	Receivers []string `mapstructure:"receivers"`
}

type LeakDetectionConfig struct {
	PSOParticles        int     `mapstructure:"pso_particles"`
	PSOIterations       int     `mapstructure:"pso_iterations"`
	PSOInertiaWeight    float64 `mapstructure:"pso_inertia_weight"`
	PSOCognitiveWeight  float64 `mapstructure:"pso_cognitive_weight"`
	PSOSocialWeight     float64 `mapstructure:"pso_social_weight"`
	DiffusionRadiusBase float64 `mapstructure:"diffusion_radius_base"`
}

type PipeCorridorConfig struct {
	TotalLength     float64 `mapstructure:"total_length"`
	DetectorInterval float64 `mapstructure:"detector_interval"`
	TotalDetectors  int     `mapstructure:"total_detectors"`
	TotalValves     int     `mapstructure:"total_valves"`
}

var AppConfig *Config

func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigFile(configPath)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &Config{}
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	AppConfig = config
	return config, nil
}

func (c *PostgreSQLConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
}

type LaserReceiverConfig struct {
	MaxValidConcentration  float64       `mapstructure:"max_valid_concentration"`
	MaxDataAge             time.Duration `mapstructure:"max_data_age"`
	MaxFutureTime          time.Duration `mapstructure:"max_future_time"`
	AlarmForwardThreshold  float64       `mapstructure:"alarm_forward_threshold"`
	StatsInterval          time.Duration `mapstructure:"stats_interval"`
}

type LeakLocatorConfig struct {
	DetectionInterval       time.Duration `mapstructure:"detection_interval"`
	MaxReadingsPerDetector  int           `mapstructure:"max_readings_per_detector"`
	HighConcentrationThreshold float64    `mapstructure:"high_concentration_threshold"`
	MinHighConcReadings     int           `mapstructure:"min_high_conc_readings"`
	AtmosphericStability    float64       `mapstructure:"atmospheric_stability"`
	MinConfidence           float64       `mapstructure:"min_confidence"`
	MinConfidenceDegraded   float64       `mapstructure:"min_confidence_degraded"`
	LeakMergeDistance       float64       `mapstructure:"leak_merge_distance"`
	PSONumParticles         int           `mapstructure:"pso_num_particles"`
	PSOMaxIterations        int           `mapstructure:"pso_max_iterations"`
	PSOInertiaWeight        float64       `mapstructure:"pso_inertia_weight"`
	PSOCognitiveWeight      float64       `mapstructure:"pso_cognitive_weight"`
	PSOSocialWeight         float64       `mapstructure:"pso_social_weight"`
	SearchMinX              float64       `mapstructure:"search_min_x"`
	SearchMaxX              float64       `mapstructure:"search_max_x"`
	SearchMinRate           float64       `mapstructure:"search_min_rate"`
	SearchMaxRate           float64       `mapstructure:"search_max_rate"`
}

type EmergencyControllerConfig struct {
	ValveControlLevel    int           `mapstructure:"valve_control_level"`
	EvacuationLevel      int           `mapstructure:"evacuation_level"`
	FanSpeedNormal       int           `mapstructure:"fan_speed_normal"`
	FanSpeedHigh         int           `mapstructure:"fan_speed_high"`
	FanSpeedHighLevel    int           `mapstructure:"fan_speed_high_level"`
	ControlCooldown      time.Duration `mapstructure:"control_cooldown"`
}

type AlarmRouterConfig struct {
	Level1Threshold     float64       `mapstructure:"level1_threshold"`
	Level2Threshold     float64       `mapstructure:"level2_threshold"`
	Level3Threshold     float64       `mapstructure:"level3_threshold"`
	SMSMinLevel         int           `mapstructure:"sms_min_level"`
	SMSInterval         time.Duration `mapstructure:"sms_interval"`
	SMSMaxPerInterval   int           `mapstructure:"sms_max_per_interval"`
}
