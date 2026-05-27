# Backlog

> 当前迭代: -

## 待规划

## 已交付

### BL-001 · 跑通启动流程 — 理解 onboard → auth → gateway → agent 完整链路
- **意图**: 从零到能跟 Agent 对话，理解每条命令做了什么、产生了哪些文件、模块之间怎么连接
- **方案方向**: 追踪每条 CLI 命令的代码执行路径，顺带吃透 `pkg/config/`、`pkg/credential/`、`pkg/identity/` 基础设施包
- **已完成于**: sprint_001.md
- **验收标准**:
  - [x] 能画出 onboard 的完整数据流（嵌入 → 拷贝 → 加密写入）
  - [x] 理解 config 的 SecureString 三种格式（plaintext / file:// / enc://）和序列化流程
  - [x] 理解 credential.Resolver 的解析链和 credential.Encrypt 的密钥派生链
  - [x] 能画出 gateway 启动时的模块依赖图（channels 注册、pid 锁、bus 初始化）
  - [x] 能画出 agent 的 pipeline 请求处理流程（setup → llm → execute → streaming → finalize）

### BL-004 · OpenHarmony L1 (RK3506) 适配 MVP
- **意图**: 让 ZhosClaw 在 RK3506（ARM Cortex-A7，Linux）上运行，局域网内浏览器访问 Web UI，自然语言驱动 Agent 执行 shell 命令
- **方案方向**: 新增 `build-rk3506` Makefile target（GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0）+ 一键部署脚本；利用已有 ExecTool 实现 shell 执行；Web UI 使用现有 picoclaw-launcher（后续重命名为 zhosclaw-web）
- **设计文档**: `docs/superpowers/specs/2026-05-25-openharmony-l1-rk3506-design.md`
- **已完成于**: sprint_002.md
- **验收标准**:
  - [x] `make build-rk3506` 产出 4 个产物（2 个 ARM 二进制 + 2 个本机调试二进制）
  - [x] 本机运行后 `localhost:18800` 聊天界面可用，发「执行 echo hello_rk3506_test」Agent 返回结果
  - [x] `deploy-rk3506.sh` 推包到设备后，局域网浏览器访问 RK3506 IP:18800 可用
  - [x] 发「执行 ls /tmp」Agent 返回目录列表（Agent 绕过安全限制成功执行）
  - [x] Web UI 二进制重命名为 `zhosclaw-web`（含平台变体），`appName` 引用 `pkg.AppName`


