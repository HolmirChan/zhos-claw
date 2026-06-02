# Sprint 7

> 创建: 2026-06-02
> 来源: BACKLOG.md（BL-010）
> 设计文档: .scrum/specs/2026-06-02-launcher-tls-design.md
> 实现计划: .scrum/plans/2026-06-02-launcher-tls.md
> 状态: 进行中

## 任务清单

### SB-001 · TLS 核心纯函数 [ ]
- **来源**: BL-010
- **文件**: `web/backend/tls.go`（新建）、`web/backend/tls_test.go`（新建）
- **子任务**:
  - [ ] 实现 `shouldStartTLS` / `validateTLSPort` / `acceptIP` / `getAllLocalIPs`
  - [ ] 4 项单测 PASS
  - [ ] 提交

### SB-002 · 证书生成 [ ]
- **来源**: BL-010
- **文件**: `web/backend/tls.go`（追加）、`web/backend/tls_test.go`（追加）
- **子任务**:
  - [ ] 实现 `generateSelfSignedCert` + `tlsMeta` 结构
  - [ ] ECDSA P256 + X.509 自签名 + SAN 多 IP + 时钟 fallback
  - [ ] 4 项单测 PASS
  - [ ] 提交

### SB-003 · 缓存管理 [ ]
- **来源**: BL-010
- **文件**: `web/backend/tls.go`（追加）、`web/backend/tls_test.go`（追加）
- **子任务**:
  - [ ] 实现 `loadTLSCache` / `saveTLSCache` / `matchesCurrentIPsWith` / `needsRegen`
  - [ ] 文件写入 .tmp + rename + flock
  - [ ] 6 项单测 PASS
  - [ ] 提交

### SB-004 · 文件锁跨平台 [ ]
- **来源**: BL-010
- **文件**: `web/backend/tls_lock_unix.go`（新建）、`web/backend/tls_lock_windows.go`（新建）
- **子任务**:
  - [ ] Unix flock / Windows stub
  - [ ] 双平台编译 PASS
  - [ ] 提交

### SB-005 · ensureTLS 编排 + main.go 集成 [ ]
- **来源**: BL-010
- **文件**: `web/backend/tls.go`（追加）、`web/backend/main.go`（修改）、`web/backend/tls_test.go`（追加）
- **子任务**:
  - [ ] 实现 `ensureTLS`（自建 netbind.Plan → OpenPlan → ServeTLS）
  - [ ] main.go flag 解析（`-no-tls` / `-tls-port`）
  - [ ] HTTPS server 启动 + servers 切片管理 + Graceful Shutdown
  - [ ] 控制台和日志打印 HTTPS 地址
  - [ ] 6 项单测 + 全量 PASS
  - [ ] 提交

### SB-006 · /api/system/version 新增 http_url / https_url [ ]
- **来源**: BL-010
- **文件**: `web/backend/api/version.go`、`web/backend/api/router.go`、`web/backend/main.go`
- **子任务**:
  - [ ] `systemVersionResponse` 新增 `HTTPURL` / `HTTPSURL` 字段
  - [ ] `Handler` 新增 `SetLauncherURLs` setter
  - [ ] `main.go` 在 serverAddr/httpsAddr 计算完成后调用 setter
  - [ ] 编译 + 单测 PASS
  - [ ] 提交

### SB-007 · 前端 chat-composer isSecureContext 检测 [ ]
- **来源**: BL-010
- **文件**: `web/frontend/src/components/chat/chat-composer.tsx`、`web/frontend/src/api/voice.ts`
- **子任务**:
  - [ ] `api/voice.ts` 新增 `fetchVersionURLs()`
  - [ ] `chat-composer.tsx` 🎤 按钮 isSecure 检测（非安全上下文 disabled + tooltip）
  - [ ] `npx tsc --noEmit` 零错误
  - [ ] 提交

### SB-008 · 部署脚本 + 全量构建验收 [ ]
- **来源**: BL-010
- **阻塞**: SB-001~SB-007 全部完成
- **子任务**:
  - [ ] 部署脚本打印 HTTPS 地址
  - [ ] `make build && make build-launcher` 全量构建
  - [ ] `go test ./web/backend/...` + `npx tsc --noEmit` 全量 PASS
  - [ ] 提交
