# 流式语音识别（Streaming ASR）设计

> 创建: 2026-06-01
> 修订: 2026-06-01（第 6 轮审阅）
> 来源: BL-009 · 流式语音识别 + 播放交互优化
> 状态: 已确认

## 目标

Web 前端语音输入支持实时流式识别（边说边出字），替换当前「录完→上传→一次性返回」的模式。同时优化 TTS 播放按钮位置。

## 方案

新增 WebSocket 流式 ASR 通道，对接火山引擎（Volcengine）v3 SAUC BigModel 语音识别。保留现有 HTTP `/api/voice/transcribe`（SiliconFlow）文件上传接口不变，两者并存。

## 架构

```
浏览器                                    Launcher(18800)                      火山引擎
  │                                              │                                  │
  │── ws://localhost:18800/api/voice/stream ────▶│ gorilla/websocket upgrade        │
  │   binary: PCM 16kHz 16bit mono               │ CheckOrigin: serverPublic+CIDRs  │
  │                                              │                                  │
  │                                              │── wss://openspeech.bytedance.com─▶│
  │                                              │   /api/v3/sauc/bigmodel           │
  │                                              │   Headers (v3 仅走 X-Api-*):      │
  │                                              │     X-Api-App-Key: {app_key}      │
  │                                              │     X-Api-Access-Key: {access_key}│
  │                                              │     X-Api-Resource-Id: {...}      │
  │                                              │     X-Api-Request-Id: {uuid}      │
  │                                              │   火山 binary protocol             │
  │◀── {"type":"partial","text":"今天", ──────── │◀── utterances[0](definite=false) ───│
  │     "definite":false}                        │     utterances[1](definite=false)    │
  │◀── {"type":"partial","text":"今天天气", ──── │◀── utterances[0](definite=false) ───│
  │     "definite":false}                        │     utterances[1](definite=false)    │
  │◀── {"type":"partial","text":"今天天气", ──── │◀── utterances[0](definite=true) ────│
  │     "definite":true}                         │     utterances[1](definite=false)    │
  │◀── {"type":"partial","text":"怎么样", ────── │◀── utterances[0](definite=true) ────│
  │     "definite":true}                         │     utterances[1](definite=true)     │
  │◀── {"type":"final","text":"今天天气怎么样"} ──│◀── is_last_packet=true ────────────│
  │──▶ user clicks send or mic (stop record) ──▶│                                  │
```

**后端→前端消息 schema**（每个 utterance 独立一条消息，text = 该 utterance 文本，非累积全文）：

```json
// 中间结果（definite=false，前端替换当前 indefinite 段）
{"type": "partial", "text": "今天天气", "definite": false}
// 已确认段落（definite=true，前端锁定此段不覆盖）
{"type": "partial", "text": "今天天气", "definite": true}
// 流结束（最终累积全文，方便前端直接使用）
{"type": "final",  "text": "今天天气怎么样"}
// 错误
{"type": "error",  "code": "45000001", "message": "ASR 参数配置错误，请检查 app_id"}
```

## 后端改动

### 1. `pkg/audio/asr/streaming.go` — 新增流式接口

```go
type StreamingTranscriber interface {
    Name() string
    StartStream(ctx context.Context, cfg StreamingConfig) (StreamingSession, error)
}

type StreamingConfig struct {
    Codec      string // "pcm_s16le"
    SampleRate int    // 16000
    Bits       int    // 16
    Channels   int    // 1
    Language   string // "zh-CN"
}

type StreamingSession interface {
    SendAudio(chunk []byte) error
    Results() <-chan StreamingResult
    Close() error
}

type StreamingResult struct {
    Text     string // 当前文本
    Definite bool   // true = 已确认段落，前端锁定；false = 中间结果，可覆盖
    Final    bool   // true = 流结束（最后一个 result 同时 Definite=true, Final=true）
    Error    string
}
```

### 2. `pkg/audio/asr/volcengine_streaming.go` — 新增火山引擎实现

#### 2.1 连接参数

| 参数 | 值 | 说明 |
|------|-----|------|
| URL | `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel` | v3 SAUC 大模型端点 |
| X-Api-App-Key | `{app_key}` | 火山控制台 → 语音技术 → 应用列表 → App Key |
| X-Api-Access-Key | `{access_key}` | 火山控制台 → 语音技术 → 应用列表 → Access Key |
| X-Api-Resource-Id | `volc.bigasr.sauc.duration` | 时长计费资源 ID（默认值） |
| X-Api-Request-Id | `{uuid}` | 每次连接唯一 UUID |

