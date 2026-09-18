package drifthotspots

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestLoadFieldKeyWireToConstName(t *testing.T) {
	src := []byte(`
const (
	FieldKeyBatchID = "batch_id"
	FieldKeyStatus  = "status"
)
`)
	m, err := LoadFieldKeyWireToConstName(src)
	if err != nil {
		t.Fatal(err)
	}
	if m[objects.FieldKeyBatchID] != "FieldKeyBatchID" || m[objects.FieldKeyStatus] != "FieldKeyStatus" {
		t.Fatalf("map: %#v", m)
	}
}

func TestShouldScanFieldKeyLiteralPath(t *testing.T) {
	if !ShouldScanFieldKeyLiteralPath("pkg/storage/foo.go") {
		t.Fatal("expected pkg storage")
	}
	if ShouldScanFieldKeyLiteralPath("pkg/objects/field_keys.go") {
		t.Fatal("exclude field_keys")
	}
	if ShouldScanFieldKeyLiteralPath("pkg/specbuilder/bldr_instance_v1/x.go") {
		t.Fatal("exclude instance builders")
	}
}

func TestFixFieldKeyLiteralsInFile_nonObjectsPackage(t *testing.T) {
	src := []byte(`package storage

import "context"

func f(ctx context.Context) {
	m := map[string]any{"batch_id": "b1", "status": "active"}
	_ = m["title"]
	var b builder
	b.SetField("kind", "x")
}

type builder struct{}

func (builder) SetField(string, any) {}
`)
	out, n, err := FixFieldKeyLiteralsInFile("sample.go", src, map[string]string{
		objects.FieldKeyBatchID: "FieldKeyBatchID",
		objects.FieldKeyStatus:  "FieldKeyStatus",
		objects.FieldKeyTitle:   "FieldKeyTitle",
		objects.FieldKeyKind:    "FieldKeyKind",
	}, "storage")
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("replacements: want 4 got %d\n%s", n, string(out))
	}
	s := string(out)
	if !strings.Contains(s, "objects.FieldKeyBatchID") || !strings.Contains(s, `objects.FieldKeyKind`) {
		t.Fatalf("expected objects. prefix: %s", s)
	}
	if strings.Contains(s, `"batch_id"`) || strings.Contains(s, `"kind"`) {
		t.Fatalf("leftover literals: %s", s)
	}
}

func TestFixFieldKeyLiteralsInFile_objectsPackage(t *testing.T) {
	src := []byte(`package objects

func f() {
	m := map[string]any{"batch_id": "x"}
	_ = m["batch_id"]
}
`)
	out, n, err := FixFieldKeyLiteralsInFile(filepath.Join("pkg", "objects", "sample.go"), src, map[string]string{
		objects.FieldKeyBatchID: "FieldKeyBatchID",
	}, "objects")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2 got %d: %s", n, string(out))
	}
	if !strings.Contains(string(out), "FieldKeyBatchID") || strings.Contains(string(out), "objects.FieldKey") {
		t.Fatalf("objects package should not qualify: %s", string(out))
	}
}
