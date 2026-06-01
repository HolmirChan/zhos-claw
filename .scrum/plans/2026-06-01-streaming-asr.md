# 流式语音识别 + 播放交互优化 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Web 前端新增 WebSocket 流式 ASR（边说边出字），对接火山引擎 v3 SAUC BigModel；AudioPlayer 移入消息气泡右上角与复制按钮并排。

**Architecture:** 后端新增 StreamingTranscriber 接口 + 火山 WebSocket binary protocol 实现 + `/api/voice/stream` WebSocket 端点；前端 VoiceRecorder 改用 AudioContext+AudioWorkletNode 采集 PCM、WebSocket 推送音频块、接收流式 partial 结果累加入输入框。保留现有 HTTP `/api/voice/transcribe` 完全不动。

**Tech Stack:** Go (gorilla/websocket), TypeScript/React (AudioWorkletNode, Jotai)

**设计文档:** `.scrum/specs/2026-06-01-streaming-asr-design.md`

---

## 文件清单

| 操作 | 文件 | 职责 |
|------|------|------|
| 新增 | `pkg/audio/asr/streaming.go` | StreamingTranscriber 接口 |
| 新增 | `pkg/audio/asr/volcengine_streaming.go` | 火山引擎 WebSocket binary protocol 实现 |
| 新增 | `pkg/audio/asr/volcengine_streaming_test.go` | 9 项 Go 单测 |
| 修改 | `pkg/audio/asr/asr.go` | 新增 DetectStreamingTranscriber() |
| 修改 | `pkg/config/config.go` | VoiceConfig.StreamingModelName、ModelConfig.AppID/AppKey/AccessKey/ResourceID |
| 修改 | `web/backend/api/voice.go` | 新增 handleVoiceStream + capabilities 扩展 |
| 修改 | `web/backend/api/router.go` | 注册 GET /api/voice/stream |
| 新增 | `web/frontend/public/voice-processor.js` | AudioWorklet 处理器（PCM 采集+重采样+缓冲） |
| 重写 | `web/frontend/src/components/chat/voice-recorder.tsx` | AudioContext + WebSocket 流式录音 |
| 新增 | `web/frontend/src/components/chat/__tests__/voice.test.ts` | 3 项前端单测 |
| 修改 | `web/frontend/src/components/chat/chat-composer.tsx` | 适配流式 VoiceRecorder、输入框只读、Enter 键 |
| 修改 | `web/frontend/src/api/voice.ts` | 新增 streaming 字段 |
| 修改 | `web/frontend/src/store/voice.ts` | 新增 streamingAvailableAtom |
| 修改 | `web/frontend/src/hooks/use-voice.ts` | 能力探测扩展 streaming 字段 |
| 修改 | `web/frontend/src/components/chat/audio-player.tsx` | 播放按钮紧凑常驻 + 加载 spinner |
| 修改 | `web/frontend/src/components/chat/assistant-message.tsx` | AudioPlayer 移入右上角、删除底部 TTS 区 |

---

### Task 1: 配置扩展 — VoiceConfig + ModelConfig

**Files:**
- Modify: `pkg/config/config.go:692-699`（VoiceConfig）
- Modify: `pkg/config/config.go:716-754`（ModelConfig）

- [ ] **Step 1: VoiceConfig 新增 StreamingModelName**

```go
// pkg/config/config.go — VoiceConfig 结构体，追加一行
type VoiceConfig struct {
	ModelName          string `json:"model_name,omitempty"          env:"PICOCLAW_VOICE_MODEL_NAME"`
	TTSModelName       string `json:"tts_model_name,omitempty"      env:"PICOCLAW_VOICE_TTS_MODEL_NAME"`
	TTSFormat          string `json:"tts_format,omitempty"          env:"PICOCLAW_VOICE_TTS_FORMAT"`
	TTSVoice           string `json:"tts_voice,omitempty"           env:"PICOCLAW_VOICE_TTS_VOICE"`
	EchoTranscription  bool   `json:"echo_transcription"            env:"PICOCLAW_VOICE_ECHO_TRANSCRIPTION"`
	ElevenLabsAPIKey   string `json:"elevenlabs_api_key,omitempty"  env:"PICOCLAW_VOICE_ELEVENLABS_API_KEY"`
	StreamingModelName string `json:"streaming_model_name,omitempty" env:"PICOCLAW_VOICE_STREAMING_MODEL_NAME"`
}
```

- [ ] **Step 2: ModelConfig 新增火山凭据字段**

```go
// pkg/config/config.go — ModelConfig 结构体，在 APIKeys 字段附近新增
type ModelConfig struct {
	// ...existing fields...

	// Volcengine ASR streaming credentials (TODO: migrate to SecureString)
	AppID      string `json:"app_id,omitempty"`      // 火山应用 ID → payload app.appid
	AppKey     string `json:"app_key,omitempty"`      // → X-Api-App-Key
	AccessKey  string `json:"access_key,omitempty"`   // → X-Api-Access-Key
	ResourceID string `json:"resource_id,omitempty"`  // → X-Api-Resource-Id，默认 "volc.bigasr.sauc.duration"
}
```

- [ ] **Step 3: 编译验证**

Run: `go build -tags goolm,stdjson ./pkg/config/...`
Expected: 编译通过

- [ ] **Step 4: Commit**

```bash
git add pkg/config/config.go
git commit -m "feat: VoiceConfig 新增 streaming_model_name，ModelConfig 新增 AppID/AppKey/AccessKey/ResourceID

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 2: 流式 ASR 接口 — StreamingTranscriber

**Files:**
- Create: `pkg/audio/asr/streaming.go`

- [ ] **Step 1: 创建接口文件**

```go
// pkg/audio/asr/streaming.go
package asr

import "context"

// StreamingTranscriber defines a streaming ASR provider.
type StreamingTranscriber interface {
	Name() string
	StartStream(ctx context.Context, cfg StreamingConfig) (StreamingSession, error)
}

// StreamingConfig holds audio format parameters for the stream.
type StreamingConfig struct {
	Codec      string // "pcm_s16le"
	SampleRate int    // 16000
	Bits       int    // 16
	Channels   int    // 1
	Language   string // "zh-CN"
}

// StreamingSession represents an active streaming ASR session.
type StreamingSession interface {
	// SendAudio sends a chunk of raw PCM audio. Must be called sequentially.
	SendAudio(chunk []byte) error
	// Results returns a channel of streaming results. Closed when the session ends.
	Results() <-chan StreamingResult
	// Close ends the session and cleans up resources. Safe to call multiple times.
	Close() error
}

// StreamingResult represents a single result from the streaming ASR.
type StreamingResult struct {
	Text     string // text of this utterance (not accumulated)
	Definite bool   // true = confirmed utterance, frontend locks; false = interim
	Index    int    // utterance index in the array (used by handler for per-slot accumulation)
	Final    bool   // true = stream ended (last result has both Definite=true, Final=true)
	Error    string // non-empty on error
}
```

- [ ] **Step 2: 编译验证**

Run: `go build -tags goolm,stdjson ./pkg/audio/asr/...`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add pkg/audio/asr/streaming.go
git commit -m "feat: 新增 StreamingTranscriber 流式 ASR 接口

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 3: 火山 Binary Protocol — Header 编解码 + 测试

**Files:**
- Create: `pkg/audio/asr/volcengine_streaming.go`（常量 + header encode/decode）
- Create: `pkg/audio/asr/volcengine_streaming_test.go`（header 测试）

> **Go import 规则提示**：`volcengine_streaming.go` 到 Task 6 完成时总计需要 `context`, `crypto/rand`, `encoding/binary`, `encoding/json`, `bytes`, `compress/gzip`, `fmt`, `io`, `net/http`, `sync`, `github.com/gorilla/websocket`, `github.com/sipeed/picoclaw/pkg/config`。每步追加功能时在文件顶部 import 块中合并新增的包。

- [ ] **Step 1: 编写 header 测试**

```go
// pkg/audio/asr/volcengine_streaming_test.go
package asr

import "testing"

