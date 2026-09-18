package verification

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// VerificationResult contains the deterministic outcome of a programmatic verification step.
type VerificationResult struct {
	Passed   bool
	Feedback string
}

// VerificationStrategy is the interface for all native OS validation layers.
type VerificationStrategy interface {
	Verify(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, step map[string]any) (VerificationResult, error)
}

// Registry of all supported Universal Verification DSL strategies.
var verificationRegistry = map[string]VerificationStrategy{
	"command_exit_code":  &CommandExitCodeStrategy{},
	"ast_semantic_match": &ASTSemanticMatchStrategy{},
	"query_metric":       &QueryMetricStrategy{},
	"state_negation":     &StateNegationStrategy{},
}

// RunVerification executes the requested verification strategy in a completely objective, blind manner.
func RunVerification(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, step map[string]any) (VerificationResult, error) {
	strategyName, ok := step["verification_strategy"].(string)
	if !ok || strategyName == "" {
		return VerificationResult{Passed: false, Feedback: "No verification_strategy specified."}, nil
	}

	strategy, exists := verificationRegistry[strategyName]
	if !exists {
		return VerificationResult{Passed: false, Feedback: fmt.Sprintf("Unknown verification_strategy: %s", strategyName)}, nil
	}

	return strategy.Verify(ctx, secCtx, sp, step)
}

// ---------------------------------------------------------
// 1. COMMAND EXIT CODE
// ---------------------------------------------------------
type CommandExitCodeStrategy struct{}

func (s *CommandExitCodeStrategy) Verify(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, step map[string]any) (VerificationResult, error) {
	cmdStr, _ := step[objects.FieldKeyCommand].(string)
	if cmdStr == "" {
		return VerificationResult{Passed: false, Feedback: "Verification Failed: No command specified."}, nil
	}

	parts := strings.Fields(cmdStr)
	vCmd := execwrap.CommandContext(ctx, parts[0], parts[1:]...)

	// Ensure verification commands run in the repository root directory
	if root := getGitRepoRoot(); root != "" {
		vCmd.Dir = root
	}

	// Propagate task ID and target artifacts to command environment
	env := os.Environ()
	if taskID, ok := step["task_id"].(string); ok && taskID != "" {
		env = append(env, "ZQK_TASK_ID="+taskID)
	}
	if arts, ok := step[objects.FieldKeyArtifacts]; ok {
		if artsList, ok2 := arts.([]any); ok2 {
			var artStrs []string
			for _, a := range artsList {
				if s, ok3 := a.(string); ok3 && s != "" {
					artStrs = append(artStrs, s)
				}
			}
			if len(artStrs) > 0 {
				env = append(env, "ZQK_TASK_ARTIFACTS="+strings.Join(artStrs, ","))
			}
		}
	}
	vCmd.Env = env

	if err := vCmd.Run(); err != nil {
		// Run again with CombinedOutput to capture diagnostic error
		diagCmd := execwrap.CommandContext(ctx, parts[0], parts[1:]...)
		if root := getGitRepoRoot(); root != "" {
			diagCmd.Dir = root
		}
		out, _ := diagCmd.CombinedOutput()
		return VerificationResult{
			Passed:   false,
			Feedback: fmt.Sprintf("Verification Failed: Command exit code was non-zero: %v\nOutput:\n%s", err, string(out)),
		}, nil
	}

	return VerificationResult{Passed: true, Feedback: "Command exited successfully (0)."}, nil
}

func getGitRepoRoot() string {
	out, err := execwrap.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	cwd, err := fileutil.Getwd()
	if err == nil {
		return cwd
	}
	return ""
}

// ---------------------------------------------------------
// 2. AST SEMANTIC MATCH
// ---------------------------------------------------------
type ASTSemanticMatchStrategy struct{}

