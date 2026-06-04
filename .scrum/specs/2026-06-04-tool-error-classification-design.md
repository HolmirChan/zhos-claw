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

两项改动同时上线，互为保底。

#### A. System prompt 注入熔断规则

在 system prompt 的 `PromptLayerCapability / PromptSlotTooling` 中注入工具失败处理规则，注册新 `PromptSource` `tool:guardrails`。

Prompt 内容与现有 system prompt 整体语言保持一致（英文）：

```
# TOOL FAILURE RULES (CRITICAL)

Tool results may be prefixed with error classifications:
- [BLOCKED] — command itself is prohibited (dangerous pattern, not in allowlist, explicit path traversal). DO NOT retry or bypass.
- [DENIED] — path or target is outside the allowed scope (workspace boundary, permission boundary). You may retry with an in-scope alternative.

Rules:
1. If you receive [BLOCKED], stop immediately. Do not try alternative commands, encoding tricks, or path variants. Tell the user what was blocked and why.
2. If you receive [DENIED], you may retry with a workspace-internal or in-scope path. If the retry also yields [DENIED], stop.
3. After 3 consecutive [BLOCKED] or [DENIED] results, stop immediately. Tell the user: "I'm unable to complete this task because of safety restrictions."
4. Normal errors (file not found, invalid arguments, OS permission denied on an in-workspace file) do NOT count toward the limit — only [BLOCKED] and [DENIED] count.
```

#### B. 宿主侧连续拦截计数器

在 `turnState` 中增加 `consecutiveBlockedCount int`，每次工具返回 `BlockedType` 非空时 +1。连续 5 次触发 `requestHardAbort()`。

**阈值设计**：LLM prompt 设 3 次自律停止，宿主设 5 次硬中断，中间留 2 轮缓冲供 LLM 完成"识别规则 → 输出告知用户"的环节。若 `MaxToolIterations < 5`，计数器无机会触发——此时 iteration limit 本身就是有效保底，无需额外处理。

**计数器作用域**：仅本 turn。SubAgent（子 turn）内的 `consecutiveBlockedCount` 不向上传播到父 turn——父 turn 看到的是子 turn 的正常完成/失败，不是父 turn 的直接工具失败。

**改动文件**：
- `pkg/agent/prompt.go` — 注册 `PromptSourceToolGuard`
- `pkg/agent/context.go` — 追加 prompt part
- `pkg/agent/turn_state.go` — 新增 `consecutiveBlockedCount` 字段 + `recordBlockedResult()`
- `pkg/agent/pipeline_execute.go` — 工具执行后读 `BlockedType`，调用 `ts.recordBlockedResult()`，>=5 时 `requestHardAbort()`

**改动量**：~40 行

### 阶段二：ToolResult 结构化错误标记

#### BlockedType 分类：按重试语义，不按实现位置

| 常量 | 值 | 前缀 | 语义 | 判定标准 |
|------|-----|------|------|----------|
| `BlockedTypeBlocked` | `"BLOCKED"` | `[BLOCKED]` | 命令/操作本身被禁 | 危险的命令模式、不在白名单、显式路径逃逸 `../../` |
| `BlockedTypeDenied` | `"DENIED"` | `[DENIED]` | 目标在允许范围外 | workspace 边界、路径越界 |

两类在不同工具间统一使用，不以"是 shell 还是 fs 报的"区分。

#### result.go 改动

