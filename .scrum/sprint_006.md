# Sprint 6

> 创建: 2026-06-01
> 完成: 2026-06-01
> 来源: BACKLOG.md（BL-009）
> 设计文档: .scrum/specs/2026-06-01-streaming-asr-design.md
> 实现计划: .scrum/plans/2026-06-01-streaming-asr.md
> 状态: 已完成

## 任务清单

### SB-001 · 配置扩展 — VoiceConfig + ModelConfig [ ]
- **来源**: BL-009
- **文件**: `pkg/config/config.go`
- **子任务**:
  - [x] VoiceConfig 新增 `StreamingModelName string` 字段（`json:"streaming_model_name,omitempty"`）
  - [x] ModelConfig 新增 `AppID` / `AppKey` / `AccessKey` / `ResourceID` 四个字段（用于火山凭证映射）
  - [x] `go build -tags goolm,stdjson ./pkg/config/...` 编译通过
  - [x] 提交

### SB-002 · 流式 ASR 接口 — StreamingTranscriber [ ]
- **来源**: BL-009
- **文件**: `pkg/audio/asr/streaming.go`（新建）
- **子任务**:
  - [x] 定义 `StreamingTranscriber` 接口（`Name()` + `StartStream()`）
  - [x] 定义 `StreamingConfig` 结构体（Codec/SampleRate/Bits/Channels/Language）
  - [x] 定义 `StreamingSession` 接口（`SendAudio()` + `Results()` + `Close()`）
  - [x] 定义 `StreamingResult` 结构体（Text/Definite/Index/Final/Error）
  - [x] `go build -tags goolm,stdjson ./pkg/audio/asr/...` 编译通过
  - [x] 提交

### SB-003 · 火山 Binary Protocol — Header 编解码 + 单测 [ ]
- **来源**: BL-009
- **文件**: `pkg/audio/asr/volcengine_streaming.go`（新建）、`volcengine_streaming_test.go`（新建）
- **子任务**:
  - [x] 定义常量：Message Type（0b0001/0b0010/0b1001/0b1011/0b1111）、Flags、Serialization、Compression
  - [x] 实现 `encodeVolcengineHeader(msgType, flags, serial, compr) [4]byte`
  - [x] 实现 `decodeVolcengineHeader(h [4]byte) (msgType, flags, serial, compr)`
  - [x] 编写 `TestVolcengineHeaderEncode`（5 种消息类型 + 组合）
  - [x] 编写 `TestVolcengineHeaderDecode`
  - [x] TDD: 先跑单测确认 FAIL，再实现确认 PASS
  - [x] 提交

### SB-004 · 火山 Binary Protocol — 帧构造与响应解析 + 单测 [ ]
- **来源**: BL-009
- **文件**: `pkg/audio/asr/volcengine_streaming.go`（追加）、`volcengine_streaming_test.go`（追加）
- **子任务**:
  - [x] 实现 `buildFullClientRequestPayload(appID, uid) map[string]any`（v3 SAUC 格式）
  - [x] 实现 `buildFullClientFrame(appID, uid) ([]byte, error)`（JSON → gzip → [header][payloadLen][payload]）
  - [x] 实现 `encodeAudioOnlyFrame(audio, seq, isLast) (header, seqBytes, payloadSize, payload)`（不压缩 PCM）
  - [x] 实现 `parseUtterances(jsonBody) []StreamingResult`
  - [x] 实现 `parseVolcengineError(jsonBody) (int, string)`
  - [x] 实现 `gunzip(data) ([]byte, error)`
  - [x] 编写 `TestBuildFullClientRequestPayload`（验证 format:raw / codec:pcm_s16le / result_type:full）
  - [x] 编写 `TestEncodeAudioOnlyFrame`（正序号 / 负序号结束帧）
  - [x] 编写 `TestParseServerResponse`（definite=false / definite=true 两种）
  - [x] 编写 `TestParseErrorResponse`（45000001）
  - [x] 编写 `TestIsServerACK`（0b1011 不被当成错误）
  - [x] TDD: 先跑单测确认 FAIL，再实现确认 PASS
  - [x] 提交

