package agentprompt

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// FormatTaskStepsPromptSection formats task steps into explicit sequential Markdown instructions.
// If stepsRaw is empty or nil and workClass requires coding/implementation, it synthesizes
// structured default execution steps to prevent compact LLMs from falling into unguided read loops.
func FormatTaskStepsPromptSection(stepsRaw any, workClass WorkClass) string {
	var steps []map[string]any
	switch s := stepsRaw.(type) {
	case []map[string]any:
		steps = s
	case []any:
		for _, item := range s {
			if m, ok := item.(map[string]any); ok {
				steps = append(steps, m)
			}
		}
	}

	if len(steps) == 0 {
		if !workClass.IsCode() {
			return ""
		}
		// Synthesize mandatory execution steps for coding tasks without explicit task_steps
		return strings.TrimSpace(`## Mandatory Execution Steps (Mutation Evidence Required)
Execute following steps in order to avoid read-looping and ensure mutation evidence:
1. **Scope & Existing Test Inspection**
   - Read target file(s) and existing tests to verify baseline behavior.
2. **Code Implementation & Mutation**
   - Apply necessary edits via write_code or write_file satisfying requirements.
3. **Automated Verification & Regression Check**
   - Run tests to confirm zero regressions and acceptance criteria pass.
4. **Completion & Mutation Evidence Summary**
   - Provide concise completion summary referencing the modified paths.`)
	}

	var sb strings.Builder
	sb.WriteString("## Sequenced Task Steps (Mandatory Execution Order)\n")
	sb.WriteString("Execute following steps in order to avoid read-looping and ensure mutation evidence:\n")
	for idx, step := range steps {
		title, _ := step[objects.FieldKeyTitle].(string)
		desc, _ := step[objects.FieldKeyDescription].(string)
		status, _ := step[objects.FieldKeyStatus].(string)
		cmd, _ := step[objects.FieldKeyCommand].(string)
		if title == "" {
			title = fmt.Sprintf("Step %d", idx+1)
		}
		sb.WriteString(fmt.Sprintf("%d. **%s**", idx+1, title))
		if status != "" {
			sb.WriteString(fmt.Sprintf(" [%s]", status))
		}
		sb.WriteString("\n")
		if desc != "" {
			sb.WriteString(fmt.Sprintf("   - %s\n", desc))
		}
		if cmd != "" {
			sb.WriteString(fmt.Sprintf("   - Verification Command: `%s`\n", cmd))
		}
	}
	return strings.TrimSpace(sb.String())
}
