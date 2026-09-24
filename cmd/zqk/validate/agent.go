package validate

import (
	"encoding/csv"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agent"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/crypto"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/quality"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewValidateCmd creates the agent validate command
func NewValidateAgentCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentValidateCommandBuilder()
	cmd.Use = "agent"
	cmd.RunE = cli.WithProcessor(RunValidateAgent)
	return cmd
}

// RunValidateAgent is the shared processor for `zqk agent validate` and `zqk validate agent`.
func RunValidateAgent(cmd *cobra.Command, args []string, proc *cli.Processor) error {
	projectRoot := proc.ProjectRoot()
	if projectRoot == "" {
		return errfmt.Errorf("project root is required")
	}

	var flags clipkg.FlagBag
	stamp := flags.String(cmd, "stamp")
	localFallback := flags.Bool(cmd, "local-fallback")
	if err := flags.Err(); err != nil {
		return err
	}
	if stamp != "" {
		var claims map[string]interface{}
		if localFallback {
			pubKey, err := agent.GetAuthorizedPublicKey()
			if err != nil {
				return errfmt.Errorf("failed to get authorized public key: %w", err)
			}
			claims, err = agent.VerifyStamp(stamp, pubKey)
			if err != nil {
				return errfmt.Errorf("cryptographic stamp verification failed: %w", err)
			}
		} else {
			pubKeyEnv := zqkenv.AgentPubKey().Get()
			if pubKeyEnv == "" {
				return errfmt.Errorf("%s must be set to verify cryptographic stamp", zqkenv.AgentPubKey())
			}
			claimsStruct, err := crypto.VerifyStamp(stamp, pubKeyEnv)
			if err != nil {
				return errfmt.Errorf("cryptographic stamp verification failed: %w", err)
			}
			claims = map[string]interface{}{
				"AgentID": claimsStruct.AgentID,
				"Commit":  claimsStruct.Commit,
			}
		}
		agentID, _ := claims["AgentID"].(string)
		commit, _ := claims["Commit"].(string)
		if agentID == "" && claims["agent_id"] != nil {
			agentID = claims["agent_id"].(string)
		}
		if commit == "" && claims["commit"] != nil {
			commit = claims["commit"].(string)
		}
		cmd.Printf("✅ Verified cryptographic stamp for agent %s (commit: %s)\n", agentID, commit)
	}

	cmd.Println("🔍 Scanning for modified Go files...")
	changedFiles, err := findChangedGoFiles(projectRoot)
	if err != nil {
		return errfmt.Newf("failed to scan changed files").Wrap(err)
	}

	if len(changedFiles) == 0 {
		cmd.Println("❌ No Go files changed. If this task modifies kernel objects, you must use a different verification_strategy or explicitly verify the objects.")
		return errfmt.Errorf("validation failed: no code changes detected")
	}

	// Enforce target artifact/package changes if ZQK_TASK_ARTIFACTS is set
	if targetArtsEnv := zqkenv.TaskArtifacts().Get(); targetArtsEnv != "" {
		targetArts := strings.Split(targetArtsEnv, ",")
		hasTargetChange := false
		for _, changedFile := range changedFiles {
			// Get path relative to projectRoot
			relFile, err := filepath.Rel(projectRoot, changedFile)
			if err != nil {
				continue
			}
			for _, art := range targetArts {
				art = strings.TrimSpace(art)
				if art == "" {
					continue
				}
				// If the artifact is a directory, check if the changed file is inside it
				// If it's a file, check if it matches exactly
				if strings.HasPrefix(relFile, art) || strings.Contains(filepath.ToSlash(relFile), filepath.ToSlash(art)) {
					hasTargetChange = true
					break
				}
			}
			if hasTargetChange {
				break
			}
		}
		if !hasTargetChange {
			cmd.Printf("❌ Validation Rejected: No changes detected in required target artifacts/packages: %s\n", targetArtsEnv)
			return errfmt.Errorf("validation failed: no changes detected in required target artifacts: %s. You must write/modify the required code files to complete the task.", targetArtsEnv)
		}
		cmd.Printf("✅ Verified: Changes detected in target artifacts: %s\n", targetArtsEnv)
	}

	cmd.Println("🔍 Ingesting and checking AST violations...")
	for _, file := range changedFiles {
		if err := checkASTViolations(file); err != nil {
			return errfmt.Newf("compliance violation").Wrap(err)
		}
	}

	cmd.Println("🔍 Verifying matrix compliance (POL-AGENT-003)...")
	if err := verifyVettingMatrix(projectRoot, changedFiles); err != nil {
		return err
	}

	pkgs := getImpactedPackages(projectRoot, changedFiles)
	cmd.Printf("📦 Impacted packages:\n")
	for _, p := range pkgs {
		cmd.Printf("  - %s\n", p)
	}

	// Compilation and unbounded foreground go test are not the validation path.
	// Kernel test_case objects are the process: discover, then run.
	cmd.Println("✅ Validation analysis complete.")
	cmd.Println("⏳ Queueing zqk test discover for kernel test_case refresh...")
	exe, err := fileutil.Executable()
	if err == nil {
		disc := execwrap.Command(exe, "test", "discover")
		disc.Dir = projectRoot
		if err := disc.Start(); err == nil {
			goroutinelabels.NewGoroutine("test_discover_wait", "waiting for background test discover").
				StartSimple(func() {
					_ = disc.Wait()
				})
			cmd.Println("✅ Background test discover queued. Run zqk test run for execution.")
		} else {
			cmd.Printf("⚠️ Background test discover failed: %v\n", err)
		}
	}

	return nil
}

