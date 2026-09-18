package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/printer"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/quality"
	"gopkg.in/yaml.v3"
)

// FormatHandler handles output formatting for commands
// Supports both single-response and streaming formats
type FormatHandler interface {
	// Format formats data for output (single response)
	Format(data any) ([]byte, error)

	// IsStreaming returns true if this format streams data
	IsStreaming() bool

	// Stream streams data as it becomes available (for streaming formats)
	// Returns error if streaming fails
	Stream(ctx context.Context, data any, writer io.Writer) error

	// Validate checks if the handler can process the given data
	Validate(data any) error
}

// FormatHandlerRegistry manages format handlers
type FormatHandlerRegistry struct {
	mu       sync.RWMutex
	handlers map[OutputFormat]FormatHandler
}

var globalRegistry = &FormatHandlerRegistry{
	handlers: make(map[OutputFormat]FormatHandler),
}

const (
	jsonRPCVersionValue = "2.0"
	jsonRPCFieldVersion = "jsonrpc"
	jsonRPCFieldResult  = "result"
)

// RegisterFormatHandler registers a format handler
func RegisterFormatHandler(format OutputFormat, handler FormatHandler) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&globalRegistry.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameFormatHandlerRegister,
		func() error {
			globalRegistry.handlers[format] = handler
			return nil
		},
	)
}

// GetFormatHandler returns the handler for a format
func GetFormatHandler(format OutputFormat) FormatHandler {
	var handler FormatHandler
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&globalRegistry.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameFormatHandlerGet,
		func() error {
			handler = globalRegistry.handlers[format]
			return nil
		},
	)
	return handler
}

// TableFormatHandler handles table format output
type TableFormatHandler struct{}

func unwrapData(data any) any {
	if err, ok := data.(error); ok && err != nil {
		return map[string]any{
			"error":  err.Error(),
			"status": "error",
		}
	}
	if wrapper, ok := data.(printer.DataWrapper); ok {
		return sanitizeNonFiniteFloats(wrapper.Unwrap())
	}
	return sanitizeNonFiniteFloats(data)
}

func sanitizeNonFiniteFloats(v any) any {
	switch val := v.(type) {
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return 0.0
		}
		return val
	case float32:
		f := float64(val)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return float32(0.0)
		}
		return val
	case map[string]any:
		m := make(map[string]any, len(val))
		for k, item := range val {
			m[k] = sanitizeNonFiniteFloats(item)
		}
		return m
	case []any:
		s := make([]any, len(val))
		for i, item := range val {
			s[i] = sanitizeNonFiniteFloats(item)
		}
		return s
	case []map[string]any:
		s := make([]map[string]any, len(val))
		for i, item := range val {
			if sm, ok := sanitizeNonFiniteFloats(item).(map[string]any); ok {
				s[i] = sm
			} else {
				s[i] = item
			}
		}
		return s
	default:
		return v
	}
}

