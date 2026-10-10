package callback

import (
	"context"
	"io"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/walutil"
)

const (
	// DefaultRingBufferCapacity is the fallback capacity per ring buffer shard.
	DefaultRingBufferCapacity = 1024

	// DefaultShardedRingBufferShards is the default shard count (power of 2).
	DefaultShardedRingBufferShards = 4

	// SubscriberNameShardedRingBuffer identifies the ring buffer subscriber in dispatchers.
	SubscriberNameShardedRingBuffer = "sharded_ring_buffer"

	// fnvOffset64 and fnvPrime64 define the constants for FNV-1a hashing.
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

var (
	_ CallbackSubscriber = (*ShardedRingBuffer)(nil)
	_ Subscriber         = (*ShardedRingBuffer)(nil)
)

// WALTruncator defines the contract for truncating write-ahead log files up to a committed sequence watermark.
type WALTruncator interface {
	TruncateBefore(watermark int64) error
}

// WALTruncateFunc adapts a standard function into a WALTruncator.
type WALTruncateFunc func(watermark int64) error

// TruncateBefore invokes the underlying closure.
func (f WALTruncateFunc) TruncateBefore(watermark int64) error {
	return f(watermark)
}

// RingBuffer implements a bounded circular buffer with FIFO overwrite eviction on saturation.
type RingBuffer struct {
	mu           sync.RWMutex
	buffer       []*CallbackEntry
	head         int // Index of oldest entry (read position)
	tail         int // Index of next write position
	size         int // Active count in buffer
	capacity     int // Fixed buffer capacity
	evictedCount int64
	totalPushed  int64
	totalPopped  int64
	watermark    int64 // Highest committed / consumed sequence
}

// NewRingBuffer allocates an initialized fixed-capacity circular ring buffer.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = DefaultRingBufferCapacity
	}
	return &RingBuffer{
		buffer:   make([]*CallbackEntry, capacity),
		capacity: capacity,
	}
}

// Capacity returns the total allocated capacity of the ring buffer.
func (r *RingBuffer) Capacity() int {
	return r.capacity
}

// Size returns the count of active unread items in the ring buffer.
func (r *RingBuffer) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size
}

// IsEmpty reports whether the ring buffer contains no entries.
func (r *RingBuffer) IsEmpty() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size == 0
}

// IsFull reports whether the ring buffer is at maximum capacity.
func (r *RingBuffer) IsFull() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size == r.capacity
}

// EvictedCount returns the cumulative number of entries dropped due to buffer saturation.
func (r *RingBuffer) EvictedCount() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.evictedCount
}

// TotalPushed returns the cumulative count of entries pushed to this buffer.
func (r *RingBuffer) TotalPushed() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.totalPushed
}

// TotalPopped returns the cumulative count of entries popped or drained from this buffer.
func (r *RingBuffer) TotalPopped() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.totalPopped
}

