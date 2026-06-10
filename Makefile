.PHONY: help build up down logs restart ps clean build-backend build-simulator

help:
	@echo "智慧城市地下综合管廊燃气泄漏监测系统"
	@echo "========================================="
	@echo ""
	@echo "可用命令:"
	@echo "  make up              启动所有服务（不含模拟器）"
	@echo "  make up-all          启动所有服务（含模拟器）"
	@echo "  make down            停止并移除所有服务"
	@echo "  make restart         重启所有服务"
	@echo "  make ps              查看服务状态"
	@echo "  make logs            查看所有服务日志"
	@echo "  make logs-backend    查看后端日志"
	@echo "  make logs-simulator  查看模拟器日志"
	@echo "  make logs-mqtt       查看MQTT Broker日志"
	@echo "  make build           构建所有镜像"
	@echo "  make build-backend   构建后端镜像"
	@echo "  make build-simulator 构建模拟器镜像"
	@echo "  make clean           清理所有资源（含数据卷）"
	@echo ""
	@echo "模拟器控制:"
	@echo "  make simulator-start 启动模拟器"
	@echo "  make simulator-stop  停止模拟器"
	@echo "  make leak-add POS=15000 RATE=2.0  添加泄漏源"
	@echo "  make wind-set SPEED=3.0 DIR=180   设置风速风向"
	@echo "  make simulator-reset 重置模拟器"
	@echo ""

build: build-backend build-simulator

build-backend:
	docker compose build backend

build-simulator:
	docker compose build simulator

up:
	docker compose up -d

up-all:
	docker compose --profile simulator up -d

down:
	docker compose down

restart:
	docker compose restart

ps:
	docker compose ps

logs:
	docker compose logs -f --tail=100

logs-backend:
	docker compose logs -f --tail=100 backend

logs-simulator:
	docker compose logs -f --tail=100 simulator

logs-mqtt:
	docker compose logs -f --tail=100 mosquitto

clean:
	docker compose down -v
	@echo "已清理所有容器和数据卷"

simulator-start:
	docker compose --profile simulator up -d simulator

simulator-stop:
	docker compose stop simulator
	docker compose rm -f simulator

leak-add:
	@if [ -z "$(POS)" ]; then echo "请指定位置: make leak-add POS=15000 [RATE=2.0]"; exit 1; fi
	@RATE_VAL=$${RATE:-1.0}; \
	curl -s -X POST http://localhost:8081/api/leaks/add \
		-H "Content-Type: application/json" \
		-d "{\"position\": $(POS), \"rate\": $$RATE_VAL}" | python3 -m json.tool

leak-remove:
	@if [ -z "$(ID)" ]; then echo "请指定泄漏源ID: make leak-remove ID=leak-xxx"; exit 1; fi
	curl -s -X POST http://localhost:8081/api/leaks/remove \
		-H "Content-Type: application/json" \
		-d '{"id": "$(ID)"}' | python3 -m json.tool

leak-toggle:
	@if [ -z "$(ID)" ]; then echo "请指定泄漏源ID: make leak-toggle ID=leak-xxx"; exit 1; fi
	curl -s -X POST http://localhost:8081/api/leaks/toggle \
		-H "Content-Type: application/json" \
		-d '{"id": "$(ID)"}' | python3 -m json.tool

leak-list:
	curl -s http://localhost:8081/api/leaks | python3 -m json.tool

wind-set:
	@if [ -z "$(SPEED)" ] && [ -z "$(DIR)" ]; then echo "请指定风速或风向: make wind-set SPEED=3.0 [DIR=180]"; exit 1; fi
	@DATA="{}"; \
	if [ -n "$(SPEED)" ]; then DATA="{\"wind_speed\": $(SPEED)"; fi; \
	if [ -n "$(DIR)" ]; then \
		if [ -n "$(SPEED)" ]; then DATA="$${DATA}, \"wind_dir\": $(DIR)}"; else DATA="{\"wind_dir\": $(DIR)}"; fi; \
	else DATA="$${DATA}}"; fi; \
	curl -s -X POST http://localhost:8081/api/wind \
		-H "Content-Type: application/json" \
		-d "$$DATA" | python3 -m json.tool

wind-get:
	curl -s http://localhost:8081/api/wind | python3 -m json.tool

simulator-reset:
	curl -s -X POST http://localhost:8081/api/reset | python3 -m json.tool

simulator-status:
	curl -s http://localhost:8081/api/health | python3 -m json.tool

db-purge-alarms:
	docker exec gas-monitor-postgres psql -U postgres -d gas_monitoring \
		-c "DELETE FROM alarms; DELETE FROM leak_sources;"
	@echo "已清除所有告警和泄漏源记录"

db-backup:
	docker exec gas-monitor-postgres pg_dump -U postgres -d gas_monitoring > backup_$$(date +%Y%m%d_%H%M%S).sql
	@echo "数据库已备份到 backup_*.sql"

prometheus-reload:
	curl -s -X POST http://localhost:9090/-/reload
	@echo "Prometheus配置已重载"
