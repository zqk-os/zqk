package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	clitool "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/quality"
)

// TestMatrixGet_filterAndFieldParsesStringArray regressions: --filter and --field use string_array in specs;
// handlers must call GetStringArray or repeated flags are ignored.
func TestMatrixGet_filterAndFieldParsesStringArray(t *testing.T) {
	isolatedMatrixProject(t, "file_path,fully_vetted\npkg/a.go,pending\npkg/b.go,yes\n")

	var buf bytes.Buffer
	root := clitool.NewCommandBuilder("zqk").Build()
	root.AddCommand(NewMatrixCmd())
	root.SetOut(&buf)
	root.SetErr(io.Discard)
	root.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	root.SetArgs([]string{
		"matrix", "get", "--name", "t",
		"--filter", "fully_vetted=pending",
		"--field", "file_path",
		"--limit", "1",
		"--format", "json",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got quality.MatrixGetResult
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\nout=%s", err, buf.String())
	}
	if got.RowCount != 1 || len(got.Rows) != 1 {
		t.Fatalf("want 1 row, got row_count=%d rows=%d", got.RowCount, len(got.Rows))
	}
	if len(got.Header) != 1 || got.Header[0] != "file_path" {
		t.Fatalf("header: %#v", got.Header)
	}
	if got.Rows[0][objects.FieldKeyFilePath] != "pkg/a.go" {
		t.Fatalf("row: %#v", got.Rows[0])
	}
}
