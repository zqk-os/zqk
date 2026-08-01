package system

import (
	stdcontext "context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"gopkg.in/yaml.v3"
)

func reloadHashRegistriesForKinds(stdCtx stdcontext.Context, ctx *cli.Context, kinds []string) {
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)

	for _, kind := range kinds {
		kindDir := getKindDirectory(projectRoot, kind)
		if kindDir == emptyValue {
			continue
		}

		// Create new registry instance and load it
		// This will pick up any newly created files and their hashes
		registry := storage.NewHashRegistry(stdCtx, kind, kindDir)
		if err := registry.Load(); err == nil {
			// Registry loaded successfully - hashes for newly created files should now be available
			// Note: We can't update the hashRegistryCache here because it's not passed to this function
			// But the next time a hash registry is requested for this kind, it will be reloaded
		}
	}
}
func updateHashInRegistry(stdCtx stdcontext.Context, ctx *cli.Context, obj *parser.ParsedObject, filePath, kind string) error {
	// Use file's directory (not kind directory) to handle bucketed objects correctly
	// This ensures we use the correct registry location for both bucketed and non-bucketed objects
	fileDir := filepath.Dir(filePath)

	// Create new registry instance in the file's directory
	registry := storage.NewHashRegistry(stdCtx, kind, fileDir)
	if err := registry.Load(); err != nil {
		// Create new registry if it doesn't exist
	}

	return updateHashInRegistryWithInstance(stdCtx, ctx, obj, filePath, kind, registry)
}
func updateHashInRegistryWithInstance(stdCtx stdcontext.Context, ctx *cli.Context, obj *parser.ParsedObject, filePath, kind string, registry storage.HashRegistryProvider) error {
	logger := logging.GetLoggerFromProfile(ctx.Profile)
	filename := filepath.Base(filePath)
	logging.Fluent(logger).Debug("Updating hash in registry instance").
		String("object_id", obj.ID).
		String("file_path", filePath).
		String("filename", filename).
		Kind(kind).
		String("registry_nil", fmt.Sprintf("%v", registry == nil)).
		Log()

	if registry == nil {
		// Fallback to creating new instance if none provided
		return updateHashInRegistry(stdCtx, ctx, obj, filePath, kind)
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return errfmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Calculate hash from CURRENT file content
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	// Debug logging for hash update (when ZQK_DEBUG_VALIDATION_OBJECT_IDS includes this ID)
	if obj != nil && isDebugValidationObject(obj.ID) {
		logging.Fluent(logger).Debug("Setting hash in registry").
			String("object_id", obj.ID).
			String("filename", filename).
			String("hash_prefix", hashStr[:16]).
			Log()
	}

	// Debug: Log what we're updating (critical for debugging missing hash fixes)
	logging.Fluent(logger).Info("Updating hash in registry").
		String("object_id", obj.ID).
		String("file_path", filePath).
		String("filename", filename).
		String("hash", hashStr[:16]+"...").
		String("registry_file", fmt.Sprintf("%s/.%s.hashes", filepath.Dir(filePath), obj.Kind)).
		Log()

	// Verify filename matches object ID before setting hash
	expectedFilename := obj.ID + ".yaml"
	if filename != expectedFilename {
		logging.Fluent(logger).Warn("Filename mismatch in hash update").
			String("object_id", obj.ID).
			String("expected_filename", expectedFilename).
			String("actual_filename", filename).
			String("file_path", filePath).
			Log()
		// Don't return error - just log warning, as filename might be different for valid reasons
	}

	// Verify we're reading the correct file by checking the object ID in the file content
	// This is a safety check to ensure we're not processing the wrong file
	if obj.ID != emptyValue {
		// Try to parse the file to verify it contains the expected object ID
		var fileObj map[string]any
		if err := yaml.Unmarshal(content, &fileObj); err == nil {
			if fileID, ok := fileObj[objects.FieldKeyID].(string); ok && fileID != obj.ID {
				logging.Fluent(logger).Error("File content ID mismatch", errfmt.Errorf("expected object ID %s, but file contains %s", obj.ID, fileID)).
					String("object_id", obj.ID).
					String("file_id", fileID).
					String("file_path", filePath).
					Log()
				return errfmt.Errorf("file content ID mismatch: expected %s, got %s", obj.ID, fileID)
			}
		}
	}

	registry.SetHash(filename, hashStr)

	// Verify hash was set immediately after setting it
	verifyHash := registry.GetHash(filename)
	if verifyHash != hashStr {
		logging.Fluent(logger).Error("Hash was not set correctly in registry", errfmt.Errorf("expected %s, got %s", hashStr[:16]+"...", verifyHash[:16]+"...")).
			String("object_id", obj.ID).
			String("filename", filename).
			Log()
		return errfmt.Errorf("hash was not set correctly in registry for %s", filename)
	}

	// Save to disk - this is critical, errors here must be returned
	if err := registry.Save(); err != nil {
		return errfmt.Errorf("failed to save hash registry for %s: %w", kind, err)
	}

	return nil
}
