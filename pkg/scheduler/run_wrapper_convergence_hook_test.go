package scheduler

import "testing"

func TestConvergenceTickEnvKeysForwardedFromTestBundleJob_convergenceKeys(t *testing.T) {
	t.Parallel()
	want := []string{
		EnvKeyConvergenceTickSpawnFollowupDraft,
		EnvKeyConvergenceTickRollup,
		EnvKeyCVSOrchestrateRollupOut,
	}
outer:
	for _, id := range want {
		for _, k := range convergenceTickEnvKeysForwardedFromTestBundleJob {
			if k == id {
				continue outer
			}
		}
		t.Fatalf("expected %q in convergenceTickEnvKeysForwardedFromTestBundleJob", id)
	}
}
