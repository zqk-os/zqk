package logging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/lanceman/zqk/pkg/when"
)

// EventLogger provides context-aware logging that automatically extracts
// relevant details from events and context
type EventLogger struct {
	logger Logger
	ctx    context.Context
}

// NewEventLogger creates a new event logger with context
// First tries to extract LoggingContext from Go context
// Falls back to extracting profile string for backward compatibility
func NewEventLogger(ctx context.Context) *EventLogger {
	// Try to get LoggingContext from Go context
	loggingCtx := getLoggingContextFromGoContext(ctx)
	// Pass parent context to GetLoggerFromLoggingContext for formatter context propagation
	logger := GetLoggerFromLoggingContext(ctx, loggingCtx)
	return &EventLogger{
		logger: logger.WithContext(ctx),
		ctx:    ctx,
	}
}

// extractProfileFromContext extracts the profile from various context types
//
//nolint:unused // Helper function - reserved for future use
func extractProfileFromContext(ctx context.Context) string {
	// Try direct profile value
	if profileVal := ctx.Value("profile"); profileVal != nil {
		if p, ok := profileVal.(string); ok {
			return p
		}
	}

	// Try cli_context wrapper
	if cliCtx := ctx.Value("cli_context"); cliCtx != nil {
		// Use reflection to extract profile if it's a struct with Profile field
		val := reflect.ValueOf(cliCtx)
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
		}
		if val.Kind() == reflect.Struct {
			profileField := val.FieldByName("Profile")
			if profileField.IsValid() && profileField.Kind() == reflect.String {
				return profileField.String()
			}
		}
	}

	return "human" // Default
}

// LogSpecLoad logs a spec loading event with automatic context extraction
func (el *EventLogger) LogSpecLoad(specFile string, err error) {
	fields := []Field{
		String("event", "spec_load"),
		String("spec_file", specFile),
		String("spec_name", filepath.Base(specFile)),
	}

	// Add caller information
	if file, line := el.getCaller(); file != emptyValue {
		fields = append(fields,
			String("caller_file", file),
			Int("caller_line", line))
	}

	when.When(func() bool { return err != nil }).Then(func() {
		FluentEvent(el).Error("Failed to load spec", err).WithFields(fields...).Log()
	}).OrElse(func() {
		FluentEvent(el).Info("Spec loaded").WithFields(fields...).Log()
	}).Run()
}

// LogSpecValidation logs a spec validation event
// errors can be []ValidationError or []any (from pkg/objects)
func (el *EventLogger) LogSpecValidation(specFile string, errors any) {
	// Convert errors to []ValidationError
	var validationErrors []ValidationError
	errorCount := 0

	if errors != nil {
		val := reflect.ValueOf(errors)
		if val.Kind() == reflect.Slice {
			errorCount = val.Len()
			for i := 0; i < val.Len(); i++ {
				elem := val.Index(i).Interface()
				validationErrors = append(validationErrors, FromObjectsValidationError(elem))
			}
		}
	}

	fields := []Field{
		String("event", "spec_validation"),
		String("spec_file", specFile),
		Int("error_count", errorCount),
	}

	when.When(func() bool { return errorCount > 0 }).Then(func() {
		errorDetails := make([]map[string]any, 0, len(validationErrors))
		for _, err := range validationErrors {
			if len(errorDetails) < 5 {
				errorDetails = append(errorDetails, map[string]any{
					objectFieldKeyField: err.Field,
					"missing_item":      err.MissingItem,
					"criteria_ref":      err.CriteriaRef,
				})
			}
		}
		fields = append(fields, Field{Key: "errors", Value: errorDetails})
		FluentEvent(el).Warn("Spec validation found issues").WithFields(fields...).Log()
	}).OrElse(func() {
		FluentEvent(el).Info("Spec validation passed").WithFields(fields...).Log()
	}).Run()
}

// LogCheckResult logs a check command result with automatic aggregation
func (el *EventLogger) LogCheckResult(kind string, objectCount, issueCount int, tierCounts map[int]int) {
	fields := []Field{
		String("event", "check_result"),
		String("kind", kind),
		Int("object_count", objectCount),
		Int("issue_count", issueCount),
	}

	// Add tier breakdown
	for tier, count := range tierCounts {
		fields = append(fields, Int(fmt.Sprintf("tier_%d_count", tier), count))
	}

	level := InfoLevel
	if issueCount > 0 {
		if tierCounts[1] > 0 { // Tier 1 (blocking) issues
			level = ErrorLevel
		} else if tierCounts[2] > 0 { // Tier 2 (warnings)
			level = WarnLevel
		}
	}

	msg := fmt.Sprintf("Check completed: %d objects, %d issues", objectCount, issueCount)
	el.log(level, msg, nil, fields...)
}

