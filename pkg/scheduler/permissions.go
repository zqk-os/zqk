package scheduler

import (
	"fmt"
	"slices"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// checkPermission checks if the security context has the required permission
func (s *Scheduler) checkPermission(requiredPerm string) error {
	if s.secCtx == nil {
		return errfmt.Errorf("security context not set")
	}

	// Check for admin role (bypasses all permission checks)
	if slices.Contains(s.secCtx.Roles, "admin") {
		return nil
	}

	// Check for exact permission match
	if slices.Contains(s.secCtx.Permissions, requiredPerm) {
		return nil
	}
	// Wildcard match (e.g., "execute:*" matches "execute:scheduler_job")
	if slices.Contains(s.secCtx.Permissions, "*") {
		return nil
	}
	// Pattern match (e.g., "execute:*" matches "execute:scheduler_job")
	if strings.Contains(requiredPerm, ":") {
		parts := strings.Split(requiredPerm, ":")
		if len(parts) == 2 {
			op := parts[0]
			if slices.Contains(s.secCtx.Permissions, fmt.Sprintf("%s:*", op)) {
				return nil
			}
		}
	}

	return errfmt.Errorf("required permission: %s", requiredPerm)
}

// CheckJobReadPermission checks if the security context has permission to read jobs
func (s *Scheduler) CheckJobReadPermission() error {
	return s.checkPermission("read:scheduler_job")
}

// CheckJobWritePermission checks if the security context has permission to create/update jobs
func (s *Scheduler) CheckJobWritePermission() error {
	return s.checkPermission("write:scheduler_job")
}

// CheckJobDeletePermission checks if the security context has permission to delete jobs
func (s *Scheduler) CheckJobDeletePermission() error {
	return s.checkPermission("delete:scheduler_job")
}

// CheckJobExecutePermission checks if the security context has permission to execute/trigger jobs
func (s *Scheduler) CheckJobExecutePermission() error {
	return s.checkPermission("execute:scheduler_job")
}

// CheckSchedulerManagePermission checks if the security context has permission to manage scheduler (start/stop)
func (s *Scheduler) CheckSchedulerManagePermission() error {
	return s.checkPermission("manage:scheduler")
}
