package services

import (
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	metricsOnce     sync.Once
	metricsRegistry *prometheus.Registry
	StartTime       time.Time

	SensorDataReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_sensor_data_received_total",
			Help: "Total number of sensor data points received",
		},
		[]string{"device_id", "device_type"},
	)

	SensorDataValid = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_sensor_data_valid_total",
			Help: "Total number of valid sensor data points",
		},
		[]string{"device_id", "device_type"},
	)

	SensorDataInvalid = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_sensor_data_invalid_total",
			Help: "Total number of invalid sensor data points",
		},
		[]string{"device_id", "fail_reason"},
	)

	ConcentrationGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_concentration_percent_lel",
			Help: "Current gas concentration in %LEL",
		},
		[]string{"detector_id", "fire_zone"},
	)

	AlarmsTriggered = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_alarms_triggered_total",
			Help: "Total number of alarms triggered",
		},
		[]string{"level", "detector_id"},
	)

	AlarmsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_alarms_active",
			Help: "Number of currently active alarms",
		},
		[]string{"level"},
	)

	LeakSourcesDetected = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_leak_sources_detected_total",
			Help: "Total number of leak sources detected",
		},
		[]string{"algorithm", "confidence_level"},
	)

	LeakSourcesActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_leak_sources_active",
			Help: "Number of currently active leak sources",
		},
	)

	EmergencyCommandsSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_emergency_commands_sent_total",
			Help: "Total number of emergency commands sent",
		},
		[]string{"command_type", "zone", "status"},
	)

	MQTTMessagesPublished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_mqtt_messages_published_total",
			Help: "Total number of MQTT messages published",
		},
		[]string{"topic", "qos"},
	)

	MQTTMessagesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gas_monitoring_mqtt_messages_received_total",
			Help: "Total number of MQTT messages received",
		},
		[]string{"topic"},
	)

	InfluxDBWriteLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "gas_monitoring_influxdb_write_latency_seconds",
			Help:    "InfluxDB write latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	InfluxDBWriteErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gas_monitoring_influxdb_write_errors_total",
			Help: "Total number of InfluxDB write errors",
		},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gas_monitoring_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status_code"},
	)

	WebSocketConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gas_monitoring_websocket_connections",
			Help: "Number of active WebSocket connections",
		},
	)

	SystemUptime = promauto.NewCounterFunc(
		prometheus.CounterOpts{
			Name: "gas_monitoring_system_uptime_seconds",
			Help: "System uptime in seconds",
		},
		func() float64 {
			return time.Since(StartTime).Seconds()
		},
	)
)

func InitMetrics() {
	metricsOnce.Do(func() {
		metricsRegistry = prometheus.NewRegistry()

		metricsRegistry.MustRegister(
			SensorDataReceived,
			SensorDataValid,
			SensorDataInvalid,
			ConcentrationGauge,
			AlarmsTriggered,
			AlarmsActive,
			LeakSourcesDetected,
			LeakSourcesActive,
			EmergencyCommandsSent,
			MQTTMessagesPublished,
			MQTTMessagesReceived,
			InfluxDBWriteLatency,
			InfluxDBWriteErrors,
			HTTPRequestDuration,
			WebSocketConnections,
			SystemUptime,
		)
	})
}

func MetricsHandler() http.Handler {
	return promhttp.HandlerFor(
		metricsRegistry,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
		},
	)
}
