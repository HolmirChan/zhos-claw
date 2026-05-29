package api

import (
	"context"
	"encoding/json"
	"fmt"
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
		http.Error(w, fmt.Sprintf("Failed to parse form: %v", err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, fmt.Sprintf("Missing file field: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := filepath.Ext(header.Filename)
	tmpFile, err := os.CreateTemp("", "voice-upload-*"+ext)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create temp file: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())

	if _, err := io.Copy(tmpFile, file); err != nil {
		tmpFile.Close()
		http.Error(w, fmt.Sprintf("Failed to save upload: %v", err), http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	transcriber := asr.DetectTranscriber(cfg)
	if transcriber == nil {
		http.Error(w, "No ASR provider configured", http.StatusBadRequest)
		return
	}

	result, err := transcriber.Transcribe(context.Background(), tmpFile.Name())
	if err != nil {
		http.Error(w, fmt.Sprintf("Transcription failed: %v", err), http.StatusInternalServerError)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON body: %v", err), http.StatusBadRequest)
		return
	}
	if req.Text == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	provider := tts.DetectTTS(cfg)
	if provider == nil {
		http.Error(w, "No TTS provider configured", http.StatusBadRequest)
		return
	}

	cacheDir := filepath.Join(config.GetHome(), "tts-cache")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create cache dir: %v", err), http.StatusInternalServerError)
		return
	}

	cleanTTSCache(cacheDir, time.Hour)

	audioStream, err := provider.Synthesize(context.Background(), req.Text)
	if err != nil {
		http.Error(w, fmt.Sprintf("TTS synthesis failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer audioStream.Close()

	ext := ".ogg"
	if provider.Name() == "mimo-tts" {
		ext = ".mp3"
	}

	tmp, err := os.CreateTemp(cacheDir, "tts-*"+ext)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create temp file: %v", err), http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(tmp, audioStream); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		http.Error(w, fmt.Sprintf("Failed to write audio: %v", err), http.StatusInternalServerError)
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
		http.Error(w, "Invalid file ID", http.StatusBadRequest)
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
