# InfluxDB 初始化说明

## 1. 安装并启动 InfluxDB 2.x

```bash
# Docker 方式
docker run -d \
  --name influxdb2 \
  -p 8086:8086 \
  -v influxdb2-data:/var/lib/influxdb2 \
  -v influxdb2-config:/etc/influxdb2 \
  influxdb:2.7
```

## 2. 初始化配置

访问 http://localhost:8086，首次登录时设置：
- 用户名: admin
- 密码: admin123456
- 组织名称: smart-city
- 初始存储桶: gas-data

## 3. 创建 Token

在 InfluxDB UI 中创建一个 All Access Token，然后复制到 config.yaml 中。

## 4. 使用命令行初始化（可选）

```bash
# 设置 CLI 配置
influx config create \
  --config-name gas-monitoring \
  --host-url http://localhost:8086 \
  --org smart-city \
  --token your-token-here \
  --active

# 创建存储桶（如尚未创建）
influx bucket create \
  --name gas-data \
  --org smart-city \
  --retention 720h

# 创建用于长时间存储的聚合存储桶
influx bucket create \
  --name gas-data-downsampled \
  --org smart-city \
  --retention 8760h
```

## 5. 数据保留策略

- **gas-data**: 原始数据，保留 30 天 (720h)
- **gas-data-downsampled**: 降采样数据，保留 1 年 (8760h)

## 6. 数据结构

### 6.1 激光甲烷检测器数据

**Measurement**: `laser_methane`

**Tags**:
- `device_id`: 设备ID (如 LASER-0001)
- `fire_zone`: 防火分区 (如 ZONE-01)
- `status`: 设备状态 (normal/fault/maintenance)

**Fields**:
- `concentration`: 甲烷浓度 (%LEL)
- `temperature`: 设备内部温度 (°C)
- `voltage`: 工作电压 (V)
- `signal_strength`: 信号强度 (%)

**Timestamp**: 数据采集时间

### 6.2 环境传感器数据

**Measurement**: `environment`

**Tags**:
- `device_id`: 设备ID
- `sensor_type`: 传感器类型 (oxygen/temp_humidity)
- `location_type`: 安装位置 (vent/valve)
- `fire_zone`: 防火分区

**Fields**:
- `temperature`: 环境温度 (°C)
- `humidity`: 相对湿度 (%)
- `oxygen`: 氧气浓度 (%)
- `wind_speed`: 风速 (m/s)
- `wind_direction`: 风向 (°)

### 6.3 告警数据

**Measurement**: `alarms`

**Tags**:
- `device_id`: 触发告警的设备ID
- `level`: 告警级别 (1/2/3)
- `level_name`: 告警级别名称
- `fire_zone`: 防火分区

**Fields**:
- `concentration`: 触发告警时的浓度
- `threshold`: 告警阈值
- `acknowledged`: 是否已确认 (0/1)
- `resolved`: 是否已解决 (0/1)

## 7. 降采样任务配置

创建任务将 1Hz 的原始数据降采样为 1 分钟平均值：

```flux
option task = {name: "gas_data_downsample", every: 1m}

data = from(bucket: "gas-data")
  |> range(start: -1m)
  |> filter(fn: (r) => r._measurement == "laser_methane")

data
  |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)
  |> to(bucket: "gas-data-downsampled", org: "smart-city")

env_data = from(bucket: "gas-data")
  |> range(start: -1m)
  |> filter(fn: (r) => r._measurement == "environment")

env_data
  |> aggregateWindow(every: 1m, fn: mean, createEmpty: false)
  |> to(bucket: "gas-data-downsampled", org: "smart-city")
```

## 8. 连续查询（InfluxDB 1.x 兼容方式）

如果使用 InfluxDB 1.x，可以使用以下连续查询：

```sql
CREATE CONTINUOUS QUERY cq_laser_1m ON gas_data
BEGIN
  SELECT mean(concentration) AS concentration,
         mean(temperature) AS temperature
  INTO gas_data_downsampled."default".laser_methane_1m
  FROM laser_methane
  GROUP BY time(1m), device_id, fire_zone
END;
```
