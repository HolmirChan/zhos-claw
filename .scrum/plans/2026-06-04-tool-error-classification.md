# Tool Result 错误分类与熔断 — 实现计划

> **For agentic workers:** 使用 superpowers:subagent-driven-development 或 superpowers:executing-plans 按任务逐个实现。
> 设计文档: .scrum/specs/2026-06-04-tool-error-classification-design.md
> 分支: fix-调用工具被safety-guard拦截不停止的问题

**目标:** 让 Agent 工具调用被 safety guard / workspace 边界拦截时主动停止并告知用户，解决 RK3506 上 42 次重复拦截不停止的问题。

**架构:** 两层保底——LLM 侧 system prompt 注入熔断规则（3 次自律停止）+ 宿主侧连续拦截计数器（5 次硬中断）。ToolResult 新增 BlockedType 字段让 LLM 精准识别错误类型。

**技术栈:** Go 标准库 (errors, fmt, strings)，无外部依赖。

---

## 阶段一：提示词注入 + 宿主侧硬中断

### Task 1: 注册 PromptSourceToolGuard 常量

**文件:**
- 修改: `pkg/agent/prompt.go`

- [ ] **Step 1: 添加常量定义**

在 `pkg/agent/prompt.go` 的 `PromptSourceID` 常量块末尾（`PromptSourceInterrupt` 之后）追加：

```go
PromptSourceToolGuard PromptSourceID = "tool:guardrails"
```

- [ ] **Step 2: 注册 PromptSource 描述符**

在 `defaultPromptRegistry()` 函数末尾 `PromptSourceInterrupt` 条目之后、`}` 闭合之前追加：

```go
{
    ID:              PromptSourceToolGuard,
    Owner:           "agent",
    Description:     "Tool failure guardrails — when to stop retrying blocked/denied tools",
    Allowed:         []PromptPlacement{{Layer: PromptLayerCapability, Slot: PromptSlotTooling}},
    StableByDefault: true,
},
```

- [ ] **Step 3: 编译验证**

```bash
go build -tags goolm,stdjson ./pkg/agent/...
```

- [ ] **Step 4: 提交**

```bash
git add pkg/agent/prompt.go
git commit -m "feat: 注册 PromptSourceToolGuard 常量

阶段一：LLM 侧 prompt 注入的准备——注册 tool:guardrails 源及其 placement。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 2: 注入 system prompt 熔断规则

**文件:**
- 修改: `pkg/agent/context.go`

- [ ] **Step 1: 在 BuildSystemPromptParts 中追加 prompt part**

在 `splitOnMarker` 条件块之后、`stack.Seal()` 之前（约 274 行），追加：

```go
// Tool failure guardrails
add(PromptPart{
    ID:     "capability.tool_guardrails",
    Layer:  PromptLayerCapability,
    Slot:   PromptSlotTooling,
    Source: PromptSource{ID: PromptSourceToolGuard, Name: "guardrails"},
    Title:  "tool failure rules",
    Content: `# TOOL FAILURE RULES (CRITICAL)

Tool results may be prefixed with error classifications:
- [BLOCKED] — command itself is prohibited (dangerous pattern, not in allowlist, explicit path traversal). DO NOT retry or bypass.
- [DENIED] — path or target is outside the allowed scope (workspace boundary). You may retry with an in-scope path.

Rules:
1. If you receive [BLOCKED], stop immediately. Do not try alternative commands or encoding tricks.
2. If you receive [DENIED], you may retry with a workspace-internal or in-scope path. If the retry also yields [DENIED], stop.
3. After 3 consecutive [BLOCKED] or [DENIED] results, stop immediately. Tell the user: "I'm unable to complete this task because of safety restrictions."
4. Normal errors (file not found, invalid arguments, OS permission denied on an in-workspace file) do NOT count toward the limit — only [BLOCKED] and [DENIED] count.`,
    Stable: true,
    Cache:  PromptCacheEphemeral,
})
```

- [ ] **Step 2: 运行现有测试验证 prompt 结构未被破坏**

```bash
go test -tags goolm,stdjson ./pkg/agent/ -run "TestBuild" -v
```

- [ ] **Step 3: 全量编译**

```bash
go build -tags goolm,stdjson ./pkg/agent/...
```

- [ ] **Step 4: 提交**

```bash
git add pkg/agent/context.go
git commit -m "feat: system prompt 注入工具失败熔断规则

