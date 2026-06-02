# HAP 语音接口对接说明

> 适用版本：ZhosClaw 0bbd64d5+
> 最后更新：2026-06-02

## 概述

ZhosClaw 通过后端 HTTP/WebSocket API 提供语音识别（ASR）和语音合成（TTS）能力，HAP 原生应用可直接调用，无需前端页面。

所有接口基于 `http://设备IP:18800`，需要携带 dashboard 登录后的 session cookie（`picoclaw_launcher_auth`）。

## 接口总览

| 方法 | 路径 | 用途 |
|------|------|------|
| GET | `/api/voice/capabilities` | 查询语音能力是否可用 |
| GET | `/api/voice/stream` | WebSocket 流式 ASR（边说边出字） |
| POST | `/api/voice/transcribe` | 录完上传一次性识别 |
| POST | `/api/voice/synthesize` | TTS 合成（文字→语音） |
| GET | `/api/voice/audio/{file_id}` | 下载合成的音频文件 |

---

## 错误响应矩阵

所有接口统一返回 JSON，格式 `{"error":"..."}`。

| 端点 | 状态码 | 场景 | 响应 |
|------|--------|------|------|
| transcribe | 400 | 文件超过 10 MB | `{"error":"文件过大或解析失败"}` |
| transcribe | 400 | 请求未携带 file 字段 | `{"error":"缺少音频文件"}` |
| transcribe | 200 | 识别完成但未检测到语音 | `{"text":"","error":"未识别到语音"}` |
| transcribe | 500 | ASR provider 调用失败 | `{"error":"语音识别失败: ..."}` |
| transcribe | 501 | 未配置 ASR provider | `{"error":"未配置语音识别"}` |
| synthesize | 400 | 请求 JSON 缺 text 字段 | `{"error":"缺少 text 参数"}` |
| synthesize | 500 | TTS 流程内部错误（合成/缓存目录/文件写入失败） | `{"error":"..."}` |
| synthesize | 501 | 未配置 TTS provider | `{"error":"未配置 TTS"}` |
| stream | 403 | WebSocket upgrade 被 CIDR 拒绝 | HTTP 403（WebSocket 未建立） |
| stream | error 帧 | 配置加载失败 | `{"type":"error","code":"config","message":"配置加载失败"}` |
| stream | error 帧 | 火山引擎连接失败 | `{"type":"error","code":"connect","message":"..."}` |
| stream | error 帧 | 未配置流式 ASR | `{"type":"error","code":"no_provider","message":"未配置流式 ASR"}` |
| capabilities | 200 | 任何情况 | 各字段 true/false（见下节） |
| audio | 400 | file_id 非法（含 .. 或空） | `{"error":"无效的文件 ID"}` |
| 全部 | 401 | 未携带或过期 session cookie | `{"error":"unauthorized"}` |

**注意 transcribe 的特殊情况：** HTTP 200 但 `error` 字段非空 + `text` 为空，表示识别完成但没检测到语音。HAP 应**优先检查 `text` 是否非空**来判断识别是否成功，而非仅判断 HTTP 状态码或 error 字段存在性。

---

## 1. 能力查询

```
GET /api/voice/capabilities
```

**请求：** 无参数，需携带 session cookie。

**响应：**

```json
{
  "asr": true,
  "tts": true,
  "streaming": true
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `asr` | bool | 一次性语音识别（POST /transcribe）是否可用 |
| `tts` | bool | 语音合成是否可用 |
| `streaming` | bool | 流式 ASR（WebSocket）是否可用 |

全 false 表示语音功能不可用（未配置或服务异常，无法区分具体原因），HAP 应当作语音功能关闭处理。

---

## 2. 流式 ASR（WebSocket）

> HAP 集成语音输入时推荐使用此接口，边说边出字。

### 连接

```
ws://设备IP:18800/api/voice/stream
```

WebSocket 不需要额外请求头，session 通过连接时的 cookie 传递。

若 launcher 启用 `-public` 且配置了 `allowed_cidrs`，HAP 调用 IP 必须在允许范围内，否则 WebSocket upgrade 会被拒绝（`CheckOrigin` 返回 false）。超时由 HAP 端自行控制，建议 60s 自动停止。

### 音频格式（客户端 → 服务端）

HAP 发送原始二进制 PCM 数据，每条 WebSocket binary message 为一段音频块。

| 参数 | 值 |
|------|-----|
| 编码 | PCM signed 16-bit little-endian |
| 采样率 | 16000 Hz |
| 位深 | 16 bit |
| 声道 | 1（mono） |
| 建议块大小 | ~100ms / 3200 bytes |

### 识别结果（服务端 → 客户端）

JSON 文本消息，三种 type：

```typescript
// partial: 实时识别片段（边说边出）
{ "type": "partial", "text": "你好", "definite": false }

// final: 识别结束后的聚合文本（仅一条）
{ "type": "final", "text": "你好世界" }

// error: 识别过程出错
{ "type": "error", "code": "connect", "message": "火山引擎连接失败" }

