// BLI-STARTER-COMMUNITY-033 / PRI-STARTER-COMMUNITY-033 coverage elevation
package walutil

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReadWriteJSONFile(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "x.json")
	var out map[string]any
	if err := ReadJSONFile(p, &out); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(p, nil, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := ReadJSONFile(p, &out); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(p, []byte("{"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := ReadJSONFile(p, &out); err == nil {
		t.Fatal("bad json")
	}
	good := filepath.Join(t.TempDir(), "ok.json")
	if err := WriteJSONFileAtomic(good, map[string]any{"a": 1}, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	out = nil
	if err := ReadJSONFile(good, &out); err != nil || out["a"] == nil {
		t.Fatalf("%v %v", out, err)
	}
}

func TestOpenJSONLineWAL_EmptyArgsAndExtract(t *testing.T) {
	t.Parallel()
	if _, err := OpenJSONLineWAL[map[string]any]("", "f.jsonl", ".ck", 0); err == nil {
		t.Fatal("empty root")
	}
	if _, err := OpenJSONLineWAL[map[string]any](t.TempDir(), "", ".ck", 0); err == nil {
		t.Fatal("empty file")
	}
	if _, err := OpenJSONLineWAL[map[string]any](t.TempDir(), "f.jsonl", "", 0); err == nil {
		t.Fatal("empty ck")
	}
	if ExtractSeqFromJSON([]byte("not-json")) != 0 {
		t.Fatal("bad json seq")
	}
	if ExtractSeqFromJSON([]byte(`{"seq":3}`)) != 3 {
		t.Fatal("float seq")
	}
	if ExtractSeqFromAnyJSON(map[string]any{"seq": 4}) != 4 {
		t.Fatal("any seq")
	}
	if ExtractSeqFromJSONLine([]byte(`{"seq": 12}`)) != 12 {
		t.Fatal("line seq")
	}
	if ExtractSeqFromJSONLine([]byte(`{"id":1}`)) != 0 {
		t.Fatal("no seq")
	}
}

func TestReplayCursorCheckpointAndMissingWAL(t *testing.T) {
	t.Parallel()
	c, err := ParseReplayCursorCheckpoint(nil)
	if err != nil || c.Seq != 0 {
		t.Fatalf("%v %v", c, err)
	}
	c, err = ParseReplayCursorCheckpoint([]byte("7"))
	if err != nil || c.Seq != 7 {
		t.Fatalf("%v %v", c, err)
	}
	c, err = ParseReplayCursorCheckpoint([]byte(`{"seq":8,"offset":9}`))
	if err != nil || c.Seq != 8 || c.Offset != 9 {
		t.Fatalf("%v %v", c, err)
	}
	if _, err := ParseReplayCursorCheckpoint([]byte("{")); err == nil {
		t.Fatal("bad json")
	}
	b := FormatReplayCursorCheckpoint(ReplayCursor{Seq: 1, Offset: 2})
	if len(b) == 0 {
		t.Fatal("format")
	}
	missing := filepath.Join(t.TempDir(), "no.jsonl")
	err = ReplayFrom[map[string]any](missing, 0, 0, func([]byte) (*map[string]any, error) {
		return nil, nil
	}, func(*map[string]any) int64 { return 0 }, func(*map[string]any) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := CloseAfterFlush(nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestJSONLineWAL_AppendReplayRoundTrip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	w, err := OpenJSONLineWAL[map[string]any](root, "ev.jsonl", ".ck", 0)
	if err != nil {
		t.Fatal(err)
	}
	if w.Path() == "" || w.CheckpointPath() == "" {
		t.Fatal("paths")
	}
	if err := w.Append(func(seq int64, ts time.Time) map[string]any {
		return map[string]any{"seq": seq, "n": seq}
	}, 0); err != nil {
		t.Fatal(err)
	}
	if err := w.Append(func(seq int64, ts time.Time) map[string]any {
		return map[string]any{"seq": seq, "n": seq}
	}, 1); err == nil {
		t.Fatal("line too large")
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	walPath := w.Path()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var got []int64
	err = ReplayFrom[map[string]any](walPath, 0, 0,
		func(b []byte) (*map[string]any, error) {
			var m map[string]any
			if uerr := json.Unmarshal(b, &m); uerr != nil {
				return nil, uerr
			}
			return &m, nil
		},
		func(m *map[string]any) int64 { return int64((*m)["seq"].(float64)) },
		func(m *map[string]any) error {
			got = append(got, int64((*m)["seq"].(float64)))
			return nil
		},
	)
	if err != nil || len(got) != 1 {
		t.Fatalf("replay %v %v", got, err)
	}
}

func TestOpenJSONLineWAL_NextSeqClampAndNilFileClose(t *testing.T) {
	t.Parallel()
	w, err := openJSONLineWALWithNextSeqAndSyncDir[map[string]any](
		t.TempDir(), "ev.jsonl", ".ck", 0,
		func(string, string) (int64, error) { return 0, nil },
		func(string) error { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	if w.nextSeq != 1 {
		t.Fatalf("nextSeq=%d", w.nextSeq)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	empty := &JSONLineWAL[map[string]any]{}
	if err := empty.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestJSONLineWAL_NextSeqFnNilAndReplayEdges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	w, err := OpenJSONLineWALWithNextSeqFunc[map[string]any](root, "ev.jsonl", ".ck", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Append(func(seq int64, ts time.Time) map[string]any {
		return map[string]any{"seq": seq}
	}, 0); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	walPath := w.Path()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	last, err := ReadLastSeqFromJSONLines(walPath, 0)
	if err != nil || last < 1 {
		t.Fatalf("last=%d %v", last, err)
	}
	_, err = OpenJSONLineWALWithNextSeqFunc[map[string]any](root, "fail.jsonl", ".ck", 0, func(string, string) (int64, error) {
		return 0, errSentinelWAL{}
	})
	if err == nil {
		t.Fatal("nextSeq err")
	}
	parse := func(b []byte) (*map[string]any, error) {
		var m map[string]any
		if uerr := json.Unmarshal(b, &m); uerr != nil {
			return nil, uerr
		}
		return &m, nil
	}
	extract := func(m *map[string]any) int64 { return int64((*m)["seq"].(float64)) }
	_, err = ReplayFromCursor[map[string]any](walPath, 0, ReplayCursor{Offset: 1 << 20}, parse, extract, func(*map[string]any) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	_, err = ReplayFromCursor[map[string]any](walPath, 0, ReplayCursor{}, func([]byte) (*map[string]any, error) {
		return nil, errSentinelWAL{}
	}, extract, func(*map[string]any) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	_, err = ReplayFromCursor[map[string]any](walPath, 0, ReplayCursor{}, parse, extract, func(*map[string]any) error {
		return errSentinelWAL{}
	})
	if err == nil {
		t.Fatal("fn err")
	}
}

type errSentinelWAL struct{}

func (errSentinelWAL) Error() string { return "wal-sentinel" }
