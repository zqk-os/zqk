package cli

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// loadShippedCommandTimeouts reads the repo's config/command_timeouts.yaml, which is the default
// consulted when a project has no .zqk/config override.
func loadShippedCommandTimeouts(t *testing.T) commandTimeoutsConfig {
	t.Helper()
	path := filepath.Join("..", "..", "config", paths.CommandTimeoutsConfigFile)
	raw, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var cfg commandTimeoutsConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(cfg.Rules) == 0 {
		t.Fatalf("%s declared no rules", path)
	}
	return cfg
}

func parseRuleDuration(t *testing.T, field, pattern, value string) time.Duration {
	t.Helper()
	if value == "" {
		return 0
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		t.Fatalf("rule %q has unparseable %s %q: %v", pattern, field, value, err)
	}
	return d
}

// TestCommandTimeouts_generalFallbackIsNotStricterThanTheSpecificRuleItCatches pins the
// consequence of first-match-wins substring matching: when a specific pattern is followed by a
// general one that is a substring of it, the general rule is the fallback for every invocation of
// the same command that happens not to spell the specific flag. If that fallback is *stricter*,
// then work of identical cost is killed sooner purely because of how the request was written.
//
// This is not hypothetical. "scheduler scan-tests --all" carried 20m plus a child-cap exemption
// and a 5m idle window, while the bare "scheduler scan-tests" fallback carried 10m and neither.
// Scan cost is driven by how many bundles get generated — one scheduler_job object written per
// bundle — and a --package list of 45 packages generates as many as --all does. Such a scan was
// killed at 10m mid-generation and reported a timeout indistinguishable from a hang.
//
// A pattern cannot count packages, so it cannot tell a large scan from a small one. The only
// sound rule is that the fallback may not be tighter than the specific case it shadows.
func TestCommandTimeouts_generalFallbackIsNotStricterThanTheSpecificRuleItCatches(t *testing.T) {
	cfg := loadShippedCommandTimeouts(t)

	for i, specific := range cfg.Rules {
		for _, general := range cfg.Rules[i+1:] {
			// Only compare rules for the same command family, where the later pattern is a
			// strict generalization of the earlier one and thus shadows it as the fallback.
			if general.Pattern == specific.Pattern || !strings.Contains(specific.Pattern, general.Pattern) {
				continue
			}

			specificTimeout := parseRuleDuration(t, "timeout", specific.Pattern, specific.Timeout)
			generalTimeout := parseRuleDuration(t, "timeout", general.Pattern, general.Timeout)
			if generalTimeout < specificTimeout {
				t.Errorf("fallback %q allows %v but the specific %q it shadows allows %v; "+
					"an invocation doing the same work is killed sooner for omitting a flag",
					general.Pattern, generalTimeout, specific.Pattern, specificTimeout)
			}

			if specific.ChildMaxTimeoutExempt && !general.ChildMaxTimeoutExempt {
				t.Errorf("specific %q is exempt from the child-max-timeout cap but fallback %q is not; "+
					"the cap would apply to the same work when invoked without that flag",
					specific.Pattern, general.Pattern)
			}

			specificIdle := parseRuleDuration(t, "idle_shutdown_duration", specific.Pattern, specific.IdleShutdownDuration)
			generalIdle := parseRuleDuration(t, "idle_shutdown_duration", general.Pattern, general.IdleShutdownDuration)
			if generalIdle < specificIdle {
				t.Errorf("specific %q gets a %v idle window but fallback %q gets %v; "+
					"the same bulk writes would trip the idle watchdog when invoked without that flag",
					specific.Pattern, specificIdle, general.Pattern, generalIdle)
			}
		}
	}
}

// TestCommandTimeouts_scanTestsRulesAgreeOnScale anchors the specific case above so the
// regression that motivated the invariant stays named even if the rules are reorganized.
func TestCommandTimeouts_scanTestsRulesAgreeOnScale(t *testing.T) {
	cfg := loadShippedCommandTimeouts(t)

	var found []commandTimeoutRule
	for _, r := range cfg.Rules {
		if strings.Contains(r.Pattern, "scheduler scan-tests") {
			found = append(found, r)
		}
	}
	if len(found) == 0 {
		t.Fatal("no scheduler scan-tests timeout rule found; scans would fall back to default_max")
	}

	want := found[0]
	for _, got := range found[1:] {
		if got.Timeout != want.Timeout {
			t.Errorf("scan-tests rules disagree on timeout: %q=%s vs %q=%s (bundle count, not flag spelling, drives cost)",
				want.Pattern, want.Timeout, got.Pattern, got.Timeout)
		}
		if got.ChildMaxTimeoutExempt != want.ChildMaxTimeoutExempt {
			t.Errorf("scan-tests rules disagree on child_max_timeout_exempt: %q=%v vs %q=%v",
				want.Pattern, want.ChildMaxTimeoutExempt, got.Pattern, got.ChildMaxTimeoutExempt)
		}
		if got.IdleShutdownDuration != want.IdleShutdownDuration {
			t.Errorf("scan-tests rules disagree on idle_shutdown_duration: %q=%s vs %q=%s",
				want.Pattern, want.IdleShutdownDuration, got.Pattern, got.IdleShutdownDuration)
		}
	}
}
