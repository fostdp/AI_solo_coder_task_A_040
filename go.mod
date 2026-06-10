module gas-monitoring-system

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
	github.com/gin-contrib/cors v1.5.0
	github.com/gin-contrib/gzip v1.0.1
	github.com/influxdata/influxdb-client-go/v2 v2.13.0
	github.com/jackc/pgx/v5 v5.5.0
	github.com/eclipse/paho.mqtt.golang v1.4.3
	github.com/spf13/viper v1.18.2
	github.com/google/uuid v1.5.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/gorilla/websocket v1.5.1
	github.com/prometheus/client_golang v1.17.0
	github.com/prometheus/client_model v0.5.0
	github.com/prometheus/common v0.44.0
	github.com/prometheus/procfs v0.12.0
)