// 三种 error code：
//   config      — 配置加载失败
//   connect     — 火山引擎 WebSocket 建连失败
//   no_provider — 未配置流式 ASR provider
```

### partial / final 状态说明

| 消息 | 何时收到 | text 含义 | HAP 处理 |
|------|---------|-----------|----------|
| partial `definite=false` | 说话进行中 | 临时识别结果，可能随后续修正 | 灰色/半透明显示 |
| partial `definite=true` | 一句话断句完成 | 本句已锁定，不会变 | 追加到已确定文本区 |
| final | 连接关闭后 | 服务端聚合的最终完整文本 | 替换全部文本展示 |

HAP 不需要自己维护 locked/current 累加逻辑（服务端已做），直接用 final 的 text 当作最终结果。real-time 展示时用 partial。

### 连接终止流程

**正常关闭：**

```
HAP 关闭 WS
    │
    ├── 服务端发送结束帧给火山引擎 → 等最后一批结果
    ├── 发送 final 消息
    └── 服务端关闭 WS
```

HAP 在发起 close 后应继续监听，等待 final 消息。如果在合理时间内（≤5s）未收到 final 或连接已被对端关闭，以最后收到的 partial 作为降级结果。

**异常关闭：** 服务端可能因火山引擎连接断开等原因未发 final 就直接 close，HAP 端应设 ≤5s 超时兜底，超时后用已收到的 partial 拼接。

---

## 3. 一次性识别（HTTP 上传）

```
POST /api/voice/transcribe
Content-Type: multipart/form-data
```

**请求：** `multipart/form-data`，字段名 `file`，值为录音文件。单次上传上限 **10 MB**。

支持格式取决于 ASR provider 配置（当前 SiliconFlow 支持 WAV/MP3/FLAC 等常见格式）。

**成功响应（识别到语音）：**

```json
{
  "text": "你好世界",
  "language": "zh"
}
```

**成功响应（未检测到语音）：**

```json
{
  "text": "",
  "error": "未识别到语音"
}
```

> HAP 应用必须检查 `text` 字段非空来判断是否有识别结果，不能仅依赖 HTTP 200 或 error 字段。

**失败响应：**

```json
{"error": "语音识别失败: API error (status 404): "}
```

---

## 4. TTS 合成

```
POST /api/voice/synthesize
Content-Type: application/json
```

**请求：**

```json
{
  "text": "你好，有什么可以帮你？"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `text` | string | 是 | 要合成的文本 |

**响应：**

```json
{
  "audio_url": "/api/voice/audio/tts-xxxxxxxx.mp3"
}
```

### 下载音频

```
GET /api/voice/audio/{file_id}
```

完整的 HAP 端 URL 为 `http://设备IP:18800/api/voice/audio/tts-xxxxxxxx.mp3`。

**同样需要携带 session cookie**，否则返回 401。

### 缓存生命周期

合成后的音频文件存于 `<home>/tts-cache/`。每次调用 `synthesize` 时，服务端会自动清理 mtime 超过 1 小时的旧文件（`cleanTTSCache(dir, 1h)`）。HAP 应在收到 `audio_url` 后尽快下载——若不下载且超过 1 小时，且期间有新的合成调用触发清理，旧文件可能被删除。

---

## 5. 鉴权

所有接口需要携带 launcher dashboard 的 session cookie：

```
Cookie: picoclaw_launcher_auth=<token>
```

token 通过 dashboard 登录后由服务端 Set-Cookie 获取，具体登录接口见 dashboard 文档，不在本文档范围。

---

## 6. HAP 集成建议

### 完整交互流程

```
1. GET /api/voice/capabilities
   ├── streaming=true  → 走流式 ASR（步骤 2a）
   └── streaming=false → 走一次性识别（步骤 2b）

2a. 流式 ASR：
    ws://IP:18800/api/voice/stream
    → 持续发送 PCM binary chunks
    → 实时接收 partial（更新 UI）
    → HAP 关闭 WS
    → 等待 final（≤5s timeout）
    → 拿到最终文本

2b. 一次性识别（降级）：
    录完整段 → 存本地文件 → POST /api/voice/transcribe
    → 检查响应 text 字段非空

3. TTS（可选）：
    POST /api/voice/synthesize {"text":"..."}
    → 拿到 audio_url
    → GET /api/voice/audio/{file_id}
    → 播放 MP3
```

### 麦克风采集要点（HarmonyOS）

```ts
import audio from '@ohos.multimedia.audio'

const capturer = audio.createAudioCapturer({
  streamInfo: {
    samplingRate: audio.AudioSamplingRate.SAMPLE_RATE_16000,
    channels: audio.AudioChannel.CHANNEL_1,
    sampleFormat: audio.AudioSampleFormat.SAMPLE_FORMAT_S16LE,
    encodingType: audio.AudioEncodingType.ENCODING_TYPE_RAW,
  },
  capturerInfo: {
    source: audio.SourceType.SOURCE_TYPE_MIC,
    capturerFlags: 0,
  },
})

capturer.on('readData', (buffer) => {
  // 流式：ws.send(buffer)
  // 一次性：accumulate into file
})
```

关键参数：16kHz 采样率、单声道、16bit signed PCM。如果设备麦克风原生不是此格式，需在 HAP 端做重采样。

### 不依赖前端

HAP 原生应用不需要调用任何前端页面，所有能力通过上述 HTTP/WebSocket API 即可使用。不需要 HTTPS、不需要浏览器、不需要安全上下文。
