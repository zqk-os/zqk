package mcp

import (
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

// RegisterTool registers a tool with the server
// Thread-safe for concurrent registration
// Updates the immutable cache atomically for lock-free reads
func (s *Server) RegisterTool(name, description string, inputSchema any, handler ToolHandler) {
	_ = concurrency.RunInLockWithLogger(
		&s.toolsMu, LockNameMcpServerRegisterTool, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.tools[name] = Tool{
				Name:        name,
				Description: description,
				InputSchema: inputSchema,
				Handler:     handler, // Store handler for direct invocation
			}
			// Update immutable cache atomically (lock-free reads)
			s.toolsCache.Store(mapValues(s.tools))
			return nil
		},
	)

	if s.mcpMetrics != nil {
		s.mcpMetrics.RegisterUncalledToolMetrics(name)
	}
}

// RegisterPrompt registers a prompt template with the server
// Thread-safe for concurrent registration
// Updates the immutable cache atomically for lock-free reads
func (s *Server) RegisterPrompt(name, description string, arguments []PromptArgument) {
	_ = concurrency.RunInLockWithLogger(
		&s.promptsMu, LockNameMcpServerRegisterPrompt, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.prompts[name] = Prompt{
				Name:        name,
				Description: description,
				Arguments:   arguments,
			}
			// Update immutable cache atomically (lock-free reads)
			s.promptsCache.Store(mapValues(s.prompts))
			return nil
		},
	)
}

// RegisterResource registers a resource with the server
// Thread-safe for concurrent registration
// Updates the immutable cache atomically for lock-free reads
func (s *Server) RegisterResource(uri, name, description, mimeType string) {
	_ = concurrency.RunInLockWithLogger(
		&s.resourcesMu, LockNameMcpServerRegisterResource, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.resources[uri] = Resource{
				URI:         uri,
				Name:        name,
				Description: description,
				MimeType:    mimeType,
				Category:    "", // Can be set via RegisterResourceWithMetadata
				Priority:    "", // Can be set via RegisterResourceWithMetadata
				Tags:        nil,
				Metadata:    nil,
			}
			// Update immutable cache atomically (lock-free reads)
			s.resourcesCache.Store(mapValues(s.resources))
			return nil
		},
	)
}

// RegisterResourceWithMetadata registers a resource with additional metadata
// Thread-safe for concurrent registration
// Updates the immutable cache atomically for lock-free reads
func (s *Server) RegisterResourceWithMetadata(uri, name, description, mimeType, category, priority string, tags []string, metadata map[string]string) {
	_ = concurrency.RunInLockWithLogger(
		&s.resourcesMu, LockNameMcpServerRegisterResourceMetadata, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.resources[uri] = Resource{
				URI:         uri,
				Name:        name,
				Description: description,
				MimeType:    mimeType,
				Category:    category,
				Priority:    priority,
				Tags:        tags,
				Metadata:    metadata,
			}
			// Update immutable cache atomically (lock-free reads)
			s.resourcesCache.Store(mapValues(s.resources))
			return nil
		},
	)
}

// ListTools returns all registered tools (for testing and inspection)
// Lock-free: uses immutable cached slice for zero-copy reads
func (s *Server) ListTools() []Tool {
	if cached := s.toolsCache.Load(); cached != nil {
		return cached.([]Tool)
	}
	// Fallback: cache not initialized yet, build it (shouldn't happen in normal operation)
	var tools []Tool
	_ = concurrency.RunInRLockWithLogger(
		&s.toolsMu, LockNameMcpServerListToolsFallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			tools = mapValues(s.tools)
			return nil
		},
	)
	s.toolsCache.Store(tools)
	return tools
}

// ApplyToolsAllowlist removes any registered tool whose name is not in allowlist.
// When allowlist is non-empty, only these tool names remain exposed (e.g. for IDE's tool limit).
// When allowlist is empty, this is a no-op.
func (s *Server) ApplyToolsAllowlist(allowlist []string) {
	if len(allowlist) == 0 {
		return
	}
	allowedSet := make(map[string]bool, len(allowlist))
	for _, name := range allowlist {
		allowedSet[name] = true
	}
	_ = concurrency.RunInLockWithLogger(
		&s.toolsMu, LockNameMcpServerApplyToolsAllowlist, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for name := range s.tools {
				if !allowedSet[name] {
					delete(s.tools, name)
				}
			}
			s.toolsCache.Store(mapValues(s.tools))
			return nil
		},
	)
}

// getToolCount returns the number of registered tools (lock-free)
func (s *Server) getToolCount() int {
	if cached := s.toolsCache.Load(); cached != nil {
		return len(cached.([]Tool))
	}
	// Fallback: cache not initialized yet
	var count int
	_ = concurrency.RunInRLockWithLogger(
		&s.toolsMu, LockNameMcpServerGetToolCountFallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			count = len(s.tools)
			return nil
		},
	)
	return count
}

// getResourceCount returns the number of registered resources (lock-free)
func (s *Server) getResourceCount() int {
	if cached := s.resourcesCache.Load(); cached != nil {
		return len(cached.([]Resource))
	}
	var count int
	_ = concurrency.RunInRLockWithLogger(
		&s.resourcesMu, LockNameMcpServerGetResourceCountFallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			count = len(s.resources)
			return nil
		},
	)
	return count
}

// ListPrompts returns all registered prompts (for testing and inspection)
// Lock-free: uses immutable cached slice for zero-copy reads
func (s *Server) ListPrompts() []Prompt {
	if cached := s.promptsCache.Load(); cached != nil {
		return cached.([]Prompt)
	}
	// Fallback: cache not initialized yet, build it (shouldn't happen in normal operation)
	var prompts []Prompt
	_ = concurrency.RunInRLockWithLogger(
		&s.promptsMu, LockNameMcpServerListPromptsFallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			prompts = mapValues(s.prompts)
			return nil
		},
	)
	s.promptsCache.Store(prompts)
	return prompts
}

// getPromptCount returns the number of registered prompts (lock-free)
func (s *Server) getPromptCount() int {
	if cached := s.promptsCache.Load(); cached != nil {
		return len(cached.([]Prompt))
	}
	// Fallback: cache not initialized yet
	var count int
	_ = concurrency.RunInRLockWithLogger(
		&s.promptsMu, LockNameMcpServerGetPromptCountFallback, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			count = len(s.prompts)
			return nil
		},
	)
	return count
}