// LogObjectCheck logs a single object check event
func (el *EventLogger) LogObjectCheck(objectID, objectKind string, issues []CheckIssue) {
	fields := []Field{
		String("event", "object_check"),
		String("object_id", objectID),
		String("object_kind", objectKind),
		Int("issue_count", len(issues)),
	}

	// Categorize issues
	issueCategories := make(map[string]int)
	for _, issue := range issues {
		issueCategories[issue.Category]++
	}
	if len(issueCategories) > 0 {
		fields = append(fields, Field{Key: "issue_categories", Value: issueCategories})
	}

	// Determine log level based on issue severity
	hasBlocking := false
	hasWarnings := false
	for _, issue := range issues {
		switch issue.Tier {
		case 1:
			hasBlocking = true
		case 2:
			hasWarnings = true
		}
	}

	level := InfoLevel
	if hasBlocking {
		level = ErrorLevel
	} else if hasWarnings {
		level = WarnLevel
	}

	msg := fmt.Sprintf("Checked object %s (%s)", objectID, objectKind)
	el.log(level, msg, nil, fields...)
}

// LogDependencyGraph logs a dependency graph operation
func (el *EventLogger) LogDependencyGraph(operation string, specCount int, errors []GraphError) {
	fields := []Field{
		String("event", "dependency_graph"),
		String("operation", operation),
		Int("spec_count", specCount),
		Int("error_count", len(errors)),
	}

	when.When(func() bool { return len(errors) > 0 }).Then(func() {
		errorTypes := make(map[string]int)
		for _, err := range errors {
			errorTypes[err.Type]++
		}
		fields = append(fields, Field{Key: "error_types", Value: errorTypes})
		FluentEvent(el).Error("Dependency graph operation failed", nil).WithFields(fields...).Log()
	}).OrElse(func() {
		FluentEvent(el).Info("Dependency graph operation completed").WithFields(fields...).Log()
	}).Run()
}

// LogFileOperation logs a file operation with automatic path extraction
func (el *EventLogger) LogFileOperation(operation, filePath string, err error) {
	fields := []Field{
		String("event", "file_operation"),
		String("operation", operation),
		String("file_path", filePath),
		String("file_name", filepath.Base(filePath)),
		String("file_dir", filepath.Dir(filePath)),
	}

	// Add file size if available
	if info, statErr := os.Stat(filePath); statErr == nil {
		fields = append(fields, Int("file_size", int(info.Size())), Timestamp(info.ModTime()))
	}

	when.When(func() bool { return err != nil }).Then(func() {
		FluentEvent(el).Error("File operation failed", err).WithFields(fields...).Log()
	}).OrElse(func() {
		FluentEvent(el).Info("File operation completed").WithFields(fields...).Log()
	}).Run()
}

// LogHashOperation logs a hash registry operation
func (el *EventLogger) LogHashOperation(operation, kind, filename, hash string, err error) {
	fields := []Field{
		String("event", "hash_operation"),
		String("operation", operation),
		String("kind", kind),
		String("filename", filename),
	}

	if hash != emptyValue {
		// Only log first 16 chars of hash for brevity
		fields = append(fields, String("hash", hash[:minInt(16, len(hash))]+"..."))
	}

	when.When(func() bool { return err != nil }).Then(func() {
		FluentEvent(el).Error("Hash operation failed", err).WithFields(fields...).Log()
	}).OrElse(func() {
		FluentEvent(el).Debug("Hash operation completed").WithFields(fields...).Log()
	}).Run()
}

// LogValidationError logs a validation error with automatic field extraction
func (el *EventLogger) LogValidationError(err ValidationError) {
	fields := []Field{
		String("event", "validation_error"),
		String("field", err.Field),
		String("missing_item", err.MissingItem),
		String("message", err.Message),
	}

	if err.CriteriaRef != emptyValue {
		fields = append(fields, String("criteria_ref", err.CriteriaRef))
	}

	FluentEvent(el).Warn("Validation error").WithFields(fields...).Log()
}

// LogError logs a generic error with automatic context extraction
func (el *EventLogger) LogError(msg string, err error, additionalFields ...Field) {
	fields := []Field{
		String("event", "error"),
	}

	// Add caller information
	if file, line := el.getCaller(); file != emptyValue {
		fields = append(fields,
			String("caller_file", file),
			Int("caller_line", line))
	}

	// Add any additional fields
	fields = append(fields, additionalFields...)

	FluentEvent(el).Error(msg, err).WithFields(fields...).Log()
}

