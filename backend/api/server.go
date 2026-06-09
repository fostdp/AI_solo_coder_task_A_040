package api

import (
	"fmt"
	"log"

	"gas-monitoring-system/backend/config"
	"gas-monitoring-system/backend/mqtt"
	"gas-monitoring-system/backend/services"
)

type APIServer struct {
	cfg         *config.Config
	dbService   *services.DatabaseService
	mqttService *mqtt.MQTTService
	wsService   *services.WebSocketService
}

func NewAPIServer(cfg *config.Config, db *services.DatabaseService, mqtt *mqtt.MQTTService, ws *services.WebSocketService) *APIServer {
	return &APIServer{
		cfg:         cfg,
		dbService:   db,
		mqttService: mqtt,
		wsService:   ws,
	}
}

func (s *APIServer) Start() error {
	router := SetupRouter()

	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	log.Printf("API服务器监听地址: %s", addr)

	return router.Run(addr)
}
