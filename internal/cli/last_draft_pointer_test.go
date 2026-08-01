package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestWriteReadClearLastDraftPointer(t *testing.T) {
	tmp := t.TempDir()
	draftDir := filepath.Join(tmp, paths.ProjectDataDir, paths.DraftsDir)
	if err := os.MkdirAll(draftDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	draftPath := filepath.Join(draftDir, objects.KindBacklogItem+"-20060102-120000.yaml")
	if err := os.WriteFile(draftPath, []byte("kind: "+objects.KindBacklogItem+"\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	if err := WriteLastDraftPointer(tmp, LastDraftScopeObject, objects.KindBacklogItem, draftPath); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLastDraftPointer(tmp)
	if err != nil || got == nil {
		t.Fatalf("read pointer: %v %v", got, err)
	}
	if got.Scope != LastDraftScopeObject || got.Kind != objects.KindBacklogItem {
		t.Fatalf("pointer: %+v", got)
	}

	resolved, err := ResolveLastDraftFile(tmp, LastDraftScopeObject, objects.KindBacklogItem)
	if err != nil || resolved == emptyValue {
		t.Fatalf("resolve: %q %v", resolved, err)
	}

	if err := ClearLastDraftPointerIfPath(tmp, resolved); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(LastDraftPointerPath(tmp)); !os.IsNotExist(err) {
		t.Fatalf("expected pointer removed: %v", err)
	}
}

func TestResolveLastDraftFile_wrongKind(t *testing.T) {
	tmp := t.TempDir()
	draftDir := filepath.Join(tmp, paths.ProjectDataDir, paths.DraftsDir)
	if err := os.MkdirAll(draftDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	draftPath := filepath.Join(draftDir, "x.yaml")
	if err := os.WriteFile(draftPath, []byte("kind: "+objects.KindBacklogItem+"\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := WriteLastDraftPointer(tmp, LastDraftScopeObject, objects.KindBacklogItem, draftPath); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveLastDraftFile(tmp, LastDraftScopeObject, objects.KindGoal)
	if err == nil {
		t.Fatal("expected kind mismatch error")
	}
}
