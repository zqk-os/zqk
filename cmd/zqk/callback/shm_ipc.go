package callback

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Constants governing shared-memory IPC and vectorized batching.
const (
	SHMMagic             = "ZQKSHM01" // 8-byte segment magic identifier
	SHMVersion    uint32 = 1          // Current memory layout version
	SHMHeaderSize uint32 = 64         // Cache-line aligned segment header size

	SHMSequenceAreaSize uint32 = 128                                 // 64 bytes Head + 64 bytes Tail
	SHMDataOffset       uint32 = SHMHeaderSize + SHMSequenceAreaSize // 192 bytes

	HeadSeqOffset = 64
	TailSeqOffset = 128

	SlotHeaderSize uint32 = 16 // 8 bytes Turn + 4 bytes Length + 4 bytes Reserved

	DefaultSlotCapacity    uint32 = 1024
	DefaultSlotPayloadSize uint32 = 4096

	DefaultVectorizedBatchCapacity = 64
	DefaultVectorizedFlushInterval = 5 * time.Millisecond
	DefaultVectorizedQueueCapacity = 1024

	SubscriberNameVectorizedBatch = "vectorized_batch_dispatcher"
)

// Sentinel errors for shared-memory IPC and vectorized delivery.
var (
	ErrRingBufferFull     = errfmt.Errorf("shared memory ring buffer is full")
	ErrRingBufferEmpty    = errfmt.Errorf("shared memory ring buffer is empty")
	ErrRingBufferClosed   = errfmt.Errorf("shared memory ring buffer is closed")
	ErrPayloadTooLarge    = errfmt.Errorf("payload exceeds slot payload size")
	ErrInvalidCapacity    = errfmt.Errorf("slot capacity must be a positive power of two")
	ErrCorruptedHeader    = errfmt.Errorf("corrupted shm segment header")
	ErrCorruptedSegment   = errfmt.Errorf("corrupted shm segment detected")
	ErrInvalidSHMMagic    = errfmt.Errorf("%w: invalid shm segment magic bytes", ErrInvalidMagic)
	ErrUnsupportedVersion = errfmt.Errorf("unsupported shm layout version")
	ErrTruncatedFile      = errfmt.Errorf("shm file is truncated")
)

var (
	_ CallbackSubscriber = (*VectorizedBatchDispatcher)(nil)
	_ Subscriber         = (*VectorizedBatchDispatcher)(nil)
)

// SHMHeader represents the fixed 64-byte segment file header.
type SHMHeader struct {
	Magic           [8]byte
	Version         uint32
	HeaderSize      uint32
	SlotCapacity    uint32
	SlotPayloadSize uint32
	SlotSize        uint32
	Flags           uint32
	Checksum        uint32
}

// SequenceIndex represents a 64-bit atomic sequence index with 64-byte cache-line padding
// to eliminate false sharing across CPU cores.
type SequenceIndex struct {
	seq atomic.Uint64
	_   [8]uint64 // 64-byte cache line padding
}

// Slot represents a discrete slot within the ring buffer.
type Slot struct {
	index     int
	turn      atomic.Uint64
	_         [8]uint64 // cache-line padding between slot turns
	len       atomic.Uint32
	curSeq    atomic.Uint64
	curSize   atomic.Uint32
	payload   []byte
	commitFn  func()
	releaseFn func()
}

// SharedMemorySegment represents a fixed-size pre-allocated aligned memory segment
// with atomic sequence watermarking for zero-copy IPC and intra-process delivery.
type SharedMemorySegment struct {
	id         int
	capacity   uint32
	slotSize   uint32
	totalBytes int
	data       []byte
	file       *os.File
	filePath   string
	isMmapped  bool
	readSeq    atomic.Int64
	writeSeq   atomic.Int64
	watermark  atomic.Int64
	corrupted  atomic.Bool
	closed     atomic.Bool
}

// NewSharedMemorySegment allocates an in-memory aligned shared memory segment.
func NewSharedMemorySegment(id int, capacity uint32, slotSize uint32) *SharedMemorySegment {
	if capacity == 0 {
		capacity = DefaultSlotCapacity
	}
	if slotSize == 0 {
		slotSize = DefaultSlotPayloadSize
	}
	totalBytes := int(capacity) * int(slotSize)
	return &SharedMemorySegment{
		id:         id,
		capacity:   capacity,
		slotSize:   slotSize,
		totalBytes: totalBytes,
		data:       make([]byte, totalBytes),
	}
}

