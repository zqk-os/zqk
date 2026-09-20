package scenario

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ApplyOptions optional overrides for ApplyScenarioBundle (e.g. use existing storage in tests).
type ApplyOptions struct {
	Storage    storage.ObjectStorageProvider // if set, use instead of creating storage from projectRoot
	StepStdout io.Writer                     // when set, step command stdout is written here (e.g. CLI stdout)
	StepStderr io.Writer                     // when set, step command stderr is written here (e.g. CLI stderr)
}

// BundleSummary describes the result of applying a bundle.
type BundleSummary struct {
	ProjectRoot string `json:"project_root"`
	BundleName  string `json:"bundle_name"`

	// Created object IDs by kind.
	CreatedSchedulerJobIDs       []string `json:"created_scheduler_job_ids,omitempty"`
	CreatedGoalIDs               []string `json:"created_goal_ids,omitempty"`
	CreatedRequirementIDs        []string `json:"created_requirement_ids,omitempty"`
	CreatedCriteriaIDs           []string `json:"created_criteria_ids,omitempty"`
	CreatedBacklogItemIDs        []string `json:"created_backlog_item_ids,omitempty"`
	CreatedTestCaseIDs           []string `json:"created_test_case_ids,omitempty"`
	CreatedDocEntryIDs           []string `json:"created_doc_entry_ids,omitempty"`
	CreatedConvergenceSessionIDs []string `json:"created_convergence_session_ids,omitempty"`

	// HintToID maps any id_hint used in the bundle to the final object ID.
	HintToID map[string]string `json:"hint_to_id,omitempty"`
}

