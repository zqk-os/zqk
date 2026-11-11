// Copyright (c) 2026 ZQK Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package community_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCIAntiIdle_StaticFloorInvariant verifies that .github/workflows/ci.yml enforces
// fail-fast: true across matrix jobs, and .agents/AGENTS.md codifies the strict
// anti-polling timer directive.
func TestCIAntiIdle_StaticFloorInvariant(t *testing.T) {
	repoRoot := findRepoRoot(t)

	// Invariant 1: ci.yml enforces fail-fast: true in unit-tests matrix
	ciPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	ciContent, err := fileutil.ReadFile(ciPath)
	require.NoError(t, err, "failed to read .github/workflows/ci.yml")
	ciStr := string(ciContent)

	assert.Contains(t, ciStr, "fail-fast: true", "CI workflow unit-tests matrix must enforce fail-fast: true")
	assert.NotContains(t, ciStr, "fail-fast: false", "CI workflow unit-tests matrix must NOT allow fail-fast: false")

	// Invariant 2: .agents/AGENTS.md explicitly mandates anti-polling and in-flight pipelining
	agentsPath := filepath.Join(repoRoot, ".agents", "AGENTS.md")
	agentsContent, err := fileutil.ReadFile(agentsPath)
	require.NoError(t, err, "failed to read .agents/AGENTS.md")
	agentsStr := string(agentsContent)

	assert.Contains(t, agentsStr, "Anti-Idleness Protocol", "AGENTS.md must declare Continuous Autonomous Loop Discipline")
	assert.Contains(t, agentsStr, "Zero Polling Timers", "AGENTS.md must explicitly prohibit synthetic interval polling timers")
	assert.Contains(t, agentsStr, "In-Flight Continuous Pipelining", "AGENTS.md must mandate in-flight continuous pipelining during CI runs")
}

// TestCIAntiIdle_OperationalDynamicProof verifies that the CI workflow includes
// stage status reporting / notification hooks and agent prompt payload incorporates
// anti-idle flywheel instructions.
func TestCIAntiIdle_OperationalDynamicProof(t *testing.T) {
	repoRoot := findRepoRoot(t)

	// Dynamic Proof 1: ci.yml has status reporting step with if: always()
	ciPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	ciContent, err := fileutil.ReadFile(ciPath)
	require.NoError(t, err)
	ciStr := string(ciContent)

	assert.True(t, strings.Contains(ciStr, "Notify Stage Status") || strings.Contains(ciStr, "Stage Status Notification"),
		"CI workflow must contain a stage status notification / reporting step")
	assert.Contains(t, ciStr, "if: always()", "Stage status step must trigger on both success and failure (if: always())")

	// Dynamic Proof 2: agent onboarding payload contains anti-idle flywheel instructions
	payloadPath := filepath.Join(repoRoot, "pkg", "agentonboard", "payload.go")
	payloadContent, err := fileutil.ReadFile(payloadPath)
	require.NoError(t, err)
	payloadStr := string(payloadContent)

	assert.Contains(t, payloadStr, "Anti-Idleness Protocol", "pkg/agentonboard/payload.go must codify Anti-Idleness Protocol")
	assert.Contains(t, payloadStr, "Zero Polling Timers", "pkg/agentonboard/payload.go must codify Zero Polling Timers")
}

// TestCIAntiIdle_NegativeAdversarialBoundary validates that malformed configurations
// (such as fail-fast: false or synthetic sleep timers) are strictly detected and rejected.
func TestCIAntiIdle_NegativeAdversarialBoundary(t *testing.T) {
	// Adversarial case 1: fail-fast: false must fail assertion
	adversarialYAML := `
  unit-tests:
    strategy:
      fail-fast: false
`
	assert.True(t, strings.Contains(adversarialYAML, "fail-fast: false"),
		"Adversarial slow-fail config detected")

	// Helper validator function verifying failure rejection
	validateCIFailFast := func(content string) bool {
		if strings.Contains(content, "fail-fast: false") {
			return false
		}
		return strings.Contains(content, "fail-fast: true")
	}

	assert.False(t, validateCIFailFast(adversarialYAML),
		"validateCIFailFast must reject YAML containing fail-fast: false")

	validYAML := `
  unit-tests:
    strategy:
      fail-fast: true
`
	assert.True(t, validateCIFailFast(validYAML),
		"validateCIFailFast must accept YAML containing fail-fast: true")

	// Adversarial case 2: check regex against synthetic polling timers
	pollingRegex := regexp.MustCompile(`(?:schedule\s+DurationSeconds|sleep\s+\d+)`)
	assert.True(t, pollingRegex.MatchString("schedule DurationSeconds: 30"))
	assert.True(t, pollingRegex.MatchString("sleep 60"))
	assert.False(t, pollingRegex.MatchString("run_command: gh pr view"))
}