### SB-005 · 火山 WebSocket 客户端 — 连接 + 收发 + 去重 + 错误码映射 [ ]
- **来源**: BL-009
- **文件**: `pkg/audio/asr/volcengine_streaming.go`（追加）
- **子任务**:
  - [x] 实现 `volcengineStreamingTranscriber`（工厂，持有 modelCfg，`Name()` 返回 "volcengine-asr"）
  - [x] 实现 `newVolcengineSession(ctx, modelCfg)`（建连 `wss://openspeech.bytedance.com/api/v3/sauc/bigmodel`）
  - [x] 建连携带 4 个 X-Api-* 头（App-Key / Access-Key / Resource-Id / Request-Id）
  - [x] 建连后立即发送 Full Client Request 帧（appID 取自 `modelCfg.AppID`）
  - [x] 实现 `SendAudio(chunk)`（seq++ → encodeAudioOnlyFrame → WriteMessage）
  - [x] 实现 `Close()`（发送负序号结束帧，closeOnce 保护）
  - [x] 实现 `readLoop()` 协程：ReadMessage → 解析 header → 分派处理
  - [x] readLoop 分派：Server ACK(0b1011) 忽略 | Error(0b1111) → 错误码映射 | FullServerResp(0b1001) → parseUtterances → dedupAndPush
  - [x] readLoop 解析响应帧时按 flags 判断 offset（有 seq 则 +4，无则 0），再取 payload_size + payload
  - [x] 实现 `dedupAndPush(utterances)`：逐索引比对 lastUtterances，三类推送 + 两类跳过，更新缓存
  - [x] 实现 `volcengineErrorMessage(code, msg)`（8 种错误码 → 中文提示）
  - [x] 实现 `newUUID()` / `isClosedError()`
  - [x] 注意：单文件中所有 import 在文件顶部合并，每步追加功能时合并新增的包
  - [x] `go build -tags goolm,stdjson ./pkg/audio/asr/...` 编译通过
  - [x] 提交

### SB-006 · DetectStreamingTranscriber + 去重单测 + detect 单测 [ ]
- **来源**: BL-009
- **文件**: `pkg/audio/asr/asr.go`（修改）、`volcengine_streaming_test.go`（追加）
- **子任务**:
  - [x] 实现 `DetectStreamingTranscriber(cfg)`：取 `voice.streaming_model_name` → `GetModelConfig` → 匹配 `volcengine-asr` 协议 → 返回 `volcengineStreamingTranscriber`；为空或无匹配返回 `(nil, nil)`
  - [x] 编写 `TestUtteranceDedup`（5 种场景全覆盖：新增 / text 变化 / definite false→true / 完全相同跳过 / true→false 兜底）
  - [x] 编写 `TestDetectStreamingTranscriber`（3 种 case：空 model_name → nil / 不存在 model → (nil,nil) / 匹配 volcengine-asr → 返回 transcriber）
  - [x] TDD: 先跑单测确认 FAIL，再实现确认 PASS
  - [x] 全量跑 `-run "TestVolcengine|TestBuildFull|TestEncodeAudioOnly|TestParse|TestIsServerACK|TestUtteranceDedup|TestDetectStreaming"` 确认 9 项 PASS
  - [x] 提交

### SB-007 · WebSocket Handler + capabilities 扩展 + CheckOrigin [ ]
- **来源**: BL-009
- **文件**: `web/backend/api/voice.go`（修改）、`web/backend/api/router.go`（修改）
- **子任务**:
  - [x] 扩展 `handleVoiceCapabilities`：新增 `streaming` 字段（调用 `DetectStreamingTranscriber`）
  - [x] 实现 `cidrMatch(remoteAddr, cidrs) bool`（net.SplitHostPort + net.ParseCIDR）
  - [x] 实现 `handleVoiceStream`：gorilla/websocket upgrader 升级
  - [x] CheckOrigin: 非 public 模式允许 same-origin；public 模式走 CIDR 匹配（加 TODO 反代 X-Forwarded-For 注释）
  - [x] 后端 → 火山：binary PCM chunk 原样转发给 session.SendAudio
  - [x] 火山 → 前端：readLoop → 按 `result.Index` 维护 `current[Index]` + `locked` 累积 → `writeWSJSON(conn, partial/final/error)`
  - [x] fullText 累积正确：definite=true → locked+=text, current[i]=""；definite=false → current[i]=text；拼接 locked+current[0..maxIdx] 作为 final
  - [x] 实现 `writeWSJSON(conn, v any)` 辅助函数
  - [x] `registerVoiceRoutes` 注册 `GET /api/voice/stream`
  - [x] `go build -tags goolm,stdjson ./web/backend/...` 编译通过
  - [x] 提交

