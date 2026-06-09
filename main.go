package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gas-monitoring-system/backend/api"
	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/mqtt"
	"gas-monitoring-system/backend/services"
)

func main() {
	configPath := "backend/config/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := services.InitDatabase(cfg); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer services.DB.Close()

	services.InitAlarmEngine(&cfg.Alarm, &cfg.SMS)

	if err := mqtt.InitMQTT(&cfg.MQTT); err != nil {
		log.Fatalf("Failed to initialize MQTT: %v", err)
	}
	defer mqtt.MQTT.Close()

	if err := services.InitLeakDetector(&cfg.LeakDetection); err != nil {
		log.Fatalf("Failed to initialize leak detector: %v", err)
	}
	defer services.LeakDetector.Close()

	services.InitWebSocket()
	defer services.WSService.Close()

	r := api.SetupRouter()

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Server starting on %s", addr)

	go func() {
		if err := r.Run(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down gracefully...")
}
