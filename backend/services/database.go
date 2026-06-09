package services

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2api "github.com/influxdata/influxdb-client-go/v2/api"
	influxdb2options "github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/jackc/pgx/v5/pgxpool"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
)

type DatabaseService struct {
	pgPool      *pgxpool.Pool
	influxClient influxdb2.Client
	writeAPI     influxdb2api.WriteAPI
	queryAPI     influxdb2api.QueryAPI
	
	batchSize     int
	flushInterval time.Duration
	batchMutex    sync.Mutex
	batchPoints   []*influxdb2options.Point
	lastFlush     time.Time
	batchTimer    *time.Timer
}

var DB *DatabaseService

func InitDatabase(cfg *config.Config) error {
	var err error
	DB, err = NewDatabaseService(cfg)
	return err
}

func NewDatabaseService(cfg *config.Config) (*DatabaseService, error) {
	pgPool, err := initPostgreSQL(cfg.Database.PostgreSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to init PostgreSQL: %w", err)
	}

	influxClient, writeAPI, queryAPI := initInfluxDB(cfg.Database.InfluxDB)

	batchSize := 5000
	flushInterval := 100 * time.Millisecond
	if cfg.Database.InfluxDB.BatchSize > 0 {
		batchSize = cfg.Database.InfluxDB.BatchSize
	}
	if cfg.Database.InfluxDB.FlushIntervalMs > 0 {
		flushInterval = time.Duration(cfg.Database.InfluxDB.FlushIntervalMs) * time.Millisecond
	}

	db := &DatabaseService{
		pgPool:        pgPool,
		influxClient:  influxClient,
		writeAPI:      writeAPI,
		queryAPI:      queryAPI,
		batchSize:     batchSize,
		flushInterval: flushInterval,
		batchPoints:   make([]*influxdb2options.Point, 0, batchSize),
		lastFlush:     time.Now(),
	}

	db.startBatchFlusher()

	return db, nil
}

func initPostgreSQL(cfg config.PostgreSQLConfig) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	poolConfig.MaxConns = 20
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	log.Println("PostgreSQL connected successfully")
	return pool, nil
}

func initInfluxDB(cfg config.InfluxDBConfig) (influxdb2.Client, influxdb2api.WriteAPI, influxdb2api.QueryAPI) {
	client := influxdb2.NewClient(cfg.Host, cfg.Token)
	writeAPI := client.WriteAPI(cfg.Org, cfg.Bucket)
	queryAPI := client.QueryAPI(cfg.Org)

	writeAPI.SetWriteFailedCallback(func(batch string, err error, retryCount int) bool {
		log.Printf("InfluxDB write failed (retry %d): %v\n", retryCount, err)
		return retryCount < 3
	})

	log.Println("InfluxDB client initialized")
	return client, writeAPI, queryAPI
}

func (d *DatabaseService) Close() {
	if d.batchTimer != nil {
		d.batchTimer.Stop()
	}
	
	d.Flush()
	
	if d.pgPool != nil {
		d.pgPool.Close()
		log.Println("PostgreSQL connection closed")
	}
	if d.influxClient != nil {
		d.writeAPI.Flush()
		d.influxClient.Close()
		log.Println("InfluxDB client closed")
	}
}

func (d *DatabaseService) PG() *pgxpool.Pool {
	return d.pgPool
}

func (d *DatabaseService) startBatchFlusher() {
	d.batchTimer = time.AfterFunc(d.flushInterval, func() {
		d.flushBatch()
		d.batchTimer.Reset(d.flushInterval)
	})
}

func (d *DatabaseService) WriteSensorData(data *models.InfluxSensorData) {
	point := influxdb2.NewPointWithMeasurement(data.Measurement)
	for k, v := range data.Tags {
		point.AddTag(k, v)
	}
	for k, v := range data.Fields {
		point.AddField(k, v)
	}
	point.SetTime(data.Timestamp)

	d.batchMutex.Lock()
	d.batchPoints = append(d.batchPoints, point)
	
	shouldFlush := len(d.batchPoints) >= d.batchSize
	d.batchMutex.Unlock()

	if shouldFlush {
		go d.flushBatch()
	}
}

func (d *DatabaseService) WriteSensorDataBatch(data []*models.InfluxSensorData) {
	points := make([]*influxdb2options.Point, 0, len(data))
	for _, item := range data {
		point := influxdb2.NewPointWithMeasurement(item.Measurement)
		for k, v := range item.Tags {
			point.AddTag(k, v)
		}
		for k, v := range item.Fields {
			point.AddField(k, v)
		}
		point.SetTime(item.Timestamp)
		points = append(points, point)
	}

	d.batchMutex.Lock()
	d.batchPoints = append(d.batchPoints, points...)
	
	shouldFlush := len(d.batchPoints) >= d.batchSize
	d.batchMutex.Unlock()

	if shouldFlush {
		go d.flushBatch()
	}
}

