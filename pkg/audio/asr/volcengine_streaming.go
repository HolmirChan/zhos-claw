package asr

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/sipeed/picoclaw/pkg/config"
)

const (
	volcengineMsgFullClientReq  = 0b0001
	volcengineMsgAudioOnlyReq   = 0b0010
	volcengineMsgFullServerResp = 0b1001
	volcengineMsgServerACK      = 0b1011
	volcengineMsgError          = 0b1111

	volcengineFlagNone        = 0b0000
	volcengineFlagPosSequence = 0b0001
	volcengineFlagLastNoSeq   = 0b0010 // 最后一包，无 seq
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

func buildFullClientRequestPayload(uid string) map[string]any {
	return map[string]any{
		"user": map[string]string{"uid": uid},
		"audio": map[string]any{
			"format":   "pcm",
			"rate":     16000,
			"bits":     16,
			"channel":  1,
			"language": "zh-CN",
		},
		"request": map[string]any{
			"model_name":     "bigmodel",
			"enable_itn":     true,
			"enable_punc":    true,
			"show_utterances": true,
		},
	}
}

func buildFullClientFrame(uid string) ([]byte, error) {
	payloadJSON, err := json.Marshal(buildFullClientRequestPayload(uid))
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

// encodeAudioOnlyFrame returns header, seqBytes (4B, only when !isLast), payloadSize (4B), payload.
// Regular frame: [4B header][4B seq][4B payload_size][payload]
// Last frame (flags=0b0010): [4B header][4B payload_size][payload] — no seq
// Compression=0b0000 (no gzip for PCM).
func encodeAudioOnlyFrame(audioData []byte, seq int32, isLast bool) (header [4]byte, seqBytes []byte, payloadSize []byte, payload []byte) {
	flags := byte(volcengineFlagPosSequence)
	if isLast {
		flags = volcengineFlagLastNoSeq
	}
	header = encodeVolcengineHeader(volcengineMsgAudioOnlyReq, flags, volcengineSerialRaw, volcengineCompressNone)
	if !isLast {
		seqBytes = make([]byte, 4)
		binary.BigEndian.PutUint32(seqBytes, uint32(seq))
	}
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

// volcengineStreamingTranscriber is the factory that holds model config.
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
	headers.Set("X-Api-Key", modelCfg.APIKey())
	if modelCfg.ResourceID == "" {
		modelCfg.ResourceID = "volc.bigasr.sauc.duration"
	}
	headers.Set("X-Api-Resource-Id", modelCfg.ResourceID)
	headers.Set("X-Api-Request-Id", newUUID())
	headers.Set("X-Api-Sequence", "-1")

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

	frame, err := buildFullClientFrame("web-user")
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
		// Last frame: flags=0b0010, no seq, empty payload
		header := encodeVolcengineHeader(volcengineMsgAudioOnlyReq, volcengineFlagLastNoSeq, volcengineSerialRaw, volcengineCompressNone)
		payloadSize := make([]byte, 4) // zero
		frame := append(header[:], payloadSize...)
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
		msgType, _, _, compression := decodeVolcengineHeader(header)
		body := msg[4:]

		switch msgType {
		case volcengineMsgServerACK:
			continue

		case volcengineMsgError:
			// Error frame: [4B header][4B error_code][4B error_size][error_msg]
			if len(body) < 8 {
				continue
			}
			errorCode := binary.BigEndian.Uint32(body[0:4])
			errorSize := binary.BigEndian.Uint32(body[4:8])
			if len(body) < 8+int(errorSize) {
				continue
			}
			errorMsg := string(body[8 : 8+errorSize])
			s.results <- StreamingResult{Error: volcengineErrorMessage(int(errorCode), errorMsg)}
			return

		case volcengineMsgFullServerResp:
			// Response frame: [4B header][4B sequence][4B payload_size][payload]
			// Sequence is always present for server responses
			if len(body) < 8 {
				continue
			}
			payloadSize := binary.BigEndian.Uint32(body[4:8])
			if len(body) < 8+int(payloadSize) {
				continue
			}
			payload := body[8 : 8+payloadSize]
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
				continue // safety: definite never goes true->false
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
		return "ASR 参数配置错误，请检查请求参数"
	case 40200002:
		return "ASR 鉴权失败，请检查 App Key"
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
