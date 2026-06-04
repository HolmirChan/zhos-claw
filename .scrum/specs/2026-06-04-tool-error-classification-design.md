# Tool Result 错误分类与熔断

> 创建: 2026-06-04
> 修订: 2026-06-04 (r5)
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
- [DENIED] — path or target is outside the allowed scope (workspace boundary). You may retry with an in-scope path.

Rules:
1. If you receive [BLOCKED], stop immediately. Do not try alternative commands or encoding tricks.
2. If you receive [DENIED], you may retry with a workspace-internal or in-scope path. If the retry also yields [DENIED], stop.
3. After 3 consecutive [BLOCKED] or [DENIED] results, stop immediately. Tell the user: "I'm unable to complete this task because of safety restrictions."
4. Normal errors (file not found, invalid arguments, OS permission denied on an in-workspace file) do NOT count toward the limit — only [BLOCKED] and [DENIED] count.
```

#### B. 宿主侧连续拦截计数器

在 `turnState` 中增加 `consecutiveBlockedCount int` + 方法 `recordToolResult(tr *ToolResult)`：

```go
func (ts *turnState) recordToolResult(tr *ToolResult) {
    // 防御：hard abort 后不再累加，避免 abort 后日志出现 count=6,7,8...
    if ts.hardAbortRequested() {
        return
    }
    if tr != nil && tr.BlockedType != "" {
        ts.consecutiveBlockedCount++
    } else {
        // BlockedType==""（成功或普通错误）→ 清零，保持"连续"语义
        ts.consecutiveBlockedCount = 0
    }
    if ts.consecutiveBlockedCount >= 5 {
        ts.requestHardAbort()
    }
}
```

**关键语义**：非 BlockedType 结果（含普通 IsError 如 file-not-found）**清零**计数器——这是"真·连续"，不是"累计"。

**阈值设计**：LLM prompt 设 3 次自律停止，宿主设 5 次硬中断，中间留 2 轮缓冲供 LLM 完成"识别规则 → 输出告知用户"。若 `MaxToolIterations < 5`，计数器无机会触发——此时 iteration limit 本身就是有效保底。

**计数器作用域**：仅本 turn。SubAgent（子 turn）内的计数器不向上传播到父 turn。子 turn 返回给父 turn 的最终 ToolResult **不携带**子 turn 的 BlockedType——在 `pkg/tools/spawn.go` 的 async goroutine 回调前主动清空：

```go
// spawn.go ~136-143: async goroutine 完成子 turn 后，回调前清空 BlockedType
if result != nil {
    result.BlockedType = "" // 子 turn 的边界拦截不传播给父 turn 计数器
}
if cb != nil {
    cb(ctx, result)
}
```

**改动文件**：
- `pkg/agent/prompt.go` — 注册 `PromptSourceToolGuard`
- `pkg/agent/context.go` — 追加 prompt part
- `pkg/agent/turn_state.go` — 新增字段 + `recordToolResult(*ToolResult)`
- `pkg/agent/pipeline_execute.go` — 三处调用 `ts.recordToolResult(toolResult)`：
  - 同步执行路径：toolResult 已完成 hook 后处理、尚未 `messages = append` 之前
  - Hook respond 路径：hookResult 确定后、messages append 之前
  - Async 回调路径：async 工具独立回流，防御未来新增 async 工具产生 BlockedType（当前 spawn_agent 等不直接产生，但加调用完备对称）
- `pkg/tools/spawn.go` — SubAgent async goroutine 回调前清空 `result.BlockedType`

**改动量**：~55 行

### 阶段二：ToolResult 结构化错误标记

#### BlockedType 分类：按重试语义

| 常量 | 值 | 前缀 | 语义 |
|------|-----|------|------|
| `BlockedTypeBlocked` | `"BLOCKED"` | `[BLOCKED]` | 命令/操作本身被禁，不可重试 |
| `BlockedTypeDenied` | `"DENIED"` | `[DENIED]` | 目标在允许范围外，可换 scope 内路径重试 |

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

**ContentForLLM 拼装顺序**：`[PREFIX] body \n HandledNote \n ArtifactNote`。前缀仅当 `BlockedType != ""` 且 `content != ""` 时追加（避免输出裸 `[BLOCKED] `）。BlockedType 必须与非空 ForLLM 一起设置（约定）。

#### shell.go 改动

`guardCommand` 签名从 `string` 改为 `(string, string)`：`(blockedType, errorMsg)`。

| 位置 | 拦截原因 | 分类 |
|------|----------|------|
| 1088 | dangerous pattern detected | `BlockedTypeBlocked` |
| 1102 | not in allowlist | `BlockedTypeBlocked` |
| 1109 | path traversal (`../../`) | `BlockedTypeBlocked` |
| 1192 | path outside working dir | `BlockedTypeDenied` |
| 332 | cwd 验证失败 | `BlockedTypeDenied` |
| 356 | path resolution failed | `BlockedTypeDenied` |
| 368 | working directory escaped workspace | `BlockedTypeDenied` |

#### filesystem.go 改动

**Sentinel**（名与值分离：内部识别用 `ErrWorkspaceBoundary`，人类文案保持 "access denied"）：

```go
var ErrWorkspaceBoundary = errors.New("workspace boundary")
```

**挂 sentinel 的 6 个点**（resolvePath + getSafeRelPath + sandboxFs）：

| 位置 | 函数 | 改动 |
|------|------|------|
| `resolvePath:69` | `validatePathWithAllowPaths` | `fmt.Errorf("access denied: path is outside the workspace: %w", ErrWorkspaceBoundary)` |
| `resolvePath:80` | `validatePathWithAllowPaths` | `fmt.Errorf("access denied: symlink resolves outside workspace: %w", ErrWorkspaceBoundary)` |
| `resolvePath:86` | `validatePathWithAllowPaths` | `fmt.Errorf("access denied: symlink resolves outside workspace: %w", ErrWorkspaceBoundary)` |
| `getSafeRelPath:1246` | `getSafeRelPath` | `fmt.Errorf("path escapes workspace: %s: %w", path, ErrWorkspaceBoundary)` |
| `sandboxFs.ReadFile:1079` | `sandboxFs` | escapes from parent 分支 → `%w ErrWorkspaceBoundary` |
| `sandboxFs.Open:1167` | `sandboxFs` | escapes from parent 分支 → `%w ErrWorkspaceBoundary` |

**sandboxFs 拆分**（1079/1167 行）：原来 `os.IsPermission || escapes from parent` 合并，改为：

```go
if strings.Contains(err.Error(), "escapes from parent") {
    return fmt.Errorf("failed to read file: access denied: %w", ErrWorkspaceBoundary)
}
if os.IsPermission(err) {
    return fmt.Errorf("failed to read file: access denied: %w", err) // OS 权限，不挂 sentinel
}
```

**hostFs**（1015/1039）：不改。`os.IsPermission` 是 OS 权限问题，不挂 sentinel。另将 hostFs 中 OS 权限分支的文案从 "access denied" 改为 "permission denied"，降低与 `[DENIED]` 分类的视觉相似度，避免 LLM 误判。

**Execute 层 helper**：

```go
// errorResultFromFS wraps an error into an ErrorResult, attaching BlockedTypeDenied
// when the error is (or wraps) ErrWorkspaceBoundary.
func errorResultFromFS(err error) *ToolResult {
    if errors.Is(err, ErrWorkspaceBoundary) {
        return ErrorResult(err.Error()).WithBlockedType(BlockedTypeDenied)
    }
    return ErrorResult(err.Error())
}