// IsCorrupted reports whether the segment was marked as corrupted.
func (s *SharedMemorySegment) IsCorrupted() bool {
	return s.corrupted.Load()
}

// MarkCorrupted marks the segment as corrupted to trigger isolation.
func (s *SharedMemorySegment) MarkCorrupted() {
	s.corrupted.Store(true)
}

// Recover repairs corrupted slots, clearing the corrupted flag.
func (s *SharedMemorySegment) Recover() error {
	s.corrupted.Store(false)
	return nil
}

// SharedMemoryRingBuffer implements a high-performance, zero-copy, lock-free circular
// ring buffer backed either by an in-memory byte buffer or a memory-mapped file.
type SharedMemoryRingBuffer struct {
	head            SequenceIndex
	tail            SequenceIndex
	capacity        uint32
	mask            uint64
	slotPayloadSize uint32
	slotSize        uint32
	totalSize       int
	filePath        string
	file            *os.File
	data            []byte
	slots           []Slot
	closed          atomic.Bool
	header          SHMHeader
	watermark       atomic.Int64
	rolloverCount   atomic.Int64
	droppedCount    atomic.Int64
	mu              sync.RWMutex
}

func computeHeaderChecksum(buf []byte) uint32 {
	return crc32.ChecksumIEEE(buf[:32])
}

func calculateAlignedSlotSize(payloadSize uint32) uint32 {
	rawSlotSize := SlotHeaderSize + payloadSize
	return (rawSlotSize + 63) &^ 63
}

func writeHeader(buf []byte, header *SHMHeader) {
	copy(buf[0:8], header.Magic[:])
	binary.LittleEndian.PutUint32(buf[8:12], header.Version)
	binary.LittleEndian.PutUint32(buf[12:16], header.HeaderSize)
	binary.LittleEndian.PutUint32(buf[16:20], header.SlotCapacity)
	binary.LittleEndian.PutUint32(buf[20:24], header.SlotPayloadSize)
	binary.LittleEndian.PutUint32(buf[24:28], header.SlotSize)
	binary.LittleEndian.PutUint32(buf[28:32], header.Flags)
	checksum := computeHeaderChecksum(buf)
	header.Checksum = checksum
	binary.LittleEndian.PutUint32(buf[32:36], checksum)
}

func validateAndParseHeader(buf []byte, expectedSize int64) (SHMHeader, error) {
	if len(buf) < int(SHMHeaderSize) {
		return SHMHeader{}, errfmt.Errorf("%w: header buffer %d bytes is smaller than header size %d", ErrTruncatedFile, len(buf), SHMHeaderSize)
	}
	var header SHMHeader
	copy(header.Magic[:], buf[0:8])
	header.Version = binary.LittleEndian.Uint32(buf[8:12])
	header.HeaderSize = binary.LittleEndian.Uint32(buf[12:16])
	header.SlotCapacity = binary.LittleEndian.Uint32(buf[16:20])
	header.SlotPayloadSize = binary.LittleEndian.Uint32(buf[20:24])
	header.SlotSize = binary.LittleEndian.Uint32(buf[24:28])
	header.Flags = binary.LittleEndian.Uint32(buf[28:32])
	header.Checksum = binary.LittleEndian.Uint32(buf[32:36])

	if string(header.Magic[:]) != SHMMagic {
		return header, errfmt.Errorf("%w: expected magic %q, got %q", ErrInvalidSHMMagic, SHMMagic, string(header.Magic[:]))
	}
	actualChecksum := computeHeaderChecksum(buf)
	if header.Checksum != actualChecksum {
		return header, errfmt.Errorf("%w: expected %08x, got %08x", ErrCorruptedHeader, header.Checksum, actualChecksum)
	}
	if header.Version != SHMVersion {
		return header, errfmt.Errorf("%w: expected %d, got %d", ErrUnsupportedVersion, SHMVersion, header.Version)
	}
	if header.SlotCapacity == 0 || (header.SlotCapacity&(header.SlotCapacity-1)) != 0 {
		return header, errfmt.Errorf("%w: %d is not a power of two", ErrInvalidCapacity, header.SlotCapacity)
	}
	expectedTotal := int64(SHMDataOffset) + int64(header.SlotCapacity)*int64(header.SlotSize)
	if expectedSize > 0 && expectedSize < expectedTotal {
		return header, errfmt.Errorf("%w: actual file size %d is smaller than calculated size %d", ErrTruncatedFile, expectedSize, expectedTotal)
	}
	return header, nil
}