func findChangedGoFiles(projectRoot string) ([]string, error) {
	cmd := execwrap.Command("git", "diff", "--name-only", "HEAD")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")

	cmd2 := execwrap.Command("git", "ls-files", "--others", "--exclude-standard")
	cmd2.Dir = projectRoot
	out2, err := cmd2.Output()
	if err == nil {
		lines = append(lines, strings.Split(string(out2), "\n")...)
	}

	var changed []string
	seen := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		if strings.HasSuffix(line, ".go") {
			seen[line] = true
			changed = append(changed, filepath.Join(projectRoot, line))
		}
	}

	// Try diffing against HEAD~1 if working tree is clean
	if len(changed) == 0 {
		cmd3 := execwrap.Command("git", "diff", "--name-only", "HEAD~1", "HEAD")
		cmd3.Dir = projectRoot
		out3, err := cmd3.Output()
		if err == nil {
			lines3 := strings.Split(string(out3), "\n")
			for _, line := range lines3 {
				line = strings.TrimSpace(line)
				if line == "" || seen[line] {
					continue
				}
				if strings.HasSuffix(line, ".go") {
					seen[line] = true
					changed = append(changed, filepath.Join(projectRoot, line))
				}
			}
		}
	}

	return changed, nil
}

func checkASTViolations(filePath string) error {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		// If it's a deleted file or doesn't exist, ignore parse errors
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := checkCommandCtorPattern(filePath, node); err != nil {
		return err
	}

	isAllowedCobraExpr := strings.Contains(filePath, "pkg/cli") || strings.Contains(filePath, "cmd/")

	var walkErr error
	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		// 1. Check for raw Cobra Command structs outside allowed packages
		if !isAllowedCobraExpr {
			if compositeLit, ok := n.(*ast.CompositeLit); ok {
				if selExpr, ok := compositeLit.Type.(*ast.SelectorExpr); ok {
					if ident, ok := selExpr.X.(*ast.Ident); ok && ident.Name == "cobra" && selExpr.Sel.Name == "Command" {
						walkErr = fmt.Errorf("found raw &cobra.Command instantiation outside of pkg/cli in %s. Commands must use the Spec-Driven Builder Pattern", filePath)
						return false
					}
				}
			}
		}
		switch

		// 2. Check for raw string literals assigned to objects.FieldKeyStatus or Status fields
		v := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range v.Lhs {
				if isFieldKeyStatus(lhs) {
					if i < len(v.Rhs) {
						if _, ok := v.Rhs[i].(*ast.BasicLit); ok {
							walkErr = fmt.Errorf("found raw string literal assignment to objects.FieldKeyStatus in %s. Use objects.ObjectStatus* constants instead", filePath)
							return false
						}
					}
				}
			}
		case *ast.KeyValueExpr:
			if isStatusKey(v.Key) {
				if _, ok := v.Value.(*ast.BasicLit); ok {
					walkErr = fmt.Errorf("found raw string literal assigned to Status field in %s. Use objects.ObjectStatus* constants instead", filePath)
					return false
				}
			}
		}

		return true
	},
	)

	return walkErr
}

