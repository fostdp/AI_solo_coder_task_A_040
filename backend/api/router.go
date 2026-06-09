package api

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization"}
	r.Use(cors.New(config))

	h := NewHandler()

	api := r.Group("/api")
	{
		api.GET("/health", h.Health)
		api.GET("/stats", h.GetStatistics)
		api.GET("/wind", h.GetWindData)
		api.GET("/receiver/stats", h.GetReceiverStats)

		detectors := api.Group("/detectors")
		{
			detectors.GET("", h.GetDetectors)
			detectors.GET("/:id", h.GetDetector)
			detectors.GET("/:id/history", h.GetDetectorHistory)
			detectors.GET("/:id/health", h.GetDetectorHealth)
		}

		api.GET("/pipe-corridor", h.GetPipeCorridor)
		api.GET("/concentrations", h.GetCurrentConcentrations)

		alarms := api.Group("/alarms")
		{
			alarms.GET("", h.GetAlarms)
			alarms.POST("/:id/acknowledge", h.AcknowledgeAlarm)
		}

		leaks := api.Group("/leaks")
		{
			leaks.GET("", h.GetLeaks)
			leaks.POST("/:id/resolve", h.ResolveLeak)
		}

		valves := api.Group("/valves")
		{
			valves.GET("", h.GetValves)
			valves.POST("/:id/control", h.ControlValve)
		}

		fans := api.Group("/fans")
		{
			fans.GET("", h.GetFans)
			fans.POST("/:id/control", h.ControlFan)
		}

		api.POST("/zones/:zone/reset", h.ResetZone)

		api.GET("/ws", h.WebSocket)
	}

	r.Static("/static", "./frontend")
	r.StaticFile("/", "./frontend/index.html")

	return r
}