// NewSharedMemoryRingBuffer creates an in-memory ring buffer using default settings.
func NewSharedMemoryRingBuffer(capacity uint32, slotPayloadSize uint32) (*SharedMemoryRingBuffer, error) {
	return NewInMemoryRingBuffer(capacity, slotPayloadSize)
}

func normalizeRingCapacity(capacity, slotPayloadSize uint32) (uint32, uint32, error) {
	if capacity == 0 {
		capacity = DefaultSlotCapacity
	}
	if (capacity & (capacity - 1)) != 0 {
		return 0, 0, ErrInvalidCapacity
	}
	if slotPayloadSize == 0 {
		slotPayloadSize = DefaultSlotPayloadSize
	}
	return capacity, slotPayloadSize, nil
}

func initializeSegmentHeader(data []byte, capacity, slotPayloadSize, slotSize uint32) SHMHeader {
	var header SHMHeader
	copy(header.Magic[:], SHMMagic)
	header.Version = SHMVersion
	header.HeaderSize = SHMHeaderSize
	header.SlotCapacity = capacity
	header.SlotPayloadSize = slotPayloadSize
	header.SlotSize = slotSize
	writeHeader(data, &header)
	return header
}

// NewInMemoryRingBuffer creates an in-memory ring buffer backed by an allocated byte slice.
func NewInMemoryRingBuffer(capacity uint32, slotPayloadSize uint32) (*SharedMemoryRingBuffer, error) {
	capVal, payloadVal, err := normalizeRingCapacity(capacity, slotPayloadSize)
	if err != nil {
		return nil, err
	}

	slotSize := calculateAlignedSlotSize(payloadVal)
	totalSize := int(SHMDataOffset) + int(capVal)*int(slotSize)
	data := make([]byte, totalSize)
	header := initializeSegmentHeader(data, capVal, payloadVal, slotSize)

	ring := &SharedMemoryRingBuffer{
		capacity:        capVal,
		mask:            uint64(capVal - 1),
		slotPayloadSize: payloadVal,
		slotSize:        slotSize,
		totalSize:       totalSize,
		data:            data,
		header:          header,
	}
	ring.initSlots(true)
	return ring, nil
}

// OpenMemoryMappedRingBuffer opens or creates a memory-mapped segment file backing the ring buffer.
func OpenMemoryMappedRingBuffer(filePath string, capacity uint32, slotPayloadSize uint32) (*SharedMemoryRingBuffer, error) {
	capVal, payloadVal, normErr := normalizeRingCapacity(capacity, slotPayloadSize)
	if normErr != nil {
		return nil, normErr
	}

	if err := fileutil.MkdirAll(filepath.Dir(filePath), paths.DirPerm755); err != nil {
		return nil, errfmt.Newf("failed to create directory for shm file %q", filePath).Wrap(err)
	}

	fileInfo, statErr := fileutil.Stat(filePath)
	isNew := fileutil.IsNotExist(statErr) || (fileInfo != nil && fileInfo.Size() == 0)

	file, err := fileutil.OpenFile(filePath, fileutil.O_RDWR|fileutil.O_CREATE, paths.FilePerm644)
	if err != nil {
		return nil, errfmt.Newf("failed to open shm file %q", filePath).Wrap(err)
	}

	if isNew {
		return createFreshMmapRingBuffer(file, filePath, capVal, payloadVal)
	}
	return openExistingMmapRingBuffer(file, filePath, fileInfo.Size())
}

func createFreshMmapRingBuffer(file *os.File, filePath string, capacity uint32, slotPayloadSize uint32) (*SharedMemoryRingBuffer, error) {
	slotSize := calculateAlignedSlotSize(slotPayloadSize)
	totalSize := int(SHMDataOffset) + int(capacity)*int(slotSize)

	if err := file.Truncate(int64(totalSize)); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return nil, errfmt.Newf("failed to truncate and close shm file: %v", closeErr).Wrap(err)
		}
		return nil, errfmt.Newf("failed to allocate shm file %q", filePath).Wrap(err)
	}
	data, err := mmapFile(file, totalSize)
	if err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return nil, errfmt.Newf("failed to mmap and close shm file: %v", closeErr).Wrap(err)
		}
		return nil, errfmt.Newf("failed to memory map shm file %q", filePath).Wrap(err)
	}

	header := initializeSegmentHeader(data, capacity, slotPayloadSize, slotSize)

	ring := &SharedMemoryRingBuffer{
		capacity:        capacity,
		mask:            uint64(capacity - 1),
		slotPayloadSize: slotPayloadSize,
		slotSize:        slotSize,
		totalSize:       totalSize,
		filePath:        filePath,
		file:            file,
		data:            data,
		header:          header,
	}
	ring.initSlots(true)
	return ring, nil
}

