package utility

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/zqk-os/zqk/cmd/zqk/system"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// layerProcessingState holds the state needed for processing layers
type layerProcessingState struct {
	referenceCache      map[string]bool
	referenceCacheMu    *sync.Mutex
	idStream            map[string]string
	idStreamMu          *sync.Mutex
	activeValidations   *sync.WaitGroup
	validationCounter   *int
	validationCounterMu *sync.Mutex
}

// createObjectInLayer creates a single object within a layer (goroutine body)
// This function is called from within a goroutine for parallel processing
func (sb *ScenarioBuilder) createObjectInLayer(
	ctx context.Context,
	obj map[string]any,
	objIndex, layerIndex int,
	state *layerProcessingState,
	layerCreatedMu, layerSkippedMu *sync.Mutex,
	layerCreated *int,
) error {
	kind, _ := obj[objects.FieldKeyKind].(string)
	objID, _ := obj[objects.FieldKeyID].(string)

	// Resolve references from ID stream (for objects already created in previous layers)
	sb.resolveReferencesFromIDStream(obj, state.idStream, state.idStreamMu)

	// Note: We skip pre-validation of references here because:
	// 1. Storage layer has cache checker set (batch creation mode)
	// 2. Storage will allow non-blocking reference errors during batch creation
	// 3. Objects are created in dependency order (layers), so references will resolve
	// 4. When we control the data, all references should be valid
	// Storage layer validation will catch any real issues and log them as warnings

	// Ensure required fields are set
	if err := sb.prepareObjectFromDataFile(obj, kind); err != nil {
		// Check if error indicates object should be skipped (validation failure)
		if strings.Contains(err.Error(), "missing required fields") ||
			strings.Contains(err.Error(), "validation failed") ||
			strings.Contains(err.Error(), "cannot create") {
			// Object is invalid - emit error and skip
			sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusError,
				"Object validation failed, skipping",
				map[string]any{
					objects.FieldKeyKind:  kind,
					objects.FieldKeyLayer: layerIndex + 1,
					"object_index":        objIndex,
					objects.FieldKeyID:    objID,
					"error":               err.Error(),
				})
			return err
		}
		// Other preparation errors - emit warning
		sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
			"Failed to prepare object",
			map[string]any{
				objects.FieldKeyKind:  kind,
				objects.FieldKeyLayer: layerIndex + 1,
				"object_index":        objIndex,
				objects.FieldKeyID:    objID,
				"error":               err.Error(),
			})
		return err
	}

	// Final validation before creation
	if err := sb.validateObjectBeforeCreation(obj, kind); err != nil {
		sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusError,
			"Object validation failed after preparation, skipping",
			map[string]any{
				objects.FieldKeyKind:  kind,
				objects.FieldKeyLayer: layerIndex + 1,
				"object_index":        objIndex,
				objects.FieldKeyID:    objID,
				"error":               err.Error(),
			})
		return err
	}

	// Create object - objects in same layer have no dependencies on each other
	objKind, _ := obj[objects.FieldKeyKind].(string)
	if objKind == emptyValue {
		objKind = kind
	}

	// Set cache context to ensure Object ID Cache is updated when object is created
	createCtx := pkgctx.WithCacheUpdate(ctx, objID, objKind, "")

	// Track that validation is starting
	// Use defer to ensure Done() is always called, even on panic
	// Track if we added to prevent double Done() calls
	validationAdded := false
	state.activeValidations.Add(1)
	validationAdded = true
	withLock(state.validationCounterMu, func() {
		*state.validationCounter++
	})
	defer func() {
		// Track that validation is complete (always called via defer)
		// Only call Done() if we actually called Add()
		// Recover from any WaitGroup panics to prevent cascading failures
		defer func() {
			if r := recover(); r != nil {
				// Log but don't re-panic - WaitGroup errors shouldn't crash the goroutine
				sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
					fmt.Sprintf("Recovered from WaitGroup panic in defer: %v", r),
					map[string]any{
						objects.FieldKeyKind:  kind,
						"object_id":           objID,
						objects.FieldKeyLayer: layerIndex + 1,
						"panic":               fmt.Sprintf("%v", r),
					})
			}
		}()
		if validationAdded {
			state.activeValidations.Done()
			withLock(state.validationCounterMu, func() {
				*state.validationCounter--
			})
		}
	}()

	// Create object - the cache callback will update the cache synchronously
	// Wrap in recover to catch any panics from storage.Create() (e.g., WaitGroup panics from audit events)
	var createErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Convert panic to error - this handles WaitGroup panics from storage layer
				panicStr := fmt.Sprintf("%v", r)
				if strings.Contains(panicStr, "negative WaitGroup counter") {
					// WaitGroup panic from storage layer - convert to error
					createErr = errfmt.Errorf("storage operation panicked (WaitGroup error): %v", r)
					sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
						fmt.Sprintf("Recovered WaitGroup panic from storage.Create: %v", r),
						map[string]any{
							objects.FieldKeyKind:  kind,
							"object_id":           objID,
							objects.FieldKeyLayer: layerIndex + 1,
							"panic":               panicStr,
						})
				} else {
					// Other panics - re-panic to let goroutinelabels handle it
					panic(r)
				}
			}
		}()
		createErr = sb.storage.Create(createCtx, sb.secCtx, obj)
	}()

	if createErr != nil {
		// Handle duplicate objects
		if strings.Contains(createErr.Error(), "already exists") || strings.Contains(createErr.Error(), "duplicate") || createErr == storagepkg.ErrObjectExists {
			// If --force flag is set, update the existing object instead of skipping
			if sb.force {
				// Get ID from object (may have been set during Create attempt or was already in object)
				updateID := objID
				if idFromObj, ok := obj[objects.FieldKeyID].(string); ok && idFromObj != emptyValue {
					updateID = idFromObj
				}
				// If we have an ID, update the object
				if updateID != emptyValue {
					updateCtx := pkgctx.WithCacheUpdate(ctx, updateID, objKind, "")
					if updateErr := sb.storage.Update(updateCtx, sb.secCtx, updateID, obj); updateErr != nil {
						sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
							"Failed to update existing object with --force",
							map[string]any{
								"object_id":          updateID,
								objects.FieldKeyKind: kind,
								"error":              updateErr.Error(),
							})
						return updateErr
					}
					// Update succeeded - treat as created
					withLock(layerCreatedMu, func() {
						*layerCreated++
					})
					// Add to ID stream and cache (thread-safe)
					withLock(state.idStreamMu, func() {
						state.idStream[updateID] = updateID
						if objID != emptyValue && objID != updateID {
							state.idStream[objID] = updateID
						}
					})
					withLock(state.referenceCacheMu, func() {
						state.referenceCache[updateID] = true
					})
					// Update generated count (thread-safe)
					withLock(&sb.generatedMu, func() {
						sb.generated[kind]++
					})
					// Update cache entry
					if cacheCtx := pkgctx.GetCacheContext(updateCtx); cacheCtx != nil && cacheCtx.FilePath != emptyValue {
						_ = sb.updateObjectIDCacheSync(updateID, kind, cacheCtx.FilePath) //nolint:errcheck
					}
					return nil // Success
				}
				// No ID available - skip (shouldn't happen, but handle gracefully)
				sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
					"Cannot update object with --force: no ID available",
					map[string]any{
						objects.FieldKeyKind:  kind,
						objects.FieldKeyLayer: layerIndex + 1,
						"object_index":        objIndex,
					})
				return createErr
			}
			// No --force flag - skip duplicate
			// Don't return error (not a failure), but don't count as created either
			// The object already exists, so we skip it
			return nil // Success (object already exists, skip silently)
		}
		// Emit error for creation failure
		sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusError,
			fmt.Sprintf("Failed to create object: %v", createErr),
			map[string]any{
				objects.FieldKeyKind:  kind,
				objects.FieldKeyLayer: layerIndex + 1,
				"object_index":        objIndex,
				objects.FieldKeyID:    objID,
				"error":               createErr.Error(),
			})
		return createErr
	}

	// After successful creation, add object ID to ID stream and update cache
	finalID, _ := obj[objects.FieldKeyID].(string)
	if finalID == emptyValue {
		finalID = objID // Use original ID if storage didn't generate one
	}
	if finalID != emptyValue {
		withLock(state.idStreamMu, func() {
			state.idStream[finalID] = finalID
			if objID != emptyValue && objID != finalID {
				state.idStream[objID] = finalID
			}
			// For accounts, also map username to the full account ID
			if kind == objects.KindAccount {
				if username, ok := obj[objects.FieldKeyUsername].(string); ok && username != emptyValue {
					accountID := "" // ACC-* from storage; TRACK: BLI-1785905134201010000-07393484
					if accountID == finalID {
						state.idStream[username] = finalID
					}
				}
			}
		})

		// Update reference cache (thread-safe)
		withLock(state.referenceCacheMu, func() {
			state.referenceCache[finalID] = true
		})

		// Ensure ObjectIDCache is updated for reference validation
		// The cache should be updated automatically during storage.Create via finalizeObjectCreation
		// (for non-CAS objects) or PostSyncCallback (for CAS objects), but we verify and update
		// if needed to ensure system check can find referenced objects immediately
		cache := system.GetGlobalObjectIDCache()
		entry, exists := cache.Get(finalID)
		if !exists || entry == nil || entry.FilePath == emptyValue {
			// Cache entry missing or incomplete - update it
			// First, check cache context from creation (should have been set by storage layer)
			filePath := ""
			if cacheCtx := pkgctx.GetCacheContext(createCtx); cacheCtx != nil && cacheCtx.FilePath != emptyValue {
				filePath = cacheCtx.FilePath
			}
			// If cache context doesn't have path, try to reconstruct it
			// Note: This may not be accurate for bucketed storage, but it's better than nothing
			if filePath == emptyValue {
				filePath = sb.reconstructFilePath(finalID, kind)
			}
			// Update cache if we have a file path
			if filePath != emptyValue {
				if err := sb.updateObjectIDCacheSync(finalID, kind, filePath); err != nil {
					// Log warning but don't fail - cache update is best effort
					sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
						"Failed to update ObjectIDCache after creation",
						map[string]any{
							"object_id":              finalID,
							objects.FieldKeyKind:     kind,
							objects.FieldKeyFilePath: filePath,
							"error":                  err.Error(),
						})
				}
			}
			// Note: For CAS objects, the PostSyncCallback should have updated the cache,
			// but if it didn't (e.g., callback not registered or failed), the manual update above
			// will use the reconstructed path which may not be accurate for hash-based filenames.
			// This is acceptable as a fallback - the cache will be rebuilt on next system check.
		}
	}

	// Object created successfully
	withLock(layerCreatedMu, func() {
		*layerCreated++
	})
	// Update generated count (thread-safe)
	withLock(&sb.generatedMu, func() {
		sb.generated[kind]++
	})

	return nil // Success - object created
}

