# Tool Result 错误分类与熔断

> 创建: 2026-06-04
> 状态: 待实现
> 来源: RK3506 session 日志分析 — Agent 42 次工具调用全被 safety guard 拦截，不停止、不告知用户

## 问题

Agent 在执行任务时，工具调用被 safety guard / workspace 边界反复拦截，但从未告知用户"我被限制了"，直到 `max_tool_iterations` 耗尽才失败。根因有两个：

1. LLM 不区分"系统性限制"和"可重试错误"——看到错误就换参数再试
2. Tool Result 没有结构化分类——LLM 只能靠字符串模糊匹配判断错误类型

## 方案

### 阶段一：提示词注入 + 宿主侧硬中断

两项改动同时上线，互为保底：

**A. System prompt 注入熔断规则**

在 system prompt 的 `PromptLayerCapability / PromptSlotTooling` 中注入工具失败处理规则，注册新 `PromptSource` `tool:guardrails`。

Prompt 内容与现有 system prompt 整体语言保持一致（英文）：

```
# TOOL FAILURE RULES (CRITICAL)

Tool results may be prefixed with error classifications:
- [BLOCKED] — command blocked by safety policy. DO NOT retry or bypass.
- [DENIED] — path outside workspace or permission denied. You MAY retry with a path inside the workspace.

Rules:
1. If you receive [BLOCKED], stop immediately. Do not try alternative commands, encoding tricks, or path variants. Tell the user what was blocked and why.
2. If you receive [DENIED], you may retry ONCE with a workspace-internal path. If that also fails, stop and tell the user.
3. After 3 consecutive [BLOCKED] or [DENIED] results, stop immediately. Tell the user: "I'm unable to complete this task because of safety restrictions."
4. Normal errors (file not found, invalid arguments) do NOT count toward the 3-failure limit — only [BLOCKED] and [DENIED] count.
```

**B. 宿主侧连续拦截计数器**

在 `turnState` 中增加 `consecutiveBlockedCount int`，每次工具返回 `BlockedType` 非空时 +1，达到 5 次强制 `requestHardAbort()`。提示词 + 硬中断双重保底——LLM 自律和宿主强制同时生效。

**改动文件**：
- `pkg/agent/prompt.go` — 注册 `PromptSourceToolGuard`
- `pkg/agent/context.go` — 追加 prompt part
- `pkg/agent/turn_state.go` — 新增 `consecutiveBlockedCount` 字段 + `recordBlockedResult()`
- `pkg/agent/pipeline_execute.go` — 工具执行后读 `BlockedType`，调用 `ts.recordBlockedResult()`，>=5 时 `requestHardAbort()`

**改动量**：~40 行

### 阶段二：ToolResult 结构化错误标记

#### BlockedType 枚举

只定义两个当前需要的常量（YAGNI）：

| 常量 | 前缀 | 语义 | 来源 |
|------|------|------|------|
| `BlockedTypeSafety` | `[BLOCKED]` | 安全策略拦截，不可重试 | shell guardCommand |
| `BlockedTypeDenied` | `[DENIED]` | 权限/边界不足 | filesystem workspace 检查 |

#### result.go 改动

```go
const (
    BlockedTypeSafety = "BLOCKED"
    BlockedTypeDenied = "DENIED"
)

type ToolResult struct {
    // ... 现有字段不变 ...
    BlockedType string `json:"blocked_type,omitempty"` // 空 = 正常
}

func (tr *ToolResult) WithBlockedType(t string) *ToolResult {
    tr.BlockedType = t
    return tr
}
```

**ContentForLLM 拼装顺序**（明确说明以消除实现歧义）：

```
[PREFIX] body                  ← 错误分类前缀在最前
\nHandledToolLLMNote           ← ResponseHandled 时追加（错误时通常不发）
\n[file:/path/to/artifact]     ← ArtifactTags 注释（错误时通常无 artifact）
```

即错误发生在先，前缀在最外层，避免被后续 note 包裹。

#### shell.go 改动

`guardCommand` 返回类型从 `string` 改为 `(string, string)`：`(blockedType, errorMsg)`，`BlockedTypeSafety` 即拦截原因。调用方一处改为：

```go
if blockedType, guardError := t.guardCommand(command, cwd); guardError != "" {
    return ErrorResult(guardError).WithBlockedType(blockedType)
}
```

guardCommand 内部 4 处返回改为 `return BlockedTypeSafety, "Command blocked by..."`。另外 3 处直接返回 `*ToolResult` 的点（332/356/368）直接加 `.WithBlockedType(BlockedTypeSafety)`。