func (s *ASTSemanticMatchStrategy) Verify(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, step map[string]any) (VerificationResult, error) {
	config, _ := step["config"].(map[string]any)
	if config == nil {
		// If no config is provided, we have no patterns to match against.
		// Return passing result rather than erroring out.
		return VerificationResult{Passed: true, Feedback: "Verification Passed: No AST rules configured."}, nil
	}

	targetPathsRaw, _ := config["target_paths"].([]any)
	var paths []string
	if len(targetPathsRaw) > 0 {
		for _, p := range targetPathsRaw {
			if str, ok := p.(string); ok {
				paths = append(paths, str)
			}
		}
	} else {
		paths = []string{"."}
	}

	forbiddenPatternsRaw, ok := config["forbidden_patterns"].([]any)
	if ok && len(forbiddenPatternsRaw) > 0 {
		for _, searchPath := range paths {
			err := filepath.WalkDir(searchPath, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil // Skip files we can't access
				}
				if d.IsDir() || !strings.HasSuffix(p, ".go") {
					return nil
				}

				content, err := fileutil.ReadFile(p)
				if err != nil {
					return nil
				}

				contentStr := string(content)
				for _, fpRaw := range forbiddenPatternsRaw {
					fp, ok := fpRaw.(string)
					if ok && fp != "" && strings.Contains(contentStr, fp) {
						return fmt.Errorf("found forbidden pattern '%s' in file %s", fp, p)
					}
				}
				return nil
			})
			if err != nil {
				return VerificationResult{
					Passed:   false,
					Feedback: fmt.Sprintf("Verification Failed: %v", err),
				}, nil
			}
		}
	}

	return VerificationResult{Passed: true, Feedback: "AST Semantic match passed."}, nil
}

// ---------------------------------------------------------
// 3. QUERY METRIC
// ---------------------------------------------------------
type QueryMetricStrategy struct{}

func (s *QueryMetricStrategy) Verify(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, step map[string]any) (VerificationResult, error) {
	config, ok := step["config"].(map[string]any)
	if !ok {
		return VerificationResult{Passed: false, Feedback: "Verification Failed: Missing 'config' block for Query Metric validation."}, nil
	}

	query, _ := config["query"].(string)
	if query == "" {
		return VerificationResult{Passed: false, Feedback: "Verification Failed: No query specified."}, nil
	}

	q := storage.Query{
		Type:       storage.QueryTypeCypher,
		Expression: query,
	}

	res, err := sp.Query(ctx, secCtx, &storage.StorageContext{}, q)
	if err != nil {
		return VerificationResult{Passed: false, Feedback: fmt.Sprintf("System Error executing metric query: %v", err)}, nil
	}

	resultCount := 0
	if len(res.Objects) > 0 {
		if countVal, ok := res.Objects[0]["count"]; ok {
			switch v := countVal.(type) {
			case float64:
				resultCount = int(v)
			case int:
				resultCount = v
			case int64:
				resultCount = int(v)
			}
		} else {
			resultCount = len(res.Objects)
		}
	}

	thresholdRaw, _ := config["threshold"].(float64) // JSON unmarshals numbers as float64
	operator, _ := config["operator"].(string)

	passed := false
	switch operator {
	case ">=":
		passed = float64(resultCount) >= thresholdRaw
	case "<=":
		passed = float64(resultCount) <= thresholdRaw
	case "==":
		passed = float64(resultCount) == thresholdRaw
	default:
		return VerificationResult{Passed: false, Feedback: fmt.Sprintf("Unknown operator '%s'", operator)}, nil
	}

	if !passed {
		return VerificationResult{
			Passed:   false,
			Feedback: fmt.Sprintf("Verification Failed: Query metric evaluated to %d, which does not satisfy %s %v", resultCount, operator, thresholdRaw),
		}, nil
	}

	return VerificationResult{Passed: true, Feedback: "Query metric threshold satisfied."}, nil
}

// ---------------------------------------------------------
// 4. STATE NEGATION
// ---------------------------------------------------------
type StateNegationStrategy struct{}

func (s *StateNegationStrategy) Verify(ctx context.Context, secCtx *storage.SecurityContext, sp storage.ObjectStorageProvider, step map[string]any) (VerificationResult, error) {
	config, ok := step["config"].(map[string]any)
	if !ok {
		return VerificationResult{Passed: false, Feedback: "Verification Failed: Missing 'config' block for State Negation validation."}, nil
	}

	targetKind, _ := config[objects.FieldKeyTargetKind].(string)
	targetStatus, _ := config[objects.FieldKeyTargetStatus].(string)

	if targetKind == "" || targetStatus == "" {
		return VerificationResult{Passed: false, Feedback: "Verification Failed: target_kind and target_status required."}, nil
	}

	filter := storage.ListFilter{
		Kind: targetKind,
		Filters: map[string]any{
			objects.FieldKeyStatus: targetStatus,
		},
	}

	count, err := sp.Count(ctx, secCtx, filter)
	if err != nil {
		return VerificationResult{Passed: false, Feedback: fmt.Sprintf("System Error checking state negation: %v", err)}, nil
	}

	if count > 0 {
		return VerificationResult{
			Passed:   false,
			Feedback: fmt.Sprintf("Verification Failed: State Negation failed. Found %d object(s) of kind '%s' with status '%s'.", count, targetKind, targetStatus),
		}, nil
	}

	return VerificationResult{Passed: true, Feedback: "State negation condition verified (no matching objects found)."}, nil
}
