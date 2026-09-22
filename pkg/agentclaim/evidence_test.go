// BLI-STARTER-COMMUNITY-017 / PRI-STARTER-COMMUNITY-048
// Git-evidence gate: trunk-tip freshness must fail closed unless the test bypass is set.
package agentclaim

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestEnforceTrunkTipFreshness_GitEvidenceFailClosed(t *testing.T) {
	ctx := context.Background()
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "1")
	if err := enforceTrunkTipFreshness(ctx, t.TempDir()); err != nil {
		t.Fatalf("bypass must skip git: %v", err)
	}
	t.Setenv(zqkenv.TestBypassGitevidence().Name(), "")
	if err := enforceTrunkTipFreshness(ctx, t.TempDir()); err == nil {
		t.Fatal("missing git repo must fail closed")
	}
}
