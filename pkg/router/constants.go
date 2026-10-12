package router

import (
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// ModelTier identifies the performance, latency, and capability envelope of an LLM.
type ModelTier string

const (
	// TierFast designates low-latency, high-throughput models (e.g., Flash, Haiku, Mini).
	TierFast ModelTier = "fast"
	// TierStandard designates balanced general-purpose reasoning models (e.g., Pro, Sonnet, GPT-4o).
	TierStandard ModelTier = "standard"
	// TierHeavy designates deep-reasoning or chain-of-thought models (e.g., o1, o3, Opus, Thinking).
	TierHeavy ModelTier = "heavy"
	// TierLocal designates self-hosted or air-gapped models (e.g., Ollama, vLLM, Llama, Qwen-local).
	TierLocal ModelTier = "local"
)

// ScopeMode defines the enforcement strictness for tool filesystem and command scopes.
type ScopeMode string

const (
	// ScopeModeStrict rejects any access outside designated workspace roots or write operations in read-only mode.
	ScopeModeStrict ScopeMode = "strict"
	// ScopeModePermissive allows unmanaged paths with telemetry warning.
	ScopeModePermissive ScopeMode = "permissive"
	// ScopeModeReadOnly enforces strictly read-only operations across all tools.
	ScopeModeReadOnly ScopeMode = "read_only"
)

// Standard default durations for adaptive timeout envelopes.
const (
	DefaultFastTimeout     = 15 * time.Second
	DefaultStandardTimeout = 45 * time.Second
	DefaultHeavyTimeout    = 120 * time.Second
	DefaultLocalTimeout    = 180 * time.Second

	DefaultMinTimeout     = 1 * time.Second
	DefaultMaxTimeout     = 600 * time.Second
	DefaultTokenHeadroom  = 10 * time.Millisecond
	DefaultByteHeadroom   = 1 * time.Microsecond
	DefaultTimeoutPadding = 500 * time.Millisecond
)

// Well-known tool names recognized for scope and path inspection.
const (
	ToolReadFile     = "read_file"
	ToolWriteFile    = "write_file"
	ToolEditFile     = "replace_file_content"
	ToolListDir      = "list_dir"
	ToolDeleteFile   = "delete_file"
	ToolRunCommand   = "run_command"
	ToolGenerateCode = "generate_code"
)

// Common argument keys for tool parameter extraction.
const (
	ArgPath         = "path"
	ArgFilePath     = "file_path"
	ArgTargetFile   = "target_file"
	ArgAbsolutePath = "absolute_path"
	ArgCwd          = "cwd"
	ArgCommand      = "command"
)

// Sentinel error definitions for the Universal Gradient Router.
var (
	ErrNilRequest            = errfmt.Errorf("router: model request cannot be nil")
	ErrEmptyModel            = errfmt.Errorf("router: target model name cannot be empty")
	ErrModelNotFound         = errfmt.Errorf("router: no handler registered for model")
	ErrModelTimeout          = errfmt.Errorf("router: model execution exceeded timeout envelope")
	ErrAllTiersFailed        = errfmt.Errorf("router: all gradient fallback tiers failed")
	ErrNilToolCall           = errfmt.Errorf("router: tool call cannot be nil")
	ErrEmptyToolName         = errfmt.Errorf("router: tool name cannot be empty")
	ErrToolNotFound          = errfmt.Errorf("router: tool not registered and passthrough disabled")
	ErrOutOfScope            = errfmt.Errorf("router: target path is outside allowed workspace scope")
	ErrPermissionDenied      = errfmt.Errorf("router: operation denied by tool scope policy")
	ErrPathTraversalDetected = errfmt.Errorf("router: path traversal sequence detected")
	ErrReadOnlyViolation     = errfmt.Errorf("router: write operation prohibited in read-only scope")
	ErrNilHandler            = errfmt.Errorf("router: handler cannot be nil")
)
