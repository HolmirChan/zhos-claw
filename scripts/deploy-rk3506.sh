#!/usr/bin/env bash
set -e

DEVICE_IP=${DEVICE_IP:-"192.168.1.100"}
DEVICE_USER=${DEVICE_USER:-"root"}
REMOTE_DIR="/opt/zhosclaw"

echo "==> Building for RK3506..."
make build-rk3506

echo "==> Stopping existing processes on $DEVICE_IP..."
ssh "${DEVICE_USER}@${DEVICE_IP}" "mkdir -p ${REMOTE_DIR} && \
    pkill -f zhosclaw || true && \
    pkill -f picoclaw-launcher || true"

echo "==> Uploading binaries..."
scp build/zhosclaw-linux-arm           "${DEVICE_USER}@${DEVICE_IP}:${REMOTE_DIR}/zhosclaw"
scp build/picoclaw-launcher-linux-arm  "${DEVICE_USER}@${DEVICE_IP}:${REMOTE_DIR}/picoclaw-launcher"

echo "==> Starting services..."
ssh "${DEVICE_USER}@${DEVICE_IP}" "cd ${REMOTE_DIR} && \
    chmod +x zhosclaw picoclaw-launcher && \
    nohup ./zhosclaw gateway > gateway.log 2>&1 & \
    sleep 2 && \
    nohup ./picoclaw-launcher > launcher.log 2>&1 & \
    echo 'Services started'"

echo ""
echo "部署完成！访问 http://${DEVICE_IP}:3000"