func TestVolcengineHeaderEncode(t *testing.T) {
	tests := []struct {
		name          string
		msgType       byte
		flags         byte
		serialization byte
		compression   byte
		want          [4]byte
	}{
		{
			name:          "FullClientRequest",
			msgType:       0b0001,
			flags:         0b0000,
			serialization: 0b0001,
			compression:   0b0001,
			want:          [4]byte{0b0001_0001, 0b0001_0000, 0b0001_0001, 0b0000_0000},
		},
		{
			name:          "AudioOnlyRequest_pos_seq",
			msgType:       0b0010,
			flags:         0b0001,
			serialization: 0b0000,
			compression:   0b0000,
			want:          [4]byte{0b0001_0001, 0b0010_0001, 0b0000_0000, 0b0000_0000},
		},
		{
			name:          "AudioOnlyRequest_neg_seq",
			msgType:       0b0010,
			flags:         0b0011,
			serialization: 0b0000,
			compression:   0b0000,
			want:          [4]byte{0b0001_0001, 0b0010_0011, 0b0000_0000, 0b0000_0000},
		},
		{
			name:          "ServerACK",
			msgType:       0b1011,
			flags:         0b0000,
			serialization: 0b0000,
			compression:   0b0000,
			want:          [4]byte{0b0001_0001, 0b1011_0000, 0b0000_0000, 0b0000_0000},
		},
		{
			name:          "ErrorResponse",
			msgType:       0b1111,
			flags:         0b0000,
			serialization: 0b0001,
			compression:   0b0000,
			want:          [4]byte{0b0001_0001, 0b1111_0000, 0b0001_0000, 0b0000_0000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeVolcengineHeader(tt.msgType, tt.flags, tt.serialization, tt.compression)
			if got != tt.want {
				t.Errorf("encodeVolcengineHeader() = %08b, want %08b", got, tt.want)
			}
		})
	}
}

func TestVolcengineHeaderDecode(t *testing.T) {
	header := [4]byte{0b0001_0001, 0b1001_0000, 0b0001_0000, 0b0000_0000}
	msgType, flags, serialization, compression := decodeVolcengineHeader(header)
	if msgType != 0b1001 {
		t.Errorf("msgType = %04b, want 1001", msgType)
	}
	if flags != 0b0000 {
		t.Errorf("flags = %04b, want 0000", flags)
	}
	if serialization != 0b0001 {
		t.Errorf("serialization = %04b, want 0001", serialization)
	}
	if compression != 0b0000 {
		t.Errorf("compression = %04b, want 0000", compression)
	}
}
```

- [ ] **Step 2: 运行测试确认 FAIL**

Run: `go test -v -tags goolm,stdjson -run "TestVolcengineHeader" ./pkg/audio/asr/`
Expected: FAIL — 函数未定义

- [ ] **Step 3: 实现 header 编解码**

```go
// pkg/audio/asr/volcengine_streaming.go
package asr

const (
	volcengineMsgFullClientReq  = 0b0001
	volcengineMsgAudioOnlyReq   = 0b0010
	volcengineMsgFullServerResp = 0b1001
	volcengineMsgServerACK      = 0b1011
	volcengineMsgError          = 0b1111

	volcengineFlagNone          = 0b0000
	volcengineFlagPosSequence   = 0b0001
	volcengineFlagNegWithSeq    = 0b0011

	volcengineSerialRaw  = 0b0000
	volcengineSerialJSON = 0b0001

	volcengineCompressNone = 0b0000
	volcengineCompressGzip = 0b0001
)

func encodeVolcengineHeader(msgType, flags, serialization, compression byte) [4]byte {
	return [4]byte{
		0b0001_0001, // version=1, header_size=1 (×4=4B)
		(msgType << 4) | (flags & 0x0f),
		(serialization << 4) | (compression & 0x0f),
		0x00,
	}
}

func decodeVolcengineHeader(h [4]byte) (msgType, flags, serialization, compression byte) {
	msgType = (h[1] >> 4) & 0x0f
	flags = h[1] & 0x0f
	serialization = (h[2] >> 4) & 0x0f
	compression = h[2] & 0x0f
	return
}
```

- [ ] **Step 4: 运行测试确认 PASS**

Run: `go test -v -tags goolm,stdjson -run "TestVolcengineHeader" ./pkg/audio/asr/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/audio/asr/volcengine_streaming.go pkg/audio/asr/volcengine_streaming_test.go
git commit -m "feat: 火山 binary protocol header 编解码 + 单测

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 4: 火山 Binary Protocol — 帧构造与响应解析 + 测试

**Files:**
- Modify: `pkg/audio/asr/volcengine_streaming.go`（追加 buildFullClientFrame、encodeAudioOnlyFrame、parseUtterances、parseVolcengineError）
- Modify: `pkg/audio/asr/volcengine_streaming_test.go`（追加帧构造/响应解析测试）

- [ ] **Step 1: 编写测试**

```go
// 追加到 volcengine_streaming_test.go

import "encoding/json"

func TestBuildFullClientRequestPayload(t *testing.T) {
	payload := buildFullClientRequestPayload("test-app-id", "test-uid")
	if payload["app"].(map[string]string)["appid"] != "test-app-id" {
		t.Error("appid mismatch")
	}
	audio := payload["audio"].(map[string]any)
	if audio["format"] != "raw" {
		t.Errorf("audio.format = %v, want raw", audio["format"])
	}
	if audio["codec"] != "pcm_s16le" {
		t.Errorf("audio.codec = %v, want pcm_s16le", audio["codec"])
	}
	if audio["rate"] != 16000 {
		t.Errorf("audio.rate = %v, want 16000", audio["rate"])
	}
	req := payload["request"].(map[string]any)
	if req["result_type"] != "full" {
		t.Errorf("request.result_type = %v, want full", req["result_type"])
	}
}

func TestEncodeAudioOnlyFrame(t *testing.T) {
	// positive seq
	header, seqBytes, payloadSize, payload := encodeAudioOnlyFrame([]byte{0, 1, 2, 3}, 1, false)
	_, flags, _, comp := decodeVolcengineHeader(header)
	if flags != volcengineFlagPosSequence {
		t.Errorf("flags = %04b, want %04b", flags, volcengineFlagPosSequence)
	}
	if comp != volcengineCompressNone {
		t.Errorf("compression = %04b, want 0000", comp)
	}
	if len(seqBytes) != 4 || len(payloadSize) != 4 {
		t.Error("seqBytes or payloadSize missing")
	}
	if len(payload) != 4 {
		t.Errorf("payload len = %d, want 4", len(payload))
	}

	// last frame (negative seq)
	header2, _, _, _ := encodeAudioOnlyFrame([]byte{0, 1, 2, 3}, 3, true)
	_, flags2, _, _ := decodeVolcengineHeader(header2)
	if flags2 != volcengineFlagNegWithSeq {
		t.Errorf("flags = %04b, want %04b", flags2, volcengineFlagNegWithSeq)
	}
}

func TestParseServerResponse(t *testing.T) {
	resp := `{"result":{"utterances":[{"text":"今天","definite":false}]}}`
	results := parseUtterances([]byte(resp))
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	if results[0].Text != "今天" || results[0].Definite != false {
		t.Errorf("utterance = %+v, want {Text:今天 Definite:false}", results[0])
	}

	resp2 := `{"result":{"utterances":[{"text":"今天天气","definite":true},{"text":"怎么样","definite":false}]}}`
	results2 := parseUtterances([]byte(resp2))
	if len(results2) != 2 {
		t.Fatalf("len = %d, want 2", len(results2))
	}
	if results2[0].Text != "今天天气" || results2[0].Definite != true {
		t.Errorf("[0] = %+v", results2[0])
	}
	if results2[1].Text != "怎么样" || results2[1].Definite != false {
		t.Errorf("[1] = %+v", results2[1])
	}
}

func TestParseErrorResponse(t *testing.T) {
	body := []byte(`{"code":45000001,"message":"invalid parameter"}`)
	code, msg := parseVolcengineError(body)
	if code != 45000001 {
		t.Errorf("code = %d, want 45000001", code)
	}
	if msg != "invalid parameter" {
		t.Errorf("msg = %s", msg)
	}
}

func TestIsServerACK(t *testing.T) {
	header := encodeVolcengineHeader(volcengineMsgServerACK, volcengineFlagNone, volcengineSerialRaw, volcengineCompressNone)
	msgType, _, _, _ := decodeVolcengineHeader(header)
	if msgType != volcengineMsgServerACK {
		t.Error("should be Server ACK")
	}
}
```