#### filesystem.go 改动

定义 sentinel error（防字符串匹配）：

```go
var ErrAccessDenied = errors.New("access denied")
```

**resolvePath**（3 处：69/80/86）改用：

```go
return "", fmt.Errorf("access denied: path is outside the workspace: %w", ErrAccessDenied)
return "", fmt.Errorf("access denied: symlink resolves outside workspace: %w", ErrAccessDenied)
```

**hostFs**（4 处：1015/1039/1081/1167）改用：

```go
return nil, fmt.Errorf("failed to read file: access denied: %w", ErrAccessDenied)
return fmt.Errorf("failed to read file: access denied: %w", ErrAccessDenied)
```

**Execute 层**新增 helper，在 `ErrorResult(err.Error())` 调用点替换：

```go
func errorResultFromFS(err error) *ToolResult {
    if errors.Is(err, ErrAccessDenied) {
        return ErrorResult(err.Error()).WithBlockedType(BlockedTypeDenied)
    }
    return ErrorResult(err.Error())
}
```

替换 Execute 方法中所有捕获 fs 错误的 `ErrorResult(err.Error())` 为 `errorResultFromFS(err)`（约 5 处：ReadFileTool 一处、ReadFileLinesTool 两处、WriteFileTool 一处、ListDirTool 一处）。

> **兼容性验证**：evolution / seahorse 均不消费 ToolResult 的序列化字段，seahorse 只通过 `tools.NewToolResult()` 构造（不涉及 BlockedType）。`omitempty` 保证旧 session JSONL 回放不受影响。

**改动文件**：
- `pkg/tools/shared/result.go` — BlockedType 字段 + 常量 + `WithBlockedType()` + ContentForLLM 前缀拼装
- `pkg/tools/shell.go` — guardCommand 签名改为 `(string, string)` + 7 处返回点
- `pkg/tools/fs/filesystem.go` — sentinel error + resolvePath/hostFs 7 处 + Execute 层 5 处 helper 调用

**改动量**：~60 行

### 联动关系

阶段二的 `[BLOCKED]` / `[DENIED]` 前缀让 LLM 精准匹配阶段一 prompt 规则。阶段一的宿主计数器按 BlockedType 触发硬中断。两阶段 + 硬中断三层保底：

| 机制 | 触发条件 | 效果 |
|------|----------|------|
| LLM 自律 | 看到 `[BLOCKED]` / `[DENIED]` 前缀 | 主动停止，告知用户 |
| 宿主计数器 | BlockedType 连续 5 次 | 强制硬中断 |
| max_tool_iterations | 原有兜底 | 最终兜底 |

### 提示词语义对齐

| 拦截场景 | BlockedType | LLM 行为 |
|----------|-------------|----------|
| dangerous pattern / not in allowlist | `[BLOCKED]` | 不重试，告知用户 |
| path traversal / path outside workspace | `[BLOCKED]` | 不重试，告知用户 |
| workspace 外路径访问 | `[DENIED]` | 允许在 workspace 内重试一次，再失败则停 |

## 实施策略

严格分三段，每段部署到 RK3506 观察效果：

1. **阶段一**（prompt + 宿主计数器）→ 验证连续失败是否从 42 次降到 <5 次
2. **阶段二**（结构化标记）→ 验证 LLM 是否在看到 `[BLOCKED]` 前缀后立即停止（不等到 5 次硬中断）
3. **两阶段结合** → 对比效果，理想情况 LLM 在 1-3 次 `[BLOCKED]` 后自行停止

## 成功指标

- 阶段一上线后，被拦截的 session 中 tool iteration ≤ 5（宿主计数器硬限制）
- 阶段二上线后，被拦截的 session 中 tool iteration ≤ 3（LLM 在看到 `[BLOCKED]` 后自行停止）
- 正常探索（file-not-found 等）不受影响：普通 `IsError` 不计入计数器
- JSONL 中 BlockedType 字段可审计：`grep -o '"blocked_type":"..."' sessions/*.jsonl | sort | uniq -c`

## 测试策略

- `go test ./pkg/agent/...` — prompt part 注入 + 计数器逻辑
- `go test ./pkg/tools/shared/...` — ContentForLLM 前缀拼装顺序
- `go test ./pkg/tools/...` — sentinel error 传播 + guardCommand 新签名
- `go test ./pkg/tools/fs/...` — errorResultFromFS 分类正确
- 集成：部署 RK3506，`ls /etc`、`cat /proc/cpuinfo` 等被拦截命令，观察 session 日志 iteration 次数
