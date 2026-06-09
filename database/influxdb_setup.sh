#!/bin/bash

# InfluxDB 自动初始化脚本
# 适用于 InfluxDB 2.x

set -e

INFLUX_HOST="${INFLUX_HOST:-http://localhost:8086}"
INFLUX_USER="${INFLUX_USER:-admin}"
INFLUX_PASSWORD="${INFLUX_PASSWORD:-admin123456}"
INFLUX_ORG="${INFLUX_ORG:-smart-city}"
INFLUX_BUCKET="${INFLUX_BUCKET:-gas-data}"
INFLUX_RETENTION="${INFLUX_RETENTION:-720h}"

echo "=== InfluxDB 初始化 ==="
echo "Host: $INFLUX_HOST"
echo "Org: $INFLUX_ORG"
echo "Bucket: $INFLUX_BUCKET"

# 等待 InfluxDB 启动
echo "等待 InfluxDB 启动..."
until curl -s "$INFLUX_HOST/health" | grep -q '"status":"pass"'; do
    sleep 2
    echo -n "."
done
echo ""
echo "InfluxDB 已就绪"

# 检查是否已初始化
if ! influx bucket list --host "$INFLUX_HOST" --org "$INFLUX_ORG" 2>/dev/null | grep -q "$INFLUX_BUCKET"; then
    echo "开始初始化..."

    # 创建初始用户和组织
    influx setup \
        --host "$INFLUX_HOST" \
        --username "$INFLUX_USER" \
        --password "$INFLUX_PASSWORD" \
        --org "$INFLUX_ORG" \
        --bucket "$INFLUX_BUCKET" \
        --retention "$INFLUX_RETENTION" \
        --force

    echo "初始设置完成"

    # 获取 admin token
    ADMIN_TOKEN=$(influx auth list --host "$INFLUX_HOST" --org "$INFLUX_ORG" --user "$INFLUX_USER" | grep "all-access" | awk '{print $4}')
    echo "Admin Token: $ADMIN_TOKEN"
    echo "请将此 token 配置到 config.yaml 中"

    # 创建降采样存储桶
    influx bucket create \
        --host "$INFLUX_HOST" \
        --org "$INFLUX_ORG" \
        --name "gas-data-downsampled" \
        --retention 8760h

    echo "降采样存储桶创建完成"
else
    echo "InfluxDB 已初始化"
fi

echo "=== 初始化完成 ==="
