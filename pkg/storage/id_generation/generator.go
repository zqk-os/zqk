package id_generation

import (
	"context"
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

// Generator provides ID generation using configurable strategies
type Generator struct {
	idValidator *validation.IDValidator
	projectRoot string
}

// NewGenerator creates a new ID generator
func NewGenerator(idValidator *validation.IDValidator, projectRoot string) *Generator {
	return &Generator{
		idValidator: idValidator,
		projectRoot: projectRoot,
	}
}

// GenerateNextID generates the next ID for an object kind using the configured strategy
// If no strategy is configured, uses the default sequential strategy
func (g *Generator) GenerateNextID(ctx context.Context, kind string, strategyConfig StrategyConfig) (string, error) {
	// Load ID patterns
	if err := g.idValidator.LoadPatterns(); err != nil {
		return "", errfmt.Newf(ConstFailedToLoadIDPatterns).Wrap(err)
	}

	// Get valid prefixes for this kind
	prefixes := g.idValidator.GetValidPrefixes(kind)
	if len(prefixes) == 0 {
		return "", errfmt.Errorf(ConstNoIDPrefixConfiguredForKindS, kind)
	}

	// Use the first prefix (most common)
	prefix := prefixes[0]
	// Remove trailing dash if present (validator returns "ITEM-", we need "BLI")
	if len(prefix) > 0 && prefix[len(prefix)-1] == '-' {
		prefix = prefix[:len(prefix)-1]
	}

	// Get directory for this kind to collect existing IDs
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return "", errfmt.Errorf(ConstUnknownObjectKindS, kind)
	}

	kindDir := datacell.CellCASPrimaryDir(g.projectRoot, dirName)

	// Get strategy
	var strategy Strategy
	if strategyConfig.Strategy != emptyValue {
		strategy = GetStrategy(strategyConfig.Strategy)
		if strategy == nil {
			return "", errfmt.Errorf(ConstUnknownIDGenerationStrategyS, strategyConfig.Strategy)
		}
	} else {
		// Use default sequential strategy
		strategy = GetDefaultStrategy()
		if strategy == nil {
			return "", errfmt.Errorf(ConstDefaultSequentialStrategyNotRegistered)
		}
	}

	// For strategies that support parameters (like SequentialStrategy), create a new instance with params
	var minDigits, startAt = 3, 1
	if strategyConfig.Params != nil {
		if md, ok := strategyConfig.Params["min_digits"].(int); ok && md > 0 {
			minDigits = md
		}
		if sa, ok := strategyConfig.Params["start_at"].(int); ok && sa >= 0 {
			startAt = sa
		}
	}

	// For sequential strategy, use thread-safe batch generator directly
	if strategyConfig.Strategy == emptyValue || strategyConfig.Strategy == "sequential" {
		// Get buffer size from config (default: 0 = no queue, use direct generation)
		bufferSize := 0
		if strategyConfig.BufferSize > 0 {
			bufferSize = strategyConfig.BufferSize
		} else if strategyConfig.Params != nil {
			// Check params for buffer_size (backward compatibility)
			if bs, ok := strategyConfig.Params["buffer_size"].(int); ok && bs > 0 {
				bufferSize = bs
			}
		}

		// Use thread-safe batch generator with optional queue buffer
		batchGenerator := GetBatchIDGeneratorWithBuffer(ctx, kindDir, kind, prefix, minDigits, startAt, bufferSize)
		return batchGenerator.GenerateNextID()
	}

	// For other strategies (UUID, etc.), use legacy path with existing IDs collection
	// Collect existing IDs from directory (needed for UUID collision detection)
	existingIDs, err := g.collectExistingIDs(kindDir, kind)
	if err != nil {
		return "", errfmt.Newf(ConstFailedToCollectExistingIDs).Wrap(err)
	}

	if len(strategyConfig.Params) > 0 && strategyConfig.Strategy == "uuid" {
		strategy = NewUUIDStrategyWithParams(strategyConfig.Params)
	}

	// Generate next ID using strategy
	return strategy.GenerateNextID(ctx, kind, prefix, existingIDs)
}

// collectExistingIDs collects existing object IDs from the directory
func (g *Generator) collectExistingIDs(kindDir string, kind string) ([]string, error) {
	var existingIDs []string

	entries, err := os.ReadDir(kindDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, errfmt.Newf(ConstFailedToReadDirectory).Wrap(err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		// Extract ID from filename (remove .yaml/.yml extension)
		filenameID := entry.Name()
		filenameID = filepath.Base(filenameID)
		filenameID = filenameID[:len(filenameID)-len(ext)]

		// For accounts, skip (they use account:username format)
		if kind == objects.KindAccount && filepath.Base(filenameID) != filenameID {
			continue
		}

		existingIDs = append(existingIDs, filenameID)
	}

	return existingIDs, nil
}
