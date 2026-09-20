package migration

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	objfield "github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// mstepKey* are migration step YAML config keys and StepResult Output/Metadata map keys
// that are not shared object FieldKey* constants.
const (
	mstepKeyArgs         = "args"
	mstepKeyBuilder      = "builder"
	mstepKeyCascade      = "cascade"
	mstepKeyCount        = "count"
	mstepKeyCreated      = "created"
	mstepKeyDeleted      = "deleted"
	mstepKeyDirectory    = "directory"
	mstepKeyEnv          = "env"
	mstepKeyErrors       = "errors"
	mstepKeyExclude      = "exclude"
	mstepKeyExitCode     = "exit_code"
	mstepKeyFile         = "file"
	mstepKeyFiles        = "files"
	mstepKeyIDs          = "ids"
	mstepKeyItems        = "items"
	mstepKeyPattern      = "pattern"
	mstepKeySkipExisting = "skip_existing"
	mstepKeyStderr       = "stderr"
	mstepKeyStdout       = "stdout"
	mstepKeyTransform    = "transform"
	mstepKeyWorkingDir   = "working_dir"
)

// executeScanFilesStep executes a scan_files step
func (e *Executor) executeScanFilesStep(_ context.Context, step *Step, _ map[string]any) StepResult {
	result := StepResult{
		StepID:   step.ID,
		Output:   make(map[string]any),
		Metadata: make(map[string]any),
	}

	// Parse config
	config := step.Config
	if config == nil {
		result.Error = errfmt.Errorf("scan_files step requires config")
		result.Success = false
		return result
	}

	directory, ok := config[mstepKeyDirectory].(string)
	if !ok || directory == emptyValue {
		result.Error = errfmt.Errorf("scan_files step requires directory in config")
		result.Success = false
		return result
	}

	pattern, ok := config[mstepKeyPattern].(string)
	if !ok {
		pattern = "*"
	}

	var exclude []string
	if excl, ok := config[mstepKeyExclude].([]any); ok {
		exclude = make([]string, len(excl))
		for i, v := range excl {
			if s, ok := v.(string); ok {
				exclude[i] = s
			}
		}
	}

	// Resolve directory path (relative to project root)
	dirPath := directory
	if !filepath.IsAbs(dirPath) {
		dirPath = filepath.Join(e.projectRoot, dirPath)
	}

	// Scan directory
	var files []FileInfo
	entries, err := fileutil.ReadDir(dirPath)
	if err != nil {
		result.Error = errfmt.Errorf("failed to read directory %s: %w", dirPath, err)
		result.Success = false
		return result
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Check exclude patterns
		excluded := false
		for _, excl := range exclude {
			matched, err := filepath.Match(excl, name)
			if err == nil && matched {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		// Check pattern
		matched, err := filepath.Match(pattern, name)
		if err != nil {
			continue
		}
		if !matched {
			continue
		}

		filePath := filepath.Join(dirPath, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, FileInfo{
			Path: filePath,
			Name: name,
			Size: info.Size(),
			Mode: info.Mode(),
		})
	}

	result.Output[mstepKeyFiles] = files
	result.Output[mstepKeyCount] = len(files)
	result.Metadata[mstepKeyCount] = len(files)
	result.Success = true

	return result
}

// FileInfo represents file information
type FileInfo struct {
	Path string
	Name string
	Size int64
	Mode fileutil.FileMode
}

// executeTransformStep executes a transform step
// Supports both file-based (from scan_files) and object-based (from read_objects) transformations
func (e *Executor) executeTransformStep(_ context.Context, _ *Spec, step *Step, stepOutputs map[string]any) StepResult {
	result := StepResult{
		StepID:   step.ID,
		Output:   make(map[string]any),
		Metadata: make(map[string]any),
	}

	// Get source from for_each step
	forEachStepID := step.ForEach
	if forEachStepID == emptyValue {
		result.Error = errfmt.Errorf("transform step requires for_each")
		result.Success = false
		return result
	}

	sourceOutput, ok := stepOutputs[forEachStepID]
	if !ok {
		result.Error = errfmt.Errorf("source step %s not found", forEachStepID)
		result.Success = false
		return result
	}

	// Extract source output map
	sourceOutputMap, ok := sourceOutput.(map[string]any)
	if !ok {
		result.Error = errfmt.Errorf("source step output is not a map")
		result.Success = false
		return result
	}

	// Parse transform config
	config := step.Config
	if config == nil {
		result.Error = errfmt.Errorf("transform step requires config")
		result.Success = false
		return result
	}

	transformConfig, ok := config[mstepKeyTransform].(map[string]any)
	if !ok {
		result.Error = errfmt.Errorf("transform step requires transform config")
		result.Success = false
		return result
	}

	builderName, ok := transformConfig[mstepKeyBuilder].(string)
	if !ok || builderName == emptyValue {
		result.Error = errfmt.Errorf("transform step requires builder in transform config")
		result.Success = false
		return result
	}

	version, ok := transformConfig[objfield.FieldKeyVersion].(string)
	if !ok {
		version = "v1_0_0"
	}

	// Check if source is objects (from read_objects) or files (from scan_files)
	var objects []map[string]any

	if itemsInterface, hasItems := sourceOutputMap[mstepKeyItems]; hasItems {
		// Object-based transformation (from read_objects step)
		items, ok := itemsInterface.([]map[string]any)
		if !ok {
			result.Error = errfmt.Errorf("source step items is not []map[string]any")
			result.Success = false
			return result
		}

		// Handle lifecycle ID transformation
		switch builderName {
		case "lifecycle_id_transform":
			helper := &lifecycleMigrationHelper{
				storageProvider: e.storageProvider,
				projectRoot:     e.projectRoot,
				logger:          e.logger,
			}

			for _, obj := range items {
				transformedObj, err := helper.transformLifecycleID(obj)
				if err != nil {
					result.Error = errfmt.Newf("failed to transform lifecycle object ID").Wrap(err)
					result.Success = false
					return result
				}
				objects = append(objects, transformedObj)
			}
		case "uuid_id_transform":
			// Handle UUID ID transformation (sequential -> UUID with 8-char hex)
			helper, err := newUUIDMigrationHelper(e.storageProvider, e.projectRoot, e.logger)
			if err != nil {
				result.Error = errfmt.Newf("failed to create UUID migration helper").Wrap(err)
				result.Success = false
				return result
			}

			for _, obj := range items {
				transformedObj, err := helper.transformToUUIDID(obj)
				if err != nil {
					result.Error = errfmt.Newf("failed to transform object ID to UUID format").Wrap(err)
					result.Success = false
					return result
				}
				objects = append(objects, transformedObj)
			}
		default:
			result.Error = errfmt.Errorf("unsupported builder for object-based transform: %s", builderName)
			result.Success = false
			return result
		}
	} else if filesInterface, hasFiles := sourceOutputMap[mstepKeyFiles]; hasFiles {
		// File-based transformation (from scan_files step)
		files, ok := filesInterface.([]FileInfo)
		if !ok {
			result.Error = errfmt.Errorf("source step files is not []FileInfo")
			result.Success = false
			return result
		}

		// For lifecycle file to object conversion, use lifecycle helper
		switch builderName {
		case "lifecycle_instance_builder":
			helper := &lifecycleMigrationHelper{
				storageProvider: e.storageProvider,
				projectRoot:     e.projectRoot,
				logger:          e.logger,
			}

			for _, file := range files {
				// Extract object_type from filename
				filename := filepath.Base(file.Name)
				objectType := strings.TrimSuffix(filename, "_lifecycle.yaml")
				objectType = strings.TrimSuffix(objectType, "_lifecycle.yml")

				// Convert lifecycle file to object
				lifecycleObj, err := helper.lifecycleToObjectFromFile(file.Path, objectType, version)
				if err != nil {
					result.Error = errfmt.Errorf("failed to convert lifecycle file %s: %w", file.Path, err)
					result.Success = false
					return result
				}

				objects = append(objects, lifecycleObj)
			}
		case "yaml_file_to_object":
			// Transformation from FileInfo (scan_files) to object map
			for _, file := range files {
				data, err := fileutil.ReadFile(file.Path)
				if err != nil {
					result.Error = errfmt.Errorf("failed to read YAML file %s: %w", file.Path, err)
					result.Success = false
					return result
				}

				var obj map[string]any
				if err := yaml.Unmarshal(data, &obj); err != nil {
					result.Error = errfmt.Errorf("failed to unmarshal YAML file %s: %w", file.Path, err)
					result.Success = false
					return result
				}

				// Ensure kind is set (infer from filename or directory if missing)
				if _, ok := obj[objfield.FieldKeyKind]; !ok {
					// Infer kind from directory name
					kindDir := filepath.Base(filepath.Dir(file.Path))
					obj[objfield.FieldKeyKind] = kindDir
				}

				objects = append(objects, obj)
			}
		default:
			result.Error = errfmt.Errorf("unsupported builder for file-based transform: %s", builderName)
			result.Success = false
			return result
		}
	} else {
		result.Error = errfmt.Errorf("source step output missing both 'items' and 'files' keys")
		result.Success = false
		return result
	}

	result.Output[mstepKeyItems] = objects
	result.Output[mstepKeyCount] = len(objects)
	result.Metadata[mstepKeyCount] = len(objects)
	result.Success = true
	return result
}

// executeCreateObjectsStep executes a create_objects step
func (e *Executor) executeCreateObjectsStep(ctx context.Context, _ *Spec, step *Step, stepOutputs map[string]any, options ExecutionOptions) StepResult {
	result := StepResult{
		StepID:   step.ID,
		Output:   make(map[string]any),
		Metadata: make(map[string]any),
	}

	// Get objects from depends_on step
	config := step.Config
	if config == nil {
		result.Error = errfmt.Errorf("create_objects step requires config")
		result.Success = false
		return result
	}

	skipExisting := true
	if skip, ok := config[mstepKeySkipExisting].(bool); ok {
		skipExisting = skip
	}

	force := options.Force != nil && *options.Force

	// Find source step (should be in depends_on)
	var sourceOutput any
	for _, dep := range step.DependsOn {
		if output, ok := stepOutputs[dep]; ok {
			sourceOutput = output
			break
		}
	}

	if sourceOutput == nil {
		result.Error = errfmt.Errorf("create_objects step requires source from depends_on step")
		result.Success = false
		return result
	}

	// Extract items from source output
	sourceOutputMap, ok := sourceOutput.(map[string]any)
	if !ok {
		result.Error = errfmt.Errorf("source step output is not a map")
		result.Success = false
		return result
	}

	itemsInterface, ok := sourceOutputMap[mstepKeyItems]
	if !ok {
		result.Error = errfmt.Errorf("source step output missing 'items' key")
		result.Success = false
		return result
	}

	items, ok := itemsInterface.([]map[string]any)
	if !ok {
		result.Error = errfmt.Errorf("source step items is not []map[string]any")
		result.Success = false
		return result
	}

	// Check if dry-run mode
	dryRun := options.DryRun != nil && *options.DryRun

	// Create objects
	secCtx := pkgctx.NewSystemSecurityContext()
	created := 0
	skipped := 0
	errors := 0

	for _, obj := range items {
		objID, ok := obj[objfield.FieldKeyID].(string)
		if !ok || objID == emptyValue {
			errors++
			continue
		}

		// Check if object exists
		objectExists := false
		if skipExisting && !force {
			_, err := e.storageProvider.Read(ctx, secCtx, objID)
			if err == nil {
				objectExists = true
				skipped++
				continue
			}
			// If error is not "not found", log but continue
			if err != storage.ErrObjectNotFound && !strings.Contains(err.Error(), "not found") {
				logging.Fluent(e.logger).Warn("Failed to check if object exists").
					ObjectID(objID).
					WithError(err).
					Log()
				// Continue anyway - treat as non-existent for dry-run purposes
			}
		}

		if dryRun {
			// In dry-run mode, just count what would be created
			if !objectExists {
				created++
				logging.Fluent(e.logger).Debug("DRY RUN: Would create object").
					ObjectID(objID).
					Log()
			}
			continue
		}

		// Create object (non-dry-run mode)
		err := e.storageProvider.Create(ctx, secCtx, obj)
		if err != nil {
			if err == storage.ErrObjectExists && skipExisting {
				skipped++
				logging.Fluent(e.logger).Debug("Object already exists, skipping").
					ObjectID(objID).
					Log()
				continue
			}
			kind, _ := obj[objfield.FieldKeyKind].(string)
			logging.Fluent(e.logger).Error("Failed to create object", err).
				ObjectID(objID).
				Kind(kind).
				Log()
			errors++
			continue
		}

		created++
	}

	result.Output[mstepKeyCreated] = created
	result.Output[objfield.FieldKeySkipped] = skipped
	result.Output[mstepKeyErrors] = errors
	result.Metadata[mstepKeyCreated] = created
	result.Metadata[objfield.FieldKeySkipped] = skipped
	result.Metadata[mstepKeyErrors] = errors
	result.Success = errors == 0

	return result
}

// executeReadIDListStep executes a read_id_list step
// Reads a file containing object IDs (YAML list or one-per-line) and outputs them for batch processing
func (e *Executor) executeReadIDListStep(_ context.Context, step *Step, _ map[string]any) StepResult {
	result := StepResult{
		StepID:   step.ID,
		Output:   make(map[string]any),
		Metadata: make(map[string]any),
	}

	// Parse config
	config := step.Config
	if config == nil {
		result.Error = errfmt.Errorf("read_id_list step requires config")
		result.Success = false
		return result
	}

	filePath, ok := config[mstepKeyFile].(string)
	if !ok || filePath == emptyValue {
		result.Error = errfmt.Errorf("read_id_list step requires file in config")
		result.Success = false
		return result
	}

	// Resolve file path (relative to project root)
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(e.projectRoot, filePath)
	}

	// Read file
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		result.Error = errfmt.Errorf("failed to read ID list file %s: %w", filePath, err)
		result.Success = false
		return result
	}

	// Parse IDs - try YAML list first, then fall back to line-by-line
	var ids []string
	var yamlList []string
	if err := yaml.Unmarshal(data, &yamlList); err == nil && len(yamlList) > 0 {
		// Successfully parsed as YAML list
		ids = yamlList
	} else {
		// Parse as line-by-line text file
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != emptyValue && !strings.HasPrefix(line, "#") {
				ids = append(ids, line)
			}
		}
		if err := scanner.Err(); err != nil {
			result.Error = errfmt.Errorf("failed to read ID list file %s: %w", filePath, err)
			result.Success = false
			return result
		}
	}

	result.Output[mstepKeyIDs] = ids
	result.Output[mstepKeyCount] = len(ids)
	result.Metadata[mstepKeyCount] = len(ids)
	result.Success = true

	return result
}

