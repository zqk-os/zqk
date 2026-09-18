package system

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

const (
	ensureOnboardingListTimeout   = 30 * time.Second
	ensureOnboardingCreateTimeout = 60 * time.Second
)

// EnsureOnboardingRoadmapJobInProject creates the onboarding roadmap seed scheduler job from template
// if not already present. When the scheduler runs, that job (immediate, one_time) creates the
// priority plan, workstream, and backlog items. Callable from init --with-onboarding-roadmap.
func EnsureOnboardingRoadmapJobInProject(projectRoot string, logger logging.Logger) error {
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root is empty")
	}
	zqkBin := filepath.Join(projectRoot, paths.CLICommandName)
	if _, err := fileutil.Stat(zqkBin); err != nil {
		zqkBin = paths.CLICommandName
	}
	templatePath := filepath.Join(projectRoot, "scripts", "scheduler_jobs", "onboarding_roadmap_seed.yaml")
	if _, err := fileutil.Stat(templatePath); err != nil {
		logging.Fluent(logger).Warn("Onboarding roadmap seed template not found; run bootstrap or copy scripts").
			Path(templatePath).
			WithError(err).
			Log()
		return nil // Not fatal; user may not have onboarding in this project
	}

	listCtx, listCancel := context.WithTimeout(context.Background(), ensureOnboardingListTimeout) // Background: request-or-shutdown derived
	defer listCancel()
	execCmd := execwrap.CommandContext(listCtx, zqkBin, "internal", "list", objects.KindSchedulerJob, "--format", "json")
	zqkenv.WireExecForIsolatedProject(execCmd, projectRoot)
	output, err := execCmd.CombinedOutput()
	if listCtx.Err() == context.DeadlineExceeded {
		return errfmt.Errorf("list scheduler_job timed out")
	}
	if err != nil {
		return errfmt.Errorf("list scheduler_job: %w (output: %s)", err, string(output))
	}

	var listOut struct {
		Objects []map[string]any `json:"objects"`
	}
	if err := json.Unmarshal(output, &listOut); err != nil {
		return errfmt.Newf("parse list output").Wrap(err)
	}
	jobObjs := listOut.Objects
	if jobObjs == nil {
		jobObjs = []map[string]any{}
	}
	for _, obj := range jobObjs {
		title, _ := obj[objects.FieldKeyTitle].(string)
		if title == "Onboarding Roadmap Seed" {
			id, _ := obj[objects.FieldKeyID].(string)
			logging.Fluent(logger).Info("Onboarding roadmap seed job already present").
				ObjectID(id).
				Log()
			return nil
		}
	}

	createCtx, createCancel := context.WithTimeout(context.Background(), ensureOnboardingCreateTimeout) // Background: request-or-shutdown derived
	defer createCancel()
	createCmd := execwrap.CommandContext(createCtx, zqkBin, "internal", "create", objects.KindSchedulerJob, "--file", templatePath)
	zqkenv.WireExecForIsolatedProject(createCmd, projectRoot)
	createOutput, createErr := createCmd.CombinedOutput()
	if createCtx.Err() == context.DeadlineExceeded {
		return errfmt.Errorf("create onboarding roadmap seed job timed out")
	}
	if createErr != nil {
		if strings.Contains(string(createOutput), "already exists") {
			return nil
		}
		return errfmt.Errorf("create scheduler_job from template: %w (output: %s)", createErr, string(createOutput))
	}
	logging.Fluent(logger).Info("Created onboarding roadmap seed job; start the scheduler to create the curriculum").
		String("template_path", templatePath).
		Log()
	return nil
}