阶段一：在 PromptLayerCapability/PromptSlotTooling 注入 [BLOCKED]/[DENIED] 分类规则。
连续 3 次停止告知用户；区分系统性限制与可重试错误。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 3: 添加 consecutiveBlockedCount 计数器 + recordToolResult 方法

**文件:**
- 修改: `pkg/agent/turn_state.go`

- [ ] **Step 1: 找到 turnState 结构体定义并添加字段**

在 `turnState` struct 中（`hardAbort` 字段附近）追加：

```go
// consecutiveBlockedCount tracks consecutive tools returning BlockedType != "".
// Reset to 0 on any non-blocked result (success or normal error). Reaching 5
// triggers a hard abort. SubTurn counters do NOT propagate to parent.
consecutiveBlockedCount int
```

- [ ] **Step 2: 在 hardAbortRequested 方法附近添加 recordToolResult 方法**

```go
// recordToolResult updates the consecutive blocked counter based on tool result.
// BlockedType non-empty → +1; BlockedType empty → reset to 0.
// Reaching 5 consecutive blocked results triggers a hard abort.
// No-op if hard abort already requested (defensive).
func (ts *turnState) recordToolResult(tr *ToolResult) {
    ts.mu.Lock()
    if ts.hardAbort {
        ts.mu.Unlock()
        return
    }
    if tr != nil && tr.BlockedType != "" {
        ts.consecutiveBlockedCount++
    } else {
        ts.consecutiveBlockedCount = 0
    }
    shouldAbort := ts.consecutiveBlockedCount >= 5
    ts.mu.Unlock()

    if shouldAbort {
        ts.requestHardAbort() // idempotent, self-locking — called outside our lock
    }
}
```

- [ ] **Step 3: 编译验证**

```bash
go build -tags goolm,stdjson ./pkg/agent/...
```

- [ ] **Step 4: 提交**

```bash
git add pkg/agent/turn_state.go
git commit -m "feat: turnState 新增 consecutiveBlockedCount 计数器

阶段一宿主侧：recordToolResult() 在 BlockedType 非空时 +1，
非 Blocked 结果（含普通错误）清零。连续 5 次在锁外调用 idempotent requestHardAbort。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 4: pipeline_execute.go 三处调用 recordToolResult

**文件:**
- 修改: `pkg/agent/pipeline_execute.go`

- [ ] **Step 1: 同步执行路径**

在 `ts.recordToolExecution(...)` 之后、`messages = append(messages, toolResultMsg)` 之前（约 673-680 行之间），添加：

```go
ts.recordToolResult(toolResult)
```

- [ ] **Step 2: Hook respond 路径**

在 `ts.recordToolExecution(...)` 之后、`messages = append(messages, toolResultMsg)` 之前（约 292-299 行之间），添加：

```go
ts.recordToolResult(hookResult)
```

- [ ] **Step 3: Async 回调路径**

在 `asyncCallback` 函数体内（约 477 行），`content := result.ContentForLLM()` 之前，添加：

```go
ts.recordToolResult(result)
```

- [ ] **Step 4: 编译验证**

```bash
go build -tags goolm,stdjson ./pkg/agent/...
```

- [ ] **Step 5: 提交**

```bash
git add pkg/agent/pipeline_execute.go
git commit -m "feat: pipeline_execute 三处调用 recordToolResult

同步执行、hook respond、async 回调三个路径均接入计数器。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 5: SubAgent 清空 BlockedType（防止子 turn 传播）

**文件:**
- 修改: `pkg/tools/spawn.go`

- [ ] **Step 1: 在 async goroutine 回调前清空 BlockedType**

在约 136-143 行，`cb != nil` 调用之前，添加：

```go
// 子 turn 的边界拦截不传播给父 turn 计数器
if result != nil {
    result.BlockedType = ""
}
if cb != nil {
    cb(ctx, result)
}
```

- [ ] **Step 2: 编译验证**

```bash
go build -tags goolm,stdjson ./pkg/tools/...
```