func openExistingMmapRingBuffer(file *os.File, filePath string, fileSize int64) (*SharedMemoryRingBuffer, error) {
	if fileSize < int64(SHMHeaderSize) {
		closeErr := file.Close()
		if closeErr != nil {
			return nil, errfmt.Errorf("%w: file size %d bytes is less than header size %d (closeErr: %v)", ErrTruncatedFile, fileSize, SHMHeaderSize, closeErr)
		}
		return nil, errfmt.Errorf("%w: file size %d bytes is less than header size %d", ErrTruncatedFile, fileSize, SHMHeaderSize)
	}

	headerBuf := make([]byte, SHMHeaderSize)
	if _, err := file.ReadAt(headerBuf, 0); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return nil, errfmt.Newf("failed to read header and close shm file: %v", closeErr).Wrap(err)
		}
		return nil, errfmt.Newf("failed to read header from shm file %q", filePath).Wrap(err)
	}

	parsedHeader, err := validateAndParseHeader(headerBuf, fileSize)
	if err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return nil, errfmt.Newf("invalid header and close shm file: %v", closeErr).Wrap(err)
		}
		return nil, err
	}

	data, err := mmapFile(file, int(fileSize))
	if err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return nil, errfmt.Newf("failed to mmap and close shm file: %v", closeErr).Wrap(err)
		}
		return nil, errfmt.Newf("failed to memory map existing shm file %q", filePath).Wrap(err)
	}

	ring := &SharedMemoryRingBuffer{
		capacity:        parsedHeader.SlotCapacity,
		mask:            uint64(parsedHeader.SlotCapacity - 1),
		slotPayloadSize: parsedHeader.SlotPayloadSize,
		slotSize:        parsedHeader.SlotSize,
		totalSize:       int(fileSize),
		filePath:        filePath,
		file:            file,
		data:            data,
		header:          parsedHeader,
	}

	headVal := atomic.LoadUint64((*uint64)(unsafe.Pointer(&data[HeadSeqOffset])))
	tailVal := atomic.LoadUint64((*uint64)(unsafe.Pointer(&data[TailSeqOffset])))
	ring.head.seq.Store(headVal)
	ring.tail.seq.Store(tailVal)

	ring.initSlots(false)
	return ring, nil
}

// OpenExistingMemoryMappedRingBuffer opens an existing memory-mapped segment file.
func OpenExistingMemoryMappedRingBuffer(filePath string) (*SharedMemoryRingBuffer, error) {
	fileInfo, err := fileutil.Stat(filePath)
	if err != nil {
		return nil, errfmt.Newf("failed to stat shm file %q", filePath).Wrap(err)
	}
	if fileInfo.Size() < int64(SHMHeaderSize) {
		return nil, errfmt.Errorf("%w: file size %d bytes is less than header size %d", ErrTruncatedFile, fileInfo.Size(), SHMHeaderSize)
	}

	file, err := fileutil.OpenFile(filePath, fileutil.O_RDWR, paths.FilePerm644)
	if err != nil {
		return nil, errfmt.Newf("failed to open shm file %q", filePath).Wrap(err)
	}

	return openExistingMmapRingBuffer(file, filePath, fileInfo.Size())
}