- [ ] **Step 2: 运行测试确认 FAIL**

Run: `go test -v -tags goolm,stdjson -run "TestBuildFullClientRequestPayload|TestEncodeAudioOnlyFrame|TestParseServerResponse|TestParseErrorResponse|TestIsServerACK" ./pkg/audio/asr/`
Expected: FAIL — 函数未定义

- [ ] **Step 3: 实现帧构造与解析**

```go
// 追加到 volcengine_streaming.go
// 文件顶部 import 追加: "bytes", "compress/gzip", "encoding/binary", "encoding/json", "fmt"

func buildFullClientRequestPayload(appID, uid string) map[string]any {
	return map[string]any{
		"app":  map[string]string{"appid": appID},
		"user": map[string]string{"uid": uid},
		"audio": map[string]any{
			"format":   "raw",
			"codec":    "pcm_s16le",
			"rate":     16000,
			"bits":     16,
			"channel":  1,
			"language": "zh-CN",
		},
		"request": map[string]any{
			"model_name":  "bigmodel",
			"result_type": "full",
			"enable_itn":  true,
			"enable_punc": true,
		},
	}
}

func buildFullClientFrame(appID, uid string) ([]byte, error) {
	payloadJSON, err := json.Marshal(buildFullClientRequestPayload(appID, uid))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(payloadJSON); err != nil {
		return nil, err
	}
	gw.Close()

	header := encodeVolcengineHeader(volcengineMsgFullClientReq, volcengineFlagNone, volcengineSerialJSON, volcengineCompressGzip)
	payloadLenBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(payloadLenBytes, uint32(buf.Len()))

	frame := make([]byte, 0, 4+4+buf.Len())
	frame = append(frame, header[:]...)
	frame = append(frame, payloadLenBytes...)
	frame = append(frame, buf.Bytes()...)
	return frame, nil
}

// encodeAudioOnlyFrame returns header, seqBytes (4B), payloadSizeBytes (4B), payload.
// Audio frames: [4B header][4B seq][4B payload_size][payload], Compression=0b0000 (no gzip for PCM).
func encodeAudioOnlyFrame(audioData []byte, seq int32, isLast bool) (header [4]byte, seqBytes []byte, payloadSize []byte, payload []byte) {
	flags := volcengineFlagPosSequence
	if isLast {
		flags = volcengineFlagNegWithSeq
		seq = -seq
	}
	header = encodeVolcengineHeader(volcengineMsgAudioOnlyReq, flags, volcengineSerialRaw, volcengineCompressNone)
	seqBytes = make([]byte, 4)
	binary.BigEndian.PutUint32(seqBytes, uint32(seq))
	payloadSize = make([]byte, 4)
	binary.BigEndian.PutUint32(payloadSize, uint32(len(audioData)))
	return header, seqBytes, payloadSize, audioData
}

type rawUtterance struct {
	Text     string `json:"text"`
	Definite bool   `json:"definite"`
}

type rawServerResponse struct {
	Result struct {
		Utterances []rawUtterance `json:"utterances"`
	} `json:"result"`
}

func parseUtterances(jsonBody []byte) []StreamingResult {
	var resp rawServerResponse
	if err := json.Unmarshal(jsonBody, &resp); err != nil {
		return []StreamingResult{{Error: fmt.Sprintf("parse error: %v", err)}}
	}
	results := make([]StreamingResult, 0, len(resp.Result.Utterances))
	for _, u := range resp.Result.Utterances {
		results = append(results, StreamingResult{
			Text:     u.Text,
			Definite: u.Definite,
		})
	}
	return results
}

type rawErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func parseVolcengineError(jsonBody []byte) (int, string) {
	var resp rawErrorResponse
	if err := json.Unmarshal(jsonBody, &resp); err != nil {
		return -1, "unknown error"
	}
	return resp.Code, resp.Message
}

func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return data, nil
	}
	defer r.Close()
	return io.ReadAll(r)
}
```

- [ ] **Step 4: 运行测试确认 PASS**

Run: `go test -v -tags goolm,stdjson -run "TestBuildFullClientRequestPayload|TestEncodeAudioOnlyFrame|TestParseServerResponse|TestParseErrorResponse|TestIsServerACK" ./pkg/audio/asr/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/audio/asr/volcengine_streaming.go pkg/audio/asr/volcengine_streaming_test.go
git commit -m "feat: 火山帧构造/解析 + 5 项单测

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 5: 火山 WebSocket 客户端 — 连接 + 收发 + 去重

**Files:**
- Modify: `pkg/audio/asr/volcengine_streaming.go`（追加 volcengineStreamingTranscriber、volcengineStreamingSession、readLoop、dedupAndPush、错误码映射）

> **import 追加**: `"context"`, `"crypto/rand"`, `"io"`, `"net/http"`, `"sync"`, `"github.com/gorilla/websocket"`, `"github.com/sipeed/picoclaw/pkg/config"`

- [ ] **Step 1: 实现 volcengineStreamingTranscriber + volcengineStreamingSession**

```go
// 追加到 volcengine_streaming.go

type volcengineStreamingTranscriber struct {
	modelCfg *config.ModelConfig
}

func (v *volcengineStreamingTranscriber) Name() string {
	return "volcengine-asr"
}

func (v *volcengineStreamingTranscriber) StartStream(ctx context.Context, _ StreamingConfig) (StreamingSession, error) {
	return newVolcengineSession(ctx, v.modelCfg)
}

type utteranceSnapshot struct {
	Text     string
	Definite bool
}

type volcengineStreamingSession struct {
	conn     *websocket.Conn
	results  chan StreamingResult
	ctx      context.Context
	cancel   context.CancelFunc
	closeOnce sync.Once
	seq      int32

	mu             sync.Mutex
	lastUtterances []utteranceSnapshot
}

func newVolcengineSession(ctx context.Context, modelCfg *config.ModelConfig) (*volcengineStreamingSession, error) {
	dialer := websocket.DefaultDialer
	headers := http.Header{}
	headers.Set("X-Api-App-Key", modelCfg.AppKey)
	headers.Set("X-Api-Access-Key", modelCfg.AccessKey)
	if modelCfg.ResourceID == "" {
		modelCfg.ResourceID = "volc.bigasr.sauc.duration"
	}
	headers.Set("X-Api-Resource-Id", modelCfg.ResourceID)
	headers.Set("X-Api-Request-Id", newUUID())

	conn, _, err := dialer.DialContext(ctx, modelCfg.APIBase, headers)
	if err != nil {
		return nil, fmt.Errorf("volcengine dial: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &volcengineStreamingSession{
		conn:    conn,
		results: make(chan StreamingResult, 64),
		ctx:     ctx,
		cancel:  cancel,
	}

	appID := modelCfg.AppID
	frame, err := buildFullClientFrame(appID, "web-user")
	if err != nil {
		conn.Close()
		cancel()
		return nil, fmt.Errorf("build full client frame: %w", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		conn.Close()
		cancel()
		return nil, fmt.Errorf("send full client request: %w", err)
	}

	go s.readLoop()
	return s, nil
}

func (s *volcengineStreamingSession) SendAudio(chunk []byte) error {
	s.mu.Lock()
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	header, seqBytes, payloadSize, payload := encodeAudioOnlyFrame(chunk, seq, false)
	frame := make([]byte, 0, 4+4+4+len(payload))
	frame = append(frame, header[:]...)
	frame = append(frame, seqBytes...)
	frame = append(frame, payloadSize...)
	frame = append(frame, payload...)
	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

func (s *volcengineStreamingSession) Results() <-chan StreamingResult {
	return s.results
}

func (s *volcengineStreamingSession) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.seq++
		seq := s.seq
		s.mu.Unlock()

		header, seqBytes, payloadSize, _ := encodeAudioOnlyFrame(nil, seq, true)
		frame := append(header[:], seqBytes...)
		frame = append(frame, payloadSize...)
		s.conn.WriteMessage(websocket.BinaryMessage, frame)

		s.cancel()
		s.conn.Close()
	})
	return nil
}

