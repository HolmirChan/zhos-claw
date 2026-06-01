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
