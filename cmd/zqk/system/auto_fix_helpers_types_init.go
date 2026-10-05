package system

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/migration/parser"
)

// AutoFixContext groups state for auto-fix operations
type AutoFixContext struct {
	Ctx               *cli.Context
	Cmd               *cobra.Command
	Obj               *parser.ParsedObject
	FilePath          string
	Kind              string
	AutoFix           bool
	Force             bool
	Logger            logging.Logger
	HashRegistryCache *HashRegistryCacheType
	ObjectIDCache     *ObjectIDCache
	// Efficiency improvements: track processed objects and batch hash mismatches
	ProcessedObjects map[string]bool    // Track which objects have been processed in this run
	HashMismatches   []HashMismatchInfo // Collect hash mismatches for batch processing
}

// initializeAutoFixContext sets up the auto-fix context
func initializeAutoFixContext(ctx *cli.Context, cmd *cobra.Command, obj *parser.ParsedObject, filePath, kind string, hashRegistryCache *HashRegistryCacheType, objectIDCache *ObjectIDCache) *AutoFixContext {
	logger := logging.GetLoggerFromProfile(ctx.Profile)
	autoFix, _ := cmd.Flags().GetBool("auto-fix")
	force, _ := cmd.Flags().GetBool("force")

	// ProcessedObjects and HashMismatches are initialized as nil
	// They can be set by the caller if shared state is needed across multiple objects
	return &AutoFixContext{
		Ctx:               ctx,
		Cmd:               cmd,
		Obj:               obj,
		FilePath:          filePath,
		Kind:              kind,
		AutoFix:           autoFix,
		Force:             force,
		Logger:            logger,
		HashRegistryCache: hashRegistryCache,
		ObjectIDCache:     objectIDCache,
		ProcessedObjects:  nil, // Will be initialized if needed
		HashMismatches:    nil, // Will be initialized if needed
	}
}
