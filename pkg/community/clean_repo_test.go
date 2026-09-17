package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// TestCommunityCleanRepoCandidate verifies candidate directory exists, is a valid git repo, and excludes studio paths.
func TestCommunityCleanRepoCandidate(t *testing.T) {
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

// TestPoliceAndPayloadGates verifies candidate satisfies police-community-tree.sh and check-public-release-payload.sh.
func TestPoliceAndPayloadGates(t *testing.T) {
	root := paths.ResolveProjectRoot(".")
	candidateDir := publicCandidateFixture(t)

	policeScript := filepath.Join(root, "scripts", "open-core", "police-community-tree.sh")
	cmdPolice := execwrap.Command("sh", policeScript, candidateDir)
	outPolice, errPolice := cmdPolice.CombinedOutput()
	if errPolice != nil {
		t.Fatalf("police-community-tree.sh failed: %v\nOutput: %s", errPolice, string(outPolice))
	}
	if !strings.Contains(string(outPolice), "POLICE: PASS") {
		t.Fatalf("police-community-tree.sh did not output POLICE: PASS: %s", string(outPolice))
	}

	payloadScript := filepath.Join(root, "scripts", "check-public-release-payload.sh")
	cmdPayload := execwrap.Command("sh", payloadScript, candidateDir)
	outPayload, errPayload := cmdPayload.CombinedOutput()
	if errPayload != nil {
		t.Fatalf("check-public-release-payload.sh failed: %v\nOutput: %s", errPayload, string(outPayload))
	}
	if !strings.Contains(string(outPayload), "RESULT=PASS") {
		t.Fatalf("check-public-release-payload.sh did not output RESULT=PASS: %s", string(outPayload))
	}
}

// TestCandidateBinaryHelp verifies candidate builds standalone and runs --help with exit code 0.
func TestCandidateBinaryHelp(t *testing.T) {
	candidateDir := publicCandidateFixture(t)
	binPath := filepath.Join(candidateDir, "bin", "zqk-community")

	if !fileutil.Exists(binPath) {
		cmdBuild := execwrap.Command("go", "build", "-buildvcs=false", "-o", binPath, "./cmd/zqk-community")
		cmdBuild.Dir = candidateDir
		outBuild, errBuild := cmdBuild.CombinedOutput()
		if errBuild != nil {
			t.Fatalf("failed to build candidate binary: %v\nOutput: %s", errBuild, string(outBuild))
		}
	}

	cmdHelp := execwrap.Command(binPath, "--help")
	outHelp, errHelp := cmdHelp.CombinedOutput()
	if errHelp != nil {
		t.Fatalf("candidate binary --help failed: %v\nOutput: %s", errHelp, string(outHelp))
	}

	helpStr := string(outHelp)
	if !strings.Contains(helpStr, "Available Commands") && !strings.Contains(helpStr, "Usage:") {
		t.Fatalf("unexpected help output from candidate binary:\n%s", helpStr)
	}
}

// TestCommunityDefaultPoliciesIntegrity verifies that all 13 canonical starter policies exist and conform to schema.
func TestCommunityDefaultPoliciesIntegrity(t *testing.T) {
	candidateDir := publicCandidateFixture(t)
	policiesDir := filepath.Join(candidateDir, "scripts", "default_policies")

	if !fileutil.Exists(policiesDir) {
		t.Fatalf("candidate scripts/default_policies does not exist at %s", policiesDir)
	}

	entries, err := fileutil.ReadDir(policiesDir)
	if err != nil {
		t.Fatalf("failed to read policies dir: %v", err)
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

	if len(entries) != len(expectedPolicies) {
		t.Errorf("expected %d policies, found %d", len(expectedPolicies), len(entries))
	}

	for _, expected := range expectedPolicies {
		policyPath := filepath.Join(policiesDir, expected)
		data, readErr := fileutil.ReadFile(policyPath)
		if readErr != nil {
			t.Errorf("missing expected policy file %s: %v", expected, readErr)
			continue
		}

		var obj map[string]any
		if yamlErr := yaml.Unmarshal(data, &obj); yamlErr != nil {
			t.Errorf("policy %s is invalid YAML: %v", expected, yamlErr)
			continue
		}

		if obj["kind"] != "policy" {
			t.Errorf("policy %s has kind %v, want policy", expected, obj["kind"])
		}
		if obj["schema_version"] != "2.0.0" {
			t.Errorf("policy %s has schema_version %v, want 2.0.0", expected, obj["schema_version"])
		}
		if obj["status"] != "active" {
			t.Errorf("policy %s has status %v, want active", expected, obj["status"])
		}
		if title, _ := obj["title"].(string); title == "" {
			t.Errorf("policy %s missing title", expected)
		}
	}
}

// TestCommunityDefaultPersonasAndSkillsIntegrity verifies the 6 default personas and 5 default skills in candidate tree.
func TestCommunityDefaultPersonasAndSkillsIntegrity(t *testing.T) {
	candidateDir := publicCandidateFixture(t)

	// Personas
	personasDir := filepath.Join(candidateDir, "scripts", "default_personas")
	if !fileutil.Exists(personasDir) {
		t.Fatalf("candidate scripts/default_personas does not exist at %s", personasDir)
	}
	personaEntries, err := fileutil.ReadDir(personasDir)
	if err != nil {
		t.Fatalf("failed to read default_personas dir: %v", err)
	}
	if len(personaEntries) != 6 {
		t.Errorf("expected 6 default personas, found %d", len(personaEntries))
	}

	// Skills
	skillsDir := filepath.Join(candidateDir, "scripts", "default_agent_skills")
	if !fileutil.Exists(skillsDir) {
		t.Fatalf("candidate scripts/default_agent_skills does not exist at %s", skillsDir)
	}
	skillEntries, err := fileutil.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("failed to read default_agent_skills dir: %v", err)
	}
	if len(skillEntries) != 5 {
		t.Errorf("expected 5 default skills, found %d", len(skillEntries))
	}
}