// Push appends an entry to the circular buffer. If the buffer is saturated,
// the oldest entry is evicted, evicted count incremented, and true is returned.
func (r *RingBuffer) Push(entry *CallbackEntry) bool {
	if entry == nil {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.totalPushed++
	if r.size == r.capacity {
		r.evictOldest()
		r.insertAtTail(entry)
		return true
	}

	r.insertAtTail(entry)
	r.size++
	return false
}

func (r *RingBuffer) evictOldest() {
	evicted := r.buffer[r.head]
	r.buffer[r.head] = nil
	r.head = (r.head + 1) % r.capacity
	r.evictedCount++
	if evicted != nil && evicted.Seq > r.watermark {
		r.watermark = evicted.Seq
	}
}

func (r *RingBuffer) insertAtTail(entry *CallbackEntry) {
	r.buffer[r.tail] = entry
	r.tail = (r.tail + 1) % r.capacity
}

// Pop extracts the oldest entry in FIFO order. Returns false if the buffer is empty.
func (r *RingBuffer) Pop() (*CallbackEntry, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.size == 0 {
		return nil, false
	}

	entry := r.buffer[r.head]
	r.buffer[r.head] = nil
	r.head = (r.head + 1) % r.capacity
	r.size--
	r.totalPopped++

	if entry != nil && entry.Seq > r.watermark {
		r.watermark = entry.Seq
	}

	return entry, true
}

// Drain extracts up to max entries in FIFO order.
func (r *RingBuffer) Drain(max int) []*CallbackEntry {
	if max <= 0 {
		return []*CallbackEntry{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.size == 0 {
		return []*CallbackEntry{}
	}

	count := max
	if count > r.size {
		count = r.size
	}

	results := make([]*CallbackEntry, count)
	for i := 0; i < count; i++ {
		entry := r.buffer[r.head]
		r.buffer[r.head] = nil
		r.head = (r.head + 1) % r.capacity
		results[i] = entry
		if entry != nil && entry.Seq > r.watermark {
			r.watermark = entry.Seq
		}
	}

	r.size -= count
	r.totalPopped += int64(count)
	return results
}

// Watermark calculates the committed sequence watermark for WAL truncation.
// If the buffer holds unconsumed entries, the watermark is the sequence before the oldest unconsumed entry.
// If the buffer is empty, the watermark is the highest consumed/evicted sequence.
func (r *RingBuffer) Watermark() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.size == 0 {
		return r.watermark
	}

	oldest := r.buffer[r.head]
	if oldest == nil || oldest.Seq <= 1 {
		return r.watermark
	}

	candidate := oldest.Seq - 1
	if candidate > r.watermark {
		return candidate
	}
	return r.watermark
}

// AdvanceWatermark explicitly sets the watermark sequence if seq exceeds current watermark.
func (r *RingBuffer) AdvanceWatermark(seq int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if seq > r.watermark {
		r.watermark = seq
		return true
	}
	return false
}

// Reset clears all buffer contents, zeroing pointers and indices.
func (r *RingBuffer) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.buffer {
		r.buffer[i] = nil
	}
	r.head = 0
	r.tail = 0
	r.size = 0
	r.evictedCount = 0
	r.totalPushed = 0
	r.totalPopped = 0
	r.watermark = 0
}

// ShardedRingBufferConfig contains configuration parameters for ShardedRingBuffer.
type ShardedRingBufferConfig struct {
	NumShards         int
	CapacityPerShard  int
	Truncator         WALTruncator
	AutoTruncateDelta int64
}

// ShardedRingBufferOption configures a ShardedRingBuffer instance.
type ShardedRingBufferOption func(*ShardedRingBufferConfig)

// WithWALTruncator configures an active WAL truncator on the sharded buffer.
func WithWALTruncator(t WALTruncator) ShardedRingBufferOption {
	return func(c *ShardedRingBufferConfig) {
		c.Truncator = t
	}
}

// WithAutoTruncateDelta triggers truncation when committed watermark advances by delta.
func WithAutoTruncateDelta(delta int64) ShardedRingBufferOption {
	return func(c *ShardedRingBufferConfig) {
		c.AutoTruncateDelta = delta
	}
}

// ShardedRingBuffer partitions incoming callback events across N independent ring buffers.
type ShardedRingBuffer struct {
	mu                     sync.RWMutex
	shards                 []*RingBuffer
	shardMask              uint64
	roundRobinCounter      atomic.Uint64
	seqGenerator           atomic.Int64
	truncator              WALTruncator
	autoTruncateDelta      int64
	lastTruncatedWatermark int64
}

// NewShardedRingBuffer initializes a partitioned ring buffer pool.
func NewShardedRingBuffer(numShards int, capacityPerShard int, opts ...ShardedRingBufferOption) *ShardedRingBuffer {
	numShards = normalizePowerOfTwo(numShards)
	if capacityPerShard <= 0 {
		capacityPerShard = DefaultRingBufferCapacity
	}

	cfg := ShardedRingBufferConfig{
		NumShards:        numShards,
		CapacityPerShard: capacityPerShard,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	shards := make([]*RingBuffer, numShards)
	for i := 0; i < numShards; i++ {
		shards[i] = NewRingBuffer(capacityPerShard)
	}

	return &ShardedRingBuffer{
		shards:            shards,
		shardMask:         uint64(numShards - 1),
		truncator:         cfg.Truncator,
		autoTruncateDelta: cfg.AutoTruncateDelta,
	}
}

func normalizePowerOfTwo(n int) int {
	if n <= 0 {
		return DefaultShardedRingBufferShards
	}
	p := 1
	for p < n {
		p <<= 1
	}
	if p < DefaultShardedRingBufferShards {
		return DefaultShardedRingBufferShards
	}
	return p
}

// Name returns the subscriber identifier for integration into event dispatchers.
func (s *ShardedRingBuffer) Name() string {
	return SubscriberNameShardedRingBuffer
}

// Notify handles incoming events from MultiSubscriberDispatcher, fulfilling Subscriber.
func (s *ShardedRingBuffer) Notify(ctx context.Context, entry *CallbackEntry) error {
	s.Push(entry)
	return nil
}

// NumShards returns the number of active partition shards.
func (s *ShardedRingBuffer) NumShards() int {
	return len(s.shards)
}

// Shard retrieves the ring buffer at the specified partition index.
func (s *ShardedRingBuffer) Shard(idx int) *RingBuffer {
	if idx < 0 || idx >= len(s.shards) {
		return nil
	}
	return s.shards[idx]
}

// TotalCapacity returns aggregate allocated capacity across all partition shards.
func (s *ShardedRingBuffer) TotalCapacity() int {
	total := 0
	for _, shard := range s.shards {
		total += shard.Capacity()
	}
	return total
}

// TotalSize returns the aggregate number of active buffered entries across shards.
func (s *ShardedRingBuffer) TotalSize() int {
	total := 0
	for _, shard := range s.shards {
		total += shard.Size()
	}
	return total
}

// EvictedCount returns total dropped entries across all shards due to saturation.
func (s *ShardedRingBuffer) EvictedCount() int64 {
	var total int64
	for _, shard := range s.shards {
		total += shard.EvictedCount()
	}
	return total
}

// ShardIndexFor returns the target shard index for an entry using FNV-1a hashing on JobID.
func (s *ShardedRingBuffer) ShardIndexFor(entry *CallbackEntry) int {
	if entry == nil {
		return 0
	}

	key := entry.JobID
	if key == "" && entry.Payload != nil {
		key = extractKeyFromPayload(entry.Payload)
	}

	if key == "" {
		next := s.roundRobinCounter.Add(1)
		return int((next - 1) & s.shardMask)
	}

	return int(hashFNV1a64(key) & s.shardMask)
}

func extractKeyFromPayload(payload map[string]any) string {
	if objID := objects.GetString(payload, "object_id"); objID != "" {
		return objID
	}
	if id := objects.GetString(payload, "id"); id != "" {
		return id
	}
	return ""
}

func hashFNV1a64(s string) uint64 {
	var h uint64 = fnvOffset64
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}
	return h
}

// Push routes an entry to its partition shard and enqueues it.
func (s *ShardedRingBuffer) Push(entry *CallbackEntry) bool {
	if entry == nil {
		return false
	}

	s.ensureSequence(entry)
	idx := s.ShardIndexFor(entry)
	evicted := s.shards[idx].Push(entry)

	s.maybeAutoTruncate()
	return evicted
}

func (s *ShardedRingBuffer) ensureSequence(entry *CallbackEntry) {
	if entry.Seq > 0 {
		return
	}
	if entry.Payload != nil {
		if rawSeq, ok := entry.Payload["seq"]; ok {
			entry.Seq = parseNumericSeq(rawSeq)
		}
	}
	if entry.Seq <= 0 {
		entry.Seq = s.seqGenerator.Add(1)
	}
}

func parseNumericSeq(val any) int64 {
	switch v := val.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

// DrainShard drains up to max entries from a specific partition shard.
func (s *ShardedRingBuffer) DrainShard(shardIdx int, max int) []*CallbackEntry {
	if shardIdx < 0 || shardIdx >= len(s.shards) {
		return []*CallbackEntry{}
	}
	entries := s.shards[shardIdx].Drain(max)
	s.maybeAutoTruncate()
	return entries
}

// DrainAll drains up to maxPerShard entries from each partition shard.
func (s *ShardedRingBuffer) DrainAll(maxPerShard int) []*CallbackEntry {
	var collected []*CallbackEntry
	for _, shard := range s.shards {
		drained := shard.Drain(maxPerShard)
		if len(drained) > 0 {
			collected = append(collected, drained...)
		}
	}
	s.maybeAutoTruncate()
	return collected
}

// CommittedWatermark calculates the minimum committed watermark among all active shards.
// Active shards are partitions that have received at least one event.
func (s *ShardedRingBuffer) CommittedWatermark() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.committedWatermarkLocked()
}

// SetWALTruncator assigns the WAL truncation driver.
func (s *ShardedRingBuffer) SetWALTruncator(t WALTruncator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.truncator = t
}

// SetTruncateHook assigns a truncation callback closure.
func (s *ShardedRingBuffer) SetTruncateHook(hook func(watermark int64) error) {
	s.SetWALTruncator(WALTruncateFunc(hook))
}

// LastTruncatedWatermark returns the watermark at the most recent successful truncation.
func (s *ShardedRingBuffer) LastTruncatedWatermark() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastTruncatedWatermark
}

