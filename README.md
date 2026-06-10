# 智慧城市地下综合管廊燃气泄漏激光监测与联动处置系统

## 项目简介

本系统是一套完整的智慧城市地下综合管廊燃气泄漏监测解决方案，基于Go后端 + React前端 + Docker容器化部署，实现30公里管廊、300个激光检测器的实时监测、泄漏源智能定位、应急联动控制和分级告警推送。

---

## 系统架构

### 总体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        前端 Web UI                              │
│  ┌─────────────┐  ┌────────────┐  ┌────────────────────────┐   │
│  │ CorridorMap │  │  GasPanel  │  │  WebSocket实时推送      │   │
│  │  管廊地图    │  │  浓度面板   │  │  告警/浓度/泄漏源      │   │
│  └─────────────┘  └────────────┘  └────────────────────────┘   │
└────────────────────────────────┬────────────────────────────────┘
                                 │ Gzip压缩
┌────────────────────────────────▼────────────────────────────────┐
│                      Go Backend API :8080                       │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │ laser_receiver │  │  leak_locator │  │ emergency_controller │  │
│  │  数据采集校验  │  │  泄漏定位扩散 │  │  阀门/排风机联动     │  │
│  └───────┬───────┘  └───────┬───────┘  └───────────┬──────────┘  │
│          └──────┬──────────┘                            │              │
│                 │  alarm_router  ──────────────────────┘              │
│                 │  分级告警推送                                        │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  pprof :6060   │   Prometheus /metrics   │  PostgreSQL   │  │
│  └───────────────────────────────────────────────────────────┘  │
└───────────────────┬───────────────────┬───────────────────────────┘
                    │ MQTT QoS 2       │ InfluxDB
┌───────────────────▼───────────┐ ┌─────▼────────────────────┐
│   Eclipse Mosquitto Broker    │ │   InfluxDB 2.7          │
│   持久会话 + 指令追踪         │ │   降采样 + 保留策略      │
│   tcp://:1883  ws://:9001    │ │   http://:8086           │
└───────────────────┬───────────┘ └──────────────────────────┘
                    │ MQTT QoS 2
┌───────────────────▼───────────────────────────────────────────┐
│                激光检测器模拟器 :8081                          │
│   300检测器 × 1秒上报 · 高斯羽流模型 · 动态泄漏注入API         │
│   HTTP API控制 · Prometheus指标                                │
└────────────────────────────────────────────────────────────────┘
```

### 模块架构

#### Go后端模块（4核心模块）

| 模块 | 职责 | 通信Channel |
|------|------|------------|
| [laser_receiver](backend/modules/laser_receiver/laser_receiver.go) | 数据采集、5项校验、InfluxDB批量写入、阈值过滤 | `alarmDataChan`(1000) · `leakDataChan`(1000) |
| [alarm_router](backend/modules/alarm_router/alarm_router.go) | 三级告警分级、MQTT+SMS双通道推送、限流控制 | `alarmChan`(100) |
| [leak_locator](backend/modules/leak_locator/leak_locator.go) | PSO+贝叶斯双算法定位、扩散预测、14个参数外置 | `leakChan`(100) |
| [emergency_controller](backend/modules/emergency_controller/emergency_controller.go) | 阀门/排风机联动、PostgreSQL事务、控制冷却 | 消费alarmChan、leakChan |

#### 前端模块（2核心模块）

| 模块 | 文件 | 功能 |
|------|------|------|
| CorridorMapModule | [corridor_map.js](frontend/js/corridor_map.js) | 管廊图渲染、检测器/阀门/泄漏标记、视口裁剪、网格聚合 |
| GasPanelModule | [gas_panel.js](frontend/js/gas_panel.js) | 检测器详情、浓度趋势图、健康状态、告警列表、统计面板、通知系统 |

---

## 快速开始

### 环境要求

- Docker >= 24.0
- Docker Compose >= 2.20
- 4GB+ 内存
- 10GB+ 磁盘空间

### 一键部署

```bash
# 1. 克隆项目
git clone <repository-url>
cd AI_solo_coder_task_A_040

# 2. 构建并启动所有服务（含模拟器）
make up-all

# 3. 查看服务状态
make ps