- [ ] **Step 3: 提交**

```bash
git add pkg/tools/spawn.go
git commit -m "feat: SubAgent 返回前清空 BlockedType 防父 turn 误计数

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 6: 阶段一单元测试

**文件:**
- 创建: `pkg/agent/turn_state_test.go`

测试文件 package 为 `agent`，需要 import `tools "github.com/sipeed/picoclaw/pkg/tools"` 用 `tools.ToolResult`。

- [ ] **Step 1: 写计数器清零测试**

以下三个测试函数追加到同一个文件 `pkg/agent/turn_state_test.go`。

```go
package agent

import (
    "testing"
    tools "github.com/sipeed/picoclaw/pkg/tools"
)

func TestRecordToolResult_ResetsOnNormalError(t *testing.T) {
    ts := &turnState{}
    // 测试只验证 BlockedType != "" 的语义，常量在 Task 8 引入后由分类测试覆盖
    ts.recordToolResult(&tools.ToolResult{BlockedType: "BLOCKED"})
    ts.recordToolResult(&tools.ToolResult{BlockedType: "DENIED"})
    ts.recordToolResult(&tools.ToolResult{BlockedType: "BLOCKED"})
    if ts.consecutiveBlockedCount != 3 {
        t.Fatalf("expected count=3 after 3 blocked, got %d", ts.consecutiveBlockedCount)
    }
    // Normal error resets
    ts.recordToolResult(&tools.ToolResult{IsError: true})
    if ts.consecutiveBlockedCount != 0 {
        t.Fatalf("expected count reset to 0 after normal error, got %d", ts.consecutiveBlockedCount)
    }
}
```

- [ ] **Step 2: 写硬中断测试（同文件追加）**

```go
func TestRecordToolResult_HardAbortsAt5(t *testing.T) {
    ts := &turnState{}
    // 用字面值避免对 Task 8 常量的编译期依赖
    for i := 0; i < 5; i++ {
        ts.recordToolResult(&tools.ToolResult{BlockedType: "BLOCKED"})
    }
    if !ts.hardAbortRequested() {
        t.Fatal("expected hard abort after 5 consecutive blocked results")
    }
}
```

- [ ] **Step 3: 写 abort 后不再累加测试（同文件追加）**

```go
func TestRecordToolResult_NoOpAfterHardAbort(t *testing.T) {
    ts := &turnState{}
    for i := 0; i < 5; i++ {
        ts.recordToolResult(&tools.ToolResult{BlockedType: "BLOCKED"})
    }
    ts.recordToolResult(&tools.ToolResult{BlockedType: "BLOCKED"})
    if ts.consecutiveBlockedCount != 5 {
        t.Fatalf("expected count=5 after abort, got %d", ts.consecutiveBlockedCount)
    }
}
```

- [ ] **Step 4: 运行测试**

```bash
go test -tags goolm,stdjson ./pkg/agent/ -run "TestRecordToolResult" -v
```

- [ ] **Step 5: 提交**

```bash
git add pkg/agent/turn_state_test.go
git commit -m "test: recordToolResult 计数器单元测试

覆盖：清零、硬中断、abort 后防御。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 7: 阶段一集成验证

- [ ] **Step 1: 全量测试**

```bash
go test -tags goolm,stdjson ./pkg/agent/... -v
```

- [ ] **Step 2: 全量构建**

```bash
make build && make build-launcher
```

- [ ] **Step 3: 部署到 RK3506，验证 counter 生效**

```bash
# 观察 session 日志中连续被拦截的 tool iteration <= 5
```

- [ ] **Step 4: 阶段一标记提交**

