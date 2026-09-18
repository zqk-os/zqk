package scheduler

import (
	"fmt"
	"path/filepath"
	"strings"

	clicontext "github.com/zqk-os/zqk/internal/cli/context"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/config"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// resolveSchedulerCLIBinary returns the preferred CLI binary path for scheduler-spawned subprocesses.
// Priority:
//  1. ZQK_BIN environment variable (explicit operator override)
//  2. config/zqk.yaml / config/zqk-local.yaml cli.binary_path (explicit project configuration)
//  3. project-local ./bin/zqk or ./zqk (canonical operational binary)
//  4. current executable path
//  5. bare "zqk" (PATH fallback)
func resolveSchedulerCLIBinary(projectRoot string) string {
	// Honor ZQK_BIN only when the path exists; stale Local CI pins must not block fallbacks.
	// TRACK: TDE-1785808957221945000-fcd15e47
	if zqkBin := zqkenv.Bin().Get(); zqkBin != emptyValue {
		if info, err := fileutil.Stat(zqkBin); err == nil && !info.IsDir() {
			return zqkBin
		}
	}

	if configuredPath, settingsBinary, settingsValid := binaryFromSettings(projectRoot); settingsValid {
		_ = configuredPath // Acknowledged
		return settingsBinary
	}

	if projectRoot != emptyValue {
		if path := firstExistingFile(localCLIBinaryCandidates(projectRoot)); path != emptyValue {
			return path
		}
	}

	// In test mode, never fall back to the running process (which is the test runner)
	if zqkenv.IsInTest() {
		return fallbackCLIBinaryName()
	}
	// Try climbing up to the module root to find the compiled binary in the repository
	if wd, err := fileutil.Getwd(); err == nil {
		if root, err := paths.ModuleRootFromPath(wd); err == nil {
			if path := firstExistingFile(localCLIBinaryCandidates(root)); path != emptyValue {
				return path
			}
		}
	}

	if exe, err := fileutil.Executable(); err == nil && exe != emptyValue && !isTestBinary(exe) {
		// Prefer the running binary (including workshop stable). Do not flip stable→tip:
		// that recreates tip/stable split-brain for scheduler-spawned children.
		// TRACK: TDE-1785808957221945000-fcd15e47
		return exe
	}

	return fallbackCLIBinaryName()
}

// SchedulerCLIBinaryConfigWarning returns a warning when cli.binary_path is configured
// but cannot be resolved to an existing file.
func SchedulerCLIBinaryConfigWarning(projectRoot string) string {
	configured, resolved, valid := binaryFromSettings(projectRoot)
	_ = resolved // Acknowledged
	if configured == emptyValue || valid {
		return ""
	}
	return fmt.Sprintf("config cli.binary_path is configured but not found: %s", configured)
}

// ResolveSchedulerDaemonBinary returns the executable path for the detached scheduler daemon.
// Priority:
//  1. ZQK_SCHEDULER_DAEMON_BIN (daemon only; does not affect job subprocesses)
//  2. ZQK_BIN
//  3. config/zqk.yaml / config/zqk-local.yaml cli.binary_path when it resolves to an existing file
//  4. project-local bin/zqk-scheduler when present (avoids go build -o bin/zqk overwriting the running daemon)
//  5. project-local bin/zqk or ./zqk
//  6. os.Executable() (foreground-equivalent binary)
//
// Rationale: background start used os.Executable(), so rebuilding the same path (e.g. bin/zqk) while the
// daemon runs could destabilize or replace the on-disk image; preferring zqk-scheduler matches operational docs.
func ResolveSchedulerDaemonBinary(projectRoot string) (string, error) {
	if v := config.SchedulerDaemonBin().OrDefault(""); v != "" {
		return v, nil
	}
	// Honor ZQK_BIN only when the path exists; stale Local CI pins must not block fallbacks.
	// TRACK: TDE-1785808957221945000-fcd15e47
	if zqkBin := zqkenv.Bin().Get(); zqkBin != emptyValue {
		if info, err := fileutil.Stat(zqkBin); err == nil && !info.IsDir() {
			return zqkBin, nil
		}
	}

	if configuredPath, settingsBinary, settingsValid := binaryFromSettings(projectRoot); settingsValid {
		_ = configuredPath // Acknowledged
		return settingsBinary, nil
	}

	if projectRoot != emptyValue {
		if path := firstExistingFile(localDaemonBinaryCandidates(projectRoot)); path != emptyValue {
			return path, nil
		}
	}

	exe, err := fileutil.Executable()
	if err != nil {
		return "", errfmt.Newf("failed to get executable path").Wrap(err)
	}
	// Never spawn the test runner as the daemon; prefer real project/module binaries.
	// TRACK: BLI-1785443942668406000-1ec5c811 — IsInTest early-return invented missing projectRoot/bin/zqk.
	if zqkenv.IsInTest() || isTestBinary(exe) {
		if path, ok := firstExistingDaemonBinary(projectRoot); ok {
			return path, nil
		}
		if wd, wdErr := fileutil.Getwd(); wdErr == nil {
			if root, rootErr := paths.ModuleRootFromPath(wd); rootErr == nil {
				if path, ok := firstExistingDaemonBinary(root); ok {
					return path, nil
				}
			}
		}
		return "", fmt.Errorf("no scheduler daemon binary found for tests (projectRoot=%s)", projectRoot)
	}
	return exe, nil
}

func firstExistingDaemonBinary(root string) (string, bool) {
	if root == emptyValue {
		return "", false
	}
	path := firstExistingFile(localDaemonBinaryCandidates(root))
	return path, path != emptyValue
}

func localCLIBinaryCandidates(root string) []string {
	exe := brand.ExecutableName()
	cands := []string{
		filepath.Join(root, binDirName, exe),
		filepath.Join(root, exe),
		filepath.Join(root, paths.ProjectDataDir, binDirName, exe),
	}
	if exe != zqkBinaryName {
		cands = append(cands,
			filepath.Join(root, binDirName, zqkBinaryName),
			filepath.Join(root, zqkBinaryName),
		)
	}
	cands = append(cands,
		filepath.Join(root, paths.ProjectDataDir, binDirName, brand.ZqkStableName),
		filepath.Join(root, binDirName, zqkBinaryName),
		filepath.Join(root, zqkBinaryName),
	)
	return uniqueStrings(cands)
}

func localDaemonBinaryCandidates(root string) []string {
	exe := brand.ExecutableName()
	var cands []string
	if exe != zqkBinaryName {
		cands = append(cands,
			filepath.Join(root, binDirName, exe),
			filepath.Join(root, exe),
			filepath.Join(root, paths.ProjectDataDir, binDirName, exe),
		)
	}
	cands = append(cands,
		filepath.Join(root, paths.ProjectDataDir, binDirName, brand.ZqkStableName),
		filepath.Join(root, binDirName, zqkSchedulerBinaryName),
		filepath.Join(root, binDirName, zqkBinaryName),
		filepath.Join(root, zqkBinaryName),
	)
	return uniqueStrings(cands)
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == emptyValue {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func firstExistingFile(candidates []string) string {
	for _, candidate := range candidates {
		if info, err := fileutil.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return emptyValue
}

func fallbackCLIBinaryName() string {
	if name := strings.TrimSpace(brand.ExecutableName()); name != emptyValue {
		return name
	}
	return zqkBinaryName
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
		if info, statErr := fileutil.Stat(path); statErr == nil && !info.IsDir() {
			return path, path, true
		}
		return path, "", false
	}
	candidate := filepath.Join(root, path)
	if info, statErr := fileutil.Stat(candidate); statErr == nil && !info.IsDir() {
		return path, candidate, true
	}
	return path, "", false
}
