package system

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// disabledHookSuffix marks a hook as intentionally off. Git only runs a file
	// named exactly after the hook, so renaming it out of the way is the standard
	// way to disable one, and it stays visible in `ls .git/hooks`.
	disabledHookSuffix = ".disabled"
	// gitSampleHookSuffix marks the inert examples git creates in every repo.
	gitSampleHookSuffix = ".sample"
	ownerExecuteBit     = 0o100
)

// installHook copies a hook template into place with the executable mode git
// requires before it will run the hook.
func installHook(templatePath, installedPath string) error {
	return fileutil.CopyExecutableFile(templatePath, installedPath)
}

// NewSyncGitHooksCmd creates a new command to synchronize git hooks from templates.
func NewSyncGitHooksCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemSyncGitHooksCommandBuilder()
	cli.BindAsyncProgress(cmd, runSyncGitHooks)
	return cmd
}

func runSyncGitHooks(cmd *cobra.Command, args []string) error {
	_ = args // unused
	projectRoot := cli.ResolveProjectRoot(".")

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	sourceDir, _ := cmd.Flags().GetString("source")
	targetDir, _ := cmd.Flags().GetString("target")
	listHooks, _ := cmd.Flags().GetBool("list")
	disableHook, _ := cmd.Flags().GetString("disable")
	enableHook, _ := cmd.Flags().GetString("enable")

	templatesDir := sourceDir
	if !filepath.IsAbs(templatesDir) {
		templatesDir = filepath.Join(projectRoot, templatesDir)
	}
	installDir := targetDir
	if !filepath.IsAbs(installDir) {
		installDir = filepath.Join(projectRoot, installDir)
	}

	// Inspecting or toggling installed hooks does not need the templates.
	switch {
	case listHooks:
		return listGitHooks(cmd, installDir)
	case disableHook != "":
		return setGitHookEnabled(cmd, installDir, disableHook, false)
	case enableHook != "":
		return setGitHookEnabled(cmd, installDir, enableHook, true)
	}

	// Read templates directory dynamically
	files, err := fileutil.ReadDir(templatesDir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			cmd.Printf("⚠️  Source templates directory not found: %s; skipping hook sync\n", sourceDir)
			return nil
		}
		return fmt.Errorf("failed to read source directory %s: %w", templatesDir, err)
	}

	if _, err := fileutil.Stat(installDir); fileutil.IsNotExist(err) {
		cmd.Printf("⚠️  Target hooks directory not found: %s; skipping hook sync\n", targetDir)
		return nil
	}

	// Valid standard Git hooks list
	validHooks := map[string]bool{
		"applypatch-msg": true, "pre-applypatch": true, "post-applypatch": true,
		"pre-commit": true, "pre-merge-commit": true, "prepare-commit-msg": true,
		"commit-msg": true, "post-commit": true, "pre-rebase": true,
		"post-checkout": true, "post-merge": true, "pre-push": true,
		"pre-receive": true, "update": true, "proc-receive": true,
		"post-receive": true, "post-update": true, "reference-transaction": true,
		"push-to-checkout": true, "pre-auto-gc": true, "post-rewrite": true,
		"sendemail-validate": true, "fsmonitor-watchman": true, "p4-changelist": true,
		"p4-prepare-changelist": true, "p4-post-changelist": true, "p4-pre-submit": true,
	}

	updated := false
	outOfSync := false

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		hook := file.Name()
		if !validHooks[hook] {
			continue
		}

		templatePath := filepath.Join(templatesDir, hook)
		installedPath := filepath.Join(installDir, hook)

		// An operator who turned a hook off keeps it off; sync must not quietly
		// re-arm it on the next commit.
		if _, err := fileutil.Stat(installedPath + disabledHookSuffix); err == nil {
			cmd.Printf("⏸  Skipping %s hook (disabled)\n", hook)
			continue
		}

		// Check if installed file exists
		info, err := fileutil.Stat(installedPath)
		if fileutil.IsNotExist(err) {
			outOfSync = true
			if dryRun {
				cmd.Printf("⚠️  Hook out of sync: %s (not installed)\n", hook)
			} else {
				if err := installHook(templatePath, installedPath); err != nil {
					return fmt.Errorf("failed to copy hook %s: %w", hook, err)
				}
				cmd.Printf("✅ Installed %s hook from template\n", hook)
				updated = true
			}
			continue
		}

		// Compare files
		equal, err := compareFiles(templatePath, installedPath)
		if err != nil {
			return fmt.Errorf("failed to compare hook %s: %w", hook, err)
		}

		if !equal {
			outOfSync = true
			if dryRun {
				cmd.Printf("⚠️  Hook out of sync: %s (template differs from installed)\n", hook)
			} else {
				if err := installHook(templatePath, installedPath); err != nil {
					return fmt.Errorf("failed to update hook %s: %w", hook, err)
				}
				cmd.Printf("✅ Updated %s hook from template (was out of sync)\n", hook)
				updated = true
			}
			continue
		}

		// Content matching is not enough. Git skips a hook that lost its
		// executable bit, printing only a hint, so an up-to-date hook can sit
		// there doing nothing. Content comparison alone never noticed.
		if info.Mode().Perm()&ownerExecuteBit == 0 {
			outOfSync = true
			if dryRun {
				cmd.Printf("⚠️  Hook out of sync: %s (installed but not executable; git ignores it)\n", hook)
			} else {
				if err := fileutil.EnsureExecutable(installedPath); err != nil {
					return fmt.Errorf("failed to make hook %s executable: %w", hook, err)
				}
				cmd.Printf("✅ Repaired %s hook permissions (was not executable, so git ignored it)\n", hook)
				updated = true
			}
		}
	}

	if dryRun && outOfSync {
		cmd.Printf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("Run 'zqk system sync-git-hooks --source %s --target %s' to update hooks.\n", sourceDir, targetDir)))
		return fmt.Errorf("git hooks are out of sync")
	}

	if updated {
		cmd.Printf("   Hooks are now current with %s/\n", sourceDir)
	} else if !outOfSync {
		cmd.Println("   All git hooks are up to date.")
	}

	return nil
}