> **鉴权说明**：v3 SAUC 端点仅走 `X-Api-App-Key` + `X-Api-Access-Key` 头部鉴权，不需要 `Authorization: Bearer;{token}`（那是 v1/v2 端点的方式）。两者二选一，v3 用 X-Api-* 头。

#### 2.2 Binary 协议 Header 布局（4 字节，大端序）

```
Byte 0  ┌──────────────────┬──────────────────┐
        │ Protocol Version │   Header Size    │
        │    0b0001 (4b)   │   0b0001 (4b)    │
        ├──────────────────┼──────────────────┤
Byte 1  │  Message Type    │  Type-Specific   │
        │     (4b)         │    Flags (4b)    │
        ├──────────────────┼──────────────────┤
Byte 2  │ Serialization    │  Compression     │
        │   Method (4b)    │    Type (4b)     │
        ├──────────────────┼──────────────────┤
Byte 3  │    Reserved      │    Reserved      │
        │    0x00 (4b)     │    0x00 (4b)     │
        └──────────────────┴──────────────────┘
```

**字段枚举：**

| 字段 | 值 | 含义 |
|------|-----|------|
| Protocol Version | `0b0001` | 版本 1 |
| Header Size | `0b0001` | 4 字节（值×4），`0b1111` 表示 ≥60B 需 header extension |
| Message Type | `0b0001` | Full Client Request（初始请求，含配置 JSON） |
| | `0b0010` | Audio-only Request（纯音频数据，客户端→服务端） |
| | `0b1001` | Full Server Response（识别结果，服务端→客户端） |
| | `0b1011` | Server ACK（无 payload 应答帧，服务端→客户端，**忽略即可**） |
| | `0b1111` | Error Message（服务端错误） |
| Type-Specific Flags | `0b0000` | Full Client Request 固定值 |
| | `0b0001` | 音频包带正序列号（普通帧） |
| | `0b0011` | 音频包带负序列号（最后一帧，结束标记） |
| Serialization | `0b0000` | Raw bytes（音频数据） |
| | `0b0001` | JSON |
| Compression | `0b0000` | 不压缩 |
| | `0b0001` | gzip |
| Reserved | `0x00` | 全零填充 |

#### 2.3 Sequence 管理与结束帧

音频包（Message Type `0b0010`）通过 Type-Specific Flags 管理序列号：

- 普通帧：Flags = `0b0001`，payload 前 4 字节为 **正** int32 序号
- 结束帧：Flags = `0b0011`，payload 前 4 字节为 **负** int32 序号（最后一包）
- 火山服务端收到负序号后触发 final utterance（`definite=true`）

> **压缩策略**：PCM 是随机分布的二进制数据，gzip 几乎无压缩比却显著消耗 CPU。音频帧 Compression 固定 `0b0000`（不压缩）。仅 Full Client Request 的 JSON payload 使用 gzip（Compression=`0b0001`）。

**发送流程：**

```
1. Full Client Request (0b0001, flags=0b0000, Compression=gzip, JSON payload)
   → 包含 appid, uid, audio{format:"raw", codec:"pcm_s16le", rate:16000, ...}
2. Audio-only Request (0b0010, flags=0b0001, Compression=0b0000, seq=1) → PCM chunk 不压缩
3. Audio-only Request (0b0010, flags=0b0001, Compression=0b0000, seq=2) → PCM chunk 不压缩
4. ... 中间可能收到 Server ACK (0b1011)，忽略即可
N. Audio-only Request (0b0010, flags=0b0011, Compression=0b0000, seq=-N) → 最后一帧
   ← Server: utterance(definite=true)
```

#### 2.4 Full Client Request payload（v3 SAUC BigModel）

```json
{
  "app": {
    "appid": "{app_id}"
  },
  "user": {
    "uid": "{unique_user_id}"
  },
  "audio": {
    "format": "raw",
    "codec": "pcm_s16le",
    "rate": 16000,
    "bits": 16,
    "channel": 1,
    "language": "zh-CN"
  },
  "request": {
    "model_name": "bigmodel",
    "result_type": "full",
    "enable_itn": true,
    "enable_punc": true
  }
}
```