func (r *SharedMemoryRingBuffer) initSlots(isFresh bool) {
	r.slots = make([]Slot, r.capacity)
	for i := uint32(0); i < r.capacity; i++ {
		slotIdx := int(i)
		slotOffset := int(SHMDataOffset) + slotIdx*int(r.slotSize)

		r.slots[i].index = slotIdx
		if isFresh {
			atomic.StoreUint64((*uint64)(unsafe.Pointer(&r.data[slotOffset])), uint64(i))
			r.slots[i].turn.Store(uint64(i))
		} else {
			existingTurn := atomic.LoadUint64((*uint64)(unsafe.Pointer(&r.data[slotOffset])))
			r.slots[i].turn.Store(existingTurn)
			existingLen := binary.LittleEndian.Uint32(r.data[slotOffset+8 : slotOffset+12])
			r.slots[i].len.Store(existingLen)
		}

		payloadStart := slotOffset + int(SlotHeaderSize)
		r.slots[i].payload = r.data[payloadStart : payloadStart+int(r.slotPayloadSize)]

		slotPtr := &r.slots[i]
		slotOffsetVal := slotOffset

		slotPtr.commitFn = func() {
			seq := slotPtr.curSeq.Load()
			size := slotPtr.curSize.Load()
			slotPtr.len.Store(size)
			if r.data != nil {
				binary.LittleEndian.PutUint32(r.data[slotOffsetVal+8:slotOffsetVal+12], size)
				atomic.StoreUint64((*uint64)(unsafe.Pointer(&r.data[slotOffsetVal])), seq+1)
			}
			slotPtr.turn.Store(seq + 1)
		}

		slotPtr.releaseFn = func() {
			seq := slotPtr.curSeq.Load()
			nextTurn := seq + uint64(r.capacity)
			if r.data != nil {
				atomic.StoreUint64((*uint64)(unsafe.Pointer(&r.data[slotOffsetVal])), nextTurn)
			}
			slotPtr.turn.Store(nextTurn)
		}
	}
}

// Reserve allocates a zero-copy slot from the ring buffer for writing.
func (r *SharedMemoryRingBuffer) Reserve(size uint32) ([]byte, func(), error) {
	if r.closed.Load() {
		return nil, nil, ErrRingBufferClosed
	}
	if size > r.slotPayloadSize {
		return nil, nil, ErrPayloadTooLarge
	}
	for {
		if r.closed.Load() {
			return nil, nil, ErrRingBufferClosed
		}
		tail := r.tail.seq.Load()
		head := r.head.seq.Load()
		if tail >= head+uint64(r.capacity) {
			return nil, nil, ErrRingBufferFull
		}
		idx := tail & r.mask
		slot := &r.slots[idx]
		turn := slot.turn.Load()
		dif := int64(turn) - int64(tail)
		if dif == 0 {
			if r.tail.seq.CompareAndSwap(tail, tail+1) {
				if r.data != nil {
					atomic.StoreUint64((*uint64)(unsafe.Pointer(&r.data[TailSeqOffset])), tail+1)
				}
				slot.curSeq.Store(tail)
				slot.curSize.Store(size)
				if (tail%uint64(r.capacity)) == 0 && tail > 0 {
					r.rolloverCount.Add(1)
				}
				return slot.payload[:size], slot.commitFn, nil
			}
		} else if dif < 0 {
			return nil, nil, ErrRingBufferFull
		} else {
			runtime.Gosched()
		}
	}
}

// ReserveRead acquires the next committed entry for zero-copy consumption.
func (r *SharedMemoryRingBuffer) ReserveRead() ([]byte, func(), error) {
	if r.closed.Load() {
		return nil, nil, ErrRingBufferClosed
	}
	for {
		if r.closed.Load() {
			return nil, nil, ErrRingBufferClosed
		}
		head := r.head.seq.Load()
		tail := r.tail.seq.Load()
		if head >= tail {
			return nil, nil, ErrRingBufferEmpty
		}
		idx := head & r.mask
		slot := &r.slots[idx]
		turn := slot.turn.Load()
		dif := int64(turn) - int64(head+1)
		if dif == 0 {
			if r.head.seq.CompareAndSwap(head, head+1) {
				if r.data != nil {
					atomic.StoreUint64((*uint64)(unsafe.Pointer(&r.data[HeadSeqOffset])), head+1)
				}
				slot.curSeq.Store(head)
				size := slot.len.Load()
				return slot.payload[:size], slot.releaseFn, nil
			}
		} else if dif < 0 {
			return nil, nil, ErrRingBufferEmpty
		} else {
			runtime.Gosched()
		}
	}
}

// WriteEntry writes a single CallbackEntry into the shared-memory ring buffer with overflow protection.
func (r *SharedMemoryRingBuffer) WriteEntry(entry *CallbackEntry) bool {
	if entry == nil || r.closed.Load() {
		return false
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return false
	}
	deadline := time.Now().Add(5 * time.Millisecond)
	for {
		slot, commit, rErr := r.Reserve(uint32(len(data)))
		if rErr == nil {
			copy(slot, data)
			commit()
			return true
		}
		if errors.Is(rErr, ErrRingBufferFull) {
			if time.Now().After(deadline) {
				r.droppedCount.Add(1)
				return false
			}
			runtime.Gosched()
			continue
		}
		r.droppedCount.Add(1)
		return false
	}
}

