package object

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/objects/koi"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

func parseTargetObjectIDs(cmd *cobra.Command, args []string) ([]string, error) {
	args = expandObjectIDArgs(cmd, args)
	if len(args) == 0 {
		return nil, fmt.Errorf("at least one object ID is required (positional, comma-separated, and/or --ids)")
	}
	return args, nil
}

type transitionContext struct {
	args         []string
	ctx          context.Context
	secCtx       *pkgctx.SecurityContext
	env          *objectTransitionEnv
	flushTracker *flushKindTracker
}

func setupTransitionContext(cmd *cobra.Command, proc *cli.Processor, rawArgs []string) (*transitionContext, error) {
	args, err := parseTargetObjectIDs(cmd, rawArgs)
	if err != nil {
		return nil, err
	}
	ctx := proc.OperationContext()
	env, err := initObjectTransitionEnv(ctx, proc.ProjectRoot())
	if err != nil {
		return nil, err
	}
	return &transitionContext{
		args:         args,
		ctx:          ctx,
		secCtx:       proc.SecurityContext(),
		env:          env,
		flushTracker: newFlushKindTracker(len(args)),
	}, nil
}

type objectTransitionEnv struct {
	specLoader      *objects.SpecLoader
	lifecycleLoader *objects.LifecycleLoader
	validator       *validation.GoValidator
}

func initObjectTransitionEnv(ctx context.Context, projectRoot string) (*objectTransitionEnv, error) {
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)

	specLoader.SetBuilderRegistry(builders.NewSpecLoaderAdapter(builders.GetGlobalRegistry()))

	if err := specLoader.EnsureReady(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize spec loader: %w", err)
	}

	lifecyclesDir := filepath.Join(projectRoot, paths.ProcessInternalLifecyclesDir)
	lifecycleLoader := objects.NewLifecycleLoader(lifecyclesDir)

	gv := validation.NewGoValidatorWithLoaders(specLoader, lifecycleLoader)
	return &objectTransitionEnv{
		specLoader:      specLoader,
		lifecycleLoader: lifecycleLoader,
		validator:       gv,
	}, nil
}

type flushKindTracker struct {
	affected []string
	seen     map[string]bool
}

func newFlushKindTracker(capacity int) *flushKindTracker {
	return &flushKindTracker{
		affected: make([]string, 0, capacity),
		seen:     make(map[string]bool, capacity),
	}
}

func (t *flushKindTracker) add(k string) {
	if k == "" || t.seen[k] {
		return
	}
	t.seen[k] = true
	t.affected = append(t.affected, k)
}

func (t *flushKindTracker) addWithBacklogCascade(k string) {
	t.add(k)
	if k == objects.KindBacklogItem {
		t.add(objects.KindPriorityPlan)
	}
}

func (t *flushKindTracker) kinds() []string {
	return t.affected
}

func executeDurabilityFlush(proc *cli.Processor, kinds []string) (time.Duration, error) {
	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	t0 := time.Now()
	err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), kinds)
	return time.Since(t0), err
}

func flushObjectMutationVisibility(proc *cli.Processor, op string, affectedKinds []string) {
	if len(affectedKinds) == 0 {
		return
	}
	duration, err := executeDurabilityFlush(proc, affectedKinds)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Warn(op+": durability flush after status write").
			WithError(err).
			String("kinds", strings.Join(affectedKinds, ",")).
			Log()
	}
	logSlowCLIObjectMutationFlush(proc.Logger(), op, "", affectedKinds, duration, 0)
	proc.TriggerCacheFreshnessCheck(op, affectedKinds)
}

func flushDeleteVisibility(cmd *cobra.Command, proc *cli.Processor, id string, cascade bool, kinds []string) time.Duration {
	duration, err := executeDurabilityFlush(proc, kinds)
	if err != nil {
		evt := logging.FluentEvent(proc.Logger()).Warn("Persist flush after delete timed out, but object is removed").
			WithError(err).
			Bool("cascade", cascade)
		if id != emptyValue {
			evt.ObjectID(id)
		}
		evt.Log()
		if id != emptyValue {
			fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString(fmt.Sprintf("Warning: Object '%s' deleted successfully, but index refresh is delayed.", id)))
		} else {
			fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("Warning: Bulk delete completed, but index refresh is delayed."))
		}
	}
	return duration
}