// executeReadObjectsStep executes a read_objects step
// Reads objects from storage by ID list and outputs them for transformation
func (e *Executor) executeReadObjectsStep(ctx context.Context, step *Step, stepOutputs map[string]any) StepResult {
	result := StepResult{
		StepID:   step.ID,
		Output:   make(map[string]any),
		Metadata: make(map[string]any),
	}

	// Get source IDs from depends_on step
	var sourceOutput any
	for _, dep := range step.DependsOn {
		if output, ok := stepOutputs[dep]; ok {
			sourceOutput = output
			break
		}
	}

	if sourceOutput == nil {
		result.Error = errfmt.Errorf("read_objects step requires source from depends_on step")
		result.Success = false
		return result
	}

	// Extract IDs from source output
	sourceOutputMap, ok := sourceOutput.(map[string]any)
	if !ok {
		result.Error = errfmt.Errorf("source step output is not a map")
		result.Success = false
		return result
	}

	idsInterface, ok := sourceOutputMap[mstepKeyIDs]
	if !ok {
		result.Error = errfmt.Errorf("source step output missing 'ids' key")
		result.Success = false
		return result
	}

	ids, ok := idsInterface.([]string)
	if !ok {
		// Try to convert []any to []string
		idsSlice, ok := idsInterface.([]any)
		if !ok {
			result.Error = errfmt.Errorf("source step ids is not []string or []any")
			result.Success = false
			return result
		}
		ids = make([]string, len(idsSlice))
		for i, v := range idsSlice {
			if idStr, ok := v.(string); ok {
				ids[i] = idStr
			} else {
				result.Error = errfmt.Errorf("id at index %d is not a string", i)
				result.Success = false
				return result
			}
		}
	}

	// Read objects from storage
	secCtx := pkgctx.NewSystemSecurityContext()
	var objects []map[string]any
	errors := 0

	for _, id := range ids {
		obj, err := e.storageProvider.Read(ctx, secCtx, id)
		if err != nil {
			logging.Fluent(e.logger).Warn("Failed to read object").
				ObjectID(id).
				WithError(err).
				Log()
			errors++
			continue
		}
		objects = append(objects, obj)
	}

	result.Output[mstepKeyItems] = objects
	result.Output[mstepKeyCount] = len(objects)
	result.Output[mstepKeyErrors] = errors
	result.Metadata[mstepKeyCount] = len(objects)
	result.Metadata[mstepKeyErrors] = errors
	result.Success = errors == 0

	return result
}

