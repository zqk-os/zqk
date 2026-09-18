package cli

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objectrecord"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// cmdExecTrackerKey is the context key for CommandExecutionTracker (empty struct; avoids string-key collisions).
type cmdExecTrackerKey struct{}

// CommandExecutionTracker tracks command execution with complete traceability
type CommandExecutionTracker struct {
	mu sync.RWMutex

	// Command information
	Command       string
	NormalizedCmd string
	Args          []string
	Flags         map[string]any

	// Execution context
	PriorityPlan string
	Workstream   string
	Milestone    string

	// Actor information
	ActorID    string
	ActorRoles []string

	// Timing
	StartTime time.Time
	EndTime   time.Time

	// State changes
	ObjectsCreated []string
	ObjectsUpdated []string
	ObjectsDeleted []string

	// Outcome
	Success  bool
	ExitCode int
	Error    string
	TimedOut bool
}

// NewCommandExecutionTracker creates a new command execution tracker
func NewCommandExecutionTracker() *CommandExecutionTracker {
	return &CommandExecutionTracker{
		StartTime:      time.Now(),
		Flags:          make(map[string]any),
		ObjectsCreated: []string{},
		ObjectsUpdated: []string{},
		ObjectsDeleted: []string{},
	}
}

// SetCommand sets the command information
func (t *CommandExecutionTracker) SetCommand(command string, args []string) {
	_ = concurrency.RunInLockWithLogger(
		&t.mu, LockNameCommandTrackerSetCommand, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			t.Command = command
			t.Args = slices.Clone(args)
			return nil
		},
	)
}

// SetNormalizedCommand sets the normalized command
func (t *CommandExecutionTracker) SetNormalizedCommand(normalizedCmd string) {
	_ = concurrency.RunInLockWithLogger(
		&t.mu, LockNameCommandTrackerSetNormalized, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			t.NormalizedCmd = normalizedCmd
			return nil
		},
	)
}

// SetContext sets the execution context
func (t *CommandExecutionTracker) SetContext(priorityPlan, workstream, milestone string) {
	_ = concurrency.RunInLockWithLogger(
		&t.mu, LockNameCommandTrackerSetContext, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			t.PriorityPlan = priorityPlan
			t.Workstream = workstream
			t.Milestone = milestone
			return nil
		},
	)
}

// SetActor sets the actor information
func (t *CommandExecutionTracker) SetActor(actorID string, roles []string) {
	_ = concurrency.RunInLockWithLogger(
		&t.mu, LockNameCommandTrackerSetActor, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			t.ActorID = actorID
			t.ActorRoles = slices.Clone(roles)
			return nil
		},
	)
}

// SetFlags sets the command flags
func (t *CommandExecutionTracker) SetFlags(flags map[string]any) {
	_ = concurrency.RunInLockWithLogger(
		&t.mu, LockNameCommandTrackerSetFlags, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			t.Flags = maps.Clone(flags)
			return nil
		},
	)
}

// RecordObjectCreated records an object creation
func (t *CommandExecutionTracker) RecordObjectCreated(objectID string) {
	_ = concurrency.RunInLockWithLogger(
		&t.mu, LockNameCommandTrackerRecordCreated, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			t.ObjectsCreated = append(t.ObjectsCreated, objectID)
			return nil
		},
	)
}

// RecordObjectUpdated records an object update
func (t *CommandExecutionTracker) RecordObjectUpdated(objectID string) {
	_ = concurrency.RunInLockWithLogger(
		&t.mu, LockNameCommandTrackerRecordUpdated, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			t.ObjectsUpdated = append(t.ObjectsUpdated, objectID)
			return nil
		},
	)
}

// RecordObjectDeleted records an object deletion
func (t *CommandExecutionTracker) RecordObjectDeleted(objectID string) {
	_ = concurrency.RunInLockWithLogger(
		&t.mu, LockNameCommandTrackerRecordDeleted, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			t.ObjectsDeleted = append(t.ObjectsDeleted, objectID)
			return nil
		},
	)
}

// SetOutcome sets the command execution outcome
func (t *CommandExecutionTracker) SetOutcome(success bool, exitCode int, err error, timedOut bool) {
	_ = concurrency.RunInLockWithLogger(
		&t.mu, LockNameCommandTrackerSetOutcome, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			t.EndTime = time.Now()
			t.Success = success
			t.ExitCode = exitCode
			if err != nil {
				t.Error = err.Error()
			}
			t.TimedOut = timedOut
			return nil
		},
	)
}