type statusWithPercent struct {
	value   string
	percent float64
}

type loadedLifecycleTarget struct {
	id             string
	current        map[string]any
	kind           string
	currentStatus  string
	lifecycle      *objects.Lifecycle
	sortedStatuses []statusWithPercent
	statuses       []string
	currentIdx     int
}

func resolveAndLoadLifecycleTarget(ctx context.Context, secCtx *pkgctx.SecurityContext, proc *cli.Processor, lifecycleLoader *objects.LifecycleLoader, idArg string) (*loadedLifecycleTarget, error) {
	id, err := proc.ResolveSemanticArgument(ctx, "", idArg)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve ID: %w", err)
	}

	current, err := proc.Storage().Read(ctx, secCtx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to read object: %w", err)
	}

	kind := koi.Kind(current)
	currentStatus := koi.Status(current)

	lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
	if err != nil {
		return nil, fmt.Errorf("%s (%s): no lifecycle defined: %w", id, kind, err)
	}

	var sortedStatuses []statusWithPercent
	for _, st := range lifecycle.Statuses {
		sortedStatuses = append(sortedStatuses, statusWithPercent{
			value:   st.Value,
			percent: objects.LifecycleProgressPercent(st.Value, lifecycle.PercentComplete),
		})
	}
	sort.Slice(sortedStatuses, func(i, j int) bool {
		return sortedStatuses[i].percent < sortedStatuses[j].percent
	})

	var statuses []string
	currentIdx := -1
	for i, swp := range sortedStatuses {
		statuses = append(statuses, swp.value)
		if swp.value == currentStatus {
			currentIdx = i
		}
	}

	return &loadedLifecycleTarget{
		id:             id,
		current:        current,
		kind:           kind,
		currentStatus:  currentStatus,
		lifecycle:      lifecycle,
		sortedStatuses: sortedStatuses,
		statuses:       statuses,
		currentIdx:     currentIdx,
	}, nil
}

type lifecycleTargetHandler func(cmd *cobra.Command, proc *cli.Processor, tc *transitionContext, target *loadedLifecycleTarget) error

func executeLifecycleTransitions(
	cmd *cobra.Command,
	proc *cli.Processor,
	rawArgs []string,
	op string,
	errLabel string,
	handler lifecycleTargetHandler,
) error {
	tc, err := setupTransitionContext(cmd, proc, rawArgs)
	if err != nil {
		return err
	}

	var errors []string
	for _, idArg := range tc.args {
		process.TouchMeaningfulActivity()
		if tc.ctx.Err() != nil {
			errors = append(errors, fmt.Sprintf("%s: skipped due to context timeout: %v", idArg, tc.ctx.Err()))
			break
		}
		target, err := resolveAndLoadLifecycleTarget(tc.ctx, tc.secCtx, proc, tc.env.lifecycleLoader, idArg)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", idArg, err))
			continue
		}

		if err := handler(cmd, proc, tc, target); err != nil {
			errors = append(errors, err.Error())
		}
	}

	flushObjectMutationVisibility(proc, op, tc.flushTracker.kinds())

	if len(errors) > 0 {
		return fmt.Errorf("%s completed with errors:\n%s", errLabel, strings.Join(errors, "\n"))
	}

	return nil
}

type candidateProbeState struct {
	bestStatus        string
	rejectionByStatus map[string]string
	rejectedOrder     []string
}

func newCandidateProbeState(initialStatus string) *candidateProbeState {
	return &candidateProbeState{
		bestStatus:        initialStatus,
		rejectionByStatus: make(map[string]string),
	}
}

func (s *candidateProbeState) recordRejection(candidate, reason string) {
	s.rejectionByStatus[candidate] = reason
	s.rejectedOrder = append(s.rejectedOrder, candidate)
}
