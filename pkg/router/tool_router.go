package router

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// ToolCall encapsulates an invocation request for a specific tool.
type ToolCall struct {
	ToolName    string
	Arguments   map[string]any
	TargetPaths []string
	Scope       *ToolScope
}

// ToolResult captures the outcome of a tool execution.
type ToolResult struct {
	ToolName    string
	Output      any
	Error       error
	Duration    time.Duration
	Intercepted bool
	Passthrough bool
}

// ToolHandler represents an execution function for a tool invocation.
type ToolHandler func(ctx context.Context, call *ToolCall) (*ToolResult, error)

// ToolInterceptor wraps tool execution for policy enforcement, scoping, and telemetry.
type ToolInterceptor func(ctx context.Context, call *ToolCall, next ToolHandler) (*ToolResult, error)

// ToolScope defines path accessibility rules, filesystem boundaries, and read/write constraints.
type ToolScope struct {
	Mode             ScopeMode
	WorkspaceRoot    string
	AllowedRoots     []string
	BlockedPatterns  []string
	AllowPassthrough bool
}

// NewToolScope creates a ToolScope rooted at the specified workspace root directory.
func NewToolScope(workspaceRoot string) *ToolScope {
	cleaned := filepath.Clean(strings.TrimSpace(workspaceRoot))
	return &ToolScope{
		Mode:             ScopeModeStrict,
		WorkspaceRoot:    cleaned,
		AllowedRoots:     []string{cleaned},
		AllowPassthrough: true,
		BlockedPatterns: []string{
			".git/config",
			".git/hooks",
			"id_rsa",
			"id_ed25519",
			".env",
		},
	}
}

// ValidatePath evaluates whether a path conforms to the scope's boundary policies.
func (s *ToolScope) ValidatePath(targetPath string, isWrite bool) error {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(targetPath)
	if trimmed == "" {
		return nil
	}

	// Detect path traversal sequences.
	if strings.Contains(trimmed, "..") {
		parts := strings.Split(filepath.ToSlash(trimmed), "/")
		for _, part := range parts {
			if part == ".." {
				return ErrPathTraversalDetected
			}
		}
	}

	// Enforce read-only constraint on mutating operations.
	if isWrite && s.Mode == ScopeModeReadOnly {
		return ErrReadOnlyViolation
	}

	cleanTarget := filepath.Clean(trimmed)
	if !filepath.IsAbs(cleanTarget) && s.WorkspaceRoot != "" {
		cleanTarget = filepath.Clean(filepath.Join(s.WorkspaceRoot, cleanTarget))
	}

	// Check blocked patterns against path components.
	normalizedSlash := filepath.ToSlash(cleanTarget)
	for _, blocked := range s.BlockedPatterns {
		if blocked != "" && (strings.Contains(normalizedSlash, blocked) || strings.HasSuffix(normalizedSlash, blocked)) {
			return errfmt.Errorf("%w: path matches blocked pattern %q", ErrPermissionDenied, blocked)
		}
	}

	if s.Mode == ScopeModePermissive {
		return nil
	}

	// Validate path is contained within at least one permitted root.
	isContained := false
	candidateRoots := s.AllowedRoots
	if len(candidateRoots) == 0 && s.WorkspaceRoot != "" {
		candidateRoots = []string{s.WorkspaceRoot}
	}

	for _, root := range candidateRoots {
		cleanRoot := filepath.Clean(root)
		rel, err := filepath.Rel(cleanRoot, cleanTarget)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			isContained = true
			break
		}
	}

	if !isContained {
		return errfmt.Errorf("%w: target %q is outside roots %v", ErrOutOfScope, cleanTarget, candidateRoots)
	}

	return nil
}

// ToolRouter orchestrates scope enforcement, interceptor chains, and tool execution.
type ToolRouter struct {
	mu             sync.RWMutex
	scope          *ToolScope
	handlers       map[string]ToolHandler
	interceptors   []ToolInterceptor
	defaultHandler ToolHandler
}

// NewToolRouter creates a new ToolRouter with the given scope.
func NewToolRouter(scope *ToolScope) *ToolRouter {
	tr := &ToolRouter{
		scope:    scope,
		handlers: make(map[string]ToolHandler),
	}
	// Install the foundational scope verification interceptor.
	tr.interceptors = append(tr.interceptors, ScopeVerificationInterceptor)
	return tr
}

