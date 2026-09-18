package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	clitool "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/quality"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestMatrixUpdate_flagValidation covers early validation that does not require registry CSV/profile.
func TestMatrixUpdate_flagValidation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmp)
	testkit.RegisterStandardTeardown(t, testkit.TempProjectTeardown(tmp, nil))

	tests := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{
			name:    "cvs_id_requires_append",
			args:    []string{"matrix", "update", "--cvs-id", "CVS-1"},
			wantSub: "--cvs-id requires --append-cvs-activity",
		},
		{
			name:    "append_cvs_incompatible_with_dry_run",
			args:    []string{"matrix", "update", "--append-cvs-activity", "--dry-run"},
			wantSub: "--append-cvs-activity cannot be used with --dry-run",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := clitool.NewCommandBuilder("zqk").Build()
			root.AddCommand(NewMatrixCmd())
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(tt.args)
			err := root.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Fatalf("error %q should contain %q", err.Error(), tt.wantSub)
			}
		})
	}
}

// TestMatrixUpdate_dryRunSetParsesStringArray regressions: --set must use GetStringArray (string_array spec),
// otherwise values are empty and the row is unchanged.
func TestMatrixUpdate_dryRunSetParsesStringArray(t *testing.T) {
	isolatedMatrixProject(t, "file_path,fully_vetted\npkg/x.go,pending\n")

	var buf bytes.Buffer
	root := clitool.NewCommandBuilder("zqk").Build()
	root.AddCommand(NewMatrixCmd())
	root.SetOut(&buf)
	root.SetErr(io.Discard)
	root.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	root.SetArgs([]string{
		"matrix", "update", "--name", "t",
		"--dry-run", "--file-path", "pkg/x.go", "--set", "fully_vetted=yes",
		"--format", "json",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got quality.MatrixUpdateResult
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\nout=%s", err, buf.String())
	}
	if got.RowAfter == nil || got.RowAfter["fully_vetted"] != "yes" {
		t.Fatalf("expected row_after.fully_vetted=yes, got %#v", got.RowAfter)
	}
}