```bash
git add -A
git commit -m "feat: 阶段一完成 — prompt 熔断规则 + 宿主侧计数器

阶段一实现：LLM 侧 prompt 注入 [BLOCKED]/[DENIED] 分类规则，
宿主侧连续 5 次硬中断保底。SubAgent BlockedType 清空防传播。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

## 阶段二：ToolResult 结构化错误标记

### Task 8: result.go 新增 BlockedType + facade 常量 re-export

**文件:**
- 修改: `pkg/tools/shared/result.go`
- 修改: `pkg/tools/shared_facade.go`
- 修改: `pkg/tools/fs/shared.go`

- [ ] **Step 1: result.go 添加常量**

在 `pkg/tools/shared/result.go` 现有常量块后追加：

```go
const (
    BlockedTypeBlocked = "BLOCKED" // command prohibited, do not retry
    BlockedTypeDenied  = "DENIED"  // target out of scope, retry in-scope once
)
```

- [ ] **Step 2: result.go ToolResult 结构体添加字段**

在 `ToolResult` struct 中追加：

```go
// BlockedType indicates this result was blocked/denied. Empty = normal.
// BlockedTypeBlocked → systemic restriction, don't retry.
// BlockedTypeDenied → scope/boundary restriction, retry in-scope once.
BlockedType string `json:"blocked_type,omitempty"`
```

- [ ] **Step 3: result.go 添加 WithBlockedType 方法**

```go
func (tr *ToolResult) WithBlockedType(t string) *ToolResult {
    tr.BlockedType = t
    return tr
}
```

- [ ] **Step 4: result.go 修改 ContentForLLM 添加前缀**

在 `ContentForLLM()` 方法中，`if content == "" && tr.Err != nil` 段（约 73 行）**之后**、`if tr.ResponseHandled` 段（约 74 行）**之前**，插入：

```go
if tr.BlockedType != "" && content != "" {
    prefix := "[ERROR] "
    switch tr.BlockedType {
    case BlockedTypeBlocked:
        prefix = "[BLOCKED] "
    case BlockedTypeDenied:
        prefix = "[DENIED] "
    }
    content = prefix + content
}
```

- [ ] **Step 5: shared_facade.go 常量 re-export**

在 `pkg/tools/shared_facade.go` 的 `const` 块中追加：

```go
BlockedTypeBlocked = toolshared.BlockedTypeBlocked
BlockedTypeDenied  = toolshared.BlockedTypeDenied
```

- [ ] **Step 6: pkg/tools/fs/shared.go 常量 re-export**

在 `pkg/tools/fs/shared.go` 文件尾部追加：

```go
const (
    BlockedTypeDenied = toolshared.BlockedTypeDenied
)
```

- [ ] **Step 7: 编译验证**

```bash
go build -tags goolm,stdjson ./pkg/tools/...
```

- [ ] **Step 8: 运行单元测试**

```bash
go test -tags goolm,stdjson ./pkg/tools/shared/ -run "TestContentForLLM" -v
```

- [ ] **Step 9: 提交**

```bash
git add pkg/tools/shared/result.go pkg/tools/shared_facade.go pkg/tools/fs/shared.go
git commit -m "feat: ToolResult 新增 BlockedType + facade re-export

BlockedTypeBlocked/DENIED 常量，WithBlockedType builder，
ContentForLLM 前缀拼装。facade 层 re-export 供 shell/fs 包直接使用。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 9: shell.go guardCommand 签名改为 (string, string)

**文件:**
- 修改: `pkg/tools/shell.go`

shell.go 属于 package tools，通过 facade re-export 可直接用 `BlockedTypeBlocked` / `BlockedTypeDenied`，无需额外 import。

- [ ] **Step 1: 修改 guardCommand 签名**

```go
func (t *ExecTool) guardCommand(command, cwd string) (string, string) {
```

- [ ] **Step 2: 修改 6 处内部 return**

| 行 | 返回 | 分类 |
|----|------|------|
| 1088 | `return BlockedTypeBlocked, "Command blocked by safety guard (dangerous pattern detected)"` | BLOCKED |
| 1102 | `return BlockedTypeBlocked, "Command blocked by safety guard (not in allowlist)"` | BLOCKED |
| 1109 | `return BlockedTypeBlocked, "Command blocked by safety guard (path traversal detected)"` | BLOCKED |
| 1114 | `return "", ""` | 放行（filepath.Abs 失败） |
| 1192 | `return BlockedTypeDenied, "Command blocked by safety guard (path outside working dir)"` | DENIED |
| 1197 | `return "", ""` | 默认放行 |

- [ ] **Step 3: 修改调用方（347 行）**

