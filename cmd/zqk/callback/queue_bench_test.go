package callback

import (
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

func BenchmarkQueue_Enqueue(b *testing.B) {
	q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	entry := &CallbackEntry{
		Payload: map[string]any{
			"job_id":                     "bench-job",
			objects.FieldKeyCallbackType: "completion",
		},
		Timestamp: time.Now().UTC(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.Enqueue(entry)
	}
}

func BenchmarkQueue_DequeueSorted(b *testing.B) {
	q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	// Pre-populate queue
	for i := 0; i < 1000; i++ {
		entry := &CallbackEntry{
			Payload: map[string]any{
				"job_id": "bench-job",
			},
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Millisecond),
		}
		_ = q.Enqueue(entry)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Re-enqueue to maintain queue size
		if q.Size() == 0 {
			for j := 0; j < 100; j++ {
				entry := &CallbackEntry{
					Payload: map[string]any{
						"job_id": "bench-job",
					},
					Timestamp: time.Now().UTC(),
				}
				_ = q.Enqueue(entry)
			}
		}
		_ = q.DequeueSorted(10)
	}
}

func BenchmarkQueue_ConcurrentEnqueue(b *testing.B) {
	q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	entry := &CallbackEntry{
		Payload: map[string]any{
			"job_id": "bench-job",
		},
		Timestamp: time.Now().UTC(),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = q.Enqueue(entry)
		}
	})
}

func BenchmarkQueue_ConcurrentEnqueueDequeue(b *testing.B) {
	q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		entry := &CallbackEntry{
			Payload: map[string]any{
				"job_id": "bench-job",
			},
			Timestamp: time.Now().UTC(),
		}

		for pb.Next() {
			_ = q.Enqueue(entry)
			_ = q.DequeueSorted(1)
		}
	})
}

func BenchmarkTimestampSorter_Sort(b *testing.B) {
	sorter := &TimestampSorter{}
	entries := make([]*CallbackEntry, 1000)

	for i := 0; i < 1000; i++ {
		entries[i] = &CallbackEntry{
			Payload: map[string]any{
				"job_id": "bench-job",
			},
			Timestamp: time.Now().UTC().Add(time.Duration(1000-i) * time.Millisecond),
			Priority:  i % 20,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sorter.Sort(entries)
	}
}

func BenchmarkPrioritySorter_Sort(b *testing.B) {
	sorter := &PrioritySorter{}
	entries := make([]*CallbackEntry, 1000)

	for i := 0; i < 1000; i++ {
		entries[i] = &CallbackEntry{
			Payload: map[string]any{
				"job_id": "bench-job",
			},
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Millisecond),
			Priority:  i % 20,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sorter.Sort(entries)
	}
}

func BenchmarkQueue_LargeQueue(b *testing.B) {
	q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	// Pre-populate with large queue
	const queueSize = 10000
	for i := 0; i < queueSize; i++ {
		entry := &CallbackEntry{
			Payload: map[string]any{
				"job_id": "bench-job",
			},
			Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Millisecond),
			Priority:  i % 20,
		}
		_ = q.Enqueue(entry)
	}

	b.ResetTimer()
	b.Run("DequeueBatch10", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if q.Size() == 0 {
				// Re-populate
				for j := 0; j < queueSize; j++ {
					entry := &CallbackEntry{
						Payload: map[string]any{
							"job_id": "bench-job",
						},
						Timestamp: time.Now().UTC(),
					}
					_ = q.Enqueue(entry)
				}
			}
			_ = q.DequeueSorted(10)
		}
	})

	b.Run("DequeueBatch100", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if q.Size() == 0 {
				// Re-populate
				for j := 0; j < queueSize; j++ {
					entry := &CallbackEntry{
						Payload: map[string]any{
							"job_id": "bench-job",
						},
						Timestamp: time.Now().UTC(),
					}
					_ = q.Enqueue(entry)
				}
			}
			_ = q.DequeueSorted(100)
		}
	})
}

func BenchmarkQueue_HighConcurrency(b *testing.B) {
	q := NewQueue(0, &TimestampSorter{}, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		entry := &CallbackEntry{
			Payload: map[string]any{
				"job_id": "bench-job",
			},
			Timestamp: time.Now().UTC(),
		}

		for pb.Next() {
			var wg sync.WaitGroup
			// Simulate high concurrency: multiple operations per iteration
			goroutinelabels.NewGoroutine("bench_enqueuer", "enqueuing in benchmark").
				WithWaitGroup(&wg).
				StartSimple(func() {
					_ = q.Enqueue(entry)
				})
			goroutinelabels.NewGoroutine("bench_dequeuer", "dequeuing in benchmark").
				WithWaitGroup(&wg).
				StartSimple(func() {
					_ = q.DequeueSorted(1)
				})
			wg.Wait()
		}
	})
}
