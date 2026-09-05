package storage

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestStreamCurrentPath_SanitizesID(t *testing.T) {
	root := t.TempDir()
	p := streamCurrentPath(root, "zqk_session", "ZQK-178")
	if p == emptyValue {
		t.Fatal("streamCurrentPath returned empty")
	}
	if !strings.Contains(p, "zqk_session") || !strings.Contains(p, "ZQK-178") {
		t.Errorf("path should contain kind and id: %s", p)
	}
	// ID with colon should be sanitized
	p2 := streamCurrentPath(root, "kind", "account:user")
	if strings.Contains(p2, "account:user") {
		t.Errorf("colon in id should be sanitized in path: %s", p2)
	}
}

func TestWriteAndReadStreamBackedCurrentState(t *testing.T) {
	root := t.TempDir()
	kind, id := "zqk_session", "ZQK-001"
	data := []byte("id: ZQK-001\nkind: zqk_session\nstatus: active\n")
	if err := WriteStreamBackedCurrentState(root, kind, id, data); err != nil {
		t.Fatalf("WriteStreamBackedCurrentState: %v", err)
	}
	got, ok := ReadStreamBackedCurrentState(root, kind, id)
	if !ok {
		t.Fatal("ReadStreamBackedCurrentState returned ok=false")
	}
	if !bytes.Equal(got, data) {
		t.Errorf("read back %q, want %q", got, data)
	}
	if !StreamCurrentPathExists(root, kind, id) {
		t.Error("StreamCurrentPathExists should be true after write")
	}
	overlay := GetStreamBackedObjectFilePath(root, kind, id)
	if overlay == emptyValue {
		t.Error("GetStreamBackedObjectFilePath should return path after write")
	}
	if err := RemoveStreamBackedCurrentState(root, kind, id); err != nil {
		t.Errorf("RemoveStreamBackedCurrentState: %v", err)
	}
	if _, ok := ReadStreamBackedCurrentState(root, kind, id); ok {
		t.Error("after remove, ReadStreamBackedCurrentState should return ok=false")
	}
}

func TestIsStreamCurrentPath(t *testing.T) {
	root := t.TempDir()
	kind, id := "zqk_session", "ZQK-1"
	_ = WriteStreamBackedCurrentState(root, kind, id, []byte("id: ZQK-1\n"))
	p := GetStreamBackedObjectFilePath(root, kind, id)
	if p == emptyValue {
		t.Fatal("need a path")
	}
	if !IsStreamCurrentPath(root, p) {
		t.Errorf("IsStreamCurrentPath(%q) should be true", p)
	}
	if IsStreamCurrentPath(root, "/other/path/file.yaml") {
		t.Error("unrelated path should be false")
	}
	// Path with stream_current segment (aligned with datacell.CellStreamOverlayKindDir)
	segPath := filepath.Join(datacell.CellStreamOverlayKindDir(root, "zqk_session"), "ZQK-1.yaml")
	if !IsStreamCurrentPath(root, segPath) {
		t.Errorf("IsStreamCurrentPath(%q) should be true", segPath)
	}
}

func TestComputeUpdatesMap(t *testing.T) {
	prev := map[string]any{objects.FieldKeyID: "ZQK-1", objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeyTitle: "old"}
	newObj := map[string]any{objects.FieldKeyID: "ZQK-1", objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeyTitle: "new", objects.FieldKeyUpdatedAt: "2030-03-05T12:00:00Z"}
	updates := computeUpdatesMap(prev, newObj)
	if updates == nil {
		t.Fatal("computeUpdatesMap returned nil")
	}
	if updates[objects.FieldKeyTitle] != "new" {
		t.Errorf("updates[title]=%v, want new", updates[objects.FieldKeyTitle])
	}
	if updates[objects.FieldKeyUpdatedAt] != "2030-03-05T12:00:00Z" {
		t.Errorf("updates[updated_at]=%v", updates[objects.FieldKeyUpdatedAt])
	}
	if _, ok := updates[objects.FieldKeyID]; ok {
		t.Error("id unchanged, should not be in updates")
	}
	if _, ok := updates[objects.FieldKeyStatus]; ok {
		t.Error("status unchanged, should not be in updates")
	}
}
