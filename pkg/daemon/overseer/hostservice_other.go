//go:build !darwin
// +build !darwin

package overseer

import (
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// LaunchAgentsDir is unsupported on non-Darwin platforms.
func LaunchAgentsDir() (string, error) {
	return "", errfmt.Errorf("LaunchAgents are only supported on macOS (darwin)")
}

// CleanLegacyLaunchAgents is a no-op on non-Darwin platforms.
func CleanLegacyLaunchAgents() ([]string, error) {
	return nil, nil
}

// InstallOverseerLaunchAgent is unsupported on non-Darwin platforms.
func InstallOverseerLaunchAgent(projectRoot, binaryPath string) (*LaunchAgentStatus, error) {
	return nil, errfmt.Errorf("LaunchAgent service management is only supported on macOS (darwin); on Linux use systemd units")
}

// UninstallOverseerLaunchAgent is unsupported on non-Darwin platforms.
func UninstallOverseerLaunchAgent() error {
	return errfmt.Errorf("LaunchAgent service management is only supported on macOS (darwin); on Linux use systemd units")
}

// StatusOverseerLaunchAgent reports unsupported on non-Darwin platforms.
func StatusOverseerLaunchAgent() (*LaunchAgentStatus, error) {
	return &LaunchAgentStatus{
		Label: OverseerLaunchAgentLabel,
	}, nil
}