// listGitHooks reports each installed hook and whether git will actually run it.
// A hook counts as off when it is parked as <hook>.disabled or when it lost its
// executable bit, since git skips both and says so only in a hint.
func listGitHooks(cmd *cobra.Command, installDir string) error {
	entries, err := fileutil.ReadDir(installDir)
	if err != nil {
		return fmt.Errorf("failed to read hooks directory %s: %w", installDir, err)
	}

	found := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasSuffix(name, gitSampleHookSuffix) {
			continue
		}
		hook := strings.TrimSuffix(name, disabledHookSuffix)
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("failed to stat hook %s: %w", name, err)
		}

		found = true
		switch {
		case strings.HasSuffix(name, disabledHookSuffix):
			cmd.Printf("  %-22s disabled\n", hook)
		case info.Mode().Perm()&ownerExecuteBit == 0:
			cmd.Printf("  %-22s disabled (not executable; run sync-git-hooks to repair)\n", hook)
		default:
			cmd.Printf("  %-22s enabled\n", hook)
		}
	}
	if !found {
		cmd.Printf("No git hooks installed in %s\n", installDir)
	}
	return nil
}

// setGitHookEnabled parks a hook as <hook>.disabled or restores it. Renaming is
// how git itself ships inactive hooks (*.sample), so the state is obvious from
// a directory listing and needs no extra config file to stay in sync.
func setGitHookEnabled(cmd *cobra.Command, installDir, hook string, enable bool) error {
	activePath := filepath.Join(installDir, hook)
	disabledPath := activePath + disabledHookSuffix

	from, to := activePath, disabledPath
	if enable {
		from, to = disabledPath, activePath
	}

	if _, err := fileutil.Stat(from); err != nil {
		if fileutil.IsNotExist(err) {
			if _, err := fileutil.Stat(to); err == nil {
				cmd.Printf("Hook %s is already %s\n", hook, enabledWord(enable))
				return nil
			}
			return fmt.Errorf("hook %s not found in %s", hook, installDir)
		}
		return fmt.Errorf("failed to stat hook %s: %w", hook, err)
	}

	if err := fileutil.RenameFile(from, to); err != nil {
		return fmt.Errorf("failed to %s hook %s: %w", enabledVerb(enable), hook, err)
	}
	if enable {
		// A parked hook may also have lost its executable bit; restoring the
		// name alone would leave git still ignoring it.
		if err := fileutil.EnsureExecutable(to); err != nil {
			return fmt.Errorf("failed to make hook %s executable: %w", hook, err)
		}
	}

	cmd.Printf("✅ Hook %s %s\n", hook, enabledWord(enable))
	return nil
}

