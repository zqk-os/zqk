package scheduler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	clicontext "github.com/lanceman/zqk/internal/cli/context"
	"github.com/lanceman/zqk/pkg/brand"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// resolveSchedulerCLIBinary returns the preferred CLI binary path for scheduler-spawned subprocesses.
// Priority:
//  1. ZQK_BIN environment variable (explicit operator override)
//  2. zqk-settings.yaml cli.binary_path (explicit project configuration)
//  3. project-local ./bin/zqk or ./zqk (canonical operational binary)
//  4. current executable path
//  5. bare "zqk" (PATH fallback)
func resolveSchedulerCLIBinary(projectRoot string) string {
	if zqkBin := os.Getenv(zqkenv.Bin()); zqkBin != emptyValue {
		return zqkBin
	}

	if configuredPath, settingsBinary, settingsValid := binaryFromSettings(projectRoot); settingsValid {
		_ = configuredPath // Acknowledged
		return settingsBinary
	}

	if projectRoot != emptyValue {
		for _, candidate := range []string{
			filepath.Join(projectRoot, paths.ProjectDataDir, binDirName, brand.ZqkStableName),
			filepath.Join(projectRoot, binDirName, zqkBinaryName),
			filepath.Join(projectRoot, zqkBinaryName),
		} {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}

	// In test mode, never fall back to the running process (which is the test runner)
	if zqkenv.IsInTest() {
		return zqkBinaryName
	}
	// Try climbing up to the module root to find the compiled binary in the repository
	if wd, err := os.Getwd(); err == nil {
		if root, err := paths.ModuleRootFromPath(wd); err == nil {
			for _, candidate := range []string{
				filepath.Join(root, paths.ProjectDataDir, binDirName, brand.ZqkStableName),
				filepath.Join(root, binDirName, zqkBinaryName),
			} {
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					return candidate
				}
			}
		}
	}

	if exe, err := os.Executable(); err == nil && exe != emptyValue && !isTestBinary(exe) {
		// If current process is a "stable" wrapper name, avoid propagating it to child jobs
		// when canonical zqk is available from the same directory.
		if strings.Contains(strings.ToLower(filepath.Base(exe)), zqkStableBinaryNamePattern) {
			dir := filepath.Dir(exe)
			candidate := filepath.Join(dir, zqkBinaryName)
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
				return candidate
			}
		}
		return exe
	}

	return zqkBinaryName
}

// SchedulerCLIBinaryConfigWarning returns a warning when cli.binary_path is configured
// but cannot be resolved to an existing file.
func SchedulerCLIBinaryConfigWarning(projectRoot string) string {
	configured, resolved, valid := binaryFromSettings(projectRoot)
	_ = resolved // Acknowledged
	if configured == emptyValue || valid {
		return ""
	}
	return fmt.Sprintf("zqk-settings cli.binary_path is configured but not found: %s", configured)
}

// ResolveSchedulerDaemonBinary returns the executable path for the detached scheduler daemon.
// Priority:
//  1. ZQK_SCHEDULER_DAEMON_BIN (daemon only; does not affect job subprocesses)
//  2. ZQK_BIN
//  3. zqk-settings.yaml cli.binary_path when it resolves to an existing file
//  4. project-local bin/zqk-scheduler when present (avoids go build -o bin/zqk overwriting the running daemon)
//  5. project-local bin/zqk or ./zqk
//  6. os.Executable() (foreground-equivalent binary)
//
// Rationale: background start used os.Executable(), so rebuilding the same path (e.g. bin/zqk) while the
// daemon runs could destabilize or replace the on-disk image; preferring zqk-scheduler matches operational docs.
func ResolveSchedulerDaemonBinary(projectRoot string) (string, error) {
	if v := strings.TrimSpace(os.Getenv(zqkenv.SchedulerDaemonBin())); v != emptyValue {
		return v, nil
	}
	if zqkBin := os.Getenv(zqkenv.Bin()); zqkBin != emptyValue {
		return zqkBin, nil
	}

	if configuredPath, settingsBinary, settingsValid := binaryFromSettings(projectRoot); settingsValid {
		_ = configuredPath // Acknowledged
		return settingsBinary, nil
	}

	if projectRoot != emptyValue {
		for _, candidate := range []string{
			filepath.Join(projectRoot, paths.ProjectDataDir, binDirName, brand.ZqkStableName),
			filepath.Join(projectRoot, binDirName, zqkSchedulerBinaryName),
			filepath.Join(projectRoot, binDirName, zqkBinaryName),
			filepath.Join(projectRoot, zqkBinaryName),
		} {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}

	// In test mode, never return the test runner binary as the daemon binary.
	if zqkenv.IsInTest() {
		stableCandidate := filepath.Join(projectRoot, paths.ProjectDataDir, binDirName, brand.ZqkStableName)
		if _, statErr := os.Stat(stableCandidate); statErr == nil {
			return stableCandidate, nil
		}
		return filepath.Join(projectRoot, binDirName, zqkBinaryName), nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", errfmt.Newf("failed to get executable path").Wrap(err)
	}
	if isTestBinary(exe) {
		// If running under test, do not allow spawning test binary as daemon.
		// Try to fallback to the built binary under repository module root first.
		if wd, err := os.Getwd(); err == nil {
			if root, err := paths.ModuleRootFromPath(wd); err == nil {
				for _, candidate := range []string{
					filepath.Join(root, paths.ProjectDataDir, binDirName, brand.ZqkStableName),
					filepath.Join(root, binDirName, zqkBinaryName),
				} {
					if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
						return candidate, nil
					}
				}
			}
		}

		// Fallback to project-local paths
		stableCandidate := filepath.Join(projectRoot, paths.ProjectDataDir, binDirName, brand.ZqkStableName)
		if _, statErr := os.Stat(stableCandidate); statErr == nil {
			return stableCandidate, nil
		}
		return filepath.Join(projectRoot, binDirName, zqkBinaryName), nil
	}
	return exe, nil
}

func isTestBinary(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, ".test") ||
		strings.Contains(path, "/go-build") ||
		strings.Contains(path, "\\go-build") ||
		strings.Contains(base, "test") ||
		strings.Contains(base, "___go_build")
}

func binaryFromSettings(projectRoot string) (configuredPath, resolvedPath string, valid bool) {
	root := projectRoot
	if root == emptyValue {
		resolvedRoot, isImplicit, err := clicontext.ResolveProjectRootFromSettings(".")
		_ = isImplicit // Acknowledged
		if err == nil {
			root = resolvedRoot
		}
	}
	if root == emptyValue {
		return "", "", false
	}
	settings, err := clicontext.LoadBrandSettings(root)
	if err != nil || settings == nil {
		return "", "", false
	}
	path := strings.TrimSpace(settings.CLI.BinaryPath)
	if path == emptyValue {
		return "", "", false
	}
	if filepath.IsAbs(path) {
		if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
			return path, path, true
		}
		return path, "", false
	}
	candidate := filepath.Join(root, path)
	if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
		return path, candidate, true
	}
	return path, "", false
}