// LogInfo logs an info message with automatic context.
// Implementation routes through FluentEvent (POLICY-CODE-007); prefer this over duplicating Fluent on el.Logger() when the automatic event field is desired.
func (el *EventLogger) LogInfo(msg string, additionalFields ...Field) {
	fields := []Field{
		String("event", "info"),
	}
	fields = append(fields, additionalFields...)
	FluentEvent(el).Info(msg).WithFields(fields...).Log()
}

// LogDebug logs a debug message with automatic context.
// Implementation routes through FluentEvent (POLICY-CODE-007).
func (el *EventLogger) LogDebug(msg string, additionalFields ...Field) {
	fields := []Field{
		String("event", "debug"),
	}
	fields = append(fields, additionalFields...)
	FluentEvent(el).Debug(msg).WithFields(fields...).Log()
}

// LogWarning logs a warning with automatic context.
// Implementation routes through FluentEvent (POLICY-CODE-007).
func (el *EventLogger) LogWarning(msg string, additionalFields ...Field) {
	fields := []Field{
		String("event", "warning"),
	}
	fields = append(fields, additionalFields...)
	FluentEvent(el).Warn(msg).WithFields(fields...).Log()
}

// Logger returns the wrapped Logger for pooled fluent chaining ([Fluent]; e.g. storage StorageLog).
// Prefer LogInfo/LogDebug when you rely on EventLogger's automatic `event` field; use Fluent when
// emitting POLICY-CODE-007 wire-key messages without that wrapper.
func (el *EventLogger) Logger() Logger {
	if el == nil {
		return nil
	}
	return el.logger
}

// WithObjectRef adds an object reference to subsequent logs
func (el *EventLogger) WithObjectRef(kind, id string) *EventLogger {
	return &EventLogger{
		logger: el.logger.WithObjectRef(kind, id),
		ctx:    el.ctx,
	}
}

// WithFields adds fields to subsequent logs
func (el *EventLogger) WithFields(fields ...Field) *EventLogger {
	return &EventLogger{
		logger: el.logger.WithFields(fields...),
		ctx:    el.ctx,
	}
}

// Helper methods

func (el *EventLogger) log(level LogLevel, msg string, err error, fields ...Field) {
	switch level {
	case DebugLevel:
		FluentEvent(el).Debug(msg).WithFields(fields...).Log()
	case InfoLevel:
		FluentEvent(el).Info(msg).WithFields(fields...).Log()
	case WarnLevel:
		FluentEvent(el).Warn(msg).WithFields(fields...).Log()
	case ErrorLevel:
		FluentEvent(el).Error(msg, err).WithFields(fields...).Log()
	case FatalLevel:
		el.logger.Fatal(msg, err, fields...)
	}
}

func (el *EventLogger) getCaller() (file string, line int) {
	// Skip 3 frames: getCaller -> LogXxx -> actual caller
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		return "", 0
	}
	// Return relative path
	if idx := strings.Index(file, "zqk/"); idx >= 0 {
		return file[idx+4:], line // Skip "zqk/"
	}
	return filepath.Base(file), line
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Type definitions for event-specific data

// ValidationError represents a validation error
// This is a logging-specific type that can wrap pkg/objects.ValidationError
type ValidationError struct {
	Field       string
	MissingItem string
	CriteriaRef string
	Message     string
}

// FromObjectsValidationError converts a pkg/objects.ValidationError to logging.ValidationError
func FromObjectsValidationError(err any) ValidationError {
	// Use reflection to extract fields if it's a struct
	val := reflect.ValueOf(err)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() == reflect.Struct {
		ve := ValidationError{}
		if field := val.FieldByName("Field"); field.IsValid() {
			ve.Field = field.String()
		}
		if item := val.FieldByName("MissingItem"); item.IsValid() {
			ve.MissingItem = item.String()
		}
		if ref := val.FieldByName("CriteriaRef"); ref.IsValid() {
			ve.CriteriaRef = ref.String()
		}
		if msg := val.FieldByName("Message"); msg.IsValid() {
			ve.Message = msg.String()
		}
		return ve
	}
	return ValidationError{Message: fmt.Sprintf("%v", err)}
}

// CheckIssue represents a check issue
type CheckIssue struct {
	Tier        int
	Category    string
	Message     string
	AutoFixable bool
}

// GraphError represents a graph validation error
type GraphError struct {
	Type    string
	Message string
	Specs   []string
}