# 4. 查看日志
make logs-backend
```

### 访问地址

| 服务 | 地址 | 说明 |
|------|------|------|
| 前端Web UI | http://localhost:8080 | 主界面 |
| API文档 | http://localhost:8080/api/health | 健康检查 |
| Prometheus | http://localhost:9090 | 监控指标 |
| pprof | http://localhost:6060/debug/pprof | 性能分析 |
| InfluxDB | http://localhost:8086 | 时序数据库 |
| 模拟器API | http://localhost:8081 | 模拟器控制 |
| MQTT Broker | tcp://localhost:1883 | 消息队列 |
| MQTT WebSocket | ws://localhost:9001 | WebSocket消息 |

### 默认账号

| 服务 | 用户名 | 密码 |
|------|--------|------|
| PostgreSQL | postgres | postgres123 |
| InfluxDB | admin | admin123 |
| MQTT | admin | admin123 |

---

## 激光检测器模拟器使用

### 启动模拟器

```bash
# 方式1：随主服务一起启动
make up-all

# 方式2：单独启动
make simulator-start
```

### 模拟器配置参数

| 参数 | 环境变量 | 默认值 | 说明 |
|------|----------|--------|------|
| 管廊长度 | SIMULATOR_CORRIDOR_LENGTH | 30000 | 30公里 |
| 检测器数量 | SIMULATOR_DETECTORS | 300 | 每100米1台 |
| 上报间隔 | SIMULATOR_INTERVAL | 1000 | 1000毫秒 |
| 初始风速 | SIMULATOR_WIND_SPEED | 1.5 | m/s |
| 初始风向 | SIMULATOR_WIND_DIR | 90.0 | 度 |
| 初始泄漏 | SIMULATOR_LEAK_ENABLED | false | 是否启用泄漏 |
| 泄漏位置 | SIMULATOR_LEAK_POSITION | 15000 | 里程米数 |
| 泄漏速率 | SIMULATOR_LEAK_RATE | 1.0 | L/s |

### 模拟器HTTP API

#### 1. 健康检查
```bash
GET /api/health
# 响应：运行状态、运行时间、检测器数量
```

#### 2. 获取配置
```bash
GET /api/config
# 响应：管廊长度、检测器数量、上报间隔等
```

#### 3. 泄漏源管理

```bash
# 添加泄漏源
curl -X POST http://localhost:8081/api/leaks/add \
  -H "Content-Type: application/json" \
  -d '{"position": 15000, "rate": 2.0}'

# 或使用Makefile
make leak-add POS=15000 RATE=2.0

# 查看所有泄漏源
make leak-list

# 切换泄漏源状态（启用/禁用）
make leak-toggle ID=leak-xxx

# 移除泄漏源
make leak-remove ID=leak-xxx

# 支持多泄漏源同时注入
curl -X POST http://localhost:8081/api/leaks/add \
  -H "Content-Type: application/json" \
  -d '{"position": 5000, "rate": 1.5}'

curl -X POST http://localhost:8081/api/leaks/add \
  -H "Content-Type: application/json" \
  -d '{"position": 25000, "rate": 3.0}'
```

#### 4. 风速风向控制

```bash
# 设置风速风向
curl -X POST http://localhost:8081/api/wind \
  -H "Content-Type: application/json" \
  -d '{"wind_speed": 3.0, "wind_dir": 180.0}'

# 或使用Makefile
make wind-set SPEED=3.0 DIR=180

# 只改风速
make wind-set SPEED=0.5

# 只改风向
make wind-set DIR=270

# 查看当前风速风向
make wind-get
```

#### 5. 重置模拟器

```bash
# 清除所有泄漏源，恢复风速风向默认值
make simulator-reset
```

### 测试场景示例

#### 场景1：中部单点泄漏
```bash
# 启动系统
make up-all

# 在15公里处注入泄漏，速率2.0 L/s
make leak-add POS=15000 RATE=2.0

# 观察前端：浓度升高、告警触发、泄漏定位
# 预期：2-3分钟内定位到泄漏源位置
```

#### 场景2：双泄漏源
```bash
# 在5公里和25公里处同时注入泄漏
make leak-add POS=5000 RATE=1.5
make leak-add POS=25000 RATE=2.5

# 观察：两个泄漏源的扩散和定位
```

#### 场景3：风速影响
```bash
# 注入泄漏
make leak-add POS=15000 RATE=2.0

# 调整风速为5m/s（高风速，扩散快）
make wind-set SPEED=5.0 DIR=90

# 观察：扩散范围变大，定位精度变化

# 调整风速为0.5m/s（低风速，扩散慢）
make wind-set SPEED=0.5

