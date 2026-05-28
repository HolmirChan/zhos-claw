# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 语言

所有对话使用中文。

## 构建与开发命令

```bash
# 构建主程序（默认当前平台）
make build

# 构建全平台
make build-all

# 构建 Web 管理面板
make build-launcher

# 运行测试（排除 web/，web 单独测）
make test

# 运行单个包的测试
go test -v -tags goolm,stdjson ./pkg/agent/...

# 运行单个测试函数
go test -v -tags goolm,stdjson -run TestAgentMessage ./pkg/agent/

# 代码检查
make lint          # golangci-lint + 文档链接检查
make vet           # go vet（排除 web/）

# 格式化
make fmt           # golangci-lint fmt

# 集成测试（需要 Docker）
make integration-test

# 安装到系统
make install
```

构建 tag 默认是 `goolm,stdjson`。如需 WhatsApp 原生支持（whatsmeow），加上 `whatsapp_native` tag。`CGO_ENABLED=0` 是默认值。

## 项目架构

PicoClaw 是一个超轻量个人 AI 助手，纯 Go 编写，目标是在 $10 硬件、<10MB RAM 上运行。模块路径 `github.com/sipeed/picoclaw`。

### 两个独立二进制

| 二进制 | 入口 | 用途 |
|--------|------|------|
| `zhosclaw` | `cmd/picoclaw/main.go` | 核心系统（CLI + agent + gateway） |
| `picoclaw-launcher` | `web/backend/main.go` | Web 管理面板（Go 后端 + React 前端） |

两者零依赖关系：`cmd/` 和 `pkg/` 不 import `web/`，`web/` 单向 import `pkg/`。

### 核心分层

```
cmd/picoclaw/internal/   CLI 层：cobra 子命令实现（agent/gateway/auth/onboard/mcp/...）
        │
        ▼
pkg/agent/               Agent 核心引擎：请求管道、上下文管理、工具调度、Seahorse 记忆检索
pkg/providers/           AI 供应商抽象：Anthropic / OpenAI / Bedrock / Azure / 本地 CLI / OAuth
pkg/channels/            消息通道：WeChat / Telegram / Discord / Slack / IRC / MQTT 等 20+ 平台
pkg/gateway/             多通道网关：启动通道、请求路由、PID 文件单例锁
        │
        ▼
pkg/tools/               工具执行：shell / 文件 / 搜索 / 子 Agent / cron / 硬件
pkg/seahorse/            轻量记忆引擎（SQLite FTS5）：对话压缩、上下文检索
pkg/evolution/           自我进化：LLM 驱动的反思与行为改进
pkg/skills/              技能系统：ClawHub / GitHub 注册中心、安装器、加载器
pkg/session/             会话管理：JSONL 持久化、key 分配
        │
        ▼
pkg/config/ / logger/ / bus/ / events/ / utils/ / ...  基础设施
```

### 关键设计决策

- **Seahorse 替代向量数据库**：基于 SQLite FTS5 全文检索做长期记忆，无需外部依赖
- **Evolution 自我改进**：Agent 运行时生成改进草案、审查、应用，形成自反馈循环
- **进程隔离**：`pkg/isolation/` 对所有外部命令执行做沙箱（Linux cgroups / Windows job objects）
- **单例网关**：`pkg/pid/pidfile.go` 通过 PID 文件 + 进程存活检测确保只有一个 gateway 实例
- **配置嵌入**：`cmd/picoclaw/internal/onboard/` 用 `//go:embed` 将 `workspace/` 目录编入二进制，`picoclaw onboard` 时拷贝到用户目录

### 文件命名约定

- `*_test.go` — 测试文件
- `*_unix.go` / `*_windows.go` / `*_linux.go` — 平台特定实现
- `*_32.go` / `*_64.go` — 架构特定（如 `feishu_32.go` / `feishu_64.go`）

## Git 提交

每个 commit message 末尾加上：

```
Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code
```

## 敏捷文档

- 需求池: `.scrum/BACKLOG.md`
- 迭代文件: `.scrum/sprint_*.md`
- 设计文档: `.scrum/specs/YYYY-MM-DD-<topic>-design.md`
- 实现计划: `.scrum/plans/YYYY-MM-DD-<topic>.md`
- 当前迭代由 BACKLOG.md 顶部元数据行声明
- 文档模板: `.scrum/TemplateDescript.md`
- 完整流程规则见技能 `holmir-scrum-init`

### 协作规则

1. **需求澄清**：调用 `superpowers:brainstorming`，产出设计文档到 `.scrum/specs/`，review 通过后将需求概要写入 BACKLOG.md「待规划」区
2. **迭代启动**：调用 `superpowers:writing-plans`，产出实现计划到 `.scrum/plans/`，再生成 `sprint_NNN.md`（引用对应设计文档和计划文件路径）
3. **迭代进行**：按 sprint 文件执行（subagent-driven 或 executing-plans），同步更新子任务状态（`[ ]` / `[~]` / `[x]`），遇到偏离回退 brainstorming
4. **迭代关闭**：已完成条目从 BACKLOG.md「待规划」移到「已交付」，补上 `已完成于 sprint_NNN.md`；Sprint 补完成时间；未完成项回写「待规划」
5. **先行后续**：设计文档 → 实现计划 → Sprint 文件，三步顺序执行，不跳过
6. **BACKLOG.md 只排序不标优先级/日期**，时间承诺只在 Sprint 文件中体现
7. **CLAUDE.md 为纯规则文件**，不随迭代变更
