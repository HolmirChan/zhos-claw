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

	header2, _, _, _ := encodeAudioOnlyFrame([]byte{0, 1, 2, 3}, 3, true)
	_, flags2, _, _ := decodeVolcengineHeader(header2)
	if flags2 != volcengineFlagNegWithSeq {
		t.Errorf("flags = %04b, want %04b (neg)", flags2, volcengineFlagNegWithSeq)
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
