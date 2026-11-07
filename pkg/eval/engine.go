package eval

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	maxDiamondScore = 5.0
	minDiamondScore = 1.0
	penaltyPerBlock = 0.5
	penaltyPerWarn  = 0.2
)

// Engine orchestrates scenario evaluations against declarative eval templates.
type Engine struct {
	projectRoot string
}

// NewEngine creates a new evaluation engine bound to a project root.
func NewEngine(projectRoot string) *Engine {
	return &Engine{projectRoot: projectRoot}
}

// Execute runs the evaluation pipeline according to the provided template and target override.
func (e *Engine) Execute(ctx context.Context, tmpl *EvalTemplate, targetOverride string) (*EvalResult, error) {
	if tmpl == nil {
		return nil, errfmt.Errorf("template cannot be nil")
	}

	result := &EvalResult{
		TemplateID:      tmpl.TemplateID,
		Target:          targetOverride,
		AssessedAt:      time.Now().UTC(),
		DimensionScores: make(map[string]float64),
		Violations:      make([]Violation, 0),
		Remediations:    make([]string, 0),
	}

	// 1. Resolve target files
	filesToScan, err := e.resolveTargetFiles(tmpl, targetOverride)
	if err != nil {
		return nil, errfmt.Errorf("failed to resolve target files: %w", err)
	}

	if len(filesToScan) == 0 {
		return nil, errfmt.Errorf("no target files found matching patterns")
	}

	// 2. Execute fast-path AST / structural inspection
	for _, file := range filesToScan {
		violations := e.inspectFile(file, tmpl)
		result.Violations = append(result.Violations, violations...)
	}

	// 3. Compute dimension scores
	var totalWeight float64
	var weightedSum float64

	for dimName, dimSpec := range tmpl.Dimensions {
		score := maxDiamondScore
		for _, v := range result.Violations {
			if strings.EqualFold(v.Rule, dimName) || dimName == "HCODE" {
				if v.Severity == "blocker" {
					score -= penaltyPerBlock
				} else {
					score -= penaltyPerWarn
				}
			}
		}
		if score < minDiamondScore {
			score = minDiamondScore
		}
		result.DimensionScores[dimName] = score
		weightedSum += score * dimSpec.Weight
		totalWeight += dimSpec.Weight
	}

	if totalWeight > 0 {
		result.CompositeScore = weightedSum / totalWeight
	} else {
		result.CompositeScore = maxDiamondScore
	}

	// 4. Determine pass/fail thresholds
	result.Passed = true
	for _, v := range result.Violations {
		if v.Severity == "blocker" {
			result.Passed = false
			break
		}
	}

	for dimName, dimSpec := range tmpl.Dimensions {
		if result.DimensionScores[dimName] < dimSpec.Threshold {
			result.Passed = false
			result.Violations = append(result.Violations, Violation{
				Rule:      dimName,
				Severity:  "blocker",
				Message:   fmt.Sprintf("Dimension %s score %0.2f fell below required threshold %0.2f", dimName, result.DimensionScores[dimName], dimSpec.Threshold),
				Actual:    result.DimensionScores[dimName],
				Threshold: dimSpec.Threshold,
			})
		}
	}

	// 5. Automated remediation minting
	if !result.Passed && tmpl.Remediation.AutoMint {
		remediationID, err := e.mintRemediation(ctx, tmpl, result)
		if err == nil && remediationID != "" {
			result.Remediations = append(result.Remediations, remediationID)
		}
	}

	return result, nil
}

func (e *Engine) resolveTargetFiles(tmpl *EvalTemplate, targetOverride string) ([]string, error) {
	var files []string
	searchTarget := strings.TrimSpace(targetOverride)

	if searchTarget != "" {
		fullPath := filepath.Join(e.projectRoot, searchTarget)
		info, err := fileutil.Stat(fullPath)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return []string{searchTarget}, nil
		}
		// Walk target directory
		_ = filepath.WalkDir(fullPath, func(path string, d fileutil.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(e.projectRoot, path)
			if relErr == nil && strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") {
				files = append(files, rel)
			}
			return nil
		})
		return files, nil
	}

	// Fallback to template target patterns
	for _, pattern := range tmpl.Target.Patterns {
		matches, err := filepath.Glob(filepath.Join(e.projectRoot, pattern))
		if err == nil {
			for _, m := range matches {
				rel, relErr := filepath.Rel(e.projectRoot, m)
				if relErr == nil {
					files = append(files, rel)
				}
			}
		}
	}

	return files, nil
}

func (e *Engine) inspectFile(relPath string, tmpl *EvalTemplate) []Violation {
	var violations []Violation
	if !strings.HasSuffix(relPath, ".go") || strings.HasSuffix(relPath, "_test.go") {
		return violations
	}

	fullPath := filepath.Join(e.projectRoot, relPath)
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, fullPath, nil, parser.ParseComments)
	if err != nil {
		violations = append(violations, Violation{
			File:     relPath,
			Rule:     "SYNTAX",
			Severity: "blocker",
			Message:  fmt.Sprintf("Failed to parse Go file: %v", err),
		})
		return violations
	}

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		// Fast-path rule: Check direct 'os.*' calls
		if pkgIdent.Name == "os" {
			pos := fset.Position(call.Pos())
			violations = append(violations, Violation{
				File:     relPath,
				Rule:     "POL-CODE-006",
				Severity: "blocker",
				Message:  fmt.Sprintf("Line %d: Direct standard library 'os.%s' call violates filesystem abstraction (use pkg/utils/fileutil)", pos.Line, sel.Sel.Name),
			})
		}

		return true
	})

	return violations
}

func (e *Engine) mintRemediation(ctx context.Context, tmpl *EvalTemplate, result *EvalResult) (string, error) {
	// Construct remediation technical_debt identifier and title
	persona := tmpl.Remediation.Persona
	if persona == "" {
		persona = "PER-COMMUNITY-SOFTWARE-ENGINEER"
	}

	desc := fmt.Sprintf("Evaluation failed for template %s on target %s with composite score %0.2f. Violations:\n",
		tmpl.TemplateID, result.Target, result.CompositeScore)
	for _, v := range result.Violations {
		desc += fmt.Sprintf("- [%s] %s (%s)\n", v.Severity, v.Message, v.Rule)
	}

	tdeID := fmt.Sprintf("TD-EVAL-%d", time.Now().UnixNano()%1000000)
	// Return the receipt identifier
	return tdeID, nil
}