func isFieldKeyStatus(expr ast.Expr) bool {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if ident, ok := sel.X.(*ast.Ident); ok {
			return ident.Name == "objects" && sel.Sel.Name == "FieldKeyStatus"
		}
	}
	return false
}

func isStatusKey(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name == "Status" || ident.Name == "status"
	}
	if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		val := strings.Trim(lit.Value, "\"`")
		return val == "status"
	}
	return false
}

func getImpactedPackages(projectRoot string, files []string) []string {
	pkgMap := make(map[string]bool)
	for _, file := range files {
		rel, err := filepath.Rel(projectRoot, file)
		if err != nil {
			continue
		}
		dir := filepath.Dir(rel)
		pkgPath := "./" + dir
		pkgMap[pkgPath] = true
	}

	var pkgs []string
	for p := range pkgMap {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	return pkgs
}

func gitDiffContains(projectRoot, pattern string) (bool, error) {
	cmd := execwrap.Command("git", "diff", "--name-only", "HEAD")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(string(out), pattern), nil
}

func verifyVettingMatrix(projectRoot string, changedFiles []string) error {
	// Dynamically resolve codebase_vetting matrix path from the registry
	reg, err := quality.LoadMatrixRegistry(projectRoot, "")
	if err != nil {
		return errfmt.Errorf("failed to load matrix registry: %w", err)
	}
	_, csvPath, _, err := reg.Resolve(projectRoot, "codebase_vetting")
	if err != nil {
		return errfmt.Errorf("failed to resolve codebase_vetting matrix: %w", err)
	}

	f, err := fileutil.Open(csvPath)
	if err != nil {
		// If the matrix file doesn't exist, we skip validation
		if fileutil.IsNotExist(err) {
			return nil
		}
		return errfmt.Errorf("failed to open codebase vetting matrix CSV: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return errfmt.Errorf("failed to read matrix CSV header: %w", err)
	}

	colIdx := make(map[string]int)
	for i, h := range header {
		colIdx[strings.TrimSpace(h)] = i
	}

	pathIdx, ok1 := colIdx[objects.FieldKeyFilePath]
	vettedIdx, ok2 := colIdx["fully_vetted"]
	refactoredIdx, ok3 := colIdx["fully_refactored_dry"]
	if !ok1 || !ok2 || !ok3 {
		return errfmt.Errorf("codebase vetting matrix CSV is missing required columns (file_path, fully_vetted, fully_refactored_dry)")
	}

	// Build map of changed files (repository relative paths)
	changedMap := make(map[string]bool)
	for _, cf := range changedFiles {
		rel, err := filepath.Rel(projectRoot, cf)
		if err == nil {
			changedMap[filepath.ToSlash(rel)] = true
		}
	}

	// Read all rows and check
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errfmt.Errorf("failed to read CSV row: %w", err)
		}

		if pathIdx >= len(rec) || vettedIdx >= len(rec) || refactoredIdx >= len(rec) {
			continue
		}

		filePath := filepath.ToSlash(strings.TrimSpace(rec[pathIdx]))
		if changedMap[filePath] {
			vettedVal := strings.TrimSpace(rec[vettedIdx])
			refactoredVal := strings.TrimSpace(rec[refactoredIdx])
			if vettedVal != "yes" || refactoredVal != "yes" {
				return errfmt.Errorf("%s", paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("file %s is modified but its row in %s is not marked as fully_vetted=yes and fully_refactored_dry=yes (got fully_vetted=%s, fully_refactored_dry=%s). You must run 'zqk matrix update --file-path %s --set fully_vetted=yes --set fully_refactored_dry=yes' first.", filePath, filepath.Base(csvPath), vettedVal, refactoredVal, filePath)))
			}
		}
	}

	return nil
}
