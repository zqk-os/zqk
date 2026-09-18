package validation

import (
	"testing"
	"time"
)

// BenchmarkPriorityQueueEnqueueChronological fills a queue the way `system check` does:
// one task per object, each stamped later than the last. The sorted insert scans for a
// position that cannot exist in that order, so the cost is quadratic in the object count —
// which is why the kernel growing past 8000 objects made the check disproportionately
// slower without any change to this file.
//
// TRACK: BLI-1787554988821523000-5d6ba7fd — remove when Enqueue fast-paths the append case;
// left unfixed here because it is ~0.2% of check runtime and the loop's lock semantics came
// from a contention fix (f093c7db27) that deserves its own change.
func BenchmarkPriorityQueueEnqueueChronological(b *testing.B) {
	for _, n := range []int{1000, 4000, 8000} {
		b.Run(itoa(n), func(b *testing.B) {
			for range b.N {
				pq := NewPriorityQueue()
				base := time.Now()
				for i := range n {
					pq.Enqueue(&ValidationTask{
						ObjectID:   "OBJ-" + itoa(i),
						Priority:   2,
						EnqueuedAt: base.Add(time.Duration(i) * time.Microsecond),
					})
				}
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
