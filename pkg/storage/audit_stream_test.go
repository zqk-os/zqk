package storage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAppendInstanceToStream_EmptyInputs_NoOp(t *testing.T) {
	dir := t.TempDir()

	if err := AppendInstanceToStream("", map[string]any{objects.FieldKeyKind: "audit_event", objects.FieldKeyID: "e1"}); err != nil {
		t.Errorf("empty projectRoot should be no-op, got err: %v", err)
	}
	if err := AppendInstanceToStream(dir, nil); err != nil {
		t.Errorf("nil event should be no-op, got err: %v", err)
	}
	if err := AppendInstanceToStream(dir, map[string]any{}); err != nil {
		t.Errorf("empty event should be no-op, got err: %v", err)
	}

	streamDir := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir, "audit_event")
	if _, err := fileutil.Stat(streamDir); err == nil {
		t.Errorf("expected no stream dir when inputs are empty; dir exists: %s", streamDir)
	}
}

func TestAppendInstanceToStream_AppendsOneLine(t *testing.T) {
	dir := t.TempDir()
	BuildPathAliasCacheForProject(dir)
	event := map[string]any{
		objects.FieldKeyKind:      "audit_event",
		objects.FieldKeyID:        "audit-1",
		objects.FieldKeyEventType: "object_creation",
		objects.FieldKeyCreatedAt: "2030-02-27T12:00:00Z",
	}

	err := AppendInstanceToStream(dir, event)
	if err != nil {
		t.Fatalf("AppendAuditEventToStream: %v", err)
	}

	streamDir := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir, "audit_event")
	entries, err := fileutil.ReadDir(streamDir)
	if err != nil {
		t.Fatalf("read stream dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one file in stream dir, got %d", len(entries))
	}
	fpath := filepath.Join(streamDir, entries[0].Name())
	data, err := fileutil.ReadFile(fpath)
	if err != nil {
		t.Fatalf("read stream file: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode first line: %v", err)
	}
	if decoded[objects.FieldKeyID] != "audit-1" || decoded[objects.FieldKeyEventType] != "object_creation" {
		t.Errorf("decoded event = %v", decoded)
	}
}

func TestAppendInstanceToStream_MultipleAppends_SameFile(t *testing.T) {
	dir := t.TempDir()
	BuildPathAliasCacheForProject(dir)

	for i := 0; i < 3; i++ {
		event := map[string]any{
			objects.FieldKeyKind: "audit_event",
			objects.FieldKeyID:   fmt.Sprintf("e-%d", i),
			"seq":                i,
		}
		if err := AppendInstanceToStream(dir, event); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	streamDir := filepath.Join(dir, paths.ProjectDataDir, paths.StreamsDir, "audit_event")
	entries, err := fileutil.ReadDir(streamDir)
	if err != nil {
		t.Fatalf("read stream dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one file, got %d", len(entries))
	}
	fpath := filepath.Join(streamDir, entries[0].Name())
	f, err := fileutil.Open(fpath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var lines int
	for scanner.Scan() {
		lines++
		var m map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &m); err != nil {
			t.Fatalf("line %d: %v", lines, err)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if lines != 3 {
		t.Errorf("expected 3 lines, got %d", lines)
	}
}
