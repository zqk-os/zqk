package mcp

import (
	"context"
	"fmt"
	"maps"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/interactive"
	"github.com/zqk-os/zqk/pkg/objects"
	instancebuilders "github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// HandleCreateObjectInteractive handles the create_object_interactive built-in tool
func HandleCreateObjectInteractive(ctx context.Context, server *Server, args map[string]any) (any, error) {
	// Extract kind (required)
	kind, ok := args[objects.FieldKeyKind].(string)
	if !ok || kind == emptyValue {
		return nil, NewElicitationErrorForMissingParams([]string{"kind"}, GetToolName("create_object_interactive"))
	}

	// Extract session_id (optional)
	var sessionID string
	if sid, ok := args[objects.FieldKeySessionID].(string); ok && sid != emptyValue {
		sessionID = sid
	}

	// Get session manager
	sessionManager := interactive.GetGlobalInteractiveSessionManager()

	// Initialize template generator and loop
	fieldRegistry := objects.GetGlobalFieldRegistry()
	generator := interactive.NewTemplateGenerator(fieldRegistry)
	loop := interactive.NewTemplateLoop(generator)

	// Get schema version for this kind (needed for instance builder lookup)
	schemaVersion := generator.GetSchemaVersion(kind)

	// Get or create session
	var session *interactive.InteractiveSessionState
	var loopState *interactive.LoopState
	var err error
	var builder instancebuilders.InstanceBuilder

	if sessionID != emptyValue {
		// Get existing session
		var exists bool
		session, exists = sessionManager.GetSession(sessionID)
		if !exists {
			return nil, errfmt.Errorf("session not found or expired: %s", sessionID)
		}
		loopState = session.LoopState

		// Retrieve builder from session (if available)
		if session.Builder != nil {
			if b, ok := session.Builder.(instancebuilders.InstanceBuilder); ok {
				builder = b
			}
		}
	} else {
		// Create new session (first call)
		sessionID = sessionManager.GenerateSessionID()
		// Initialize with empty loop state - will be populated below
		loopState = nil
		builder = nil
	}

	// Extract provided values from args (all fields except kind and session_id)
	providedValues := make(map[string]any)
	for key, value := range args {
		if key != objects.FieldKeyKind && key != "session_id" {
			providedValues[key] = value
		}
	}

	// Try to load instance builder from registry (if not already loaded)
	if builder == nil {
		builder = instancebuilders.NewForKind(kind, schemaVersion)
	}

	// Process the loop (for validation and completeness checking)
	if loopState == nil {
		// First iteration - process with empty values to get initial state
		loopState, err = applyWithContext(ctx, providedValues, func(vals map[string]any) (*interactive.LoopState, error) {
			return loop.ProcessLoop(kind, vals)
		}, "interactive_process_loop")
		if err != nil {
			return nil, errfmt.Newf("failed to process loop").Wrap(err)
		}
		// Create session with builder (if available)
		if builder != nil {
			sessionID = sessionManager.CreateSessionWithBuilder(kind, loopState, builder, schemaVersion)
		} else {
			sessionID = sessionManager.CreateSession(kind, loopState)
		}
	} else {
		// Subsequent iteration - merge provided values with existing
		mergedValues := make(map[string]any)
		// Copy existing values
		maps.Copy(mergedValues, loopState.ProvidedValues)
		// Override with new provided values
		for k, v := range providedValues {
			mergedValues[k] = v
		}
		// Process loop with merged values
		loopState, err = applyWithContext(ctx, mergedValues, func(vals map[string]any) (*interactive.LoopState, error) {
			return loop.ProcessLoop(kind, vals)
		}, "interactive_process_loop")
		if err != nil {
			return nil, errfmt.Newf("failed to process loop").Wrap(err)
		}
		// Update session (builder is already stored, no need to update it)
		if err := doWithContext(ctx, loopState, func(state *interactive.LoopState) error {
			return sessionManager.UpdateSession(sessionID, state)
		}, "interactive_update_session"); err != nil {
			return nil, errfmt.Newf("failed to update session").Wrap(err)
		}
	}

	// Buffer values into builder (if builder is available)
	if builder != nil {
		for fieldName, value := range providedValues {
			builder.SetField(fieldName, value)
		}
	}

	// Check if loop is complete
	if loopState.IsComplete {
		// All fields provided and valid - create object

		// If we have a builder, use it to build the instance
		if builder != nil {
			instance, err := applyWithContext(ctx, builder, func(b instancebuilders.InstanceBuilder) (any, error) {
				return b.Build()
			}, "interactive_build_instance")
			if err != nil {
				return nil, errfmt.Newf("failed to build instance").Wrap(err)
			}

			instanceMap, ok := instance.(map[string]any)
			if !ok {
				return nil, errfmt.Errorf("instance is not a map[string]any")
			}

			// Use storage.Create() directly when storage provider is available
			if server.storageProvider != nil {
				secCtx, _ := server.secCtx.(*pkgctx.SecurityContext)
				if secCtx == nil {
					secCtx, _ = server.secCtx.(*pkgctx.SecurityContext)
					if secCtx == nil {
						secCtx = pkgctx.NewSystemSecurityContext()
					}
				}
				if err := server.storageProvider.Create(ctx, secCtx, instanceMap); err != nil {
					return nil, errfmt.Newf("failed to create object").Wrap(err)
				}
				sessionManager.DeleteSession(sessionID)
				return map[string]any{
					"success":                 true,
					objects.FieldKeySessionID: sessionID,
					"result": map[string]any{
						objects.FieldKeyID:   instanceMap[objects.FieldKeyID],
						objects.FieldKeyKind: kind,
					},
				}, nil
			}

			// Fallback: write to temp file and use CLI bridge
			tmpFilePath, err := getWithContext(ctx, func() (string, error) {
				tmpFile, err := fileutil.CreateTemp("", "zqk-interactive-*.yaml")
				if err != nil {
					return "", err
				}
				return tmpFile.Name(), tmpFile.Close()
			}, "interactive_create_temp_file")
			if err != nil {
				return nil, errfmt.Newf("failed to create temporary file").Wrap(err)
			}
			defer fileutil.Remove(tmpFilePath)

			if err := doWithContext(ctx, tmpFilePath, func(path string) error {
				return builder.WriteToYAML(instanceMap, path)
			}, "interactive_write_yaml"); err != nil {
				return nil, errfmt.Newf("failed to write instance to temp file").Wrap(err)
			}

			initCtx := &pkgctx.CliInitializationContext{
				ProjectRoot: server.GetProjectRoot(),
			}
			result, err := applyWithContext(ctx, map[string]any{
				"_command_path":      "object create",
				objects.FieldKeyKind: kind,
				"file":               tmpFilePath,
			}, func(args map[string]any) (any, error) {
				return ExecuteCLICommandViaMCPWithContext(
					ctx,
					args,
					server.secCtx.(*pkgctx.SecurityContext),
					initCtx,
				)
			}, "interactive_create_object")
			if err != nil {
				return nil, err
			}

			sessionManager.DeleteSession(sessionID)
			return map[string]any{
				"success":                 true,
				objects.FieldKeySessionID: sessionID,
				"result":                  result,
			}, nil
		}

		// Fallback: use template-based approach (backward compatibility)
		// Write filled template to temporary file and execute: {executable} object create <kind> --file <temp_file>

		// Create temporary file
		execName := GetBrandPrefix()
		tmpFilePath, err := getWithContext(ctx, func() (string, error) {
			tmpFile, err := fileutil.CreateTemp("", execName+"-interactive-*.yaml")
			if err != nil {
				return "", err
			}
			tmpFilePath := tmpFile.Name()
			// Write filled template to temp file
			if _, err := tmpFile.WriteString(loopState.FilledTemplate); err != nil {
				_ = tmpFile.Close()
				return "", err
			}
			if err := tmpFile.Close(); err != nil {
				return "", err
			}
			return tmpFilePath, nil
		}, "interactive_create_template_file")
		if err != nil {
			return nil, errfmt.Newf("failed to create temporary file").Wrap(err)
		}
		defer fileutil.Remove(tmpFilePath) // Clean up temp file

		// Use CLI bridge to execute object create command
		initCtx := &pkgctx.CliInitializationContext{
			ProjectRoot: server.GetProjectRoot(),
		}

		// Execute: {executable} object create <kind> --file <temp_file>
		result, err := applyWithContext(ctx, map[string]any{
			"_command_path":      "object create",
			objects.FieldKeyKind: kind,
			"file":               tmpFilePath,
		}, func(args map[string]any) (any, error) {
			return ExecuteCLICommandViaMCPWithContext(
				ctx,
				args,
				server.secCtx.(*pkgctx.SecurityContext),
				initCtx,
			)
		}, "interactive_create_object")
		if err != nil {
			return nil, err
		}

		// Clean up session
		sessionManager.DeleteSession(sessionID)

		return map[string]any{
			"success":                 true,
			objects.FieldKeySessionID: sessionID,
			"filled_template":         loopState.FilledTemplate,
			"result":                  result,
		}, nil
	}

	// Loop not complete - return elicitation error
	fieldsToElicit := loop.GetFieldsToElicit(loopState)
	elicitationParams := ConvertFieldTokenInfosToElicitationParams(fieldsToElicit)

	return nil, NewElicitationErrorWithData(
		fmt.Sprintf("Missing required fields for %s object. Please provide the following fields:", kind),
		elicitationParams,
		map[string]any{
			objects.FieldKeySessionID: sessionID,
			objects.FieldKeyKind:      kind,
		},
	)
}

// applyWithContext applies a function with context, following the functional API pattern
// This avoids import cycles by providing a local implementation
func applyWithContext[T, U any](ctx context.Context, target T, fn func(T) (U, error), operationType string) (U, error) {
	// Context is available for cancellation/timeout
	_ = ctx
	_ = operationType // Operation type available for future metrics integration
	return fn(target)
}

// doWithContext executes an operation that returns only an error, following the functional API pattern
func doWithContext[T any](ctx context.Context, target T, fn func(T) error, operationType string) error {
	// Context is available for cancellation/timeout
	_ = ctx
	_ = operationType // Operation type available for future metrics integration
	return fn(target)
}

// getWithContext retrieves a value, following the functional API pattern
func getWithContext[T any](ctx context.Context, fn func() (T, error), operationType string) (T, error) {
	// Context is available for cancellation/timeout
	_ = ctx
	_ = operationType // Operation type available for future metrics integration
	return fn()
}