// RegisterTool binds a specific tool name to an execution handler.
func (r *ToolRouter) RegisterTool(toolName string, handler ToolHandler) {
	if handler == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[strings.ToLower(strings.TrimSpace(toolName))] = handler
}

// AddInterceptor appends a custom interceptor to the execution pipeline.
func (r *ToolRouter) AddInterceptor(interceptor ToolInterceptor) {
	if interceptor == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.interceptors = append(r.interceptors, interceptor)
}

// SetScope updates the default ToolScope for the router.
func (r *ToolRouter) SetScope(scope *ToolScope) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scope = scope
}

// SetDefaultHandler specifies the fallback or passthrough handler for unregistered tools.
func (r *ToolRouter) SetDefaultHandler(handler ToolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultHandler = handler
}

// RouteTool dispatches a tool call through interceptors and resolves to the handler.
func (r *ToolRouter) RouteTool(ctx context.Context, call *ToolCall) (*ToolResult, error) {
	if call == nil {
		return nil, ErrNilToolCall
	}
	trimmedName := strings.ToLower(strings.TrimSpace(call.ToolName))
	if trimmedName == "" {
		return nil, ErrEmptyToolName
	}

	// Auto-extract target paths from arguments if not explicitly provided.
	if len(call.TargetPaths) == 0 && call.Arguments != nil {
		call.TargetPaths = ExtractTargetPaths(call.Arguments)
	}

	// Attach router scope if call does not declare an overriding scope.
	r.mu.RLock()
	activeScope := call.Scope
	if activeScope == nil {
		activeScope = r.scope
		call.Scope = activeScope
	}
	handler := r.handlers[trimmedName]
	defaultH := r.defaultHandler
	interceptors := make([]ToolInterceptor, len(r.interceptors))
	copy(interceptors, r.interceptors)
	r.mu.RUnlock()

	isPassthrough := false
	if handler == nil {
		if defaultH != nil && (activeScope == nil || activeScope.AllowPassthrough) {
			handler = defaultH
			isPassthrough = true
		} else {
			return nil, errfmt.Errorf("%w: tool %q", ErrToolNotFound, trimmedName)
		}
	}

	// Compose the interceptor pipeline in execution order.
	start := time.Now()
	chain := handler
	for i := len(interceptors) - 1; i >= 0; i-- {
		currentInterceptor := interceptors[i]
		nextHandler := chain
		chain = func(subCtx context.Context, c *ToolCall) (*ToolResult, error) {
			return currentInterceptor(subCtx, c, nextHandler)
		}
	}

	res, err := chain(ctx, call)
	if res != nil {
		res.Duration = time.Since(start)
		if isPassthrough && !res.Intercepted && err == nil {
			res.Passthrough = true
		}
	}
	return res, err
}

// ScopeVerificationInterceptor enforces tool path containment and read/write security constraints.
func ScopeVerificationInterceptor(ctx context.Context, call *ToolCall, next ToolHandler) (*ToolResult, error) {
	if call == nil {
		return nil, ErrNilToolCall
	}
	scope := call.Scope
	if scope != nil {
		isWrite := isWriteTool(call.ToolName)
		for _, path := range call.TargetPaths {
			if err := scope.ValidatePath(path, isWrite); err != nil {
				return &ToolResult{
					ToolName:    call.ToolName,
					Error:       err,
					Intercepted: true,
				}, err
			}
		}
	}
	return next(ctx, call)
}

func isWriteTool(toolName string) bool {
	n := strings.ToLower(strings.TrimSpace(toolName))
	switch n {
	case ToolWriteFile, ToolEditFile, ToolDeleteFile, ToolGenerateCode:
		return true
	default:
		return strings.Contains(n, "write") || strings.Contains(n, "delete") || strings.Contains(n, "remove") || strings.Contains(n, "create")
	}
}

// ExtractTargetPaths extracts candidate filesystem paths from common tool argument keys.
func ExtractTargetPaths(args map[string]any) []string {
	if args == nil {
		return nil
	}
	var paths []string
	keys := []string{ArgPath, ArgFilePath, ArgTargetFile, ArgAbsolutePath, ArgCwd}
	for _, key := range keys {
		if val, exists := args[key]; exists {
			if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
				paths = append(paths, strings.TrimSpace(s))
			}
		}
	}
	return paths
}
