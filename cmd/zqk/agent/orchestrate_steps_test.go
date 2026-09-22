package agent

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestBuildOrchestrationTaskSteps_usesBrandCLI(t *testing.T) {
	t.Parallel()
	steps := buildOrchestrationTaskSteps("foo", orchestrationTaskBoundary{GraphCompose: true, CodeValidate: true})
	if len(steps) != 3 {
		t.Fatalf("steps=%d want 3", len(steps))
	}
	impl, _ := steps[0][objects.FieldKeyDescription].(string)
	if !strings.Contains(impl, "`foo object get`") || strings.Contains(impl, "`zqk ") {
		t.Fatalf("implementation step still branded zqk: %q", impl)
	}
	graph, _ := steps[1][objects.FieldKeyDescription].(string)
	if !strings.Contains(graph, "`foo object get`") || strings.Contains(graph, "`zqk ") {
		t.Fatalf("graph step still branded zqk: %q", graph)
	}
	cmd, _ := steps[2][objects.FieldKeyCommand].(string)
	if cmd != "foo agent validate" {
		t.Fatalf("validate command=%q want foo agent validate", cmd)
	}
}

func TestOrchestrationTaskBoundaryFor_usesKernelRole(t *testing.T) {
	t.Parallel()
	op := orchestrationTaskBoundaryFor("operator", "", agentprompt.WorkClassCoding)
	if !op.GraphCompose || op.CodeValidate {
		t.Fatalf("operator boundary=%+v", op)
	}
	eng := orchestrationTaskBoundaryFor("software_engineer", "", agentprompt.WorkClassCoding)
	if eng.GraphCompose || !eng.CodeValidate {
		t.Fatalf("engineer boundary=%+v", eng)
	}
	arch := orchestrationTaskBoundaryFor("system_architect", "", agentprompt.WorkClassCoding)
	if !arch.GraphCompose || !arch.CodeValidate {
		t.Fatalf("system_architect boundary=%+v", arch)
	}
	sniff := orchestrationTaskBoundaryFor("software_engineer", "", agentprompt.WorkClassCoding)
	if sniff.GraphCompose {
		t.Fatal("software_engineer must not get graph-compose from an id substring")
	}
}

func TestOrchestrationEstimatedEffort_copiesSourceNotPlaceholder(t *testing.T) {
	t.Parallel()
	if got := orchestrationEstimatedEffort(map[string]any{objects.FieldKeyEstimatedEffort: "4h"}); got != "4h" {
		t.Fatalf("got %q want 4h", got)
	}
	if got := orchestrationEstimatedEffort(map[string]any{objects.FieldKeyTitle: "no estimate"}); got != "" {
		t.Fatalf("got %q want empty (do not invent 1d)", got)
	}
}
