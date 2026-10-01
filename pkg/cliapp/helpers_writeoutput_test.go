package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	pkgcli "github.com/zqk-os/zqk/pkg/cli"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestWriteOutput_dash_uses_command_writer_not_file(t *testing.T) {
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := pkgcli.NewCommandBuilder("x").Build()
	cmd.SetContext(ctx)
	cmd.Flags().StringP(FlagOutput, "o", "", "")
	if err := cmd.Flags().Set(FlagOutput, "-"); err != nil {
		t.Fatal(err)
	}
	if err := WriteOutput(cmd, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello" {
		t.Fatalf("expected stdout buffer, got %q", buf.String())
	}
}

func TestWriteOutput_explicit_path_writes_file(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &bytes.Buffer{})
	cmd := pkgcli.NewCommandBuilder("x").Build()
	cmd.SetContext(ctx)
	cmd.Flags().StringP(FlagOutput, "o", "", "")
	if err := cmd.Flags().Set(FlagOutput, p); err != nil {
		t.Fatal(err)
	}
	if err := WriteOutput(cmd, []byte("x")); err != nil {
		t.Fatal(err)
	}
	b, err := fileutil.ReadFile(p)
	if err != nil || string(b) != "x" {
		t.Fatalf("read %q: %v %q", p, err, b)
	}
}
