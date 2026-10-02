package scheduler

import "strings"

// CommandRequirement specifies the scheduler availability requirements for a CLI command.
type CommandRequirement int

const (
	// RequirementNone indicates the command has no dependency on the scheduler.
	RequirementNone CommandRequirement = iota
	// RequirementOptional indicates the command benefits from scheduler caches but can proceed if degraded.
	RequirementOptional
	// RequirementRequired indicates the command strictly requires the scheduler unless explicitly overridden.
	RequirementRequired
)

// EvaluateSchedulerGuard determines whether execution should be blocked or warned given the command requirement.
func EvaluateSchedulerGuard(requirement CommandRequirement, running, allowDegraded bool) (block bool, warn bool) {
	if running || requirement == RequirementNone {
		return false, false
	}
	if requirement == RequirementRequired && !allowDegraded {
		return true, false
	}
	return false, true
}

// IsSchedulerRunningForRoot checks if a scheduler instance is running in memory or on disk for the given root.
func IsSchedulerRunningForRoot(projectRoot string) bool {
	if strings.TrimSpace(projectRoot) == "" {
		return true
	}
	sched, ok := GetGlobalSchedulerIfAvailable()
	if ok && sched != nil && sched.IsRunning() {
		return true
	}
	running, _, err := IsSchedulerRunning(projectRoot)
	return err == nil && running
}

// scheduledJobEnvMissing reports whether job is nil or has no environment map.
func scheduledJobEnvMissing(job *ScheduledJob) bool {
	return job == nil || job.EnvironmentVariables == nil
}
