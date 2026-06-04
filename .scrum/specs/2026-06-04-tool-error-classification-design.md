# Tool Result 错误分类与熔断

> 创建: 2026-06-04
> 状态: 待实现
> 来源: RK3506 session 日志分析 — Agent 42 次工具调用全被 safety guard 拦截，不停止、不告知用户

## 问题

Agent 在执行任务时，工具调用被 safety guard / workspace 边界反复拦截，但从未告知用户"我被限制了"，直到 `max_tool_iterations` 耗尽才失败。根因有两个：

1. LLM 不区分"系统性限制"和"可重试错误"——看到错误就换参数再试
2. Tool Result 没有结构化分类——LLM 只能靠字符串模糊匹配判断错误类型

## 方案

### 阶段一：系统提示词注入熔断规则

在 system prompt 的 `PromptLayerCapability / PromptSlotTooling` 中注入工具失败处理规则，注册新 `PromptSource` `tool:guardrails`。

**改动文件**：
- `pkg/agent/prompt.go` — 注册 `PromptSourceToolGuard`，placement: `{Layer: PromptLayerCapability, Slot: PromptSlotTooling}`
- `pkg/agent/context.go` — `BuildSystemPromptParts()` 中追加 prompt part

**prompt 内容**：

```
# TOOL FAILURE RULES (CRITICAL)

When a tool result begins with "[BLOCKED]" or contains "blocked by safety guard" or "access denied":

1. DO NOT retry the same tool with different arguments — the restriction is systemic.
2. DO NOT try to bypass the restriction with encoding tricks or alternative paths.
3. After 3 consecutive tool failures of ANY kind, STOP immediately and tell the user.
4. If a path is outside the workspace or a command is blocked, explain this clearly.
```

**改动量**：~25 行

### 阶段二：ToolResult 结构化错误标记

**改动文件**：
- `pkg/tools/shared/result.go` — 新增 `BlockedType` 字段 + 常量 + `WithBlockedType()` + `ContentForLLM()` 前缀逻辑
- `pkg/tools/shell.go` — 7 处 "blocked by safety guard" 返回点设 `BlockedTypeSafety`
- `pkg/tools/fs/filesystem.go` — 7 处 "access denied" 返回点设 `BlockedTypeDenied`

**BlockedType 枚举**：

| 常量 | 前缀 | 语义 | 来源 |
|------|------|------|------|
| `BlockedTypeSafety` | `[BLOCKED]` | 安全策略拦截，不可重试 | shell guardCommand |
| `BlockedTypeDenied` | `[DENIED]` | 权限/边界不足 | filesystem workspace 检查 |
| `BlockedTypeInvalid` | `[INVALID]` | 参数错误，修正后重试 | 预留 |
| `BlockedTypeError` | `[ERROR]` | 一般操作失败，可换方式重试 | 预留/兜底 |

**ToolResult 改动**：

```go
type ToolResult struct {
    // ... 现有字段不变 ...
    BlockedType string `json:"blocked_type,omitempty"`
}

func (tr *ToolResult) WithBlockedType(t string) *ToolResult {
    tr.BlockedType = t
    return tr
}

func (tr *ToolResult) ContentForLLM() string {
    // ... 现有逻辑 ...
    if tr.BlockedType != "" {
        prefix := "[ERROR] "
        switch tr.BlockedType {
        case BlockedTypeSafety: prefix = "[BLOCKED] "
        case BlockedTypeDenied: prefix = "[DENIED] "
        case BlockedTypeInvalid: prefix = "[INVALID] "
        }
        content = prefix + content
    }
    // ... 返回 ...
}
```

**调用点改动示例**（shell.go guardCommand）：

```go
// 原来：
return "Command blocked by safety guard (dangerous pattern detected)"

// 改为：
return ErrorResult("Command blocked by safety guard (dangerous pattern detected)").
    WithBlockedType(BlockedTypeSafety)
```

**改动量**：~40 行

### 联动关系

阶段二的 `[BLOCKED]` / `[DENIED]` 前缀让 LLM 能直接匹配阶段一 prompt 中的规则，不再依赖字符串模糊匹配。两阶段叠加效果：

1. LLM 立即识别 `[BLOCKED]` → 不重试
2. 连续 3 次任意失败 → 主动告知用户
3. 同参数重复拦截 → 系统 prompt 禁止再试

## 实施策略

**严格分三段，每段观察效果**：

1. 先上阶段一（prompt 注入），部署到 RK3506，跑 session 验证 LLM 是否在 3 次失败后停止
2. 再上阶段二（错误标记），验证 `[BLOCKED]` 前缀是否让 LLM 更早停止（减少无效重试）
3. 两阶段结合，对比最终效果：从 42 次降到多少次

## 测试策略

- 阶段一：`go test ./pkg/agent/...` — 验证 prompt part 正确注入、不破坏现有 prompt 结构
- 阶段二：`go test ./pkg/tools/shared/...` — 验证 ContentForLLM 前缀正确；`go test ./pkg/tools/...` — 验证调用点不破坏现有行为
- 集成验证：部署到 RK3506，执行 `ls /etc` 类被拦截命令，观察 session 日志中 tool iteration 次数
