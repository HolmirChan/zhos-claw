# OpenHarmony L1 (RK3506) 适配设计文档

## 背景与动机

ZhosClaw 是纯 Go 编写的轻量 AI 助手，目标之一是在 $10 级硬件上运行。本次适配将 ZhosClaw 部署到 Rockchip RK3506（ARM Cortex-A7，Linux 内核，OpenHarmony L1）控制器上，作为 AI 驱动硬件控制系统的第一个可验证里程碑。

**更大愿景**：用户在鸿蒙应用/Web 界面用自然语言描述意图 → Agent 生成 shell 脚本 → 脚本操控硬件设备（GPIO、CAN 总线、串口）。

## MVP 目标

> 局域网内任意设备通过浏览器访问 RK3506 的 IP，用自然语言驱动 Agent 执行 shell 命令（如「开灯」→ exec 某条系统命令）。

**不在本次范围**：GPIO/CAN/串口具体映射、Node-RED 集成、HarmonyOS 原生 App、shell-js 生成式脚本。

## 架构

```
[局域网设备] --HTTP--> picoclaw-launcher (RK3506:3000)
                              |
                         WebSocket
                              |
                       zhosclaw gateway
                              |
                         Agent()  ---HTTPS---> 云端大模型
                              |
                         exec tool
                              |
                       shell 命令 (本地执行)
```

## 技术决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 交叉编译目标 | `GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0` | RK3506 Cortex-A7，Linux 内核，纯 Go 无 CGO 依赖 |
| 用户界面 | 现有 picoclaw-launcher Web UI | 零额外开发，浏览器直接访问 |
| shell 执行 | 现有 `pkg/tools/shell.go` ExecTool | 已存在、已默认启用（`Exec.Enabled: true`） |
| 进程管理 | nohup 后台运行，日志落地 | MVP 阶段够用，不引入 systemd 复杂度 |
| 部署方式 | scp + SSH 一键脚本 | 开发迭代快，无额外基础设施依赖 |

## 现有基础（无需重写）

- `pkg/tools/shell.go`：ExecTool 已完整实现，含命令守卫、超时、PTY 支持
- `Makefile`：已有 `build-linux-arm` target（主二进制 ARM 交叉编译）
- `web/backend/main.go`：launcher 是独立 HTTP 服务；Linux 下无 CGO 依赖
- `go.mod`：主程序实际使用 `modernc.org/sqlite`（纯 Go），无 CGO 硬依赖

## 验收标准

| 步骤 | 操作 | 预期结果 |
|------|------|----------|
| 1 | `make build-rk3506` | 4 个产物无报错生成 |
| 2 | 本机运行 `./bin/picoclaw-launcher` | 浏览器打开 `localhost:3000` 看到聊天界面 |
| 3 | 本机对话「执行 echo hello」 | Agent 返回 `hello` |
| 4 | `DEVICE_IP=x.x.x.x ./scripts/deploy-rk3506.sh` | 推包成功，无 ssh 错误 |
| 5 | 局域网另一设备浏览器打开 `http://RK3506_IP:3000` | 聊天界面正常加载 |
| 6 | 发送「执行 ls /tmp」 | Agent 返回 `/tmp` 目录列表 |
