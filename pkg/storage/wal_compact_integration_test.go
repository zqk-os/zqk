package storage

import "testing"

func BenchmarkGetMaxSeqFromLine(b *testing.B) {
	line := []byte(`{"o":"c","k":"audit_event","i":"evt-123","s":123456789,"d":"some-base64-payload-that-would-be-huge"}`)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		getMaxSeqFromLine(line)
	}
}
