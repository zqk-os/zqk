package scheduler

import "testing"

func TestJobEnvAffirmative_convergenceTickRollup(t *testing.T) {
	t.Parallel()
	job := func(env string) *ScheduledJob {
		return &ScheduledJob{
			EnvironmentVariables: map[string]string{
				EnvKeyConvergenceTickRollup: env,
			},
		}
	}
	for _, tc := range []struct {
		env    string
		enable bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
	} {
		tc := tc
		t.Run(tc.env, func(t *testing.T) {
			t.Parallel()
			got := jobEnvAffirmative(job(tc.env), EnvKeyConvergenceTickRollup)
			if got != tc.enable {
				t.Fatalf("env %q: got %v want %v", tc.env, got, tc.enable)
			}
		})
	}
}
