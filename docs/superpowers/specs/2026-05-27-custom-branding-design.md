# 定制小龙虾 — 品牌一键替换设计

> 创建: 2026-05-27
> 状态: 草案

## 目标

在 `pkg/env.go` 中改一处值，配合 Makefile 变量，`make build` 产出的二进制完成全面换牌。源码始终保持默认品牌（PICOCLAW_ / picoclaw）不变，构建时才注入新品牌。

## 不做

- Go 模块路径 `github.com/sipeed/picoclaw` 不动（用户感知不到）
- 文档、README、注释等非代码文件不参与构建时替换

## 品牌控制中心：`pkg/env.go`

新增和整合以下变量：

```go
var (
    // Logo 是终端展示的 emoji 图标。ldflags 可覆盖。
    Logo = "🦞"

    // AppName 是用户可见的应用显示名称。ldflags 可覆盖。
    AppName = "PicoClaw"

    // EnvPrefix 是所有环境变量的前缀。ldflags 可覆盖。
    EnvPrefix = "PICOCLAW_"

    // DefaultHome 是默认配置目录名（相对于用户 home）。ldflags 可覆盖。
    DefaultHome = ".picoclaw"

    // CommandName 是 CLI 子命令名称（小写），也影响二进制文件名。ldflags 可覆盖。
    CommandName = "picoclaw"
)
```

> **注意**：全部用 `var` 而非 `const`，因为 `-ldflags "-X"` 只能覆盖 `var`。

## Env 变量统一入口：`pkg.GetEnv()` / `pkg.LookupEnv()`

```go
// GetEnv 读取带配置前缀的环境变量，新前缀优先，旧前缀兜底。
func GetEnv(suffix string) string {
    if v := os.Getenv(EnvPrefix + suffix); v != "" {
        return v
    }
    return os.Getenv("PICOCLAW_" + suffix)
}

// LookupEnv 同 GetEnv 但报告 key 是否存在。空值语义与 os.LookupEnv 一致。
func LookupEnv(suffix string) (string, bool) {
    if v, ok := os.LookupEnv(EnvPrefix + suffix); ok {
        return v, true
    }
    return os.LookupEnv("PICOCLAW_" + suffix)
}
```

存量代码中 `os.Getenv` / `os.LookupEnv` 全部替换为这两个函数调用。

> **不参与品牌替换的内部标识符**：Cookie 名（`picoclaw_launcher_auth`）、macOS LaunchAgent label（`io.picoclaw.launcher`）、autostart desktop 文件名等持久化系统标识，换牌后保持不变。否则会导致已登录用户被踢出、LaunchAgent 重复注册等破坏性问题。

## struct tag 的 `envPrefix`：构建时替换 + 运行时兼容

`pkg/config/config.go` 等文件中的 struct tag `envPrefix:"PICOCLAW_XXX_"` 无法引用 Go 变量。策略：

1. **源码中**：struct tag 保持 `PICOCLAW_` 前缀不动
2. **构建时**：Makefile 将源码拷贝到临时路径，对其中的 `.go` 文件执行精确 sed 替换，只替换 `envPrefix:"PICOCLAW_` 模式（不碰其他 `PICOCLAW_` 字符串，如 `pkg/env.go` 中的默认值和 fallback 函数）
3. **编译后**：临时目录删除，源码不变

### 向下兼容：env 映射 fallback

sed 替换后 struct tag 查找的是新前缀（如 `ZHOSCLAW_HOME`），但用户可能仍设了旧前缀 `PICOCLAW_HOME`。为此，`env.Parse()` 的两处调用点（`config.go:1413`、`config_channel.go:729`）改为 `env.ParseWithOptions()`，传入自定义 `Options.Environment`：

```go
func envOptions() env.Options {
    envMap := toMap(os.Environ()) // 当前所有 env
    // 旧前缀 fallback：PICOCLAW_* → 新前缀_*
    for _, e := range os.Environ() {
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

效果：新前缀优先，旧前缀兜底，两套 env 都能用。

## Makefile 改动

新增品牌变量 + 跨平台 sed + 校验 + 精确 struct tag 替换宏：

```makefile
CUSTOM_PREFIX ?= PICOCLAW_
CUSTOM_HOME   ?= .picoclaw
CUSTOM_CMD    ?= picoclaw
CUSTOM_APP    ?= PicoClaw
CUSTOM_LOGO   ?= 🦞

# 跨平台 sed in-place 选项
SED_INPLACE := $(if $(shell sed --version 2>/dev/null | head -1 | grep -qi gnu && echo 1),-i,-i '')

# 校验 CUSTOM_PREFIX 不含 sed 特殊字符
define validate-prefix
	@case "$(CUSTOM_PREFIX)" in */*|*\&*) echo "ERROR: invalid char in CUSTOM_PREFIX" >&2; exit 1 ;; esac
endef

# 编译宏：临时副本中精确替换 struct tag 前缀，PID 隔离防并发
define build-with-custom-prefix
	$(call validate-prefix)
	@mkdir -p $(BUILD_DIR)/custom-build.$$$$
	@cp -r cmd pkg web go.mod go.sum $(BUILD_DIR)/custom-build.$$$$/
	@if [ "$(CUSTOM_PREFIX)" != "PICOCLAW_" ]; then \
		find $(BUILD_DIR)/custom-build.$$$$ -name '*.go' \
			-exec sed $(SED_INPLACE) 's/envPrefix:"PICOCLAW_/envPrefix:"$(CUSTOM_PREFIX)/g' {} + ; \
	fi
	cd $(BUILD_DIR)/custom-build.$$$$ && $(GO) build ... -o $(1) ./cmd/picoclaw
	@rm -rf $(BUILD_DIR)/custom-build.$$$$
endef
```

LDFLAGS 追加品牌变量注入：`-X github.com/sipeed/picoclaw/pkg.EnvPrefix=$(CUSTOM_PREFIX)` 等 5 项。

## 改动范围概览

| 文件 | 改动 |
|------|------|
| `pkg/env.go` | 新增 `EnvPrefix`、`GetEnv()`、`LookupEnv()`；整合现有常量 |
| `pkg/config/envkeys.go` | `"PICOCLAW_HOME"` 等常量改为 `EnvPrefix + "HOME"` 拼接；新增 `envOptions()` 辅助函数 |
| `pkg/config/config.go` | `env.Parse(cfg)` → `env.ParseWithOptions(cfg, envOptions())` |
| `pkg/config/config_channel.go` | `env.Parse(target)` → `env.ParseWithOptions(target, envOptions())` |
| `pkg/config/gateway.go` | `os.Getenv("PICOCLAW_LOG_LEVEL")` → `pkg.GetEnv("LOG_LEVEL")` |
| `pkg/credential/credential.go` | env 读取改用 `pkg.GetEnv()` |
| `cmd/picoclaw/` | env 读取、硬编码名改用 `pkg` 常量 |
| `web/backend/utils/runtime.go` | `PICOCLAW_BINARY` 等改用 `pkg.GetEnv()` |
| Makefile | 新增 `CUSTOM_PREFIX`，构建流程加 sed 步骤 |
| `.goreleaser.yaml` | 参考 Makefile 调整 |

## 验证

1. `make build && make test` — 默认前缀全量通过
2. `CUSTOM_PREFIX=ZHOSCLAW_ make build && CUSTOM_PREFIX=ZHOSCLAW_ make test` — 自定义前缀全量通过
3. `strings bin/zhosclaw | grep PICOCLAW_` 零结果 — 二进制中无旧前缀
4. `grep -r "PICOCLAW_" pkg/ cmd/` 仅在定义处出现 — 源码级干净
