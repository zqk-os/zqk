package system

import (
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/systemcheck"
)

// Type aliases keep cmd/zqk/system call sites stable while check domain types
// live in pkg/systemcheck (TRACK: BLI-CEF-ARCH-SYSTEM-TRANCHE1).
type (
	CheckResult           = systemcheck.CheckResult
	Issue                 = systemcheck.Issue
	CheckSnapshot         = systemcheck.CheckSnapshot
	CheckSnapshotMetadata = systemcheck.CheckSnapshotMetadata
	ResolutionResult      = systemcheck.ResolutionResult
	ViolationResolver     = systemcheck.ViolationResolver
)

// NewViolationResolver creates a violation resolver (pkg/systemcheck).
func NewViolationResolver(projectRoot string, specLoader *objects.SpecLoader, logger logging.Logger) *ViolationResolver {
	return systemcheck.NewViolationResolver(projectRoot, specLoader, logger)
}
