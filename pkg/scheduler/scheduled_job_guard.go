package scheduler

// scheduledJobEnvMissing reports whether job is nil or has no environment map.
func scheduledJobEnvMissing(job *ScheduledJob) bool {
	return job == nil || job.EnvironmentVariables == nil
}