// ApplyScenarioBundle parses and applies a scenario bundle to the given project.
//
// First-pass implementation:
//   - Parses the bundle from r.
//   - Creates scheduler_job fixtures via FileObjectStorage using the provided project root.
//   - Returns a BundleSummary with IDs and hint mappings.
//   - Does not yet handle requirements/criteria/backlog/test_case objects or steps.
//
// This keeps behavior narrow but immediately useful for persistence and
// scheduler-related bundles while we evolve the full traceability surface.
func ApplyScenarioBundle(ctx stdcontext.Context, projectRoot string, r io.Reader, mode ApplyMode, opts ...*ApplyOptions) (*BundleSummary, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("projectRoot is required")
	}
	// Canonicalize to an absolute path so CAS index queues (keyed by project root)
	// are flushed consistently even when callers pass ".".
	if abs, err := filepath.Abs(projectRoot); err == nil && abs != emptyValue {
		projectRoot = abs
	}

	bundle, err := LoadBundle(r)
	if err != nil {
		return nil, err
	}

	// For now we only support object application; steps are ignored but we log
	// the fact that they were present so callers are not surprised.
	if mode != ApplyObjectsOnly && mode != ApplyObjectsAndSteps {
		return nil, errfmt.Errorf("unsupported apply mode: %d", mode)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	var storageProvider storage.ObjectStorageProvider
	createdStorageProvider := false
	if len(opts) > 0 && opts[0] != nil && opts[0].Storage != nil {
		storageProvider = opts[0].Storage
	} else {
		var err error
		storageProvider, err = storage.NewFileObjectStorage(projectRoot)
		if err != nil {
			return nil, errfmt.Errorf("create storage for project root %q: %w", projectRoot, err)
		}
		createdStorageProvider = true
	}
	if createdStorageProvider {
		defer func() {
			// Ensure we don't leak background workers after apply, which can race with
			// t.TempDir() cleanup in unit tests.
			if s, ok := storageProvider.(interface {
				Shutdown(stdcontext.Context) error
			}); ok {
				ctx2, cancel := stdcontext.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				_ = s.Shutdown(ctx2)
			}
		}()
	}
	secCtx := pkgctx.NewSystemSecurityContext()

	summary := &BundleSummary{
		ProjectRoot: projectRoot,
		BundleName:  bundle.Metadata.Name,
		HintToID:    make(map[string]string),
	}

	// Fixtures: scheduler_jobs (first so HintToID is populated for any refs from traceability objects)
	if len(bundle.Objects.Fixtures.SchedulerJobs) > 0 {
		if err := applySchedulerJobFixtures(ctx, storageProvider, secCtx, bundle.Objects.Fixtures.SchedulerJobs, summary); err != nil {
			return nil, err
		}
	}

	// Traceability: create order derived from spec index (dependencies first; cycles broken with deferred ref updates).
	// Set a cache checker so ref validation accepts IDs we've already created in this apply (avoids CAS index
	// visibility delay when creating requirement/criteria/test_case/backlog_item that reference each other).
	createdIDsChecker := makeCreatedIDsCacheChecker(summary)
	prevChecker := storage.GetCacheChecker()
	storage.SetCacheChecker(createdIDsChecker)
	defer storage.SetCacheChecker(prevChecker)

	bundleKinds := bundleTraceabilityKinds(bundle)
	createOrder, _, err := CreateOrderFromSpecIndex(projectRoot, bundleKinds)
	if err != nil {
		return nil, err
	}
	for _, kind := range createOrder {
		switch kind {
		case objects.KindGoal:
			if len(bundle.Objects.Goals) > 0 {
				if err := applyGoals(ctx, storageProvider, secCtx, bundle.Objects.Goals, summary); err != nil {
					_ = emitScenarioSummary(projectRoot, summary)
					return nil, err
				}
				// Flush CAS index so requirement goal_refs validation can resolve goals.
				if flushErr := caspkg.FlushKindListingIndexForProjectRootWithTimeout(projectRoot, objects.KindGoal, 15*time.Second); flushErr != nil {
					logging.Fluent(logger).Warn("CAS flush after goals failed (requirement refs may fail)").
						WithError(flushErr).
						Log()
				}
			}
		case objects.KindCriteria:
			if len(bundle.Objects.Criteria) > 0 {
				if err := applyCriteria(ctx, storageProvider, secCtx, bundle.Objects.Criteria, summary); err != nil {
					_ = emitScenarioSummary(projectRoot, summary)
					return nil, err
				}
				// Flush so requirement criteria_refs validation can resolve criteria.
				flushKind(projectRoot, objects.KindCriteria, logger)
			}
		case objects.KindRequirement:
			if len(bundle.Objects.Requirements) > 0 {
				if err := applyRequirements(ctx, storageProvider, secCtx, bundle.Objects.Requirements, summary); err != nil {
					_ = emitScenarioSummary(projectRoot, summary)
					return nil, err
				}
				flushKind(projectRoot, objects.KindRequirement, logger)
			}
		case objects.KindTestCase:
			if len(bundle.Objects.TestCases) > 0 {
				if err := applyTestCases(ctx, storageProvider, secCtx, bundle.Objects.TestCases, summary); err != nil {
					_ = emitScenarioSummary(projectRoot, summary)
					return nil, err
				}
				flushKind(projectRoot, objects.KindTestCase, logger)
			}
		case objects.KindBacklogItem:
			if len(bundle.Objects.BacklogItems) > 0 {
				if err := applyBacklogItems(ctx, storageProvider, secCtx, bundle.Objects.BacklogItems, summary); err != nil {
					_ = emitScenarioSummary(projectRoot, summary)
					return nil, err
				}
				flushKind(projectRoot, objects.KindBacklogItem, logger)
			}
		case objects.KindDocEntry:
			if len(bundle.Objects.DocEntries) > 0 {
				if err := applyDocEntries(ctx, storageProvider, secCtx, bundle.Objects.DocEntries, summary); err != nil {
					_ = emitScenarioSummary(projectRoot, summary)
					return nil, err
				}
				flushKind(projectRoot, objects.KindDocEntry, logger)
			}
		case objects.KindConvergenceSession:
			if len(bundle.Objects.ConvergenceSessions) > 0 {
				if err := applyConvergenceSessions(ctx, storageProvider, secCtx, bundle.Objects.ConvergenceSessions, summary); err != nil {
					_ = emitScenarioSummary(projectRoot, summary)
					return nil, err
				}
				flushKind(projectRoot, objects.KindConvergenceSession, logger)
			}
		}
	}

	// Deferred ref updates rely on referenced objects being visible via CAS indexes.
	// On larger workspaces those indexes can lag behind; flush once here with a
	// longer timeout so we don't fail mid-apply with "object not found".
	if flushErr := caspkg.FlushAllListingIndexesForProjectRootWithTimeout(projectRoot, 15*time.Second); flushErr != nil {
		_ = emitScenarioSummary(projectRoot, summary) // best-effort for debugging
		return nil, errfmt.Newf("CAS index flush before deferred refs failed").Wrap(flushErr)
	}
	if len(bundle.Objects.Criteria) > 0 {
		updateErr := linkCriteriaTemplatesOntoRequirements(ctx, storageProvider, secCtx, bundle.Objects.Criteria, summary)
		if updateErr != nil {
			if strings.Contains(updateErr.Error(), "object not found") {
				if flushErr := caspkg.FlushAllListingIndexesForProjectRootWithTimeout(projectRoot, 15*time.Second); flushErr == nil {
					if retryErr := linkCriteriaTemplatesOntoRequirements(ctx, storageProvider, secCtx, bundle.Objects.Criteria, summary); retryErr == nil {
						updateErr = nil
					} else {
						updateErr = retryErr
					}
				}
			}
			if updateErr != nil {
				_ = emitScenarioSummary(projectRoot, summary)
				return nil, updateErr
			}
		}
	}

	if err := emitScenarioSummary(projectRoot, summary); err != nil {
		logging.Fluent(logger).Warn("Failed to write scenario summary (non-fatal)").
			WithError(err).
			String("bundle_name", summary.BundleName).
			Log()
	}

	// Steps: when requested, execute bundle.Steps after objects are applied and summary is written.
	if mode == ApplyObjectsAndSteps && len(bundle.Steps) > 0 {
		var stepOut, stepErr io.Writer
		if len(opts) > 0 && opts[0] != nil {
			stepOut, stepErr = opts[0].StepStdout, opts[0].StepStderr
		}
		if err := executeBundleSteps(ctx, projectRoot, bundle, logger, stepOut, stepErr); err != nil {
			return summary, err
		}
	}

	return summary, nil
}

