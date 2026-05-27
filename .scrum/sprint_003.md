# Sprint 3

> 创建: 2026-05-27
> 最后更新: 2026-05-27（review 后重生成）
> 目标版本: -
> 来源: BACKLOG.md（BL-005）
> 状态: 已完成

**设计文档**: `docs/superpowers/specs/2026-05-27-custom-branding-design.md`
**实现计划**: `docs/superpowers/plans/2026-05-27-custom-branding.md`

> 执行时有歧义以实现计划为准（计划经过 review，细节更完整）。设计文档提供架构概览。

## 任务清单

### SB-001 · 升级 pkg/env.go 为品牌控制中心 [x]
- **来源**: BL-005
- **子任务**:
  - [x] 重写 `pkg/env.go`：`Logo`、`AppName`、`EnvPrefix`、`DefaultHome`、`CommandName` 全部为 `var`（支持 ldflags 覆盖）
  - [x] 新增 `GetEnv(suffix)` / `LookupEnv(suffix)` 函数（新前缀优先，旧前缀 PICOCLAW_ fallback）
  - [x] `LookupEnv` 空值语义与 `os.LookupEnv` 一致（不额外判断 `v != ""`）
  - [x] `go build -tags goolm,stdjson ./pkg/` 验证编译通过
  - [x] 提交
- **阻塞**: -

### SB-002 · envkeys.go 常量改用 EnvPrefix 拼接 [x]
- **来源**: BL-005
- **子任务**:
  - [x] `const` 改为 `var`（pkg.EnvPrefix 是 var，Go 不允许 const 引用 var）
  - [x] `"PICOCLAW_HOME"` 等改为 `pkg.EnvPrefix + "HOME"`
  - [x] `GetHome()` 中 `.picoclaw` 引用改为 `pkg.DefaultHome`
  - [x] 验证编译通过并提交
- **阻塞**: SB-001

### SB-003 · 核心 pkg/ 目录 env 读取迁移 [x]
- **来源**: BL-005
- **子任务**:
  - [x] `pkg/logger/logger.go` — `os.Getenv("PICOCLAW_LOG_FILE")` → `pkg.GetEnv("LOG_FILE")`
  - [x] `pkg/config/gateway.go` — `os.Getenv("PICOCLAW_LOG_LEVEL")` → `pkg.GetEnv("LOG_LEVEL")`
  - [x] `pkg/config/config_channel.go` — 3 处 `os.LookupEnv` → `pkg.LookupEnv`
  - [x] `pkg/credential/credential.go` — `PassphraseEnvVar`、`SSHKeyPathEnvVar` 改用 `pkg.EnvPrefix` 拼接，删除 `picoclawHome` 常量改用 `config.EnvHome`
  - [x] 验证编译通过并提交
- **阻塞**: SB-001

### SB-004 · 新增 envOptions() fallback 并迁移 env.Parse [x]
- **来源**: BL-005
- **子任务**:
  - [x] `pkg/config/envkeys.go` 新增 `envOptions()` 函数（构建自定义 env map，新前缀优先，旧 PICOCLAW_* fallback）
  - [x] `pkg/config/config.go:1413` — `env.Parse(cfg)` → `env.ParseWithOptions(cfg, envOptions())`
  - [x] `pkg/config/config_channel.go:729` — 同上
  - [x] 验证编译通过并提交
- **阻塞**: SB-001

### SB-005 · cmd/picoclaw/ 硬编码迁移 [x]
- **来源**: BL-005
- **子任务**:
  - [x] `cmd/picoclaw/dns_noresolv.go` — env 读取改用 `pkg.GetEnv`
  - [x] `cmd/picoclaw/main.go` — 包注释 `// PicoClaw - Ultra-lightweight...` 改为通用描述
  - [x] `cmd/picoclaw/internal/helpers.go` — 路径引用修正
  - [x] `cmd/picoclaw/internal/migrate/command.go` — 路径引用修正
  - [x] 验证编译通过并提交
- **阻塞**: SB-001

### SB-006 · web/backend/ 硬编码迁移 [x]
- **来源**: BL-005
- **子任务**:
  - [x] `utils/runtime.go` — `FindPicoclawBinary` 内部改用 `pkg.CommandName`
  - [x] `launcherconfig/config.go` — `EnvLauncherHost` 改用 `pkg.EnvPrefix`
  - [x] `main.go` — 帮助文本中的路径改用 `pkg.DefaultHome`
  - [x] `api/gateway.go` — 日志和注释中的 "picoclaw" 改为 `pkg.CommandName`
  - [x] `api/update.go` — binary 名改用 `pkg.CommandName`
  - [x] `api/version.go` — 正则以 `pkg.CommandName` 构建
  - [x] `api/skills.go` — temp dir 名改用 `pkg.CommandName`
  - [x] `api/oauth.go` / `wecom.go` / `session.go` / `router.go` — 对应常量修正
  - [x] **不修改** `api/startup.go` 的 launchAgentLabel 和 desktop 文件名（持久化系统标识，换牌后变化会导致重复注册）
  - [x] **不修改** `middleware/launcher_dashboard_auth.go` 的 Cookie 名（浏览器持久标识，换牌后变化会导致已登录用户被踢出）
  - [x] 验证编译通过并提交
- **阻塞**: SB-001

### SB-007 · Makefile 构建系统适配 [x]
- **来源**: BL-005
- **子任务**:
  - [x] 新增 `CUSTOM_PREFIX ?= ZHOSCLAW_` 等 5 个品牌变量（默认值为本项目 zhosclaw）
  - [x] `BINARY_NAME ?= $(CUSTOM_CMD)`（替代硬编码 `BINARY_NAME=zhosclaw`）
  - [x] 新增 `SED_INPLACE` 跨平台检测（BSD `-i ''` vs GNU `-i`）
  - [x] 新增 `validate-prefix` 宏（校验 CUSTOM_PREFIX 不含 `/` 或 `&`）
  - [x] LDFLAGS 追加 5 个品牌变量的 ldflags 注入
  - [x] 新增 `build-with-custom-prefix` 宏（临时副本 + 精确 sed 替换 `envPrefix:"PICOCLAW_` + PID 隔离）
  - [x] 修改 build、build-linux-arm、build-all、build-rk3506 等 target 调用宏
  - [x] 验证默认构建和 `CUSTOM_PREFIX=PICOCLAW_ make build` 两场景编译均通过并提交
- **阻塞**: SB-001 ~ SB-006

### SB-008 · 全量验证与收尾 [x]
- **来源**: BL-005
- **子任务**:
  - [x] `make build && make test` 默认构建全通过
  - [x] `CUSTOM_PREFIX=PICOCLAW_ make build` 还原上游品牌构建全通过
  - [x] 二进制泄露检查：`strings build/zhosclaw | grep -c PICOCLAW_` ≤ 4（GetEnv 2 处 + envOptions 2 处 fallback）
  - [x] 源码检查：非 struct-tag 的 PICOCLAW_ 仅出现在 `pkg/env.go` 和 `pkg/config/envkeys.go`
  - [x] 创建 `scripts/verify-branding.sh` 验证脚本
  - [x] 更新 BACKLOG.md BL-005 验收标准并提交
- **阻塞**: SB-007
