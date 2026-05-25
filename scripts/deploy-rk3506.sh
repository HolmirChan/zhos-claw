#!/usr/bin/env bash
set -e

DEVICE_IP=${DEVICE_IP:-"192.168.1.100"}
DEVICE_USER=${DEVICE_USER:-"root"}
REMOTE_DIR="/opt/zhosclaw"
SSH_OPTS="-o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new"

echo "==> Building for RK3506..."
make build-rk3506

echo "==> Stopping existing processes on $DEVICE_IP..."
ssh ${SSH_OPTS} "${DEVICE_USER}@${DEVICE_IP}" "mkdir -p ${REMOTE_DIR} && \
    pkill -f zhosclaw || true && \
    pkill -f zhosclaw-web || true"

echo "==> Uploading binaries..."
scp ${SSH_OPTS} build/zhosclaw-linux-arm           "${DEVICE_USER}@${DEVICE_IP}:${REMOTE_DIR}/zhosclaw"
scp ${SSH_OPTS} build/zhosclaw-web-linux-arm  "${DEVICE_USER}@${DEVICE_IP}:${REMOTE_DIR}/zhosclaw-web"

echo "==> Starting services..."
ssh ${SSH_OPTS} "${DEVICE_USER}@${DEVICE_IP}" "cd ${REMOTE_DIR} && \
    chmod +x zhosclaw zhosclaw-web && \
    nohup ./zhosclaw gateway >> gateway.log 2>&1 & \
    sleep 2 && \
    nohup ./zhosclaw-web >> launcher.log 2>&1 & \
    echo 'Services started'"

echo ""
echo "部署完成！访问 http://${DEVICE_IP}:3000"
