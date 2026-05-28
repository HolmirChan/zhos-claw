# OpenHarmony L1 (RK3506) 适配 MVP 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 RK3506（ARM Cortex-A7，Linux）上运行 ZhosClaw + Web UI，局域网浏览器访问，自然语言驱动 Agent 执行 shell 命令。

**Architecture:** 新增 `build-rk3506` Makefile target 交叉编译两个二进制（主程序 + launcher），配套 `scripts/deploy-rk3506.sh` 一键 scp+ssh 推包并后台启动，利用已有 ExecTool 实现 shell 执行能力。

**Tech Stack:** Go 1.x cross-compile (`GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0`)，现有 `pkg/tools/shell.go` ExecTool，现有 `web/backend` launcher，Makefile，bash

---

## 文件变更总览

| 文件 | 操作 | 职责 |
|------|------|------|
| `Makefile` | 修改 | 新增 `build-rk3506` target |
| `scripts/deploy-rk3506.sh` | 新建 | 一键构建 + 推包 + 启动 |
| `docs/superpowers/specs/2026-05-25-openharmony-l1-rk3506-design.md` | 新建 | 设计文档（已完成） |
| `.scrum/BACKLOG.md` | 修改 | 新增需求条目 |

---

## Task 1：更新 BACKLOG.md

**Files:**
- Modify: `.scrum/BACKLOG.md`

- [ ] **Step 1：在「待规划」区插入新需求条目**

将 `.scrum/BACKLOG.md` 中的 `（暂无条目）` 替换为：

```markdown
### BL-004 · OpenHarmony L1 (RK3506) 适配 MVP
- **意图**: 让 ZhosClaw 在 RK3506（ARM Cortex-A7，Linux）上运行，局域网内浏览器访问 Web UI，自然语言驱动 Agent 执行 shell 命令
- **方案方向**: 新增 `build-rk3506` Makefile target（GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0）+ 一键部署脚本；利用已有 ExecTool 实现 shell 执行；Web UI 使用现有 picoclaw-launcher
- **验收标准**:
  - [ ] `make build-rk3506` 产出 4 个产物（2 个 ARM 二进制 + 2 个本机调试二进制）
  - [ ] 本机运行后 `localhost:3000` 聊天界面可用，发「执行 echo hello」Agent 返回 hello
  - [ ] `deploy-rk3506.sh` 推包到设备后，局域网浏览器访问 RK3506 IP:3000 可用
  - [ ] 发「执行 ls /tmp」Agent 返回目录列表
- **设计文档**: `docs/superpowers/specs/2026-05-25-openharmony-l1-rk3506-design.md`
```

- [ ] **Step 2：提交**

```bash
git add .scrum/BACKLOG.md docs/superpowers/specs/2026-05-25-openharmony-l1-rk3506-design.md
git commit -m "$(cat <<'EOF'
docs: add RK3506 OpenHarmony L1 MVP spec and backlog entry

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

## Task 2：新增 `build-rk3506` Makefile target

**Files:**
- Modify: `Makefile`（在 `build-pi-zero` target 之后，约第 313 行）

- [ ] **Step 1：验证 target 不存在（预期失败）**

```bash
make build-rk3506
```

预期输出：`make: *** No rule to make target 'build-rk3506'. Stop.`

- [ ] **Step 2：在 Makefile 中 `build-pi-zero` 行之后插入新 target**

找到这一行（约 312-314 行）：
```makefile
## build-pi-zero: Build for Raspberry Pi Zero 2 W (32-bit and 64-bit)
build-pi-zero: build-linux-arm build-linux-arm64
	@echo "Pi Zero 2 W builds: ..."
```

在其后插入：

```makefile

## build-rk3506: Build for RK3506 (linux/arm GOARM=7) + local debug binaries
build-rk3506: build-linux-arm ## RK3506 ARM binary + launcher ARM binary + local debug binaries
	@echo "Building picoclaw-launcher for linux/arm (RK3506)..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
		go build -v -tags stdjson -ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/picoclaw-launcher-linux-arm ./web/backend
	@echo "Build complete: $(BUILD_DIR)/picoclaw-launcher-linux-arm"
	$(MAKE) build build-launcher
	@echo "RK3506 build complete. Artifacts:"
	@echo "  $(BUILD_DIR)/$(BINARY_NAME)-linux-arm          (RK3506)"
	@echo "  $(BUILD_DIR)/picoclaw-launcher-linux-arm       (RK3506)"
	@echo "  $(BUILD_DIR)/$(BINARY_NAME)                    (local debug)"
	@echo "  $(BUILD_DIR)/picoclaw-launcher                 (local debug)"