// TruncateWAL triggers a WAL truncation execution using the current CommittedWatermark.
func (s *ShardedRingBuffer) TruncateWAL() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	wm := s.committedWatermarkLocked()
	if wm <= 0 || wm <= s.lastTruncatedWatermark {
		return s.lastTruncatedWatermark, nil
	}

	if s.truncator != nil {
		if err := s.truncator.TruncateBefore(wm); err != nil {
			return s.lastTruncatedWatermark, errfmt.Newf("WAL truncation hook failed at watermark %d", wm).Wrap(err)
		}
	}

	s.lastTruncatedWatermark = wm
	return wm, nil
}

func (s *ShardedRingBuffer) committedWatermarkLocked() int64 {
	var minWatermark int64 = -1
	activeShards := 0

	for _, shard := range s.shards {
		if shard.TotalPushed() == 0 {
			continue
		}
		activeShards++
		wm := shard.Watermark()
		if minWatermark == -1 || wm < minWatermark {
			minWatermark = wm
		}
	}

	if activeShards == 0 || minWatermark < 0 {
		return 0
	}
	return minWatermark
}

func (s *ShardedRingBuffer) maybeAutoTruncate() {
	if s.autoTruncateDelta <= 0 {
		return
	}
	wm := s.CommittedWatermark()
	s.mu.RLock()
	last := s.lastTruncatedWatermark
	s.mu.RUnlock()

	if wm-last >= s.autoTruncateDelta {
		s.triggerAutoTruncation()
	}
}

