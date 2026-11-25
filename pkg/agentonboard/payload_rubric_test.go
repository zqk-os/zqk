package agentonboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBootPayload_IncludesZgrepDecisionRubric(t *testing.T) {
	t.Parallel()

	payload := BootPayload()
	assert.Contains(t, payload, "Decision Rubric (When to Use vs. When NOT to Use)")
	assert.Contains(t, payload, "MUST use")
	assert.Contains(t, payload, "Do NOT use `zgrep`")
	assert.Contains(t, payload, "--max-tokens 2000 -f json")
}