// ToCommandMetric converts the tracker to a CommandMetric
func (t *CommandExecutionTracker) ToCommandMetric() *CommandMetric {
	var objectsCreated, objectsUpdated, objectsDeleted, actorRoles []string
	var command, normalizedCmd, priorityPlan, workstream, milestone, actorID, errorStr string
	var success bool
	var exitCode int
	var timedOut bool
	var startTime, endTime time.Time
	var args []string
	var flags map[string]any
	_ = concurrency.RunInRLockWithLogger(
		&t.mu, LockNameCommandTrackerToMetric, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Copy slices to avoid race conditions
			objectsCreated = make([]string, len(t.ObjectsCreated))
			copy(objectsCreated, t.ObjectsCreated)
			objectsUpdated = make([]string, len(t.ObjectsUpdated))
			copy(objectsUpdated, t.ObjectsUpdated)
			objectsDeleted = make([]string, len(t.ObjectsDeleted))
			copy(objectsDeleted, t.ObjectsDeleted)
			actorRoles = make([]string, len(t.ActorRoles))
			copy(actorRoles, t.ActorRoles)
			command = t.Command
			normalizedCmd = t.NormalizedCmd
			priorityPlan = t.PriorityPlan
			workstream = t.Workstream
			milestone = t.Milestone
			actorID = t.ActorID
			success = t.Success
			exitCode = t.ExitCode
			errorStr = t.Error
			timedOut = t.TimedOut
			startTime = t.StartTime
			endTime = t.EndTime
			args = slices.Clone(t.Args)
			flags = maps.Clone(t.Flags)
			return nil
		},
	)

	return &CommandMetric{
		Command:        command,
		NormalizedCmd:  normalizedCmd,
		Duration:       endTime.Sub(startTime),
		StartTime:      startTime,
		EndTime:        endTime,
		Success:        success,
		ExitCode:       exitCode,
		Error:          errorStr,
		TimedOut:       timedOut,
		Timestamp:      startTime,
		Args:           args,
		Flags:          flags,
		PriorityPlan:   priorityPlan,
		Workstream:     workstream,
		Milestone:      milestone,
		ActorID:        actorID,
		ActorRoles:     actorRoles,
		ObjectsCreated: objectsCreated,
		ObjectsUpdated: objectsUpdated,
		ObjectsDeleted: objectsDeleted,
	}
}

// ExtractFlagsFromCommand extracts flags from a cobra command
func ExtractFlagsFromCommand(cmd *cobra.Command) map[string]any {
	flags := make(map[string]any)

	// Get all flags
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Changed {
			switch flag.Value.Type() {
			case "string":
				if val, err := cmd.Flags().GetString(flag.Name); err == nil {
					flags[flag.Name] = val
				}
			case "bool":
				if val, err := cmd.Flags().GetBool(flag.Name); err == nil {
					flags[flag.Name] = val
				}
			case "int":
				if val, err := cmd.Flags().GetInt(flag.Name); err == nil {
					flags[flag.Name] = val
				}
			case "stringSlice":
				if val, err := cmd.Flags().GetStringSlice(flag.Name); err == nil {
					flags[flag.Name] = val
				}
			case "duration":
				// Duration flags need special handling
				if val, err := cmd.Flags().GetDuration(flag.Name); err == nil {
					flags[flag.Name] = val
				}
			default:
				// For unknown types, just record that the flag was set
				flags[flag.Name] = true
			}
		}
	})

	return flags
}

// GetTrackerFromContext gets the command execution tracker from context
func GetTrackerFromContext(ctx context.Context) *CommandExecutionTracker {
	if tracker, ok := ctx.Value(cmdExecTrackerKey{}).(*CommandExecutionTracker); ok {
		return tracker
	}
	return nil
}

// WithTracker adds a command execution tracker to context
func WithTracker(ctx context.Context, tracker *CommandExecutionTracker) context.Context {
	ctx = context.WithValue(ctx, cmdExecTrackerKey{}, tracker)
	return objectrecord.WithRecorder(ctx, tracker)
}

// NormalizeCommand normalizes a command for metrics tracking
// This is a public wrapper around the internal normalization logic
func NormalizeCommand(command string, args []string) string {
	hook := GetTimeoutHook()
	return hook.normalizeCommand(command, args)
}