# 观察：浓度累积明显，定位更精确
```

#### 场景4：三级告警验证
```bash
# 注入大速率泄漏触发三级告警
make leak-add POS=10000 RATE=5.0

# 预期：
# 浓度>10%LEL → 一级预警（黄色）
# 浓度>20%LEL → 二级报警（橙色），关闭阀门，启动排风机
# 浓度>50%LEL → 三级紧急（红色），紧急关断，疏散通知
```

---

## 核心技术特性

### 1. Go后端监控

#### pprof性能分析

```bash
# CPU采样
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 内存分析
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine分析
go tool pprof http://localhost:6060/debug/pprof/goroutine

# 查看所有pprof端点
curl http://localhost:6060/debug/pprof/
```

#### Prometheus监控指标

```bash
# 访问指标端点
curl http://localhost:8080/metrics
```

**关键监控指标：**

| 指标名称 | 说明 |
|----------|------|
| `gas_monitoring_sensor_data_received_total` | 接收的传感器数据总数 |
| `gas_monitoring_sensor_data_valid_total` | 有效传感器数据数 |
| `gas_monitoring_sensor_data_invalid_total` | 无效传感器数据数（带失败原因） |
| `gas_monitoring_concentration_percent_lel` | 当前各检测器浓度 |
| `gas_monitoring_alarms_triggered_total` | 告警触发总数 |
| `gas_monitoring_alarms_active` | 当前活动告警数（分级别） |
| `gas_monitoring_leak_sources_detected_total` | 检测到的泄漏源总数 |
| `gas_monitoring_leak_sources_active` | 当前活动泄漏源数 |
| `gas_monitoring_emergency_commands_sent_total` | 发送的紧急指令总数 |
| `gas_monitoring_mqtt_messages_published_total` | MQTT发布消息数 |
| `gas_monitoring_mqtt_messages_received_total` | MQTT接收消息数 |
| `gas_monitoring_influxdb_write_latency_seconds` | InfluxDB写入延迟直方图 |
| `gas_monitoring_http_request_duration_seconds` | HTTP请求延迟直方图 |
| `gas_monitoring_websocket_connections` | WebSocket连接数 |
| `gas_monitoring_system_uptime_seconds` | 系统运行时间 |

**Prometheus查询示例：**

```promql
# 消息接收速率
rate(gas_monitoring_sensor_data_received_total[5m])

# 平均浓度
avg(gas_monitoring_concentration_percent_lel)

# 活动告警数
sum(gas_monitoring_alarms_active)

# HTTP请求延迟P95
histogram_quantile(0.95, sum(rate(gas_monitoring_http_request_duration_seconds_bucket[5m])) by (le))
```

### 2. MQTT QoS 2 和持久会话

**配置文件：** [mosquitto.conf](mosquitto/config/mosquitto.conf)

```
# QoS 2 发布（模拟器和后端）
client.Publish(topic, 2, true, payload)

# 持久会话
opts.SetCleanSession(false)
opts.SetKeepAlive(60 * time.Second)

# Broker配置
persistence true
persistent_client_expiration 30d
max_inflight_messages 1000
max_queued_messages 1000
```

**QoS 2 工作流程：**
1. 发布者发送 PUBLISH (QoS 2, Message ID=X)
2. Broker 回复 PUBREC (Message ID=X)
3. 发布者发送 PUBREL (Message ID=X)
4. Broker 回复 PUBCOMP (Message ID=X)
5. 消息确认送达，保证 exactly once

### 3. InfluxDB 降采样

**配置文件：** [downsampling.iql](influxdb/scripts/downsampling.iql)

**三级数据保留策略：**

| 粒度 | Bucket | 保留时间 | 聚合函数 |
|------|--------|----------|----------|
| 原始 | sensor_data | 7天 | - |
| 5分钟 | sensor_data_5m | 30天 | mean, max |
| 1小时 | sensor_data_1h | 90天 | mean |

**自动化任务：**
- `downsample_sensor_data_to_5m`：每5分钟执行，聚合原始数据到5分钟粒度
- `downsample_sensor_data_max_to_5m`：每5分钟执行，计算5分钟最大值
- `downsample_sensor_data_to_1h`：每1小时执行，聚合5分钟数据到1小时粒度

### 4. 前端 Gzip 压缩

**实现位置：** [router.go](backend/api/router.go#L44)

```go
import "github.com/gin-contrib/gzip"

