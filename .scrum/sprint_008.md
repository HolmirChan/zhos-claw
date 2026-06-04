# Sprint 8

> 创建: 2026-06-04
> 完成: -
> 来源: BACKLOG.md（BL-011）
> 设计文档: .scrum/specs/2026-06-04-tool-error-classification-design.md
> 实现计划: .scrum/plans/2026-06-04-tool-error-classification.md
> 状态: 进行中

## 任务清单

### SB-001 · 阶段一：prompt 注入熔断规则 [ ]
- **来源**: BL-011
- **实现计划**: Task 1, 2
- **子任务**:
  - [ ] 注册 PromptSourceToolGuard 常量
  - [ ] BuildSystemPromptParts 注入工具失败规则 prompt
  - [ ] 编译 + 测试 PASS
  - [ ] 提交

### SB-002 · 阶段一：宿主侧连续拦截计数器 [ ]
- **来源**: BL-011
- **实现计划**: Task 3, 4, 6
- **子任务**:
  - [ ] turnState 新增 consecutiveBlockedCount + recordToolResult()
  - [ ] pipeline_execute 三处调用 recordToolResult（同步/hook respond/async）
  - [ ] 单元测试：清零、硬中断、abort 后防御
  - [ ] 提交

### SB-003 · 阶段一：SubAgent BlockedType 清空 [ ]
- **来源**: BL-011
- **实现计划**: Task 5
- **子任务**:
  - [ ] spawn.go async goroutine 回调前清空 result.BlockedType
  - [ ] 编译 PASS
  - [ ] 提交

### SB-004 · 阶段一：集成验证 [ ]
- **来源**: BL-011
- **阻塞**: SB-001, SB-002, SB-003
- **实现计划**: Task 7
- **子任务**:
  - [ ] 全量测试 PASS
  - [ ] make build && make build-launcher
  - [ ] 部署 RK3506 验证 tool iteration ≤ 5
  - [ ] 提交

### SB-005 · 阶段二：ToolResult BlockedType + facade re-export [ ]
- **来源**: BL-011
- **实现计划**: Task 8, 12
- **子任务**:
  - [ ] result.go 新增常量 + 字段 + WithBlockedType + ContentForLLM 前缀
  - [ ] shared_facade.go + fs/shared.go 常量 re-export
  - [ ] 单元测试：前缀拼装、空 content、普通错误无前缀
  - [ ] 编译 + 测试 PASS
  - [ ] 提交

### SB-006 · 阶段二：shell.go guardCommand 签名改造 [ ]
- **来源**: BL-011
- **实现计划**: Task 9
- **子任务**:
  - [ ] guardCommand 改为 (string, string)，6 处 return 全适配
  - [ ] 3 处直接 ErrorResult 套 WithBlockedType
  - [ ] 测试 PASS
  - [ ] 提交

### SB-007 · 阶段二：filesystem.go sentinel + helper + Execute 层替换 [ ]
- **来源**: BL-011
- **实现计划**: Task 10, 11
- **子任务**:
  - [ ] ErrWorkspaceBoundary sentinel + 6 处挂载
  - [ ] sandboxFs 3 条件拆分
  - [ ] errorResultFromFS / errorResultFromFSCtx helper
  - [ ] Execute 层 6 处 ErrorResult 替换
  - [ ] 单元测试：sentinel 传播、OS 权限不触发
  - [ ] 测试 PASS
  - [ ] 提交

### SB-008 · 阶段二：集成验证 [ ]
- **来源**: BL-011
- **阻塞**: SB-005, SB-006, SB-007
- **实现计划**: Task 13
- **子任务**:
  - [ ] 全量测试 PASS
  - [ ] make build && make build-launcher
  - [ ] 部署 RK3506 验证 LLM 自律停止 ≤ 3 次
  - [ ] 审计 blocked_type JSONL 字段
  - [ ] 提交

### SB-009 · 联合验证 [ ]
- **来源**: BL-011
- **阻塞**: SB-004, SB-008
- **实现计划**: Task 14
- **子任务**:
  - [ ] 全量回归 go test ./... -count=1
  - [ ] 全量构建
  - [ ] 部署 RK3506 混合拦截场景验证
  - [ ] grep blocked_type 审计
