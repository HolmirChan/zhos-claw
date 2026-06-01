# Sprint 5

> 创建: 2026-05-29
> 完成: 2026-06-01
> 目标版本: -
> 来源: BACKLOG.md（BL-008）
> 设计文档: .scrum/specs/2026-05-28-web-voice-support-design.md
> 实现计划: .scrum/plans/2026-05-28-web-voice-support.md
> 状态: 进行中

## 任务清单

### SB-001 · 语音端点 handler [x]
- **来源**: BL-008
- **子任务**:
  - [x] 新建 `web/backend/api/voice.go`：handleVoiceCapabilities / handleVoiceTranscribe / handleVoiceSynthesize / handleVoiceAudio + writeJSON / cleanTTSCache 辅助函数
  - [x] 编译验证：`go build -tags goolm,stdjson ./web/backend/...`
  - [x] 提交
- **阻塞**: -

### SB-002 · 注册语音路由 [x]
- **来源**: BL-008
- **子任务**:
  - [x] 修改 `web/backend/api/router.go`：新增 `registerVoiceRoutes` 方法，注册 4 个 `/api/voice/` 端点
  - [x] 确认 auth middleware 覆盖（非 public path，自动鉴权）
  - [x] 编译验证：`go build -tags goolm,stdjson ./web/backend/...`
  - [x] 提交
- **阻塞**: SB-001

### SB-003 · 前端 voice API 客户端 [x]
- **来源**: BL-008
- **子任务**:
  - [x] 新建 `web/frontend/src/api/voice.ts`：fetchVoiceCapabilities / transcribeAudio / synthesizeSpeech（使用 launcherFetch）
  - [x] 确认 AbortSignal.timeout 超时支持
  - [x] 提交
- **阻塞**: -

### SB-004 · voice Jotai store + useVoice hook [x]
- **来源**: BL-008
- **子任务**:
  - [x] 新建 `web/frontend/src/store/voice.ts`：voiceInputAtom / voiceOutputAtom / voiceAsrAvailableAtom / voiceTtsAvailableAtom
  - [x] 新建 `web/frontend/src/hooks/use-voice.ts`：能力探测 + 状态管理 + toggle
  - [x] 提交
- **阻塞**: SB-003

### SB-005 · VoiceRecorder 组件 [x]
- **来源**: BL-008
- **子任务**:
  - [x] 新建 `web/frontend/src/components/chat/voice-recorder.tsx`：MediaRecorder 录音 → transcribeAudio → onTranscribed 回调
  - [x] 60 秒自动停止、Safari 降级 audio/mp4
  - [x] 编译验证：`cd web/frontend && npx tsc --noEmit`
  - [x] 提交
- **阻塞**: SB-003

### SB-006 · 聊天输入框集成（麦克风+喇叭按钮） [x]
- **来源**: BL-008
- **子任务**:
  - [x] 修改 `web/frontend/src/components/chat/chat-composer.tsx`：集成 useVoice hook + VoiceRecorder 组件，新增麦克风和喇叭按钮
  - [x] 验证 onSend 取值方式：input prop → onInputChange + onSend 安全
  - [x] 编译验证：`cd web/frontend && npx tsc --noEmit`
  - [x] 提交
- **阻塞**: SB-004, SB-005

### SB-007 · AudioPlayer + assistant-message TTS 集成 [x]
- **来源**: BL-008
- **子任务**:
  - [x] 新建 `web/frontend/src/components/chat/audio-player.tsx`：<audio> 播放条 + 播放/暂停按钮
  - [x] 修改 `web/frontend/src/components/chat/assistant-message.tsx`：新增 isComplete prop，消息完成后触发 TTS，异步追加音频
  - [x] 修改 `web/frontend/src/components/chat/chat-page.tsx`：传递 isComplete={!isTyping}
  - [x] 编译验证：`cd web/frontend && npx tsc --noEmit`
  - [x] 全量构建验证：`make build && make build-launcher`
  - [x] 提交
- **阻塞**: SB-003, SB-006

### SB-008 · ASR/TTS Provider 扩展 — 支持 SiliconFlow [x]
- **来源**: BL-008（Phase 4 阻塞发现）
- **子任务**:
  - [x] `pkg/audio/asr/asr.go`：`supportsWhisperTranscription` 和 `supportsAudioTranscription` 协议白名单各新增 `"siliconflow"`
  - [x] `pkg/audio/asr/asr.go`：`whisperModelID` 移除 model ID 必须含 `"whisper"` 的限制
  - [x] `pkg/audio/tts/openai_tts.go`：`response_format` 改为可配置（新增 `TTSFormat` 字段，默认 `"mp3"`）
  - [x] `pkg/config/config.go`：`VoiceConfig` 新增 `tts_format`、`tts_voice` 字段
  - [x] `pkg/audio/tts/tts.go`：`DetectTTS` / `providerFromModelConfig` 透传 format 和 voice 参数
  - [x] 编译验证：`go build -tags goolm,stdjson ./pkg/audio/... ./web/backend/...`
  - [x] Lint 验证：0 issues
  - [x] 提交（2 commits）
- **阻塞**: -

