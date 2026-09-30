package bldr_cli_cmd_v1

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/storage/crud"
)

func TestObjectCreate_RelaxedFlagDefaultFalse(t *testing.T) {
	cmd := NewObjectCreateCommandBuilder()
	if cmd == nil {
		t.Fatal("expected object create command to not be nil")
	}
	f := cmd.Flags().Lookup("relaxed")
	if f == nil {
		t.Fatal("expected flag --relaxed to exist on object create command")
	}
	if f.DefValue != "false" {
		t.Errorf("expected --relaxed default value to be 'false' for fail-closed safety, got %q", f.DefValue)
	}
}

func TestObjectCreate_KeepFileHelpTruthfulness(t *testing.T) {
	cmd := NewObjectCreateCommandBuilder()
	if cmd == nil {
		t.Fatal("expected object create command to not be nil")
	}
	f := cmd.Flags().Lookup("keep-file")
	if f == nil {
		t.Fatal("expected flag --keep-file to exist on object create command")
	}
	if !strings.Contains(f.Usage, "tmp-*.yaml") {
		t.Errorf("expected --keep-file usage to clarify temporary scratch files, got %q", f.Usage)
	}
}

func TestStorageError_NoRawStreamTokenLeak(t *testing.T) {
	// Verify that constant strings used in file read do not leak internal SCREAMING_SNAKE tokens
	if strings.Contains(crud.ConstStreamCouldNotInferKindFromIdStr, "STREAM_") {
		t.Errorf("ConstStreamCouldNotInferKindFromIdStr leaks internal token: %q", crud.ConstStreamCouldNotInferKindFromIdStr)
	}
	if strings.Contains(crud.ConstStreamFailedToLoadIdPatterns, "STREAM_") {
		t.Errorf("ConstStreamFailedToLoadIdPatterns leaks internal token: %q", crud.ConstStreamFailedToLoadIdPatterns)
	}
	if strings.Contains(crud.ConstStreamFailedToUnmarshalPendingObject, "STREAM_") {
		t.Errorf("ConstStreamFailedToUnmarshalPendingObject leaks internal token: %q", crud.ConstStreamFailedToUnmarshalPendingObject)
	}
}

func TestDocumentation_OperatorSwarmLinkResolves(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "quality", "codebase_evaluation", "OPERATOR.md"))
	if err != nil {
		t.Skipf("skipping doc link test if not running from repo tree: %v", err)
	}
	text := string(content)
	if strings.Contains(text, "](../../packs/code-eval/swarm.yaml)") {
		t.Errorf("OPERATOR.md contains broken relative link with insufficient depth: ../../packs/code-eval/swarm.yaml")
	}
	if !strings.Contains(text, "](../../../packs/code-eval/swarm.yaml)") {
		t.Errorf("OPERATOR.md missing canonical link: ../../../packs/code-eval/swarm.yaml")
	}
}

func TestMatrixCommand_ShortDescription(t *testing.T) {
	cmd := NewMatrixCommandBuilder()
	if cmd == nil {
		t.Fatal("expected matrix command to not be nil")
	}
	if strings.HasPrefix(cmd.Short, "Generated spec") {
		t.Errorf("expected matrix command to have non-tautological short description, got %q", cmd.Short)
	}
}