func enabledWord(enable bool) string {
	if enable {
		return "enabled"
	}
	return "disabled"
}

func enabledVerb(enable bool) string {
	if enable {
		return "enable"
	}
	return "disable"
}

func compareFiles(path1, path2 string) (bool, error) {
	b1, err := fileutil.ReadFile(path1)
	if err != nil {
		return false, err
	}
	b2, err := fileutil.ReadFile(path2)
	if err != nil {
		return false, err
	}
	// Normalize line endings to prevent false positives on Windows/Mac differences
	norm1 := bytes.ReplaceAll(b1, []byte("\r\n"), []byte("\n"))
	norm2 := bytes.ReplaceAll(b2, []byte("\r\n"), []byte("\n"))
	return bytes.Equal(norm1, norm2), nil
}

const defaultPreCommitHookScript = `#!/bin/sh
#
# Pre-commit hook for ZQK
# Fail-closed enforcement for projects initialized with ZQK:
# 0. Branch Protection Gate: Direct commits to main/master prohibited (POL-WORKFLOW-002)
# 1. CAS Membrane Integrity Gate (Prevent hand-editing of files past the CAS membrane)
# 2. Knowledge Kernel CAS & Referential Integrity Gate (zqk system check)
# 3. Test-Driven Development & Lineage Gate (zqk test dashboard --check-dod)
# 4. Secret & Credential Leak Scanning Gate (scripts/scan-secrets.sh)
# 5. Codebase Verification Gate (zqk-vet hygiene & tree police)

set -e

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT" || exit 1

# If this repository does not contain a .zqk directory, nothing to enforce
if [ ! -d "$REPO_ROOT/.zqk" ]; then
	exit 0
fi

# Locate ZQK binary (local repo bin, or system PATH)
ZQK_BIN=""
if [ -x "$REPO_ROOT/bin/zqk" ]; then
	ZQK_BIN="$REPO_ROOT/bin/zqk"
elif [ -x "$REPO_ROOT/bin/zcom" ]; then
	ZQK_BIN="$REPO_ROOT/bin/zcom"
elif command -v zqk >/dev/null 2>&1; then
	ZQK_BIN="$(command -v zqk)"
elif command -v zcom >/dev/null 2>&1; then
	ZQK_BIN="$(command -v zcom)"
fi

if [ -z "$ZQK_BIN" ]; then
	# ZQK binary not installed or in PATH; allow commit to proceed cleanly
	exit 0
fi

# 0. Branch Protection Gate (POL-WORKFLOW-002)
# Committing directly to protected branches (main, master) is strictly prohibited.
CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")"
if [ "$CURRENT_BRANCH" = "main" ] || [ "$CURRENT_BRANCH" = "master" ]; then
	if [ "${ZQK_ALLOW_MAIN_COMMIT}" != "1" ]; then
		echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
		echo "❌ [ZQK PRE-COMMIT] FAIL-CLOSED BRANCH PROTECTION VIOLATION" >&2
		echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
		echo "Direct commit to protected branch '$CURRENT_BRANCH' is strictly forbidden." >&2
		echo "" >&2
		echo "All code changes MUST be committed on a topic/feature/integration branch" >&2
		echo "and merged through a validated Pull Request (POL-WORKFLOW-002)." >&2
		echo "" >&2
		echo "Remediation:" >&2
		echo "  1. Switch to a feature/integration branch:" >&2
		echo "     git checkout -b <branch-name>" >&2
		echo "  2. Commit your staged changes there:" >&2
		echo "     git commit -m \"...\"" >&2
		echo "  3. Open a Pull Request once tests and criteria pass:" >&2
		echo "     gh pr create --fill" >&2
		echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
		exit 1
	fi
fi

# 1. CAS Membrane Integrity Gate (Prevent hand-editing of files past the CAS membrane)
# Any staged file under .zqk/process/**/*.yaml must:
# a) Have its filename match sha256 of its staged content (CAS content-addressable hash invariant)
# b) Not be in a preliminary status (conceptual) - those must remain on the draft plane (.zqk/object_drafts/)
STAGED_CAS_FILES=$(git diff --cached --name-only --diff-filter=ACMR | grep -E '^\.zqk/process/.*\.ya?ml$' || true)
if [ -n "$STAGED_CAS_FILES" ]; then
	SHASUM_CMD=""
	if command -v sha256sum >/dev/null 2>&1; then
		SHASUM_CMD="sha256sum"
	elif command -v shasum >/dev/null 2>&1; then
		SHASUM_CMD="shasum -a 256"
	fi

	for f in $STAGED_CAS_FILES; do
		# Verify preliminary status (conceptual) is prohibited in .zqk/process/ (draft status may be valid in CAS)
		if git show ":$f" 2>/dev/null | grep -E '^[[:space:]]*status:[[:space:]]*["'\'']?conceptual["'\'']?[[:space:]]*$' >/dev/null 2>&1; then
			echo "❌ [ZQK PRE-COMMIT] CAS Membrane Violation: Preliminary status (conceptual) detected in $f"
			echo "   Objects with preliminary status (conceptual) must remain on the draft plane (.zqk/object_drafts/)."
			echo "   Promote the object via '$ZQK_BIN object promote' before committing to CAS."
			exit 1
		fi

		# Verify filename equals sha256 hash of staged content
		base=$(basename "$f" | sed -E 's/\.ya?ml$//')
		if echo "$base" | grep -E '^[0-9a-fA-F]{64}$' >/dev/null 2>&1; then
			if [ -n "$SHASUM_CMD" ]; then
				actual_hash=$(git show ":$f" 2>/dev/null | $SHASUM_CMD | awk '{print $1}')
				if [ "$base" != "$actual_hash" ]; then
					echo "❌ [ZQK PRE-COMMIT] CAS Membrane Violation: Hash mismatch in $f"
					echo "   Expected hash: $base"
					echo "   Actual hash:   $actual_hash"
					echo "   Direct hand-editing of objects past the CAS membrane (.zqk/process/) is strictly prohibited."
					echo "   Use '$ZQK_BIN object update' or CLI mutation commands to update objects cleanly."
					exit 1
				fi
			fi
		fi
	done
fi

# 2. Knowledge Kernel CAS & Referential Integrity Gate
if [ -d "$REPO_ROOT/.zqk/process" ]; then
	echo "🔍 [ZQK PRE-COMMIT] Verifying Knowledge Kernel integrity..."
	if ! "$ZQK_BIN" system check --tier 1; then
		echo "❌ [ZQK PRE-COMMIT] System check failed! Blocking violations found in kernel graph."
		echo "   Run '$ZQK_BIN system check --details' to inspect and resolve."
		exit 1
	fi
fi

# 3. Test-Driven Development (TDD) Definition of Done Verification
if [ -d "$REPO_ROOT/.zqk/process/test_cases" ] || [ -f "$REPO_ROOT/.zqk/state/test_dashboard_lite.json" ]; then
	echo "⚡ [ZQK PRE-COMMIT] Verifying Test Matrix Definition of Done..."
	if ! "$ZQK_BIN" test dashboard --check-dod; then
		echo "❌ [ZQK PRE-COMMIT] Definition of Done validation failed! Broken lineage detected."
		echo "   Run '$ZQK_BIN test dashboard' to inspect broken bindings."
		exit 1
	fi
fi

# 4. Secret & Credential Leak Scanning Gate
if [ -f "$REPO_ROOT/scripts/scan-secrets.sh" ]; then
	echo "🔒 [ZQK PRE-COMMIT] Scanning repository for credentials and secrets..."
	if ! /bin/sh "$REPO_ROOT/scripts/scan-secrets.sh" "$REPO_ROOT"; then
		echo "❌ [ZQK PRE-COMMIT] Potential secrets detected! Commit aborted."
		exit 1
	fi
fi

export PATH="/usr/local/go/bin:/opt/homebrew/bin:$HOME/go/bin:$GOPATH/bin:$PATH"

# 5. Codebase Verification Gate (zqk-vet)
# Enforces AST hygiene (paths, permissions, CLI names, raw goroutines), tree policing, and invariants.
ZQK_VET_BIN=""

# Rebuild zqk-vet if missing or if sources/config are newer than the binary
NEED_VET_BUILD=0
if [ ! -x "$REPO_ROOT/bin/zqk-vet" ]; then
	NEED_VET_BUILD=1
elif [ -n "$(find "$REPO_ROOT/cmd/zqk-vet" "$REPO_ROOT/pkg/vet" "$REPO_ROOT/config/gates.yaml" -newer "$REPO_ROOT/bin/zqk-vet" 2>/dev/null)" ]; then
	NEED_VET_BUILD=1
fi

if [ "$NEED_VET_BUILD" -eq 1 ]; then
	if command -v go >/dev/null 2>&1 && [ -f "$REPO_ROOT/cmd/zqk-vet/main.go" ]; then
		echo "⚙️  [ZQK PRE-COMMIT] Compiling bin/zqk-vet..."
		mkdir -p "$REPO_ROOT/bin"
		go build -trimpath -o "$REPO_ROOT/bin/zqk-vet" "$REPO_ROOT/cmd/zqk-vet" 2>/dev/null || true
	fi
fi

if [ -x "$REPO_ROOT/bin/zqk-vet" ]; then
	ZQK_VET_BIN="$REPO_ROOT/bin/zqk-vet"
elif command -v zqk-vet >/dev/null 2>&1; then
	ZQK_VET_BIN="$(command -v zqk-vet)"
fi

if [ -z "$ZQK_VET_BIN" ]; then
	echo "❌ [ZQK PRE-COMMIT] Failed to find or compile zqk-vet verification engine! Aborting commit." >&2
	echo "   Ensure 'go' is installed and available in PATH to compile bin/zqk-vet." >&2
	exit 1
fi

echo "🛡️  [ZQK PRE-COMMIT] Running zqk-vet hygiene and tree police checks..."
VET_SUITES="${ZQK_VET_SUITE:-hygiene,tree_police}"
if ! "$ZQK_VET_BIN" --suite "$VET_SUITES"; then
	echo "❌ [ZQK PRE-COMMIT] zqk-vet verification failed! Please resolve findings before committing." >&2
	exit 1
fi
`