// scenarioSummaryDir returns the directory for a bundle's scenario output: .zqk/scenarios/<sanitized-name>.
func scenarioSummaryDir(projectRoot, bundleName string) string {
	sanitized := strings.TrimSpace(bundleName)
	sanitized = strings.ReplaceAll(sanitized, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, "\\", "-")
	if sanitized == emptyValue {
		sanitized = "default"
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, "scenarios", sanitized)
}

// LoadScenarioSummary reads .zqk/scenarios/<bundle-name>/scenario-summary.json for a given project root and bundle name.
// Use the same bundle name as in the bundle metadata (e.g. "persistence-traceability-bundle"); the path is derived
// using the same sanitization as emitScenarioSummary.
func LoadScenarioSummary(projectRoot, bundleName string) (*BundleSummary, error) {
	if projectRoot == emptyValue || bundleName == emptyValue {
		return nil, errfmt.Errorf("projectRoot and bundleName are required")
	}
	path := filepath.Join(scenarioSummaryDir(projectRoot, bundleName), "scenario-summary.json")
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return nil, errfmt.Errorf("read scenario summary %q: %w", path, err)
	}
	var summary BundleSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, errfmt.Newf("unmarshal scenario summary").Wrap(err)
	}
	return &summary, nil
}

// emitScenarioSummary writes summary to .zqk/scenarios/<bundle-name>/scenario-summary.json for traceability and debugging.
func emitScenarioSummary(projectRoot string, summary *BundleSummary) error {
	dir := scenarioSummaryDir(projectRoot, summary.BundleName)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Errorf("create scenario dir %q: %w", dir, err)
	}
	path := filepath.Join(dir, "scenario-summary.json")
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return errfmt.Newf("marshal scenario summary").Wrap(err)
	}
	if err := fileutil.WriteFile(path, data, paths.FilePerm600); err != nil {
		return errfmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

// executeBundleSteps runs the commands defined in bundle.Steps in order when ApplyMode is ApplyObjectsAndSteps.
// Commands run with projectRoot as their working directory and inherit the current environment, with ZQK_TEST_ROOT
// set to projectRoot so they behave like test-scenarios. Any failing command stops execution and returns an error.
// When stepStdout or stepStderr are non-nil, step command output is streamed there (e.g. CLI stdout/stderr).
func executeBundleSteps(ctx stdcontext.Context, projectRoot string, bundle *Bundle, logger logging.Logger, stepStdout, stepStderr io.Writer) error {
	for i, step := range bundle.Steps {
		if len(step.Commands) == 0 {
			continue
		}
		stepID := step.ID
		if strings.TrimSpace(stepID) == emptyValue {
			stepID = fmt.Sprintf("step-%d", i+1)
		}

		logging.Fluent(logger).Info("ApplyScenarioBundle: executing scenario step").
			String("bundle_name", bundle.Metadata.Name).
			String("step_id", stepID).
			String("description", step.Description).
			Int("command_count", len(step.Commands)).
			Log()

		for _, cmdStr := range step.Commands {
			cmdStr = strings.TrimSpace(cmdStr)
			if cmdStr == emptyValue {
				continue
			}

			logging.Fluent(logger).Info("ApplyScenarioBundle: running step command").
				String("bundle_name", bundle.Metadata.Name).
				String("step_id", stepID).
				String("command", cmdStr).
				Log()

			command := execwrap.CommandContext(ctx, "bash", "-lc", cmdStr)
			command.Dir = projectRoot
			env := os.Environ()
			env = append(env, zqkenv.TestRoot().Name()+"="+projectRoot)
			command.Env = env

			if stepStdout != nil || stepStderr != nil {
				command.Stdout = stepStdout
				command.Stderr = stepStderr
				if command.Stdout == nil {
					command.Stdout = io.Discard
				}
				if command.Stderr == nil {
					command.Stderr = io.Discard
				}
				if err := command.Run(); err != nil {
					exitCode := 0
					if command.ProcessState != nil {
						exitCode = command.ProcessState.ExitCode()
					}
					logging.Fluent(logger).Warn("ApplyScenarioBundle: step command failed").
						String("bundle_name", bundle.Metadata.Name).
						String("step_id", stepID).
						String("command", cmdStr).
						Int("exit_code", exitCode).
						WithError(err).
						Log()
					return errfmt.Errorf("execute scenario step %s command %q: %w", stepID, cmdStr, err)
				}
			} else {
				output, err := command.CombinedOutput()
				exitCode := 0
				if command.ProcessState != nil {
					exitCode = command.ProcessState.ExitCode()
				}
				if err != nil {
					logging.Fluent(logger).Warn("ApplyScenarioBundle: step command failed").
						String("bundle_name", bundle.Metadata.Name).
						String("step_id", stepID).
						String("command", cmdStr).
						Int("exit_code", exitCode).
						String("output", string(output)).
						WithError(err).
						Log()
					return errfmt.Errorf("execute scenario step %s command %q: %w", stepID, cmdStr, err)
				}
				logging.Fluent(logger).Info("ApplyScenarioBundle: step command completed").
					String("bundle_name", bundle.Metadata.Name).
					String("step_id", stepID).
					String("command", cmdStr).
					Int("exit_code", exitCode).
					Log()
			}
		}
	}

	return nil
}

// makeCreatedIDsCacheChecker returns a cache checker that reports "exists" for any ID already created in this apply.
// Used so ref validation accepts goal/criteria/requirement/etc. refs when the referent was created earlier in the same apply.
func makeCreatedIDsCacheChecker(summary *BundleSummary) func(string) (string, bool) {
	return func(id string) (string, bool) {
		if slices.Contains(summary.CreatedGoalIDs, id) ||
			slices.Contains(summary.CreatedCriteriaIDs, id) ||
			slices.Contains(summary.CreatedRequirementIDs, id) ||
			slices.Contains(summary.CreatedTestCaseIDs, id) ||
			slices.Contains(summary.CreatedBacklogItemIDs, id) ||
			slices.Contains(summary.CreatedSchedulerJobIDs, id) ||
			slices.Contains(summary.CreatedDocEntryIDs, id) ||
			slices.Contains(summary.CreatedConvergenceSessionIDs, id) {
			return "", true
		}

		return "", false
	}
}

// bundleTraceabilityKinds returns the list of object kinds that the bundle creates (non-empty sections).
func bundleTraceabilityKinds(bundle *Bundle) []string {
	var kinds []string
	if len(bundle.Objects.Goals) > 0 {
		kinds = append(kinds, objects.KindGoal)
	}
	if len(bundle.Objects.Requirements) > 0 {
		kinds = append(kinds, objects.KindRequirement)
	}
	if len(bundle.Objects.Criteria) > 0 {
		kinds = append(kinds, objects.KindCriteria)
	}
	if len(bundle.Objects.TestCases) > 0 {
		kinds = append(kinds, objects.KindTestCase)
	}
	if len(bundle.Objects.BacklogItems) > 0 {
		kinds = append(kinds, objects.KindBacklogItem)
	}
	if len(bundle.Objects.DocEntries) > 0 {
		kinds = append(kinds, objects.KindDocEntry)
	}
	if len(bundle.Objects.ConvergenceSessions) > 0 {
		kinds = append(kinds, objects.KindConvergenceSession)
	}
	return kinds
}

func applySchedulerJobFixtures(
	ctx stdcontext.Context,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	fixtures []SchedulerJobFixture,
	summary *BundleSummary,
) error {
	for _, fx := range fixtures {
		if fx.Template == nil {
			return errfmt.Errorf("scheduler_job fixture missing template (id_hint=%q id=%q)", fx.IDHint, fx.ID)
		}

		// Determine final ID:
		//   - Prefer explicit ID on fixture.
		//   - Otherwise use id_hint as the ID.
		//   - Otherwise look for an id field in the template.
		finalID := fx.ID
		if finalID == emptyValue {
			if fx.IDHint != emptyValue {
				finalID = fx.IDHint
			} else if rawID, ok := fx.Template[objects.FieldKeyID].(string); ok && rawID != emptyValue {
				finalID = rawID
			}
		}
		if finalID == emptyValue {
			return errfmt.Errorf("scheduler_job fixture must specify id, id_hint, or template.id")
		}

		// Copy template so we do not mutate the original map from the bundle.
		obj := make(map[string]any, len(fx.Template)+2)
		for k, v := range fx.Template {
			obj[k] = v
		}
		obj[objects.FieldKeyID] = finalID
		if _, ok := obj[objects.FieldKeyKind]; !ok {
			obj[objects.FieldKeyKind] = objects.KindSchedulerJob
		}

		if err := storageProvider.Create(ctx, secCtx, obj); err != nil {
			return errfmt.Errorf("create scheduler_job %q from fixture (id_hint=%q): %w", finalID, fx.IDHint, err)
		}

		summary.CreatedSchedulerJobIDs = append(summary.CreatedSchedulerJobIDs, finalID)
		if fx.IDHint != emptyValue {
			summary.HintToID[fx.IDHint] = finalID
		}
	}
	return nil
}

func flushKind(projectRoot string, kind string, logger logging.Logger) {
	if flushErr := caspkg.FlushKindListingIndexForProjectRootWithTimeout(projectRoot, kind, 15*time.Second); flushErr != nil {
		logging.Fluent(logger).Warn("CAS flush after " + kind + " failed").
			WithError(flushErr).
			Log()
	}
}
