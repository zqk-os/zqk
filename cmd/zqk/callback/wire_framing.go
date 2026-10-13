package callback

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Wire protocol framing constants.
const (
	WireMagicByte0 byte = 'Z'
	WireMagicByte1 byte = 'Q'
	WireMagicByte2 byte = 'K'
	WireMagicByte3 byte = '1'

	HeaderSize                int    = 22
	HeaderChecksumFieldOffset int    = 18
	MaxFramePayloadSize       uint32 = 64 * 1024 * 1024

	FlagNone        uint16 = 0
	FlagBatch       uint16 = 1 << 0
	FlagCompressed  uint16 = 1 << 1
	FlagHeartbeat   uint16 = 1 << 2
	FlagEndOfStream uint16 = 1 << 3

	DefaultPooledBufferSize int = 65536
)

// MagicWireProtocol represents the standard 4-byte protocol identifier.
var MagicWireProtocol = [4]byte{WireMagicByte0, WireMagicByte1, WireMagicByte2, WireMagicByte3}

// Error descriptions extracted to named constants.
const (
	ErrTextInvalidMagic     = "invalid wire protocol magic bytes"
	ErrTextHeaderTruncated  = "frame header is truncated"
	ErrTextPayloadTruncated = "frame payload is truncated"
	ErrTextChecksumMismatch = "frame header checksum mismatch"
	ErrTextFrameTooLarge    = "frame payload exceeds maximum allowed size"
	ErrTextBufferTooSmall   = "destination buffer is too small"
	ErrTextEmptyBatch       = "batch framing requires at least one payload"
)

// Standard wire framing error values.
var (
	ErrInvalidWireMagic = errfmt.Errorf("%w: %s", ErrInvalidMagic, ErrTextInvalidMagic)
	ErrHeaderTruncated  = errors.New(ErrTextHeaderTruncated)
	ErrPayloadTruncated = errors.New(ErrTextPayloadTruncated)
	ErrChecksumMismatch = errors.New(ErrTextChecksumMismatch)
	ErrFrameTooLarge    = errors.New(ErrTextFrameTooLarge)
	ErrBufferTooSmall   = errors.New(ErrTextBufferTooSmall)
	ErrEmptyBatch       = errors.New(ErrTextEmptyBatch)
)

// FrameHeader represents the 22-byte wire protocol header.
// Layout:
//   - Magic: 4 bytes ("ZQK1")
//   - Flags: 2 bytes (uint16 big endian)
//   - SeqID: 8 bytes (uint64 big endian)
//   - Length: 4 bytes (uint32 big endian)
//   - Checksum: 4 bytes (IEEE CRC32 over the preceding 18 bytes)
type FrameHeader struct {
	Magic    [4]byte
	Flags    uint16
	SeqID    uint64
	Length   uint32
	Checksum uint32
}

// Frame represents a decoded or prepared wire protocol frame.
// Payload is a direct subslice of the underlying buffer for zero-copy efficiency.
type Frame struct {
	Header  FrameHeader
	Payload []byte
}

// TotalSize returns the total wire size in bytes (header + payload).
func (f Frame) TotalSize() int {
	return HeaderSize + len(f.Payload)
}

// NewFrameHeader creates a FrameHeader initialized with the default magic identifier.
func NewFrameHeader(seqID uint64, flags uint16, length uint32) FrameHeader {
	return FrameHeader{
		Magic:  MagicWireProtocol,
		Flags:  flags,
		SeqID:  seqID,
		Length: length,
	}
}

// CalculateHeaderChecksum computes the IEEE CRC32 checksum across the 18 header metadata bytes.
func CalculateHeaderChecksum(magic [4]byte, flags uint16, seqID uint64, length uint32) uint32 {
	var buf [HeaderChecksumFieldOffset]byte
	buf[0] = magic[0]
	buf[1] = magic[1]
	buf[2] = magic[2]
	buf[3] = magic[3]
	binary.BigEndian.PutUint16(buf[4:6], flags)
	binary.BigEndian.PutUint64(buf[6:14], seqID)
	binary.BigEndian.PutUint32(buf[14:18], length)
	return crc32.ChecksumIEEE(buf[:])
}

