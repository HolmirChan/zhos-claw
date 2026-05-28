# Web 语音交互支持

## 概述

为 Web UI 新增语音输入（ASR）和语音输出（TTS）能力。输入和输出独立切换，用户可自由组合：文字/语音输入 × 文字/语音输出。

## 用户行为

| 输入方式 | 输出方式 | 效果 |
|---------|---------|------|
| 文字 | 文字 | 当前默认行为 |
| 文字 | 语音 | 发文字 → Agent 回复文字 → 前端调 TTS → 播放朗读 |
| 语音 | 语音 | 录音 → ASR 转文字 → Agent 回复文字 → 前端调 TTS → 播放朗读 |
| 语音 | 文字 | 录音 → ASR 转文字 → Agent 回复文字 → 文字显示 |

两个独立按钮：
- **麦克风按钮** — 管输入方式（默认文字，点击切换录音）
- **喇叭按钮** — 管输出方式（默认文字，开启后收到文字回复时自动调 TTS 播放）

## 架构

```
                         GET  /api/voice/capabilities     +----------+
    页面加载、探测能力 ──────────────────────────────────→ |          |
                                                          |  web/    |
                         POST /api/voice/transcribe       |  backend |
    浏览器录音 ──────────────────────────────────────────→ |  (HTTP)  |
                                                          |          |
                         POST /api/voice/synthesize       |          |
    收到回复、喇叭开启 ──────────────────────────────────→ |          |
                                                          |          |
                         GET  /api/voice/audio/{id}       |          |
    前端播放 ────────────────────────────────────────────→ |          |
                                                          +----------+
                                                               |
                                                每次请求 loadConfig / 创建 Provider
                                                               |
                                                          +----+----+
                                                          | pkg/audio|
                                                          | asr / tts|
                                                          +----------+
```

ASR 和 TTS 都是 **前端主动调 REST API**，不修改 WebSocket/gateway 消息管道。

- WebSocket 完全不动——照常透传文字消息
- TTS 不改消息协议——前端在收到文字回复后，若喇叭开启，调 `POST /api/voice/synthesize` 获取音频

## API 设计

所有 `/api/voice/` 端点复用 web/backend 现有 session 鉴权，未登录返回 401。

### 0. 能力探测

```
GET /api/voice/capabilities
```

- 响应: `{ "asr": true, "tts": false }`
- 实现: `config.LoadConfig` → `asr.DetectTranscriber` / `tts.DetectTTS` 判空
- 前端页面加载时调用，据此决定按钮显隐

### 1. ASR 端点

```
POST /api/voice/transcribe
```

- Content-Type: `multipart/form-data`
- 字段: `file` — 音频文件（WAV/WebM，限制 60 秒）
- 响应: `{ "text": "你好", "language": "zh" }`
- 错误: `{ "error": "未识别到语音" }` (text 为空时)

### 2. TTS 端点

```
POST /api/voice/synthesize
Content-Type: application/json

请求: { "text": "要朗读的文字" }
响应: { "audio_url": "/api/voice/audio/tts-xxx.ogg" }
```

### 3. 音频文件服务

```
GET /api/voice/audio/{file_id}
```

- 对 `file_id` 做 `filepath.Base(file_id)` sanitize，防止路径穿越
- 从 `config.GetHome()/tts-cache/` 目录读取，`http.ServeFile` 服务
- 支持 Range 请求
- 定时清理：每次 TTS 请求时删除目录下超过 1 小时的旧文件

## 后端实现

### ASR 流程

```
POST /api/voice/transcribe
  → 保存音频到临时文件
  → cfg := config.LoadConfig(h.configPath)
  → trans := asr.DetectTranscriber(cfg)
  → trans.Transcribe(ctx, filePath)
  → 返回 { "text": "...", "language": "..." }
  → 清理临时文件
```

复用 `pkg/audio/asr/` 现有 Provider（Whisper、ElevenLabs），不新增 Provider。

### TTS 流程