### SB-008 · 前端 store + api + hook 扩展 [ ]
- **来源**: BL-009
- **文件**: `web/frontend/src/store/voice.ts`、`api/voice.ts`、`hooks/use-voice.ts`
- **子任务**:
  - [x] store 新增 `streamingAvailableAtom = atom(false)`
  - [x] `fetchVoiceCapabilities` 返回类型扩展 `streaming: boolean`
  - [x] `useVoice` hook 解构新增 `streamingAvailable` + `setStreamingAvailable` + `fetchVoiceCapabilities().streaming` 赋值
  - [x] `cd web/frontend && npx tsc --noEmit` 零错误
  - [x] 提交

### SB-009 · AudioWorklet 处理器 [ ]
- **来源**: BL-009
- **文件**: `web/frontend/public/voice-processor.js`（新建）
- **子任务**:
  - [x] 实现 `VoiceProcessor extends AudioWorkletProcessor`
  - [x] `process(inputs)`: 取 mono channel 0 → 线性插值重采样至 16kHz → 累积 Float32Array buffer
  - [x] buffer ≥ 1600 samples 时转换为 Int16Array PCM（`Math.round` 避免截断失真），`postMessage` 给主线程，清空 buffer
  - [x] `_linearResample(samples, fromRate, toRate)`：线性插值（满足 80-3400Hz 语音频段）
  - [x] `registerProcessor("voice-processor", VoiceProcessor)`
  - [x] 提交

### SB-010 · VoiceRecorder 重写 [ ]
- **来源**: BL-009
- **文件**: `web/frontend/src/components/chat/voice-recorder.tsx`（重写）
- **子任务**:
  - [x] 组件挂载 = 自动开始录音（useEffect 调用 `startRecording`），卸载 = cleanup 停录音+关 WS
  - [x] `startRecording`: `new AudioContext({sampleRate: 16000})` → 加载 `voice-processor.js` → getUserMedia → AudioWorkletNode → 连接 WebSocket(`/api/voice/stream`)
  - [x] ws.onopen: pipe worklet PCM chunk → ws.send
  - [x] ws.onmessage: 解析 partial 消息 → lockedTextRef + currentTextRef 合并（definite 锁定+当前 indefinite 替换） → `onInterimText(merged, definite)`；final → `onTranscribed(fullText)`
  - [x] 60s auto-stop（`autoStopRef` 记录 setTimeout，cleanup 时 `clearTimeout`）
  - [x] 计时器每秒更新 elapsed，59s 自动 cleanup
  - [x] `onError` 回调
  - [x] 按钮 `disabled` 显示计时秒数，录音中脉冲动画（复用现有 CSS）
  - [x] `cd web/frontend && npx tsc --noEmit` 零错误
  - [x] 提交

### SB-011 · 前端单测 [ ]
- **来源**: BL-009
- **文件**: `web/frontend/src/components/chat/__tests__/voice.test.ts`（新建）
- **子任务**:
  - [x] `TestUtteranceMerge`: 模拟 definite=false→false→true→false→true 序列，验证 locked+indefinite 累积正确
  - [x] `TestAudioContextFallback`: 模拟 48kHz→16kHz 线性插值重采样，检验 sample 数量正确；同采样率直通
  - [x] `TestWorkletBuffer`: 验证 ≥1600 触发 flush（12帧1536不触发 / 13帧1664触发 / 边界1600触发），stop 时 flush 余量
  - [x] `npx vitest run src/components/chat/__tests__/voice.test.ts` 3 项 PASS
  - [x] 提交

