package system

import (
	"github.com/lanceman/zqk/internal/bootstrap"
	"github.com/lanceman/zqk/pkg/logging"
)

// ExtractBootstrapFiles extracts bootstrap files into the project: .zqk/specs and .zqk/cli/specs.
// Implementation lives in internal/bootstrap (TRACK: BLI-CEF-ARCH-SYSTEM-TRANCHE1).
func ExtractBootstrapFiles(projectRoot string, logger logging.Logger, force bool) error {
	return bootstrap.ExtractFiles(projectRoot, logger, force)
}
