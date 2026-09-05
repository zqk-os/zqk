package systempeel_test

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/systempeel"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestSystemInspector(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "cmd", "zqk", "sample")
	if err := fileutil.EnsureDir(pkgDir); err != nil {
		t.Fatalf("failed to create temp pkg dir: %v", err)
	}

	f1 := filepath.Join(pkgDir, "cmd_one.go")
	f2 := filepath.Join(pkgDir, "cmd_two.go")
	if err := fileutil.WriteStandardFile(f1, []byte("package sample\n\nfunc One() {}\n")); err != nil {
		t.Fatalf("failed to write f1: %v", err)
	}
	if err := fileutil.WriteStandardFile(f2, []byte("package sample\n\nfunc Two() {}\n")); err != nil {
		t.Fatalf("failed to write f2: %v", err)
	}

	inspector := systempeel.NewSystemInspector(tempDir)
	meta, err := inspector.InspectSystemPackage(filepath.Join("cmd", "zqk", "sample"))
	if err != nil {
		t.Fatalf("InspectSystemPackage failed: %v", err)
	}

	if meta.LinesOfCode < 6 {
		t.Errorf("expected at least 6 LOC, got %d", meta.LinesOfCode)
	}
	if len(meta.Subcommands) != 2 {
		t.Errorf("expected 2 subcommands, got %d", len(meta.Subcommands))
	}
}

func TestFindFilesExceedingThreshold(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "cmd", "zqk", "large")
	if err := fileutil.EnsureDir(pkgDir); err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	largeFile := filepath.Join(pkgDir, "large_cmd.go")
	var lines string
	for i := 0; i < 50; i++ {
		lines += "// line\n"
	}
	if err := fileutil.WriteStandardFile(largeFile, []byte(lines)); err != nil {
		t.Fatalf("failed to write large file: %v", err)
	}

	inspector := systempeel.NewSystemInspector(tempDir)
	reports, err := inspector.FindFilesExceedingThreshold(filepath.Join("cmd", "zqk", "large"), 25)
	if err != nil {
		t.Fatalf("FindFilesExceedingThreshold failed: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 large file report, got %d", len(reports))
	}
	if reports[0].LinesOfCode <= 25 {
		t.Errorf("expected LOC > 25, got %d", reports[0].LinesOfCode)
	}
}

func TestPeeledComponentRegistry(t *testing.T) {
	reg := systempeel.GetGlobalRegistry()
	reg.Register("system_check", &systempeel.PeeledCommandMetadata{
		Name:        "system_check",
		PackagePath: "cmd/zqk/system",
		LinesOfCode: 1500,
	})

	meta, ok := reg.Get("system_check")
	if !ok || meta == nil {
		t.Fatalf("expected to retrieve system_check component")
	}
	if meta.LinesOfCode != 1500 {
		t.Errorf("expected LOC 1500, got %d", meta.LinesOfCode)
	}

	list := reg.List()
	if len(list) == 0 {
		t.Fatalf("expected non-empty registry list")
	}
}

func TestAttachPeeledCommand(t *testing.T) {
	parent := cli.NewCommandBuilder("parent").Build()
	child := cli.NewCommandBuilder("child").Build()

	systempeel.AttachPeeledCommand(parent, child)

	if len(parent.Commands()) != 1 {
		t.Fatalf("expected 1 child command, got %d", len(parent.Commands()))
	}
	if parent.Commands()[0].Use != "child" {
		t.Errorf("expected command 'child', got %s", parent.Commands()[0].Use)
	}
}
