# Web 语音交互支持 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Web UI 新增独立切换的语音输入（ASR）和语音输出（TTS）能力

**Architecture:** 前端主动调 REST API，ASR/TTS 在 web/backend 做 HTTP 端点，复用 `pkg/audio/asr` 和 `pkg/audio/tts` 现有 Provider，不动 WebSocket/gateway

**Tech Stack:** Go (web/backend), React/TypeScript (前端), MediaRecorder API, `<audio>` 元素

---

## 文件结构

```
web/backend/api/
  voice.go          ← 新建：4 个语音端点 handler
  router.go         ← 修改：注册 /api/voice/ 路由

web/frontend/src/
  api/voice.ts      ← 新建：语音 API 客户端
  store/voice.ts    ← 新建：voiceInput/voiceOutput 状态 atom
  hooks/use-voice.ts← 新建：能力探测 + 状态管理
  components/chat/
    voice-recorder.tsx  ← 新建：录音按钮 + MediaRecorder
    audio-player.tsx    ← 新建：<audio> 播放条
    chat-composer.tsx   ← 修改：集成麦克风/喇叭按钮
    assistant-message.tsx ← 修改：TTS 异步加载 + 音频追加
```

---

### Task 1: voice.go — 语音端点 handler

**Files:**
- Create: `web/backend/api/voice.go`

- [ ] **Step 1: 写 voice.go 骨架和 handleVoiceCapabilities**

