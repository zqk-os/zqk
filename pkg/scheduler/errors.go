package scheduler

import "errors"

var (
	ErrJobNotFound         = errors.New("scheduler job not found")
	ErrJobAlreadyRunning   = errors.New("scheduler job is already running")
	ErrJobCancelled        = errors.New("scheduler job was cancelled")
	ErrJobTimeout          = errors.New("scheduler job execution timed out")
	ErrSchedulerDown       = errors.New("scheduler daemon is down or unreachable")
	ErrQueueFull           = errors.New("scheduler submission queue is full")
	ErrWorkerPoolExhausted = errors.New("scheduler worker pool is exhausted")
	ErrInvalidCronSchedule = errors.New("invalid cron schedule expression")
	ErrJobValidationFailed = errors.New("scheduler job specification validation failed")
)