```
POST /api/voice/synthesize { "text": "..." }
  → cfg := config.LoadConfig(h.configPath)
  → provider := tts.DetectTTS(cfg)
  → stream := provider.Synthesize(ctx, text)
  → ext := ".ogg"  (OpenAI / 默认), ".mp3" (Mimo)
  → os.MkdirAll(filepath.Join(config.GetHome(), "tts-cache"), 0700)
  → tmp := os.CreateTemp(config.GetHome()+"/tts-cache/", "tts-*"+ext)
  → io.Copy(tmp, stream)
  → 返回 { "audio_url": "/api/voice/audio/" + filepath.Base(tmp.Name()) }
```

- 不使用 `tts.SynthesizeAndStore`——不引入 MediaStore 依赖
- 直接写临时文件，文件名即为 file_id
- 每次请求重新 `config.LoadConfig` + `tts.DetectTTS`，无需缓存和变更检测

输出格式取决于 Provider：

| Provider | 输出格式 |
|---------|------|
| OpenAI TTS | audio/ogg (.ogg) |
| Mimo TTS | audio/mpeg (.mp3) |

两种格式现代浏览器 `<audio>` 标签均原生支持，不做格式转码。

### 临时文件清理

在 `POST /api/voice/synthesize` 处理开始时，遍历 `config.GetHome()/tts-cache/` 目录，删除修改时间超过 1 小时的文件。清理失败不影响请求。

> **部署注意**：TTS 缓存目录跟随工作目录（`config.GetHome()/tts-cache/`），嵌入式设备上工作目录有读写保证。旧文件 1 小时自动清理，空间可控。

### 降级策略

- TTS 生成失败 → 返回 HTTP 500，前端降级为纯文字，不阻塞消息
- ASR 返回空 → 返回 `{ "text": "" }`，前端提示「未识别到语音」
- 未配置 ASR/TTS Provider → 能力探测返回 false，前端隐藏对应按钮

## 前端实现

### 组件变化

- 输入框旁新增 **麦克风按钮**（开/关态）
- 输入框旁新增 **喇叭按钮**（开/关态）
- 消息气泡新增 **音频播放条**（audio_url 不为空时显示）

### 录音流程

```
点击麦克风 → 开始录音 (MediaRecorder API)
→ 再次点击 / 60 秒超时 → 停止录音
→ POST /api/voice/transcribe
→ 文字填入输入框 → 发送消息 (走现有 WebSocket)
```

### TTS 播放流程（异步追加）

```
WebSocket 收到 Agent 文字回复
→ 气泡立即渲染文字
→ 若喇叭开启：
    → 气泡显示 TTS loading 态（喇叭图标闪烁）
    → POST /api/voice/synthesize { text: "回复文字" }
    → 成功：loading 消失，音频播放条附加到气泡 → 自动播放
    → 失败：loading 消失，降级纯文字（无感知）
→ 播放完毕等待下一条
```

关键时序：气泡不是一次渲染完毕，而是**先渲染文字，再异步追加音频**。

## 边界情况

| 场景 | 处理 |
|------|------|
| 录音超过 60 秒 | 自动停止并发送 |
| ASR 返回空文本 | 前端提示「未识别到语音，请重试」 |
| TTS 合成失败 | 降级纯文字，不阻塞消息/UI |
| 中途切换喇叭/麦克风 | 当前消息不受影响，下条生效 |
| 浏览器不支持录音 | 隐藏麦克风按钮 |
| 音频文件加载失败 | 只显示文字，不阻塞 UI |
| 未配置 ASR/TTS Provider | 对应按钮隐藏 |

## 不涉及

- **CLI 语音交互** — 不在本次范围。CLI 是纯文本终端交互，语音输入/输出无实际用途；声卡依赖、平台兼容性（OpenHarmony 等无标准音频设备）复杂，收益为零。
- **Provider 新增/替换** — 复用现有 Whisper/ElevenLabs(ASR) 和 OpenAI/Mimo(TTS)
- **实时语音通道（WebRTC/RTP/Opus 管线）** — 不动现有实时管线
- **WebSocket/gateway 消息协议** — 零改动
- **音频格式转码** — 不做，Provider 输出什么就用什么
- **MediaStore / SynthesizeAndStore** — 不引入，直接临时文件