// errorResultFromFSCtx is like errorResultFromFS but prepends context.
// For call sites that currently use fmt.Sprintf to add context before the error.
func errorResultFromFSCtx(context string, err error) *ToolResult {
    msg := fmt.Sprintf("%s: %v", context, err)
    if errors.Is(err, ErrWorkspaceBoundary) {
        return ErrorResult(msg).WithBlockedType(BlockedTypeDenied)
    }
    return ErrorResult(msg)
}
```

**Execute 层替换清单**：

| 文件:行 | 工具 | 错误来源 | helper |
|---------|------|----------|--------|
| `edit.go:74` | EditFileTool | `editFile → sysFs.ReadFile` | `errorResultFromFS(err)` |
| `edit.go:128` | AppendFileTool | `appendFile → sysFs.WriteFile` | `errorResultFromFS(err)` |
| `filesystem.go:424` | ReadFileTool | `t.fs.Open → sandboxFs → execute → getSafeRelPath` | `errorResultFromFS(err)` |
| `filesystem.go:572` | ReadFileLinesTool | `t.fs.Open → sandboxFs` | `errorResultFromFS(err)` |
| `filesystem.go:932` | WriteFileTool | `t.fs.WriteFile → sandboxFs` | `errorResultFromFS(err)` |
| `filesystem.go:979` | ListDirTool | `t.fs.ReadDir → sandboxFs` | `errorResultFromFSCtx("failed to read directory", err)` |

**不走 helper**：filesystem.go 404/413/544/563 — `getInt64Arg` 参数解析错误，与 fs 无关。

**改动文件**：
- `pkg/tools/shared/result.go` — 常量 + 字段 + `WithBlockedType()` + ContentForLLM 前缀
- `pkg/tools/shell.go` — guardCommand 签名 + 7 处返回点
- `pkg/tools/fs/filesystem.go` — `ErrWorkspaceBoundary` sentinel + resolvePath 3 处 + getSafeRelPath 1 处 + sandboxFs 2 处 + helper ×2 + Execute 层 6 处

**改动量**：~85 行

**兼容性**：evolution / seahorse 不消费 ToolResult 序列化字段。`omitempty` 保证旧 session JSONL 回放不受影响。

### 联动关系

| 机制 | 触发条件 | 效果 |
|------|----------|------|
| LLM 自律 | 看到 `[BLOCKED]` / `[DENIED]` 前缀 | 按规则停止或换 scope 内路径重试 |
| 宿主计数器 | BlockedType 连续 5 次 | 硬中断保底；遇成功/普通错误清零 |
| max_tool_iterations | 原有兜底 | 最终兜底 |

### 分类对齐总表

| 拦截场景 | 工具 | 分类 | LLM 行为 |
|----------|------|------|----------|
| dangerous pattern | shell | `[BLOCKED]` | 不重试，告知用户 |
| not in allowlist | shell | `[BLOCKED]` | 不重试，告知用户 |
| path traversal（`../../`） | shell | `[BLOCKED]` | 不重试，告知用户 |
| path outside working dir | shell | `[DENIED]` | 可换 workspace 内路径重试 |
| cwd outside workspace | shell | `[DENIED]` | 可换 workspace 内路径重试 |
| path outside workspace | fs | `[DENIED]` | 可换 workspace 内路径重试 |
| path escapes workspace | fs (sandboxFs) | `[DENIED]` | 可换 workspace 内路径重试 |
| chmod 000 / OS 权限 | fs | 无（普通 IsError） | 当作常规失败，不计数 |
| file not found | fs | 无（普通 IsError） | 不计数（正常探索） |

## 实施策略

严格分三段，每段部署到 RK3506 观察效果：

1. **阶段一**（prompt + 宿主计数器）→ 验证被拦截 session 中 tool iteration ≤ 5
2. **阶段二**（结构化标记）→ 验证 LLM 在看到 `[BLOCKED]` 前缀后 1-2 次自行停止，不等硬中断
3. **两阶段结合** → 理想：LLM ≤ 3 次自律停止；保底：宿主 ≤ 5 次硬中断

## 成功指标

- 阶段一上线：被拦截 session 中 tool iteration ≤ 5（宿主计数器硬限制）
- 阶段二上线：被拦截 session 中 tool iteration ≤ 3（LLM 自律停止）
- 正常探索（file-not-found 等）不受影响（计数器清零机制保证）
- JSONL 可审计：`grep -o '"blocked_type":"[^"]*"' sessions/*.jsonl | sort | uniq -c`

## 测试策略

- `go test ./pkg/agent/...` — prompt part 注入 + 计数器逻辑 + 成功时清零 + 计数器不上溯父 turn + abort 后不累加 + async 回调路径计数
- `go test ./pkg/tools/shared/...` — ContentForLLM 前缀拼装顺序
- `go test ./pkg/tools/...` — ErrWorkspaceBoundary 传播 + guardCommand 签名 + errorResultFromFS/errorResultFromFSCtx 覆盖全 6 处
- `go test ./pkg/tools/fs/...` — helper 分类正确 + OS 权限错误不挂 sentinel + getSafeRelPath 挂 sentinel
- 集成：部署 RK3506，执行 `ls /etc`、`cat /proc/cpuinfo`、`read_file /etc/passwd` 等混合拦截场景