// processObjectsSequentially processes objects sequentially (legacy fallback)
// This is used when layers are not available for backward compatibility
// Returns the number of objects created and skipped
func (sb *ScenarioBuilder) processObjectsSequentially(ctx context.Context, state *layerProcessingState) (created, skipped int) {
	for i, obj := range sb.dataFileObjects {
		kind, _ := obj[objects.FieldKeyKind].(string)
		objID, _ := obj[objects.FieldKeyID].(string)

		// Resolve references from ID stream (for objects already created)
		sb.resolveReferencesFromIDStream(obj, state.idStream, state.idStreamMu)

		// Verify all referenced objects exist (using memoized cache)
		if err := sb.verifyReferencedObjectsExistMemoized(ctx, obj, kind, state.idStream, state.idStreamMu, state.referenceCache, state.referenceCacheMu); err != nil {
			sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusError,
				fmt.Sprintf("Referenced objects do not exist: %v", err),
				map[string]any{
					objects.FieldKeyKind: kind,
					"index":              i,
					objects.FieldKeyID:   objID,
					"error":              err.Error(),
				})
			skipped++
			continue // Skip this object - it references non-existent objects
		}

		// Ensure required fields are set
		if err := sb.prepareObjectFromDataFile(obj, kind); err != nil {
			// Check if error indicates object should be skipped (validation failure)
			if strings.Contains(err.Error(), "missing required fields") ||
				strings.Contains(err.Error(), "validation failed") ||
				strings.Contains(err.Error(), "cannot create") {
				// Object is invalid - emit error and skip
				sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusError,
					"Object validation failed, skipping",
					map[string]any{
						objects.FieldKeyKind: kind,
						"index":              i,
						objects.FieldKeyID:   objID,
						"error":              err.Error(),
					})
				skipped++
				continue
			}
			// Other preparation errors - emit warning
			sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
				"Failed to prepare object",
				map[string]any{
					objects.FieldKeyKind: kind,
					"index":              i,
					objects.FieldKeyID:   objID,
					"error":              err.Error(),
				})
			skipped++
			continue
		}
		// Final validation before creation
		if err := sb.validateObjectBeforeCreation(obj, kind); err != nil {
			sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusError,
				"Object validation failed after preparation, skipping",
				map[string]any{
					objects.FieldKeyKind: kind,
					"index":              i,
					objects.FieldKeyID:   objID,
					"error":              err.Error(),
				})
			skipped++
			continue
		}

		// Create object - objects are already in topological order (dependencies first)
		objKind, _ := obj[objects.FieldKeyKind].(string)
		if objKind == emptyValue {
			objKind = kind
		}

		// Extract to function to avoid deferInLoop lint warning
		// Returns createCtx (needed for cache update after creation) and error
		createCtx, err := func() (context.Context, error) {
			// Set cache context to ensure Object ID Cache is updated when object is created
			createCtx := pkgctx.WithCacheUpdate(ctx, objID, objKind, "")

			// Track that validation is starting (for sequential path)
			// Use defer to ensure Done() is always called, even on panic
			// Track if we added to prevent double Done() calls
			validationAdded := false
			state.activeValidations.Add(1)
			validationAdded = true
			withLock(state.validationCounterMu, func() {
				*state.validationCounter++
			})
			defer func() {
				// Track that validation is complete (always called via defer)
				// Only call Done() if we actually called Add()
				// Recover from any WaitGroup panics to prevent cascading failures
				defer func() {
					if r := recover(); r != nil {
						// Log but don't re-panic - WaitGroup errors shouldn't crash the goroutine
						sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
							fmt.Sprintf("Recovered from WaitGroup panic in defer: %v", r),
							map[string]any{
								objects.FieldKeyKind: kind,
								objects.FieldKeyID:   objID,
								"panic":              fmt.Sprintf("%v", r),
							})
					}
				}()
				if validationAdded {
					state.activeValidations.Done()
					withLock(state.validationCounterMu, func() {
						*state.validationCounter--
					})
				}
			}()

			// Create object - the cache callback will update the cache synchronously
			// Wrap in recover to catch any panics from storage.Create() (e.g., WaitGroup panics from audit events)
			var createErr error
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Convert panic to error - this handles WaitGroup panics from storage layer
						panicStr := fmt.Sprintf("%v", r)
						if strings.Contains(panicStr, "negative WaitGroup counter") {
							// WaitGroup panic from storage layer - convert to error
							createErr = errfmt.Errorf("storage operation panicked (WaitGroup error): %v", r)
							sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
								fmt.Sprintf("Recovered WaitGroup panic from storage.Create: %v", r),
								map[string]any{
									objects.FieldKeyKind: kind,
									objects.FieldKeyID:   objID,
									"panic":              panicStr,
								})
						} else {
							// Other panics - re-panic (sequential path doesn't have goroutinelabels protection)
							panic(r)
						}
					}
				}()
				createErr = sb.storage.Create(createCtx, sb.secCtx, obj)
			}()
			return createCtx, createErr
		}()

		if err != nil {
			// Handle duplicate objects
			if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") || err == storagepkg.ErrObjectExists {
				// If --force flag is set, update the existing object instead of skipping
				if sb.force {
					// Get ID from object (may have been set during Create attempt or was already in object)
					updateID := objID
					if idFromObj, ok := obj[objects.FieldKeyID].(string); ok && idFromObj != emptyValue {
						updateID = idFromObj
					}
					// If we have an ID, update the object
					if updateID != emptyValue {
						updateCtx := pkgctx.WithCacheUpdate(ctx, updateID, objKind, "")
						if updateErr := sb.storage.Update(updateCtx, sb.secCtx, updateID, obj); updateErr != nil {
							sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
								"Failed to update existing object with --force",
								map[string]any{
									"object_id":          updateID,
									objects.FieldKeyKind: kind,
									"error":              updateErr.Error(),
								})
							skipped++
							continue
						}
						// Update succeeded - treat as created
						created++
						// Add to ID stream and cache
						withLock(state.idStreamMu, func() {
							state.idStream[updateID] = updateID
							if objID != emptyValue && objID != updateID {
								state.idStream[objID] = updateID
							}
						})
						withLock(state.referenceCacheMu, func() {
							state.referenceCache[updateID] = true
						})
						withLock(&sb.generatedMu, func() {
							sb.generated[kind]++
						})
						// Update cache entry
						if cacheCtx := pkgctx.GetCacheContext(updateCtx); cacheCtx != nil && cacheCtx.FilePath != emptyValue {
							_ = sb.updateObjectIDCacheSync(updateID, kind, cacheCtx.FilePath) //nolint:errcheck
						}
						continue
					}
					// No ID available - skip (shouldn't happen, but handle gracefully)
					sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
						"Cannot update object with --force: no ID available",
						map[string]any{
							objects.FieldKeyKind: kind,
							"index":              i,
						})
					skipped++
					continue
				}
				// No --force flag - skip duplicate
				skipped++
				continue
			}
			// Emit error for creation failure
			sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusError,
				fmt.Sprintf("Failed to create object: %v", err),
				map[string]any{
					objects.FieldKeyKind: kind,
					"index":              i,
					objects.FieldKeyID:   objID,
					"error":              err.Error(),
				})
			skipped++
			continue
		}

		// After successful creation, add object ID to ID stream and update cache
		// Note: activeValidations.Done() is called via defer above
		finalID, _ := obj[objects.FieldKeyID].(string)
		if finalID == emptyValue {
			finalID = objID // Use original ID if storage didn't generate one
		}
		if finalID != emptyValue {
			withLock(state.idStreamMu, func() {
				state.idStream[finalID] = finalID
				if objID != emptyValue && objID != finalID {
					state.idStream[objID] = finalID
				}
				// For accounts, also map username to the full account ID
				if kind == objects.KindAccount {
					if username, ok := obj[objects.FieldKeyUsername].(string); ok && username != emptyValue {
						accountID := "" // ACC-* from storage; TRACK: BLI-1785905134201010000-07393484
						if accountID == finalID {
							state.idStream[username] = finalID
						}
					}
				}
			})

			// Update reference cache
			withLock(state.referenceCacheMu, func() {
				state.referenceCache[finalID] = true
			})

			// Ensure ObjectIDCache is updated for reference validation
			// The cache should be updated automatically during storage.Create via finalizeObjectCreation
			// (for non-CAS objects) or PostSyncCallback (for CAS objects), but we verify and update
			// if needed to ensure system check can find referenced objects immediately
			cache := system.GetGlobalObjectIDCache()
			entry, exists := cache.Get(finalID)
			if !exists || entry == nil || entry.FilePath == emptyValue {
				// Cache entry missing or incomplete - update it
				// First, check cache context from creation (should have been set by storage layer)
				filePath := ""
				if cacheCtx := pkgctx.GetCacheContext(createCtx); cacheCtx != nil && cacheCtx.FilePath != emptyValue {
					filePath = cacheCtx.FilePath
				}
				// If cache context doesn't have path, try to reconstruct it
				// Note: This may not be accurate for bucketed storage, but it's better than nothing
				if filePath == emptyValue {
					filePath = sb.reconstructFilePath(finalID, kind)
				}
				// Update cache if we have a file path
				if filePath != emptyValue {
					if err := sb.updateObjectIDCacheSync(finalID, kind, filePath); err != nil {
						// Log warning but don't fail - cache update is best effort
						sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
							"Failed to update ObjectIDCache after creation",
							map[string]any{
								"object_id":              finalID,
								objects.FieldKeyKind:     kind,
								objects.FieldKeyFilePath: filePath,
								"error":                  err.Error(),
							})
					}
				}
				// Note: For CAS objects, the PostSyncCallback should have updated the cache,
				// but if it didn't (e.g., callback not registered or failed), the manual update above
				// will use the reconstructed path which may not be accurate for hash-based filenames.
				// This is acceptable as a fallback - the cache will be rebuilt on next system check.
			}
		}

		created++
		// Update generated count (thread-safe)
		withLock(&sb.generatedMu, func() {
			sb.generated[kind]++
		})
	}
	return created, skipped
}
