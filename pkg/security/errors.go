package security

import "errors"

// Common security errors used across packages.
var (
	ErrAuthnNoCredential     = errors.New("security: fail-closed auth denied — no credential provided")
	ErrAuthnInvalidInput     = errors.New("security: fail-closed auth denied — invalid input")
	ErrMCPInvalidToolName    = errors.New("security: MCP bind — invalid tool name")
	ErrSandboxPathDenied     = errors.New("security: file operation denied by sandbox allowlist")
	ErrZeroTimeout           = errors.New("security: fail-closed timeouts require a positive non-zero deadline")
	ErrTimeoutBlocked        = errors.New("execution blocked by enforced timeout")
	ErrPathTraversalDetected = errors.New("security: path traversal detected")
)

// ============================================================================
// Constants for internal message strings.  All should be short and reusable.
// ============================================================================

const (
	magicCredTooShort       = "credential too short to be valid" //nolint:gosec // error message string, not hardcoded credential
	magicUnverified         = "unverified credential — requires explicit verification"
	magicUserIDEmpty        = "user ID empty after auth — denying"
	magicNoDeadlineStr      = "no deadline set for"
	magicDeadlineReqd       = "deadline required for"
	magicEmptyToolName      = "empty tool name"
	magicToolNameTooLong    = "tool name exceeds 256 characters"
	magicToolNameBadCharset = "tool name must match allowed ASCII character set"
	magicNoRootsConfigured  = "no roots configured"
	magicSandboxNotAllowed  = "path not allowed by sandbox"
)
