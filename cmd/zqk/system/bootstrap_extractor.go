package system

import (
	"github.com/zqk-os/zqk/internal/bootstrap"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ExtractBootstrapFiles extracts bootstrap files into the project: .zqk/specs and .zqk/cli/specs.
// Implementation lives in internal/bootstrap ( ).
func ExtractBootstrapFiles(projectRoot string, logger logging.Logger, force bool) error {
	return bootstrap.ExtractFiles(projectRoot, logger, force)
}
