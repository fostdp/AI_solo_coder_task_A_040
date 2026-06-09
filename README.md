# 智慧城市地下综合管廊燃气泄漏激光监测与联动处置系统

## 项目概述

本系统是一套完整的智慧城市地下综合管廊燃气泄漏监测与应急联动处置全栈应用。针对某市30公里地下管廊燃气舱，实现300台激光甲烷检测器的实时数据采集、泄漏源智能定位、三级告警联动和可视化展示。

## 系统架构

```
┌─────────────────┐     MQTT      ┌─────────────────┐     HTTP/WS     ┌─────────────────┐
│  激光检测器     │ ────────────> │   Go后端服务    │ ─────────────> │   前端可视化    │
│  (300台)        │               │  (数据处理)     │                │  (Leaflet地图)  │
└─────────────────┘               └────────┬────────┘                └─────────────────┘
                                            │
                                            ▼
                        ┌──────────────────────────────────┐
                        │  数据存储层                      │
                        │  - InfluxDB (时序传感器数据)    │
                        │  - PostgreSQL (设备/告警数据)   │
                        └──────────────────────────────────┘
```

## 技术栈

### 后端
- **语言**: Go 1.21
- **Web框架**: Gin v1.9.1
- **数据库**: 
  - InfluxDB 2.x (时序数据存储)
  - PostgreSQL 15+PostGIS (关系型数据)
- **消息协议**: MQTT (paho.mqtt.golang)
- **实时推送**: WebSocket (gorilla/websocket)
- **配置管理**: Viper

### 前端
- **地图框架**: Leaflet 1.9.4
- **图表绘制**: Canvas 2D
- **样式**: CSS3 (深色主题)
- **通信**: RESTful API + WebSocket

### 核心算法
- **泄漏源定位**: 粒子群优化算法(PSO) + 高斯羽流扩散模型
- **备选算法**: 贝叶斯推断

## 功能特性

### 1. 数据采集与存储
- 300台激光甲烷检测器，每秒上报浓度数据
- 50台环境传感器（氧气、温湿度）数据采集
- MQTT协议异步接收，批量写入InfluxDB
- PostgreSQL存储设备配置、告警记录、联动日志

### 2. 泄漏源定位模型
- **高斯羽流模型**: 模拟燃气在管廊中的扩散规律
- **粒子群优化(PSO)**: 
  - 50个粒子，100次迭代
  - 并行计算，1秒内完成定位
  - 输出泄漏源位置、泄漏速率、置信度
- 地图上用红圈标注扩散范围

### 3. 三级告警机制
| 告警级别 | 浓度阈值 | 联动动作 |
|---------|---------|---------|
| 一级预警 | >10%LEL | 短信/MQTT通知 |
| 二级报警 | >20%LEL | 关闭防火分区阀门、启动排风机 |
| 三级紧急 | >50%LEL | 紧急关断、推送疏散通知 |

### 4. 应急联动控制
- 自动识别告警所在防火分区
- 模拟MQTT指令关闭阀门、启动排风机
- 5分钟冷却机制，避免重复触发
- SMS短信+MQTT双通道告警推送

### 5. 前端可视化
- Leaflet地图展示30公里管廊走向
- Canvas热力图叠加显示浓度分布
- 检测器圆点标记，颜色随浓度动态变化
- 点击检测器弹出详情面板：
  - 近1小时浓度趋势图（Canvas绘制）
  - 传感器健康状态
  - 工作温度、电压、信号强度等参数
- 实时告警列表和通知弹窗
- 泄漏源扩散范围红圈标注

## 目录结构