> 注意：
> - v3 鉴权走 HTTP 头部 `X-Api-Access-Key`，payload 中**不需要** `app.token` 和 `app.cluster` 字段。`audio.format` 为 `"raw"`（非 `"pcm"`），配合 `codec: "pcm_s16le"`。
> - `request.model_name` 字段值**以火山控制台分配为准**（可能为 `bigmodel`、`seed_asr_zh_v2` 等），请对照控制台开通的模型名称填写。
> - `result_type: "full"` 确保火山每次返回**累积所有 utterances 数组**，后端直接遍历 `result.utterances[]` 映射到前端消息。累积责任在火山侧，后端无需自行维护已确认句列表。

#### 2.5 后端→前端映射规则（方案 A：后端去重）

`result_type: "full"` 下火山**每帧都返回全部累积**的 `result.utterances[]` 数组。后端维护 `lastUtterances []UtteranceSnapshot{Text, Definite}`，每帧逐索引比对，**三类推送条件**：

| 条件 | 触发场景 | 说明 |
|------|---------|------|
| 新增 | 数组长度 > 缓存长度，出现新索引 | 新 utterance 产生 |
| 同索引 text 变化 | `utterances[i].text != cache[i].text` | indefinite 段实时增量（**边说边出字的关键**） |
| definite 由 false→true | `utterances[i].definite=true && cache[i].definite=false` | 段落确认，前端锁定 |

**跳过条件**（不推送）：

| 条件 | 说明 |
|------|------|
| `(text, definite)` 完全相同 | 未变化，跳过 |
| definite 由 true→false | 不应发生，兜底不推送（definite 段不会回退） |

**完整示例**（从第 1 帧开始）：

```
第 1 帧火山返回: [{今天, false}]
  缓存: []
  比对: [0] 新增 → 推送
  后端推送:
    {"type": "partial", "text": "今天", "definite": false}
  更新缓存: [{今天, false}]

第 2 帧火山返回: [{今天天气, false}]
  缓存: [{今天, false}]
  比对: [0] text "今天"→"今天天气" → 推送（同索引 text 变化）
  后端推送:
    {"type": "partial", "text": "今天天气", "definite": false}
  更新缓存: [{今天天气, false}]

第 3 帧火山返回: [{今天天气, true}, {怎么样, false}]
  缓存: [{今天天气, false}]
  比对: [0] definite false→true → 推送; [1] 新增 → 推送
  后端推送:
    {"type": "partial", "text": "今天天气", "definite": true}
    {"type": "partial", "text": "怎么样",   "definite": false}
  更新缓存: [{今天天气, true}, {怎么样, false}]

第 4 帧火山返回: [{今天天气, true}, {怎么样, true}]
  缓存: [{今天天气, true}, {怎么样, false}]
  比对: [0] 未变 → 跳过; [1] definite false→true → 推送
  后端推送:
    {"type": "partial", "text": "怎么样", "definite": true}
  更新缓存: [{今天天气, true}, {怎么样, true}]
```

- `text` = **该 utterance 的文本**（非累积全文）
- `definite` = 该 utterance 的 definite 值
- 前端收到后按已有合并策略自行累积：definite 段锁定，当前 indefinite 段增量替换
- 流结束时后端额外发一条 `{"type": "final", "text": "<全部 utterance 拼接>"}` 作为确认

### 3. `pkg/audio/asr/asr.go` — 新增检测函数

```go
func DetectStreamingTranscriber(cfg *config.Config) (StreamingTranscriber, error)
```

按 `voice.streaming_model_name` 查 model_list，匹配 `volcengine-asr` 协议。`streaming_model_name` 为空或找不到对应 model 时返回 `(nil, nil)`（非错误），capabilities handler 据此返回 `streaming: false`。匹配成功则返回对应实现。

### 4. `pkg/config/config.go` — 配置扩展

VoiceConfig 新增字段：

```go
type VoiceConfig struct {
    // ...existing fields...
    StreamingModelName string `json:"streaming_model_name,omitempty"`
}
```

ModelConfig 新增字段（仅 volcengine-asr 条目需要）：

```go
type ModelConfig struct {
    // ...existing fields...
    AppKey     string `json:"app_key,omitempty"`     // → X-Api-App-Key。TODO: 后续迁移为 SecureString 类型（支持 plaintext/file:///enc://），与 ModelConfig.APIKey 风格统一
    AccessKey  string `json:"access_key,omitempty"`   // → X-Api-Access-Key。TODO: 同上
    ResourceID string `json:"resource_id,omitempty"`  // → X-Api-Resource-Id，默认 "volc.bigasr.sauc.duration"
}
```

