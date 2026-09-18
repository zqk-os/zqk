package context

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestGetDestinations_DefaultUsesAggregateLogOnly(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	ldc := NewLoggingDecisionContext().
		WithLoggingContext(NewLoggingContext(ProfileSystem))

	dest := ldc.GetDestinations(1, tmp)
	if _, ok := dest["file"]; !ok {
		t.Fatal("expected aggregate file destination for default/system context")
	}
	if _, ok := dest["component_system"]; ok {
		t.Fatal("did not expect component file when Component is system")
	}
	if p := dest["file"].FilePath; !strings.HasSuffix(p, filepath.Join(paths.ProjectDataDir, paths.LogsDir, "log-events.json")) {
		t.Fatalf("unexpected file path: %s", p)
	}
}

func TestGetDestinations_ComponentWritesOnlyComponentFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	ldc := NewLoggingDecisionContext().
		WithLoggingContext(NewLoggingContext(ProfileSystem)).
		WithComponent("validation")

	dest := ldc.GetDestinations(1, tmp)
	if _, ok := dest["file"]; ok {
		t.Fatal("component logger should not write duplicate aggregate log-events.json")
	}
	cd, ok := dest["component_validation"]
	if !ok {
		t.Fatal("expected component_validation destination")
	}
	cd, ok = nildecode.DecodeNonNilPayload[*LogDestination](cd)
	if !ok {
		t.Fatal("expected component_validation destination")
	}
	wantSuffix := filepath.Join(paths.ProjectDataDir, paths.LogsDir, paths.ComponentsLogDir, "validation-events.json")
	if !strings.HasSuffix(cd.FilePath, wantSuffix) {
		t.Fatalf("unexpected component path: %s want suffix %s", cd.FilePath, wantSuffix)
	}
}
