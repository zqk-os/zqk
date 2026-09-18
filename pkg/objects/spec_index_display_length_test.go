package objects

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSpecIndexKeepsDisplayLengthSeparateFromStorageLength(t *testing.T) {
	t.Parallel()

	specsDir := t.TempDir()
	const specYAML = `ontology: display_contract
schema_version: "2.0.0"
visibility: internal
fields:
  title:
    type: string
    validation:
      min_length: 5
      max_length: 200
      display_length: 120
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "display_contract.yaml"), []byte(specYAML), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	idx, err := BuildSpecIndexFromSpecsDir(specsDir)
	if err != nil {
		t.Fatal(err)
	}
	fields := idx.Kinds["display_contract"].Fields
	if len(fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(fields))
	}
	got := fields[0]
	if got.MinLength != 5 || got.MaxLength != 200 || got.DisplayLength != 120 {
		t.Fatalf("length contract = min:%d max:%d display:%d", got.MinLength, got.MaxLength, got.DisplayLength)
	}
}
