package vet

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CheckCommandSpecs validates:
// 1. Declarative command specs in .zqk/cli/specs/ parse as valid YAML and contain required schema fields (name, short or description).
// 2. All CLI commands defined in cmd/zqk/ are either backed by a spec in .zqk/cli/specs/ or baselined in .zqk/cli/command_spec_coverage_baseline.json.
// Any new, un-specced, and un-baselined command generates a SeverityError finding.
func CheckCommandSpecs(projectRoot string, cfg *GatesConfig, rootCmd any) ([]Finding, error) {
	var findings []Finding

	specsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CLISpecsDir)
	if fi, err := fileutil.Stat(specsDir); err != nil || !fi.IsDir() {
		return findings, nil
	}

	knownSpecs := make(map[string]string)       // canonical ID -> relative path
	knownSpecsByName := make(map[string]string) // spec "name" -> relative path

	// 1. Walk and validate specs in .zqk/cli/specs
	err := filepath.WalkDir(specsDir, func(path string, d fileutil.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == "_schemas" || name == "schemas" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
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

		rel, _ := filepath.Rel(projectRoot, path)
		data, err := fileutil.ReadFile(path)
		if err != nil {
			findings = append(findings, Finding{
				CheckID:  "command_spec/unreadable",
				Suite:    "hygiene",
				Severity: SeverityError,
				File:     rel,
				Message:  fmt.Sprintf("failed to read command spec: %v", err),
			})
			return nil
		}

		var specMap map[string]any
		if err := yaml.Unmarshal(data, &specMap); err != nil {
			findings = append(findings, Finding{
				CheckID:  "command_spec/syntax",
				Suite:    "hygiene",
				Severity: SeverityError,
				File:     rel,
				Message:  fmt.Sprintf("invalid YAML syntax in command spec: %v", err),
			})
			return nil
		}

		name, _ := specMap["name"].(string)
		short, _ := specMap["short"].(string)
		desc, _ := specMap["description"].(string)

		if strings.TrimSpace(name) == "" {
			findings = append(findings, Finding{
				CheckID:  "command_spec/schema",
				Suite:    "hygiene",
				Severity: SeverityError,
				File:     rel,
				Message:  "command spec missing required 'name' field",
			})
		}
		if strings.TrimSpace(short) == "" && strings.TrimSpace(desc) == "" {
			findings = append(findings, Finding{
				CheckID:  "command_spec/schema",
				Suite:    "hygiene",
				Severity: SeverityError,
				File:     rel,
				Message:  fmt.Sprintf("command spec %q missing required 'short' or 'description' field", name),
			})
		}

		relToSpecs, _ := filepath.Rel(specsDir, path)
		canonicalID := canonicalCommandSpecID(relToSpecs)
		knownSpecs[canonicalID] = rel
		if name != "" {
			firstWord := strings.Fields(name)[0]
			knownSpecsByName[firstWord] = rel
			knownSpecsByName[canonicalCommandIDString(name)] = rel
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	baselinePath := filepath.Join(projectRoot, paths.ProjectDataDir, "cli", "command_spec_coverage_baseline.json")

	seenMissingCmds := make(map[string]bool)

	// 2. If dynamic RootCommand is available, use canonical AnalyzeCommandSpecCoverage
	if cmd, ok := rootCmd.(*cobra.Command); ok && cmd != nil {
		coverage, err := clipkg.AnalyzeCommandSpecCoverage(cmd, specsDir)
		if err == nil {
			if baseline, err := clipkg.LoadCommandSpecCoverageBaseline(baselinePath); err == nil {
				coverage = clipkg.ApplyCommandSpecCoverageBaseline(coverage, baseline, baselinePath)
			}
			for _, missingCmd := range coverage.NewCommandsWithoutSpecs {
				seenMissingCmds[missingCmd] = true
				seenMissingCmds[canonicalCommandIDString(missingCmd)] = true
				findings = append(findings, Finding{
					CheckID:  "command_spec/missing",
					Suite:    "hygiene",
					Severity: SeverityError,
					File:     filepath.Join(paths.ProjectDataDir, paths.CLISpecsDir),
					Message:  fmt.Sprintf("command %q lacks a declarative specification in .zqk/cli/specs/ (un-baselined command drift)", missingCmd),
				})
			}
			for _, missingSpec := range coverage.NewSpecsWithoutCommands {
				findings = append(findings, Finding{
					CheckID:  "command_spec/orphaned",
					Suite:    "hygiene",
					Severity: SeverityError,
					File:     missingSpec,
					Message:  fmt.Sprintf("command spec %q has no matching active CLI command (un-baselined spec drift)", missingSpec),
				})
			}
			return findings, nil
		}
	}

	// 3. Fallback: Load historical baseline
	baselineMap := make(map[string]bool)
	if baselineData, err := fileutil.ReadFile(baselinePath); err == nil {
		var baseline struct {
			CommandsWithoutSpecs []string `json:"commands_without_specs"`
			SpecsWithoutCommands []string `json:"specs_without_commands"`
		}
		if err := json.Unmarshal(baselineData, &baseline); err == nil {
			for _, cmd := range baseline.CommandsWithoutSpecs {
				baselineMap[cmd] = true
				baselineMap[canonicalCommandIDString(cmd)] = true
			}
		}
	}

	// Scan cmd/zqk/app/root_commands.go for root CLI commands
	rootCmdsFile := filepath.Join(projectRoot, "cmd", "zqk", "app", "root_commands.go")
	if fileutil.Exists(rootCmdsFile) {
		fset := token.NewFileSet()
		src, err := fileutil.ReadFile(rootCmdsFile)
		if err == nil {
			node, err := parser.ParseFile(fset, rootCmdsFile, src, parser.ParseComments)
			if err == nil {
				varToCmd := make(map[string]string)
				varToPos := make(map[string]token.Position)

				ast.Inspect(node, func(n ast.Node) bool {
					assign, ok := n.(*ast.AssignStmt)
					if !ok {
						return true
					}
					for i, lhs := range assign.Lhs {
						ident, ok := lhs.(*ast.Ident)
						if !ok || i >= len(assign.Rhs) {
							continue
						}
						rhs := assign.Rhs[i]
						if call, ok := rhs.(*ast.CallExpr); ok {
							fnName := callFunName(call)
							cmdName := inferCommandNameFromConstructor(fnName)
							if cmdName == "" {
								if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
									if xIdent, ok := sel.X.(*ast.Ident); ok {
										cmdName = xIdent.Name
									}
								}
							}
							if cmdName != "" {
								varToCmd[ident.Name] = cmdName
								varToPos[ident.Name] = fset.Position(assign.Pos())
							}
						}
					}
					return true
				})

				ast.Inspect(node, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || sel.Sel == nil || sel.Sel.Name != "AddCommand" {
						return true
					}
					xIdent, ok := sel.X.(*ast.Ident)
					if !ok || xIdent.Name != "rootCmd" {
						return true
					}

					for _, arg := range call.Args {
						var cmdName string
						var pos token.Position
						if ident, ok := arg.(*ast.Ident); ok {
							cmdName = varToCmd[ident.Name]
							pos = varToPos[ident.Name]
						} else if callArg, ok := arg.(*ast.CallExpr); ok {
							cmdName = inferCommandNameFromConstructor(callFunName(callArg))
							if cmdName == "" {
								if sel, ok := callArg.Fun.(*ast.SelectorExpr); ok {
									if xIdent, ok := sel.X.(*ast.Ident); ok {
										cmdName = xIdent.Name
									}
								}
							}
							pos = fset.Position(callArg.Pos())
						}

						if cmdName == "" || cmdName == "help" || cmdName == "completion" || cmdName == "internal" {
							continue
						}

						canonID := canonicalCommandIDString(cmdName)
						hasSpec := knownSpecs[canonID] != "" || knownSpecsByName[cmdName] != "" || knownSpecsByName[canonID] != ""
						if !hasSpec {
							isBaselined := baselineMap[cmdName] || baselineMap[canonID]
							if !isBaselined && !seenMissingCmds[cmdName] && !seenMissingCmds[canonID] {
								seenMissingCmds[cmdName] = true
								seenMissingCmds[canonID] = true
								relFile, _ := filepath.Rel(projectRoot, rootCmdsFile)
								findings = append(findings, Finding{
									CheckID:  "command_spec/missing",
									Suite:    "hygiene",
									Severity: SeverityError,
									File:     relFile,
									Line:     pos.Line,
									Message:  fmt.Sprintf("root command %q lacks a declarative specification in .zqk/cli/specs/ and is not in baseline", cmdName),
								})
							}
						}
					}
					return true
				})
			}
		}
	}

	return findings, nil
}