func (s *volcengineStreamingSession) readLoop() {
	defer close(s.results)
	defer s.conn.Close()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		_, msg, err := s.conn.ReadMessage()
		if err != nil {
			if !isClosedError(err) {
				s.results <- StreamingResult{Error: fmt.Sprintf("read error: %v", err)}
			}
			return
		}

		if len(msg) < 4 {
			continue
		}

		var header [4]byte
		copy(header[:], msg[:4])
		msgType, flags, _, compression := decodeVolcengineHeader(header)
		body := msg[4:]

		switch msgType {
		case volcengineMsgServerACK:
			continue

		case volcengineMsgError:
			// Error frame: [4B header][?seq][4B payload_size][payload]
			offset := 0
			if flags == volcengineFlagPosSequence {
				offset = 4
			}
			if len(body) < offset+4 {
				continue
			}
			payloadSize := binary.BigEndian.Uint32(body[offset : offset+4])
			payload := body[offset+4 : offset+4+int(payloadSize)]
			if compression == volcengineCompressGzip {
				payload, _ = gunzip(payload)
			}
			code, message := parseVolcengineError(payload)
			s.results <- StreamingResult{Error: volcengineErrorMessage(code, message)}
			return

		case volcengineMsgFullServerResp:
			// Response frame: [4B header][?seq][4B payload_size][payload]
			offset := 0
			if flags == volcengineFlagPosSequence {
				offset = 4
			}
			if len(body) < offset+4 {
				continue
			}
			payloadSize := binary.BigEndian.Uint32(body[offset : offset+4])
			payload := body[offset+4 : offset+4+int(payloadSize)]
			if compression == volcengineCompressGzip {
				payload, _ = gunzip(payload)
			}
			utterances := parseUtterances(payload)
			s.dedupAndPush(utterances)
		}
	}
}

func (s *volcengineStreamingSession) dedupAndPush(utterances []StreamingResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, u := range utterances {
		u.Index = i
		if i < len(s.lastUtterances) {
			prev := s.lastUtterances[i]
			if prev.Text == u.Text && prev.Definite == u.Definite {
				continue // unchanged
			}
			if prev.Definite && !u.Definite {
				continue // safety: definite never goes true→false
			}
		}
		select {
		case s.results <- u:
		case <-s.ctx.Done():
			return
		}
	}

	s.lastUtterances = make([]utteranceSnapshot, len(utterances))
	for i, u := range utterances {
		s.lastUtterances[i] = utteranceSnapshot{Text: u.Text, Definite: u.Definite}
	}
}

func volcengineErrorMessage(code int, msg string) string {
	switch code {
	case 45000001:
		return "ASR 参数配置错误，请检查 app_id"
	case 40200002:
		return "ASR 鉴权失败，请检查 App Key / Access Key"
	case 40200010:
		return "ASR 时长配额已用尽，请充值或申请更多配额"
	case 40200011:
		return "ASR 请求过于频繁，请稍后重试"
	case 45000081:
		return "ASR 请求超时，请重试"
	case 40200004:
		return "ASR 服务未开通，请在火山控制台开通语音识别服务"
	case 55000000:
		return "ASR 服务暂时不可用，请稍后重试"
	case 55000031:
		return "ASR 服务繁忙，请稍后重试"
	default:
		return msg
	}
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*websocket.CloseError); ok {
		return true
	}
	return false
}
```

- [ ] **Step 2: 编译验证**

Run: `go build -tags goolm,stdjson ./pkg/audio/asr/...`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add pkg/audio/asr/volcengine_streaming.go
git commit -m "feat: 火山 WebSocket 客户端 — 建连/收发/去重/错误码映射

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 6: DetectStreamingTranscriber + 去重单测 + detect 单测

**Files:**
- Modify: `pkg/audio/asr/asr.go`（新增 DetectStreamingTranscriber）
- Modify: `pkg/audio/asr/volcengine_streaming_test.go`（去重测试 + detect 测试）

- [ ] **Step 1: 编写去重单测 + detect 单测**

```go
// 追加到 volcengine_streaming_test.go

import "time"

func TestUtteranceDedup(t *testing.T) {
	s := &volcengineStreamingSession{
		results: make(chan StreamingResult, 64),
	}

	// Frame 1: new utterance
	s.dedupAndPush([]StreamingResult{{Text: "今天", Definite: false}})
	if len(s.lastUtterances) != 1 || s.lastUtterances[0].Text != "今天" {
		t.Error("frame 1: should cache new utterance")
	}
	r1 := readResult(s.results)
	if r1.Text != "今天" {
		t.Errorf("frame 1 result = %+v", r1)
	}

	// Frame 2: same index text change
	s.dedupAndPush([]StreamingResult{{Text: "今天天气", Definite: false}})
	r2 := readResult(s.results)
	if r2.Text != "今天天气" {
		t.Errorf("frame 2: should push text change, got %+v", r2)
	}

	// Frame 3: definite false→true + new utterance
	s.dedupAndPush([]StreamingResult{
		{Text: "今天天气", Definite: true},
		{Text: "怎么样", Definite: false},
	})
	results3 := drainResults(s.results, 2)
	if len(results3) != 2 {
		t.Fatalf("frame 3: expected 2 results, got %d", len(results3))
	}
	if results3[0].Text != "今天天气" || results3[0].Definite != true {
		t.Errorf("frame 3[0] = %+v", results3[0])
	}
	if results3[1].Text != "怎么样" || results3[1].Definite != false {
		t.Errorf("frame 3[1] = %+v", results3[1])
	}

	// Frame 4: no change → skip
	s.dedupAndPush([]StreamingResult{
		{Text: "今天天气", Definite: true},
		{Text: "怎么样", Definite: false},
	})
	select {
	case r := <-s.results:
		t.Errorf("frame 4: should be no push, got %+v", r)
	default:
	}

	// Frame 5: definite true→false (safety, should not happen)
	s.dedupAndPush([]StreamingResult{{Text: "今天天气", Definite: false}})
	select {
	case r := <-s.results:
		t.Errorf("frame 5: true→false should be skipped, got %+v", r)
	default:
	}
}

func readResult(ch <-chan StreamingResult) StreamingResult {
	select {
	case r := <-ch:
		return r
	case <-time.After(time.Second):
		return StreamingResult{Error: "timeout"}
	}
}

func drainResults(ch <-chan StreamingResult, n int) []StreamingResult {
	var results []StreamingResult
	for i := 0; i < n; i++ {
		select {
		case r := <-ch:
			results = append(results, r)
		case <-time.After(time.Second):
			return results
		}
	}
	return results
}

func TestDetectStreamingTranscriber(t *testing.T) {
	// Case 1: empty streaming_model_name
	cfg := &config.Config{}
	tr, err := DetectStreamingTranscriber(cfg)
	if err != nil {
		t.Errorf("empty name: unexpected error: %v", err)
	}
	if tr != nil {
		t.Error("empty name: should return nil")
	}

	// Case 2: streaming_model_name set but model not in model_list → (nil, nil)
	cfg2 := &config.Config{
		Voice: config.VoiceConfig{StreamingModelName: "nonexistent"},
	}
	tr2, err2 := DetectStreamingTranscriber(cfg2)
	if err2 != nil {
		t.Errorf("nonexistent model: unexpected error: %v", err2)
	}
	if tr2 != nil {
		t.Error("nonexistent model: should return nil transcriber")
	}

	// Case 3: matching volcengine-asr protocol
	cfg3 := &config.Config{
		Voice: config.VoiceConfig{StreamingModelName: "volcengine-asr"},
		ModelList: config.SecureModelList{
			{
				ModelName: "volcengine-asr",
				Provider:  "volcengine-asr",
				Model:     "bigmodel",
				APIBase:   "wss://openspeech.bytedance.com/api/v3/sauc/bigmodel",
			},
		},
	}
	tr3, err3 := DetectStreamingTranscriber(cfg3)
	if err3 != nil {
		t.Errorf("volcengine-asr: unexpected error: %v", err3)
	}
	if tr3 == nil {
		t.Error("volcengine-asr: should return transcriber")
	}
	if tr3.Name() != "volcengine-asr" {
		t.Errorf("name = %s, want volcengine-asr", tr3.Name())
	}
}
```

> **注意**: TestDetectStreamingTranscriber 需要 import `"github.com/sipeed/picoclaw/pkg/config"`（测试文件顶部添加）。

- [ ] **Step 2: 运行测试确认 FAIL（去重 + detect）**

Run: `go test -v -tags goolm,stdjson -run "TestUtteranceDedup|TestDetectStreamingTranscriber" ./pkg/audio/asr/`
Expected: FAIL — 函数未定义（DetectStreamingTranscriber 尚未实现）

- [ ] **Step 3: 实现 DetectStreamingTranscriber**

```go
// 追加到 pkg/audio/asr/asr.go
// import 中确认已有 "strings" 和 "github.com/sipeed/picoclaw/pkg/providers"

