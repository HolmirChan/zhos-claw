# Sprint 3

> 创建: 2026-05-27
> 目标版本: -
> 来源: BACKLOG.md（BL-005）
> 状态: 进行中

## 任务清单

### SB-001 · 升级 pkg/env.go 为品牌控制中心 [ ]
- **来源**: BL-005
- **子任务**:
  - [ ] 重写 `pkg/env.go`：新增 `EnvPrefix`、`GetEnv()`、`LookupEnv()`，整合现有 `DefaultHome`、`CommandName`
  - [ ] `go build -tags goolm,stdjson ./pkg/` 验证编译通过
  - [ ] 提交
- **阻塞**: -

### SB-002 · envkeys.go 常量改用 EnvPrefix 拼接 [ ]
- **来源**: BL-005
- **子任务**:
  - [ ] `const` 改为 `var`（Go 不允许 const 引用 var）
  - [ ] `"PICOCLAW_HOME"` 等改为 `pkg.EnvPrefix + "HOME"`
  - [ ] `GetHome()` 中 `pkg.DefaultPicoClawHome` → `pkg.DefaultHome`
  - [ ] 验证编译通过并提交
- **阻塞**: SB-001

### SB-003 · 核心 pkg/ 目录 env 读取迁移 [ ]
- **来源**: BL-005
- **子任务**:
  - [ ] `pkg/logger/logger.go` — `os.Getenv("PICOCLAW_LOG_FILE")` → `pkg.GetEnv("LOG_FILE")`
  - [ ] `pkg/config/gateway.go` — `os.Getenv("PICOCLAW_LOG_LEVEL")` → `pkg.GetEnv("LOG_LEVEL")`
  - [ ] `pkg/config/config_channel.go` — 3 处 `os.LookupEnv` → `pkg.LookupEnv`
  - [ ] `pkg/credential/credential.go` — `PassphraseEnvVar` 等改用 `pkg.EnvPrefix` 拼接
  - [ ] 验证编译通过并提交
- **阻塞**: SB-001

### SB-004 · 新增 envOptions() fallback 并迁移 env.Parse [ ]
- **来源**: BL-005
- **子任务**:
  - [ ] `pkg/config/envkeys.go` 新增 `envOptions()` 函数（新旧前缀 env map fallback）
  - [ ] `pkg/config/config.go:1413` — `env.Parse(cfg)` → `env.ParseWithOptions(cfg, envOptions())`
  - [ ] `pkg/config/config_channel.go:729` — 同上
  - [ ] 验证编译通过并提交
- **阻塞**: SB-001

### SB-005 · cmd/picoclaw/ 硬编码迁移 [ ]
- **来源**: BL-005
- **子任务**:
  - [ ] `cmd/picoclaw/dns_noresolv.go` — env 读取改用 `pkg.GetEnv`
  - [ ] `cmd/picoclaw/internal/helpers.go` — 路径注释修正
  - [ ] `cmd/picoclaw/internal/migrate/command.go` — 路径注释修正
  - [ ] 验证编译通过并提交
- **阻塞**: SB-001

### SB-006 · web/backend/ 硬编码迁移 [ ]
- **来源**: BL-005
- **子任务**:
  - [ ] `utils/runtime.go` — `FindPicoclawBinary` 内部改用 `pkg.CommandName`
  - [ ] `launcherconfig/config.go` — `EnvLauncherHost` 改用 `pkg.EnvPrefix`
  - [ ] `main.go` — 帮助文本中的路径改用 `pkg.DefaultHome`
  - [ ] `api/gateway.go` — 日志和注释中的 "picoclaw" 改为 `pkg.CommandName`
  - [ ] `api/startup.go` — launchAgentLabel、desktop 文件名改用 `pkg.CommandName`
  - [ ] `api/update.go` — binary 名改用 `pkg.CommandName`
  - [ ] `api/version.go` — 正则以 `pkg.CommandName` 构建
  - [ ] `api/skills.go` — temp dir 名改用 `pkg.CommandName`
  - [ ] `api/oauth.go` / `wecom.go` / `session.go` / `router.go` — 对应常量修正
  - [ ] `middleware/launcher_dashboard_auth.go` — cookie 名改用 `pkg.CommandName`
  - [ ] 验证编译通过并提交
- **阻塞**: SB-001

### SB-007 · Makefile 构建系统适配 [ ]
- **来源**: BL-005
- **子任务**:
  - [ ] 新增 `CUSTOM_PREFIX`、`CUSTOM_HOME`、`CUSTOM_CMD`、`CUSTOM_APP`、`CUSTOM_LOGO` 变量
  - [ ] LDFLAGS 追加品牌变量注入
  - [ ] 新增 `build-with-custom-prefix` 宏（sed 只替换 `envPrefix:"PICOCLAW_` → 新前缀）
  - [ ] 修改 build、build-linux-arm、build-all、build-rk3506 等 target 调用宏
  - [ ] 验证默认前缀和自定义前缀编译均通过并提交
- **阻塞**: SB-001 ~ SB-006

### SB-008 · 全量验证与收尾 [ ]
- **来源**: BL-005
- **子任务**:
  - [ ] `make build && make test` 默认前缀全通过
  - [ ] `CUSTOM_PREFIX=ZHOSCLAW_ make build && make test` 自定义前缀全通过
  - [ ] `strings build/zhosclaw | grep PICOCLAW_` 二进制泄露检查
  - [ ] 源码干净检查：非 struct-tag 的 PICOCLAW_ 仅在 `pkg/env.go` 中
  - [ ] 创建 `scripts/verify-branding.sh` 验证脚本
  - [ ] 更新 BACKLOG.md BL-005 验收标准并提交
- **阻塞**: SB-007
