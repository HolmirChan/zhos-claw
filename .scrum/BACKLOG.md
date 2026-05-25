# Backlog

> 当前迭代: -

## 待规划

（暂无条目）

## 已交付

### BL-001 · 跑通启动流程 — 理解 onboard → auth → gateway → agent 完整链路
- **意图**: 从零到能跟 Agent 对话，理解每条命令做了什么、产生了哪些文件、模块之间怎么连接
- **方案方向**: 追踪每条 CLI 命令的代码执行路径，顺带吃透 `pkg/config/`、`pkg/credential/`、`pkg/identity/` 基础设施包
- **已完成于**: sprint_001.md
- **验收标准**:
  - [ ] 能画出 onboard 的完整数据流（嵌入 → 拷贝 → 加密写入）
  - [ ] 理解 config 的 SecureString 三种格式（plaintext / file:// / enc://）和序列化流程
  - [ ] 理解 credential.Resolver 的解析链和 credential.Encrypt 的密钥派生链
  - [ ] 能画出 gateway 启动时的模块依赖图（channels 注册、pid 锁、bus 初始化）
  - [ ] 能画出 agent 的 pipeline 请求处理流程（setup → llm → execute → streaming → finalize）

### BL-002 · 深入核心模块 — agent 管道、providers 抽象、channels 架构、seahorse 记忆
- **意图**: 能接手核心引擎的开发，理解四个最关键模块的内部组织、关键数据结构和扩展点
- **方案方向**: 对每个核心模块做二级文件拆解 + 关键接口梳理 + 数据流追踪
- **验收标准**:
  - [ ] agent: 理解 pipeline 各阶段的职责和输入输出，subturn/turn_coord/steering 的协作关系
  - [ ] providers: 理解 Factory 如何路由到具体供应商，fallback/cooldown 机制
  - [ ] channels: 理解通道注册、生命周期（Init/Start/Stop）、消息入站到 Agent 再到出站的完整链路
  - [ ] seahorse: 理解 short_engine 的写入/压缩/检索流程，FTS5 schema 设计

### BL-003 · 中等深度扫完其余模块
- **意图**: 对剩余 pkg 包做到「知道干什么、知道关键接口、能定位问题」
- **方案方向**: 快速过 tools / skills / evolution / session / mcp / events / bus / isolation / devices / audio，不深挖实现细节
- **验收标准**:
  - [ ] 每个包能一句话说清职责
  - [ ] 知道每个包对外暴露的关键类型/函数
  - [ ] 能在需要时快速定位到对应源码

## 已交付

（暂无条目）
