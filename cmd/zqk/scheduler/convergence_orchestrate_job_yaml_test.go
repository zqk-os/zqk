package scheduler

import (
	"github.com/zqk-os/zqk/pkg/execwrap"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/objects"
)

const convergenceOrchestrateJobRel = "scripts/scheduler_jobs/convergence_orchestrate.yaml"

// TestConvergenceOrchestrateJobYAMLStructure validates the maintained scheduler_job YAML used for
// cvs_convergence_orchestrate.sh (Appendix E). Keeps id, command, and env shape aligned; CONVERGENCE_SESSION_ID
// is empty in-repo until the operator sets it on the job object.
func TestConvergenceOrchestrateJobYAMLStructure(t *testing.T) {
	t.Parallel()
	projectRoot := findProjectRootForTest(t)
	path := filepath.Join(projectRoot, filepath.FromSlash(convergenceOrchestrateJobRel))
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var job map[string]any
	if err := yaml.Unmarshal(data, &job); err != nil {
		t.Fatalf("parse YAML: %v", err)
	}

	required := []string{
		objects.FieldKeyID,
		objects.FieldKeyKind,
		objects.FieldKeySchemaVersion,
		objects.FieldKeyStatus,
		objects.FieldKeyJobType,
		objects.FieldKeyTriggerType,
		objects.FieldKeyCategory,
		objects.FieldKeyExecutionMode,
		objects.FieldKeyCommand,
		objects.FieldKeyCommandArgs,
		objects.FieldKeyMaxRuntimeSeconds,
		objects.FieldKeyEnvironmentVariables,
	}
	for _, k := range required {
		if _, ok := job[k]; !ok {
			t.Errorf("missing field %q", k)
		}
	}

	if job[objects.FieldKeyID] != "SCH-convergence-orchestrate" {
		t.Errorf("id: got %v want SCH-convergence-orchestrate", job[objects.FieldKeyID])
	}
	if job[objects.FieldKeyKind] != "scheduler_job" {
		t.Errorf("kind: got %v", job[objects.FieldKeyKind])
	}
	if job[objects.FieldKeyJobType] != "run_wrapper" {
		t.Errorf("job_type: got %v want run_wrapper", job[objects.FieldKeyJobType])
	}
	if job[objects.FieldKeyTriggerType] != "timer" {
		t.Errorf("trigger_type: got %v want timer", job[objects.FieldKeyTriggerType])
	}

	env, _ := job[objects.FieldKeyEnvironmentVariables].(map[string]any)
	if env == nil {
		t.Fatal("environment_variables must be a map")
	}
	cvsID, ok := env["CONVERGENCE_SESSION_ID"].(string)
	if !ok {
		t.Fatalf("CONVERGENCE_SESSION_ID must be present (use empty string in template until configured)")
	}
	if cvsID != "" && !strings.HasPrefix(cvsID, "CVS-") {
		t.Errorf("CONVERGENCE_SESSION_ID must be empty or CVS-* prefixed, got %q", cvsID)
	}

	args, _ := job[objects.FieldKeyCommandArgs].([]any)
	if len(args) < 2 {
		t.Fatalf("command_args: want at least [\"-c\", script], got %#v", job[objects.FieldKeyCommandArgs])
	}
	if args[0] != "-c" {
		t.Errorf("command_args[0]: got %v want -c", args[0])
	}
	script, ok := args[1].(string)
	if !ok {
		t.Fatalf("command_args[1] must be string, got %T", args[1])
	}
	if !strings.Contains(script, "cvs_convergence_orchestrate.sh") {
		t.Errorf("embedded script must invoke cvs_convergence_orchestrate.sh:\n%s", script)
	}
	if !strings.Contains(script, "--no-fail-on-gates") {
		t.Errorf("embedded script must pass --no-fail-on-gates for scheduled runs:\n%s", script)
	}
}

// TestConvergenceOrchestrateJobYAML_CLIValidate runs zqk object create --dry-run against the same file
// the operator uses, so spec/validation regressions are caught when bin/zqk is present.
func TestConvergenceOrchestrateJobYAML_CLIValidate(t *testing.T) {
	projectRoot := findProjectRootForTest(t)
	zqkBin := filepath.Join(projectRoot, "bin", "zqk")
	if _, err := fileutil.Stat(zqkBin); err != nil {
		t.Skip("bin/zqk not built; run: go build -o bin/zqk ./cmd/zqk")
	}
	yamlPath := filepath.Join(projectRoot, filepath.FromSlash(convergenceOrchestrateJobRel))
	cmd := execwrap.Command(zqkBin, "object", "create", "scheduler_job", "--file", yamlPath, "--promote", "--dry-run")
	zqkenv.WireExecForIsolatedProject(cmd, projectRoot)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zqk object create scheduler_job --dry-run failed: %v\n%s", err, string(out))
	}
}
