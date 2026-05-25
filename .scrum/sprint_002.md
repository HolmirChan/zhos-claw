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
  - [x] 启动 picoclaw-launcher（自动管理 gateway，无需手动启动 gateway）
  - [x] 浏览器打开 localhost:18800，确认聊天界面加载
  - [x] 发送「执行 echo hello_rk3506_test」，确认 Agent 回复包含该字符串
  - [x] 停止本机进程
- **阻塞**: SB-002 完成后
- **备注**: 发现并修复 rebrand 遗留 bug（FindPicoclawBinary 硬编码 "picoclaw"）；token 由 gateway 运行时生成写入 PID 文件，launcher 自动读取，config.json 无需手动配置

### SB-005 · RK3506 真机部署验证 [~]
- **来源**: BL-004
- **子任务**:
  - [ ] 将 RK3506 接入局域网，确认 SSH 可达（`ssh root@<DEVICE_IP>`）
  - [ ] 在 RK3506 上执行 `picoclaw onboard`（或手动初始化 `~/.zhosclaw/config.json`，填入 API Key）
  - [ ] 执行 `DEVICE_IP=<ip> bash scripts/deploy-rk3506.sh`，确认 4 个步骤无报错
  - [ ] 局域网另一台设备浏览器打开 `http://<DEVICE_IP>:18800`，确认聊天界面加载
  - [ ] 发送「执行 ls /tmp」，确认 Agent 返回目录列表
- **阻塞**: 需要 RK3506 实机
