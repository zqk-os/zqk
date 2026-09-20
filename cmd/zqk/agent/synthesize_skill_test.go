package agent

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage/audit"
)

func TestSynthesizeSkillCommitContext_SetsPromoteOnCreate(t *testing.T) {
	t.Parallel()

	cliCtx := audit.WithCLIOperation(pkgctx.NewSystemContext())
	got := synthesizeSkillCommitContext(cliCtx)
	if !pkgctx.GetPromoteOnCreate(got) {
		t.Fatal("COMMIT Create ctx must set WithPromoteOnCreate (same as object create --promote)")
	}
	if !audit.HasCLIMarker(got) {
		t.Fatal("COMMIT Create ctx must keep the CLI marker from OperationContext")
	}
}

func TestSynthesizeSkillCommitContext_NilFallsBackToSystem(t *testing.T) {
	t.Parallel()

	got := synthesizeSkillCommitContext(nil)
	if !pkgctx.GetPromoteOnCreate(got) {
		t.Fatal("nil op ctx must still set WithPromoteOnCreate")
	}
}
