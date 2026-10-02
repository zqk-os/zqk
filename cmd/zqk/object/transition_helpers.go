package object

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
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

type objectTransitionEnv struct {
	specLoader      *objects.SpecLoader
	lifecycleLoader *objects.LifecycleLoader
	validator       *validation.GoValidator
}

func initObjectTransitionEnv(ctx context.Context, projectRoot string) (*objectTransitionEnv, error) {
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)

	builderRegistry := builders.GetGlobalRegistry()
	adapter := builders.NewSpecLoaderAdapter(builderRegistry)
	specLoader.SetBuilderRegistry(adapter)

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

func flushObjectMutationVisibility(proc *cli.Processor, op string, affectedKinds []string) {
	if len(affectedKinds) == 0 {
		return
	}
	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	t0 := time.Now()
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), affectedKinds); err != nil {
		logging.FluentEvent(proc.Logger()).Warn(op + ": durability flush after status write").
			WithError(err).
			String("kinds", strings.Join(affectedKinds, ",")).
			Log()
	}
	logSlowCLIObjectMutationFlush(proc.Logger(), op, "", affectedKinds, time.Since(t0), 0)
	proc.TriggerCacheFreshnessCheck(op, affectedKinds)
}

func flushDeleteVisibility(cmd *cobra.Command, proc *cli.Processor, id string, cascade bool, kinds []string) time.Duration {
	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	t0 := time.Now()
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), kinds); err != nil {
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
	return time.Since(t0)
}