```go
const (
    BlockedTypeBlocked = "BLOCKED"
    BlockedTypeDenied  = "DENIED"
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

**ContentForLLM 拼装顺序**：

```
[PREFIX] body                  ← 错误分类前缀在最前
\nHandledToolLLMNote           ← ResponseHandled 时追加（错误结果通常不触发）
\n[file:/path/to/artifact]     ← ArtifactTags 注释（错误结果通常无 artifact）
```

错误分类前缀在最外层，不被后续 note 包裹。`IsError` 结果通常不含 Media/ArtifactTags，但仍明确顺序以消除歧义。

#### shell.go 改动

`guardCommand` 返回值从 `string` 改为 `(string, string)`：`(blockedType, guardError)`。

| 位置 | 拦截原因 | 分类 |
|------|----------|------|
| 1088 | dangerous pattern detected | `BlockedTypeBlocked` |
| 1102 | not in allowlist | `BlockedTypeBlocked` |
| 1109 | path traversal (`../../`) detected | `BlockedTypeBlocked` |
| 1192 | path outside working dir | **`BlockedTypeDenied`** |
| 332 | cwd 验证失败（本质 workspace 边界） | `BlockedTypeDenied` |
| 356 | path resolution failed（换路径可能成功） | `BlockedTypeDenied` |
| 368 | working directory escaped workspace | `BlockedTypeDenied` |

调用方（347 行）改为：

```go
blockedType, guardError := t.guardCommand(command, cwd)
if guardError != "" {
    return ErrorResult(guardError).WithBlockedType(blockedType)
}
```

#### filesystem.go 改动

**sentinel**：

```go
var ErrAccessDenied = errors.New("access denied")
```

`ErrAccessDenied` 只挂到 **workspace 边界** 语义上。OS 权限错误（`os.IsPermission`）不挂——文件已在 workspace 内但 mode 000 或属主不同，LLM 应作为普通失败处理，重试也没用。

**resolvePath**（3 处：69/80/86）：

```go
return "", fmt.Errorf("access denied: path is outside the workspace: %w", ErrAccessDenied)
return "", fmt.Errorf("access denied: symlink resolves outside workspace: %w", ErrAccessDenied)
```

**sandboxFs**（1081/1167 行）：需要拆分分支。当前代码：

```go
if os.IsPermission(err) || strings.Contains(err.Error(), "escapes from parent") || ... {
    return fmt.Errorf("failed to read file: access denied: %w", err)
}
```

改为：

```go
// escapes from parent = workspace boundary → sentinel
if strings.Contains(err.Error(), "escapes from parent") {
    return fmt.Errorf("failed to read file: access denied: %w", ErrAccessDenied)
}
// OS permission denied → no sentinel, normal error
if os.IsPermission(err) {
    return fmt.Errorf("failed to read file: access denied: %w", err)
}
```

**hostFs**（1015/1039 行）：保持原样，**不加** `ErrAccessDenied`。——`os.IsPermission` 是 OS 权限问题，不是 workspace 边界。

**Execute 层 helper**：

```go
func errorResultFromFS(err error) *ToolResult {
    if errors.Is(err, ErrAccessDenied) {
        return ErrorResult(err.Error()).WithBlockedType(BlockedTypeDenied)
    }
    return ErrorResult(err.Error())
}
```

替换清单（6 处，按文件顺序）：

| 文件 | 行 | 上下文 | 错误来源 |
|------|-----|------|----------|
| `edit.go` | 74 | `EditFileTool` `editFile(t.fs, path, ...)` | `sysFs.ReadFile` → resolvePath |
| `edit.go` | 128 | `AppendFileTool` `appendFile(t.fs, path, content)` | `sysFs.WriteFile` → resolvePath |
| `filesystem.go` | 424 | `ReadFileTool` `t.fs.Open(path)` | resolvePath / sandboxFs |
| `filesystem.go` | 572 | `ReadFileLinesTool` `t.fs.Open(path)` | resolvePath / sandboxFs |
| `filesystem.go` | 932 | `WriteFileTool` `t.fs.WriteFile(path, data)` | resolvePath / sandboxFs |
| `filesystem.go` | 979 | `ListDirTool` `t.fs.ReadDir(path)` | resolvePath / sandboxFs |

**不走 helper 的**（保持原样）：filesystem.go 404/413/544/563 — 均为 `getInt64Arg` 参数解析错误，与 fs 无关。

**改动文件**：
- `pkg/tools/shared/result.go` — BlockedType 字段 + 常量 + `WithBlockedType()` + ContentForLLM 前缀拼装
- `pkg/tools/shell.go` — guardCommand 签名改为 `(string, string)` + 7 处返回点
- `pkg/tools/fs/filesystem.go` — `ErrAccessDenied` sentinel + resolvePath 3 处 + sandboxFs 拆分 + helper + 6 处 Execute 层替换

**改动量**：~75 行

**兼容性**：evolution / seahorse 不消费 ToolResult 序列化字段。`omitempty` 保证旧 session JSONL 回放不受影响。

### 联动关系

| 机制 | 触发条件 | 效果 |
|------|----------|------|
| LLM 自律 | 看到 `[BLOCKED]` / `[DENIED]` 前缀 | 按规则停止或换路径重试一次 |
| 宿主计数器 | BlockedType 连续 5 次 | 强制硬中断（保底） |
| max_tool_iterations | 原有兜底 | 最终兜底 |

### 分类对齐总表

| 拦截场景 | 工具 | 分类 | LLM 行为 |
|----------|------|------|----------|
| dangerous pattern（如 `rm -rf /`） | shell | `[BLOCKED]` | 不重试，告知用户 |
| not in allowlist | shell | `[BLOCKED]` | 不重试，告知用户 |
| path traversal（`../../`） | shell | `[BLOCKED]` | 不重试，告知用户 |
| path outside working dir | shell | `[DENIED]` | 可换 workspace 内路径重试一次 |
| cwd outside workspace | shell | `[DENIED]` | 可换 workspace 内路径重试一次 |
| path outside workspace | fs | `[DENIED]` | 可换 workspace 内路径重试一次 |
| chmod 000 文件（OS 权限） | fs | 无（普通 IsError） | 当作常规失败，不计数 |
| file not found | fs | 无（普通 IsError） | 不计数（正常探索） |

## 实施策略

严格分三段，每段部署到 RK3506 观察效果：

1. **阶段一**（prompt + 宿主计数器）→ 验证被拦截 session 中 tool iteration ≤ 5
2. **阶段二**（结构化标记）→ 验证 LLM 在看到 `[BLOCKED]` 前缀后 1-2 次自行停止，不等硬中断
3. **两阶段结合** → 理想：LLM ≤ 3 次自律停止；保底：宿主 ≤ 5 次硬中断

## 成功指标

- 阶段一上线：被拦截 session 中 tool iteration ≤ 5（宿主计数器硬限制）
- 阶段二上线：被拦截 session 中 tool iteration ≤ 3（LLM 自律停止）
- 正常探索（file-not-found 等）不受影响
- JSONL 可审计：`grep -o '"blocked_type":"[^"]*"' sessions/*.jsonl | sort | uniq -c`

## 测试策略

- `go test ./pkg/agent/...` — prompt part 注入 + 计数器逻辑 + 计数器不上溯父 turn
- `go test ./pkg/tools/shared/...` — ContentForLLM 前缀拼装顺序
- `go test ./pkg/tools/...` — sentinel error 传播 + guardCommand 签名 + errorResultFromFS 覆盖全 6 处
- `go test ./pkg/tools/fs/...` — errorResultFromFS 分类正确 + OS 权限错误不走 sentinel
- 集成：部署 RK3506，执行 `ls /etc`、`cat /proc/cpuinfo` 等混合拦截场景