func (h *TableFormatHandler) Format(data any) ([]byte, error) {
	if tp, ok := data.(printer.TablePrintable); ok {
		return tp.FormatTable()
	}

	// Unify generic table formatting: if it is a slice of maps or a map containing "objects",
	// format it as a table to prevent layout drift (BLI-303).
	unwrapped := unwrapData(data)

	var objectList []map[string]any
	if m, ok := unwrapped.(map[string]any); ok {
		if objs, ok := m["objects"]; ok {
			if slice, ok := objs.([]map[string]any); ok {
				objectList = slice
			} else if anySlice, ok := objs.([]any); ok {
				allMaps := true
				temp := make([]map[string]any, 0, len(anySlice))
				for _, item := range anySlice {
					if itemMap, ok := item.(map[string]any); ok {
						temp = append(temp, itemMap)
					} else {
						allMaps = false
						break
					}
				}
				if allMaps {
					objectList = temp
				}
			}
		}
	} else if slice, ok := unwrapped.([]map[string]any); ok {
		objectList = slice
	} else if anySlice, ok := unwrapped.([]any); ok {
		allMaps := true
		temp := make([]map[string]any, 0, len(anySlice))
		for _, item := range anySlice {
			if itemMap, ok := item.(map[string]any); ok {
				temp = append(temp, itemMap)
			} else {
				allMaps = false
				break
			}
		}
		if allMaps {
			objectList = temp
		}
	}

	if len(objectList) > 0 {
		hasField := func(field string) bool {
			for _, obj := range objectList {
				if _, ok := obj[field]; ok {
					return true
				}
			}
			return false
		}

		var columnFields []string
		var columnHeaders []string
		var columnWidths []int

		if hasField("id") {
			columnFields = append(columnFields, "id")
			columnHeaders = append(columnHeaders, "ID")
			columnWidths = append(columnWidths, 15)
		}

		if hasField("title") {
			columnFields = append(columnFields, "title")
			columnHeaders = append(columnHeaders, "TITLE")
			columnWidths = append(columnWidths, 50)
		} else if hasField("name") {
			columnFields = append(columnFields, "name")
			columnHeaders = append(columnHeaders, "NAME")
			columnWidths = append(columnWidths, 50)
		}

		if hasField("status") {
			columnFields = append(columnFields, "status")
			columnHeaders = append(columnHeaders, "STATUS")
			columnWidths = append(columnWidths, 15)
		}

		if hasField("kind") {
			columnFields = append(columnFields, "kind")
			columnHeaders = append(columnHeaders, "KIND")
			columnWidths = append(columnWidths, 15)
		}

		// Fallback to first 3 keys of the first object if no standard columns found
		if len(columnFields) == 0 && len(objectList[0]) > 0 {
			count := 0
			for k := range objectList[0] {
				columnFields = append(columnFields, k)
				columnHeaders = append(columnHeaders, strings.ToUpper(k))
				columnWidths = append(columnWidths, 20)
				count++
				if count >= 3 {
					break
				}
			}
		}

		if len(columnFields) > 0 {
			var rows [][]string
			for _, obj := range objectList {
				row := make([]string, len(columnFields))
				for i, f := range columnFields {
					val := obj[f]
					if val == nil {
						row[i] = ""
					} else {
						row[i] = clipkg.TruncateString(strings.ReplaceAll(fmt.Sprintf("%v", val), "\n", " "), columnWidths[i])
					}
				}
				rows = append(rows, row)
			}

			tableStr := clipkg.RenderTable(columnHeaders, columnWidths, rows)
			return []byte(tableStr + "\n"), nil
		}
	}

	// For now, use YAML as table representation (human-readable structured format)
	return yamlMarshal(unwrapData(data))
}

func (h *TableFormatHandler) IsStreaming() bool {
	return false
}

func (h *TableFormatHandler) Stream(ctx context.Context, data any, writer io.Writer) error {
	// Table format doesn't stream
	formatted, err := h.Format(data)
	if err != nil {
		return err
	}
	_, err = writer.Write(formatted)
	return err
}

func (h *TableFormatHandler) Validate(data any) error {
	return nil // Table format accepts any data
}

// JSONFormatHandler handles JSON format output
type JSONFormatHandler struct{}

func (h *JSONFormatHandler) Format(data any) ([]byte, error) {
	return json.MarshalIndent(unwrapData(data), "", "  ")
}

func (h *JSONFormatHandler) IsStreaming() bool {
	return false
}

func (h *JSONFormatHandler) Stream(ctx context.Context, data any, writer io.Writer) error {
	// JSON format doesn't stream (single response)
	formatted, err := h.Format(data)
	if err != nil {
		return err
	}
	formatted = append(formatted, '\n')
	_, err = writer.Write(formatted)
	return err
}

func (h *JSONFormatHandler) Validate(data any) error {
	// Validate that data can be marshaled to JSON
	_, err := json.Marshal(unwrapData(data))
	return err
}

// JSONLFormatHandler handles JSONL format output (one JSON object per line)
// This is useful for machine-readable streaming output
type JSONLFormatHandler struct{}

func (h *JSONLFormatHandler) Format(data any) ([]byte, error) {
	data = unwrapData(data)
	// For single response, format as JSONL (one object per line)
	// If data is a slice, output each element on its own line
	switch v := data.(type) {
	case []any:
		var lines []byte
		encoder := json.NewEncoder(&lineWriter{lines: &lines})
		encoder.SetEscapeHTML(false)
		for _, item := range v {
			if err := encoder.Encode(item); err != nil {
				return nil, err
			}
		}
		return lines, nil
	default:
		// Single object - output as one line
		return json.Marshal(data)
	}
}

func (h *JSONLFormatHandler) IsStreaming() bool {
	return true // JSONL is inherently streaming-friendly
}

func (h *JSONLFormatHandler) Stream(ctx context.Context, data any, writer io.Writer) error {
	data = unwrapData(data)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)

	// If data is a slice, stream each element
	switch v := data.(type) {
	case []any:
		for _, item := range v {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if err := encoder.Encode(item); err != nil {
				return err
			}
			// Flush after each encode if writer supports it (for file output)
			if flusher, ok := writer.(interface{ Flush() error }); ok {
				_ = flusher.Flush() // Best effort - don't fail on flush errors
			}
		}
		return nil
	default:
		// Single object - output as one line
		err := encoder.Encode(data)
		if err != nil {
			return err
		}
		// Flush after encode if writer supports it (for file output)
		if flusher, ok := writer.(interface{ Flush() error }); ok {
			_ = flusher.Flush() // Best effort - don't fail on flush errors
		}
		return nil
	}
}