```go
blockedType, guardError := t.guardCommand(command, cwd)
if guardError != "" {
    return ErrorResult(guardError).WithBlockedType(blockedType)
}
```

- [ ] **Step 4: 修改 3 处直接 ErrorResult 返回（332/356/368）**

```go
// 332: cwd 验证失败
return ErrorResult("Command blocked by safety guard (" + err.Error() + ")").
    WithBlockedType(BlockedTypeDenied)

// 356: 路径解析失败
return ErrorResult(fmt.Sprintf("Command blocked by safety guard (path resolution failed: %v)", err)).
    WithBlockedType(BlockedTypeDenied)

// 368: 工作目录逃逸 workspace
return ErrorResult("Command blocked by safety guard (working directory escaped workspace)").
    WithBlockedType(BlockedTypeDenied)
```

- [ ] **Step 5: 运行测试**

```bash
go test -tags goolm,stdjson ./pkg/tools/ -run "TestExecTool" -v
```

- [ ] **Step 6: 提交**

```bash
git add pkg/tools/shell.go
git commit -m "feat: shell.go guardCommand 签名改为 (blockedType, errorMsg)

6 处 return 全部适配：dangerous pattern/not allowlist/path traversal → BLOCKED；
path outside/symlink/cwd escape → DENIED；2 处放行 → (\"\", \"\")。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 10: filesystem.go ErrWorkspaceBoundary sentinel + 6 处挂载

**文件:**
- 修改: `pkg/tools/fs/filesystem.go`

- [ ] **Step 1: 定义 sentinel error**

在文件顶部 `var` 块中追加：

```go
var ErrWorkspaceBoundary = errors.New("workspace boundary")
```

确保顶部 import 已有 `"errors"`。

- [ ] **Step 2: resolvePath 3 处挂 sentinel（69/80/86）**

```go
// Line 69:
return "", fmt.Errorf("access denied: path is outside the workspace: %w", ErrWorkspaceBoundary)
// Line 80:
return "", fmt.Errorf("access denied: symlink resolves outside workspace: %w", ErrWorkspaceBoundary)
// Line 86:
return "", fmt.Errorf("access denied: symlink resolves outside workspace: %w", ErrWorkspaceBoundary)
```

- [ ] **Step 3: getSafeRelPath 1 处挂 sentinel（1246）**

```go
return "", fmt.Errorf("path escapes workspace: %s: %w", path, ErrWorkspaceBoundary)
```

- [ ] **Step 4: sandboxFs.ReadFile 拆分三条件（1079-1081）**

```go
// 原来：
if os.IsPermission(err) || strings.Contains(err.Error(), "escapes from parent") ||
    strings.Contains(err.Error(), "permission denied") {
    return fmt.Errorf("failed to read file: access denied: %w", err)
}

// 改为：
if strings.Contains(err.Error(), "escapes from parent") {
    return fmt.Errorf("failed to read file: access denied: %w", ErrWorkspaceBoundary)
}
if os.IsPermission(err) || strings.Contains(err.Error(), "permission denied") {
    return fmt.Errorf("failed to read file: permission denied: %w", err)
}
```

- [ ] **Step 5: sandboxFs.Open 同样拆分（1165-1167）**

```go
// 改为（与 Step 4 对称）：
if strings.Contains(err.Error(), "escapes from parent") {
    return fmt.Errorf("failed to open file: access denied: %w", ErrWorkspaceBoundary)
}
if os.IsPermission(err) || strings.Contains(err.Error(), "permission denied") {
    return fmt.Errorf("failed to open file: permission denied: %w", err)
}
```

- [ ] **Step 6: hostFs 不改 sentinel，改文案（1015/1039）**

```go
// 1015:
return nil, fmt.Errorf("failed to read file: permission denied: %w", err)
// 1039:
return nil, fmt.Errorf("failed to open file: permission denied: %w", err)
```

- [ ] **Step 7: 编译 + 测试**

```bash
go build -tags goolm,stdjson ./pkg/tools/fs/...
go test -tags goolm,stdjson ./pkg/tools/fs/ -v
```

- [ ] **Step 8: 提交**

```bash
git add pkg/tools/fs/filesystem.go
git commit -m "feat: filesystem ErrWorkspaceBoundary sentinel + 6 处挂载

