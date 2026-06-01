package asr

import (
	"context"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
)

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

func TestBuildFullClientRequestPayload(t *testing.T) {
	payload := buildFullClientRequestPayload("test-uid")
	if payload["app"] != nil {
		t.Error("no app field expected in v3 payload")
	}
	audio := payload["audio"].(map[string]any)
	if audio["format"] != "pcm" {
		t.Errorf("audio.format = %v, want pcm", audio["format"])
	}
	if audio["rate"] != 16000 {
		t.Errorf("audio.rate = %v, want 16000", audio["rate"])
	}
	req := payload["request"].(map[string]any)
	if req["model_name"] != "bigmodel" {
		t.Errorf("request.model_name = %v, want bigmodel", req["model_name"])
	}
}

func TestEncodeAudioOnlyFrame(t *testing.T) {
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

	header2, seqBytes2, _, _ := encodeAudioOnlyFrame([]byte{0, 1, 2, 3}, 3, true)
	_, flags2, _, _ := decodeVolcengineHeader(header2)
	if flags2 != volcengineFlagLastNoSeq {
		t.Errorf("flags = %04b, want %04b (last, no seq)", flags2, volcengineFlagLastNoSeq)
	}
	if len(seqBytes2) != 0 {
		t.Error("last frame should have no seq bytes")
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

func TestUtteranceDedup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &volcengineStreamingSession{
		results: make(chan StreamingResult, 64),
		ctx:     ctx,
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

	// Frame 3: definite false->true + new utterance
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

	// Frame 4: no change -> skip
	s.dedupAndPush([]StreamingResult{
		{Text: "今天天气", Definite: true},
		{Text: "怎么样", Definite: false},
	})
	select {
	case r := <-s.results:
		t.Errorf("frame 4: should be no push, got %+v", r)
	default:
	}

	// Frame 5: definite true->false (safety skip, should not happen)
	s.dedupAndPush([]StreamingResult{{Text: "今天天气", Definite: false}})
	select {
	case r := <-s.results:
		t.Errorf("frame 5: true->false should be skipped, got %+v", r)
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

	// Case 2: streaming_model_name set but model not in model_list -> (nil, nil)
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
