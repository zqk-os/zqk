package vet

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CheckCLIBuilders verifies:
// 1. Every declarative command spec in .zqk/cli/specs/ has a generated builder in pkg/cli/bldr_cli_cmd_v1/.
// 2. Command constructors in cmd/zqk/ follow the established builder pattern (using bldr_cli_cmd_v1 or clipkg.ApplyBuilder)
//    rather than constructing ad-hoc &cobra.Command structs directly for specced or un-baselined commands.
func CheckCLIBuilders(projectRoot string, cfg *GatesConfig, rootCmd any) ([]Finding, error) {
	var findings []Finding

	specsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CLISpecsDir)
	buildersDir := filepath.Join(projectRoot, "pkg", "cli", "bldr_cli_cmd_v1")
	baselinePath := filepath.Join(projectRoot, paths.ProjectDataDir, "cli", "command_spec_coverage_baseline.json")

	// Index existing builders in pkg/cli/bldr_cli_cmd_v1
	knownBuilders := make(map[string]bool)
	fset := token.NewFileSet()
	if fi, err := fileutil.Stat(buildersDir); err == nil && fi.IsDir() {
		_ = filepath.WalkDir(buildersDir, func(path string, d fileutil.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			src, err := fileutil.ReadFile(path)
			if err != nil {
				return nil
			}
			fullNode, err := parser.ParseFile(fset, path, src, 0)
			if err != nil {
				return nil
			}
			for _, decl := range fullNode.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name != nil {
					knownBuilders[strings.ToLower(fn.Name.Name)] = true
				}
			}
			return nil
		})
	}

	// 1. Check spec -> builder existence
	speccedCommands := make(map[string]bool)
	if fi, err := fileutil.Stat(specsDir); err == nil && fi.IsDir() {
		_ = filepath.WalkDir(specsDir, func(path string, d fileutil.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				if d != nil && d.IsDir() {
					name := d.Name()
					if name == "_schemas" || name == "schemas" || strings.HasPrefix(name, ".") {
						return filepath.SkipDir
					}
				}
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".yaml" && ext != ".yml" {
				return nil
			}

			// scheduler/convergence/*.yaml are sub-spec fragments for convergence_command.yaml
			if strings.Contains(path, "/scheduler/convergence/") || strings.Contains(path, "\\scheduler\\convergence\\") {
				return nil
			}

			relToSpecs, _ := filepath.Rel(specsDir, path)
			baseName := strings.TrimSuffix(relToSpecs, ext)
			baseName = strings.TrimSuffix(baseName, "_command")
			baseName = strings.ReplaceAll(baseName, string(filepath.Separator), "_")
			baseName = strings.ReplaceAll(baseName, "/", "_")
			baseName = strings.ReplaceAll(baseName, "-", "_")

			canonSpec := canonicalCommandIDString(baseName)
			speccedCommands[canonSpec] = true

			expectedBuilderName := strings.ToLower(fmt.Sprintf("New%sCommandBuilder", toCamelCase(baseName)))
			if !knownBuilders[expectedBuilderName] {
				relSpec, _ := filepath.Rel(projectRoot, path)
				findings = append(findings, Finding{
					CheckID:  "cli/missing_builder",
					Suite:    "hygiene",
					Severity: SeverityError,
					File:     relSpec,
					Message:  fmt.Sprintf("command spec %q is missing builder func %s in pkg/cli/bldr_cli_cmd_v1/", baseName, fmt.Sprintf("New%sCommandBuilder()", toCamelCase(baseName))),
				})
			}
			return nil
		})
	}

	// 2. Identify new un-baselined commands via live Cobra root if available
	newCommandsMap := make(map[string]bool)
	if cmd, ok := rootCmd.(*cobra.Command); ok && cmd != nil {
		coverage, err := clipkg.AnalyzeCommandSpecCoverage(cmd, specsDir)
		if err == nil {
			if baseline, err := clipkg.LoadCommandSpecCoverageBaseline(baselinePath); err == nil {
				coverage = clipkg.ApplyCommandSpecCoverageBaseline(coverage, baseline, baselinePath)
			}
			for _, missingCmd := range coverage.NewCommandsWithoutSpecs {
				newCommandsMap[missingCmd] = true
				newCommandsMap[canonicalCommandIDString(missingCmd)] = true
			}
		}
	}

	// 3. Check cmd/zqk/ command constructors for specced or un-baselined commands
	cmdDir := filepath.Join(projectRoot, "cmd", "zqk")
	if fi, err := fileutil.Stat(cmdDir); err != nil || !fi.IsDir() {
		return findings, nil
	}

	err := filepath.WalkDir(cmdDir, func(path string, d fileutil.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		src, err := fileutil.ReadFile(path)
		if err != nil {
			return nil
		}
		if isGeneratedCode(src) {
			return nil
		}

		fileNode, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return nil
		}

		relFile, _ := filepath.Rel(projectRoot, path)
		subpkg := extractSubpackage(cmdDir, path)

		for _, decl := range fileNode.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil || fn.Body == nil {
				continue
			}

			if !isCommandConstructorName(fn.Name.Name) {
				continue
			}

			cmdName := extractCommandNameFromFunc(fn)
			if cmdName == "" {
				continue
			}
			primaryCmd := strings.Fields(cmdName)[0]
			fullCmdPath := resolveFullCommandPath(subpkg, primaryCmd)

			fnStem := strings.TrimPrefix(fn.Name.Name, "New")
			fnStem = strings.TrimSuffix(fnStem, "Cmd")
			fnStem = strings.TrimSuffix(fnStem, "Command")
			fnCanon := canonicalCommandIDString(fnStem)
			subpkgCanon := canonicalCommandIDString(subpkg)

			var canonID string
			if strings.HasPrefix(fnCanon, subpkgCanon) {
				canonID = fnCanon
			} else {
				canonID = subpkgCanon + fnCanon
			}
			shortID := canonicalCommandIDString(primaryCmd)

			// Only enforce builder pattern on specced commands or new un-baselined commands
			isSpecced := speccedCommands[canonID] || (subpkg == "" && speccedCommands[shortID])
			isNewDrift := newCommandsMap[fullCmdPath] || newCommandsMap[primaryCmd] || newCommandsMap[canonID] || newCommandsMap[shortID]

			if !isSpecced && !isNewDrift {
				continue
			}

			violates, line := checkDirectCobraInstantiation(fn, fset)
			if violates {
				findings = append(findings, Finding{
					CheckID:  "cli/builder_pattern_violation",
					Suite:    "hygiene",
					Severity: SeverityError,
					File:     relFile,
					Line:     line,
					Message: fmt.Sprintf("command constructor %s directly instantiates &cobra.Command for %q (%s); must adopt bldr_cli_cmd_v1 builder pattern or clipkg.ApplyBuilder",
						fn.Name.Name, primaryCmd, fullCmdPath),
				})
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return findings, nil
}

// toCamelCase converts snake_case or kebab-case to CamelCase.
func toCamelCase(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[0:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// checkDirectCobraInstantiation returns true if the function directly constructs &cobra.Command
// without using bldr_cli_cmd_v1 or clipkg builder wrappers.
func checkDirectCobraInstantiation(fn *ast.FuncDecl, fset *token.FileSet) (bool, int) {
	usesBuilder := false
	hasDirectCobraLit := false
	directCobraLine := 0

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			callName := callFunName(node)
			if strings.Contains(callName, "CommandBuilder") ||
				strings.HasPrefix(callName, "ApplyBuilder") ||
				strings.HasSuffix(callName, "ApplyBuilder") {
				usesBuilder = true
			}
		case *ast.CompositeLit:
			if sel, ok := node.Type.(*ast.SelectorExpr); ok {
				if sel.Sel != nil && sel.Sel.Name == "Command" {
					if xIdent, ok := sel.X.(*ast.Ident); ok && xIdent.Name == "cobra" {
						hasDirectCobraLit = true
						directCobraLine = fset.Position(node.Pos()).Line
					}
				}
			}
		}
		return true
	})

	if hasDirectCobraLit && !usesBuilder {
		return true, directCobraLine
	}
	return false, 0
}
