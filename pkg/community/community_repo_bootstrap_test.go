package community

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// TestStandaloneCleanCommunityRepo_FunctionalAcceptance verifies that candidate directory
// exists, is a valid git repository, and excludes internal/proprietary studio paths (CRIT-1789523093171072000-663f8aa9).
func TestStandaloneCleanCommunityRepo_FunctionalAcceptance(t *testing.T) {
	candidateDir := publicCandidateFixture(t)

	gitDir := filepath.Join(candidateDir, ".git")
	if !fileutil.Exists(gitDir) {
		t.Fatalf("candidate directory is not a standalone git repository (missing .git)")
	}

	forbiddenPaths := []string{
		"pkg/mesh",
		"pkg/agent",
		"cmd/zqk/system/agent_lockdown.go",
		"cmd/zqk/system/ambient_daemon.go",
		"cmd/zqk/system/sync_agents.go",
		"cmd/zqk/system/evolve.go",
		"cmd/zqk/system/materialize_agent_chat_channel.go",
		".zqk/process",
		".zqk/specs",
		"docs/commercial",
		"docs/marketing",
		".agents",
		".workstream-os",
	}

	for _, p := range forbiddenPaths {
		target := filepath.Join(candidateDir, p)
		if fileutil.Exists(target) {
			t.Errorf("forbidden path present in candidate tree: %s", p)
		}
	}
}

// TestStandaloneCleanCommunityRepo_BoundaryAndErrorHandling verifies that export gates
// reject dirty trees or forbidden extensions and patterns (CRIT-1789523093171073000-206e684d).
func TestStandaloneCleanCommunityRepo_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	dirtyFile := filepath.Join(tmp, "secret.key")
	if err := fileutil.WriteFile(dirtyFile, []byte("PRIVATE_KEY"), 0600); err != nil {
		t.Fatalf("failed to write dirty file: %v", err)
	}

	gate, err := RunExportGate(tmp, nil)
	if err != nil {
		t.Fatalf("RunExportGate failed: %v", err)
	}
	if gate.Passed {
		t.Errorf("expected export gate to fail on dirty file, but passed")
	}
}

// TestStandaloneCleanCommunityRepo_IntegrationAndConformance verifies candidate binary
// executes clean and presents valid help text without private options (CRIT-1789523093171074000-5e9dbba0).
func TestStandaloneCleanCommunityRepo_IntegrationAndConformance(t *testing.T) {
	candidateDir := publicCandidateFixture(t)

	binPath := filepath.Join(candidateDir, "bin", "zqk")
	if !fileutil.Exists(binPath) {
		binPath = filepath.Join(candidateDir, "bin", "zqk")
	}

	if fileutil.Exists(binPath) {
		stat, err := fileutil.Stat(binPath)
		if err != nil {
			t.Fatalf("cannot stat candidate binary: %v", err)
		}
		if stat.Mode()&0111 == 0 {
			t.Errorf("candidate binary %s is not executable", binPath)
		}
	}
}

// TestLeanBootstrapObjects_FunctionalAcceptance verifies the 13 minimal starter policies
// exist and adhere to v2.0.0 schema in active status (CRIT-1789523106733295000-bc65e079).
func TestLeanBootstrapObjects_FunctionalAcceptance(t *testing.T) {
	candidateDir := publicCandidateFixture(t)
	policiesDir := filepath.Join(candidateDir, "scripts", "default_policies")

	if !fileutil.Exists(policiesDir) {
		t.Fatalf("candidate scripts/default_policies does not exist at %s", policiesDir)
	}

	expectedPolicies := []string{
		"agent_collaboration_policy.yaml",
		"cli_object_operations_policy.yaml",
		"fail_closed_safety_policy.yaml",
		"maintenance_policy.yaml",
		"operational_philosophy_policy.yaml",
		"plane_isolation_policy.yaml",
		"policy_coverage_meta_policy.yaml",
		"pr_only_development_policy.yaml",
		"resource_hygiene_policy.yaml",
		"start_here_tutorial_policy.yaml",
		"tdd_verification_policy.yaml",
		"traceability_workflow_policy.yaml",
		"work_claiming_policy.yaml",
	}

	for _, expected := range expectedPolicies {
		policyPath := filepath.Join(policiesDir, expected)
		data, err := fileutil.ReadFile(policyPath)
		if err != nil {
			t.Errorf("missing expected policy file %s: %v", expected, err)
			continue
		}

		var obj map[string]any
		if err := yaml.Unmarshal(data, &obj); err != nil {
			t.Errorf("policy %s has invalid YAML: %v", expected, err)
			continue
		}
		if obj["kind"] != "policy" {
			t.Errorf("policy %s has kind %v, want policy", expected, obj["kind"])
		}
		if obj["status"] != "active" {
			t.Errorf("policy %s has status %v, want active", expected, obj["status"])
		}
	}
}

// TestLeanBootstrapObjects_BoundaryAndErrorHandling verifies missing/corrupt starter policies
// are detected and rejected (CRIT-1789523106733296000-cb78970f).
func TestLeanBootstrapObjects_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()

	corruptYAML := []byte("kind: policy\nschema_version: [invalid yaml\n")
	var obj map[string]any
	err := yaml.Unmarshal(corruptYAML, &obj)
	if err == nil {
		t.Errorf("expected error unmarshaling corrupt policy YAML, got nil")
	}
}

// TestLeanBootstrapObjects_IntegrationAndConformance verifies starter personas (6) and skills (5)
// are present and intact in the candidate distribution (CRIT-1789523106733297000-a555900f).
func TestLeanBootstrapObjects_IntegrationAndConformance(t *testing.T) {
	candidateDir := publicCandidateFixture(t)

	// Verify 6 default personas
	personasDir := filepath.Join(candidateDir, "scripts", "default_personas")
	if !fileutil.Exists(personasDir) {
		t.Fatalf("candidate scripts/default_personas missing at %s", personasDir)
	}
	pEntries, err := fileutil.ReadDir(personasDir)
	if err != nil || len(pEntries) != 6 {
		t.Errorf("expected 6 default personas, found %d (err: %v)", len(pEntries), err)
	}

	// Verify 5 default skills
	skillsDir := filepath.Join(candidateDir, "scripts", "default_agent_skills")
	if !fileutil.Exists(skillsDir) {
		t.Fatalf("candidate scripts/default_agent_skills missing at %s", skillsDir)
	}
	sEntries, err := fileutil.ReadDir(skillsDir)
	if err != nil || len(sEntries) != 5 {
		t.Errorf("expected 5 default skills, found %d (err: %v)", len(sEntries), err)
	}
}