```

注意：Makefile 缩进必须用 **Tab**，不能用空格。

- [ ] **Step 3：运行构建，验证 4 个产物均存在**

```bash
make build-rk3506 2>&1 | tail -20
```

然后检查产物：

```bash
ls -lh bin/zhosclaw-linux-arm bin/picoclaw-launcher-linux-arm bin/zhosclaw bin/picoclaw-launcher
```

预期：4 个文件均存在，`*-linux-arm` 两个文件大小 > 10MB（包含 Go runtime）。

- [ ] **Step 4：验证 ARM 二进制架构**

```bash
file bin/zhosclaw-linux-arm bin/picoclaw-launcher-linux-arm
```

预期输出包含 `ELF 32-bit LSB executable, ARM`。

- [ ] **Step 5：提交**

```bash
git add Makefile
git commit -m "$(cat <<'EOF'
feat: add build-rk3506 Makefile target for OpenHarmony L1 adaptation

Adds cross-compilation target for RK3506 (ARM Cortex-A7, linux/arm GOARM=7,
CGO_ENABLED=0). Reuses existing build-linux-arm for main binary and adds
direct go build for launcher with stdjson tag (no CGO needed on Linux).
Also builds local debug binaries via existing build/build-launcher targets.

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

## Task 3：新建部署脚本 `scripts/deploy-rk3506.sh`

**Files:**
- Create: `scripts/deploy-rk3506.sh`

- [ ] **Step 1：创建脚本文件**

```bash
cat > scripts/deploy-rk3506.sh << 'SCRIPT'
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
scp bin/zhosclaw-linux-arm           "${DEVICE_USER}@${DEVICE_IP}:${REMOTE_DIR}/zhosclaw"
scp bin/picoclaw-launcher-linux-arm  "${DEVICE_USER}@${DEVICE_IP}:${REMOTE_DIR}/picoclaw-launcher"

echo "==> Starting services..."
ssh "${DEVICE_USER}@${DEVICE_IP}" "cd ${REMOTE_DIR} && \
    chmod +x zhosclaw picoclaw-launcher && \
    nohup ./zhosclaw gateway > gateway.log 2>&1 & \
    sleep 2 && \
    nohup ./picoclaw-launcher > launcher.log 2>&1 & \
    echo 'Services started'"

echo ""
echo "部署完成！访问 http://${DEVICE_IP}:3000"
SCRIPT
chmod +x scripts/deploy-rk3506.sh
```

- [ ] **Step 2：验证脚本语法无错误**

```bash
bash -n scripts/deploy-rk3506.sh && echo "语法检查通过"
```

预期：`语法检查通过`

- [ ] **Step 3：提交**

```bash
git add scripts/deploy-rk3506.sh
git commit -m "$(cat <<'EOF'
feat: add deploy-rk3506.sh one-click deploy script

Builds RK3506 binaries, scp to device, restarts services via nohup.
Configure via DEVICE_IP and DEVICE_USER env vars (defaults: root@192.168.1.100).

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
EOF
)"
```

---

## Task 4：本机冒烟验证

**目的**：在推包到设备之前，先在本机验证 Agent exec 链路通。

- [ ] **Step 1：确认配置文件存在（onboard 过已有）**

```bash
ls ~/.config/picoclaw/ 2>/dev/null || ls ~/.picoclaw/ 2>/dev/null || echo "需要先运行 ./bin/zhosclaw onboard"
```

若不存在，先执行：

```bash
./bin/zhosclaw onboard
```

- [ ] **Step 2：启动 gateway（后台）**

```bash
./bin/zhosclaw gateway &
GATEWAY_PID=$!
sleep 2
echo "gateway PID: $GATEWAY_PID"
```

- [ ] **Step 3：启动 launcher**

```bash
./bin/picoclaw-launcher &
LAUNCHER_PID=$!
sleep 1
echo "launcher PID: $LAUNCHER_PID"
```

- [ ] **Step 4：浏览器验证**

打开 `http://localhost:3000`，确认聊天界面加载。

- [ ] **Step 5：发送 exec 测试消息**

在聊天界面发送：`执行 echo hello_rk3506_test`

预期：Agent 回复内容包含 `hello_rk3506_test`

- [ ] **Step 6：停止本机进程**

```bash
kill $LAUNCHER_PID $GATEWAY_PID 2>/dev/null || true
```

---

## 验收总结

本机验证通过后，设备验证：

```bash
DEVICE_IP=<实际IP> ./scripts/deploy-rk3506.sh
```

在局域网另一台设备浏览器打开 `http://<RK3506_IP>:3000`，发送「执行 ls /tmp」，Agent 返回目录列表即为完整验收通过。
