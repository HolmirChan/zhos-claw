#!/usr/bin/env bash
set -e

DEVICE_IP=${DEVICE_IP:-"192.168.1.100"}
DEVICE_PORT=${DEVICE_PORT:-"5555"}
REMOTE_DIR="/data/zhosclaw"
PICOCLAW_HOME_DIR="/opt/zhosclaw/.zhosclaw"
LOG_DIR="${PICOCLAW_HOME_DIR}/logs"
HDC="hdc -t ${DEVICE_IP}:${DEVICE_PORT}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ASSETS_DIR="${SCRIPT_DIR}/deploy-rk3506"

PASSPHRASE=$(cat "${ASSETS_DIR}/passphrase.txt" | tr -d '\n')

echo "==> Building for RK3506..."
make build-rk3506

echo "==> Connecting to device ${DEVICE_IP}:${DEVICE_PORT}..."
hdc conn "${DEVICE_IP}:${DEVICE_PORT}"

echo "==> Stopping existing processes..."
${HDC} shell "pkill -f zhosclaw-web || true; pkill -f zhosclaw || true"
${HDC} shell "mkdir -p ${REMOTE_DIR} ${PICOCLAW_HOME_DIR} ${LOG_DIR}"


echo "==> Uploading files..."
${HDC} file send "${ASSETS_DIR}/cert.pem"              "${REMOTE_DIR}/cert.pem"
${HDC} file send "${ASSETS_DIR}/zhosclaw_ed25519.key"  "${PICOCLAW_HOME_DIR}/zhosclaw_ed25519.key"
${HDC} file send "build/zhosclaw-linux-arm"            "${REMOTE_DIR}/zhosclaw-linux-arm"
${HDC} file send "build/zhosclaw-web-linux-arm"        "${REMOTE_DIR}/zhosclaw-web-linux-arm"

echo "==> Setting permissions and symlink..."
${HDC} shell "chmod +x ${REMOTE_DIR}/zhosclaw-linux-arm ${REMOTE_DIR}/zhosclaw-web-linux-arm"
${HDC} shell "chmod 600 ${PICOCLAW_HOME_DIR}/zhosclaw_ed25519.key"
${HDC} shell "ln -sf ${REMOTE_DIR}/zhosclaw-linux-arm ${REMOTE_DIR}/zhosclaw"

echo "==> Initializing home directory (first deploy only)..."
CONFIG_EXISTS=$(${HDC} shell "ls ${PICOCLAW_HOME_DIR}/config.json 2>/dev/null && echo yes || echo no")
if echo "${CONFIG_EXISTS}" | grep -qF "no"; then
    jq --arg ws "${PICOCLAW_HOME_DIR}/workspace" '
        .agents.defaults.workspace = $ws |
        walk(if type == "object" then with_entries(select(.key != "_comment")) else . end) |
        del(.tools.web.brave.api_key, .tools.web.perplexity.api_key, .tools.web.tavily.api_key)
    ' "${SCRIPT_DIR}/../config/config.example.json" > /tmp/zhosclaw_deploy_config.json
    ${HDC} file send /tmp/zhosclaw_deploy_config.json "${PICOCLAW_HOME_DIR}/config.json"
    rm /tmp/zhosclaw_deploy_config.json
    ${HDC} file send "cmd/picoclaw/internal/onboard/workspace" "${PICOCLAW_HOME_DIR}/workspace"
fi

echo "==> Generating and uploading init script..."
cat > /tmp/S81zhosclaw << EOF
#!/bin/sh
export PICOCLAW_HOME=${PICOCLAW_HOME_DIR}
export PICOCLAW_SSH_KEY_PATH=${PICOCLAW_HOME_DIR}/zhosclaw_ed25519.key
export PICOCLAW_KEY_PASSPHRASE=${PASSPHRASE}
export SSL_CERT_FILE=${REMOTE_DIR}/cert.pem
GODEBUG=asyncpreemptoff=1 nohup ${REMOTE_DIR}/zhosclaw-web-linux-arm -public >> ${LOG_DIR}/launcher.log 2>&1 &
EOF
${HDC} file send /tmp/S81zhosclaw "/etc/init.d/S81zhosclaw"
rm /tmp/S81zhosclaw
${HDC} shell "chmod +x /etc/init.d/S81zhosclaw"
${HDC} shell "sync"

echo "==> Starting services..."
${HDC} shell "/etc/init.d/S81zhosclaw"

echo ""
echo "部署完成！访问 http://${DEVICE_IP}:18800"
