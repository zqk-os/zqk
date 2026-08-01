package system

import (
	"path/filepath"
	"regexp"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/logging"

	"github.com/lanceman/zqk/pkg/objects"
)

var (
	reSpecAutoFixerSimpleIdentifier     = regexp.MustCompile(`^[a-z0-9_]+$`)
	reSpecAutoFixerNonIdentifierChars   = regexp.MustCompile(`[^a-z0-9_]`)
	reSpecAutoFixerInvalidNamespaceChar = regexp.MustCompile(`[^a-z0-9_:]`)
	reSpecAutoFixerAlphaDigits          = regexp.MustCompile(`([A-Za-z]+)(\d+)`)
	reSpecAutoFixerDigitsOnly           = regexp.MustCompile(`^\d+$`)
)

// SpecBasedAutoFixer handles spec-based auto-fixing of validation issues
type SpecBasedAutoFixer struct {
	specLoader *objects.SpecLoader
	logger     logging.Logger
	ctx        *cli.Context
}

// getProjectRootFromContext gets the project root from context or finds it
func getProjectRootFromContext(ctx *cli.Context) string {
	return ProjectRootOrResolveDot(ctx.ProjectRoot)
}

// NewSpecBasedAutoFixer creates a new spec-based auto-fixer
func NewSpecBasedAutoFixer(ctx *cli.Context, logger logging.Logger) *SpecBasedAutoFixer {
	projectRoot := getProjectRootFromContext(ctx)

	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	specLoader := objects.NewSpecLoader(specsDir)

	return &SpecBasedAutoFixer{
		specLoader: specLoader,
		logger:     logger,
		ctx:        ctx,
	}
}