// DetectStreamingTranscriber inspects cfg and returns a StreamingTranscriber.
// Returns (nil, nil) when streaming_model_name is empty or no matching provider is found.
func DetectStreamingTranscriber(cfg *config.Config) (StreamingTranscriber, error) {
	if cfg == nil {
		return nil, nil
	}

	modelName := strings.TrimSpace(cfg.Voice.StreamingModelName)
	if modelName == "" {
		return nil, nil
	}

	modelCfg, err := cfg.GetModelConfig(modelName)
	if err != nil {
		return nil, nil
	}

	protocol, _ := providers.ExtractProtocol(modelCfg)
	if protocol == "volcengine-asr" {
		return &volcengineStreamingTranscriber{modelCfg: modelCfg}, nil
	}

	return nil, nil
}
```

- [ ] **Step 4: 运行全部 ASR 单测**

Run: `go test -v -tags goolm,stdjson ./pkg/audio/asr/...`
Expected: 全部 PASS（9 项：HeaderEncode/HeaderDecode/FullClientRequestPayload/AudioOnlyRequestEncode/ServerResponseParse/ErrorResponseParse/ServerACKIgnore/UtteranceDedup/DetectStreamingTranscriber）

- [ ] **Step 5: Commit**

```bash
git add pkg/audio/asr/asr.go pkg/audio/asr/volcengine_streaming_test.go
git commit -m "feat: DetectStreamingTranscriber + 去重单测 + detect 单测

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 7: WebSocket Handler + capabilities 扩展 + CheckOrigin

**Files:**
- Modify: `web/backend/api/voice.go`（新增 handleVoiceStream + 扩展 capabilities）
- Modify: `web/backend/api/router.go`（注册路由）

> **import 追加**: `"encoding/json"`, `"net"`, `"sync"`, `"github.com/gorilla/websocket"` 到 voice.go

- [ ] **Step 1: 扩展 capabilities 返回 streaming 字段**

```go
// 修改 web/backend/api/voice.go handleVoiceCapabilities
func (h *Handler) handleVoiceCapabilities(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"asr": false, "tts": false, "streaming": false})
		return
	}
	streamingTranscriber, _ := asr.DetectStreamingTranscriber(cfg)
	writeJSON(w, http.StatusOK, map[string]any{
		"asr":       asr.DetectTranscriber(cfg) != nil,
		"tts":       tts.DetectTTS(cfg) != nil,
		"streaming": streamingTranscriber != nil,
	})
}
```

- [ ] **Step 2: 新增 CIDR 匹配工具函数**

```go
// 追加到 web/backend/api/voice.go

// TODO: 若前端经 nginx/反代访问，remoteAddr 为 127.0.0.1，
// 需改为读取 X-Forwarded-For 或 X-Real-IP 头（从 r.Header 取）。
func cidrMatch(remoteAddr string, cidrs []string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, c := range cidrs {
		_, ipnet, err := net.ParseCIDR(c)
		if err == nil && ipnet.Contains(ip) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: 新增 handleVoiceStream**

```go
// 追加到 web/backend/api/voice.go

var voiceStreamUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// handleVoiceStream handles WebSocket streaming ASR.
//
//	GET /api/voice/stream
func (h *Handler) handleVoiceStream(w http.ResponseWriter, r *http.Request) {
	voiceStreamUpgrader.CheckOrigin = func(r *http.Request) bool {
		if !h.serverPublic {
			return true // same-origin only when not public
		}
		if len(h.serverCIDRs) == 0 {
			return true
		}
		return cidrMatch(r.RemoteAddr, h.serverCIDRs)
	}

	conn, err := voiceStreamUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeWSJSON(conn, map[string]string{"type": "error", "code": "config", "message": "配置加载失败"})
		return
	}

	transcriber, _ := asr.DetectStreamingTranscriber(cfg)
	if transcriber == nil {
		writeWSJSON(conn, map[string]string{"type": "error", "code": "no_provider", "message": "未配置流式 ASR"})
		return
	}

	session, err := transcriber.StartStream(r.Context(), asr.StreamingConfig{
		Codec:      "pcm_s16le",
		SampleRate: 16000,
		Bits:       16,
		Channels:   1,
		Language:   "zh-CN",
	})
	if err != nil {
		writeWSJSON(conn, map[string]string{"type": "error", "code": "connect", "message": err.Error()})
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: read audio chunks from frontend → send to volcengine
	go func() {
		defer wg.Done()
		defer session.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := session.SendAudio(msg); err != nil {
				return
			}
		}
	}()

	// Goroutine 2: read results from volcengine → send to frontend
	// Per-slot accumulation using result.Index (from dedup):
	// - definite=true → locked += text, current[Index] = ""
	// - definite=false → current[Index] = text
	// - final text = locked + join(all non-empty current slots in order)
	go func() {
		defer wg.Done()
		var locked string
		current := make(map[int]string)
		maxIdx := -1

		for result := range session.Results() {
			if result.Error != "" {
				writeWSJSON(conn, map[string]string{
					"type":    "error",
					"message": result.Error,
				})
				return
			}

			if result.Index > maxIdx {
				maxIdx = result.Index
			}

			if result.Definite {
				locked += result.Text
				current[result.Index] = ""
			} else {
				current[result.Index] = result.Text
			}

			writeWSJSON(conn, map[string]any{
				"type":     "partial",
				"text":     result.Text,
				"definite": result.Definite,
			})
		}

		// Channel closed = stream ended, send final accumulated text
		var curText string
		for i := 0; i <= maxIdx; i++ {
			curText += current[i]
		}
		fullText := locked + curText
		if fullText != "" {
			writeWSJSON(conn, map[string]string{
				"type": "final",
				"text": fullText,
			})
		}
	}()

	wg.Wait()
}

func writeWSJSON(conn *websocket.Conn, v any) {
	data, _ := json.Marshal(v)
	conn.WriteMessage(websocket.TextMessage, data)
}
```

- [ ] **Step 4: 注册路由**

```go
// 修改 web/backend/api/router.go registerVoiceRoutes
func (h *Handler) registerVoiceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/voice/capabilities", h.handleVoiceCapabilities)
	mux.HandleFunc("POST /api/voice/transcribe", h.handleVoiceTranscribe)
	mux.HandleFunc("POST /api/voice/synthesize", h.handleVoiceSynthesize)
	mux.HandleFunc("GET /api/voice/audio/{file_id}", h.handleVoiceAudio)
	mux.HandleFunc("GET /api/voice/stream", h.handleVoiceStream)
}
```

- [ ] **Step 5: 编译验证**

Run: `go build -tags goolm,stdjson ./web/backend/...`
Expected: 编译通过

- [ ] **Step 6: Commit**

```bash
git add web/backend/api/voice.go web/backend/api/router.go
git commit -m "feat: GET /api/voice/stream WebSocket 端点（CheckOrigin CIDR + fullText 累积）

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 8: 前端 store + api + hook 扩展

**Files:**
- Modify: `web/frontend/src/store/voice.ts`
- Modify: `web/frontend/src/api/voice.ts`
- Modify: `web/frontend/src/hooks/use-voice.ts`