> 凭证字段精简说明：火山 v3 SAUC 实际只需 `app_id`（已有）、`app_key`、`access_key` 三个值。`app_id` 即火山应用 ID（填入 payload `app.appid`），`app_key` 和 `access_key` 在火山控制台「语音技术→应用列表→应用详情」中可找到。

### 5. `web/backend/api/voice.go` — 新增 WebSocket handler

`handleVoiceStream`：

- gorilla/websocket upgrader 升级连接
- **CheckOrigin**：
  ```go
  upgrader.CheckOrigin = func(r *http.Request) bool {
      if !h.serverPublic { return true } // 非 public 模式允许 same-origin
      return matchCIDR(r.RemoteAddr, h.serverCIDRs)
  }
  ```
- 鉴权：已有 dashboard session 中间件覆盖（`GET /api/voice/stream` 非 public path）
- 前端→后端：binary message（PCM audio chunk）
- 后端→前端：JSON text message（含 `type` / `text` / `definite` 字段）
- 生命周期：前端关闭连接 → buffer 剩余音频 → 发送结束帧 → 等待 final result → 关闭火山 WS
- **错误码透传**：按火山返回码分类（见错误处理节）

### 6. `web/backend/api/router.go` — 注册路由

```go
mux.HandleFunc("GET /api/voice/stream", h.handleVoiceStream)
```

## 前端改动

### 1. `voice-recorder.tsx` — 重写

| 改动 | 说明 |
|------|------|
| 音频采集 | `MediaRecorder` → `AudioContext({sampleRate: 16000})` + `AudioWorkletNode` |
| **重采样兼容** | 创建后校验 `audioContext.sampleRate`；等于 16000 则直通，不等于（Safari 忽略参数 / 蓝牙耳机强制原生 rate）则 worklet 内**线性插值**重采样至 16kHz（线性插值已满足语音识别准确率，语音能量集中在 80–3400Hz，混叠影响可忽略。如后续发现高频杂音可补 anti-alias FIR） |
| 采样格式 | AudioWorklet 内 `Float32Array` → `Int16Array`，输出 16bit PCM s16le |
| **Worklet buffer 策略** | 每帧 128 samples (8ms)，内部累积 **≥ 1600 samples (≈100ms)** 时 `postMessage` 给主线程发 WS（实际 13 帧 1664 samples ≈ 104ms）；录音停止时 flush 不足阈值的剩余 buffer |
| WebSocket | 打开 `/api/voice/stream`，流式发送 PCM chunk（每 ≈100ms 约 3200 bytes，不压缩） |
| 流式回调 | 服务端推送 `partial` → `onInterimText(text, definite)` 回调 |
| definite 管理 | definite=true 的段锁定不覆盖，仅替换当前 indefinite 段（见下方合并策略） |
| 生命周期 | 组件挂载 = 录音中；组件卸载 = 停止录音 + 关闭 WS；**不暴露 imperative stop()** |
| Safari 降级 | 不支持 AudioWorklet → 回退 ScriptProcessorNode + console.warn（此时重采样/缓冲逻辑同 worklet 内逻辑） |

Props：

```ts
interface VoiceRecorderProps {
  onInterimText: (text: string, definite: boolean) => void  // 流式中间结果
  onTranscribed: (text: string) => void                      // 最终结果
  onError?: (error: string) => void
}
```

**utterance 合并策略**（前端侧）：

```
收到 {text: "今天", definite: false}        → 输入框：今天
收到 {text: "今天天气", definite: false}    → 输入框：今天天气（替换当前 indefinite 段）
收到 {text: "今天天气", definite: true}     → 锁定 "今天天气"，后续文本追加其后
收到 {text: "怎么样", definite: false}      → 输入框：今天天气怎么样（锁定段 + 新 indefinite）
收到 {text: "怎么样", definite: true}       → 锁定全部
```

**生命周期规则**（方案二：卸载即停止）：

- 组件渲染 = 开始录音 + 打开 WS
- 组件卸载 = 停止录音 + 关闭 WS（useEffect cleanup）+ flush buffer
- 父组件通过 `setShowRecorder(false)` 卸载 → 即停止
- 点发送：`setShowRecorder(false)` → 组件卸载 cleanup → `onSend()`
- 60s 超时：内部 `setTimeout` → `onTranscribed(text)` → 父组件 `setShowRecorder(false)`

### 2. `chat-composer.tsx` — 适配

