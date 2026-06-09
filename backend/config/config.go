package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	MQTT          MQTTConfig          `mapstructure:"mqtt"`
	Alarm         AlarmConfig         `mapstructure:"alarm"`
	SMS           SMSConfig           `mapstructure:"sms"`
	LeakDetection LeakDetectionConfig `mapstructure:"leak_detection"`
	PipeCorridor  PipeCorridorConfig  `mapstructure:"pipe_corridor"`
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
	Host   string `mapstructure:"host"`
	Token  string `mapstructure:"token"`
	Org    string `mapstructure:"org"`
	Bucket string `mapstructure:"bucket"`
}

type MQTTConfig struct {
	Broker   string            `mapstructure:"broker"`
	ClientID string            `mapstructure:"client_id"`
	Username string            `mapstructure:"username"`
	Password string            `mapstructure:"password"`
	Topics   map[string]string `mapstructure:"topics"`
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
