// BLI-STARTER-COMMUNITY-026 / PRI-STARTER-COMMUNITY-026 coverage elevation
package integrity

import (
	"github.com/zqk-os/zqk/pkg/paths"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTDE_NilAndDefaults(t *testing.T) {
	t.Parallel()
	if err := ValidateTDE(nil); err == nil {
		t.Fatal("nil tde")
	}
	tde := &TaskDefEnvelope{ID: "ATK-1", Task: "do work"}
	if err := ValidateTDE(tde); err != nil {
		t.Fatal(err)
	}
	if tde.Kind != "agent_task" || tde.Status != "pending" {
		t.Fatalf("defaults: %+v", tde)
	}
	keep := &TaskDefEnvelope{ID: "ATK-2", Task: "x", Kind: "custom", Status: "running"}
	if err := ValidateTDE(keep); err != nil {
		t.Fatal(err)
	}
	if keep.Kind != "custom" || keep.Status != "running" {
		t.Fatalf("must not overwrite: %+v", keep)
	}
}

func TestValidateBLI(t *testing.T) {
	t.Parallel()
	if err := ValidateBLI("", "", ""); err == nil {
		t.Fatal("empty id")
	}
	if err := ValidateBLI("REQ-1", "backlog_item", "planned"); err == nil {
		t.Fatal("wrong prefix")
	}
	if err := ValidateBLI("BLI-1", "backlog_item", "planned"); err != nil {
		t.Fatal(err)
	}
}

func TestPrefixFromID_NoHyphen(t *testing.T) {
	t.Parallel()
	if got := prefixFromID("NOHYPHEN"); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := prefixFromID("BLI-abc"); got != "BLI-" {
		t.Fatalf("got %q", got)
	}
}

func TestTDEWithDefaults(t *testing.T) {
	t.Parallel()
	nilOut := TDEWithDefaults(nil)
	if nilOut.Kind != "agent_task" || nilOut.Status != "pending" {
		t.Fatalf("nil defaults: %+v", nilOut)
	}
	in := &TaskDefEnvelope{Kind: "keep"}
	out := TDEWithDefaults(in)
	if out != in || out.Kind != "keep" || out.Status != "pending" {
		t.Fatalf("got %+v", out)
	}
}

func TestBuildIntegrityProof_SkipsEmpty(t *testing.T) {
	t.Parallel()
	if n := BuildIntegrityProof([]*Skill{nil, {}}); n == nil || len(n) != 0 {
		t.Fatalf("got %#v", n)
	}
	s := &Skill{ID: "ASK-1", Name: "n", Kind: "skill", Status: "active"}
	got := BuildIntegrityProof([]*Skill{s})
	if len(got) != 1 || got[0].ID != "ASK-1" || len(got[0].Hash) == 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestVerifySkillHash_UnknownID(t *testing.T) {
	t.Parallel()
	if err := VerifySkillHash("ASK-MISSING", []byte("x")); err == nil {
		t.Fatal("unknown skill")
	}
}

func TestCollectWorkspaceEvidence_Hints(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "subdir"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("ok"), paths.FilePerm600); err != nil {
		t.Fatal(err)
	}
	got, err := CollectWorkspaceEvidence(root, []EvidenceHint{
		{Kind: EviDirExists, Path: "subdir", Timestamp: true},
		{Kind: EviDirExists, Path: "nope"},
		{Kind: EviFileExists, Path: "file.txt"},
		{Kind: EviFileExists, Path: "missing.txt"},
		{Kind: EviGoVersion},
		{Kind: EviGitStatus},
		{Kind: EviDirExists}, // empty path
		{Kind: EviFileExists},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got[EviDirExists].Present {
		t.Fatalf("dir: %+v", got[EviDirExists])
	}
	if !got[EviFileExists].Present {
		t.Fatalf("file: %+v", got[EviFileExists])
	}
	if !got[EviGoVersion].Present || got[EviGoVersion].Value == "" {
		t.Fatalf("go version: %+v", got[EviGoVersion])
	}
	if got[EviGitStatus] != nil {
		t.Fatalf("unknown kind should be nil: %+v", got[EviGitStatus])
	}
}
