# Backlog

> 当前迭代: sprint_007

## 待规划

### BL-010 · Launcher TLS 支持 + 自签名证书自动签发
- **意图**: Launcher 启动时自动生成自签名证书，启用 HTTPS 端口（18443），解决 RK3506 设备 HTTP 非 localhost 下浏览器阻止 getUserMedia/AudioWorklet 导致语音录音不可用的问题
- **方案方向**: Go `crypto/tls` + `crypto/x509` 标准库在启动时自动生成 ECDSA P256 自签名证书，SAN 绑定设备 IP，缓存到 `<home>/tls/`；双端口并存（HTTP 18800 + HTTPS 18443）；前端 VoiceRecorder 补 `isSecureContext` 检测
- **设计文档**: .scrum/specs/2026-06-02-launcher-tls-design.md
- **实现计划**: .scrum/plans/2026-06-02-launcher-tls.md
- **验收标准**:
  - [ ] `-public` 开启时自动生成自签名证书，缓存到 `<home>/tls/`
  - [ ] `-public=false` 时不启动 HTTPS，不生成证书
  - [ ] `-no-tls` 显式关闭 HTTPS
  - [ ] `-tls-port == -port` 启动直接 fatal
  - [ ] HTTPS 端口 18443（可通过 `-tls-port` 修改）
  - [ ] HTTP 和 HTTPS 共用 netbind.Plan，双端口行为一致
  - [ ] 设备 IP 集合未变时，启动 5 次仍使用同一证书
  - [ ] 设备新增 LAN IPv4 时证书自动重新生成
  - [ ] link-local / IPv6 GUA/ULA 变化不触发重生成
  - [ ] 时钟未同步（RTC < 2020）时生成 NotAfter ≥ 2099 的 fallback 证书
  - [ ] NTP 同步后重启，检测 clock_fallback → 自动替换为正常证书
  - [ ] 多网卡设备从任一 LAN IP 访问 HTTPS 不报 hostname mismatch
  - [ ] 18443 被占用时打印明确错误，不静默失败
  - [ ] HTTPS server 参与 graceful shutdown
  - [ ] `/api/system/version` 包含 `https_url` 字段
  - [ ] 前端非安全上下文显示 disabled 麦克风 + 从 `/api/system/version` 读取端口
  - [ ] 前端无硬编码 `http://` 自引用导致 mixed-content
  - [ ] 浏览器访问 `https://IP:18443` 录音功能正常工作
  - [ ] 控制台和文件日志同时打印 HTTPS 入口地址
  - [ ] 部署脚本无需额外证书推送步骤

## 已交付

### BL-009 · 流式语音识别 + 播放交互优化
- **意图**: ASR 从「录完上传→一次性返回」升级为实时流式识别（边说边出字），对接火山引擎 WebSocket 协议；TTS 播放按钮从气泡底部行内移到右上角与复制按钮并排
- **方案方向**: 后端新增 `pkg/audio/asr/streaming.go` 流式接口 + 火山引擎 WebSocket 实现 + `GET /api/voice/stream` WebSocket 端点；前端 VoiceRecorder 改用 AudioContext 采集 PCM、WebSocket 推送音频块、流式更新输入框；AudioPlayer 移入 assistant-message 右上角
- **设计文档**: .scrum/specs/2026-06-01-streaming-asr-design.md
- **实现计划**: .scrum/plans/2026-06-01-streaming-asr.md
- **已完成于**: sprint_006.md
- **验收标准**:
  - [x] 点🎤立即录音，按钮脉冲动画 + 计时，说话时输入框实时出字
  - [x] 点发送/Enter 停止录音并发送；再点🎤停止录音文字留输入框
  - [x] 60s 自动停止
  - [x] HTTP `/api/voice/transcribe`（SiliconFlow）并行可用
  - [x] TTS 播放按钮在消息气泡右上角，与复制按钮并排

## 已交付

### BL-008 · 语音交互支持
- **意图**: Agent 支持语音输入（STT）和语音输出（TTS），Web UI 和 CLI 均可使用
- **方案方向**: 接入语音识别/合成 API（如 OpenAI Whisper/TTS、Azure Speech），Web 前端增加录音按钮和音频播放组件；pkg/providers 新增 speech provider 抽象
- **设计文档**: .scrum/specs/2026-05-28-web-voice-support-design.md
- **实现计划**: .scrum/plans/2026-05-28-web-voice-support.md
- **已完成于**: sprint_005.md
- **验收标准**:
  - [x] Web 对话页支持语音输入（点击录音 → STT → 填入输入框）
  - [x] Agent 回复支持语音朗读（TTS → 音频播放）
  - [x] 语音识别支持中文

### BL-007 · RK3588 一键部署
- **意图**: 一条命令完成构建→推送→初始化→启动，解决设备无 /etc/hosts、无 CA 证书、toybox 环境等兼容性问题
- **方案方向**: 完善 `scripts/deploy-rk3588.sh`，补全启动脚本，自动配置 SSL_CERT_FILE、生成适合设备的 config.json
- **设计文档**: -
- **实现计划**: -
- **已完成于**: 直接验收
- **验收标准**:
  - [x] `DEVICE_IP=x.x.x.x ./deploy-rk3588.sh` 一键部署并启动
  - [x] 部署后浏览器登录、WebSocket 连接、Agent 对话均正常
  - [x] 设备重启后服务自动启动

### BL-006 · Web 前端品牌定制
- **意图**: Web UI 中所有用户可见的品牌文字（标题、文案、组件、i18n）替换为当前品牌值（ZaiAgent），logo 图片替换，外部文档链接移除
- **方案方向**: index.html 标题、i18n 文案、组件内硬编码文字、路径占位符全部修改；logo_with_text.png 替换；docs.picoclaw.io 链接删除；localStorage key 和 data 属性保持不动
- **设计文档**: .scrum/specs/2026-05-27-custom-branding-design.md
- **实现计划**: .scrum/plans/2026-05-27-custom-branding.md
- **已完成于**: sprint_004.md
- **验收标准**:
  - [x] 页面标题为 ZaiAgent
  - [x] UI 中不再出现 "PicoClaw" 文字
  - [x] 外部文档链接已移除
  - [x] `~/.picoclaw` 路径占位符替换为 `~/.zaiagent`
  - [x] MQTT 默认 topic 前缀替换为 `/zaiagent`
  - [x] localStorage key 和 data 属性未被改动

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
- **设计文档**: `.scrum/specs/2026-05-25-openharmony-l1-rk3506-design.md`
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
- **设计文档**: `.scrum/specs/2026-05-27-custom-branding-design.md`
- **已完成于**: sprint_003.md
- **验收标准**:
  - [x] `make build` 默认前缀编译通过
  - [x] `CUSTOM_PREFIX=PICOCLAW_ make build` 还原上游品牌编译通过
  - [x] 二进制泄露检查：Struct tag 全部替换，fallback 函数有保留的旧前缀字符串（符合设计）
  - [x] 旧前缀 `PICOCLAW_*` env 仍能正常识别（向下兼容）
  - [x] Web UI 二进制名由 `pkg.CommandName` 控制