### SB-012 · ChatComposer 适配 [ ]
- **来源**: BL-009
- **文件**: `web/frontend/src/components/chat/chat-composer.tsx`
- **子任务**:
  - [x] `useVoice()` 解构新增 `streamingAvailable`
  - [x] 麦克风按钮：`streamingAvailable` 时显示流式按钮（录音中高亮），否则不显示（不保留旧 asrAvailable 回退）
  - [x] `showRecorder=true` 时渲染 `<VoiceRecorder>`（挂载即录音）
  - [x] 录音中：`<TextareaAutosize readOnly />`，停止后可编辑
  - [x] `handleSend`: 录音中时 `setShowRecorder(false)` + 立即 `onSend()`（input 值已由 onInterimText 实时更新，不等 final RTT）
  - [x] `handleKeyDown`: Enter 统一走 `handleSend`
  - [x] `VoiceRecorder.onInterimText(text, definite)` → `onInputChange(text)`（合并后全文）
  - [x] `VoiceRecorder.onTranscribed(finalText)` → 关闭录音器
  - [x] `cd web/frontend && npx tsc --noEmit` 零错误
  - [x] 提交

### SB-013 · AudioPlayer 改位 — 紧凑常驻 + 加载态 [ ]
- **来源**: BL-009
- **文件**: `web/frontend/src/components/chat/audio-player.tsx`
- **子任务**:
  - [x] `audioUrl` 可选（undefined = loading），加载时渲染 12×12 CSS spinner（`animate-spin`）
  - [x] 播放按钮紧凑内联（h-6 w-6），常驻显示（非 hover-only）
  - [x] 保留 `<audio autoPlay>` 自动播放
  - [x] `cd web/frontend && npx tsc --noEmit` 零错误
  - [x] 提交

### SB-014 · AssistantMessage — AudioPlayer 移入右上角 [ ]
- **来源**: BL-009
- **文件**: `web/frontend/src/components/chat/assistant-message.tsx`
- **子任务**:
  - [x] 右上角 `top-2 right-2` 区域：AudioPlayer（常驻/互斥渲染 loading spinner 或播放按钮）在左，复制按钮（hover-only）在右
  - [x] 互斥渲染：`{ttsLoading ? <AudioPlayer /> : audioUrl ? <AudioPlayer audioUrl={audioUrl} /> : null}`
  - [x] 保留 `ttsLoading` / `audioUrl` / `ttsTriggeredRef` 等现有 TTS 状态不变（仅改渲染位置）
  - [x] 删除原底部 `border-t` TTS 加载行和 AudioPlayer wrapper
  - [x] `cd web/frontend && npx tsc --noEmit` 零错误
  - [x] 提交


### SB-016 · 火山引擎语音模型写入默认模板 [x]
- **来源**: BL-009
- **文件**: `pkg/config/defaults.go`
- **子任务**:
  - [x] `ModelList` 新增 `volcengine-asr`（bigmodel / wss://openspeech.bytedance.com）
  - [x] `ModelList` 新增 `volcengine-tts`（seed-tts-2.0 / 爽快思思2.0）
  - [x] Voice 默认值预填 `streaming_model_name` / `tts_model_name` / `tts_voice` / `tts_format`
  - [x] `go build -tags goolm,stdjson ./pkg/config/...` 编译通过

### SB-015 · 全量构建 + 验证 + 验收 [ ]
- **来源**: BL-009
- **阻塞**: SB-001~SB-014 全部完成
- **子任务**:
  - [x] `make build && make build-launcher` 全量构建成功
  - [x] `make lint` 零告警
  - [x] `go vet -tags goolm,stdjson ./pkg/audio/... ./web/backend/...` 零输出
  - [x] `go test -v -tags goolm,stdjson -run "TestVolcengine|TestBuildFull|TestEncodeAudioOnly|TestParse|TestIsServerACK|TestUtteranceDedup|TestDetectStreaming" ./pkg/audio/asr/` 9 项 PASS
  - [x] `cd web/frontend && npx vitest run src/components/chat/__tests__/voice.test.ts` 3 项 PASS
  - [x] `cd web/frontend && npx tsc --noEmit` 零错误
  - [x] 启动服务 + `curl /api/voice/capabilities` 确认 `{"asr":true,"tts":true,"streaming":true}`
  - [x] 回归：`POST /api/voice/transcribe`（SiliconFlow）仍正常返回
  - [x] **UI 验收**：点🎤脉冲+计时 / 说话实时出字 / 发送+关闭流程 / 60s 超时 / 输入框只读
  - [x] **AudioPlayer 验收**：右上角常驻 / 与复制按钮并排 / autoplay / 加载 spinner
  - [x] **边界场景**：Safari 降级 / 蓝牙重采样 / 鉴权失败 toast / 余额不足 toast
  - [x] 最终提交
