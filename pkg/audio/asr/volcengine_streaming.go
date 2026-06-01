package asr

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const (
	volcengineMsgFullClientReq  = 0b0001
	volcengineMsgAudioOnlyReq   = 0b0010
	volcengineMsgFullServerResp = 0b1001
	volcengineMsgServerACK      = 0b1011
	volcengineMsgError          = 0b1111

	volcengineFlagNone        = 0b0000
	volcengineFlagPosSequence = 0b0001
	volcengineFlagNegWithSeq  = 0b0011

	volcengineSerialRaw  = 0b0000
	volcengineSerialJSON = 0b0001

	volcengineCompressNone = 0b0000
	volcengineCompressGzip = 0b0001
)

func encodeVolcengineHeader(msgType, flags, serialization, compression byte) [4]byte {
	return [4]byte{
		0b0001_0001, // version=1, header_size=1 (x4 = 4 bytes)
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

// encodeAudioOnlyFrame returns header, seqBytes (4B), payloadSize (4B), payload.
// Audio frames: [4B header][4B seq][4B payload_size][payload], Compression=0b0000 (no gzip for PCM).
func encodeAudioOnlyFrame(audioData []byte, seq int32, isLast bool) (header [4]byte, seqBytes []byte, payloadSize []byte, payload []byte) {
	var flags byte = volcengineFlagPosSequence
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
