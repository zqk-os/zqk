package wal_test

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/storage/wal"
)

// FuzzParseWALLine fuzzes the WAL record line parser (v1 single, v2 single, v2 batch)
// ensuring that arbitrary corrupted bytes, malformed JSON, and invalid base64 payloads
// never trigger panics or memory corruption.
func FuzzParseWALLine(f *testing.F) {
	// Seed corpus with valid v1, v2 single, v2 batch, and malformed inputs
	seeds := [][]byte{
		[]byte(`{"op":"create","kind":"backlog_item","id":"bli-001","seq":1,"data_base64":"e2lkOiBibGktMDAxfQ=="}`),
		[]byte(`{"o":"c","k":"task","i":"tsk-001","s":2,"d":"payload"}`),
		[]byte(`[{"o":"u","k":"plan","i":"pri-001","s":3},{"o":"d","k":"task","i":"tsk-002","s":4}]`),
		[]byte(`{"op":"","kind":"","id":"","seq":0}`),
		[]byte(`[{"invalid_json":`),
		[]byte(``),
		[]byte(`\x00\xff\xfe`),
		[]byte(`"s":123456789`),
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Test ParseWALLine
		records, err := wal.ParseWALLine(data)
		if err == nil {
			for _, r := range records {
				if r != nil {
					_, _ = r.DecodeRecordData()
				}
			}
		}

		// Test GetMaxSeqFromLine
		_ = wal.GetMaxSeqFromLine(data)
	})
}
