package agentprompt

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestFormatTaskStepsPromptSection_ExplicitSteps(t *testing.T) {
	t.Parallel()

	steps := []map[string]any{
		{
			objects.FieldKeyTitle:       "Inspect files",
			objects.FieldKeyDescription: "Read target source file",
			objects.FieldKeyStatus:      "pending",
		},
		{
			objects.FieldKeyTitle:       "Apply mutation",
			objects.FieldKeyDescription: "Modify parser logic",
			objects.FieldKeyCommand:     "go test ./pkg/...",
		},
	}

	got := FormatTaskStepsPromptSection(steps, WorkClassCoding)
	if !strings.Contains(got, "Sequenced Task Steps") {
		t.Fatalf("expected header in output, got: %s", got)
	}
	if !strings.Contains(got, "1. **Inspect files** [pending]") {
		t.Fatalf("expected step 1 formatted, got: %s", got)
	}
	if !strings.Contains(got, "Verification Command: `go test ./pkg/...`") {
		t.Fatalf("expected verification command in step 2, got: %s", got)
	}
}

func TestFormatTaskStepsPromptSection_SynthesizedForCode(t *testing.T) {
	t.Parallel()

	got := FormatTaskStepsPromptSection(nil, WorkClassCoding)
	if !strings.Contains(got, "Mandatory Execution Steps (Mutation Evidence Required)") {
		t.Fatalf("expected synthesized steps for code task, got: %s", got)
	}
	if !strings.Contains(got, "Code Implementation & Mutation") {
		t.Fatalf("expected mutation phase in synthesized steps, got: %s", got)
	}
}

func TestFormatTaskStepsPromptSection_EmptyForNonCode(t *testing.T) {
	t.Parallel()

	got := FormatTaskStepsPromptSection(nil, WorkClassDocsEval)
	if got != "" {
		t.Fatalf("expected empty steps for docs/eval, got: %s", got)
	}
}