| 改动 | 说明 |
|------|------|
| VoiceRecorder 集成 | `onInterimText(text, definite)` → `onInputChange(text)` |
| 录音中输入框**只读** | `showRecorder=true` 时 `<TextareaAutosize readOnly />`，停录后可编辑 |
| 点发送/Enter | `setShowRecorder(false)` 卸载组件 → 组件 cleanup → `onSend()` |
| 点🎤 toggle off | `setShowRecorder(false)` → 文字留在输入框，可编辑后手动发 |
| 按钮状态 | 录音中显示脉冲动画 + 计时（复用现有样式） |

## 音频播放优化

### 3. `audio-player.tsx` — 改位

| 改动 | 说明 |
|------|------|
| 位置 | 从气泡底部 `border-t` 独立行 → 移到右上角，与复制按钮并排 |
| 显示策略 | **常驻显示**（非 hover-only），与 hover 显示的复制按钮明确区分 |
| 加载态 | 小尺寸 CSS spinner（12×12），占播放按钮同一位置，替换「正在生成语音...」文字行 |
| 布局 | 播放按钮在左，复制按钮在右，同处 `top-2 right-2` 区域 |
| **autoPlay** | **保留** `<audio autoPlay>`，合成完成自动播放；用户可随时点击暂停 |

### 4. `assistant-message.tsx` — 适配

- AudioPlayer + 加载 spinner 移入右上角 `top-2 right-2` 区域
- 复制按钮保持在 AudioPlayer 右边（hover-only）
- 删除原底部 `border-t` TTS 区域（`ttsLoading` 文字 + AudioPlayer wrapper）

## 配置

`config.json` 示例：

```json
{
  "model_list": [
    {
      "model_name": "volcengine-asr",
      "provider": "volcengine-asr",
      "api_base": "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel",
      "app_id": "your_app_id",
      "app_key": "your_app_key",
      "access_key": "your_access_key",
      "resource_id": "volc.bigasr.sauc.duration"
    }
  ],
  "voice": {
    "streaming_model_name": "volcengine-asr",
    "model_name": "TeleSpeechASR",
    "tts_model_name": "MOSS-TTSD"
  }
}
```

> **凭证字段映射到火山控制台：**
> - `app_id`：控制台 → 语音技术 → 应用列表 → **APP ID**
> - `app_key`：控制台 → 语音技术 → 应用列表 → 应用详情 → **App Key**（→ HTTP 头 `X-Api-App-Key`）
> - `access_key`：控制台 → 语音技术 → 应用列表 → 应用详情 → **Access Key**（→ HTTP 头 `X-Api-Access-Key`）
> - `resource_id`：固定 `"volc.bigasr.sauc.duration"`（时长计费资源标识）

## 交互流程

| 用户操作 | 行为 |
|----------|------|
| 点🎤 | 组件挂载 → 开始录音 + 开 WS，按钮变「录音中」脉冲动画 + 计时，输入框只读 |
| 说话 | 输入框实时出字；definite 段锁定，当前 indefinite 段增量替换 |
| 点发送 / Enter | `setShowRecorder(false)` → 组件卸载 cleanup 关 WS → `onSend()` |
| 再点🎤 | `setShowRecorder(false)` → 组件卸载 cleanup 关 WS → 文字留在输入框，可编辑后手动发 |
| 60s 超时 | 内部自动关 WS → `onTranscribed(text)` → 父组件 `setShowRecorder(false)` |
| 录音中输入框 | 只读（`readOnly`），防止用户编辑与流式文字冲突 |

## 错误处理

### 火山返回码分类透传

| 火山错误码 | 分类 | 前端 message |
|-----------|------|-------------|
| `45000001` | 配置错误（非重试） | "ASR 参数配置错误，请检查 app_id" |
| `40200002` | 鉴权失败（非重试） | "ASR 鉴权失败，请检查 App Key / Access Key" |
| `40200010` | 配额超限 | "ASR 时长配额已用尽，请充值或申请更多配额" |
| `40200011` | QPS 超限 | "ASR 请求过于频繁，请稍后重试" |
| `45000081` | 超时 | "ASR 请求超时，请重试" |
| `40200004` | 服务未开通 | "ASR 服务未开通，请在火山控制台开通语音识别服务" |
| `55000000` | 服务端内部错误 | "ASR 服务暂时不可用，请稍后重试" |
| `55000031` | 服务繁忙 | "ASR 服务繁忙，请稍后重试" |
| 其他 | 通用错误 | 透传原始 message |

### 前端错误场景