// executeDeleteObjectsStep executes a delete_objects step
// Deletes objects from storage by ID list
func (e *Executor) executeDeleteObjectsStep(ctx context.Context, step *Step, stepOutputs map[string]any, options ExecutionOptions) StepResult {
	result := StepResult{
		StepID:   step.ID,
		Output:   make(map[string]any),
		Metadata: make(map[string]any),
	}

	// Parse config (can be empty, defaults will be used)
	config := step.Config
	if config == nil {
		config = make(map[string]any)
	}

	// Get cascade setting (default: false)
	cascade := false
	if cascadeVal, ok := config[mstepKeyCascade].(bool); ok {
		cascade = cascadeVal
	}

	// Get source IDs from depends_on step
	var sourceOutput any
	for _, dep := range step.DependsOn {
		if output, ok := stepOutputs[dep]; ok {
			sourceOutput = output
			break
		}
	}

	if sourceOutput == nil {
		result.Error = errfmt.Errorf("delete_objects step requires source from depends_on step")
		result.Success = false
		return result
	}

	// Extract IDs from source output
	sourceOutputMap, ok := sourceOutput.(map[string]any)
	if !ok {
		result.Error = errfmt.Errorf("source step output is not a map")
		result.Success = false
		return result
	}

	// Support both "ids" (from read_id_list) and "items" (from transform/create)
	var ids []string

	// Try "ids" first (from read_id_list step)
	if idsInterface, hasIDs := sourceOutputMap[mstepKeyIDs]; hasIDs {
		idsSlice, ok := idsInterface.([]string)
		if !ok {
			// Try to convert []any to []string
			idsSliceInterface, ok := idsInterface.([]any)
			if !ok {
				result.Error = errfmt.Errorf("source step ids is not []string or []any")
				result.Success = false
				return result
			}
			ids = make([]string, len(idsSliceInterface))
			for i, v := range idsSliceInterface {
				if idStr, ok := v.(string); ok {
					ids[i] = idStr
				} else {
					result.Error = errfmt.Errorf("id at index %d is not a string", i)
					result.Success = false
					return result
				}
			}
		} else {
			ids = idsSlice
		}
	} else if itemsInterface, hasItems := sourceOutputMap[mstepKeyItems]; hasItems {
		// Extract IDs from items (from transform/create steps)
		items, ok := itemsInterface.([]map[string]any)
		if !ok {
			result.Error = errfmt.Errorf("source step items is not []map[string]any")
			result.Success = false
			return result
		}
		ids = make([]string, len(items))
		for i, item := range items {
			id, ok := item[objfield.FieldKeyID].(string)
			if !ok || id == emptyValue {
				result.Error = errfmt.Errorf("item at index %d missing id field", i)
				result.Success = false
				return result
			}
			ids[i] = id
		}
	} else {
		result.Error = errfmt.Errorf("source step output missing both 'ids' and 'items' keys")
		result.Success = false
		return result
	}

	// Check if dry-run mode
	dryRun := options.DryRun != nil && *options.DryRun

	// Delete objects
	// Mark context as CLI operation to allow deletions
	ctx = storage.WithCLIOperation(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()
	deleted := 0
	skipped := 0
	errors := 0

	for _, id := range ids {
		if dryRun {
			// In dry-run mode, just count what would be deleted
			deleted++
			logging.Fluent(e.logger).Debug("DRY RUN: Would delete object").
				ObjectID(id).
				Log()
			continue
		}

		// Delete object (non-dry-run mode)
		err := e.storageProvider.Delete(ctx, secCtx, id, cascade)
		if err != nil {
			// Check for various "not found" error conditions
			if err == storage.ErrObjectNotFound {
				skipped++
				logging.Fluent(e.logger).Debug("Object not found, skipping").
					ObjectID(id).
					Log()
				continue
			}
			// Also handle "file not found" and "could not infer kind" errors
			// (can occur with CAS, path resolution issues, or invalid IDs)
			errLower := strings.ToLower(err.Error())
			if strings.Contains(errLower, "not found") ||
				strings.Contains(errLower, "file not found") ||
				strings.Contains(errLower, "could not infer kind") ||
				strings.Contains(errLower, "could_not_infer_kind") {
				skipped++
				logging.Fluent(e.logger).Debug("Object not found or invalid ID, skipping").
					ObjectID(id).
					WithError(err).
					Log()
				continue
			}
			logging.Fluent(e.logger).Error("Failed to delete object", err).
				ObjectID(id).
				Log()
			result.Error = err
			errors++
			continue
		}

		deleted++
	}

	result.Output[mstepKeyDeleted] = deleted
	result.Output[objfield.FieldKeySkipped] = skipped
	result.Output[mstepKeyErrors] = errors
	result.Metadata[mstepKeyDeleted] = deleted
	result.Metadata[objfield.FieldKeySkipped] = skipped
	result.Metadata[mstepKeyErrors] = errors
	result.Success = errors == 0
	if errors > 0 {
		// Include the last error if available
		result.Error = errfmt.Errorf("failed to delete %d object(s); last error: %v", errors, result.Error)
	}

	return result
}

// executeCommandStep executes an execute_command step.
// Config: command (required), args (optional []string), env (optional map[string]string), working_dir (optional).
// Runs the command with context cancellation; capturesult stdout, stderr, and exit code.
func (e *Executor) executeCommandStep(ctx context.Context, step *Step, _ map[string]any) StepResult {
	result := StepResult{
		StepID:   step.ID,
		Output:   make(map[string]any),
		Metadata: make(map[string]any),
	}

	config := step.Config
	if config == nil {
		result.Error = errfmt.Errorf("execute_command step requires config")
		result.Success = false
		return result
	}

	command, ok := config[objfield.FieldKeyCommand].(string)
	if !ok || command == emptyValue {
		result.Error = errfmt.Errorf("execute_command step requires command in config")
		result.Success = false
		return result
	}

	var args []string
	if a, ok := config[mstepKeyArgs].([]any); ok {
		args = make([]string, 0, len(a))
		for _, v := range a {
			if s, ok := v.(string); ok {
				args = append(args, s)
			}
		}
	}

	workingDir := e.projectRoot
	if wd, ok := config[mstepKeyWorkingDir].(string); ok && wd != emptyValue {
		if !filepath.IsAbs(wd) {
			workingDir = filepath.Join(e.projectRoot, wd)
		} else {
			workingDir = wd
		}
	}

	cmd := execwrap.CommandContext(ctx, command, args...)
	cmd.Dir = workingDir

	if envMap, ok := config[mstepKeyEnv].(map[string]any); ok && len(envMap) > 0 {
		env := os.Environ()
		for k, v := range envMap {
			if s, ok := v.(string); ok {
				env = append(env, k+"="+s)
			}
		}
		cmd.Env = env
	}

	stdout, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			result.Error = errfmt.Newf("execute_command failed").Wrap(err)
			result.Success = false
			result.Output[mstepKeyStdout] = string(stdout)
			result.Output[mstepKeyStderr] = ""
			result.Output[mstepKeyExitCode] = -1
			result.Metadata[mstepKeyExitCode] = -1
			return result
		}
	}

	result.Output[mstepKeyStdout] = string(stdout)
	result.Output[mstepKeyStderr] = ""
	result.Output[mstepKeyExitCode] = exitCode
	result.Metadata[mstepKeyExitCode] = exitCode
	result.Success = (exitCode == 0)
	if exitCode != 0 {
		result.Error = errfmt.Errorf("command exited with code %d: %s", exitCode, strings.TrimSpace(string(stdout)))
	}
	return result
}