func (s *ShardedRingBuffer) triggerAutoTruncation() {
	wm, err := s.TruncateWAL()
	if err != nil {
		return
	}
	if wm > 0 {
		return
	}
}

// TruncateLifecycleWALFile purges committed lines from a JSON-line WAL file up to watermark,
// rewriting retained records atomically and persisting an updated cursor checkpoint.
func TruncateLifecycleWALFile(walPath string, watermark int64) (int, int, error) {
	if walPath == "" {
		return 0, 0, errfmt.Errorf("walPath cannot be empty")
	}
	if watermark <= 0 {
		return 0, 0, nil
	}

	tmpPath := walPath + ".truncating"
	retained, truncated, err := filterWALFile(walPath, tmpPath, watermark)
	if err != nil {
		return retained, truncated, err
	}

	if err := fileutil.Rename(tmpPath, walPath); err != nil {
		return retained, truncated, errfmt.Errorf("failed to replace truncated WAL file: %w", err)
	}

	ckptPath := walPath + ".checkpoint"
	ckptData := walutil.FormatReplayCursorCheckpoint(walutil.ReplayCursor{Seq: watermark})
	if err := fileutil.WriteDurableFile(ckptPath, ckptData, paths.FilePerm644); err != nil {
		return retained, truncated, errfmt.Errorf("failed to persist WAL checkpoint after truncation: %w", err)
	}

	return retained, truncated, nil
}

func writeWALRecord(w io.Writer, line []byte) error {
	record := make([]byte, len(line)+1)
	copy(record, line)
	record[len(line)] = '\n'
	written, err := w.Write(record)
	if err != nil {
		return err
	}
	if written != len(record) {
		return io.ErrShortWrite
	}
	return nil
}

func filterWALFile(srcPath, dstPath string, watermark int64) (int, int, error) {
	retained := 0
	truncated := 0

	err := fileutil.MkdirAll(filepath.Dir(dstPath), paths.DirPerm755)
	if err != nil {
		return 0, 0, errfmt.Errorf("failed to create WAL directory: %w", err)
	}

	dstFile, err := fileutil.OpenFile(dstPath, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_TRUNC, paths.FilePerm644)
	if err != nil {
		return 0, 0, errfmt.Errorf("failed to create temp WAL file: %w", err)
	}
	defer dstFile.Close()

	var writeErr error
	scanErr := walutil.ScanFileLines(srcPath, 64*1024, func(line []byte) {
		if writeErr != nil {
			return
		}
		seq := walutil.ExtractSeqFromJSON(line)
		if seq > 0 && seq <= watermark {
			truncated++
			return
		}
		retained++
		if err := writeWALRecord(dstFile, line); err != nil {
			writeErr = err
		}
	})
	if scanErr != nil {
		return retained, truncated, scanErr
	}
	if writeErr != nil {
		return retained, truncated, writeErr
	}

	if err := dstFile.Sync(); err != nil {
		return retained, truncated, err
	}

	return retained, truncated, nil
}

// NewLifecycleWALFileTruncator returns a WALTruncator targeting an on-disk WAL file path.
func NewLifecycleWALFileTruncator(walPath string) WALTruncator {
	return WALTruncateFunc(func(watermark int64) error {
		retained, truncated, err := TruncateLifecycleWALFile(walPath, watermark)
		if err != nil {
			return err
		}
		if retained >= 0 && truncated >= 0 {
			return nil
		}
		return nil
	})
}
