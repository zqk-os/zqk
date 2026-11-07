package bundle

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLanguageBundle_StaticFloorInvariant(t *testing.T) {
	manifestYAML := `
schema_version: "2.0.0"
name: "lang-go"
language: "go"
version: "1.0.0"
description: "Core Go architecture standards, AST hygiene, and verification gates"
standards:
  - id: "STD-GO-CONCURRENCY"
    title: "Context Propagation Invariant"
    description: "Every long-running function must propagate context.Context as first argument"
    severity: "critical"
ast_policies:
  - id: "POL-GO-AST-NO-RAW-SLEEP"
    rule_type: "ast_forbidden_call"
    target_kind: "func_call"
    pattern: "time.Sleep"
gates:
  - name: "zqk-vet"
    command: "./bin/zqk vet"
    enforce_ci: true
agent_directives:
  - target_role: "software_engineer"
    directive: "Always write unit tests first before writing logic (TDD)"
    path: ".agents/rules/go.md"
`
	m, err := ParseManifestBytes([]byte(manifestYAML))
	require.NoError(t, err)
	assert.Equal(t, "lang-go", m.Name)
	assert.Equal(t, "go", m.Language)
	assert.Len(t, m.Standards, 1)
	assert.Len(t, m.ASTPolicies, 1)
	assert.Len(t, m.Gates, 1)
	assert.Len(t, m.AgentDirectives, 1)

	// Invariant: missing required fields rejected
	invalid := &LanguageBundleManifest{Name: "incomplete"}
	assert.Error(t, invalid.Validate())
}

func TestLanguageBundle_OperationalDynamicProof(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewRegistry(tmpDir)

	m := &LanguageBundleManifest{
		SchemaVersion: "2.0.0",
		Name:          "lang-python",
		Language:      "python",
		Version:       "0.1.0",
		Description:   "Python standards pack",
		AgentDirectives: []DirectiveSpec{
			{
				TargetRole: "software_engineer",
				Directive:  "Follow PEP-8 and use type annotations everywhere",
				Path:       ".agents/rules/python.md",
			},
		},
	}

	err := reg.Install(m)
	require.NoError(t, err)

	// Verify installed listing
	list, err := reg.ListInstalled()
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "lang-python", list[0].Name)

	// Verify directive file seeded
	seededPath := filepath.Join(tmpDir, ".agents/rules/python.md")
	content, err := fileutil.ReadFile(seededPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "PEP-8")

	// Get by ID
	retrieved, err := reg.Get("lang-python")
	require.NoError(t, err)
	assert.Equal(t, "python", retrieved.Language)

	// Uninstall
	err = reg.Remove("lang-python")
	require.NoError(t, err)
	listAfter, err := reg.ListInstalled()
	require.NoError(t, err)
	assert.Empty(t, listAfter)
}

func TestLanguageBundle_NegativeAdversarialBoundary(t *testing.T) {
	// 1. Path traversal in bundle name
	m1 := &LanguageBundleManifest{
		SchemaVersion: "2.0.0",
		Name:          "../../malicious",
		Language:      "go",
		Version:       "1.0.0",
	}
	assert.Error(t, m1.Validate())

	// 2. Path traversal in agent directive path
	m2 := &LanguageBundleManifest{
		SchemaVersion: "2.0.0",
		Name:          "pack-test",
		Language:      "go",
		Version:       "1.0.0",
		AgentDirectives: []DirectiveSpec{
			{
				TargetRole: "software_engineer",
				Directive:  "payload",
				Path:       "../../../../etc/passwd",
			},
		},
	}
	assert.Error(t, m2.Validate())

	// 3. Circular self-dependency
	m3 := &LanguageBundleManifest{
		SchemaVersion: "2.0.0",
		Name:          "lang-ts",
		Language:      "typescript",
		Version:       "1.0.0",
		Dependencies:  []string{"lang-ts"},
	}
	assert.Error(t, m3.Validate())
}
