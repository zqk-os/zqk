package agentrules

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// Correspondence tests: what the repo claims it enforces versus what it actually runs.
//
// These live together because they share one failure mode rather than one subject. A gate that is
// documented, named in a checklist, and invoked through a conditional is indistinguishable from a
// gate that runs — until you check. Two concrete instances motivated each test here:
//
//   - PRE_CHANGE_CHECKLIST and follow-up-tracked-debt.mdc both stated pre-commit enforces the
//     agent-rules manifest. Nothing invoked it, and the resolver's default pointed at .ide/rules,
//     which this repo does not have, so the documented no-flag invocation errored.
//   - d3dbb485ba deleted scripts/check_convergence_promotion_readiness.sh as collateral in an
//     orchestrator refactor while all twelve references survived, including the RunE behind
//     "zqk scheduler convergence promotion-readiness". That command returned "script not found"
//     for as long as it took to notice.
//
// They belong in this package because the manifest gate is this package's subject, and because
// the alternative was a new package holding four file-existence assertions.

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	if _, err := fileutil.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skipf("repo root not resolvable from this working directory: %v", err)
	}
	return root
}

var scriptPathRe = regexp.MustCompile(`scripts/[a-zA-Z0-9_/.-]+\.(?:sh|py)`)

// scriptPathsIn returns the distinct scripts/ paths named in a file.
func scriptPathsIn(t *testing.T, path string) []string {
	t.Helper()
	raw, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	seen := map[string]bool{}
	for _, m := range scriptPathRe.FindAllString(string(raw), -1) {
		seen[m] = true
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// TestPreCommitGates_referencedScriptsExistAndAreExecutable is the guard against silent gate
// loss. The hook chain invokes nearly every gate as `if [ -x "$REPO_ROOT/scripts/x.sh" ]; then
// run; fi`, so renaming a script, deleting it, or dropping its executable bit does not fail a
// commit — it removes the gate and the commit passes. Nothing else in the tree notices.
func TestPreCommitGates_referencedScriptsExistAndAreExecutable(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	chain := []string{
		"tools/git-hooks/pre-commit",
		"scripts/pre-commit-lint.sh",
		"scripts/pre-commit-policy.sh",
	}
	present := 0
	for _, rel := range chain {
		if _, err := fileutil.Stat(filepath.Join(root, rel)); err == nil {
			present++
		}
	}
	if present == 0 {
		t.Skip("pre-commit chain scripts absent (open-core)")
	}
	checked := 0
	for _, rel := range chain {
		p := filepath.Join(root, rel)
		if _, err := fileutil.Stat(p); err != nil {
			continue
		}
		for _, script := range scriptPathsIn(t, p) {
			sp := filepath.Join(root, script)
			info, err := fileutil.Stat(sp)
			if err != nil {
				t.Errorf("%s invokes %s, which does not exist — that gate is silently disabled "+
					"because the chain guards invocations with [ -x ] and skips what is absent", rel, script)
				continue
			}
			checked++
			if info.Mode()&0o111 == 0 {
				t.Errorf("%s invokes %s but it is not executable, so the [ -x ] guard skips it "+
					"and the gate does not run", rel, script)
			}
		}
	}
	if checked == 0 {
		t.Skip("no external gate scripts invoked by pre-commit chain in open-core (pure CLI)")
	}
	t.Logf("verified %d gate script invocations across %d chain files", checked, len(chain))
}

// TestProductionGoReferencedScripts_exist covers the class that broke the promotion-readiness
// command: production Go naming a shell script that is not in the tree. Test files are excluded
// because they use synthetic names (scripts/x.sh, scripts/wake-vendor.sh) as fixtures.
func TestProductionGoReferencedScripts_exist(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	refs := map[string][]string{} // script -> referencing files

	err := filepath.WalkDir(root, func(path string, d fileutil.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", ".zqk", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		raw, readErr := fileutil.ReadFile(path)
		if readErr != nil {
			return nil
		}
		// Quoted only: an unquoted mention in a comment is prose, not an invocation.
		for _, m := range regexp.MustCompile(`"(scripts/[a-zA-Z0-9_/.-]+\.(?:sh|py))"`).FindAllStringSubmatch(string(raw), -1) {
			rel, _ := filepath.Rel(root, path)
			refs[m[1]] = append(refs[m[1]], rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(refs) == 0 {
		t.Skip("no script references found in production Go (open-core)")
	}
	for script, from := range refs {
		if _, statErr := fileutil.Stat(filepath.Join(root, script)); statErr != nil {
			sort.Strings(from)
			t.Errorf("production Go references %s, which does not exist (referenced from %s) — "+
				"any command routed through it fails at runtime", script, strings.Join(from, ", "))
		}
	}
	t.Logf("verified %d distinct scripts referenced from production Go", len(refs))
}

// TestRuleFiles_referencedScriptsExist checks the rules an agent is told to obey against the
// tree. A rule naming a script that is not there sends every agent that reads it to run something
// nonexistent, and the rule still reads authoritative. public-push-capability.mdc listed
// scripts/install-public-push-guard.sh under "Mechanical enforcers (must stay installed)" — a
// fail-closed publication rule asserting an enforcer that was never written.
func TestRuleFiles_referencedScriptsExist(t *testing.T) {
	root := repoRoot(t)
	t.Setenv(zqkenv.AgentRulesDir().Name(), "")
	dir := ResolveRulesDir(root, "")
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		t.Skipf("rules dir unavailable: %v", err)
	}
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mdc") {
			continue
		}
		for _, script := range scriptPathsIn(t, filepath.Join(dir, e.Name())) {
			checked++
			if _, statErr := fileutil.Stat(filepath.Join(root, script)); statErr != nil {
				t.Errorf("%s tells agents to use %s, which does not exist", e.Name(), script)
			}
		}
	}
	if checked == 0 {
		t.Skip("no script references found in rule files")
	}
	t.Logf("verified %d script references across rule files", checked)
}

// TestAgentRulesGate_isWiredIntoPreCommit refuses the state this test was written in response to:
// two checklists asserting pre-commit enforces the manifest while no pre-commit script invoked it.
// Documentation cannot enforce anything; only the hook chain can.
func TestAgentRulesGate_isWiredIntoPreCommit(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	lint := filepath.Join(root, "scripts", "pre-commit-lint.sh")
	raw, err := fileutil.ReadFile(lint)
	if err != nil {
		t.Skipf("scripts/pre-commit-lint.sh absent (open-core): %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, "validate-agent-rules") {
		t.Error("scripts/pre-commit-lint.sh does not invoke validate-agent-rules, but " +
			"PRE_CHANGE_CHECKLIST.md and follow-up-tracked-debt.mdc both state pre-commit " +
			"enforces the agent rules manifest; either wire the gate or stop claiming it")
	}
	// Spawned, not merely defined: a job function nothing schedules is the same as no gate.
	if !strings.Contains(body, "_pcommit_spawn agent-rules") {
		t.Error("validate-agent-rules is named in pre-commit-lint.sh but no _pcommit_spawn " +
			"schedules it, so the job is defined and never runs")
	}
}

// TestRulesDir_documentedNoFlagInvocationResolves pins that `zqk system validate-agent-rules`
// works from the repo root with no flags, which is what PRE_CHANGE_CHECKLIST tells agents to run
// ("normally two commands with no other flags") and what the wired gate above depends on.
func TestRulesDir_documentedNoFlagInvocationResolves(t *testing.T) {
	root := repoRoot(t)
	// Empty reads as unset in ResolveRulesDir, so this exercises the candidate search rather
	// than whatever the developer's environment happens to name.
	t.Setenv(zqkenv.AgentRulesDir().Name(), "")
	dir := ResolveRulesDir(root, "")
	info, err := fileutil.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Skipf("no-flag ResolveRulesDir returned %s, which is not a directory (%v); skipped in open-core", dir, err)
	}
	if err := Validate(root, ""); err != nil {
		t.Errorf("no-flag Validate failed: %v", err)
	}
}
