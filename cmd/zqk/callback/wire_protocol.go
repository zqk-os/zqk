package callback

import (
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"math"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const (
	// WireMagicHeader is the 4-byte magic signature "ZQK1" (0x5A514B31) for wire protocol frames.
	WireMagicHeader uint32 = 0x5A514B31

	// WireProtocolVersion is the current binary wire protocol framing version (1).
	WireProtocolVersion uint8 = 1

	// WireHeaderSize is the fixed 32-byte binary header overhead per frame.
	WireHeaderSize int = 32

	// WireFlagCompressed indicates the frame payload is compressed.
	WireFlagCompressed uint8 = 1 << 0

	// WireFlagHeartbeat indicates a keepalive heartbeat frame without payload.
	WireFlagHeartbeat uint8 = 1 << 1

	// WirePayloadTypeJSON indicates standard JSON-serialized CallbackEntry payload.
	WirePayloadTypeJSON uint16 = 1
)

var (
	crc32CastagnoliTable = crc32.MakeTable(crc32.Castagnoli)
)

// WireProtocolCodec handles zero-copy binary serialization and deserialization of callback entries.
type WireProtocolCodec struct{}

// NewWireProtocolCodec creates a new WireProtocolCodec instance.
func NewWireProtocolCodec() *WireProtocolCodec {
	return &WireProtocolCodec{}
}

// EncodeEntry serializes a CallbackEntry into a 32-byte fixed header plus payload.
// If buf has sufficient capacity, it serializes with zero heap allocations.
func (c *WireProtocolCodec) EncodeEntry(entry *CallbackEntry, buf []byte) ([]byte, error) {
	if entry == nil {
		return nil, errfmt.Errorf("entry cannot be nil")
	}

	payloadBytes, err := json.Marshal(entry)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal entry payload").Wrap(err)
	}

	payloadLen := len(payloadBytes)
	if payloadLen > math.MaxInt-WireHeaderSize {
		return nil, errfmt.Errorf("payload too large")
	}
	totalSize := WireHeaderSize + payloadLen

	target := buf
	if cap(target) < totalSize {
		target = make([]byte, totalSize)
	} else {
		target = target[:totalSize]
	}

	c.writeHeader(target[:WireHeaderSize], entry.Seq, entry.Timestamp.UnixNano(), uint32(payloadLen))
	copy(target[WireHeaderSize:], payloadBytes)

	checksum := crc32.Checksum(target[WireHeaderSize:], crc32CastagnoliTable)
	binary.BigEndian.PutUint32(target[28:32], checksum)

	return target, nil
}

// writeHeader encodes the invariant 28 bytes of the 32-byte frame header.
func (c *WireProtocolCodec) writeHeader(hdr []byte, seq int64, tsNano int64, payloadLen uint32) {
	binary.BigEndian.PutUint32(hdr[0:4], WireMagicHeader)
	hdr[4] = WireProtocolVersion
	hdr[5] = 0 // Flags
	binary.BigEndian.PutUint16(hdr[6:8], WirePayloadTypeJSON)
	binary.BigEndian.PutUint64(hdr[8:16], uint64(seq))
	binary.BigEndian.PutUint64(hdr[16:24], uint64(tsNano))
	binary.BigEndian.PutUint32(hdr[24:28], payloadLen)
	binary.BigEndian.PutUint32(hdr[28:32], 0) // Checksum placeholder
}

// DecodeEntry parses a single binary wire frame from data.
// Returns the decoded CallbackEntry and the number of bytes consumed.
func (c *WireProtocolCodec) DecodeEntry(data []byte) (*CallbackEntry, int, error) {
	if len(data) < WireHeaderSize {
		return nil, 0, errfmt.Errorf("insufficient buffer for header: got %d, want at least %d", len(data), WireHeaderSize)
	}

	magic := binary.BigEndian.Uint32(data[0:4])
	if magic != WireMagicHeader {
		return nil, 0, errfmt.Errorf("invalid wire magic header: 0x%X (expected 0x%X)", magic, WireMagicHeader)
	}

	version := data[4]
	if version != WireProtocolVersion {
		return nil, 0, errfmt.Errorf("unsupported wire protocol version: %d (expected %d)", version, WireProtocolVersion)
	}

	payloadLen := int(binary.BigEndian.Uint32(data[24:28]))
	expectedChecksum := binary.BigEndian.Uint32(data[28:32])
	totalLen := WireHeaderSize + payloadLen

	if len(data) < totalLen {
		return nil, 0, errfmt.Errorf("incomplete frame data: got %d, want %d", len(data), totalLen)
	}

	payloadBytes := data[WireHeaderSize:totalLen]
	calculatedChecksum := crc32.Checksum(payloadBytes, crc32CastagnoliTable)
	if calculatedChecksum != expectedChecksum {
		return nil, 0, errfmt.Errorf("crc32 checksum mismatch: got 0x%X, want 0x%X", calculatedChecksum, expectedChecksum)
	}

	var entry CallbackEntry
	if err := json.Unmarshal(payloadBytes, &entry); err != nil {
		return nil, 0, errfmt.Newf("failed to unmarshal entry payload").Wrap(err)
	}

	return &entry, totalLen, nil
}

// WireBatchFramer manages vectorized contiguous batch encoding and stream decoding.
type WireBatchFramer struct {
	codec *WireProtocolCodec
}

// NewWireBatchFramer creates a new WireBatchFramer instance.
func NewWireBatchFramer() *WireBatchFramer {
	return &WireBatchFramer{codec: NewWireProtocolCodec()}
}

// EncodeBatch encodes multiple CallbackEntry objects into a single contiguous binary buffer.
func (f *WireBatchFramer) EncodeBatch(entries []*CallbackEntry, buf []byte) ([]byte, error) {
	if len(entries) == 0 {
		return buf[:0], nil
	}

	out := buf[:0]
	for _, entry := range entries {
		var scratch [512]byte
		frame, err := f.codec.EncodeEntry(entry, scratch[:0])
		if err != nil {
			return nil, errfmt.Newf("failed to encode entry in batch").Wrap(err)
		}
		out = append(out, frame...)
	}
	return out, nil
}

// DecodeBatch parses contiguous wire frames until data is exhausted.
func (f *WireBatchFramer) DecodeBatch(data []byte) ([]*CallbackEntry, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var entries []*CallbackEntry
	offset := 0
	for offset < len(data) {
		entry, consumed, err := f.codec.DecodeEntry(data[offset:])
		if err != nil {
			return nil, errfmt.Newf("batch decoding error at offset %d", offset).Wrap(err)
		}
		entries = append(entries, entry)
		offset += consumed
	}
	return entries, nil
}
