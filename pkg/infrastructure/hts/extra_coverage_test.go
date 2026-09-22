// BLI-STARTER-COMMUNITY-032 / PRI-STARTER-COMMUNITY-032 coverage elevation
package hts

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAssemble_MissingID(t *testing.T) {
	t.Parallel()
	if _, err := Assemble(context.Background(), "CELL-x", []map[string]any{{objects.FieldKeyKind: "x"}}); err == nil {
		t.Fatal("missing id")
	}
}

func TestCellPackage_WritesJSON(t *testing.T) {
	t.Parallel()
	cell, err := Assemble(context.Background(), "CELL-pkg", []map[string]any{
		{objects.FieldKeyID: "O-1", objects.FieldKeyKind: "tool_spec"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "cell.json")
	if err := CellPackage(context.Background(), cell, out); err != nil {
		t.Fatal(err)
	}
	if !fileutil.Exists(out) {
		t.Fatal("missing package")
	}
}

func TestCodec_CompressDecompress(t *testing.T) {
	t.Parallel()
	c := NewCodec(storage.NewNoopObjectStorage())
	ctx := context.Background()
	raw := map[string]any{objects.FieldKeyTitle: "t"}
	got, err := c.Compress(ctx, raw)
	if err != nil || got[objects.FieldKeyTitle] != "t" {
		t.Fatalf("%v %v", got, err)
	}
	obj := map[string]any{
		objects.FieldKeyKind:          "tool_spec",
		objects.FieldKeyTitle:         "t",
		objects.FieldKeyDescription:   "",
		"nilv":                        nil,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	comp, err := c.Compress(ctx, obj)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := comp[objects.FieldKeySchemaVersion]; ok {
		t.Fatal("default schema should omit")
	}
	if _, ok := comp[objects.FieldKeyDescription]; ok {
		t.Fatal("empty should omit")
	}
	dec, err := c.Decompress(ctx, map[string]any{objects.FieldKeyKind: "tool_spec"})
	if err != nil {
		t.Fatal(err)
	}
	if dec[objects.FieldKeySchemaVersion] != objects.DefaultSchemaVersion {
		t.Fatalf("%v", dec)
	}
}