func (h *JSONLFormatHandler) Validate(data any) error {
	// Validate that data can be marshaled to JSON
	_, err := json.Marshal(unwrapData(data))
	return err
}

// lineWriter is a helper for collecting JSONL lines
type lineWriter struct {
	lines *[]byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	*w.lines = append(*w.lines, p...)
	return len(p), nil
}

// YAMLFormatHandler handles YAML format output
type YAMLFormatHandler struct{}

func (h *YAMLFormatHandler) Format(data any) ([]byte, error) {
	return yamlMarshal(unwrapData(data))
}

func (h *YAMLFormatHandler) IsStreaming() bool {
	return false
}

func (h *YAMLFormatHandler) Stream(ctx context.Context, data any, writer io.Writer) error {
	// YAML format doesn't stream (single response)
	formatted, err := h.Format(data)
	if err != nil {
		return err
	}
	_, err = writer.Write(formatted)
	return err
}

func (h *YAMLFormatHandler) Validate(data any) error {
	return nil // YAML format accepts any data
}

// CSVFormatHandler emits RFC 4180 CSV for *quality.MatrixGetResult (zqk matrix get).
type CSVFormatHandler struct{}

func (h *CSVFormatHandler) Format(data any) ([]byte, error) {
	res, ok := data.(*quality.MatrixGetResult)
	if !ok {
		return nil, errfmt.Errorf("output format csv is only supported for zqk matrix get results")
	}
	return quality.FormatMatrixGetResultCSV(res)
}

func (h *CSVFormatHandler) IsStreaming() bool {
	return false
}

func (h *CSVFormatHandler) Stream(ctx context.Context, data any, writer io.Writer) error {
	formatted, err := h.Format(data)
	if err != nil {
		return err
	}
	_, err = writer.Write(formatted)
	return err
}

func (h *CSVFormatHandler) Validate(data any) error {
	if _, ok := data.(*quality.MatrixGetResult); !ok {
		return errfmt.Errorf("output format csv is only supported for zqk matrix get results")
	}
	return nil
}

// JSONRPCFormatHandler handles JSON-RPC streaming format
// This integrates with the MCP event system to stream events as notifications
type JSONRPCFormatHandler struct {
	eventEmitter any          // *mcp.EventEmitter (avoid import cycle)
	mu           sync.RWMutex //nolint:unused // Reserved for future thread-safety
	maxRetries   int
	retryDelay   time.Duration
}

// NewJSONRPCFormatHandler creates a new JSON-RPC format handler
func NewJSONRPCFormatHandler(eventEmitter any) *JSONRPCFormatHandler {
	return &JSONRPCFormatHandler{
		eventEmitter: eventEmitter,
		maxRetries:   3,
		retryDelay:   100 * time.Millisecond,
	}
}

func (h *JSONRPCFormatHandler) Format(data any) ([]byte, error) {
	if err, ok := data.(error); ok && err != nil {
		response := map[string]any{
			jsonRPCFieldVersion: jsonRPCVersionValue,
			"error": map[string]any{
				"code":    -32000,
				"message": err.Error(),
			},
		}
		return json.Marshal(response)
	}
	// For single response, format as JSON-RPC response
	response := map[string]any{
		jsonRPCFieldVersion: jsonRPCVersionValue,
		jsonRPCFieldResult:  unwrapData(data),
	}
	return json.Marshal(response)
}

func (h *JSONRPCFormatHandler) IsStreaming() bool {
	return true
}

func (h *JSONRPCFormatHandler) Stream(ctx context.Context, data any, writer io.Writer) error {
	// JSON-RPC format streams events as notifications
	// This requires integration with MCP event emitter
	// For now, fall back to single response format
	return h.StreamWithEvents(ctx, data, writer, nil)
}

// StreamWithEvents streams data with event subscription
// eventTypes can be nil to subscribe to all events
func (h *JSONRPCFormatHandler) StreamWithEvents(
	ctx context.Context,
	data any,
	writer io.Writer,
	eventTypes []string,
) error {
	// This will be implemented when we have access to MCP event emitter
	// For now, format as single JSON-RPC response
	formatted, err := h.Format(data)
	if err != nil {
		return err
	}
	formatted = append(formatted, '\n')
	_, err = writer.Write(formatted)
	return err
}

