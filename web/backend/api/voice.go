package api

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sipeed/picoclaw/pkg/audio/asr"
	"github.com/sipeed/picoclaw/pkg/audio/tts"
	"github.com/sipeed/picoclaw/pkg/config"
)

const maxVoiceUploadSize = 10 << 20 // 10 MB

// handleVoiceCapabilities returns the ASR/TTS capabilities of the current config.
//
//	GET /api/voice/capabilities
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

// handleVoiceTranscribe transcribes an uploaded audio file using the configured ASR provider.
//
//	POST /api/voice/transcribe
func (h *Handler) handleVoiceTranscribe(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxVoiceUploadSize)

	if err := r.ParseMultipartForm(maxVoiceUploadSize); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "文件过大或解析失败"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少音频文件"})
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	tmpFile, err := os.CreateTemp("", "voice-upload-*"+ext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "服务器错误"})
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, copyErr := io.Copy(tmpFile, file); copyErr != nil {
		tmpFile.Close()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "保存文件失败"})
		return
	}
	tmpFile.Close()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "未配置语音识别"})
		return
	}

	transcriber := asr.DetectTranscriber(cfg)
	if transcriber == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "未配置语音识别"})
		return
	}

	result, err := transcriber.Transcribe(r.Context(), tmpFile.Name())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "语音识别失败: " + err.Error()})
		return
	}

	if result.Text == "" {
		writeJSON(w, http.StatusOK, map[string]string{"text": "", "error": "未识别到语音"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"text":     result.Text,
		"language": result.Language,
	})
}

// handleVoiceSynthesize synthesizes text to speech using the configured TTS provider.
//
//	POST /api/voice/synthesize
func (h *Handler) handleVoiceSynthesize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text         string `json:"text"`
		SessionID    string `json:"session_id,omitempty"`
		MessageIndex int    `json:"message_index,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 text 参数"})
		return
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "未配置 TTS"})
		return
	}

	provider := tts.DetectTTS(cfg)
	if provider == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "未配置 TTS"})
		return
	}

	cacheDir := filepath.Join(config.GetHome(), "tts-cache")
	if mkdirErr := os.MkdirAll(cacheDir, 0o700); mkdirErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "创建缓存目录失败"})
		return
	}

	cleanTTSCache(cacheDir, time.Hour)

	// Deterministic filename: tts-<md5(text)>.mp3
	hash := md5.Sum([]byte(req.Text))
	fileName := fmt.Sprintf("tts-%x.mp3", hash)
	filePath := filepath.Join(cacheDir, fileName)

	// If cached file exists, skip synthesis
	if _, statErr := os.Stat(filePath); statErr == nil {
		audioURL := "/api/voice/audio/" + fileName
		h.saveAudioURLIfNeeded(req.SessionID, req.MessageIndex, audioURL)
		writeJSON(w, http.StatusOK, map[string]string{"audio_url": audioURL})
		return
	}

	audioStream, err := provider.Synthesize(r.Context(), req.Text)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "TTS 合成失败"})
		return
	}
	defer audioStream.Close()

	tmp, err := os.CreateTemp(cacheDir, "tts-*.mp3")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "创建缓存文件失败"})
		return
	}

	if _, err := io.Copy(tmp, audioStream); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "写入音频失败"})
		return
	}
	tmp.Close()

	// Rename to deterministic filename
	if err := os.Rename(tmp.Name(), filePath); err != nil {
		// Fallback: keep temp name
		fileName = filepath.Base(tmp.Name())
	}

	audioURL := "/api/voice/audio/" + fileName
	h.saveAudioURLIfNeeded(req.SessionID, req.MessageIndex, audioURL)

	writeJSON(w, http.StatusOK, map[string]string{"audio_url": audioURL})
}

// handleVoiceAudio serves a cached TTS audio file.
//
//	GET /api/voice/audio/{file_id}
func (h *Handler) handleVoiceAudio(w http.ResponseWriter, r *http.Request) {
	fileID := filepath.Base(r.PathValue("file_id"))
	if fileID == "" || fileID == "." || fileID == ".." {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的文件 ID"})
		return
	}

	audioPath := filepath.Join(config.GetHome(), "tts-cache", fileID)
	http.ServeFile(w, r, audioPath)
}

// writeJSON writes the given value as a JSON response with the specified status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// cleanTTSCache removes TTS cache files older than maxAge.
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

// cidrMatch reports whether remoteAddr falls within any of the given CIDR networks.
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

var voiceStreamUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// handleVoiceStream handles WebSocket streaming ASR.
//
//	GET /api/voice/stream
func (h *Handler) handleVoiceStream(w http.ResponseWriter, r *http.Request) {
	// TODO: 若前端经 nginx/反代访问，remoteAddr 为 127.0.0.1，
	// 需改为读取 X-Forwarded-For 或 X-Real-IP 头（从 r.Header 取）。
	voiceStreamUpgrader.CheckOrigin = func(r *http.Request) bool {
		if !h.serverPublic {
			return true
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

	// Goroutine 1: read audio chunks from frontend -> send to volcengine
	go func() {
		defer wg.Done()
		defer session.Close()
		for {
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.CloseMessage {
				return
			}
			if err := session.SendAudio(msg); err != nil {
				return
			}
		}
	}()

	// Goroutine 2: read results from volcengine -> send to frontend
	// Per-slot accumulation using result.Index (from dedup):
	// - definite=true  -> locked += text, current[Index] = ""
	// - definite=false -> current[Index] = text
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

// saveAudioURLIfNeeded persists the audio_url mapping for a session message.
func (h *Handler) saveAudioURLIfNeeded(sessionID string, msgIndex int, audioURL string) {
	if sessionID == "" || audioURL == "" {
		return
	}
	dir, err := h.sessionsDir()
	if err != nil {
		return
	}
	ref, err := h.findPicoJSONLSession(dir, sessionID)
	if err != nil {
		return
	}
	_ = saveSessionAudioURL(dir, ref.Key, msgIndex, audioURL)
}
