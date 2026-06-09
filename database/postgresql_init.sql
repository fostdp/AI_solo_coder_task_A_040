-- 智慧城市地下综合管廊燃气泄漏监测系统
-- PostgreSQL 数据库初始化脚本

-- 创建数据库
CREATE DATABASE gas_monitoring
    WITH
    OWNER = postgres
    ENCODING = 'UTF8'
    LC_COLLATE = 'en_US.UTF-8'
    LC_CTYPE = 'en_US.UTF-8'
    TABLESPACE = pg_default
    CONNECTION LIMIT = -1;

\c gas_monitoring;

-- 创建扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS postgis_topology;

-- 检测器表
CREATE TABLE detectors (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    fire_zone VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'normal',
    health DOUBLE PRECISION DEFAULT 100.0,
    install_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_calib TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    geom GEOMETRY(Point, 4326),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_detectors_device_id ON detectors(device_id);
CREATE INDEX idx_detectors_fire_zone ON detectors(fire_zone);
CREATE INDEX idx_detectors_geom ON detectors USING GIST(geom);

-- 温湿度氧气传感器表
CREATE TABLE environment_sensors (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    sensor_type VARCHAR(20) NOT NULL,
    location_type VARCHAR(20) NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    fire_zone VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'normal',
    health DOUBLE PRECISION DEFAULT 100.0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 阀门表
CREATE TABLE valves (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    valve_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    fire_zone VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'open',
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    last_action TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 排风机表
CREATE TABLE fans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    fan_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    fire_zone VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'stopped',
    speed INTEGER DEFAULT 0,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 管廊路径表
CREATE TABLE pipe_corridor_path (
    id SERIAL PRIMARY KEY,
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    segment_id INTEGER,
    geom GEOMETRY(Point, 4326)
);

CREATE INDEX idx_pipe_corridor_path_position ON pipe_corridor_path(position);
CREATE INDEX idx_pipe_corridor_path_geom ON pipe_corridor_path USING GIST(geom);

-- 防火分区表
CREATE TABLE fire_zones (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    zone_id VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    start_position DOUBLE PRECISION NOT NULL,
    end_position DOUBLE PRECISION NOT NULL,
    start_latitude DOUBLE PRECISION NOT NULL,
    start_longitude DOUBLE PRECISION NOT NULL,
    end_latitude DOUBLE PRECISION NOT NULL,
    end_longitude DOUBLE PRECISION NOT NULL,
    status VARCHAR(20) DEFAULT 'normal',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 告警表
CREATE TABLE alarms (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id VARCHAR(50) NOT NULL,
    level INTEGER NOT NULL,
    level_name VARCHAR(20) NOT NULL,
    concentration DOUBLE PRECISION NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    message TEXT,
    timestamp TIMESTAMP NOT NULL,
    acknowledged BOOLEAN DEFAULT FALSE,
    acknowledged_at TIMESTAMP,
    acknowledged_by VARCHAR(50),
    resolved BOOLEAN DEFAULT FALSE,
    resolved_at TIMESTAMP,
    resolved_note TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_alarms_device_id ON alarms(device_id);
CREATE INDEX idx_alarms_level ON alarms(level);
CREATE INDEX idx_alarms_timestamp ON alarms(timestamp DESC);
CREATE INDEX idx_alarms_resolved ON alarms(resolved);

-- 阀门控制记录表
CREATE TABLE valve_control_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    valve_id VARCHAR(50) NOT NULL,
    action VARCHAR(20) NOT NULL,
    fire_zone VARCHAR(50),
    triggered_by VARCHAR(50) NOT NULL,
    reason TEXT,
    timestamp TIMESTAMP NOT NULL,
    success BOOLEAN DEFAULT TRUE,
    response TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_valve_control_valve_id ON valve_control_logs(valve_id);
CREATE INDEX idx_valve_control_timestamp ON valve_control_logs(timestamp DESC);

-- 风机控制记录表
CREATE TABLE fan_control_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    fan_id VARCHAR(50) NOT NULL,
    action VARCHAR(20) NOT NULL,
    speed INTEGER DEFAULT 0,
    fire_zone VARCHAR(50),
    triggered_by VARCHAR(50) NOT NULL,
    reason TEXT,
    timestamp TIMESTAMP NOT NULL,
    success BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 泄漏源表
CREATE TABLE leak_sources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    position DOUBLE PRECISION NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    leak_rate DOUBLE PRECISION NOT NULL,
    confidence DOUBLE PRECISION NOT NULL,
    diffusion_radius DOUBLE PRECISION NOT NULL,
    status VARCHAR(20) DEFAULT 'active',
    detected_at TIMESTAMP NOT NULL,
    resolved_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_leak_sources_status ON leak_sources(status);
CREATE INDEX idx_leak_sources_detected_at ON leak_sources(detected_at DESC);

-- 传感器健康状态表
CREATE TABLE sensor_health (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    device_id VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    health DOUBLE PRECISION NOT NULL,
    temperature DOUBLE PRECISION,
    voltage DOUBLE PRECISION,
    signal_strength DOUBLE PRECISION,
    last_update TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sensor_health_device_id ON sensor_health(device_id);
CREATE INDEX idx_sensor_health_last_update ON sensor_health(last_update DESC);

-- 短信发送记录表
CREATE TABLE sms_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    alarm_id UUID REFERENCES alarms(id),
    receiver VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    sent_at TIMESTAMP NOT NULL,
    success BOOLEAN DEFAULT TRUE,
    response TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 初始化管廊路径数据 (30公里管廊，每100米一个点)
INSERT INTO pipe_corridor_path (position, latitude, longitude, segment_id, geom)
SELECT
    (n * 100.0) as position,
    39.9042 + (n * 0.0008) + (sin(n * 0.1) * 0.0002) as latitude,
    116.4074 + (n * 0.001) + (cos(n * 0.05) * 0.0001) as longitude,
    (n / 50)::integer as segment_id,
    ST_SetSRID(ST_MakePoint(
        116.4074 + (n * 0.001) + (cos(n * 0.05) * 0.0001),
        39.9042 + (n * 0.0008) + (sin(n * 0.1) * 0.0002)
    ), 4326) as geom
FROM generate_series(0, 300) as n;

-- 初始化检测器数据 (300台激光检测器，每100米一台)
INSERT INTO detectors (device_id, name, position, latitude, longitude, fire_zone, status, health, geom)
SELECT
    'LASER-' || LPAD(n::text, 4, '0') as device_id,
    '激光甲烷检测器-' || LPAD(n::text, 4, '0') as name,
    (n * 100.0) as position,
    39.9042 + (n * 0.0008) + (sin(n * 0.1) * 0.0002) as latitude,
    116.4074 + (n * 0.001) + (cos(n * 0.05) * 0.0001) as longitude,
    'ZONE-' || LPAD(((n / 30) + 1)::text, 2, '0') as fire_zone,
    'normal' as status,
    100.0 as health,
    ST_SetSRID(ST_MakePoint(
        116.4074 + (n * 0.001) + (cos(n * 0.05) * 0.0001),
        39.9042 + (n * 0.0008) + (sin(n * 0.1) * 0.0002)
    ), 4326) as geom
FROM generate_series(0, 299) as n;

-- 初始化环境传感器 (50台氧气/温湿度传感器)
INSERT INTO environment_sensors (device_id, name, sensor_type, location_type, latitude, longitude, fire_zone)
SELECT
    CASE WHEN n % 2 = 0 THEN 'ENV-O2-' || LPAD(((n/2) + 1)::text, 2, '0')
         ELSE 'ENV-TH-' || LPAD(((n/2) + 1)::text, 2, '0')
    END as device_id,
    CASE WHEN n % 2 = 0 THEN '氧气传感器-' || LPAD(((n/2) + 1)::text, 2, '0')
         ELSE '温湿度传感器-' || LPAD(((n/2) + 1)::text, 2, '0')
    END as name,
    CASE WHEN n % 2 = 0 THEN 'oxygen' ELSE 'temp_humidity' END as sensor_type,
    CASE WHEN n < 25 THEN 'vent' ELSE 'valve' END as location_type,
    39.9042 + ((n * 600) * 0.0008) as latitude,
    116.4074 + ((n * 600) * 0.001) as longitude,
    'ZONE-' || LPAD(((n * 2) + 1)::text, 2, '0') as fire_zone
FROM generate_series(0, 49) as n;

-- 初始化阀门 (50个防火分区阀门)
INSERT INTO valves (valve_id, name, fire_zone, latitude, longitude)
SELECT
    'VALVE-' || LPAD(n::text, 2, '0') as valve_id,
    '防火分区阀门-' || LPAD(n::text, 2, '0') as name,
    'ZONE-' || LPAD(n::text, 2, '0') as fire_zone,
    39.9042 + ((n * 1000) * 0.0008) as latitude,
    116.4074 + ((n * 1000) * 0.001) as longitude
FROM generate_series(1, 50) as n;

-- 初始化排风机
INSERT INTO fans (fan_id, name, fire_zone, latitude, longitude)
SELECT
    'FAN-' || LPAD(n::text, 2, '0') as fan_id,
    '排风机-' || LPAD(n::text, 2, '0') as name,
    'ZONE-' || LPAD(n::text, 2, '0') as fire_zone,
    39.9042 + ((n * 1000) * 0.0008) as latitude,
    116.4074 + ((n * 1000) * 0.001) as longitude
FROM generate_series(1, 50) as n;

-- 初始化防火分区
INSERT INTO fire_zones (zone_id, name, start_position, end_position,
    start_latitude, start_longitude, end_latitude, end_longitude)
SELECT
    'ZONE-' || LPAD(n::text, 2, '0') as zone_id,
    '防火分区' || n || '区' as name,
    (n - 1) * 600.0 as start_position,
    n * 600.0 as end_position,
    39.9042 + (((n - 1) * 600) * 0.0008) as start_latitude,
    116.4074 + (((n - 1) * 600) * 0.001) as start_longitude,
    39.9042 + (n * 600 * 0.0008) as end_latitude,
    116.4074 + (n * 600 * 0.001) as end_longitude
FROM generate_series(1, 50) as n;

-- 创建更新时间触发器函数
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_detectors_update
    BEFORE UPDATE ON detectors
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trigger_environment_sensors_update
    BEFORE UPDATE ON environment_sensors
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trigger_valves_update
    BEFORE UPDATE ON valves
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER trigger_fans_update
    BEFORE UPDATE ON fans
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE INDEX idx_alarms_unresolved ON alarms(resolved) WHERE resolved = FALSE;
CREATE INDEX idx_leak_sources_active ON leak_sources(status) WHERE status = 'active';