```
AI_solo_coder_task_A_040/
├── backend/                    # Go后端代码
│   ├── config/                 # 配置模块
│   │   ├── config.yaml         # 系统配置文件
│   │   └── config.go           # 配置加载
│   ├── models/                 # 数据模型
│   │   └── models.go           # 结构体定义
│   ├── services/               # 业务服务
│   │   ├── database.go         # 数据库服务
│   │   ├── alarm.go            # 告警引擎
│   │   ├── emergency.go        # 应急联动
│   │   ├── leak_detector.go    # 泄漏检测
│   │   └── websocket.go        # WebSocket服务
│   ├── algorithms/             # 算法模块
│   │   └── leak_detection.go   # PSO泄漏源定位
│   ├── mqtt/                   # MQTT客户端
│   │   └── client.go           # MQTT连接与消息处理
│   ├── api/                    # HTTP API
│   │   ├── handlers.go         # API处理器
│   │   └── router.go           # 路由配置
│   └── main.go                 # 主程序入口
├── simulator/                  # 检测器模拟器
│   ├── main.go                 # 模拟器主程序
│   └── go.mod                  # 模拟器依赖
├── frontend/                   # 前端代码
│   ├── index.html              # 主页面
│   ├── css/
│   │   └── style.css           # 样式文件
│   └── js/
│       ├── config.js           # 前端配置
│       ├── map.js              # 地图交互
│       ├── heatmap.js          # 热力图渲染
│       ├── chart.js            # 图表绘制
│       ├── websocket.js        # WebSocket客户端
│       └── app.js              # 主应用逻辑
├── database/                   # 数据库脚本
│   ├── postgresql_init.sql     # PostgreSQL初始化
│   └── influxdb_init.md        # InfluxDB初始化说明
├── go.mod                      # 后端依赖
└── README.md                   # 项目说明
```

## 快速开始

### 1. 环境要求

- Go 1.21+
- PostgreSQL 15+PostGIS
- InfluxDB 2.x
- Mosquitto (MQTT Broker)
- Node.js (可选，用于前端静态服务)

### 2. 数据库初始化

#### PostgreSQL
```bash
# 创建数据库
createdb gas_monitoring

# 执行初始化脚本
psql -d gas_monitoring -f database/postgresql_init.sql
```

#### InfluxDB
```bash
# 启动InfluxDB
influxd

# 创建组织和存储桶
influx org create -n smart_city
influx bucket create -n gas_data -o smart_city -r 30d
```

### 3. 配置修改

编辑 `backend/config/config.yaml`:
```yaml
database:
  postgresql:
    host: localhost
    port: 5432
    user: postgres
    password: your_password
    dbname: gas_monitoring
  
  influxdb:
    url: http://localhost:8086
    token: your_token
    org: smart_city
    bucket: gas_data

mqtt:
  broker: tcp://localhost:1883
  client_id: gas_monitoring_server

alarm:
  level1_threshold: 10.0
  level2_threshold: 20.0
  level3_threshold: 50.0
```

### 4. 启动后端服务

```bash
# 安装依赖
go mod download

# 构建
go build -o gas-monitor main.go

# 运行
./gas-monitor -config backend/config/config.yaml
```

服务将在 `http://localhost:8080` 启动。

### 5. 启动检测器模拟器

```bash
cd simulator
go mod download
go build -o simulator main.go

# 正常模式（无泄漏）
./simulator

# 模拟泄漏模式（在15000米处，泄漏速率0.05L/s）
./simulator -leak -leak-pos 15000 -leak-rate 0.05 -wind-speed 2.0 -wind-dir 1
```

### 6. 前端访问

后端已集成静态文件服务，直接访问：
```
http://localhost:8080
```

## API接口文档

### 检测器管理
- `GET /api/detectors` - 获取所有检测器列表
- `GET /api/detectors/:id` - 获取单个检测器信息
- `GET /api/detectors/:id/history` - 获取历史数据
  - Query参数: `hours` (默认1)
- `GET /api/detectors/:id/health` - 获取健康状态

### 告警管理
- `GET /api/alarms/active` - 获取活动告警
- `GET /api/alarms/history` - 获取历史告警
- `POST /api/alarms/:id/acknowledge` - 确认告警

### 泄漏源
- `GET /api/leaks/active` - 获取活动泄漏源
- `GET /api/leaks/history` - 获取历史泄漏源