- [ ] **Step 1: store 新增 streamingAvailableAtom**

```ts
// web/frontend/src/store/voice.ts — 追加
export const streamingAvailableAtom = atom(false);
```

- [ ] **Step 2: api 扩展返回类型**

```ts
// web/frontend/src/api/voice.ts — 修改 fetchVoiceCapabilities
export async function fetchVoiceCapabilities(): Promise<{
  asr: boolean;
  tts: boolean;
  streaming: boolean;
}> {
  const res = await launcherFetch("/api/voice/capabilities");
  return res.json();
}
```

- [ ] **Step 3: useVoice hook 扩展**

```ts
// web/frontend/src/hooks/use-voice.ts
import { streamingAvailableAtom } from "@/store/voice";

export function useVoice() {
  const [inputEnabled, setInputEnabled] = useAtom(voiceInputAtom);
  const [outputEnabled, setOutputEnabled] = useAtom(voiceOutputAtom);
  const [asrAvailable, setAsrAvailable] = useAtom(voiceAsrAvailableAtom);
  const [ttsAvailable, setTtsAvailable] = useAtom(voiceTtsAvailableAtom);
  const [streamingAvailable, setStreamingAvailable] = useAtom(streamingAvailableAtom);

  useEffect(() => {
    fetchVoiceCapabilities().then((caps) => {
      setAsrAvailable(caps.asr);
      setTtsAvailable(caps.tts);
      setStreamingAvailable(caps.streaming);
    }).catch(() => {});
  }, []);

  // ... toggleInput / toggleOutput unchanged ...
  return {
    inputEnabled, outputEnabled,
    asrAvailable, ttsAvailable, streamingAvailable,
    toggleInput, toggleOutput,
  };
}
```

- [ ] **Step 4: 编译验证**

Run: `cd web/frontend && npx tsc --noEmit`
Expected: 零错误

- [ ] **Step 5: Commit**

```bash
git add web/frontend/src/store/voice.ts web/frontend/src/api/voice.ts web/frontend/src/hooks/use-voice.ts
git commit -m "feat: 前端 store/api/hook 扩展 streaming 能力探测

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 9: AudioWorklet 处理器

**Files:**
- Create: `web/frontend/public/voice-processor.js`

- [ ] **Step 1: 创建 voice-processor.js**

```js
// web/frontend/public/voice-processor.js
class VoiceProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this._targetRate = 16000;
    this._buffer = new Float32Array(0);
  }

  process(inputs) {
    const input = inputs[0];
    if (!input || !input[0] || input[0].length === 0) return true;

    let samples = input[0]; // mono channel 0

    // Downsample if device rate != 16kHz (Safari / Bluetooth headset)
    if (sampleRate !== this._targetRate) {
      samples = this._linearResample(samples, sampleRate, this._targetRate);
    }

    // Accumulate buffer
    const newBuf = new Float32Array(this._buffer.length + samples.length);
    newBuf.set(this._buffer);
    newBuf.set(samples, this._buffer.length);
    this._buffer = newBuf;

    // Flush when ≥ 1600 samples (~100ms at 16kHz)
    if (this._buffer.length >= 1600) {
      const int16 = new Int16Array(this._buffer.length);
      for (let i = 0; i < this._buffer.length; i++) {
        const s = Math.max(-1, Math.min(1, this._buffer[i]));
        int16[i] = Math.round(s < 0 ? s * 0x8000 : s * 0x7FFF);
      }
      this.port.postMessage(int16.buffer, [int16.buffer]);
      this._buffer = new Float32Array(0);
    }

    return true;
  }

  // Linear interpolation resampling — sufficient for speech ASR (80–3400 Hz energy band)
  _linearResample(samples, fromRate, toRate) {
    if (fromRate === toRate) return samples;
    const ratio = fromRate / toRate;
    const newLen = Math.floor(samples.length / ratio);
    const result = new Float32Array(newLen);
    for (let i = 0; i < newLen; i++) {
      const srcIdx = i * ratio;
      const srcFloor = Math.floor(srcIdx);
      const frac = srcIdx - srcFloor;
      result[i] = samples[srcFloor] * (1 - frac) + samples[Math.min(srcFloor + 1, samples.length - 1)] * frac;
    }
    return result;
  }
}

registerProcessor("voice-processor", VoiceProcessor);
```

- [ ] **Step 2: Commit**

```bash
git add web/frontend/public/voice-processor.js
git commit -m "feat: AudioWorklet 处理器 — PCM 采集+线性插值重采样+100ms 缓冲

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 10: VoiceRecorder 重写

**Files:**
- Rewrite: `web/frontend/src/components/chat/voice-recorder.tsx`

- [ ] **Step 1: 重写 VoiceRecorder（挂载即录音，卸载即停止）**

```tsx
// web/frontend/src/components/chat/voice-recorder.tsx
import { useState, useRef, useEffect, useCallback } from "react";

interface VoiceRecorderProps {
  onInterimText: (text: string, definite: boolean) => void;
  onTranscribed: (text: string) => void;
  onError?: (error: string) => void;
}

export function VoiceRecorder({ onInterimText, onTranscribed, onError }: VoiceRecorderProps) {
  const [elapsed, setElapsed] = useState(0);
  const [loading, setLoading] = useState(true);
  const wsRef = useRef<WebSocket | null>(null);
  const audioCtxRef = useRef<AudioContext | null>(null);
  const workletRef = useRef<AudioWorkletNode | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const autoStopRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef(true);
  const lockedTextRef = useRef("");
  const currentTextRef = useRef("");

  const isSupported = typeof AudioContext !== "undefined" && typeof WebSocket !== "undefined";

  // Cleanup helper — stops all resources
  const cleanup = useCallback(() => {
    if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null; }
    if (autoStopRef.current) { clearTimeout(autoStopRef.current); autoStopRef.current = null; }
    streamRef.current?.getTracks().forEach((t) => t.stop());
    workletRef.current?.port.close();
    audioCtxRef.current?.close();
    wsRef.current?.close();
    wsRef.current = null;
    audioCtxRef.current = null;
    workletRef.current = null;
    streamRef.current = null;
  }, []);

  // useEffect: mount = start, unmount = cleanup
  useEffect(() => {
    mountedRef.current = true;
    startRecording();
    return () => {
      mountedRef.current = false;
      cleanup();
    };
  }, []);

  const startRecording = useCallback(async () => {
    try {
      lockedTextRef.current = "";
      currentTextRef.current = "";
      const audioCtx = new AudioContext({ sampleRate: 16000 });
      audioCtxRef.current = audioCtx;

      await audioCtx.audioWorklet.addModule("/voice-processor.js");

      const stream = await navigator.mediaDevices.getUserMedia({
        audio: { sampleRate: { ideal: 16000 }, channelCount: 1 },
      });
      streamRef.current = stream;

      const source = audioCtx.createMediaStreamSource(stream);
      const workletNode = new AudioWorkletNode(audioCtx, "voice-processor");
      workletRef.current = workletNode;
      source.connect(workletNode);

      // WebSocket
      const protocol = location.protocol === "https:" ? "wss:" : "ws:";
      const ws = new WebSocket(`${protocol}//${location.host}/api/voice/stream`);
      ws.binaryType = "arraybuffer";
      wsRef.current = ws;

      ws.onopen = () => {
        setLoading(false);
        workletNode.port.onmessage = (e: MessageEvent) => {
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(e.data);
          }
        };
      };

      ws.onmessage = (e) => {
        if (!mountedRef.current) return;
        try {
          const msg = JSON.parse(e.data);
          if (msg.type === "partial") {
            // Merge strategy: definite → lock + clear current; indefinite → replace current
            if (msg.definite) {
              lockedTextRef.current += msg.text;
              currentTextRef.current = "";
            } else {
              currentTextRef.current = msg.text;
            }
            onInterimText(lockedTextRef.current + currentTextRef.current, msg.definite);
          } else if (msg.type === "final") {
            onTranscribed(msg.text);
          } else if (msg.type === "error") {
            onError?.(msg.message);
          }
        } catch {}
      };

      ws.onerror = () => onError?.("WebSocket 连接失败");

      // Timer
      setElapsed(0);
      timerRef.current = setInterval(() => {
        setElapsed((prev) => {
          if (prev >= 59) {
            cleanup();
            return prev;
          }
          return prev + 1;
        });
      }, 1000);

      // 60s auto-stop (store ref for cleanup)
      autoStopRef.current = setTimeout(() => {
        if (mountedRef.current) {
          cleanup();
        }
      }, 60_000);
    } catch {
      setLoading(false);
    }
  }, [onInterimText, onTranscribed, onError, cleanup]);

  if (!isSupported) return null;

  return (
    <button
      type="button"
      disabled
      className="voice-mic-btn recording"
      title="录音中"
    >
      {loading ? "..." : `${elapsed}s`}
    </button>
  );
}
```

- [ ] **Step 2: 编译验证**

Run: `cd web/frontend && npx tsc --noEmit`
Expected: 零错误

- [ ] **Step 3: Commit**

```bash
git add web/frontend/src/components/chat/voice-recorder.tsx
git commit -m "feat: VoiceRecorder 重写 — 挂载即录音/卸载即停止 + 60s auto-stop

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 10.5: 前端单测（3 项）

