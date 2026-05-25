# Sprint 1

> 创建: 2026-05-21
> 目标版本: -
> 来源: BACKLOG.md（BL-001）
> 状态: 已完成

## 任务清单

### SB-001 · 理解 onboard 完整数据流 [x]
- **来源**: BL-001
- **子任务**:
  - [x] 追踪 `onboard()` 主流程（config 检查、加密设置、workspace 拷贝）
  - [x] 理解 config.DefaultConfig() 的结构和默认模型/通道清单
  - [x] 理解 credential.Resolver 的三种格式解析链（plaintext / file:// / enc://）
  - [x] 理解 credential.Encrypt 的密钥派生链（passphrase + SSH 私钥 → HKDF → AES-256-GCM）
  - [x] 理清文件路径体系（PICOCLAW_HOME / PICOCLAW_CONFIG / ~/.ssh/zhosclaw_ed25519.key）
  - [x] 总结输出 onboard 完整数据流图（见下方总结）

### SB-002 · 理解 auth 命令的登录/登出/绑定流程 [x]
- **来源**: BL-001
- **子任务**:
  - [x] 读 `cmd/picoclaw/internal/auth/command.go` 理解子命令结构
  - [x] 读 `pkg/auth/store.go` 理解 token 存储格式
  - [x] 追踪 wecom/weixin 等通道的 OAuth 绑定流程
  - [x] 理清 auth 和 credential 的关系（token 最终写到哪里、什么格式）

### SB-003 · 理解 gateway 启动时的模块依赖 [x]
- **来源**: BL-001
- **子任务**:
  - [x] 读 `cmd/picoclaw/internal/gateway/command.go` 理解启动流程
  - [x] 读 `pkg/gateway/gateway.go` 理解初始化顺序（pid 锁 → config 加载 → channels 注册 → bus 初始化）
  - [x] 读 `pkg/channels/manager.go` 理解通道注册和生命周期（Init/Start/Stop）
  - [x] 理清 gateway 和 agent 的连接方式（同进程？RPC？bus 事件？）
  - [x] 画出 gateway 启动的模块依赖图（见分析输出）

### SB-004 · 理解 agent 的 pipeline 请求处理流程 [x]
- **来源**: BL-001
- **子任务**:
  - [x] 追踪 `picoclaw agent` 从 CLI 到第一条消息的完整调用链
  - [x] 读 pipeline_setup / pipeline_llm / pipeline_execute / pipeline_streaming / pipeline_finalize 各阶段的职责
  - [x] 理解 subturn（子轮次）和 turn_coord（轮次协调）的协作关系
  - [x] 理解 context_manager 的 token 预算管理机制
  - [x] 画出 agent pipeline 完整数据流图（见分析输出）