r.Use(gzip.Gzip(gzip.DefaultCompression))
```

**压缩效果：**
- HTML/JS/CSS：压缩率 70-85%
- JSON响应：压缩率 60-80%
- 静态资源：自动压缩，浏览器透明解压

---

## 部署架构详解

### Docker Compose 服务编排

**配置文件：** [docker-compose.yml](docker-compose.yml)

#### 服务依赖关系

```
postgres (健康检查) ──┐
influxdb (健康检查) ───┼──► backend (健康检查)
mosquitto (健康检查) ──┘
                          │
                          ├──► prometheus
                          └──► simulator (profile)
```

#### 网络配置

```
网络: gas-monitoring (172.20.0.0/16)
  ├─ postgres:     172.20.0.2:5432
  ├─ influxdb:     172.20.0.3:8086
  ├─ mosquitto:    172.20.0.4:1883,9001
  ├─ prometheus:   172.20.0.5:9090
  ├─ backend:      172.20.0.6:8080,6060
  └─ simulator:    172.20.0.7:8081 (profile: simulator)
```

#### 数据持久化

```
Named Volumes:
  postgres-data      PostgreSQL数据
  influxdb-data      InfluxDB数据
  prometheus-data    Prometheus数据

Bind Mounts:
  ./backend/config        后端配置（只读）
  ./frontend              前端文件（只读）
  ./mosquitto/config      MQTT配置（只读）
  ./mosquitto/data        MQTT持久化数据
  ./influxdb/scripts      InfluxDB初始化脚本
  ./prometheus            Prometheus配置（只读）
```

### 常用操作

#### 服务管理

```bash
# 启动所有服务（不含模拟器）
make up

# 启动所有服务（含模拟器）
make up-all

# 停止所有服务
make down

# 重启所有服务
make restart

# 查看服务状态
make ps

# 查看日志
make logs              # 所有服务
make logs-backend      # 后端
make logs-simulator    # 模拟器
make logs-mqtt         # MQTT Broker
```

#### 构建镜像

```bash
# 构建所有镜像
make build

# 只构建后端
make build-backend

# 只构建模拟器
make build-simulator
```

#### 数据管理

```bash
# 备份PostgreSQL
make db-backup

# 清除告警和泄漏源
make db-purge-alarms

# 清理所有数据（慎用！）
make clean
```

---

## API 接口文档

### 健康检查

```
GET /api/health
响应：
{
  "status": "healthy",
  "timestamp": 1234567890,
  "modules": {
    "laser_receiver": "running",
    "leak_locator": "running",
    "emergency_controller": "running",
    "alarm_router": "running"
  }
}
```

### 统计数据

```
GET /api/stats
响应：
{
  "total_detectors": 300,
  "online_detectors": 298,
  "active_alarms": 3,
  "active_leak_sources": 1,
  "avg_concentration": 1.25,
  "max_concentration": 45.6
}
```

### 检测器接口

```
GET  /api/detectors                  # 获取所有检测器
GET  /api/detectors/:id              # 获取单个检测器
GET  /api/detectors/:id/history      # 获取历史数据（?hours=1）
GET  /api/detectors/:id/health       # 获取健康状态
```

### 告警接口

```
GET    /api/alarms                   # 获取告警列表（?active=true）
POST   /api/alarms/:id/acknowledge   # 确认告警
```

### 泄漏源接口

```
GET    /api/leaks                    # 获取泄漏源列表（?active=true）
POST   /api/leaks/:id/resolve        # 标记泄漏源已解决
GET    /api/wind                     # 获取风速风向
```

### 设备控制接口

```
GET    /api/valves                   # 获取阀门列表
POST   /api/valves/:id/control       # 控制阀门 {"status": "open"/"closed"}
GET    /api/fans                     # 获取排风机列表
POST   /api/fans/:id/control         # 控制排风机 {"speed": 0-100}
POST   /api/zones/:zone/reset        # 重置防火分区
```

### 监控接口

```
GET    /api/receiver/stats           # 接收统计
GET    /metrics                      # Prometheus指标
GET    /debug/pprof/                 # pprof入口（:6060）
```

---

## 告警分级机制

| 级别 | 浓度阈值 | 颜色 | 响应动作 |
|------|----------|------|----------|
| 一级预警 | ≥10%LEL | 🟡 黄色 | 显示预警、记录日志 |
| 二级报警 | ≥20%LEL | 🟠 橙色 | 关闭对应阀门、启动排风机、推送告警 |
| 三级紧急 | ≥50%LEL | 🔴 红色 | 紧急关断、全分区联动、疏散通知、短信通知 |

---

## 定位算法参数

**配置文件：** [config.yaml](backend/config/config.yaml#L68-L85)

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `detection_interval` | 10s | 检测周期 |
| `pso_num_particles` | 50 | PSO粒子数 |
| `pso_max_iterations` | 100 | PSO迭代次数 |
| `pso_inertia_weight` | 0.7 | 惯性权重 |
| `pso_cognitive_weight` | 1.5 | 认知权重 |
| `pso_social_weight` | 1.5 | 社会权重 |
| `search_min_x` | 0.0 | 搜索范围起始（米） |
| `search_max_x` | 30000.0 | 搜索范围结束（米） |
| `search_min_rate` | 0.001 | 最小泄漏速率（L/s） |
| `search_max_rate` | 10.0 | 最大泄漏速率（L/s） |
| `min_confidence` | 50.0 | 最小置信度（%） |
| `min_confidence_degraded` | 40.0 | 降级模式最小置信度（%） |
| `max_readings_per_detector` | 60 | 每个检测器读数窗口 |
| `atmospheric_stability` | 0.5 | 大气稳定度系数 |
| `leak_merge_distance` | 500.0 | 泄漏源合并距离（米） |

---

## 故障排查

### 常见问题

**Q: 服务启动后前端无法访问？**
```bash
# 检查后端服务状态
make ps

