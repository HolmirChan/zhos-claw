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
