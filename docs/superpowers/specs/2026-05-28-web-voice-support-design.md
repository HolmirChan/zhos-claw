# Web 语音交互支持

## 概述

为 Web UI 新增语音输入（ASR）和语音输出（TTS）能力。输入和输出独立切换，用户可自由组合：文字/语音输入 × 文字/语音输出。

## 用户行为

| 输入方式 | 输出方式 | 效果 |
|---------|---------|------|
| 文字 | 文字 | 当前默认行为 |
| 文字 | 语音 | 文字输入 → Agent 回复 → TTS 朗读 |
| 语音 | 语音 | 录音 → ASR 转文字 → Agent 回复 → TTS 朗读 |
| 语音 | 文字 | 录音 → ASR 转文字 → Agent 回复 → 文字显示 |

两个独立按钮：
- **麦克风按钮** — 管输入方式（默认文字，点击切换录音）
- **喇叭按钮** — 管输出方式（默认文字，点击切换语音朗读）

## API 设计

### 1. ASR 端点

```
POST /api/voice/transcribe
```

- Content-Type: `multipart/form-data`
- 字段: `file` — 音频文件（WAV/WebM，限制 60 秒）
- 响应: `{ "text": "你好", "language": "zh" }`
- 错误: `{ "error": "未识别到语音" }` (text 为空时)

### 2. 消息端点新增字段

在现有消息发送端点新增 `output_mode` 参数：

```
output_mode: "text" | "voice"
```

- `text`: 默认，只返回文字
- `voice`: 返回文字 + `audio_url`

`audio_url` 指向后端缓存的 TTS 音频文件，前端放入 `<audio>` 播放。

### 3. 音频文件服务

```
GET /api/voice/audio/{file_id}.opus
```

- 返回缓存的 TTS 音频
- 支持 Range 请求
- 文件定时清理（如 1 小时后过期）

## 后端实现

### ASR 流程

```
POST /api/voice/transcribe
  → 保存音频到临时文件
  → 调用已有 Transcriber.Transcribe(ctx, filePath)
  → 返回 { "text": "...", "language": "..." }
  → 清理临时文件
```

复用 `pkg/audio/asr/` 现有 Provider（Whisper、ElevenLabs），不新增 Provider。

### TTS 流程

```
消息处理流程中 output_mode == "voice" 时：
  → Agent 生成回复文字
  → 调用已有 TTSProvider.Synthesize(ctx, text) 生成音频
  → 缓存音频文件（临时目录）
  → 返回 { "text": "...", "audio_url": "/api/voice/audio/xxx.opus" }
```

复用 `pkg/audio/tts/` 现有 Provider（OpenAI TTS、Mimo TTS），不新增 Provider。

### 降级策略

- TTS 生成失败 → 降级为纯文字回复，不阻塞消息
- ASR 返回空 → 返回 `{ "text": "" }`，前端提示「未识别到语音」

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
→ 文字填入输入框 → 发送消息
```

### TTS 播放

```
消息返回 audio_url → <audio> 元素自动播放
→ 播放完毕等待下一条
```

## 边界情况

| 场景 | 处理 |
|------|------|
| 录音超过 60 秒 | 自动停止并发送 |
| ASR 返回空文本 | 前端提示「未识别到语音，请重试」 |
| TTS 生成失败 | 降级纯文字，不阻塞消息 |
| 中途切换喇叭/麦克风 | 当前消息不受影响，下条生效 |
| 浏览器不支持录音 | 隐藏麦克风按钮 |
| 音频文件加载失败 | 只显示文字，不阻塞 UI |
| 未配置 ASR/TTS Provider | 对应按钮灰色/隐藏 |

## 不涉及

- CLI 语音交互 — 不在本次范围
- Provider 新增/替换 — 复用现有 Whisper/ElevenLabs(ASR) 和 OpenAI/Mimo(TTS)
- 实时语音通道（WebRTC/RTP/Opus 管线）— 不动现有实时管线
