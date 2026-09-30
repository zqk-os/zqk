package logging

import (
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// sharedBufferedFlusher ticks once for every BufferedWriter in the process.
//
// 2026-08-20 scheduler dump (pid 95580, 3.2GB RSS, 651MB heap): 238 of 341
// goroutines were (*BufferedWriter).autoFlush. Each 1s ticker could block in
// write(2) and grow parked Darwin Ms that never shrink.
// same OS-thread class as scheduler state
// retention; remove per-writer tickers. Shared loop is process-lifetime.
type sharedBufferedFlusher struct {
	once    sync.Once
	mu      sync.Mutex
	writers map[*BufferedWriter]struct{}
}

var processBufferedFlusher = &sharedBufferedFlusher{
	writers: make(map[*BufferedWriter]struct{}),
}

func (f *sharedBufferedFlusher) register(bw *BufferedWriter) {
	if f == nil || bw == nil {
		return
	}
	f.mu.Lock()
	f.writers[bw] = struct{}{}
	f.mu.Unlock()
	f.once.Do(func() {
		goroutinelabels.NewGoroutine("buffered_writer_shared_flush", "flush all buffered log writers on one ticker").
			StartSimple(f.loop)
	})
}

func (f *sharedBufferedFlusher) unregister(bw *BufferedWriter) {
	if f == nil || bw == nil {
		return
	}
	f.mu.Lock()
	delete(f.writers, bw)
	f.mu.Unlock()
}

func (f *sharedBufferedFlusher) snapshot() []*BufferedWriter {
	f.mu.Lock()
	out := make([]*BufferedWriter, 0, len(f.writers))
	for w := range f.writers {
		out = append(out, w)
	}
	f.mu.Unlock()
	return out
}

func (f *sharedBufferedFlusher) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		for _, w := range f.snapshot() {
			_ = w.Flush()
		}
	}
}