func inferCommandNameFromConstructor(callName string) string {
	name := callName
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	if strings.HasPrefix(name, "New") && (strings.HasSuffix(name, "Cmd") || strings.HasSuffix(name, "Command")) {
		raw := strings.TrimPrefix(name, "New")
		raw = strings.TrimSuffix(raw, "Cmd")
		raw = strings.TrimSuffix(raw, "Command")
		return strings.ToLower(raw)
	}
	return ""
}

func isCommandConstructorName(name string) bool {
	if strings.HasPrefix(name, "New") && (strings.HasSuffix(name, "Cmd") || strings.HasSuffix(name, "Command")) {
		return true
	}
	return false
}

func extractSubpackage(cmdDir, path string) string {
	rel, err := filepath.Rel(cmdDir, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) > 1 {
		return parts[0]
	}
	return ""
}

func resolveFullCommandPath(subpkg, primaryCmd string) string {
	if subpkg == "" || subpkg == "app" || subpkg == primaryCmd {
		return primaryCmd
	}
	return subpkg + " " + primaryCmd
}

func extractCommandNameFromFunc(fn *ast.FuncDecl) string {
	var cmdName string

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if cmdName != "" {
			return false
		}
		switch node := n.(type) {
		case *ast.CallExpr:
			callName := callFunName(node)
			if strings.HasSuffix(callName, "NewCommandBuilder") || strings.HasSuffix(callName, "NewCRUDCommandBuilder") {
				if len(node.Args) > 0 {
					if lit, ok := node.Args[len(node.Args)-1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						cmdName = strings.Trim(lit.Value, `"`)
						return false
					}
				}
			}
			if strings.HasPrefix(callName, "New") && strings.HasSuffix(callName, "CommandBuilder") {
				raw := strings.TrimPrefix(callName, "New")
				raw = strings.TrimSuffix(raw, "CommandBuilder")
				cmdName = strings.ToLower(raw)
				return false
			}
		case *ast.KeyValueExpr:
			if key, ok := node.Key.(*ast.Ident); ok && key.Name == "Use" {
				if val, ok := node.Value.(*ast.BasicLit); ok && val.Kind == token.STRING {
					cmdName = strings.Trim(val.Value, `"`)
					return false
				}
			}
		}
		return true
	})

	return cmdName
}

func canonicalCommandSpecID(relPath string) string {
	ext := filepath.Ext(relPath)
	raw := strings.TrimSuffix(relPath, ext)
	raw = strings.TrimSuffix(raw, "_command")
	raw = strings.TrimPrefix(raw, "CSPEC-")
	raw = strings.TrimSuffix(raw, "-command")
	return canonicalCommandIDString(raw)
}

func canonicalCommandIDString(s string) string {
	r := strings.NewReplacer(" ", "_", "-", "_", "/", "_", "\\", "_")
	parts := strings.Split(r.Replace(s), "_")
	var dedup []string
	for _, p := range parts {
		if p != "" && (len(dedup) == 0 || dedup[len(dedup)-1] != p) {
			dedup = append(dedup, p)
		}
	}
	return strings.Join(dedup, "_")
}
