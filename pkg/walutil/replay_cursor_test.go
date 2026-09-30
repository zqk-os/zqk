package walutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReplayFromCursor_SeeksPastAppliedAndDeliversNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.wal")
	var lines []string
	for i := 1; i <= 5; i++ {
		b, _ := json.Marshal(map[string]any{"seq": i, "v": i})
		lines = append(lines, string(b)+"\n")
	}
	if err := fileutil.WriteFile(path, []byte(lines[0]+lines[1]+lines[2]), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	// First pass: consume seq 1..3 from offset 0
	var got []int64
	stats, err := ReplayFromCursor[map[string]any](path, 0, ReplayCursor{},
		func(b []byte) (*map[string]any, error) {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				return nil, err
			}
			return &m, nil
		},
		func(m *map[string]any) int64 {
			return int64((*m)["seq"].(float64))
		},
		func(m *map[string]any) error {
			got = append(got, int64((*m)["seq"].(float64)))
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Delivered != 3 || len(got) != 3 {
		t.Fatalf("first pass delivered=%d got=%v", stats.Delivered, got)
	}
	if stats.Cursor.Offset <= 0 || stats.Cursor.Seq != 3 {
		t.Fatalf("cursor=%+v", stats.Cursor)
	}

	// Append seq 4..5
	f, err := fileutil.OpenFile(path, fileutil.O_APPEND|fileutil.O_WRONLY, paths.FilePerm600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(lines[3] + lines[4]); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	got = nil
	stats2, err := ReplayFromCursor[map[string]any](path, 0, stats.Cursor,
		func(b []byte) (*map[string]any, error) {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				return nil, err
			}
			return &m, nil
		},
		func(m *map[string]any) int64 {
			return int64((*m)["seq"].(float64))
		},
		func(m *map[string]any) error {
			got = append(got, int64((*m)["seq"].(float64)))
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if stats2.Delivered != 2 || stats2.Scanned != 2 {
		t.Fatalf("second pass delivered=%d scanned=%d got=%v (want only new lines)", stats2.Delivered, stats2.Scanned, got)
	}
	if len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("got=%v", got)
	}
}

func TestReplayFromCursor_IdleAtEOFScansNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.wal")
	line, _ := json.Marshal(map[string]any{"seq": 1})
	body := append(line, '\n')
	if err := fileutil.WriteFile(path, body, paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	cursor := ReplayCursor{Seq: 1, Offset: int64(len(body))}
	stats, err := ReplayFromCursor[map[string]any](path, 0, cursor,
		func(b []byte) (*map[string]any, error) {
			t.Fatal("parse should not run on idle EOF")
			return nil, nil
		},
		func(m *map[string]any) int64 { return 0 },
		func(m *map[string]any) error {
			t.Fatal("fn should not run on idle EOF")
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Scanned != 0 || stats.Delivered != 0 {
		t.Fatalf("idle stats=%+v", stats)
	}
}

func TestReplayFromCursor_IdleEOFSkipsOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.wal")
	line, _ := json.Marshal(map[string]any{"seq": 1})
	body := append(line, '\n')
	if err := fileutil.WriteFile(path, body, paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	cursor := ReplayCursor{Seq: 1, Offset: int64(len(body))}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, paths.FilePerm600) })
	if f, err := os.Open(path); err == nil {
		_ = f.Close()
		t.Skip("process can still open chmod-0 file (e.g. root); cannot prove Open was skipped")
	}
	stats, err := ReplayFromCursor[map[string]any](path, 0, cursor,
		func(b []byte) (*map[string]any, error) {
			t.Fatal("parse should not run when Open is skipped")
			return nil, nil
		},
		func(m *map[string]any) int64 { return 0 },
		func(m *map[string]any) error {
			t.Fatal("fn should not run when Open is skipped")
			return nil
		},
	)
	if err != nil {
		t.Fatalf("idle skip should Stat-only, not Open: %v", err)
	}
	if stats.Scanned != 0 || stats.Delivered != 0 {
		t.Fatalf("idle skip stats=%+v", stats)
	}
}

func TestParseReplayCursorCheckpoint_Compat(t *testing.T) {
	c, err := ParseReplayCursorCheckpoint([]byte("42"))
	if err != nil || c.Seq != 42 || c.Offset != 0 {
		t.Fatalf("plain seq: %+v err=%v", c, err)
	}
	c2, err := ParseReplayCursorCheckpoint([]byte(`{"seq":7,"offset":99}`))
	if err != nil || c2.Seq != 7 || c2.Offset != 99 {
		t.Fatalf("json: %+v err=%v", c2, err)
	}
}

func TestExtractSeqFromJSONLine(t *testing.T) {
	if got := ExtractSeqFromJSONLine([]byte(`{"seq":12,"x":1}`)); got != 12 {
		t.Fatalf("got %d", got)
	}
}

func TestReplayFromCursor_CorruptedAndOversizedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.wal")

	// Write 1 good line, 1 corrupted line, 1 oversized line, 1 good line
	content := "{\"seq\":1,\"v\":\"valid1\"}\n" +
		"{this is corrupt not json}\n" +
		"{\"seq\":3,\"v\":\"very long line that exceeds limit\"}\n" +
		"{\"seq\":4,\"v\":\"valid4\"}\n"

	if err := fileutil.WriteFile(path, []byte(content), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}

	var delivered []int64
	stats, err := ReplayFromCursor[map[string]any](path, 30, ReplayCursor{},
		func(b []byte) (*map[string]any, error) {
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				return nil, err
			}
			return &m, nil
		},
		func(m *map[string]any) int64 {
			return int64((*m)["seq"].(float64))
		},
		func(m *map[string]any) error {
			delivered = append(delivered, int64((*m)["seq"].(float64)))
			return nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stats.Scanned != 4 {
		t.Errorf("expected 4 scanned, got %d", stats.Scanned)
	}
	if stats.Delivered != 2 {
		t.Errorf("expected 2 delivered, got %d", stats.Delivered)
	}
	if stats.Corrupted != 1 {
		t.Errorf("expected 1 corrupted, got %d", stats.Corrupted)
	}
	if stats.Oversized != 1 {
		t.Errorf("expected 1 oversized, got %d", stats.Oversized)
	}
	if len(delivered) != 2 || delivered[0] != 1 || delivered[1] != 4 {
		t.Errorf("expected delivered [1, 4], got %v", delivered)
	}
}
