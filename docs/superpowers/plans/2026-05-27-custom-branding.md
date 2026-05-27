# 品牌一键替换 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `pkg/env.go` 改一处值 + Makefile 变量，`make build` 产出完全换牌二进制（env 前缀、二进制名、默认目录、显示名全部替换）

**Architecture:** 所有品牌相关变量集中到 `pkg/env.go`，运行时 env 读取统一走 `pkg.GetEnv()` 支持新旧双前缀兼容，struct tag 通过构建时临时副本 sed 替换前缀，编译后源码不变

**Tech Stack:** Go 1.x, Makefile, caarlos0/env v11, sed

**设计文档:** `docs/superpowers/specs/2026-05-27-custom-branding-design.md`

---

### Task 1: 升级 `pkg/env.go` 为品牌控制中心

**Files:**
- Modify: `pkg/env.go`

- [ ] **Step 1: 重写 pkg/env.go**

```go
package pkg

import "os"

var (
	// Logo is the emoji icon displayed in the terminal.
	// Overridable at compile time via -ldflags.
	Logo = "🦞"

	// AppName is the user-visible application display name.
	// Overridable at compile time via -ldflags.
	AppName = "PicoClaw"

	// EnvPrefix is the prefix for all environment variables.
	// Overridable at compile time via -ldflags.
	EnvPrefix = "PICOCLAW_"

	// DefaultHome is the default config directory name (relative to user home).
	// Overridable at compile time via -ldflags.
	DefaultHome = ".picoclaw"

	// CommandName is the CLI command name (lowercase).
	// Overridable at compile time via -ldflags.
	CommandName = "picoclaw"
)

const (
	WorkspaceName = "workspace"
)

// GetEnv reads an environment variable with the configured prefix,
// falling back to the PICOCLAW_ prefix for backward compatibility.
func GetEnv(suffix string) string {
	if v := os.Getenv(EnvPrefix + suffix); v != "" {
		return v
	}
	return os.Getenv("PICOCLAW_" + suffix)
}

// LookupEnv is like GetEnv but reports whether the key was present.
// Matches os.LookupEnv semantics: an empty-but-set variable returns ("", true).
func LookupEnv(suffix string) (string, bool) {
	if v, ok := os.LookupEnv(EnvPrefix + suffix); ok {
		return v, true
	}
	return os.LookupEnv("PICOCLAW_" + suffix)
}
```

- [ ] **Step 2: 验证编译通过**

```bash
go build -tags goolm,stdjson ./pkg/
```
Expected: 编译通过，无报错

- [ ] **Step 3: 提交**

```bash
git add pkg/env.go
git commit -m "feat: 升级 pkg/env.go 为品牌控制中心，新增 EnvPrefix/GetEnv/LookupEnv

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 2: envkeys.go 常量改用 EnvPrefix 拼接

**Files:**
- Modify: `pkg/config/envkeys.go`

- [ ] **Step 1: 修改 envkeys.go**

`pkg.EnvPrefix` 是 `var`（支持 ldflags 覆盖），Go 不允许 `const` 引用 `var`，因此 envkeys 的 key 定义也必须是 `var`：

```go
package config

import (
	"os"
	"path/filepath"

	"github.com/sipeed/picoclaw/pkg"
)

var (
	EnvHome          = pkg.EnvPrefix + "HOME"
	EnvConfig        = pkg.EnvPrefix + "CONFIG"
	EnvBuiltinSkills  = pkg.EnvPrefix + "BUILTIN_SKILLS"
	EnvBinary        = pkg.EnvPrefix + "BINARY"
	EnvGatewayHost   = pkg.EnvPrefix + "GATEWAY_HOST"
)

func GetHome() string {
	homePath, _ := os.UserHomeDir()
	if picoclawHome := os.Getenv(EnvHome); picoclawHome != "" {
		homePath = picoclawHome
	} else if homePath != "" {
		homePath = filepath.Join(homePath, pkg.DefaultHome)
	}
	if homePath == "" {
		homePath = "."
	}
	return homePath
}
```

检查所有引用这几个 key 的地方 —— 当前用法均为 `os.Getenv(EnvHome)` 等函数调用参数，`var` 完全兼容。

- [ ] **Step 2: 验证编译**

```bash
go build -tags goolm,stdjson ./pkg/config/
```
Expected: 编译通过

- [ ] **Step 3: 提交**

```bash
git add pkg/config/envkeys.go
git commit -m "feat: envkeys.go 改用 pkg.EnvPrefix 拼接环境变量名

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 3: 核心 pkg/ 目录 env 读取迁移到 pkg.GetEnv

