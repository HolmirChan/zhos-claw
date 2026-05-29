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

const maxVoiceUploadSize = 10 << 20 // 10 MB

// handleVoiceCapabilities returns the ASR/TTS capabilities of the current config.
//
//	GET /api/voice/capabilities
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
		Text string `json:"text"`
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

	audioStream, err := provider.Synthesize(r.Context(), req.Text)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "TTS 合成失败"})
		return
	}
	defer audioStream.Close()

	ext := ".ogg"
	if provider.Name() == "mimo-tts" {
		ext = ".mp3"
	}

	tmp, err := os.CreateTemp(cacheDir, "tts-*"+ext)
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

	writeJSON(w, http.StatusOK, map[string]string{
		"audio_url": "/api/voice/audio/" + filepath.Base(tmp.Name()),
	})
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