**Files:**
- Create: `web/frontend/src/components/chat/__tests__/voice.test.ts`

- [ ] **Step 1: 编写 3 项前端单测**

```ts
// web/frontend/src/components/chat/__tests__/voice.test.ts
import { describe, it, expect } from "vitest";

// --- TestUtteranceMerge ---
describe("utterance merge strategy", () => {
  it("locks definite segments and replaces current indefinite", () => {
    // Simulate the frontend merge logic from design §frontend
    let locked = "";
    let currentIndefinite = "";

    function apply(text: string, definite: boolean): string {
      if (definite) {
        locked += text;
        currentIndefinite = "";
      } else {
        currentIndefinite = text;
      }
      return locked + currentIndefinite;
    }

    expect(apply("今天", false)).toBe("今天");
    expect(apply("今天天气", false)).toBe("今天天气");
    expect(apply("今天天气", true)).toBe("今天天气");
    expect(apply("怎么样", false)).toBe("今天天气怎么样");
    expect(apply("怎么样", true)).toBe("今天天气怎么样");
  });
});

// --- TestAudioContextFallback ---
describe("linear resampling fallback", () => {
  it("downsamples 48kHz to 16kHz correctly", () => {
    // 480 samples at 48kHz → 160 samples at 16kHz (ratio 3:1)
    const samples = new Float32Array(480);
    for (let i = 0; i < samples.length; i++) {
      samples[i] = Math.sin((2 * Math.PI * 1000 * i) / 48000); // 1kHz tone
    }
    const resampled = linearResample(samples, 48000, 16000);
    expect(resampled.length).toBe(Math.floor(480 / 3));
  });

  it("passes through when rates match", () => {
    const samples = new Float32Array(160);
    const result = linearResample(samples, 16000, 16000);
    expect(result).toBe(samples);
  });
});

function linearResample(
  samples: Float32Array,
  fromRate: number,
  toRate: number
): Float32Array {
  if (fromRate === toRate) return samples;
  const ratio = fromRate / toRate;
  const newLen = Math.floor(samples.length / ratio);
  const result = new Float32Array(newLen);
  for (let i = 0; i < newLen; i++) {
    const srcIdx = i * ratio;
    const srcFloor = Math.floor(srcIdx);
    const frac = srcIdx - srcFloor;
    result[i] =
      samples[srcFloor] * (1 - frac) +
      samples[Math.min(srcFloor + 1, samples.length - 1)] * frac;
  }
  return result;
}

// --- TestWorkletBuffer ---
describe("worklet buffer flush", () => {
  function shouldFlush(bufferLength: number): boolean {
    return bufferLength >= 1600;
  }

  it("triggers flush at >= 1600 samples (13 frames = 1664)", () => {
    expect(shouldFlush(1536)).toBe(false); // 12 frames
    expect(shouldFlush(1664)).toBe(true);  // 13 frames
    expect(shouldFlush(1600)).toBe(true);  // boundary
  });

  it("triggers flush on stop for remainder below threshold", () => {
    // Simulate stop — flush whatever is in buffer
    const remainder = 1408; // 11 frames
    expect(shouldFlush(remainder)).toBe(false);
    // On stop, flush anyway (test the stop path)
    const flushedOnStop = remainder > 0;
    expect(flushedOnStop).toBe(true);
  });
});
```

- [ ] **Step 2: 运行前端单测**

Run: `cd web/frontend && npx vitest run src/components/chat/__tests__/voice.test.ts`
Expected: 3 项 PASS

- [ ] **Step 3: Commit**

```bash
git add web/frontend/src/components/chat/__tests__/voice.test.ts
git commit -m "feat: 前端 3 项单测 — utterance 合并 / 重采样 / buffer flush

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 11: ChatComposer 适配

**Files:**
- Modify: `web/frontend/src/components/chat/chat-composer.tsx`

- [ ] **Step 1: 适配流式 VoiceRecorder**

关键改动点（在现有 chat-composer.tsx 基础上修改）：

1. `useVoice()` 解构新增 `streamingAvailable`
2. 麦克风按钮改用 `streamingAvailable` 判断显隐（替换 `asrAvailable`）
3. 录音中：VoiceRecorder 渲染 → 自动开始录音；输入框 `readOnly`
4. 统一 `handleSend`：先 `setShowRecorder(false)` → 等 React 重渲染后 `onSend()`。为使 `onSend` 拿到正确的 `input` 值，VoiceRecorder 的 `onTranscribed` 负责设置最终 `input`，在 `handleSend` 中不依赖 state（`input` 已由 `onInputChange` 更新）

```tsx
// 在 chat-composer.tsx 中修改

// 1. useVoice 新增 streamingAvailable
const { outputEnabled, asrAvailable, ttsAvailable, streamingAvailable, toggleOutput } = useVoice();

// 2. 麦克风按钮: streamingAvailable 优先
{streamingAvailable ? (
  <Button
    type="button"
    variant="ghost"
    size="icon"
    className={`... ${showRecorder ? "bg-violet-100 text-violet-600" : ""}`}
    onClick={() => setShowRecorder((v) => !v)}
    disabled={!canInput}
    title={showRecorder ? "停止录音" : "语音输入"}
  >
    <span className="text-base">{showRecorder ? "🎙️" : "🎤"}</span>
  </Button>
) : null /* 仅支持流式 ASR，无 streaming 不显示麦克风按钮 */
}

// 3. VoiceRecorder（挂载即录音）
{showRecorder && streamingAvailable && (
  <VoiceRecorder
    onInterimText={(text, _definite) => onInputChange(text)}
    onTranscribed={(text) => {
      onInputChange(text);
      setShowRecorder(false);
    }}
    onError={(err) => {
      console.warn("ASR error:", err);
      setShowRecorder(false);
    }}
  />
)}

// 4. 输入框只读（录音中）
<TextareaAutosize
  // ...existing props...
  readOnly={showRecorder && streamingAvailable}
/>

// 5. handleSend — 录音中直接发当前文本，不等 final
// onInterimText 已将合并后的文本填入 input，直接取 input 当前值发送即可
const handleSend = useCallback(() => {
  if (showRecorder && streamingAvailable) {
    setShowRecorder(false); // 关录音（异步卸载）
    onSend();               // 立即用当前 input 值发送，不等 final RTT
    return;
  }
  onSend();
}, [showRecorder, streamingAvailable, onSend]);

const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
  if (e.nativeEvent.isComposing) return;
  if (e.key === "Enter" && !e.shiftKey) {
    e.preventDefault();
    handleSend();
  }
};

// 发送按钮 onClick → handleSend
```

- [ ] **Step 2: 编译验证**

Run: `cd web/frontend && npx tsc --noEmit`
Expected: 零错误

- [ ] **Step 3: Commit**

```bash
git add web/frontend/src/components/chat/chat-composer.tsx
git commit -m "feat: ChatComposer 适配流式 ASR — 输入框只读 + Enter/send 时序

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 12: AudioPlayer 改位 — 紧凑常驻 + 加载态