const defaultPrePushHookScript = `#!/bin/sh
#
# Pre-push hook for ZQK
# Fail-closed enforcement for Git branch protection & PR workflow (POL-WORKFLOW-002)
#
# Rules:
# 1. Pushing directly to 'main' or 'master' is strictly prohibited.
# 2. All changes must be pushed to topic/integration/feature branches and merged via Pull Request.
# 3. Can be bypassed ONLY in exceptional manual emergencies via ZQK_ALLOW_MAIN_PUSH=1.

set -e

remote="$1"
url="$2"

zero="0000000000000000000000000000000000000000"

while read -r local_ref local_oid remote_ref remote_oid; do
	# Check if deleting remote ref
	if [ "$local_oid" = "$zero" ]; then
		# Remote branch deletion
		if [ "$remote_ref" = "refs/heads/main" ] || [ "$remote_ref" = "refs/heads/master" ]; then
			echo "❌ [ZQK PRE-PUSH] Prohibited: Deleting protected branch '$remote_ref' is forbidden." >&2
			exit 1
		fi
		continue
	fi

	# Target branch is main or master
	if [ "$remote_ref" = "refs/heads/main" ] || [ "$remote_ref" = "refs/heads/master" ]; then
		if [ "${ZQK_ALLOW_MAIN_PUSH}" = "1" ]; then
			echo "⚠️  [ZQK PRE-PUSH] Warning: Direct push to protected branch '$remote_ref' permitted via ZQK_ALLOW_MAIN_PUSH=1." >&2
			continue
		fi

		echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
		echo "❌ [ZQK PRE-PUSH] FAIL-CLOSED BRANCH PROTECTION VIOLATION" >&2
		echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
		echo "Direct push to protected branch '$remote_ref' is strictly forbidden." >&2
		echo "" >&2
		echo "All code changes MUST be submitted via topic/feature/integration branches" >&2
		echo "and merged through a validated Pull Request (POL-WORKFLOW-002)." >&2
		echo "" >&2
		echo "Remediation:" >&2
		echo "  1. Create or switch to an integration/feature branch:" >&2
		echo "     git checkout -b <branch-name>" >&2
		echo "  2. Push your topic branch:" >&2
		echo "     git push -u origin <branch-name>" >&2
		echo "  3. Open a Pull Request on GitHub:" >&2
		echo "     gh pr create --fill" >&2
		echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
		exit 1
	fi
done

# Augment PATH so git GUI clients and subshells find go and tools
export PATH="/usr/local/go/bin:/opt/homebrew/bin:$HOME/go/bin:$GOPATH/bin:$PATH"

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"

# Codebase Verification Gate (zqk-vet)
# Secondary defense: catches any bypass of pre-commit (e.g. git commit --no-verify)
if [ -d "$REPO_ROOT/.zqk" ]; then
	ZQK_VET_BIN=""
	NEED_VET_BUILD=0
	if [ ! -x "$REPO_ROOT/bin/zqk-vet" ]; then
		NEED_VET_BUILD=1
	elif [ -n "$(find "$REPO_ROOT/cmd/zqk-vet" "$REPO_ROOT/pkg/vet" "$REPO_ROOT/config/gates.yaml" -newer "$REPO_ROOT/bin/zqk-vet" 2>/dev/null)" ]; then
		NEED_VET_BUILD=1
	fi

	if [ "$NEED_VET_BUILD" -eq 1 ]; then
		if command -v go >/dev/null 2>&1 && [ -f "$REPO_ROOT/cmd/zqk-vet/main.go" ]; then
			echo "⚙️  [ZQK PRE-PUSH] Compiling bin/zqk-vet..."
			mkdir -p "$REPO_ROOT/bin"
			go build -trimpath -o "$REPO_ROOT/bin/zqk-vet" "$REPO_ROOT/cmd/zqk-vet" 2>/dev/null || true
		fi
	fi

	if [ -x "$REPO_ROOT/bin/zqk-vet" ]; then
		ZQK_VET_BIN="$REPO_ROOT/bin/zqk-vet"
	elif command -v zqk-vet >/dev/null 2>&1; then
		ZQK_VET_BIN="$(command -v zqk-vet)"
	fi

	if [ -n "$ZQK_VET_BIN" ]; then
		echo "🛡️  [ZQK PRE-PUSH] Running zqk-vet verification before push..."
		VET_SUITES="${ZQK_VET_SUITE:-hygiene,tree_police}"
		if ! "$ZQK_VET_BIN" --suite "$VET_SUITES"; then
			echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
			echo "❌ [ZQK PRE-PUSH] CODEBASE VERIFICATION GATE FAILED" >&2
			echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
			echo "Push aborted because zqk-vet detected hygiene or tree police violations." >&2
			echo "Resolve all findings reported above before pushing to remote." >&2
			echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" >&2
			exit 1
		fi
	fi
fi

exit 0
`

