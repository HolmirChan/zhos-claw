package asr

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
