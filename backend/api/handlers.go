package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/services"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) GetDetectors(c *gin.Context) {
	detectors, err := services.DB.GetAllDetectors()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	concentrations, _ := services.DB.GetCurrentConcentrations()

	type DetectorWithConc struct {
		*models.Detector
		CurrentConcentration float64 `json:"current_concentration"`
	}

	result := make([]DetectorWithConc, len(detectors))
	for i, d := range detectors {
		conc, _ := concentrations[d.DeviceID]
		result[i] = DetectorWithConc{
			Detector:             &d,
			CurrentConcentration: conc,
		}
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) GetDetector(c *gin.Context) {
	deviceID := c.Param("id")

	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT device_id, name, position, latitude, longitude, fire_zone, status, health, install_date, last_calib
		FROM detectors WHERE device_id = $1
	`, deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var detector models.Detector
	if !rows.Next() {
		c.JSON(http.StatusNotFound, gin.H{"error": "detector not found"})
		return
	}

	err = rows.Scan(&detector.DeviceID, &detector.Name, &detector.Position, &detector.Latitude,
		&detector.Longitude, &detector.FireZone, &detector.Status, &detector.Health,
		&detector.InstallDate, &detector.LastCalib)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, detector)
}

func (h *Handler) GetDetectorHistory(c *gin.Context) {
	deviceID := c.Param("id")
	hours := 1
	if hStr := c.Query("hours"); hStr != "" {
		if h, err := strconv.Atoi(hStr); err == nil {
			hours = h
		}
	}

	history, err := services.DB.GetDetectorHistory(deviceID, time.Duration(hours)*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, history)
}

func (h *Handler) GetDetectorHealth(c *gin.Context) {
	deviceID := c.Param("id")

	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT device_id, status, health, temperature, voltage, signal_strength, last_update
		FROM sensor_health WHERE device_id = $1 ORDER BY last_update DESC LIMIT 1
	`, deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var health models.HealthStatus
	if !rows.Next() {
		health = models.HealthStatus{
			DeviceID:   deviceID,
			Status:     "normal",
			Health:     100.0,
			LastUpdate: time.Now(),
		}
	} else {
		err = rows.Scan(&health.DeviceID, &health.Status, &health.Health,
			&health.Temperature, &health.Voltage, &health.Signal, &health.LastUpdate)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, health)
}

func (h *Handler) GetPipeCorridor(c *gin.Context) {
	path, err := services.DB.GetPipeCorridorPath()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, path)
}

func (h *Handler) GetCurrentConcentrations(c *gin.Context) {
	concentrations, err := services.DB.GetCurrentConcentrations()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, concentrations)
}

func (h *Handler) GetAlarms(c *gin.Context) {
	active := c.Query("active")

	if active == "true" {
		var alarms []*models.Alarm
		if services.AlarmRouter != nil && services.AlarmRouter.IsRunning() {
			alarms = services.AlarmRouter.GetActiveAlarms()
		} else {
			alarms = services.AlarmEngine.GetActiveAlarms()
		}
		c.JSON(http.StatusOK, alarms)
		return
	}

	limit := 100
	if lStr := c.Query("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil {
			limit = l
		}
	}

	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT id, device_id, level, level_name, concentration, threshold, message, timestamp, acknowledged, resolved
		FROM alarms ORDER BY timestamp DESC LIMIT $1
	`, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var alarms []models.Alarm
	for rows.Next() {
		var a models.Alarm
		err := rows.Scan(&a.ID, &a.DeviceID, &a.Level, &a.LevelName, &a.Concentration,
			&a.Threshold, &a.Message, &a.Timestamp, &a.Acknowledged, &a.Resolved)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		alarms = append(alarms, a)
	}

	c.JSON(http.StatusOK, alarms)
}

func (h *Handler) AcknowledgeAlarm(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid alarm id"})
		return
	}

	var req struct {
		AcknowledgedBy string `json:"acknowledged_by"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.AcknowledgedBy = "system"
	}

	if services.AlarmRouter != nil && services.AlarmRouter.IsRunning() {
		err = services.AlarmRouter.AcknowledgeAlarm(id, req.AcknowledgedBy)
	} else {
		err = services.AlarmEngine.AcknowledgeAlarm(id, req.AcknowledgedBy)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *Handler) GetLeaks(c *gin.Context) {
	var leaks []*models.LeakSource
	if services.LeakLocator != nil && services.LeakLocator.IsRunning() {
		leaks = services.LeakLocator.GetCurrentLeaks()
	} else {
		leaks = services.LeakDetector.GetCurrentLeaks()
	}
	c.JSON(http.StatusOK, leaks)
}

