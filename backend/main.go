package main

import (
	"context"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gas-monitoring-system/backend/api"
	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/models"
	"gas-monitoring-system/backend/modules/alarm_router"
	"gas-monitoring-system/backend/modules/emergency_controller"
	"gas-monitoring-system/backend/modules/laser_receiver"
	"gas-monitoring-system/backend/modules/leak_locator"
	"gas-monitoring-system/backend/mqtt"
	"gas-monitoring-system/backend/services"
)

var (
	version   = "dev"
	buildTime = "unknown"
	startTime = time.Now()
)

func main() {
	log.Printf("=== 智慧城市地下综合管廊燃气泄漏监测系统启动 ===")
	log.Printf("Version: %s, BuildTime: %s", version, buildTime)

	go func() {
		log.Println("Starting pprof server on :6060")
		if err := http.ListenAndServe("0.0.0.0:6060", nil); err != nil {
			log.Printf("pprof server error: %v", err)
		}
	}()

	services.InitMetrics()
	services.StartTime = startTime

	cfg, err := config.LoadConfig("config/config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	log.Println("配置加载成功")

	log.Println("正在初始化数据库...")
	dbService, err := services.NewDatabaseService(cfg)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	services.DB = dbService
	defer dbService.Close()
	log.Println("数据库初始化成功")

	log.Println("正在初始化WebSocket服务...")
	services.InitWebSocket()
	log.Println("WebSocket服务初始化成功")

	log.Println("正在初始化MQTT客户端...")
	mqttService, err := mqtt.NewMQTTService(&cfg.MQTT)
	if err != nil {
		log.Fatalf("初始化MQTT客户端失败: %v", err)
	}
	mqtt.MQTT = mqttService
	defer mqttService.Disconnect()
	log.Println("MQTT客户端初始化成功")

	log.Println("正在初始化泄漏检测器(旧版)...")
	if err := services.InitLeakDetector(&cfg.LeakDetection); err != nil {
		log.Printf("初始化泄漏检测器警告: %v", err)
	}
	log.Println("泄漏检测器初始化成功")

	alarmDataChan := make(chan models.ValidatedData, 1000)
	leakDataChan := make(chan models.ValidatedData, 1000)
	alarmChan := make(chan *models.Alarm, 100)
	leakChan := make(chan *models.LeakSource, 100)

	log.Println("正在初始化新模块...")

	services.LaserReceiver = laser_receiver.NewLaserReceiver(&cfg.LaserReceiver)
	services.LaserReceiver.SetChannels(alarmDataChan, leakDataChan)

	services.AlarmRouter = alarm_router.NewAlarmRouter(&cfg.AlarmRouter)
	services.AlarmRouter.SetChannels(alarmDataChan, alarmChan)
	smsService := services.NewSMSService(&cfg.SMS)
	services.AlarmRouter.SetSMSService(smsService)

	services.LeakLocator = leak_locator.NewLeakLocator(&cfg.LeakLocator)
	services.LeakLocator.SetChannels(leakDataChan, leakChan)

	services.EmergencyController = emergency_controller.NewEmergencyController(&cfg.EmergencyController)
	services.EmergencyController.SetChannels(alarmChan, leakChan)

	services.LaserReceiver.Start()
	log.Println("[LaserReceiver] 启动成功")

	services.AlarmRouter.Start()
	log.Println("[AlarmRouter] 启动成功")

	if err := services.LeakLocator.Start(); err != nil {
		log.Printf("[LeakLocator] 启动警告: %v", err)
	} else {
		log.Println("[LeakLocator] 启动成功")
	}

	services.EmergencyController.Start()
	log.Println("[EmergencyController] 启动成功")

	log.Println("所有新模块初始化完成")

	log.Println("正在初始化告警引擎(旧版)...")
	services.InitAlarmEngine(&cfg.Alarm, &cfg.SMS)
	log.Println("告警引擎初始化成功")

	log.Println("正在启动API服务器...")
	apiServer := api.NewAPIServer(cfg, dbService, mqttService, services.WSService)
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Fatalf("API服务器启动失败: %v", err)
		}
	}()
	log.Printf("API服务器启动成功，监听端口: %d", cfg.Server.Port)

	log.Println("正在启动MQTT订阅...")
	if err := mqttService.Subscribe(); err != nil {
		log.Fatalf("MQTT订阅失败: %v", err)
	}
	log.Println("MQTT订阅成功")

	log.Println("=== 系统启动完成 ===")
	log.Println("  +----------------+      +-------------------+      +-----------------------+")
	log.Println("  | LaserReceiver  |----->|   AlarmRouter     |----->| EmergencyController   |")
	log.Println("  |  数据采集校验  |      |   分级告警推送     |      |  阀门/排风机联动控制   |")
	log.Println("  +----------------+      +-------------------+      +-----------------------+")
	log.Println("           |                                             ^")
	log.Println("           |               +-------------------+         |")
	log.Println("           +-------------->|   LeakLocator     |---------+")
	log.Println("                           |  泄漏源定位/扩散    |")
	log.Println("                           +-------------------+")
	log.Println("")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	log.Printf("收到信号: %v，正在优雅关闭...", sig)

	log.Println("正在停止新模块...")
	services.EmergencyController.Stop()
	services.LeakLocator.Stop()
	services.AlarmRouter.Stop()
	services.LaserReceiver.Stop()
	log.Println("所有新模块已停止")

	close(alarmDataChan)
	close(leakDataChan)
	close(alarmChan)
	close(leakChan)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.MQTT.CommandTimeoutSec)*time.Second)
	defer cancel()
	_ = ctx

	log.Println("=== 系统已关闭 ===")
}