func (d *DatabaseService) flushBatch() {
	d.batchMutex.Lock()
	
	if len(d.batchPoints) == 0 {
		d.batchMutex.Unlock()
		return
	}
	
	points := d.batchPoints
	d.batchPoints = make([]*influxdb2options.Point, 0, d.batchSize)
	d.lastFlush = time.Now()
	d.batchMutex.Unlock()

	startTime := time.Now()
	d.writeAPI.WritePoint(points...)
	
	if len(points) > 1000 {
		log.Printf("InfluxDB batch flush: %d points in %v", len(points), time.Since(startTime))
	}
}

func (d *DatabaseService) Flush() {
	d.flushBatch()
	d.writeAPI.Flush()
}

func (d *DatabaseService) QueryInfluxDB(query string) (*influxdb2api.QueryTableResult, error) {
	return d.queryAPI.Query(context.Background(), query)
}

func (d *DatabaseService) GetDetectorHistory(deviceID string, duration time.Duration) ([]models.DetectorHistoryPoint, error) {
	query := fmt.Sprintf(`
		from(bucket: "%s")
			|> range(start: -%dh)
			|> filter(fn: (r) => r._measurement == "laser_methane" and r.device_id == "%s" and r._field == "concentration")
			|> aggregateWindow(every: 10s, fn: mean, createEmpty: false)
			|> yield(name: "mean")
	`, config.AppConfig.Database.InfluxDB.Bucket, int(duration.Hours()), deviceID)

	result, err := d.queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	var points []models.DetectorHistoryPoint
	for result.Next() {
		if result.Record().Field() == "concentration" {
			points = append(points, models.DetectorHistoryPoint{
				Timestamp:     result.Record().Time(),
				Concentration: result.Record().Value().(float64),
			})
		}
	}

	if result.Err() != nil {
		return nil, result.Err()
	}

	return points, nil
}

func (d *DatabaseService) GetAllDetectors() ([]models.Detector, error) {
	rows, err := d.pgPool.Query(context.Background(), `
		SELECT device_id, name, position, latitude, longitude, fire_zone, status, health, install_date, last_calib
		FROM detectors
		ORDER BY position
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var detectors []models.Detector
	for rows.Next() {
		var d models.Detector
		err := rows.Scan(&d.DeviceID, &d.Name, &d.Position, &d.Latitude, &d.Longitude, &d.FireZone, &d.Status, &d.Health, &d.InstallDate, &d.LastCalib)
		if err != nil {
			return nil, err
		}
		detectors = append(detectors, d)
	}

	return detectors, rows.Err()
}

func (d *DatabaseService) GetPipeCorridorPath() ([]models.PipeCorridorPoint, error) {
	rows, err := d.pgPool.Query(context.Background(), `
		SELECT position, latitude, longitude
		FROM pipe_corridor_path
		ORDER BY position
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []models.PipeCorridorPoint
	for rows.Next() {
		var p models.PipeCorridorPoint
		err := rows.Scan(&p.Position, &p.Latitude, &p.Longitude)
		if err != nil {
			return nil, err
		}
		points = append(points, p)
	}

	return points, rows.Err()
}

func (d *DatabaseService) GetCurrentConcentrations() (map[string]float64, error) {
	query := fmt.Sprintf(`
		from(bucket: "%s")
			|> range(start: -5s)
			|> filter(fn: (r) => r._measurement == "laser_methane" and r._field == "concentration")
			|> last()
	`, config.AppConfig.Database.InfluxDB.Bucket)

	result, err := d.queryAPI.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}

	concentrations := make(map[string]float64)
	for result.Next() {
		deviceID := result.Record().ValueByKey("device_id").(string)
		if result.Record().Field() == "concentration" {
			concentrations[deviceID] = result.Record().Value().(float64)
		}
	}

	return concentrations, result.Err()
}

func (d *DatabaseService) LogValveControl(valveID, action, reason string, success bool, errorMsg string) {
	query := `
		INSERT INTO valve_control_logs (valve_id, action, reason, success, error_message, timestamp)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`
	_, err := d.pgPool.Exec(context.Background(), query, valveID, action, reason, success, errorMsg)
	if err != nil {
		log.Printf("Failed to log valve control: %v", err)
	}
}

func (d *DatabaseService) LogFanControl(fanID, action, reason string, success bool, errorMsg string) {
	query := `
		INSERT INTO fan_control_logs (fan_id, action, reason, success, error_message, timestamp)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`
	_, err := d.pgPool.Exec(context.Background(), query, fanID, action, reason, success, errorMsg)
	if err != nil {
		log.Printf("Failed to log fan control: %v", err)
	}
}

func (d *DatabaseService) GetPendingCommandCount() int {
	d.batchMutex.Lock()
	defer d.batchMutex.Unlock()
	return len(d.batchPoints)
}