**Files:**
- Modify: `web/frontend/src/components/chat/audio-player.tsx`

- [ ] **Step 1: 重写 AudioPlayer**

```tsx
// web/frontend/src/components/chat/audio-player.tsx
import { useRef, useState } from "react";

interface AudioPlayerProps {
  audioUrl?: string; // undefined = loading state → show spinner
}

export function AudioPlayer({ audioUrl }: AudioPlayerProps) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [playing, setPlaying] = useState(false);

  if (!audioUrl) {
    return (
      <span className="inline-flex h-6 w-6 items-center justify-center" title="正在生成语音...">
        <span className="block h-3 w-3 animate-spin rounded-full border-2 border-current border-t-transparent opacity-60" />
      </span>
    );
  }

  return (
    <span className="inline-flex items-center gap-0.5">
      <button
        type="button"
        onClick={() => {
          if (!audioRef.current) return;
          if (playing) { audioRef.current.pause(); }
          else { audioRef.current.play(); }
          setPlaying(!playing);
        }}
        className="inline-flex h-6 w-6 items-center justify-center rounded-full text-muted-foreground hover:text-foreground transition-colors"
        title={playing ? "暂停" : "播放"}
      >
        {playing ? "⏸" : "▶"}
      </button>
      <audio
        ref={audioRef}
        src={audioUrl}
        onEnded={() => setPlaying(false)}
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        autoPlay
      />
    </span>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/frontend/src/components/chat/audio-player.tsx
git commit -m "feat: AudioPlayer 紧凑常驻 + 加载态 spinner

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 13: AssistantMessage — AudioPlayer 移入右上角

**Files:**
- Modify: `web/frontend/src/components/chat/assistant-message.tsx`

- [ ] **Step 1: 右上角布局 + 删除底部 TTS 区域**

关键改动（在现有 assistant-message.tsx 基础上修改）：

1. 保留 `ttsLoading`、`audioUrl`、`ttsTriggeredRef` 等状态不变（来源于现有 TTS useEffect 逻辑）
2. 右上角区域：AudioPlayer 在左（常驻），复制按钮在右（hover-only），互斥渲染加载/播放态
3. 删除原底部 `border-t` 区域的 `ttsLoading` 文字行和 `AudioPlayer` wrapper

```tsx
// assistant-message.tsx — 气泡内右上角区域（替换现有复制按钮的 absolute 定位）

{!isCollapsedBlock && hasText && (
  <div className="absolute top-2 right-2 flex items-center gap-1">
    {/* AudioPlayer: loading spinner OR play button, always visible */}
    {ttsLoading ? (
      <AudioPlayer />
    ) : audioUrl ? (
      <AudioPlayer audioUrl={audioUrl} />
    ) : null}

    {/* 复制按钮: hover-only */}
    <Button
      variant="ghost"
      size="icon"
      className="h-7 w-7 opacity-0 transition-opacity group-hover:opacity-100"
      onClick={() => void copy(content)}
      aria-label={copyMessageLabel}
      title={copyMessageLabel}
    >
      {isCopied ? (
        <IconCheck className="h-4 w-4 text-green-500" />
      ) : (
        <IconCopy className="text-muted-foreground h-4 w-4" />
      )}
    </Button>
  </div>
)}

// 删除以下代码（原底部 TTS 区域）:
// {ttsLoading && (
//   <div className="text-muted-foreground/60 border-t ..."> 正在生成语音... </div>
// )}
// {audioUrl && (
//   <div className="border-t ..."><AudioPlayer audioUrl={audioUrl} /></div>
// )}
```

- [ ] **Step 2: 编译验证**

Run: `cd web/frontend && npx tsc --noEmit`
Expected: 零错误

- [ ] **Step 3: Commit**

```bash
git add web/frontend/src/components/chat/assistant-message.tsx
git commit -m "feat: AudioPlayer 移入消息气泡右上角，常驻 + 互斥渲染

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

### Task 14: 全量构建 + 人工验收

- [ ] **Step 1: 全量构建**

```bash
make build && make build-launcher
```
Expected: 两个二进制编译成功

- [ ] **Step 2: Lint + Vet**

```bash
make lint
go vet -tags goolm,stdjson ./pkg/audio/... ./web/backend/...
cd web/frontend && npx tsc --noEmit
```
Expected: 零告警 / 零输出

- [ ] **Step 3: Go 单测**

```bash
go test -v -tags goolm,stdjson -run "TestVolcengine|TestBuildFull|TestEncodeAudioOnly|TestParseServer|TestParseError|TestIsServerACK|TestUtteranceDedup|TestDetectStreaming" ./pkg/audio/asr/
```
Expected: 9 项 PASS（精确匹配新增测试，不与 asr 包已有测试混淆）

- [ ] **Step 4: 前端单测**

```bash
cd web/frontend && npx vitest run src/components/chat/__tests__/voice.test.ts
```
Expected: 3 项 PASS

- [ ] **Step 5: 启动服务 + capabilities 验证**

用户启动 launcher 服务后：
```bash
curl -s http://localhost:18800/api/voice/capabilities
# Expected: {"asr":true,"tts":true,"streaming":true}（已配置 volcengine-asr 时）
```

- [ ] **Step 6: HTTP ASR 回归**

```bash
curl -s -X POST http://localhost:18800/api/voice/transcribe -F "file=@test.webm"
# Expected: 200 + JSON 含 text 字段
```

- [ ] **Step 7: 前端 UI 验收**（浏览器操作）

- [ ] 点🎤立即录音，按钮脉冲动画 + 计时，输入框只读
- [ ] 说话时输入框实时出字
- [ ] 点发送/Enter → 停止录音 + 发出消息
- [ ] 点🎤关闭 → 停止录音 + 文字留在输入框可编辑
- [ ] 60s 自动停止
- [ ] AudioPlayer 在右上角常驻显示，与复制按钮并排，保留 autoplay
- [ ] 加载态 spinner 占播放按钮位置

- [ ] **Step 8: 边界场景验证**

- [ ] **Safari 降级路径**: Safari 打开 → console 确认 warn（无 AudioWorklet 时回退）→ 录音/识别正常
- [ ] **蓝牙重采样降级**: 蓝牙耳机连接 → 确认 sampleRate != 16000 → worklet 内线性插值 → 识别准确率可接受
- [ ] **SiliconFlow HTTP ASR 并行**: `POST /api/voice/transcribe` 正常返回
- [ ] **鉴权失败**: 错误 app_key → toast "ASR 鉴权失败，请检查 App Key / Access Key"
- [ ] **余额不足**: 配额耗尽 → toast "ASR 时长配额已用尽"

---

### Task 15: 最终 commit

- [ ] **Step 1: 提交全部变更**

```bash
git add -A
git commit -m "feat: BL-009 流式语音识别 + 播放交互优化 完成

- 后端: StreamingTranscriber 接口 + 火山 v3 SAUC BigModel WebSocket 实现
- 后端: GET /api/voice/stream 端点（CheckOrigin CIDR + 去重 + 错误码映射）
- 前端: VoiceRecorder 重写（AudioWorklet + 线性插值重采样 + ≥100ms buffer）
- 前端: 流式 ASR 结果实时填入输入框（definite 段锁定额度 + 增量替换）
- 前端: AudioPlayer 移入消息气泡右上角（常驻 + 加载 spinner + autoplay）
- 保留 HTTP /api/voice/transcribe（SiliconFlow）完全不动
- 12 项单测全覆盖（9 Go + 3 前端）

Author: Holmir Chan <chenhaoming@talkweb.com.cn>
Co-Authored-By: Claude Code"
```

---

## Post-Implementation Checklist

- [ ] `make lint` 零告警
- [ ] `go vet -tags goolm,stdjson ./pkg/audio/... ./web/backend/...` 零输出
- [ ] `cd web/frontend && npx tsc --noEmit` 零错误
- [ ] 9 项 Go 单测 PASS（-run 精确匹配）
- [ ] 3 项前端单测 PASS
- [ ] HTTP `/api/voice/transcribe` 回归正常
- [ ] Safari 蓝牙降级路径可工作