**Files:**
- Modify: `pkg/logger/logger.go:221`
- Modify: `pkg/config/gateway.go:94`
- Modify: `pkg/config/config_channel.go:753,758,763`
- Modify: `pkg/credential/credential.go:45,71,75`

- [ ] **Step 1: 修改 pkg/logger/logger.go**

```go
// 找到第 221 行附近，将：
if logFile := os.Getenv("PICOCLAW_LOG_FILE"); logFile != "" {
// 改为：
if logFile := pkg.GetEnv("LOG_FILE"); logFile != "" {
```

文件顶部需要确认已 import `"github.com/sipeed/picoclaw/pkg"`。

- [ ] **Step 2: 修改 pkg/config/gateway.go**

```go
// 找到第 94 行附近，将：
if envLevel := os.Getenv("PICOCLAW_LOG_LEVEL"); envLevel != "" {
// 改为：
if envLevel := pkg.GetEnv("LOG_LEVEL"); envLevel != "" {
```

- [ ] **Step 3: 修改 pkg/config/config_channel.go applyTelegramStreamingEnvCompat**

第 753-767 行的三个 `os.LookupEnv("PICOCLAW_CHANNELS_TELEGRAM_*")` 调用改为 `pkg.LookupEnv("CHANNELS_TELEGRAM_*")`：

```go
func applyTelegramStreamingEnvCompat(target any) {
	settings, ok := target.(*TelegramSettings)
	if !ok || settings == nil {
		return
	}
	if raw, ok := pkg.LookupEnv("CHANNELS_TELEGRAM_STREAMING_ENABLED"); ok {
		if value, err := strconv.ParseBool(raw); err == nil {
			settings.Streaming.Enabled = value
		}
	}
	if raw, ok := pkg.LookupEnv("CHANNELS_TELEGRAM_STREAMING_THROTTLE_SECONDS"); ok {
		if value, err := strconv.Atoi(raw); err == nil {
			settings.Streaming.ThrottleSeconds = value
		}
	}
	if raw, ok := pkg.LookupEnv("CHANNELS_TELEGRAM_STREAMING_MIN_GROWTH_CHARS"); ok {
		if value, err := strconv.Atoi(raw); err == nil {
			settings.Streaming.MinGrowthChars = value
		}
	}
}
```

确认 import 中已有 `"github.com/sipeed/picoclaw/pkg"`。

- [ ] **Step 4: 修改 pkg/credential/credential.go**

```go
// 第 45 行:
const PassphraseEnvVar = "PICOCLAW_KEY_PASSPHRASE"
// 改为:
var PassphraseEnvVar = pkg.EnvPrefix + "KEY_PASSPHRASE"

// 第 71 行:
const SSHKeyPathEnvVar = "PICOCLAW_SSH_KEY_PATH"
// 改为:
var SSHKeyPathEnvVar = pkg.EnvPrefix + "SSH_KEY_PATH"

// 第 75 行:
const picoclawHome = "PICOCLAW_HOME"
// 改为: 删除此常量，直接使用 config.EnvHome
// 检查第 75 行附近引用此常量的代码，改为直接使用 config.EnvHome
```

检查 `picoclawHome` 常量的所有引用点并修改。

- [ ] **Step 5: 验证编译**

```bash
go build -tags goolm,stdjson ./pkg/...
```
Expected: 编译通过

- [ ] **Step 6: 提交**