// EncodeHeader serializes the header into the destination buffer and writes the CRC32 checksum.
func EncodeHeader(h *FrameHeader, dst []byte) error {
	if len(dst) < HeaderSize {
		return ErrBufferTooSmall
	}
	dst[0] = h.Magic[0]
	dst[1] = h.Magic[1]
	dst[2] = h.Magic[2]
	dst[3] = h.Magic[3]
	binary.BigEndian.PutUint16(dst[4:6], h.Flags)
	binary.BigEndian.PutUint64(dst[6:14], h.SeqID)
	binary.BigEndian.PutUint32(dst[14:18], h.Length)

	checksum := crc32.ChecksumIEEE(dst[0:HeaderChecksumFieldOffset])
	h.Checksum = checksum
	binary.BigEndian.PutUint32(dst[HeaderChecksumFieldOffset:HeaderSize], checksum)
	return nil
}

// DecodeHeader deserializes and validates a FrameHeader from the source buffer.
func DecodeHeader(src []byte) (FrameHeader, error) {
	if len(src) < HeaderSize {
		return FrameHeader{}, ErrHeaderTruncated
	}
	if src[0] != WireMagicByte0 || src[1] != WireMagicByte1 || src[2] != WireMagicByte2 || src[3] != WireMagicByte3 {
		return FrameHeader{}, ErrInvalidWireMagic
	}
	expectedChecksum := binary.BigEndian.Uint32(src[HeaderChecksumFieldOffset:HeaderSize])
	actualChecksum := crc32.ChecksumIEEE(src[0:HeaderChecksumFieldOffset])
	if expectedChecksum != actualChecksum {
		return FrameHeader{}, ErrChecksumMismatch
	}
	length := binary.BigEndian.Uint32(src[14:18])
	if length > MaxFramePayloadSize {
		return FrameHeader{}, ErrFrameTooLarge
	}
	header := FrameHeader{
		Magic:    [4]byte{src[0], src[1], src[2], src[3]},
		Flags:    binary.BigEndian.Uint16(src[4:6]),
		SeqID:    binary.BigEndian.Uint64(src[6:14]),
		Length:   length,
		Checksum: expectedChecksum,
	}
	return header, nil
}

// EncodeFrame serializes a single header and payload into dst.
// Returns the number of bytes written.
func EncodeFrame(h FrameHeader, payload []byte, dst []byte) (int, error) {
	payloadLen := len(payload)
	totalSize := HeaderSize + payloadLen
	if len(dst) < totalSize {
		return 0, ErrBufferTooSmall
	}
	h.Length = uint32(payloadLen)
	err := EncodeHeader(&h, dst[:HeaderSize])
	if err != nil {
		return 0, err
	}
	if payloadLen > 0 {
		copy(dst[HeaderSize:totalSize], payload)
	}
	return totalSize, nil
}

// DecodeFrame deserializes a single frame with zero-copy payload referencing src.
// Returns the Frame and total bytes consumed.
func DecodeFrame(src []byte) (Frame, int, error) {
	header, err := DecodeHeader(src)
	if err != nil {
		return Frame{}, 0, err
	}
	totalSize := HeaderSize + int(header.Length)
	if len(src) < totalSize {
		return Frame{}, 0, ErrPayloadTruncated
	}
	return Frame{
		Header:  header,
		Payload: src[HeaderSize:totalSize],
	}, totalSize, nil
}

// PackBatch packs multiple frames into a contiguous byte buffer.
// If cap(dst) is insufficient, a new slice of exact required capacity is allocated.
func PackBatch(frames []Frame, dst []byte) ([]byte, error) {
	if len(frames) == 0 {
		return nil, ErrEmptyBatch
	}
	totalBytes := 0
	for i := 0; i < len(frames); i++ {
		totalBytes += HeaderSize + len(frames[i].Payload)
	}
	out := dst
	if cap(dst) >= totalBytes {
		out = dst[:totalBytes]
	} else {
		out = make([]byte, totalBytes)
	}
	offset := 0
	for i := 0; i < len(frames); i++ {
		f := frames[i]
		f.Header.Flags |= FlagBatch
		f.Header.Length = uint32(len(f.Payload))
		n, err := EncodeFrame(f.Header, f.Payload, out[offset:])
		if err != nil {
			return nil, err
		}
		offset += n
	}
	return out, nil
}