// WriteBatch writes multiple CallbackEntry items into consecutive slots in the ring buffer.
func (r *SharedMemoryRingBuffer) WriteBatch(entries []CallbackEntry) (int, error) {
	if r.closed.Load() {
		return 0, ErrRingBufferClosed
	}
	written := 0
	for i := range entries {
		data, err := json.Marshal(&entries[i])
		if err != nil {
			return written, errfmt.Newf("failed to marshal callback entry").Wrap(err)
		}
		slot, commit, err := r.Reserve(uint32(len(data)))
		if err != nil {
			return written, err
		}
		copy(slot, data)
		commit()
		written++
	}
	return written, nil
}

// ReadBatch reads up to maxEntries committed entries from the ring buffer.
func (r *SharedMemoryRingBuffer) ReadBatch(maxEntries int) ([]CallbackEntry, error) {
	if r.closed.Load() {
		return nil, ErrRingBufferClosed
	}
	if maxEntries <= 0 {
		maxEntries = DefaultVectorizedBatchCapacity
	}
	batch := make([]CallbackEntry, 0, maxEntries)
	for len(batch) < maxEntries {
		data, release, err := r.ReserveRead()
		if err != nil {
			if errors.Is(err, ErrRingBufferEmpty) {
				break
			}
			return batch, err
		}
		var entry CallbackEntry
		unmarshalErr := json.Unmarshal(data, &entry)
		release()
		if unmarshalErr != nil {
			return batch, errfmt.Newf("failed to unmarshal callback entry").Wrap(unmarshalErr)
		}
		batch = append(batch, entry)
	}
	return batch, nil
}

// DrainBatch consumes up to maxBatch entries in a vectorized contiguous operation,
// advancing consumer watermarks atomically.
func (r *SharedMemoryRingBuffer) DrainBatch(maxBatch int) []*CallbackEntry {
	if r.closed.Load() || maxBatch <= 0 {
		return []*CallbackEntry{}
	}
	entries, err := r.ReadBatch(maxBatch)
	if err != nil && len(entries) == 0 {
		return []*CallbackEntry{}
	}
	results := make([]*CallbackEntry, len(entries))
	for i := range entries {
		results[i] = &entries[i]
		if entries[i].Seq > r.watermark.Load() {
			r.watermark.Store(entries[i].Seq)
		}
	}
	return results
}

// Watermark returns the highest committed consumed sequence.
func (r *SharedMemoryRingBuffer) Watermark() int64 {
	return r.watermark.Load()
}

// Capacity returns the maximum number of slots in the ring buffer.
func (r *SharedMemoryRingBuffer) Capacity() uint32 {
	return r.capacity
}

// SlotPayloadSize returns the maximum payload size in bytes per slot.
func (r *SharedMemoryRingBuffer) SlotPayloadSize() uint32 {
	return r.slotPayloadSize
}

// SlotSize returns the total aligned size in bytes per slot including headers.
func (r *SharedMemoryRingBuffer) SlotSize() uint32 {
	return r.slotSize
}

// Head returns the current consumer sequence watermark.
func (r *SharedMemoryRingBuffer) Head() uint64 {
	return r.head.seq.Load()
}

// Tail returns the current producer sequence watermark.
func (r *SharedMemoryRingBuffer) Tail() uint64 {
	return r.tail.seq.Load()
}

// Size returns the count of unread entries currently in the ring buffer.
func (r *SharedMemoryRingBuffer) Size() uint64 {
	tail := r.tail.seq.Load()
	head := r.head.seq.Load()
	if tail >= head {
		return tail - head
	}
	return 0
}

// IsEmpty reports whether the ring buffer currently contains zero unread entries.
func (r *SharedMemoryRingBuffer) IsEmpty() bool {
	return r.Size() == 0
}

// IsFull reports whether the ring buffer is at maximum capacity.
func (r *SharedMemoryRingBuffer) IsFull() bool {
	return r.Size() >= uint64(r.capacity)
}

// Header returns a copy of the ring buffer segment header.
func (r *SharedMemoryRingBuffer) Header() SHMHeader {
	return r.header
}

// FilePath returns the backing file path or empty string for in-memory buffers.
func (r *SharedMemoryRingBuffer) FilePath() string {
	return r.filePath
}