resolvePath ×3、getSafeRelPath ×1、sandboxFs ×2 共 6 处挂 ErrWorkspaceBoundary。
hostFs OS 权限走普通 error，文案改为 permission denied 防 LLM 误判。
sandboxFs 拆分 3 条件：escapes from parent → sentinel，os.IsPermission + permission denied → 普通 error。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 11: filesystem.go helper + 6 处 Execute 层替换

**文件:**
- 修改: `pkg/tools/fs/filesystem.go`
- 修改: `pkg/tools/fs/edit.go`

- [ ] **Step 1: 添加 errorResultFromFS helper**

在 `pkg/tools/fs/filesystem.go` 文件末尾添加：

```go
func errorResultFromFS(err error) *ToolResult {
    if errors.Is(err, ErrWorkspaceBoundary) {
        return ErrorResult(err.Error()).WithBlockedType(BlockedTypeDenied)
    }
    return ErrorResult(err.Error())
}

func errorResultFromFSCtx(context string, err error) *ToolResult {
    msg := fmt.Sprintf("%s: %v", context, err)
    if errors.Is(err, ErrWorkspaceBoundary) {
        return ErrorResult(msg).WithBlockedType(BlockedTypeDenied)
    }
    return ErrorResult(msg)
}
```

确保 `"errors"` 和 `"fmt"` 已在 import 中（filesystem.go 已有）。

- [ ] **Step 2: 替换 6 处 Execute 层 ErrorResult**

| 文件:行 | 原代码 | 替换为 |
|---------|--------|--------|
| `edit.go:74` | `return ErrorResult(err.Error())` | `return errorResultFromFS(err)` |
| `edit.go:128` | `return ErrorResult(err.Error())` | `return errorResultFromFS(err)` |
| `filesystem.go:424` | `return ErrorResult(err.Error())` | `return errorResultFromFS(err)` |
| `filesystem.go:572` | `return ErrorResult(err.Error())` | `return errorResultFromFS(err)` |
| `filesystem.go:932` | `return ErrorResult(err.Error())` | `return errorResultFromFS(err)` |
| `filesystem.go:979` | `return ErrorResult(fmt.Sprintf("failed to read directory: %v", err))` | `return errorResultFromFSCtx("failed to read directory", err)` |

- [ ] **Step 3: 编译 + 测试**

```bash
go build -tags goolm,stdjson ./pkg/tools/fs/...
go test -tags goolm,stdjson ./pkg/tools/fs/ -v
```

- [ ] **Step 4: 提交**

```bash
git add pkg/tools/fs/filesystem.go pkg/tools/fs/edit.go
git commit -m "feat: fs Execute 层 6 处 ErrorResult 替换为 errorResultFromFS helper

errorResultFromFS 自动识别 ErrWorkspaceBoundary 附加 BlockedTypeDenied。
errorResultFromFSCtx 变体保留上下文前缀（ListDirTool）。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 12: 阶段二单元测试

**文件:**
- 修改: `pkg/tools/shared/result_test.go`（追加测试）
- 创建: `pkg/tools/fs/blocked_test.go`

- [ ] **Step 1: ContentForLLM 前缀测试**

在 `pkg/tools/shared/result_test.go` 追加：

```go
func TestContentForLLM_BlockedTypePrefix(t *testing.T) {
    tr := &ToolResult{ForLLM: "Command blocked", BlockedType: BlockedTypeBlocked}
    content := tr.ContentForLLM()
    if !strings.HasPrefix(content, "[BLOCKED] ") {
        t.Fatalf("expected [BLOCKED] prefix, got: %s", content)
    }
    if !strings.Contains(content, "Command blocked") {
        t.Fatalf("expected body preserved, got: %s", content)
    }
}

func TestContentForLLM_EmptyContentNoPrefix(t *testing.T) {
    tr := &ToolResult{ForLLM: "", BlockedType: BlockedTypeBlocked}
    content := tr.ContentForLLM()
    if strings.HasPrefix(content, "[BLOCKED]") {
        t.Fatalf("expected no prefix on empty content, got: %s", content)
    }
}