// PackBatchPayloads vectorizes raw payloads into a single contiguous buffer with sequential sequence IDs.
func PackBatchPayloads(startSeq uint64, flags uint16, payloads [][]byte, dst []byte) ([]byte, error) {
	if len(payloads) == 0 {
		return nil, ErrEmptyBatch
	}
	frames := make([]Frame, len(payloads))
	for i := 0; i < len(payloads); i++ {
		frames[i] = Frame{
			Header:  NewFrameHeader(startSeq+uint64(i), flags|FlagBatch, uint32(len(payloads[i]))),
			Payload: payloads[i],
		}
	}
	return PackBatch(frames, dst)
}

// UnpackBatch deserializes all contiguous frames from data without copying payloads.
func UnpackBatch(data []byte) ([]Frame, error) {
	return UnpackBatchInto(data, nil)
}

// UnpackBatchInto unpacks frames into a pre-allocated destination slice to reduce slice header allocations.
func UnpackBatchInto(data []byte, dst []Frame) ([]Frame, error) {
	if len(data) == 0 {
		return nil, ErrEmptyBatch
	}
	frames := dst[:0]
	offset := 0
	for offset < len(data) {
		frame, consumed, err := DecodeFrame(data[offset:])
		if err != nil {
			return nil, err
		}
		frames = append(frames, frame)
		offset += consumed
	}
	return frames, nil
}

// PooledBuffer wraps a pooled byte slice to avoid boxing allocations in sync.Pool.
type PooledBuffer struct {
	Buf []byte
}

// Bytes returns the underlying byte slice.
func (pb *PooledBuffer) Bytes() []byte {
	return pb.Buf
}

// WireBufferPool manages reusable PooledBuffer structures to achieve zero allocations.
type WireBufferPool struct {
	pool     sync.Pool
	capacity int
}

// NewWireBufferPool initializes a buffer pool with the given capacity.
func NewWireBufferPool(defaultCap int) *WireBufferPool {
	if defaultCap <= 0 {
		defaultCap = DefaultPooledBufferSize
	}
	capacity := defaultCap
	p := &WireBufferPool{capacity: capacity}
	p.pool = sync.Pool{
		New: func() any {
			return &PooledBuffer{Buf: make([]byte, capacity)}
		},
	}
	return p
}

// Acquire retrieves a PooledBuffer from the pool.
func (p *WireBufferPool) Acquire() *PooledBuffer {
	return p.pool.Get().(*PooledBuffer)
}

// Release returns a PooledBuffer back to the pool if its capacity satisfies the pool minimum.
func (p *WireBufferPool) Release(pb *PooledBuffer) {
	if pb == nil {
		return
	}
	if cap(pb.Buf) < p.capacity {
		return
	}
	p.pool.Put(pb)
}

var defaultGlobalWirePool = NewWireBufferPool(DefaultPooledBufferSize)

// DefaultWireBufferPool returns the process-wide default buffer pool.
func DefaultWireBufferPool() *WireBufferPool {
	return defaultGlobalWirePool
}

// EncodeFramePooled serializes a frame using the default buffer pool and returns a release callback.
func EncodeFramePooled(h FrameHeader, payload []byte) ([]byte, func(), error) {
	pool := DefaultWireBufferPool()
	pb := pool.Acquire()
	totalSize := HeaderSize + len(payload)
	if len(pb.Buf) < totalSize {
		oversized := make([]byte, totalSize)
		n, err := EncodeFrame(h, payload, oversized)
		pool.Release(pb)
		if err != nil {
			return nil, func() {}, err
		}
		return oversized[:n], func() {}, nil
	}
	n, err := EncodeFrame(h, payload, pb.Buf)
	if err != nil {
		pool.Release(pb)
		return nil, func() {}, err
	}
	release := func() {
		pool.Release(pb)
	}
	return pb.Buf[:n], release, nil
}