func (h *Handler) ResolveLeak(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid leak id"})
		return
	}

	if services.LeakLocator != nil && services.LeakLocator.IsRunning() {
		err = services.LeakLocator.ResolveLeak(id)
	} else {
		err = services.LeakDetector.ResolveLeak(id)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func (h *Handler) GetValves(c *gin.Context) {
	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT valve_id, name, fire_zone, status, latitude, longitude
		FROM valves ORDER BY valve_id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type Valve struct {
		ValveID   string  `json:"valve_id"`
		Name      string  `json:"name"`
		FireZone  string  `json:"fire_zone"`
		Status    string  `json:"status"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}

	var valves []Valve
	for rows.Next() {
		var v Valve
		err := rows.Scan(&v.ValveID, &v.Name, &v.FireZone, &v.Status, &v.Latitude, &v.Longitude)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		valves = append(valves, v)
	}

	c.JSON(http.StatusOK, valves)
}

func (h *Handler) ControlValve(c *gin.Context) {
	valveID := c.Param("id")

	var req struct {
		Action string `json:"action" binding:"required,oneof=open close"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ec := services.NewEmergencyControlService()
	fireZone := ""
	rows, _ := services.DB.PG().Query(context.Background(),
		"SELECT fire_zone FROM valves WHERE valve_id = $1", valveID)
	if rows.Next() {
		rows.Scan(&fireZone)
	}
	rows.Close()

	alarm := &models.Alarm{
		ID:            uuid.New(),
		DeviceID:      "manual",
		Level:         2,
		Concentration: 0,
	}

	if req.Action == "close" {
		query := `UPDATE valves SET status = 'closed', last_action = NOW() WHERE valve_id = $1`
		services.DB.PG().Exec(context.Background(), query, valveID)
	} else {
		query := `UPDATE valves SET status = 'open', last_action = NOW() WHERE valve_id = $1`
		services.DB.PG().Exec(context.Background(), query, valveID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"valve_id": valveID,
		"action":   req.Action,
	})
}

func (h *Handler) GetFans(c *gin.Context) {
	rows, err := services.DB.PG().Query(context.Background(), `
		SELECT fan_id, name, fire_zone, status, speed, latitude, longitude
		FROM fans ORDER BY fan_id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type Fan struct {
		FanID     string  `json:"fan_id"`
		Name      string  `json:"name"`
		FireZone  string  `json:"fire_zone"`
		Status    string  `json:"status"`
		Speed     int     `json:"speed"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}

	var fans []Fan
	for rows.Next() {
		var f Fan
		err := rows.Scan(&f.FanID, &f.Name, &f.FireZone, &f.Status, &f.Speed, &f.Latitude, &f.Longitude)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		fans = append(fans, f)
	}

	c.JSON(http.StatusOK, fans)
}

func (h *Handler) ControlFan(c *gin.Context) {
	fanID := c.Param("id")

	var req struct {
		Action string `json:"action" binding:"required,oneof=start stop"`
		Speed  int    `json:"speed"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Action == "start" && req.Speed == 0 {
		req.Speed = 75
	}

	if req.Action == "start" {
		query := `UPDATE fans SET status = 'running', speed = $1 WHERE fan_id = $2`
		services.DB.PG().Exec(context.Background(), query, req.Speed, fanID)
	} else {
		query := `UPDATE fans SET status = 'stopped', speed = 0 WHERE fan_id = $1`
		services.DB.PG().Exec(context.Background(), query, fanID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"fan_id":   fanID,
		"action":   req.Action,
		"speed":    req.Speed,
	})
}

func (h *Handler) ResetZone(c *gin.Context) {
	fireZone := c.Param("zone")

	var err error
	if services.EmergencyController != nil && services.EmergencyController.IsRunning() {
		err = services.EmergencyController.ResetZone(fireZone)
	} else {
		ec := services.NewEmergencyControlService()
		err = ec.ResetZone(fireZone)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"fire_zone": fireZone,
		"message":   "防火分区已重置，阀门已打开，风机已停止",
	})
}

func (h *Handler) GetWindData(c *gin.Context) {
	var windSpeed, windDir, temperature float64
	if services.LeakLocator != nil && services.LeakLocator.IsRunning() {
		windSpeed, windDir, temperature = services.LeakLocator.GetWindData()
	} else {
		windSpeed, windDir, temperature = services.LeakDetector.GetWindData()
	}
	c.JSON(http.StatusOK, gin.H{
		"wind_speed":  windSpeed,
		"wind_dir":    windDir,
		"temperature": temperature,
	})
}

func (h *Handler) GetStatistics(c *gin.Context) {
	var activeAlarms, activeLeaks int
	if services.AlarmRouter != nil && services.AlarmRouter.IsRunning() {
		activeAlarms = len(services.AlarmRouter.GetActiveAlarms())
	} else {
		activeAlarms = len(services.AlarmEngine.GetActiveAlarms())
	}
	if services.LeakLocator != nil && services.LeakLocator.IsRunning() {
		activeLeaks = len(services.LeakLocator.GetCurrentLeaks())
	} else {
		activeLeaks = len(services.LeakDetector.GetCurrentLeaks())
	}

	rows, _ := services.DB.PG().Query(context.Background(),
		"SELECT COUNT(*) FROM detectors")
	totalDetectors := 0
	if rows.Next() {
		rows.Scan(&totalDetectors)
	}
	rows.Close()

	rows, _ = services.DB.PG().Query(context.Background(),
		"SELECT COUNT(*) FROM detectors WHERE status = 'normal'")
	onlineDetectors := 0
	if rows.Next() {
		rows.Scan(&onlineDetectors)
	}
	rows.Close()

	concentrations, _ := services.DB.GetCurrentConcentrations()
	var avgConcentration, maxConcentration float64
	if len(concentrations) > 0 {
		var sum float64
		for _, conc := range concentrations {
			sum += conc
			if conc > maxConcentration {
				maxConcentration = conc
			}
		}
		avgConcentration = sum / float64(len(concentrations))
	}

	c.JSON(http.StatusOK, gin.H{
		"total_detectors":    totalDetectors,
		"online_detectors":   onlineDetectors,
		"active_alarms":      activeAlarms,
		"active_leak_sources": activeLeaks,
		"avg_concentration":  avgConcentration,
		"max_concentration":  maxConcentration,
	})
}

func (h *Handler) WebSocket(c *gin.Context) {
	services.WSService.HandleConnection(c.Writer, c.Request)
}

func (h *Handler) GetReceiverStats(c *gin.Context) {
	if services.LaserReceiver != nil && services.LaserReceiver.IsRunning() {
		stats := services.LaserReceiver.GetStats()
		c.JSON(http.StatusOK, stats)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"running": false,
	})
}

func (h *Handler) Health(c *gin.Context) {
	status := gin.H{
		"status": "ok",
		"time":   time.Now(),
		"modules": gin.H{
			"laser_receiver":       services.LaserReceiver != nil && services.LaserReceiver.IsRunning(),
			"alarm_router":         services.AlarmRouter != nil && services.AlarmRouter.IsRunning(),
			"leak_locator":         services.LeakLocator != nil && services.LeakLocator.IsRunning(),
			"emergency_controller": services.EmergencyController != nil && services.EmergencyController.IsRunning(),
		},
	}
	c.JSON(http.StatusOK, status)
}
