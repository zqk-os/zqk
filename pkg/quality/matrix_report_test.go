package quality

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestSummarizeMatrixCSV_vettingStyle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	content := "logical_group,file_path,fully_vetted,fully_refactored_dry,fully_working,cvs_id,last_reviewed_git_sha,notes\n" +
		"pkg/a,pkg/a/a.go,pending,yes,yes,,,\n" +
		"pkg/b,pkg/b/b.go,yes,yes,yes,CONV-1,abc,\n"
	if err := os.WriteFile(csvPath, []byte(content), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	prof := &MatrixProfileYAML{}
	prof.Completion.GateColumns = []string{"fully_vetted", "fully_refactored_dry", "fully_working"}
	done := map[string]struct{}{"yes": {}, "na": {}}

	sum, err := SummarizeMatrixCSV(csvPath, prof, done, true, "cvs_id")
	if err != nil {
		t.Fatal(err)
	}
	if sum.RowTotal != 2 {
		t.Fatalf("row total: got %d want 2", sum.RowTotal)
	}
	if sum.FullyDoneRows != 1 {
		t.Fatalf("fully done: got %d want 1", sum.FullyDoneRows)
	}
}
