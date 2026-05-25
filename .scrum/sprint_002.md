# Sprint 2

> 创建: 2026-05-25
> 目标版本: -
> 来源: BACKLOG.md（BL-004）
> 状态: 进行中

## 任务清单

### SB-001 · 提交设计文档与 BACKLOG 条目 [x]
- **来源**: BL-004
- **子任务**:
  - [x] 在 BACKLOG.md 「待规划」区写入 BL-004 条目
  - [x] `git add` 设计文档 + BACKLOG.md 并提交
- **阻塞**: -

### SB-002 · 新增 build-rk3506 Makefile target [x]
- **来源**: BL-004
- **子任务**:
  - [x] 验证 `make build-rk3506` 当前报错（no rule）
  - [x] 在 `build-pi-zero` 之后插入新 target（依赖 build-linux-arm + build-launcher-frontend，额外交叉编译 launcher）
  - [x] 运行 `make build-rk3506`，验证 4 个产物生成
  - [x] 验证 `file bin/zhosclaw-linux-arm` 输出 ELF 32-bit ARM
  - [x] 提交
- **阻塞**: -

### SB-003 · 新建 scripts/deploy-rk3506.sh [x]
- **来源**: BL-004
- **子任务**:
  - [x] 创建脚本（build → scp → ssh restart → 打印访问地址）
  - [x] `chmod +x`
  - [x] `bash -n` 语法检查
  - [x] 提交（含 SSH_OPTS 超时保护、日志追加模式修复）
- **阻塞**: -

### SB-004 · 本机冒烟验证 [x]
- **来源**: BL-004
- **子任务**:
  - [x] 确认本机 config 已 onboard（或执行 onboard）
  - [x] 启动 zhosclaw-web（自动管理 gateway，无需手动启动 gateway）
  - [x] 浏览器打开 localhost:18800，确认聊天界面加载
  - [x] 发送「执行 echo hello_rk3506_test」，确认 Agent 回复包含该字符串
  - [x] 停止本机进程
- **阻塞**: SB-002 完成后
- **备注**: 发现并修复 rebrand 遗留 bug（FindPicoclawBinary 硬编码 "picoclaw"）；token 由 gateway 运行时生成写入 PID 文件，launcher 自动读取，config.json 无需手动配置；SB-006 完成后以 `./build/zhosclaw-web` 重新验证通过

### SB-006 · 将 Web UI 二进制重命名为 zhosclaw-web [x]
- **来源**: BL-004
- **改动范围**:
  - `web/Makefile` — `OUTPUT` 默认值从 `picoclaw-launcher` 改为 `zhosclaw-web`
  - 根 `Makefile` — 所有 `picoclaw-launcher*` 输出文件名改为 `zhosclaw-web*`（含 build-rk3506 target）
  - `web/backend/main.go:40` — `appName = "PicoClaw"` 改为 `appName = pkg.AppName`（需 import `github.com/sipeed/picoclaw/pkg`）
  - `scripts/deploy-rk3506.sh` — scp 源文件名从 `picoclaw-launcher-linux-arm` 改为 `zhosclaw-web-linux-arm`
- **子任务**:
  - [x] 修改 `web/Makefile`
  - [x] 修改根 `Makefile`（build-launcher、build-rk3506 及相关 echo）
  - [x] 修改 `web/backend/main.go` appName（`"PicoClaw"` → `pkg.AppName`）
  - [x] 修改 `scripts/deploy-rk3506.sh`
  - [x] `make build-rk3506`，确认产物名称正确
  - [x] 提交
- **阻塞**: -

### SB-005 · RK3506 真机部署验证 [x]
- **来源**: BL-004
- **子任务**:
  - [x] 将 RK3506 接入局域网，确认 SSH 可达（hdc shell）
  - [x] 在 RK3506 上执行 `./zhosclaw-linux-arm onboard`，填入 API Key（config.json 写到当前目录）
  - [x] 创建软链接 `ln -s /data/zhosclaw/zhosclaw-linux-arm /data/zhosclaw/zhosclaw`（供 launcher 自动拉起 gateway）
  - [x] 启动服务：`PICOCLAW_HOME=/data/zhosclaw /data/zhosclaw/zhosclaw-web-linux-arm -public`
  - [x] 局域网浏览器打开 `http://192.168.18.250:18800`，确认聊天界面加载
  - [ ] 发送「执行 ls /tmp」，确认 Agent 返回目录列表（待 API Key 配置完成后验证）
- **阻塞**: 需要 RK3506 实机
- **备注**:
  - 设备无 `$HOME`，`GetHome()` 兜底为 `.`，两个进程若从不同目录启动会找不到对方的 PID 文件
  - 必须设 `PICOCLAW_HOME` 且只启动 launcher，launcher 会带相同环境变量拉起 gateway，token 才能一致
  - `model_list` 里 API Key 字段名是 `api_keys`（数组），不是 `api_key`
  - `-public` 参数让 Web UI 监听 `0.0.0.0`，局域网才可访问
