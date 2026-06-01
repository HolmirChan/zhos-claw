package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type volcengineTTSProvider struct {
	apiKey     string
	apiBase    string
	resourceID string
	speaker    string
	format     string
	client     *http.Client
}

func NewVolcengineTTSProvider(mc *config.ModelConfig, format, voice string) *volcengineTTSProvider {
	if mc.ResourceID == "" {
		mc.ResourceID = "seed-tts-2.0"
	}
	return &volcengineTTSProvider{
		apiKey:     mc.APIKey(),
		apiBase:    strings.TrimSuffix(providers.ResolveAPIBase(mc), "/"),
		resourceID: mc.ResourceID,
		speaker:    voice,
		format:     format,
		client:     &http.Client{},
	}
}

func (v *volcengineTTSProvider) Name() string { return "volcengine-tts" }

func (v *volcengineTTSProvider) Synthesize(ctx context.Context, text string) (io.ReadCloser, error) {
	format := v.format
	if format == "" {
		format = "mp3"
	}
	speaker := v.speaker
	if speaker == "" {
		speaker = "zh_female_shuangkuaisisi_moon_bigtts"
	}

	body := map[string]any{
		"user": map[string]string{"uid": "web-user"},
		"req_params": map[string]any{
			"text":    text,
			"speaker": speaker,
			"audio_params": map[string]any{
				"format":      format,
				"sample_rate": 24000,
			},
		},
	}
	payload, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", v.apiBase+"/unidirectional", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", v.apiKey)
	req.Header.Set("X-Api-Resource-Id", v.resourceID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("volcengine tts: %w", err)
	}

	pr, pw := io.Pipe()
	go func() {
		defer resp.Body.Close()
		defer pw.Close()

		decoder := json.NewDecoder(resp.Body)
		for {
			var chunk struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Data    string `json:"data"`
			}
			if err := decoder.Decode(&chunk); err != nil {
				if err != io.EOF {
					pw.CloseWithError(fmt.Errorf("volcengine tts decode: %w", err))
				}
				return
			}
			if chunk.Code == 20000000 {
				return
			}
			if chunk.Code != 0 {
				pw.CloseWithError(fmt.Errorf("volcengine tts: %d %s", chunk.Code, chunk.Message))
				return
			}
			if chunk.Data != "" {
				audio, err := base64.StdEncoding.DecodeString(chunk.Data)
				if err != nil {
					pw.CloseWithError(fmt.Errorf("volcengine tts base64: %w", err))
					return
				}
				pw.Write(audio)
			}
		}
	}()

	return pr, nil
}