### SB-009 · 测试验收 [x]
- **来源**: BL-008
- **阻塞**: SB-001~SB-007 全部完成
- **子任务**:

  - [x] **Phase 1：代码级验证**（Claude 独立执行）
    - `make lint` 零告警
    - `go vet -tags goolm,stdjson ./web/backend/...` 零输出
    - `cd web/frontend && npx tsc --noEmit` 零错误
    - 确认新增文件清单：`voice.go` / `voice.ts` / `store/voice.ts` / `hooks/use-voice.ts` / `voice-recorder.tsx` / `audio-player.tsx`
    - 确认修改文件清单：`router.go` / `chat-composer.tsx` / `assistant-message.tsx` / `chat-page.tsx`

  - [x] **Phase 2：启动服务**（用户配合）
    - 用户执行 `make build && make build-launcher` 构建全量二进制
    - 用户启动 launcher 服务（按现有方式启动 `zhosclaw-web`）
    - Claude 确认服务端口可访问：`curl -s http://localhost:18800/api/voice/capabilities`

  - [x] **Phase 3：API 端点 — 错误路径**（Claude curl 执行，无需配置 Provider）
    - `POST /api/voice/transcribe` 无 file 字段：`curl -s -X POST http://localhost:18800/api/voice/transcribe` → 400 + `Content-Type: application/json` + body 含 `"error"`
    - `POST /api/voice/transcribe` 超 10MB 请求体：`dd if=/dev/zero bs=1M count=11 | curl -s -X POST http://localhost:18800/api/voice/transcribe -F "file=@-"` → 400 + JSON error
    - `POST /api/voice/synthesize` 空 body：`curl -s -X POST http://localhost:18800/api/voice/synthesize -H "Content-Type: application/json" -d '{}'` → 400 + JSON error 含 "缺少 text"
    - `POST /api/voice/synthesize` 无 Provider：`curl -s -X POST http://localhost:18800/api/voice/synthesize -H "Content-Type: application/json" -d '{"text":"你好"}'` → 501 + JSON error 含 "未配置 TTS"
    - `GET /api/voice/capabilities` 无 Provider 时：`curl -s http://localhost:18800/api/voice/capabilities` → 200 + `{"asr":false,"tts":false}`
    - `GET /api/voice/audio/../../../etc/passwd`：→ 400 + JSON error
    - `GET /api/voice/audio/`（空 file_id）：→ 400 + JSON error

  - [x] **Phase 4：配置 Provider**（用户配合）
    - 用户在 `config.json` 中新增 ASR model（如 whisper-1 / groq-whisper）
    - 用户在 `config.json` 中新增 TTS model（如 tts-1 / mimo-tts），或复用已有
    - 用户重启 launcher 服务
    - Claude 确认：`curl -s http://localhost:18800/api/voice/capabilities` → `{"asr":true,"tts":true}`（或至少一个为 true）

  - [x] **Phase 5：API 端点 — 正常路径**（Claude curl 执行）
    - `POST /api/voice/transcribe` 上传有效音频：生成 1 秒静音 WebM → curl 上传 → 返回 200 + JSON 含 `"text"` 字段（可能为空文本，此时应同时含 `"error":"未识别到语音"`）
    - `POST /api/voice/synthesize` 合成语音：`curl -s -X POST http://localhost:18800/api/voice/synthesize -H "Content-Type: application/json" -d '{"text":"你好世界"}'` → 200 + JSON 含 `"audio_url": "/api/voice/audio/..."`
    - `GET /api/voice/audio/{file_id}` 播放音频：用上一步返回的 file_id → `curl -s -o /tmp/test-audio.ogg http://localhost:18800/api/voice/audio/{file_id}` → 200 + 文件大小 > 0 + `file /tmp/test-audio.ogg` 识别为 Ogg/MP3

  - [x] **Phase 6：前端功能验收**（用户浏览器操作，Claude 观察分析）
    - **能力探测 & 按钮显隐**：打开 http://localhost:18800 → 聊天页输入框左侧出现麦克风按钮和喇叭按钮（Provider 已配置时）
    - **语音输入流程**：点击麦克风 → 浏览器弹权限请求 → 允许 → 录音中显示秒数 → 再次点击或等 60s → 自动发送消息，输入框出现识别文字，Agent 正常回复
    - **语音输出流程**：点击喇叭开启（高亮态）→ 发一条文字消息 → Agent 回复后，气泡底部出现音频播放条 → 音频自动播放
    - **关闭语音输出**：点击喇叭关闭 → 发消息 → 不触发 TTS，气泡无播放条
    - **中文识别**：点麦克风 → 说一段中文（如"今天天气怎么样"）→ 停止 → 输入框出现正确的中文 → 发送后 Agent 正常回复

  - [x] **Phase 7：边界情况**（用户浏览器操作 + Claude curl 配合）
    - **空语音**：不说话直接停止录音 → 前端应提示「未识别到语音」或静默不发送（不出现空白消息）
    - **TTS 失败降级**：Claude 删除 tts-cache 目录或制造权限问题 → 发消息并开启喇叭 → 消息正常显示文字，不因 TTS 失败而阻塞
    - **切换喇叭不触发历史消息**：关闭喇叭 → 发 3 条消息 → 开启喇叭 → 已有消息不追加音频，仅新消息触发
    - **Safari 兼容**（如有条件）：Safari 浏览器打开 → 录音正常 → 格式为 audio/mp4 → ASR 正常识别
    - **音频 404**：Claude 手动删除某条 TTS 缓存文件 → 用户点播放 → 不阻塞 UI，仅音频加载失败

  - [x] **Phase 8：回归检查**（用户浏览器操作）
    - 文字输入 + Enter 发送正常
    - Shift+Enter 换行正常
    - 图片附件上传/预览/发送正常
    - Agent 流式回复正常显示
    - 未配置 Provider 时界面干净（无残影按钮、无报错）