| 场景 | 处理 |
|------|------|
| 火山 WS 建连失败 | 后端返回 `{"type":"error","code":"...","message":"..."}`，前端 toast 提示 |
| 识别过程出错 | 后端推送 error JSON，前端 toast 提示，保留已识别文字 |
| 前端 WS 断连 | 停止录音，保留已识别文字 |
| 未配置 Provider | `GET /api/voice/capabilities` 扩展返回 `streaming: false`，前端隐藏🎤 |
| 未授权麦克风 | catch 错误，静默处理（与现有行为一致） |
| AudioWorklet 不支持 | 回退 ScriptProcessorNode + console.warn |
| Safari/蓝牙 非 16kHz | AudioContext 挂载后校验 sampleRate，不匹配则 worklet 内线性插值重采样 |

## 测试规划

### 单元测试（Go）

| 测试项 | 覆盖范围 |
|--------|---------|
| `TestVolcengineHeaderEncode` | 验证 Header 4 字节编码正确性：各 message type / flags / serialization / compression 组合的字节输出与预期一致 |
| `TestVolcengineHeaderDecode` | 验证 Header 解码正确性：给定字节解析出正确的 message type / flags 等字段 |
| `TestFullClientRequestPayload` | 验证 Full Client Request 的 JSON → gzip → 二进制帧 完整链路，产出字节与火山 demo 输出比对 |
| `TestAudioOnlyRequestEncode` | 验证 Audio-only Request 帧编码：正序号 / 负序号（结束帧）两种 |
| `TestServerResponseParse` | 验证 Full Server Response 解析：utterance definite=false / definite=true 两种，输出 StreamingResult{Text, Definite, Final} 正确 |
| `TestErrorResponseParse` | 验证 Error Message (0b1111) 解析，输出错误码和 message |
| `TestServerACKIgnore` | 验证 Server ACK (0b1011) 不被当成错误处理 |
| `TestUtteranceDedup` | 跨帧去重：三类推送（新增 / 同索引 text 变化 / definite false→true）+ 两类跳过（完全相同 / true→false 兜底） |
| `TestDetectStreamingTranscriber` | 验证 `voice.streaming_model_name` → volcengine-asr 匹配逻辑 |

### 单元测试（前端）

| 测试项 | 覆盖范围 |
|--------|---------|
| `TestUtteranceMerge` | 模拟序列：definite=false → false → true → false → true，验证输入框文本累积正确 |
| `TestAudioContextFallback` | 模拟 sampleRate != 16000 场景，验证 worklet 内线性插值重采样输出正确 sample 数量 |
| `TestWorkletBuffer` | 验证 buffer 累积 ≥ 1600 samples 时触发 flush（实际 13 帧 1664），不足阈值时 stop 触发 flush 余量 |

### 手工验证（联调）

| 验证项 | 方式 |
|--------|------|
| 降级路径 — ScriptProcessorNode | Safari 打开，console 确认 warn，录音/识别正常 |
| 降级路径 — 重采样 fallback | 蓝牙耳机连接，确认 sampleRate != 16000 触发 worklet 重采样，识别准确率可接受 |
| 并行验证 — SiliconFlow HTTP ASR | 配置回原有 model，`POST /api/voice/transcribe` 仍正常返回 |
| 鉴权失败 | 错误 app_key → 确认 toast 显示"ASR 鉴权失败，请检查 App Key / Access Key" |
| 余额不足 | 配额耗尽账户 → 确认 toast 显示"ASR 时长配额已用尽" |

## 验收标准

- [ ] 点🎤立即开始录音，按钮显示脉冲动画 + 计时，输入框只读
- [ ] 说话时输入框实时出字（definite 段锁定，当前 utterance 增量更新）
- [ ] 点发送/Enter → 停止录音 + 关 WS + 发出消息
- [ ] 点🎤关闭 → 停止录音 + 关 WS + 文字留在输入框可编辑
- [ ] 60s 自动停止
- [ ] Safari 蓝牙降级路径：ScriptProcessor + 线性插值重采样均正常
- [ ] 并行验证：HTTP `/api/voice/transcribe`（SiliconFlow）仍正常工作
- [ ] AudioPlayer 常驻显示在消息气泡右上角，与复制按钮并排（播放在左，复制在右），保留 autoplay
- [ ] 加载态小 spinner 占播放按钮位置
- [ ] 火山鉴权失败/余额不足/超时分别给出对应中文提示
- [ ] 12 项单测 + 5 项手工验证全部通过