```go
package api

import (
    "encoding/json"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "time"

    "github.com/sipeed/picoclaw/pkg/audio/asr"
    "github.com/sipeed/picoclaw/pkg/audio/tts"
    "github.com/sipeed/picoclaw/pkg/config"
)

func (h *Handler) handleVoiceCapabilities(w http.ResponseWriter, r *http.Request) {
    cfg, err := config.LoadConfig(h.configPath)
    if err != nil {
        writeJSON(w, http.StatusOK, map[string]bool{"asr": false, "tts": false})
        return
    }
    writeJSON(w, http.StatusOK, map[string]bool{
        "asr": asr.DetectTranscriber(cfg) != nil,
        "tts": tts.DetectTTS(cfg) != nil,
    })
}

func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 2: 实现 handleVoiceTranscribe**

```go
func (h *Handler) handleVoiceTranscribe(w http.ResponseWriter, r *http.Request) {
    // 限制请求体 10MB
    r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

    if err := r.ParseMultipartForm(10 << 20); err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "文件过大或解析失败"})
        return
    }

    file, _, err := r.FormFile("file")
    if err != nil {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少音频文件"})
        return
    }
    defer file.Close()

    tmp, err := os.CreateTemp("", "voice-asr-*.webm")
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务器错误"})
        return
    }
    defer os.Remove(tmp.Name())
    defer tmp.Close()

    if _, err := io.Copy(tmp, file); err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存文件失败"})
        return
    }
    tmp.Close()

    cfg, err := config.LoadConfig(h.configPath)
    if err != nil {
        writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "未配置语音识别"})
        return
    }

    trans := asr.DetectTranscriber(cfg)
    if trans == nil {
        writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "未配置语音识别"})
        return
    }

    result, err := trans.Transcribe(r.Context(), tmp.Name())
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "语音识别失败: " + err.Error()})
        return
    }

    if result.Text == "" {
        writeJSON(w, http.StatusOK, map[string]string{"text": "", "error": "未识别到语音"})
        return
    }

    writeJSON(w, http.StatusOK, map[string]string{"text": result.Text, "language": result.Language})
}
```

- [ ] **Step 3: 实现 handleVoiceSynthesize**

```go
func (h *Handler) handleVoiceSynthesize(w http.ResponseWriter, r *http.Request) {
    var body struct {
        Text string `json:"text"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 text 参数"})
        return
    }

    cfg, err := config.LoadConfig(h.configPath)
    if err != nil || tts.DetectTTS(cfg) == nil {
        writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "未配置 TTS"})
        return
    }

    provider := tts.DetectTTS(cfg)

    cacheDir := filepath.Join(config.GetHome(), "tts-cache")
    os.MkdirAll(cacheDir, 0700)

    // 请求开始时清理旧文件
    cleanTTSCache(cacheDir, time.Hour)

    stream, err := provider.Synthesize(r.Context(), body.Text)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "TTS 合成失败"})
        return
    }
    defer stream.Close()

    ext := ".ogg"
    if provider.Name() == "mimo-tts" {
        ext = ".mp3"
    }
    tmp, err := os.CreateTemp(cacheDir, "tts-*"+ext)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "创建缓存文件失败"})
        return
    }
    defer tmp.Close()

    if _, err := io.Copy(tmp, stream); err != nil {
        os.Remove(tmp.Name())
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "写入音频失败"})
        return
    }

    writeJSON(w, http.StatusOK, map[string]string{
        "audio_url": "/api/voice/audio/" + filepath.Base(tmp.Name()),
    })
}

func cleanTTSCache(dir string, maxAge time.Duration) {
    entries, err := os.ReadDir(dir)
    if err != nil {
        return
    }
    cutoff := time.Now().Add(-maxAge)
    for _, e := range entries {
        info, err := e.Info()
        if err != nil {
            continue
        }
        if info.ModTime().Before(cutoff) {
            os.Remove(filepath.Join(dir, e.Name()))
        }
    }
}
```

- [ ] **Step 4: 实现 handleVoiceAudio**

```go
func (h *Handler) handleVoiceAudio(w http.ResponseWriter, r *http.Request) {
    fileID := filepath.Base(r.PathValue("file_id"))
    if fileID == "." || fileID == ".." || fileID == "" {
        http.Error(w, "invalid file", http.StatusBadRequest)
        return
    }

    cacheDir := filepath.Join(config.GetHome(), "tts-cache")
    http.ServeFile(w, r, filepath.Join(cacheDir, fileID))
}
```

- [ ] **Step 5: 编译验证**

Run: `go build -tags goolm,stdjson ./web/backend/...`

- [ ] **Step 6: 提交**

---

### Task 2: 注册语音路由

**Files:**
- Modify: `web/backend/api/router.go`

- [ ] **Step 1: 在 RegisterRoutes 中添加注册**

```go
// 在 RegisterRoutes 方法末尾（Shutdown 之前）添加：
h.registerVoiceRoutes(mux)
```

- [ ] **Step 2: 新增 registerVoiceRoutes 方法**

```go
func (h *Handler) registerVoiceRoutes(mux *http.ServeMux) {
    mux.HandleFunc("GET /api/voice/capabilities", h.handleVoiceCapabilities)
    mux.HandleFunc("POST /api/voice/transcribe", h.handleVoiceTranscribe)
    mux.HandleFunc("POST /api/voice/synthesize", h.handleVoiceSynthesize)
    mux.HandleFunc("GET /api/voice/audio/{file_id}", h.handleVoiceAudio)
}
```

- [ ] **Step 3: 确认 auth middleware 覆盖**

`/api/voice/` 不是 public path（不在 `isPublicLauncherDashboardPath` 中），自动经过 `LauncherDashboardAuth` 鉴权。

- [ ] **Step 4: 编译验证**

Run: `go build -tags goolm,stdjson ./web/backend/...`

- [ ] **Step 5: 提交**

---

### Task 3: 前端 voice API 客户端

**Files:**
- Create: `web/frontend/src/api/voice.ts`

- [ ] **Step 1: 写 voice.ts**

```ts
import { apiFetch } from "./http";

export async function fetchVoiceCapabilities(): Promise<{ asr: boolean; tts: boolean }> {
  const res = await apiFetch("/api/voice/capabilities");
  return res.json();
}

export async function transcribeAudio(blob: Blob): Promise<{ text: string; language?: string; error?: string }> {
  const form = new FormData();
  form.append("file", blob, "recording.webm");

  const res = await apiFetch("/api/voice/transcribe", {
    method: "POST",
    body: form,
    signal: AbortSignal.timeout(30_000),
  });

  return res.json();
}

export async function synthesizeSpeech(text: string): Promise<{ audio_url: string }> {
  const res = await apiFetch("/api/voice/synthesize", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ text }),
    signal: AbortSignal.timeout(60_000),
  });

  if (!res.ok) {
    throw new Error(`TTS failed: ${res.status}`);
  }

  return res.json();
}
```

- [ ] **Step 2: 确认 apiFetch 已存在并可用**

Check: `web/frontend/src/api/http.ts` 中 `apiFetch` 是否有 `timeout` / `signal` 支持，如没有则用 `fetch` + 手动拼 base URL。

- [ ] **Step 3: 提交**

---

### Task 4: voice Jotai store + useVoice hook

**Files:**
- Create: `web/frontend/src/store/voice.ts`
- Create: `web/frontend/src/hooks/use-voice.ts`

- [ ] **Step 1: 写 voice store**

```ts
// web/frontend/src/store/voice.ts
import { atom } from "jotai";

export const voiceInputAtom = atom(false);
export const voiceOutputAtom = atom(false);
export const voiceAsrAvailableAtom = atom(false);
export const voiceTtsAvailableAtom = atom(false);
```

- [ ] **Step 2: 写 useVoice hook**

```ts
// web/frontend/src/hooks/use-voice.ts
import { useAtom, useSetAtom } from "jotai";
import { useEffect, useCallback } from "react";
import { fetchVoiceCapabilities } from "@/api/voice";
import {
  voiceInputAtom,
  voiceOutputAtom,
  voiceAsrAvailableAtom,
  voiceTtsAvailableAtom,
} from "@/store/voice";

export function useVoice() {
  const [inputEnabled, setInputEnabled] = useAtom(voiceInputAtom);
  const [outputEnabled, setOutputEnabled] = useAtom(voiceOutputAtom);
  const [asrAvailable, setAsrAvailable] = useAtom(voiceAsrAvailableAtom);
  const [ttsAvailable, setTtsAvailable] = useAtom(voiceTtsAvailableAtom);

  useEffect(() => {
    fetchVoiceCapabilities().then((caps) => {
      setAsrAvailable(caps.asr);
      setTtsAvailable(caps.tts);
    }).catch(() => {});
  }, []);

  const toggleInput = useCallback(() => setInputEnabled((v) => !v), []);
  const toggleOutput = useCallback(() => setOutputEnabled((v) => !v), []);

  return { inputEnabled, outputEnabled, asrAvailable, ttsAvailable, toggleInput, toggleOutput };
}
```

- [ ] **Step 3: 提交**

---

### Task 5: VoiceRecorder 组件

**Files:**
- Create: `web/frontend/src/components/chat/voice-recorder.tsx`

- [ ] **Step 1: 写 VoiceRecorder 组件**

```tsx
// web/frontend/src/components/chat/voice-recorder.tsx
import { useState, useRef, useCallback } from "react";
import { transcribeAudio } from "@/api/voice";

interface VoiceRecorderProps {
  onTranscribed: (text: string) => void;
}

export function VoiceRecorder({ onTranscribed }: VoiceRecorderProps) {
  const [recording, setRecording] = useState(false);
  const [loading, setLoading] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const isSupported = typeof MediaRecorder !== "undefined";

  const startRecording = useCallback(async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      // Safari 不支持 audio/webm，降级 audio/mp4
      const mimeType = MediaRecorder.isTypeSupported("audio/webm")
        ? "audio/webm"
        : "audio/mp4";
      const blobType = mimeType;
      const recorder = new MediaRecorder(stream, { mimeType });
      mediaRecorderRef.current = recorder;
      chunksRef.current = [];

      recorder.ondataavailable = (e) => {
        if (e.data.size > 0) chunksRef.current.push(e.data);
      };

      recorder.onstop = async () => {
        stream.getTracks().forEach((t) => t.stop());
        setLoading(true);
        try {
          const blob = new Blob(chunksRef.current, { type: blobType });
          const result = await transcribeAudio(blob);
          if (result.text) {
            onTranscribed(result.text);
          }
        } finally {
          setLoading(false);
          setRecording(false);
          setElapsed(0);
        }
      };

      recorder.start();
      setRecording(true);
      setElapsed(0);
      timerRef.current = setInterval(() => {
        setElapsed((prev) => {
          if (prev >= 59) {
            recorder.stop();
            if (timerRef.current) clearInterval(timerRef.current);
            return prev;
          }
          return prev + 1;
        });
      }, 1000);

      // 60 秒自动停止
      const timeoutId = setTimeout(() => {
        if (recorder.state === "recording") {
          recorder.stop();
          if (timerRef.current) clearInterval(timerRef.current);
        }
      }, 60_000);
      // 正常停止时清除 timeout
      const origOnStop = recorder.onstop;
      recorder.onstop = (e) => {
        clearTimeout(timeoutId);
        origOnStop?.call(recorder, e);
      };
    } catch {
      // 用户拒绝麦克风权限
    }
  }, [onTranscribed]);

  const stopRecording = useCallback(() => {
    mediaRecorderRef.current?.stop();
    if (timerRef.current) clearInterval(timerRef.current);
  }, []);

  if (!isSupported) return null;

  return (
    <button
      type="button"
      onClick={recording ? stopRecording : startRecording}
      disabled={loading}
      className={`voice-mic-btn ${recording ? "recording" : ""} ${loading ? "loading" : ""}`}
      title={recording ? "停止录音" : "语音输入"}
    >
      {loading ? "..." : recording ? `${elapsed}s` : "🎤"}
    </button>
  );
}
```

- [ ] **Step 2: 编译验证**

Run: `cd web/frontend && npx tsc --noEmit`

- [ ] **Step 3: 提交**

---

### Task 6: 聊天输入框集成（麦克风+喇叭按钮）

**Files:**
- Modify: `web/frontend/src/components/chat/chat-composer.tsx`

- [ ] **Step 1: 验证 onSend 取值方式**

Read `chat-composer.tsx` 中 `onSend` 的实现（在父组件 `chat-page.tsx` 传入）。确认 `onSend` 是直接读 `input` prop 还是从 DOM/ref 取值：
- 若从 `input` prop 读 → `onInputChange(text)` + `onSend()` 顺序调用安全
- 若从 DOM/ref 读 → 需要先 `onInputChange(text)` 再 `setTimeout(() => onSend(), 0)` 等 React commit

- [ ] **Step 2: 在 composer 中集成 voice 按钮**

在输入框左侧（发送按钮旁边）新增两个按钮：

```tsx
// 新增 import
import { useVoice } from "@/hooks/use-voice";
import { VoiceRecorder } from "./voice-recorder";

// 在组件内
const { outputEnabled, asrAvailable, ttsAvailable, toggleOutput } = useVoice();
const [showRecorder, setShowRecorder] = useState(false);

// 在输入框 toolbar 区域添加：
{asrAvailable && (
  <button
    type="button"
    onClick={() => setShowRecorder((v) => !v)}
    className={`composer-btn ${showRecorder ? "active" : ""}`}
    title={showRecorder ? "关闭语音输入" : "语音输入"}
  >
    {showRecorder ? "🎙️" : "🎤"}
  </button>
)}
{ttsAvailable && (
  <button
    type="button"
    onClick={toggleOutput}
    className={`composer-btn ${outputEnabled ? "active" : ""}`}
    title={outputEnabled ? "关闭语音朗读" : "开启语音朗读"}
  >
    {outputEnabled ? "🔊" : "🔈"}
  </button>
)}

// 在 composer 上方条件渲染录音器：
{showRecorder && asrAvailable && (
  <VoiceRecorder
    onTranscribed={(text) => {
      onInputChange(text);
      setShowRecorder(false);
      onSend();
    }}
  />
)}
```

- [ ] **Step 3: 编译验证**

Run: `cd web/frontend && npx tsc --noEmit`

- [ ] **Step 4: 提交**

---

### Task 7: AudioPlayer + assistant-message TTS 集成

**Files:**
- Create: `web/frontend/src/components/chat/audio-player.tsx`
- Modify: `web/frontend/src/components/chat/assistant-message.tsx`
- Modify: `web/frontend/src/components/chat/chat-page.tsx`

- [ ] **Step 1: 写 AudioPlayer 组件**

```tsx
// web/frontend/src/components/chat/audio-player.tsx
import { useRef, useState } from "react";

interface AudioPlayerProps {
  audioUrl: string;
}

export function AudioPlayer({ audioUrl }: AudioPlayerProps) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [playing, setPlaying] = useState(false);

  return (
    <div className="audio-player">
      <button
        type="button"
        onClick={() => {
          if (!audioRef.current) return;
          if (playing) {
            audioRef.current.pause();
          } else {
            audioRef.current.play();
          }
          setPlaying(!playing);
        }}
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
    </div>
  );
}
```

- [ ] **Step 2a: 给 AssistantMessage 加 isComplete prop**

```tsx
// 修改 interface AssistantMessageProps，新增：
isComplete?: boolean;
```

- [ ] **Step 2b: 修改 assistant-message.tsx 的 TTS 逻辑**

流式消息的 `content` 会持续更新——必须在消息完成后才触发 TTS，否则会用不完整文本朗读。

```tsx
// 新增 import
import { useEffect, useRef, useState } from "react";
import { synthesizeSpeech } from "@/api/voice";
import { AudioPlayer } from "./audio-player";
import { useAtomValue } from "jotai";
import { voiceOutputAtom } from "@/store/voice";

// 在 assistant message 组件内（props 解构加上 isComplete）
const { content, isComplete, ... } = props;
const outputEnabled = useAtomValue(voiceOutputAtom);
const [ttsLoading, setTtsLoading] = useState(false);
const [audioUrl, setAudioUrl] = useState<string | null>(null);
const ttsTriggeredRef = useRef(false);

useEffect(() => {
  if (!isComplete || !content?.trim()) return;
  // 消息完成时立即标记，无论 TTS 当前是否开启，避免切换喇叭后历史消息同时触发
  if (ttsTriggeredRef.current) return;
  ttsTriggeredRef.current = true;

  if (!outputEnabled) return;

  let cancelled = false;

  setTtsLoading(true);
  synthesizeSpeech(content)
    .then((res) => {
      if (!cancelled) setAudioUrl(res.audio_url);
    })
    .catch(() => {})
    .finally(() => {
      if (!cancelled) setTtsLoading(false);
    });

  return () => { cancelled = true; };
}, [outputEnabled, isComplete, content]);

// 在气泡底部条件渲染：
{ttsLoading && <span className="tts-loading">🔈...</span>}
{audioUrl && <AudioPlayer audioUrl={audioUrl} />}
```

- [ ] **Step 2c: 在 chat-page.tsx 传递 isComplete**

```tsx
// chat-page.tsx 中找到 AssistantMessage 渲染处，追加 isComplete prop：
<AssistantMessage
  content={msg.content}
  attachments={msg.attachments}
  kind={msg.kind}
  toolCalls={msg.toolCalls}
  timestamp={msg.timestamp}
  isComplete={!isTyping}
/>
```

> **已知限制**：部分浏览器（Safari、Chrome 移动端）在没有用户交互时会静默屏蔽 `<audio autoPlay>`。受影响的场景是「用户首次开启喇叭后第一条消息不自动播放，需手动点击播放按钮」。可接受，AudioPlayer 组件已有播放按钮作为回退。

- [ ] **Step 3: 编译验证**

Run: `cd web/frontend && npx tsc --noEmit`

- [ ] **Step 4: 全量构建验证**

Run: `make build && make build-launcher`

- [ ] **Step 5: 提交**
