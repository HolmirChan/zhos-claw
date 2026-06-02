# Sprint 7

> 创建: 2026-06-02
> 完成: 2026-06-02
> 来源: BACKLOG.md（BL-010）
> 设计文档: .scrum/specs/2026-06-02-launcher-tls-design.md
> 实现计划: .scrum/plans/2026-06-02-launcher-tls.md
> 状态: 已完成

## 任务清单

### SB-001 · TLS 核心纯函数 [x]
- **来源**: BL-010
- **文件**: `web/backend/tls.go`（新建）、`web/backend/tls_test.go`（新建）
- **子任务**:
  - [x] 实现 `shouldStartTLS` / `validateTLSPort` / `acceptIP` / `getAllLocalIPs`
  - [x] 4 项单测 PASS
  - [x] 提交

### SB-002 · 证书生成 [x]
- **来源**: BL-010
- **文件**: `web/backend/tls.go`（追加）、`web/backend/tls_test.go`（追加）
- **子任务**:
  - [x] 实现 `generateSelfSignedCert` + `tlsMeta` 结构
  - [x] ECDSA P256 + X.509 自签名 + SAN 多 IP + 时钟 fallback
  - [x] 4 项单测 PASS
  - [x] 提交

### SB-003 · 缓存管理 [x]
- **来源**: BL-010
- **文件**: `web/backend/tls.go`（追加）、`web/backend/tls_test.go`（追加）
- **子任务**:
  - [x] 实现 `loadTLSCache` / `saveTLSCache` / `matchesCurrentIPsWith` / `needsRegen`
  - [x] 文件写入 .tmp + rename + flock
  - [x] 6 项单测 PASS
  - [x] 提交

### SB-004 · 文件锁跨平台 [x]
- **来源**: BL-010
- **文件**: `web/backend/tls_lock_unix.go`（新建）、`web/backend/tls_lock_windows.go`（新建）
- **子任务**:
  - [x] Unix flock / Windows stub
  - [x] 双平台编译 PASS
  - [x] 提交

### SB-005 · ensureTLS 编排 + main.go 集成 [x]
- **来源**: BL-010
- **文件**: `web/backend/tls.go`（追加）、`web/backend/main.go`（修改）、`web/backend/tls_test.go`（追加）
- **子任务**:
  - [x] 实现 `ensureTLS`（自建 netbind.Plan → OpenPlan → ServeTLS）
  - [x] main.go flag 解析（`-no-tls` / `-tls-port`）
  - [x] HTTPS server 启动 + servers 切片管理 + Graceful Shutdown
  - [x] 控制台和日志打印 HTTPS 地址
  - [x] 6 项单测 + 全量 PASS
  - [x] 提交

### SB-006 · /api/system/version 新增 http_url / https_url [x]
- **来源**: BL-010
- **文件**: `web/backend/api/version.go`、`web/backend/api/router.go`、`web/backend/main.go`
- **子任务**:
  - [x] `systemVersionResponse` 新增 `HTTPURL` / `HTTPSURL` 字段
  - [x] `Handler` 新增 `SetLauncherURLs` setter
  - [x] `main.go` 在 serverAddr/httpsAddr 计算完成后调用 setter
  - [x] 编译 + 单测 PASS
  - [x] 提交

### SB-007 · 前端 chat-composer isSecureContext 检测 [x]
- **来源**: BL-010
- **文件**: `web/frontend/src/components/chat/chat-composer.tsx`、`web/frontend/src/api/voice.ts`
- **子任务**:
  - [x] `api/voice.ts` 新增 `fetchVersionURLs()`
  - [x] `chat-composer.tsx` 🎤 按钮 isSecure 检测（非安全上下文 disabled + tooltip）
  - [x] `npx tsc --noEmit` 零错误
  - [x] 提交

### SB-008 · 部署脚本 + 全量构建验收 [x]
- **来源**: BL-010
- **阻塞**: SB-001~SB-007 全部完成
- **子任务**:
  - [x] 部署脚本打印 HTTPS 地址
  - [x] `make build && make build-launcher` 全量构建
  - [x] `go test ./web/backend/...` + `npx tsc --noEmit` 全量 PASS
  - [x] 提交

### SB-009 · 人工验收 [x]
- **来源**: BL-010
- **阻塞**: SB-001~SB-008 全部完成
- **子任务**:
  - [x] 部署到设备，浏览器访问 `http://IP:18800`，🎤 按钮可见，点击弹窗确认后跳转 `https://IP:18443`
  - [x] `https://IP:18443` 浏览器警告后继续访问，🎤 按钮可用、录音正常
  - [x] 证书显示为 `ZaiAgent TLS by OpenValley`
  - [x] `http://IP:18800` 聊天等原有功能不受影响
  - [x] 清空 `<home>/tls/` 后重启 launcher，证书自动重生成，IP 变化时自动更新