func (h *JSONRPCFormatHandler) Validate(data any) error {
	// Validate that data can be marshaled to JSON
	_, err := json.Marshal(unwrapData(data))
	return err
}

// PermissionChecker is defined in helpers.go to avoid circular dependencies
// This interface allows format handlers to check permissions early (like dry-run)

// FormatHandlerWithPermissions wraps a format handler with permission checks
// This moves permission validation up a level, before data access (like dry-run)
// This short-circuits unauthorized access attempts efficiently
type FormatHandlerWithPermissions struct {
	handler           FormatHandler
	permissionChecker PermissionChecker
	mu                sync.RWMutex
	maxRetries        int
	retryDelay        time.Duration
}

// NewFormatHandlerWithPermissions creates a permission-aware format handler
func NewFormatHandlerWithPermissions(handler FormatHandler, checker PermissionChecker) *FormatHandlerWithPermissions {
	return &FormatHandlerWithPermissions{
		handler:           handler,
		permissionChecker: checker,
		maxRetries:        3,
		retryDelay:        100 * time.Millisecond,
	}
}

func (h *FormatHandlerWithPermissions) Format(data any) ([]byte, error) {
	// Permission check happens at command level before Format is called
	// This handler just delegates to the underlying handler
	return h.handler.Format(data)
}

func (h *FormatHandlerWithPermissions) IsStreaming() bool {
	return h.handler.IsStreaming()
}

func (h *FormatHandlerWithPermissions) Stream(ctx context.Context, data any, writer io.Writer) error {
	var checker PermissionChecker
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithRLockTimeout(
		&h.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameFormatHandlerPermissionsStream,
		func() error {
			checker = h.permissionChecker
			return nil
		},
	)

	// Check format permission before streaming (short-circuit unauthorized access)
	if checker != nil {
		// Extract format from context or data
		format := FormatJSONRPC // Default for streaming
		allowed, reason := checker.CheckFormatPermission(ctx, format)
		if !allowed {
			return errfmt.Errorf("format not allowed: %s", reason)
		}

		// Check data access permission before streaming (like dry-run)
		// This short-circuits data access if permissions are insufficient
		allowed, reason = checker.CheckDataAccess(ctx, data)
		if !allowed {
			return errfmt.Errorf("data access denied: %s", reason)
		}
	}

	// Delegate to underlying handler with retry logic for fault tolerance
	var lastErr error
	for attempt := 0; attempt < h.maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(h.retryDelay):
			}
		}

		err := h.handler.Stream(ctx, data, writer)
		if err == nil {
			return nil
		}
		lastErr = err

		// Don't retry on permission errors
		if strings.Contains(err.Error(), "not allowed") || strings.Contains(err.Error(), "denied") {
			return err
		}
	}

	return errfmt.Errorf("streaming failed after %d attempts: %w", h.maxRetries, lastErr)
}

func (h *FormatHandlerWithPermissions) Validate(data any) error {
	return h.handler.Validate(data)
}

// SetPermissionChecker updates the permission checker (thread-safe)
func (h *FormatHandlerWithPermissions) SetPermissionChecker(checker PermissionChecker) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_ = concurrency.WithLockTimeout(
		&h.mu,
		pkgctx.NewSystemContext(),
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameFormatHandlerPermissionsSet,
		func() error {
			h.permissionChecker = checker
			return nil
		},
	)
}

// yamlMarshal encodes structured CLI output as YAML (used for --format yaml and table’s structured view).
func yamlMarshal(data any) ([]byte, error) {
	return yaml.Marshal(data)
}

// InitializeDefaultHandlers registers default format handlers
func InitializeDefaultHandlers() {
	RegisterFormatHandler(FormatTable, &TableFormatHandler{})
	RegisterFormatHandler(FormatJSON, &JSONFormatHandler{})
	RegisterFormatHandler(FormatJSONL, &JSONLFormatHandler{})
	RegisterFormatHandler(FormatYAML, &YAMLFormatHandler{})
	RegisterFormatHandler(FormatCSV, &CSVFormatHandler{})
	RegisterFormatHandler(FormatJSONRPC, NewJSONRPCFormatHandler(nil))
	RegisterFormatHandler(FormatStream, NewJSONRPCFormatHandler(nil))
	RegisterFormatHandler(FormatSemanticLink, &SemanticLinkFormatHandler{})
}

// init automatically initializes default format handlers on package load
func init() {
	InitializeDefaultHandlers()
}
