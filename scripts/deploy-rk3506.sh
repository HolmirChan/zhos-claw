#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ASSETS_DIR="${SCRIPT_DIR}/deploy-rk3506"
ENV_GO="${SCRIPT_DIR}/../pkg/env.go"

# Read branding values from pkg/env.go (single source of truth)
CMD=$(grep 'CommandName' "$ENV_GO" | grep -o '"[a-z]*"' | tr -d '"')
PREFIX=$(grep 'EnvPrefix' "$ENV_GO" | grep -o '"[A-Z_]*"' | tr -d '"')
HOME_DIR_NAME=$(grep 'DefaultHome' "$ENV_GO" | grep -o '"\.[a-z]*"' | tr -d '"')

DEVICE_IP=${DEVICE_IP:-"192.168.1.100"}
DEVICE_PORT=${DEVICE_PORT:-"5555"}
REMOTE_DIR="/data/${CMD}"
PICOCLAW_HOME_DIR="${REMOTE_DIR}/${HOME_DIR_NAME}"
LOG_DIR="${PICOCLAW_HOME_DIR}/logs"
HDC="hdc -t ${DEVICE_IP}:${DEVICE_PORT}"

PASSPHRASE=$(cat "${ASSETS_DIR}/passphrase.txt" | tr -d '\n')

echo "==> Building for RK3506..."
make build-rk3506

echo "==> Connecting to device ${DEVICE_IP}:${DEVICE_PORT}..."
hdc tconn "${DEVICE_IP}:${DEVICE_PORT}"

echo "==> Stopping existing processes..."
${HDC} shell "pkill -f ${CMD}-web || true; pkill -f ${CMD} || true"
sleep 3
${HDC} shell "mkdir -p ${REMOTE_DIR} ${PICOCLAW_HOME_DIR}"

echo "==> Uploading files..."
${HDC} file send "${ASSETS_DIR}/cert.pem"              "${REMOTE_DIR}/cert.pem"
${HDC} file send "${ASSETS_DIR}/zaiagent_ed25519.key"  "${PICOCLAW_HOME_DIR}/${CMD}_ed25519.key"
${HDC} file send "build/${CMD}-linux-arm"              "${REMOTE_DIR}/${CMD}-linux-arm"
${HDC} file send "build/${CMD}-web-linux-arm"          "${REMOTE_DIR}/${CMD}-web-linux-arm"

echo "==> Setting permissions..."
${HDC} shell "chmod +x ${REMOTE_DIR}/${CMD}-linux-arm ${REMOTE_DIR}/${CMD}-web-linux-arm"
${HDC} shell "chmod 600 ${PICOCLAW_HOME_DIR}/${CMD}_ed25519.key"

echo "==> Initializing home directory (first deploy only)..."
CONFIG_EXISTS=$(${HDC} shell "ls ${PICOCLAW_HOME_DIR}/config.json 2>/dev/null && echo yes || echo no")
if echo "${CONFIG_EXISTS}" | grep -qF "no"; then
    jq --arg ws "${PICOCLAW_HOME_DIR}/workspace" '
        .agents.defaults.workspace = $ws |
        walk(if type == "object" then with_entries(select(.key != "_comment")) else . end) |
        del(.tools.web.brave.api_key, .tools.web.perplexity.api_key, .tools.web.tavily.api_key)
    ' "${SCRIPT_DIR}/../config/config.example.json" > /tmp/${CMD}_deploy_config.json
    ${HDC} file send /tmp/${CMD}_deploy_config.json "${PICOCLAW_HOME_DIR}/config.json"
    rm /tmp/${CMD}_deploy_config.json
    ${HDC} file send "cmd/picoclaw/internal/onboard/workspace" "${PICOCLAW_HOME_DIR}/workspace"
fi

echo "==> Generating and uploading init script..."
INIT_NAME="S81${CMD}"
cat > "/tmp/${INIT_NAME}" << EOF
#!/bin/sh
export ${PREFIX}HOME=${PICOCLAW_HOME_DIR}
export ${PREFIX}SSH_KEY_PATH=${PICOCLAW_HOME_DIR}/${CMD}_ed25519.key
export ${PREFIX}KEY_PASSPHRASE=${PASSPHRASE}
export ${PREFIX}BINARY=${REMOTE_DIR}/${CMD}-linux-arm
export SSL_CERT_FILE=${REMOTE_DIR}/cert.pem
mkdir -p ${LOG_DIR}
GODEBUG=asyncpreemptoff=1 nohup ${REMOTE_DIR}/${CMD}-web-linux-arm -public >> ${LOG_DIR}/launcher.log 2>&1 &
EOF
${HDC} file send "/tmp/${INIT_NAME}" "/etc/init.d/${INIT_NAME}"
rm "/tmp/${INIT_NAME}"
${HDC} shell "chmod +x /etc/init.d/${INIT_NAME}"
${HDC} shell "sync"

echo "==> Starting services..."
${HDC} shell "/etc/init.d/${INIT_NAME}"

echo ""
echo "部署完成！"
echo "  HTTP:  http://${DEVICE_IP}:18800"
echo "  HTTPS: https://${DEVICE_IP}:18443  (语音功能)"
