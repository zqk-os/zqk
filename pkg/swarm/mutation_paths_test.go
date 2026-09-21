package swarm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestMutationWriteRelPaths_keepsAnySuffixAndDropsEscapes(t *testing.T) {
	t.Parallel()
	root := "/repo"
	got := MutationWriteRelPaths([]ToolCallRecord{
		{Name: "zqk-stable_write_file", Arguments: mustPathJSON("pkg/foo/foo.go")},
		{Name: "zqk_write_code", Arguments: mustPathJSON("docs/notes.md")},
		{Name: "zqk_object_get", Arguments: `{"id":"ATK-1"}`},
		{Name: MutationEvidenceTools()[0], Arguments: mustPathJSON("/etc/evil.go")},
	}, root)
	want := "docs/notes.md,pkg/foo/foo.go"
	if strings.Join(got, ",") != want {
		t.Fatalf("paths = %v, want %s", got, want)
	}
}

func TestHistoryHasMutationWrite(t *testing.T) {
	t.Parallel()
	if HistoryHasMutationWrite([]ToolCallRecord{{Name: "zqk_object_get"}}) {
		t.Fatal("status/get is not mutation evidence")
	}
	if !HistoryHasMutationWrite([]ToolCallRecord{{Name: "zqk_write_file", Arguments: mustPathJSON("a.txt")}}) {
		t.Fatal("write_file is mutation evidence")
	}
}

func mustPathJSON(path string) string {
	b, err := json.Marshal(map[string]string{objects.FieldKeyPath: path})
	if err != nil {
		panic(err)
	}
	return string(b)
}
