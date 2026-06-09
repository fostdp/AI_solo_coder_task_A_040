package services

import (
	"gas-monitoring-system/backend/modules/alarm_router"
	"gas-monitoring-system/backend/modules/emergency_controller"
	"gas-monitoring-system/backend/modules/laser_receiver"
	"gas-monitoring-system/backend/modules/leak_locator"
)

var (
	LaserReceiver      *laser_receiver.LaserReceiver
	LeakLocator        *leak_locator.LeakLocator
	EmergencyController *emergency_controller.EmergencyController
	AlarmRouter        *alarm_router.AlarmRouter
)