### 设备控制
- `POST /api/valves/:id/close` - 关闭阀门
- `POST /api/valves/:id/open` - 开启阀门
- `POST /api/fans/:id/start` - 启动排风机
- `POST /api/fans/:id/stop` - 停止排风机
- `POST /api/zones/:id/reset` - 重置防火分区

### 统计数据
- `GET /api/statistics` - 获取系统统计数据

### 管廊信息
- `GET /api/pipe-corridor` - 获取管廊路径数据

### WebSocket
- `WS /api/ws` - 实时数据推送
  - 消息类型: `concentration`, `alarm`, `leak_source`, `status`

## 泄漏源定位算法说明

### 高斯羽流模型
```
C(x,y,z) = (Q / (2πuσyσz)) * exp(-y²/(2σy²)) * exp(-(z-H)²/(2σz²))
```
其中:
- Q: 泄漏速率
- u: 风速
- σy, σz: 扩散系数
- H: 泄漏源高度

### 粒子群优化(PSO)
- 搜索空间: 位置(0-30000m) × 泄漏速率(0-1L/s)
- 适应度函数: 计算值与实测值的均方误差(MSE)
- 参数设置:
  - 粒子数: 50
  - 迭代次数: 100
  - 惯性权重: 0.7
  - 认知因子: 1.5
  - 社会因子: 1.5

## 告警联动流程

```
浓度数据上报
    ↓
检查浓度阈值
    ├─ <10%LEL → 正常，更新状态
    ├─ >10%LEL → 一级预警 → MQTT+短信通知
    ├─ >20%LEL → 二级报警 → 关闭阀门 + 启动排风机
    └─ >50%LEL → 三级紧急 → 紧急关断 + 疏散通知
                    ↓
              推送WebSocket消息
                    ↓
              前端实时更新显示
```

## 性能指标

- 数据处理能力: 350条/秒 (300台检测器 + 50台环境传感器)
- 泄漏源定位延迟: <1秒
- 告警响应时间: <100ms
- 前端数据刷新: 1秒
- 历史数据存储: 30天(原始) + 1年(降采样)

## 安全设计

1. **MQTT认证**: 用户名密码认证 + TLS加密
2. **API鉴权**: API Key + 请求签名
3. **操作日志**: 所有控制操作记录审计日志
4. **指令验证**: 控制指令包含设备ID和时间戳校验
5. **冷却机制**: 5分钟内同一设备不重复触发控制

## 测试场景

### 场景1: 正常运行
```bash
./simulator
```
- 所有检测器显示正常浓度(<5%LEL)
- 无告警
- 热力图显示正常

### 场景2: 小型泄漏
```bash
./simulator -leak -leak-pos 10000 -leak-rate 0.01 -wind-speed 1.0
```
- 附近检测器浓度上升至10-20%LEL
- 触发一级预警
- 泄漏源定位精度: ±50米

### 场景3: 中型泄漏
```bash
./simulator -leak -leak-pos 15000 -leak-rate 0.05 -wind-speed 2.0
```
- 浓度超过20%LEL，触发二级报警
- 自动关闭对应防火分区阀门
- 启动排风机
- 地图显示泄漏扩散范围

### 场景4: 大型泄漏
```bash
./simulator -leak -leak-pos 20000 -leak-rate 0.2 -wind-speed 3.0
```
- 浓度超过50%LEL，触发三级紧急
- 紧急关断+疏散通知
- 告警音效提醒
- 扩散范围红圈动态扩大

## 常见问题

### Q: 前端无法连接WebSocket?
A: 检查防火墙配置，确保8080端口开放，检查nginx反向代理配置是否支持WebSocket升级。

### Q: 泄漏源定位不准确?
A: 检查风速风向参数是否正确配置，PSO算法参数可在config.yaml中调整。

### Q: 数据库写入性能不足?
A: 增加InfluxDB批量写入大小，默认5000条/批次，可调整为10000。

### Q: 如何扩展更多检测器?
A: 修改postgresql_init.sql中的INSERT语句，或通过API接口动态添加。

## 许可证

MIT License

## 联系方式

技术支持: support@smartcity-gas.com
