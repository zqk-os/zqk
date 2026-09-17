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
	ErrServerNotFound          = errors.New("mcp server not found")
	ErrToolNotFound            = errors.New("mcp tool not found")
	ErrPromptNotFound          = errors.New("mcp prompt not found")
	ErrResourceNotFound        = errors.New("mcp resource not found")
	ErrProtocolVersionMismatch = errors.New("incompatible mcp protocol version")
	ErrConnectionClosed        = errors.New("mcp transport connection closed")
	ErrRPCTimeout              = errors.New("mcp rpc call timed out")
)
