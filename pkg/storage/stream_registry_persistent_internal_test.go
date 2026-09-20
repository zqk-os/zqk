package storage

import "testing"

func BenchmarkParseStreamRegistryLineFast(b *testing.B) {
	line := []byte(`{"id":"evt-123-abc","loc":"data/events/2026/06/22/evt-123-abc.json"}`)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parseStreamRegistryLineFast(line)
	}
}