func TestContentForLLM_NoPrefixOnNormalError(t *testing.T) {
    tr := &ToolResult{ForLLM: "file not found", IsError: true}
    content := tr.ContentForLLM()
    if strings.Contains(content, "[BLOCKED]") || strings.Contains(content, "[DENIED]") {
        t.Fatalf("expected no prefix on normal error, got: %s", content)
    }
}
```

- [ ] **Step 2: ErrWorkspaceBoundary 传播测试**

创建 `pkg/tools/fs/blocked_test.go`（package fstools）：

```go
package fstools

import (
    "fmt"
    "os"
    "testing"
)

func TestErrorResultFromFS_DeniedBlockedType(t *testing.T) {
    err := fmt.Errorf("access denied: path is outside the workspace: %w", ErrWorkspaceBoundary)
    result := errorResultFromFS(err)
    if result.BlockedType != BlockedTypeDenied {
        t.Fatalf("expected BlockedTypeDenied for ErrWorkspaceBoundary, got %s", result.BlockedType)
    }
}

func TestErrorResultFromFS_NoBlockedTypeOnNormalError(t *testing.T) {
    err := fmt.Errorf("file not found: %w", os.ErrNotExist)
    result := errorResultFromFS(err)
    if result.BlockedType != "" {
        t.Fatalf("expected no BlockedType for normal error, got %s", result.BlockedType)
    }
}

func TestErrorResultFromFS_NoBlockedTypeOnOSPermission(t *testing.T) {
    err := fmt.Errorf("failed to read file: permission denied: %w", os.ErrPermission)
    result := errorResultFromFS(err)
    if result.BlockedType != "" {
        t.Fatalf("expected no BlockedType for OS permission, got %s", result.BlockedType)
    }
}
```

- [ ] **Step 3: 运行测试**

```bash
go test -tags goolm,stdjson ./pkg/tools/shared/ -run "TestContentForLLM_Blocked" -v
go test -tags goolm,stdjson ./pkg/tools/fs/ -run "TestErrorResultFromFS" -v
```

- [ ] **Step 4: 提交**

```bash
git add pkg/tools/shared/result_test.go pkg/tools/fs/blocked_test.go
git commit -m "test: ContentForLLM 前缀 + errorResultFromFS 分类测试

覆盖：BlockedType 正确前缀、空 content 不追加、普通错误无前缀、
ErrWorkspaceBoundary 传播、OS 权限不触发 BlockedType。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 13: 阶段二集成验证

- [ ] **Step 1: 全量测试**

```bash
go test -tags goolm,stdjson ./pkg/tools/... -v
go test -tags goolm,stdjson ./pkg/agent/... -v
```

- [ ] **Step 2: 全量构建**

```bash
make build && make build-launcher
```

- [ ] **Step 3: 部署到 RK3506，验证 LLM 自律停止**

```bash
# 在设备上执行被拦截命令，观察 session 日志
# grep -o '"blocked_type":"[^"]*"' sessions/*.jsonl | sort | uniq -c
```

- [ ] **Step 4: 阶段二标记提交**

```bash
git add -A
git commit -m "feat: 阶段二完成 — ToolResult 结构化错误标记

BlockedTypeBlocked/DENIED 分类，ContentForLLM 前缀拼装，
shell guardCommand 签名改造，filesystem sentinel + errorResultFromFS helper。

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

## 联合验证（阶段一+阶段二）

### Task 14: 全量回归 + 指标验证

- [ ] **Step 1: 全量测试**

```bash
go test -tags goolm,stdjson ./... -count=1
```

- [ ] **Step 2: 全量构建（Go + Web 后端）**

```bash
make build && make build-launcher
```

- [ ] **Step 3: 部署 RK3506 跑混合拦截场景**

```bash
# 验证 LLM 在看到 [BLOCKED]/[DENIED] 前缀后 1-3 次自行停止
# 验证 5 次保底硬中断仍生效
# 验证正常探索（file-not-found 等）不被误中断
```

- [ ] **Step 4: 审计命令**

```bash
grep -o '"blocked_type":"[^"]*"' sessions/*.jsonl | sort | uniq -c
```

- [ ] **Step 5: 合并回 dev**

确认效果符合预期后，合并到 dev 分支。