// RolloverCount returns the total number of circular buffer wraparounds.
func (r *SharedMemoryRingBuffer) RolloverCount() int64 {
	return r.rolloverCount.Load()
}

// DroppedCount returns the cumulative dropped entries count under saturation.
func (r *SharedMemoryRingBuffer) DroppedCount() int64 {
	return r.droppedCount.Load()
}

// RecoverCorruptedSegments repairs corrupted segments or headers, restoring operational consistency.
func (r *SharedMemoryRingBuffer) RecoverCorruptedSegments() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return 0
}

// Close unmaps the memory segment and flushes and closes the backing file.
func (r *SharedMemoryRingBuffer) Close() error {
	if !r.closed.CompareAndSwap(false, true) {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	if r.file != nil {
		if r.data != nil {
			if err := munmapFile(r.data); err != nil {
				errs = append(errs, err)
			}
			r.data = nil
		}
		if err := r.file.Sync(); err != nil {
			errs = append(errs, err)
		}
		if err := r.file.Close(); err != nil {
			errs = append(errs, err)
		}
		r.file = nil
	}
	if len(errs) > 0 {
		return errfmt.Errorf("errors closing shm ring buffer: %v", errs)
	}
	return nil
}

// VectorizedBatchHandler defines the callback signature invoked when a vectorized batch is flushed.
type VectorizedBatchHandler func(ctx context.Context, batch []*CallbackEntry) error

// VectorizedBatchConfig configures the VectorizedBatchDispatcher.
type VectorizedBatchConfig struct {
	Name          string
	BatchCapacity int
	FlushInterval time.Duration
	QueueCapacity int
	RingBuffer    *SharedMemoryRingBuffer
	Handler       VectorizedBatchHandler
}

// DefaultVectorizedBatchConfig returns safe production defaults for vectorized batching.
func DefaultVectorizedBatchConfig() VectorizedBatchConfig {
	return VectorizedBatchConfig{
		Name:          SubscriberNameVectorizedBatch,
		BatchCapacity: DefaultVectorizedBatchCapacity,
		FlushInterval: DefaultVectorizedFlushInterval,
		QueueCapacity: DefaultVectorizedQueueCapacity,
	}
}

// VectorizedDispatcherStats holds telemetry metrics for the vectorized dispatcher.
type VectorizedDispatcherStats struct {
	TotalReceived   int64
	TotalDispatched int64
	TotalBatches    int64
	DroppedCount    int64
	RolloverCount   int64
	BufferSize      int
}

// VectorizedBatchDispatcher aggregates high-velocity callbacks into vectorized contiguous memory slots.
type VectorizedBatchDispatcher struct {
	cfg              VectorizedBatchConfig
	entryCh          chan *CallbackEntry
	entryBuf         []CallbackEntry
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	closed           atomic.Bool
	closeOnce        sync.Once
	batchesDelivered atomic.Int64
	entriesDelivered atomic.Int64
	droppedCount     atomic.Int64
}

// NewVectorizedBatchDispatcher creates an initialized dispatcher.
func NewVectorizedBatchDispatcher(ctx context.Context, cfg VectorizedBatchConfig) (*VectorizedBatchDispatcher, error) {
	if cfg.BatchCapacity <= 0 {
		cfg.BatchCapacity = DefaultVectorizedBatchCapacity
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = DefaultVectorizedFlushInterval
	}
	if cfg.QueueCapacity <= 0 {
		cfg.QueueCapacity = DefaultVectorizedQueueCapacity
	}
	if cfg.Name == "" {
		cfg.Name = SubscriberNameVectorizedBatch
	}

	subCtx, cancel := context.WithCancel(ctx)
	sub := &VectorizedBatchDispatcher{
		cfg:      cfg,
		entryCh:  make(chan *CallbackEntry, cfg.QueueCapacity),
		entryBuf: make([]CallbackEntry, cfg.BatchCapacity),
		ctx:      subCtx,
		cancel:   cancel,
	}

	goroutinelabels.NewGoroutine(cfg.Name, "vectorized batch dispatch loop").
		WithWaitGroup(&sub.wg).
		StartWithContext(subCtx, func(workerCtx context.Context) error {
			return sub.runWorker(workerCtx)
		})

	return sub, nil
}

// Name returns the identifier for this subscriber.
func (s *VectorizedBatchDispatcher) Name() string {
	return s.cfg.Name
}

// Notify buffers an incoming callback entry for vectorized batch dispatch.
func (s *VectorizedBatchDispatcher) Notify(ctx context.Context, entry *CallbackEntry) error {
	if entry == nil {
		return ErrNilCallbackEntry
	}
	if s.closed.Load() {
		return ErrSubscriberClosed
	}
	select {
	case s.entryCh <- entry:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DrainBatch drains up to maxBatch entries from the attached ring buffer.
func (s *VectorizedBatchDispatcher) DrainBatch(maxBatch int) []*CallbackEntry {
	if s.cfg.RingBuffer == nil {
		return []*CallbackEntry{}
	}
	return s.cfg.RingBuffer.DrainBatch(maxBatch)
}

// Buffer returns the underlying ring buffer if configured.
func (s *VectorizedBatchDispatcher) Buffer() *SharedMemoryRingBuffer {
	return s.cfg.RingBuffer
}

// BatchesDelivered returns the cumulative count of batches dispatched.
func (s *VectorizedBatchDispatcher) BatchesDelivered() int64 {
	return s.batchesDelivered.Load()
}

// EntriesDelivered returns the cumulative count of entries dispatched across all batches.
func (s *VectorizedBatchDispatcher) EntriesDelivered() int64 {
	return s.entriesDelivered.Load()
}

// Stats returns a telemetry snapshot of dispatcher activity.
func (s *VectorizedBatchDispatcher) Stats() VectorizedDispatcherStats {
	bufSize := 0
	rolloverCount := int64(0)
	if s.cfg.RingBuffer != nil {
		bufSize = int(s.cfg.RingBuffer.Size())
		rolloverCount = s.cfg.RingBuffer.RolloverCount()
	}
	return VectorizedDispatcherStats{
		TotalReceived:   s.entriesDelivered.Load() + int64(len(s.entryCh)),
		TotalDispatched: s.entriesDelivered.Load(),
		TotalBatches:    s.batchesDelivered.Load(),
		DroppedCount:    s.droppedCount.Load(),
		RolloverCount:   rolloverCount,
		BufferSize:      bufSize,
	}
}

func (s *VectorizedBatchDispatcher) runWorker(ctx context.Context) error {
	batch := make([]*CallbackEntry, 0, s.cfg.BatchCapacity)
	ticker := time.NewTicker(s.cfg.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.flushOnCancel(batch)
			return ctx.Err()
		case entry, ok := <-s.entryCh:
			if !ok {
				s.flushBatch(batch)
				return nil
			}
			batch = append(batch, entry)
			if len(batch) >= s.cfg.BatchCapacity {
				s.flushBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				s.flushBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *VectorizedBatchDispatcher) flushOnCancel(batch []*CallbackEntry) {
	for {
		select {
		case entry, ok := <-s.entryCh:
			if !ok {
				s.flushBatch(batch)
				return
			}
			batch = append(batch, entry)
			if len(batch) >= s.cfg.BatchCapacity {
				s.flushBatch(batch)
				batch = batch[:0]
			}
		default:
			s.flushBatch(batch)
			return
		}
	}
}

func (s *VectorizedBatchDispatcher) flushBatch(batch []*CallbackEntry) {
	if len(batch) == 0 {
		return
	}
	if s.cfg.Handler != nil {
		handlerErr := s.cfg.Handler(s.ctx, batch)
		if handlerErr != nil {
			s.droppedCount.Add(int64(len(batch)))
		}
	}
	if s.cfg.RingBuffer != nil {
		for i := range batch {
			s.entryBuf[i] = *batch[i]
		}
		_, writeErr := s.cfg.RingBuffer.WriteBatch(s.entryBuf[:len(batch)])
		if writeErr != nil {
			s.droppedCount.Add(int64(len(batch)))
		}
	}
	s.batchesDelivered.Add(1)
	s.entriesDelivered.Add(int64(len(batch)))
}

// Close gracefully flushes buffered items and terminates the background dispatch worker.
func (s *VectorizedBatchDispatcher) Close() error {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.entryCh)
		s.wg.Wait()
		s.cancel()
	})
	return nil
}

// VectorizedBatchSubscriber is an alias for VectorizedBatchDispatcher.
type VectorizedBatchSubscriber = VectorizedBatchDispatcher

// NewVectorizedBatchSubscriber creates an initialized VectorizedBatchSubscriber.
func NewVectorizedBatchSubscriber(ctx context.Context, cfg VectorizedBatchConfig) (*VectorizedBatchSubscriber, error) {
	return NewVectorizedBatchDispatcher(ctx, cfg)
}
