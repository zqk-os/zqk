package mcp

import "errors"

var (
	ErrProjectRootNotAvailable = errors.New("project root not available")
	ErrAccountNotFound         = errors.New("account not found")
	ErrPathRequired            = errors.New("path is required")
	ErrCommandRequired         = errors.New("command is required")
	ErrAccessDenied            = errors.New("access denied")
	ErrWorkflowNoNextItem      = errors.New("no next item available in workflow queue")
	ErrInvalidParams           = errors.New("invalid parameters")
)