// EnsureGitHooks checks if a git repository is present and installs/updates
// the ZQK pre-commit and pre-push hooks into .git/hooks and tools/git-hooks.
func EnsureGitHooks(projectRoot string, logger logging.Logger) error {
	gitDir := filepath.Join(projectRoot, ".git")
	if _, err := fileutil.Stat(gitDir); err != nil {
		return nil //nolint:nilerr // not a git repo, skip cleanly
	}

	toolsDir := filepath.Join(projectRoot, "tools", "git-hooks")
	if err := fileutil.MkdirAll(toolsDir, paths.DirPerm755); err != nil {
		return fmt.Errorf("failed to create tools/git-hooks: %w", err)
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	if err := fileutil.MkdirAll(hooksDir, paths.DirPerm755); err != nil {
		return fmt.Errorf("failed to create .git/hooks: %w", err)
	}

	standardHooks := []struct {
		name     string
		template string
	}{
		{name: "pre-commit", template: defaultPreCommitHookScript},
		{name: "pre-push", template: defaultPrePushHookScript},
	}

	for _, h := range standardHooks {
		templatePath := filepath.Join(toolsDir, h.name)
		if _, err := fileutil.Stat(templatePath); fileutil.IsNotExist(err) {
			if err := fileutil.WriteFile(templatePath, []byte(h.template), paths.DirPerm755); err != nil {
				return fmt.Errorf("failed to write hook template %s: %w", templatePath, err)
			}
		}

		installedPath := filepath.Join(hooksDir, h.name)
		if _, err := fileutil.Stat(installedPath + disabledHookSuffix); err == nil {
			if logger != nil {
				logging.Fluent(logger).Info(fmt.Sprintf("%s hook is manually disabled (%s.disabled); skipping", h.name, installedPath)).Log()
			}
			continue
		}

		// Install or update if missing or different
		needsInstall := false
		if _, err := fileutil.Stat(installedPath); fileutil.IsNotExist(err) {
			needsInstall = true
		} else {
			eq, _ := compareFiles(templatePath, installedPath)
			if !eq {
				needsInstall = true
			}
		}

		if needsInstall {
			if err := installHook(templatePath, installedPath); err != nil {
				return fmt.Errorf("failed to install %s hook: %w", h.name, err)
			}
			if logger != nil {
				logging.Fluent(logger).Info(fmt.Sprintf("Installed %s hook into .git/hooks/%s", h.name, h.name)).Log()
			}
		}
	}

	return nil
}

// TestDashboardWarmer is a function that warms the test dashboard lite projection from storage.
type TestDashboardWarmer func(ctx context.Context, projectRoot string, sp storage.ObjectStorageProvider) error

var (
	testDashboardWarmerMu     sync.RWMutex
	globalTestDashboardWarmer TestDashboardWarmer
)

// RegisterTestDashboardWarmer registers a handler to warm the test dashboard lite file.
func RegisterTestDashboardWarmer(warmer TestDashboardWarmer) {
	testDashboardWarmerMu.Lock()
	defer testDashboardWarmerMu.Unlock()
	globalTestDashboardWarmer = warmer
}

// WarmTestDashboard calls the registered test dashboard warmer if available.
func WarmTestDashboard(ctx context.Context, projectRoot string, sp storage.ObjectStorageProvider) error {
	testDashboardWarmerMu.RLock()
	warmer := globalTestDashboardWarmer
	testDashboardWarmerMu.RUnlock()
	if warmer != nil {
		return warmer(ctx, projectRoot, sp)
	}
	return nil
}