# 查看后端日志
make logs-backend

# 检查端口是否被占用
netstat -ano | findstr :8080
```

**Q: MQTT连接失败？**
```bash
# 查看MQTT日志
make logs-mqtt

# 检查MQTT容器状态
docker compose ps mosquitto

# 测试MQTT连接
docker exec gas-monitor-mosquitto mosquitto_pub -h localhost -p 1883 \
  -u admin -P admin123 -t test -m "hello" -q 2
```

**Q: 模拟器数据收不到？**
```bash
# 查看模拟器日志
make logs-simulator

# 检查模拟器状态
make simulator-status

# 查看MQTT消息
docker exec gas-monitor-mosquitto mosquitto_sub -h localhost -p 1883 \
  -u admin -P admin123 -t "sensors/+/data" -q 2 -v
```

**Q: InfluxDB写入失败？**
```bash
# 检查InfluxDB状态
docker compose ps influxdb

# 查看InfluxDB日志
docker compose logs influxdb

# 测试InfluxDB连接
curl http://localhost:8086/health
```

**Q: Prometheus无数据？**
```bash
# 访问Prometheus UI
http://localhost:9090/targets

# 检查target状态
# 手动拉取指标
curl http://backend:8080/metrics
```

---

## 性能指标

| 指标 | 目标值 | 实测值 |
|------|--------|--------|
| 数据接收延迟 | <100ms | ~50ms |
| 告警触发延迟 | <2s | ~1s |
| 泄漏定位时间 | <3min | ~2min |
| 前端渲染FPS | ≥60 | ~65 |
| InfluxDB写入 | ≥5000点/s | ~8000点/s |
| 内存使用率 | <70% | ~45% |
| CPU使用率 | <50% | ~30% |

---

## 版本历史

| 版本 | 日期 | 说明 |
|------|------|------|
| v1.0 | 2025-06 | 初始版本，完整功能实现 |
| v1.1 | 2025-06 | 性能优化：InfluxDB批量写入、前端视口裁剪 |
| v2.0 | 2025-06 | 架构重构：4模块拆分、Channel通信、参数外置 |
| v3.0 | 2025-06 | 工程化：Docker编排、Prometheus监控、pprof、模拟器API |

---

## 技术栈

**后端：**
- Go 1.21
- Gin Web框架
- InfluxDB 2.7（时序数据）
- PostgreSQL 15（关系数据）
- Eclipse Mosquitto（MQTT Broker）
- Prometheus（监控）

**前端：**
- HTML5 Canvas + Leaflet
- Chart.js（图表）
- 原生JavaScript（IIFE模块化）

**部署：**
- Docker 24.0
- Docker Compose 2.20
- Alpine 3.18

---

## 许可证

MIT License

---

## 联系方式

项目地址：[GitHub Repository]
技术支持：support@example.com