```bash
git add pkg/logger/logger.go pkg/config/gateway.go pkg/config/config_channel.go pkg/credential/credential.go
git commit -m "feat: pkg/ 目录 env 读取迁移到 pkg.GetEnv/pkg.LookupEnv

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 4: 新增 envOptions() 并迁移 env.Parse 调用

**Files:**
- Modify: `pkg/config/envkeys.go` — 新增 `envOptions()`
- Modify: `pkg/config/config.go:1413`
- Modify: `pkg/config/config_channel.go:729`

- [ ] **Step 1: 在 envkeys.go 中新增 envOptions()**

在 `pkg/config/envkeys.go` 末尾添加：

```go
import (
	"strings"

	"github.com/caarlos0/env/v11"
)

// envOptions returns env.Options that maps both the configured prefix
// and the legacy PICOCLAW_ prefix into the environment, so struct tags
// work regardless of which env vars the user has set.
func envOptions() env.Options {
	raw := os.Environ()
	envMap := make(map[string]string, len(raw))
	for _, e := range raw {
		k, v, _ := strings.Cut(e, "=")
		envMap[k] = v
	}
	// Fallback: PICOCLAW_* → configured prefix_*
	for _, e := range raw {
		k, v, _ := strings.Cut(e, "=")
		if strings.HasPrefix(k, "PICOCLAW_") {
			newKey := pkg.EnvPrefix + strings.TrimPrefix(k, "PICOCLAW_")
			if _, ok := envMap[newKey]; !ok {
				envMap[newKey] = v
			}
		}
	}
	return env.Options{Environment: envMap}
}
```

确认 import 中加入了 `"github.com/sipeed/picoclaw/pkg"`、`"strings"`、`"github.com/caarlos0/env/v11"`。

- [ ] **Step 2: 修改 config.go 的 env.Parse 调用**

第 1413 行：
```go
if err = env.Parse(cfg); err != nil {
// 改为：
if err = env.ParseWithOptions(cfg, envOptions()); err != nil {
```

- [ ] **Step 3: 修改 config_channel.go 的 env.Parse 调用**

第 729 行：
```go
if err := env.Parse(target); err != nil {
// 改为：
if err := env.ParseWithOptions(target, envOptions()); err != nil {
```

- [ ] **Step 4: 验证编译**

```bash
go build -tags goolm,stdjson ./pkg/config/...
```
Expected: 编译通过

- [ ] **Step 5: 提交**

```bash
git add pkg/config/envkeys.go pkg/config/config.go pkg/config/config_channel.go
git commit -m "feat: env.Parse 迁移到 env.ParseWithOptions，支持新旧前缀兼容

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 5: cmd/picoclaw/ 目录 env 读取和硬编码迁移

**Files:**
- Modify: `cmd/picoclaw/dns_noresolv.go:20-21`
- Modify: `cmd/picoclaw/internal/helpers.go`
- Modify: `cmd/picoclaw/internal/migrate/command.go`
- Modify: `cmd/picoclaw/main.go` — package comment

- [ ] **Step 1: 修改 cmd/picoclaw/dns_noresolv.go**

第 20-21 行：
```go
// 例如: PICOCLAW_DNS_SERVER="8.8.8.8:53;1.1.1.1:53;223.5.5.5:53"
dnsEnv := os.Getenv("PICOCLAW_DNS_SERVER")
// 改为:
dnsEnv := pkg.GetEnv("DNS_SERVER")
```
注释中的 `PICOCLAW_DNS_SERVER` 改为 `{prefix}DNS_SERVER` 或直接用通用表述。

- [ ] **Step 2: 修改 cmd/picoclaw/internal/helpers.go**

```go
// 注释中的 ~/.picoclaw 改为 ~/{DefaultHome}
// 检查是否有硬编码路径，如有则改为 pkg.DefaultHome
```

- [ ] **Step 3: 修改 cmd/picoclaw/internal/migrate/command.go**

第 49 行注释中的 `~/.picoclaw` 改为 `~/{project config dir}`。

- [ ] **Step 4: 修改 cmd/picoclaw/main.go package comment**

```go
// PicoClaw - Ultra-lightweight personal AI agent
// 改为通用注释或删除（该注释不影响用户感知的品牌）
```

- [ ] **Step 5: 验证编译**

```bash
go build -tags goolm,stdjson ./cmd/picoclaw/...
```
Expected: 编译通过

- [ ] **Step 6: 提交**

```bash
git add cmd/picoclaw/
git commit -m "feat: cmd/picoclaw env 读取和注释迁移到 pkg 常量

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 6: web/backend/ 硬编码迁移

**Files:**
- Modify: `web/backend/utils/runtime.go`
- Modify: `web/backend/main.go`
- Modify: `web/backend/launcherconfig/config.go`
- Modify: `web/backend/i18n.go`
- Modify: `web/backend/api/gateway.go`
- Modify: `web/backend/api/startup.go`
- Modify: `web/backend/api/update.go`
- Modify: `web/backend/api/version.go`
- Modify: `web/backend/api/skills.go`
- Modify: `web/backend/api/oauth.go`
- Modify: `web/backend/api/wecom.go`
- Modify: `web/backend/api/session.go`
- Modify: `web/backend/middleware/launcher_dashboard_auth.go`

- [ ] **Step 1: web/backend/utils/runtime.go — FindPicoclawBinary 等**

这是 web/backend 中最重要的文件，涉及二进制名查找逻辑：

```go
// 第 31-56 行 FindPicoclawBinary:
// - 函数名 FindPicoclawBinary → FindGatewayBinary
// - 内部 "picoclaw" 字符串 → pkg.CommandName
// - "PICOCLAW_BINARY" env → config.EnvBinary (已由 envkeys.go 提供)
// - "~/.picoclaw" 注释 → "~/{home}/.picoclaw" 或改用 DefaultHome 描述

// GetPicoclawHome → GetGatewayHome (或保留名但内部改用 pkg常数)
// GetDefaultConfigPath: 注释和路径改用 DefaultHome
```

具体修改：
```go
// 第 56 行:
return "picoclaw"
// 改为:
return pkg.CommandName

// 第 49 行注释:
logger.Debugf("Trying to find picoclaw binary in %s", exe)
// 改为:
logger.Debugf("Trying to find %s binary in %s", pkg.CommandName, exe)
```

- [ ] **Step 2: web/backend/launcherconfig/config.go**

```go
// 第 18 行:
EnvLauncherHost = "PICOCLAW_LAUNCHER_HOST"
// 改为:
var EnvLauncherHost = pkg.EnvPrefix + "LAUNCHER_HOST"
```

- [ ] **Step 3: web/backend/main.go**

```go
// 第 8-10 行注释: picoclaw-web → {command}-web, 改为通用描述
// 第 359 行:
fmt.Fprintf(os.Stderr, "  config.json    Path to the configuration file (default: ~/.picoclaw/config.json)\n\n")
// 改为运行时获取 DefaultHome:
fmt.Fprintf(os.Stderr, "  config.json    Path to the configuration file (default: ~/%s/config.json)\n\n", pkg.DefaultHome)
```

- [ ] **Step 4: web/backend/api/gateway.go — 多处 "picoclaw" 硬编码**

```go
// 第 42 行:
pidData *ppid.PidFileData // pid file data read from picoclaw.pid.json
// 注释中的 picoclaw 改为一般性描述

// 第 159, 191, 214, 268, 1031, 1089, 1173 行:
// 所有注释和日志中的 "picoclaw" → pkg.CommandName 或通用表述
// 第 191 行 "picoclaw.exe" → pkg.CommandName + ".exe"
// 第 1089 行日志消息中的 "picoclaw gateway" → pkg.CommandName + " gateway"
```

- [ ] **Step 5: web/backend/api/startup.go**

```go
// 第 18 行:
launchAgentLabel = "io.picoclaw.launcher"
// 保持不动！LaunchAgent label 是 macOS 系统级持久标识，换牌后若变化会导致新旧两个 plist 同时存在、
// 重复启动。它应被视为内部标识符，不参与品牌替换。
// 如需支持多品牌共存，后续迭代单独设计命名空间方案。

// 第 221 行:
return filepath.Join(home, ".config", "autostart", "picoclaw-web.desktop")
// 同上，保持不动。autostart 文件名是系统级标识，不应随品牌变化。
```

- [ ] **Step 6: web/backend/api/update.go**

```go
// 第 42 行:
binary = "picoclaw-launcher"
// 改为:
binary = pkg.CommandName + "-web"
```

- [ ] **Step 7: web/backend/api/version.go**

```go
// 第 56 行正则中的 "picoclaw":
`^(?:[^A-Za-z0-9]*\s*)?picoclaw(?:\.exe)?\s+([^\s(]+)`
// 改为用 pkg.CommandName 构建正则
```

- [ ] **Step 8: web/backend/api/skills.go**

```go
// 第 875 行:
tmpDir, tempDirErr := os.MkdirTemp("", "picoclaw-skill-import-*")
// 改为:
tmpDir, tempDirErr := os.MkdirTemp("", pkg.CommandName+"-skill-import-*")
```

- [ ] **Step 9: web/backend/api/oauth.go, wecom.go, session.go, router.go**

```go
// oauth.go:495: "picoclaw-oauth-result" → pkg.CommandName + "-oauth-result"
// wecom.go:24: wecomQRSourceID = "picoclaw" → wecomQRSourceID = pkg.CommandName
// session.go:760: "~/.picoclaw/workspace" → "~/" + pkg.DefaultHome + "/workspace"
// router.go:53: "PICOCLAW_LAUNCHER_HOST" → pkg.EnvPrefix + "LAUNCHER_HOST"
```

- [ ] **Step 10: web/backend/middleware/launcher_dashboard_auth.go**

```go
// 第 17 行:
const LauncherDashboardCookieName = "picoclaw_launcher_auth"
// 保持不动！Cookie 名是浏览器持久标识，换牌后会导致所有已登录用户被踢出。
// 应被视为内部标识符，不参与品牌替换。
```

- [ ] **Step 11: web/backend/i18n.go**

```go
// 第 61, 78 行: https://docs.picoclaw.io 外部 URL
// 外部文档链接不应随品牌变化，保持不动
```

- [ ] **Step 12: 验证编译**

```bash
go build -tags goolm,stdjson ./web/backend/...
```
Expected: 编译通过

- [ ] **Step 13: 提交**

```bash
git add web/backend/
git commit -m "feat: web/backend 硬编码 picoclaw 迁移到 pkg 常量

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 7: Makefile 构建系统适配

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: 新增 CUSTOM_PREFIX 变量和 build-custom target**

在 Makefile 顶部追加：

```makefile
# 品牌自定义变量（默认值保持与当前 BINARY_NAME=zhosclaw 一致）
CUSTOM_PREFIX ?= PICOCLAW_
CUSTOM_HOME   ?= .picoclaw
CUSTOM_CMD    ?= picoclaw
CUSTOM_APP    ?= PicoClaw
CUSTOM_LOGO   ?= 🦞
```

同时在文件顶部（当前 `BINARY_NAME=zhosclaw` 第 4 行）将 `BINARY_NAME` 改为从 `CUSTOM_CMD` 派生：

```makefile
# 修改前（第 4 行）：
BINARY_NAME=zhosclaw

# 修改后：
BINARY_NAME ?= $(CUSTOM_CMD)
```

这样自定义 `CUSTOM_CMD=zhosclaw make build` 会产出 `build/zhosclaw`，验证脚本的路径检查才能正确匹配。

- [ ] **Step 2: 修改 LDFLAGS 加入品牌变量**

```makefile
# 当前 LDFLAGS (第 32 行附近):
LDFLAGS=-X $(CONFIG_PKG).Version=$(VERSION) -X $(CONFIG_PKG).GitCommit=$(GIT_COMMIT) -X $(CONFIG_PKG).BuildTime=$(BUILD_TIME) -X $(CONFIG_PKG).GoVersion=$(GO_VERSION) -s -w

# 追加品牌变量:
LDFLAGS=-X $(CONFIG_PKG).Version=$(VERSION) -X $(CONFIG_PKG).GitCommit=$(GIT_COMMIT) -X $(CONFIG_PKG).BuildTime=$(BUILD_TIME) -X $(CONFIG_PKG).GoVersion=$(GO_VERSION) -X github.com/sipeed/picoclaw/pkg.EnvPrefix=$(CUSTOM_PREFIX) -X github.com/sipeed/picoclaw/pkg.DefaultHome=$(CUSTOM_HOME) -X github.com/sipeed/picoclaw/pkg.CommandName=$(CUSTOM_CMD) -X github.com/sipeed/picoclaw/pkg.AppName=$(CUSTOM_APP) -X github.com/sipeed/picoclaw/pkg.Logo=$(CUSTOM_LOGO) -s -w
```

- [ ] **Step 3: 修改 build target 加入 struct tag sed 替换**

**重要约束**：sed 只替换 struct tag `envPrefix:"PICOCLAW_..."` 中的前缀，不碰其他 `PICOCLAW_` 字符串。这是因为：
- `pkg/env.go` 中的 `EnvPrefix = "PICOCLAW_"` 是默认值，ldflags 在链接时覆盖，不需要 sed 改
- fallback 函数中的 `"PICOCLAW_"` 必须保留以支持向下兼容
- 其他 Go 代码已通过 Task 3-6 改用 `pkg.GetEnv()` 等引用变量

因此 sed 命令精确匹配 `envPrefix:"PICOCLAW_` 模式。

同时处理三个跨平台问题：
1. **BSD vs GNU sed**：macOS 用 `sed -i ''`，Linux 用 `sed -i`，自动检测
2. **CUSTOM_PREFIX 校验**：含 `/` 或 `&` 会破坏 sed，提前拦截
3. **并发安全**：临时目录加 PID 后缀避免并行 `make -j` 冲突

```makefile
CUSTOM_PREFIX ?= PICOCLAW_

# 跨平台 sed in-place 选项
SED_INPLACE := $(if $(shell sed --version 2>/dev/null | head -1 | grep -qi gnu && echo 1),-i,-i '')

# 校验 CUSTOM_PREFIX 不含 sed 特殊字符
define validate-prefix
	@case "$(CUSTOM_PREFIX)" in \
		*/*) echo "ERROR: CUSTOM_PREFIX must not contain '/': $(CUSTOM_PREFIX)" >&2; exit 1 ;; \
		*\\&*) echo "ERROR: CUSTOM_PREFIX must not contain '&': $(CUSTOM_PREFIX)" >&2; exit 1 ;; \
		*) ;; \
	esac
endef

# 内部宏：在临时目录中替换 struct tag 前缀并编译
define build-with-custom-prefix
	$(call validate-prefix)
	@mkdir -p $(BUILD_DIR)/custom-build.$$$$
	@cp -r cmd pkg web go.mod go.sum $(BUILD_DIR)/custom-build.$$$$/
	@if [ "$(CUSTOM_PREFIX)" != "PICOCLAW_" ]; then \
		echo "  Applying custom prefix to struct tags: $(CUSTOM_PREFIX)"; \
		find $(BUILD_DIR)/custom-build.$$$$ -name '*.go' -exec sed $(SED_INPLACE) 's/envPrefix:"PICOCLAW_/envPrefix:"$(CUSTOM_PREFIX)/g' {} + ; \
	fi
	cd $(BUILD_DIR)/custom-build.$$$$ && $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(1) ./cmd/picoclaw
	@rm -rf $(BUILD_DIR)/custom-build.$$$$
endef

## build: Build the binary for current platform
build: clean-workspace
	@echo "Building $(BINARY_NAME) for current platform..."
	$(call build-with-custom-prefix,$(abspath $(BINARY_PATH)$(EXT)))
```

`envPrefix:"PICOCLAW_` 这个模式在 Go 代码中唯一出现在 struct tag 里，不会误伤任何其他位置。

- [ ] **Step 4: 同步修改其他 build target**

所有使用 `$(GO) build` 的 target（linux-arm、build-all、build-rk3506 等）改为调用 `build-with-custom-prefix` 宏。对于有额外 GOOS/GOARCH 环境的 target，在宏调用前设置环境变量：

```makefile
build-linux-arm:
	@echo "Building $(BINARY_NAME)-linux-arm..."
	GOOS=linux GOARCH=arm GOARM=7 $(call build-with-custom-prefix,$(abspath $(BUILD_DIR)/$(BINARY_NAME)-linux-arm))
```

- [ ] **Step 5: 提交**

```bash
git add Makefile
git commit -m "feat: Makefile 新增 CUSTOM_PREFIX 支持，构建时 sed 替换 env 前缀

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 8: 全量验证与收尾

- [ ] **Step 1: 默认前缀构建 + 测试**

```bash
make build
```
Expected: 编译通过，产物在 `build/`

```bash
make test
```
Expected: 全量测试通过

- [ ] **Step 2: 自定义前缀构建 + 测试**

```bash
CUSTOM_PREFIX=ZHOSCLAW_ CUSTOM_HOME=.zhosclaw CUSTOM_CMD=zhosclaw CUSTOM_APP=ZhosClaw make build
```
Expected: 编译通过

```bash
# 测试使用自定义前缀的代码
CUSTOM_PREFIX=ZHOSCLAW_ CUSTOM_HOME=.zhosclaw CUSTOM_CMD=zhosclaw CUSTOM_APP=ZhosClaw make test
```
Expected: 全量测试通过（若有测试失败需检查并修复）

- [ ] **Step 3: 二进制泄露检查**

```bash
# 统计 PICOCLAW_ 在二进制中的出现次数。
# GetEnv/LookupEnv 中 2 处 + envOptions 中 2 处 = 最多 4 处有意保留的 fallback 字符串。
# 若超过 4 次说明 struct tag 或其他位置未被正确替换。
COUNT=$(strings build/zhosclaw | grep -c 'PICOCLAW_' || true)
if [ "$COUNT" -le 4 ]; then
    echo "PASS: PICOCLAW_ occurrences: $COUNT (accepted)"
else
    echo "FAIL: PICOCLAW_ found $COUNT times (max 4 allowed)"
    exit 1
fi
```

- [ ] **Step 4: 源码干净检查**

```bash
# struct tag 中的 envPrefix 保持默认值（未被 sed 修改）
grep -rn 'envPrefix:"PICOCLAW_' pkg/config/
```
Expected: 有输出（源码中 struct tag 保持默认值）

```bash
# 非 struct-tag 的 PICOCLAW_ 只应在 pkg/env.go 和 pkg/config/envkeys.go 的 envOptions() 中出现
grep -rn 'PICOCLAW_' pkg/ cmd/ web/backend/ --include='*.go' | grep -v 'envPrefix:"PICOCLAW_' | grep -v 'pkg/env.go' | grep -v 'pkg/config/envkeys.go'
```
Expected: 零结果（或仅注释中出现）

- [ ] **Step 5: 新建验证脚本并运行**

创建 `scripts/verify-branding.sh`:

```bash
#!/bin/bash
set -e
BIN="build/zhosclaw"  # BINARY_NAME 默认值
echo "=== 1. Building with default prefix ==="
make build
echo "=== 2. Running tests with default prefix ==="
make test
echo "=== 3. Building with custom prefix ==="
CUSTOM_PREFIX=ZHOSCLAW_ CUSTOM_HOME=.zhosclaw CUSTOM_CMD=zhosclaw make build
echo "=== 4. Binary residue check ($BIN) ==="
# GetEnv 2 处 + envOptions 2 处 = 最多 4 处有意保留的 fallback 字符串
COUNT=$(strings "$BIN" | grep -c 'PICOCLAW_' || true)
if [ "$COUNT" -le 4 ]; then
    echo "PASS: PICOCLAW_ occurrences: $COUNT"
else
    echo "FAIL: PICOCLAW_ found $COUNT times (max 4 allowed)"
    exit 1
fi
echo "=== All checks passed ==="
```

```bash
chmod +x scripts/verify-branding.sh
bash scripts/verify-branding.sh
```
Expected: All checks pass

- [ ] **Step 6: 更新 BACKLOG 验收标准并提交**

```bash
git add scripts/verify-branding.sh
git commit -m "feat: 添加品牌替换验证脚本

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### 完成标准

- [ ] `make build` 默认前缀编译 + 测试全通过
- [ ] `CUSTOM_PREFIX=ZHOSCLAW_ make build` 自定义前缀编译 + 测试全通过
- [ ] 自定义前缀二进制中 `PICOCLAW_` 字符串零结果（仅 fallback 函数中保留）
- [ ] 旧 `PICOCLAW_*` env 仍可正常识别（向下兼容验证）
- [ ] 二进制名由 `CUSTOM_CMD` 控制
