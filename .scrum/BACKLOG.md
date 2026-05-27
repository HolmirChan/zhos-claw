# Backlog

> 当前迭代: sprint_004.md

## 待规划

### BL-006 · Web 前端品牌定制
- **意图**: Web UI 中所有用户可见的品牌文字（标题、文案、组件、i18n）替换为当前品牌值（ZaiAgent），logo 图片替换，外部文档链接移除
- **方案方向**: index.html 标题、i18n 文案、组件内硬编码文字、路径占位符全部修改；logo_with_text.png 替换；docs.picoclaw.io 链接删除；localStorage key 和 data 属性保持不动
- **验收标准**:
  - [ ] 页面标题为 ZaiAgent
  - [ ] UI 中不再出现 "PicoClaw" 文字
  - [ ] 外部文档链接已移除
  - [ ] `~/.picoclaw` 路径占位符替换为 `~/.zaiagent`
  - [ ] MQTT 默认 topic 前缀替换为 `/zaiagent`
  - [ ] localStorage key 和 data 属性未被改动

## 已交付

### BL-001 · 跑通启动流程 — 理解 onboard → auth → gateway → agent 完整链路
- **意图**: 从零到能跟 Agent 对话，理解每条命令做了什么、产生了哪些文件、模块之间怎么连接
- **方案方向**: 追踪每条 CLI 命令的代码执行路径，顺带吃透 `pkg/config/`、`pkg/credential/`、`pkg/identity/` 基础设施包
- **已完成于**: sprint_001.md
- **验收标准**:
  - [x] 能画出 onboard 的完整数据流（嵌入 → 拷贝 → 加密写入）
  - [x] 理解 config 的 SecureString 三种格式（plaintext / file:// / enc://）和序列化流程
  - [x] 理解 credential.Resolver 的解析链和 credential.Encrypt 的密钥派生链
  - [x] 能画出 gateway 启动时的模块依赖图（channels 注册、pid 锁、bus 初始化）
  - [x] 能画出 agent 的 pipeline 请求处理流程（setup → llm → execute → streaming → finalize）

### BL-004 · OpenHarmony L1 (RK3506) 适配 MVP
- **意图**: 让 ZhosClaw 在 RK3506（ARM Cortex-A7，Linux）上运行，局域网内浏览器访问 Web UI，自然语言驱动 Agent 执行 shell 命令
- **方案方向**: 新增 `build-rk3506` Makefile target（GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0）+ 一键部署脚本；利用已有 ExecTool 实现 shell 执行；Web UI 使用现有 picoclaw-launcher（后续重命名为 zhosclaw-web）
- **设计文档**: `docs/superpowers/specs/2026-05-25-openharmony-l1-rk3506-design.md`
- **已完成于**: sprint_002.md
- **验收标准**:
  - [x] `make build-rk3506` 产出 4 个产物（2 个 ARM 二进制 + 2 个本机调试二进制）
  - [x] 本机运行后 `localhost:18800` 聊天界面可用，发「执行 echo hello_rk3506_test」Agent 返回结果
  - [x] `deploy-rk3506.sh` 推包到设备后，局域网浏览器访问 RK3506 IP:18800 可用
  - [x] 发「执行 ls /tmp」Agent 返回目录列表（Agent 绕过安全限制成功执行）
  - [x] Web UI 二进制重命名为 `zhosclaw-web`（含平台变体），`appName` 引用 `pkg.AppName`

### BL-005 · 品牌一键替换 — 定制小龙虾
- **意图**: 在 `pkg/env.go` 改一处常量 + Makefile 变量，`make build` 产出完全换牌的二进制（env 前缀、二进制名、默认目录、显示名）
- **方案方向**: `pkg/env.go` 集中管理品牌变量；所有 env 读取统一走 `pkg.GetEnv()` 支持向下兼容；struct tag 通过构建时 sed 替换；Makefile 加 `CUSTOM_PREFIX` 变量
- **设计文档**: `docs/superpowers/specs/2026-05-27-custom-branding-design.md`
- **已完成于**: sprint_003.md
- **验收标准**:
  - [x] `make build` 默认前缀编译通过
  - [x] `CUSTOM_PREFIX=PICOCLAW_ make build` 还原上游品牌编译通过
  - [x] 二进制泄露检查：Struct tag 全部替换，fallback 函数有保留的旧前缀字符串（符合设计）
  - [x] 旧前缀 `PICOCLAW_*` env 仍能正常识别（向下兼容）
  - [x] Web UI 二进制名由 `pkg.CommandName` 控制


