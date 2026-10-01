package cli

import (
	"bytes"
	"testing"
)

func ScanForObjectIDsBenchmark(b *testing.B) {
	data := []byte(`
		Here is an object PROMPT-1782226824226137000-7d41c8a7.
		And an ignored token NOT_AN_ID_HERE.
		Some random text and invalid ids like PROMPT-123-abc.
		Valid again: MIS-1775446507801844000-42ba7fc8.
	`)

	// Create a larger payload to make the benchmark meaningful
	largeData := bytes.Repeat(data, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ids := ScanForObjectIDs(largeData)
		if len(ids) != 2 {
			b.Fatalf("expected 2 unique ids, got %d", len(ids))
		}
	}
}

func TestScanForObjectIDs(t *testing.T) {
	data := []byte(`
		Here is an object PROMPT-1782226824226137000-7d41c8a7.
		And an ignored token NOT_AN_ID_HERE.
		Some random text and invalid ids like PROMPT-123-abc.
		Valid again: MIS-1775446507801844000-42ba7fc8.
	`)

	ids := ScanForObjectIDs(data)
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %v", ids)
	}
}
