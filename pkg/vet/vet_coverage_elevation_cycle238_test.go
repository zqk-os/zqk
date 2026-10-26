// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package vet

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCommon_CollectGitFilesFallback_Cycle238(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, fileutil.MkdirAll(filepath.Join(tempDir, ".git"), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, ".git", "config"), []byte("git config"), paths.FilePerm644))

	require.NoError(t, fileutil.MkdirAll(filepath.Join(tempDir, ".zqk"), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, ".zqk", "state.txt"), []byte("state"), paths.FilePerm644))

	require.NoError(t, fileutil.MkdirAll(filepath.Join(tempDir, "vendor"), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "vendor", "lib.go"), []byte("package vendor"), paths.FilePerm644))

	require.NoError(t, fileutil.MkdirAll(filepath.Join(tempDir, "node_modules"), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "node_modules", "package.json"), []byte("{}"), paths.FilePerm644))

	require.NoError(t, fileutil.MkdirAll(filepath.Join(tempDir, "bin"), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "bin", "zqk"), []byte("binary"), paths.FilePerm755))

	require.NoError(t, fileutil.MkdirAll(filepath.Join(tempDir, "src", "sub"), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "src", "sub", "main.go"), []byte("package main"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "src", "sub", "doc.txt"), []byte("documentation"), paths.FilePerm644))

	files, err := collectGitFiles(tempDir)
	require.NoError(t, err)
	assert.Contains(t, files, filepath.Join("src", "sub", "main.go"))
	assert.Contains(t, files, filepath.Join("src", "sub", "doc.txt"))
	for _, f := range files {
		assert.NotContains(t, f, ".git")
		assert.NotContains(t, f, ".zqk")
		assert.NotContains(t, f, "vendor")
		assert.NotContains(t, f, "node_modules")
		assert.NotContains(t, f, "bin")
	}

	goFiles, err := collectGoFiles(tempDir, nil)
	require.NoError(t, err)
	assert.Contains(t, goFiles, filepath.ToSlash(filepath.Join("src", "sub", "main.go")))
	assert.NotContains(t, goFiles, filepath.ToSlash(filepath.Join("src", "sub", "doc.txt")))

	filteredGoFiles, err := collectGoFiles(tempDir, []string{"src/sub"})
	require.NoError(t, err)
	assert.Equal(t, 1, len(filteredGoFiles))
}

func TestCommon_IsPathExempt_Branches_Cycle238(t *testing.T) {
	exemptions := []string{
		"pkg/*/*.go",
		"*.tmp",
		"special/*",
		"exact/file.go",
	}

	assert.True(t, isPathExempt("pkg/vet/runner.go", exemptions))
	assert.True(t, isPathExempt("deep/nested/item.tmp", exemptions))
	assert.True(t, isPathExempt("special/sub/tool.go", exemptions))
	assert.True(t, isPathExempt("special", exemptions))
	assert.True(t, isPathExempt("exact/file.go", exemptions))
	assert.False(t, isPathExempt("other/pkg/test.go", exemptions))
}

func TestRunner_SuitesAndFailures_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	cfg := DefaultConfig()

	r := NewRunner(tempDir, cfg)
	require.NotNil(t, r)

	norm := r.normalizeSuites(nil)
	assert.Equal(t, []string{"hygiene", "tree_police", "payload"}, norm)

	normAll := r.normalizeSuites([]string{"all"})
	assert.Equal(t, []string{"hygiene", "tree_police", "payload"}, normAll)

	normCustom := r.normalizeSuites([]string{" hygiene ", "tree"})
	assert.Equal(t, []string{"hygiene", "tree"}, normCustom)

	// Test FailFast with error findings
	r.Config.Hygiene.CheckPaths = true
	r.Config.Hygiene.ForbiddenPathLiterals = []string{".zqk"}
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tempDir, "pkg"), paths.DirPerm755))
	badSrc := `package pkg
func Bad() {
	_ = ".zqk/data"
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "pkg", "bad.go"), []byte(badSrc), paths.FilePerm644))

	repFailFast, err := r.Run(RunOptions{
		Suites:   []string{"hygiene", "tree"},
		FailFast: true,
	})
	require.NoError(t, err)
	assert.False(t, repFailFast.Passed)
	assert.Equal(t, 1, len(repFailFast.SuiteResults))

	// Test PrintReport with File only and neither File nor Line
	var buf bytes.Buffer
	rep := &Report{
		Passed:      false,
		TotalErrors: 1,
		TotalWarns:  1,
		SuiteResults: map[string]SuiteResult{
			"hygiene": {
				Suite:  "hygiene",
				Passed: false,
				Findings: []Finding{
					{
						CheckID:  "test/no_line",
						Suite:    "hygiene",
						File:     "file.go",
						Message:  "file finding without line",
						Severity: SeverityError,
					},
					{
						CheckID:  "test/no_file",
						Suite:    "hygiene",
						Message:  "finding without file or line",
						Severity: SeverityWarn,
					},
				},
			},
		},
	}
	r.PrintReport(&buf, rep)
	out := buf.String()
	assert.Contains(t, out, "[ERROR] file.go: file finding without line")
	assert.Contains(t, out, "[WARN] finding without file or line")
	assert.Contains(t, out, "RESULT: FAIL (1 errors, 1 warnings")
}

func TestCheckPathsAndPerms_DetailedBranches_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "sample")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	goCode := "package sample\n" +
		"import (\n" +
		"\t\"os\"\n" +
		"\tfileutil \"github.com/zqk-os/zqk/pkg/utils/fileutil\"\n" +
		")\n" +
		"func TestPerms() {\n" +
		"\tos.OpenFile(\"a.txt\", os.O_RDWR, 0644)\n" +
		"\tfileutil.OpenFile(\"b.txt\", 0, 0600)\n" +
		"\tOpenFile(\"c.txt\", 0, 0755)\n" +
		"\t" + "os." + "Mkdir(\"dir1\", 0755)\n" +
		"\tfileutil.MkdirAll(\"dir2\", 0750)\n" +
		"\tMkdirAll(\"dir3\", 0700)\n" +
		"\t" + "os." + "WriteFile(\"d.txt\", nil, 0644)\n" +
		"\tfileutil.WriteFile(\"e.txt\", nil, 0600)\n" +
		"\tWriteFile(\"f.txt\", nil, 0755)\n\n" +
		"\tp1 := \".zqk/forbidden\"\n" +
		"\tp2 := \"/Users/developer/secret\"\n" +
		"\tp3 := \"/home/runner/secret\"\n" +
		"\tp4 := \"/var/folders/aa/bb\"\n" +
		"\tp5 := \"/private/var/test\"\n" +
		"\t_ = p1\n" +
		"\t_ = p2\n" +
		"\t_ = p3\n" +
		"\t_ = p4\n" +
		"\t_ = p5\n" +
		"}\n" +
		"func OpenFile(name string, flag int, perm int) {}\n" +
		"func MkdirAll(path string, perm int) {}\n" +
		"func WriteFile(name string, data []byte, perm int) {}\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "sample.go"), []byte(goCode), paths.FilePerm644))

	cfg := DefaultConfig()
	findings, err := CheckPathsAndPerms(tempDir, cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)

	permCount := 0
	pathCount := 0
	workstationCount := 0
	for _, f := range findings {
		switch f.CheckID {
		case "hygiene/magic-perms":
			permCount++
		case "hygiene/hardcoded-path":
			pathCount++
		case "hygiene/hardcoded-workstation-path":
			workstationCount++
		}
	}
	assert.Equal(t, 9, permCount)
	assert.Equal(t, 1, pathCount)
	assert.Equal(t, 4, workstationCount)
}

func TestCheckPathsAndPerms_HelperUnits_Cycle238(t *testing.T) {
	assert.False(t, isMagicPerm(&ast.Ident{Name: "FilePerm644"}))
	assert.False(t, isMagicPerm(&ast.BasicLit{Kind: token.STRING, Value: `"0644"`}))
	assert.False(t, isMagicPerm(&ast.BasicLit{Kind: token.INT, Value: `invalid_num`}))
	assert.False(t, isMagicPerm(&ast.BasicLit{Kind: token.INT, Value: `0444`}))

	assert.True(t, isMagicPerm(&ast.BasicLit{Kind: token.INT, Value: `0755`}))
	assert.True(t, isMagicPerm(&ast.BasicLit{Kind: token.INT, Value: `0644`}))
	assert.True(t, isMagicPerm(&ast.BasicLit{Kind: token.INT, Value: `0600`}))
	assert.True(t, isMagicPerm(&ast.BasicLit{Kind: token.INT, Value: `0700`}))
	assert.True(t, isMagicPerm(&ast.BasicLit{Kind: token.INT, Value: `0750`}))

	identCall := &ast.CallExpr{Fun: &ast.Ident{Name: "CustomOpen"}}
	assert.Equal(t, "CustomOpen", getCallFunctionName(identCall))

	selCall := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.Ident{Name: "fileutil"},
			Sel: &ast.Ident{Name: "OpenFile"},
		},
	}
	assert.Equal(t, "fileutil.OpenFile", getCallFunctionName(selCall))

	deepSelCall := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X: &ast.SelectorExpr{
				X:   &ast.Ident{Name: "pkg"},
				Sel: &ast.Ident{Name: "sub"},
			},
			Sel: &ast.Ident{Name: "DeepFunc"},
		},
	}
	assert.Equal(t, "DeepFunc", getCallFunctionName(deepSelCall))

	invalidCall := &ast.CallExpr{Fun: &ast.BasicLit{Kind: token.INT, Value: "123"}}
	assert.Equal(t, "", getCallFunctionName(invalidCall))
}

func TestCheckCLINames_HitScenarios_Cycle238(t *testing.T) {
	hit, verb := firstCanonicalCLIHit("echo " + brand.CanonicalExecutableToken + " agent")
	assert.True(t, hit)
	assert.Equal(t, "agent", verb)

	hit, verb = firstCanonicalCLIHit(brand.CanonicalExecutableToken + " workflow;")
	assert.True(t, hit)
	assert.Equal(t, "workflow", verb)

	hit, verb = firstCanonicalCLIHit("my" + brand.CanonicalExecutableToken + " agent")
	assert.False(t, hit)
	assert.Empty(t, verb)

	hit, verb = firstCanonicalCLIHit(brand.CanonicalExecutableToken + " unknownverbxyz")
	assert.False(t, hit)
	assert.Empty(t, verb)

	hit, verb = firstCanonicalCLIHit("completely unrelated string")
	assert.False(t, hit)
	assert.Empty(t, verb)
}

func TestCheckCLINames_ExecutionScenarios_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "sample")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	src := `package sample
import paths "github.com/zqk-os/zqk/pkg/paths"
const CmdDocs = "run zqk agent directly in const"
func Run() {
	cmd := "zqk agent run"
	_ = cmd
	paths.CLIUsage("zqk agent exempt")
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "cli_test_file.go"), []byte(src), paths.FilePerm644))

	syntaxErrSrc := `package sample broken syntax`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "broken.go"), []byte(syntaxErrSrc), paths.FilePerm644))

	cfg := DefaultConfig()
	cfg.Hygiene.CLIExemptFunctions = []string{"CLIUsage"}
	findings, err := CheckCLINames(tempDir, nil, cfg)
	require.NoError(t, err)
	assert.Equal(t, 1, len(findings))
	assert.Equal(t, "cli_name_literal", findings[0].CheckID)
}

func TestCheckCLIBuilders_DeepCoverage_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	require.NoError(t, fileutil.MkdirAll(filepath.Join(specsDir, "_schemas"), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "_schemas", "schema.yaml"), []byte("schema"), paths.FilePerm644))

	require.NoError(t, fileutil.MkdirAll(filepath.Join(specsDir, "scheduler", "convergence"), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "scheduler", "convergence", "sub.yaml"), []byte("sub"), paths.FilePerm644))

	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "sample_command.yml"), []byte("name: sample\nshort: Sample\n"), paths.FilePerm644))

	cmdDir := filepath.Join(tempDir, "cmd", "zqk", "sample")
	require.NoError(t, fileutil.MkdirAll(cmdDir, paths.DirPerm755))

	require.NoError(t, fileutil.MkdirAll(filepath.Join(cmdDir, "testdata"), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(cmdDir, "testdata", "mock.go"), []byte("package testdata"), paths.FilePerm644))

	genSrc := `// Code generated by tool. DO NOT EDIT.
package sample
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(cmdDir, "gen.go"), []byte(genSrc), paths.FilePerm644))

	toolSrc := `package sample
import "github.com/spf13/cobra"
func NewsampleCmd() *cobra.Command {
	return &cobra.Command{
		Use: "sample",
	}
}
func HelperFunc() {}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(cmdDir, "tool.go"), []byte(toolSrc), paths.FilePerm644))

	findings, err := CheckCLIBuilders(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)

	hasMissingBuilder := false
	hasViolation := false
	for _, f := range findings {
		if f.CheckID == "cli/missing_builder" {
			hasMissingBuilder = true
		}
		if f.CheckID == "cli/builder_pattern_violation" {
			hasViolation = true
		}
	}
	assert.True(t, hasMissingBuilder)
	assert.True(t, hasViolation)
}

func TestCheckCLIBuilders_DirectCobraCheck_Cycle238(t *testing.T) {
	src := `package test
import "github.com/spf13/cobra"
func ConstructA() *cobra.Command {
	return &cobra.Command{Use: "a"}
}
func ConstructB() *cobra.Command {
	bldr := NewMyBuilder()
	return bldr.Build()
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", src, 0)
	require.NoError(t, err)

	fnA := f.Decls[1].(*ast.FuncDecl)
	violatesA, lineA := checkDirectCobraInstantiation(fnA, fset)
	assert.True(t, violatesA)
	assert.Greater(t, lineA, 0)

	fnB := f.Decls[2].(*ast.FuncDecl)
	violatesB, _ := checkDirectCobraInstantiation(fnB, fset)
	assert.False(t, violatesB)
}

func TestCheckCommandSpecs_ExtractHelpers_Cycle238(t *testing.T) {
	assert.Equal(t, "agent", extractSubpackage("/app/cmd/zqk", "/app/cmd/zqk/agent/run.go"))
	assert.Equal(t, "", extractSubpackage("/app/cmd/zqk", "/app/cmd/zqk/main.go"))
	assert.Equal(t, "", extractSubpackage("/app/cmd/zqk", "relative/path/not/rel"))

	fset := token.NewFileSet()
	fnSrc := `package test
func NewAlphaCmd() {
	bldr := NewAlphaCommandBuilder("alpha")
}
func NewBetaCmd() {
	bldr := NewBetaCommandBuilder()
}
func NewGammaCmd() {
	_ = &cobra.Command{Use: "gamma"}
}
func NewEmptyCmd() {}
`
	f, err := parser.ParseFile(fset, "test.go", fnSrc, 0)
	require.NoError(t, err)

	fnAlpha := f.Decls[0].(*ast.FuncDecl)
	assert.Equal(t, "alpha", extractCommandNameFromFunc(fnAlpha))

	fnBeta := f.Decls[1].(*ast.FuncDecl)
	assert.Equal(t, "beta", extractCommandNameFromFunc(fnBeta))

	fnGamma := f.Decls[2].(*ast.FuncDecl)
	assert.Equal(t, "gamma", extractCommandNameFromFunc(fnGamma))

	fnEmpty := f.Decls[3].(*ast.FuncDecl)
	assert.Equal(t, "", extractCommandNameFromFunc(fnEmpty))
}

func TestCheckCommandSpecs_ParityDeep_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	specContent := `name: sample
short: Sample command short
flags:
  - name: flag-a
    description: Flag A description
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "sample_command.yaml"), []byte(specContent), paths.FilePerm644))

	root := &cobra.Command{Use: "zqk"}
	subCmd := &cobra.Command{
		Use:   "sample",
		Short: "Different short description",
	}
	subCmd.Flags().String("flag-b", "", "Flag B description")
	root.AddCommand(subCmd)

	findings, err := CheckCommandSpecs(tempDir, DefaultConfig(), root)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)
}

func TestCheckDuplication_DriftDeep_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "lookup")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	driftSrc := `package lookup
const (
	ActionStart = "start"
	ActionStop  = "stop"
	ActionPause = "pause"
)
var ActionLookup = map[string]int{
	ActionStart: 1,
	ActionStop:  2,
	ActionPause: 3,
}
func HandleAction(a string) bool {
	if a == ActionStart || a == ActionStop {
		return true
	}
	return false
}
func ReturnAction(code int) string {
	if code == 1 {
		return ActionStart
	}
	if code == 2 {
		return ActionStop
	}
	return ActionPause
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "lookup.go"), []byte(driftSrc), paths.FilePerm644))

	findings, err := CheckDuplication(tempDir, nil, DefaultConfig())
	require.NoError(t, err)
	assert.NotEmpty(t, findings)

	hasDrift := false
	for _, f := range findings {
		if f.CheckID == "hygiene/duplication" && f.Severity == SeverityWarn {
			hasDrift = true
			break
		}
	}
	assert.True(t, hasDrift)
}

func TestCheckGoroutines_DeepScenarios_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "async")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	rawGoSrc := "package async\n" +
		"func RunAsync() {\n" +
		"\t" + "go" + " func() {}()\n" +
		"}\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "raw.go"), []byte(rawGoSrc), paths.FilePerm644))

	syntaxErrSrc := `package async broken(`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "broken.go"), []byte(syntaxErrSrc), paths.FilePerm644))

	testdataDir := filepath.Join(pkgDir, "testdata")
	require.NoError(t, fileutil.MkdirAll(testdataDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(testdataDir, "ignore.go"), []byte(rawGoSrc), paths.FilePerm644))

	findings, err := CheckRawGoroutines(tempDir, nil, DefaultConfig())
	require.NoError(t, err)
	assert.Equal(t, 1, len(findings))
	assert.Equal(t, "hygiene/raw_goroutine", findings[0].CheckID)
}

func TestCheckPayload_DeepScenarios_Cycle238(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &GatesConfig{
		Payload: PayloadConfig{
			RequiredModulePath: "github.com/custom/module",
			RequiredArtifacts:  []string{"README.md"},
			ForbiddenArtifacts: []string{"SECRET.key"},
			ProhibitedPatterns: []TokenRule{
				{
					ID:      "leak",
					Pattern: "LEAKED_SECRET",
					Message: "token leak",
					Scope:   []string{"*"},
				},
			},
		},
	}

	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "SECRET.key"), []byte("secret"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module github.com/wrong/mod\n"), paths.FilePerm644))

	findings, err := CheckPayload(tempDir, cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)

	hasReqMissing := false
	hasForbPresent := false
	hasInvalidMod := false
	for _, f := range findings {
		switch f.CheckID {
		case "payload/required-artifact-missing":
			hasReqMissing = true
		case "payload/forbidden-artifact-present":
			hasForbPresent = true
		case "payload/invalid-module-path":
			hasInvalidMod = true
		}
	}
	assert.True(t, hasReqMissing)
	assert.True(t, hasForbPresent)
	assert.True(t, hasInvalidMod)
}

func TestCheckReferenceIntegrity_DeepScenarios_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	bliDir := filepath.Join(tempDir, ".zqk", "process", "backlog_items")
	require.NoError(t, fileutil.MkdirAll(bliDir, paths.DirPerm755))

	require.NoError(t, fileutil.WriteFile(filepath.Join(bliDir, "not_yaml.txt"), []byte("text"), paths.FilePerm644))

	otherKind := `kind: other_object
id: OTH-001
status: in_progress
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(bliDir, "other.yaml"), []byte(otherKind), paths.FilePerm644))

	draftItem := `kind: backlog_item
id: BLI-DRAFT-001
status: draft
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(bliDir, "draft.yaml"), []byte(draftItem), paths.FilePerm644))

	goalItem := `kind: backlog_item
id: BLI-GOAL-001
status: in_progress
goal_refs:
  - GOAL-001
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(bliDir, "goal.yaml"), []byte(goalItem), paths.FilePerm644))

	unanchoredItem := `kind: backlog_item
id: BLI-UNANCHORED-001
status: in_progress
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(bliDir, "unanchored.yaml"), []byte(unanchoredItem), paths.FilePerm644))

	findings, err := CheckReferenceIntegrity(tempDir, DefaultConfig())
	require.NoError(t, err)
	assert.Equal(t, 1, len(findings))
	assert.Equal(t, "reference/missing-goal-req-ref", findings[0].CheckID)
	assert.Contains(t, findings[0].Message, "BLI-UNANCHORED-001")
}

func TestCheckTreePolice_DeepScenarios_Cycle238(t *testing.T) {
	tempDir := t.TempDir()

	scriptsDir := filepath.Join(tempDir, "scripts")
	require.NoError(t, fileutil.MkdirAll(scriptsDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(scriptsDir, "unapproved.py"), []byte("print(1)"), paths.FilePerm755))

	pkgDocsDir := filepath.Join(tempDir, "pkg", "mymodule", "docs")
	require.NoError(t, fileutil.MkdirAll(pkgDocsDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDocsDir, "info.md"), []byte("documentation text exceeding fifty bytes requirement for testing"), paths.FilePerm644))

	docsDir := filepath.Join(tempDir, "docs")
	require.NoError(t, fileutil.MkdirAll(docsDir, paths.DirPerm755))

	require.NoError(t, fileutil.WriteFile(filepath.Join(docsDir, "stub.md"), []byte("# Stub"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(docsDir, "rubbish.md"), []byte("This guide includes lorem ipsum placeholder rubbish that must be rejected"), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(docsDir, "unexpanded.md"), []byte("Documentation containing TODO_OVERWRITE marker which should fail gate"), paths.FilePerm644))

	cfg := DefaultConfig()
	findings, err := CheckTreePolice(tempDir, cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)

	hasExtraScript := false
	hasPkgDocs := false
	hasDocStub := false
	hasRubbish := false
	hasUnexpanded := false
	for _, f := range findings {
		switch f.CheckID {
		case "tree/extra-script":
			hasExtraScript = true
		case "tree/forbidden-pkg-docs-dir":
			hasPkgDocs = true
		case "tree/doc-stub":
			hasDocStub = true
		case "tree/doc-placeholder-rubbish":
			hasRubbish = true
		case "tree/doc-unexpanded-template":
			hasUnexpanded = true
		}
	}
	assert.True(t, hasExtraScript)
	assert.True(t, hasPkgDocs)
	assert.True(t, hasDocStub)
	assert.True(t, hasRubbish)
	assert.True(t, hasUnexpanded)
}

func TestCheckTestExec_DeepScenarios_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "sample")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	testSrc := "package sample\n" +
		"import (\n" +
		"\t\"os\"\n" +
		"\t\"os/exec\"\n" +
		"\ttesting \"testing\"\n" +
		")\n" +
		"func TestBadSubprocess(t *testing.T) {\n" +
		"\t_ = exec.Command(\"ls\")\n" +
		"\t_ = exec.CommandContext(nil, \"ls\")\n" +
		"\t_ = os.StartProcess(\"bin\", nil, nil)\n" +
		"}\n" +
		"func TestBadFileutil(t *testing.T) {\n" +
		"\t_, _ = " + "os." + "ReadFile(\"a.txt\")\n" +
		"\t_ = " + "os." + "WriteFile(\"b.txt\", nil, 0644)\n" +
		"\t_, _ = os.Open(\"c.txt\")\n" +
		"\t_, _ = os.OpenFile(\"d.txt\", 0, 0644)\n" +
		"\t_ = " + "os." + "RemoveAll(\"e_dir\")\n" +
		"}\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "unmanaged_cycle238_test.go"), []byte(testSrc), paths.FilePerm644))

	subprocFindings, err := CheckTestSubprocessHygiene(tempDir, DefaultConfig())
	require.NoError(t, err)
	assert.Equal(t, 3, len(subprocFindings))

	fileutilFindings, err := CheckTestFileutilHygiene(tempDir, DefaultConfig())
	require.NoError(t, err)
	assert.Equal(t, 5, len(fileutilFindings))
}

func TestCheckSensitiveTrackedFiles_Repo_Cycle238(t *testing.T) {
	files, err := checkSensitiveTrackedFiles("../..", []string{"*.go"})
	require.NoError(t, err)
	assert.NotEmpty(t, files)

	// Clean/empty check on non-repo temp dir
	tempDir := t.TempDir()
	emptyFiles, err := checkSensitiveTrackedFiles(tempDir, []string{"*.key"})
	require.NoError(t, err)
	assert.Empty(t, emptyFiles)
}

func TestCheckCommandSpecs_OrphanedAndSubdir_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	cliDir := filepath.Join(tempDir, paths.ProjectDataDir, "cli")
	require.NoError(t, fileutil.MkdirAll(cliDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(
		filepath.Join(cliDir, "command_spec_coverage_baseline.json"),
		[]byte(`{"commands_without_specs":[],"specs_without_commands":[]}`),
		paths.FilePerm644,
	))

	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(filepath.Join(specsDir, "agent"), paths.DirPerm755))

	agentSpec := `name: agent status
short: Agent status short
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "agent", "status_command.yaml"), []byte(agentSpec), paths.FilePerm644))

	orphanedSpec := `name: orphaned
short: Orphaned command
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "orphaned_command.yaml"), []byte(orphanedSpec), paths.FilePerm644))

	root := &cobra.Command{Use: "zqk"}
	agentCmd := &cobra.Command{Use: "agent"}
	statusCmd := &cobra.Command{Use: "status"}
	agentCmd.AddCommand(statusCmd)
	root.AddCommand(agentCmd)

	findings, err := CheckCommandSpecs(tempDir, DefaultConfig(), root)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)

	hasOrphaned := false
	for _, f := range findings {
		if f.CheckID == "command_spec/orphaned" {
			hasOrphaned = true
			break
		}
	}
	assert.True(t, hasOrphaned, "expected orphaned command spec finding")
}

func TestCheckCommandSpecs_RootCommandsFallbackInlineCall_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	appDir := filepath.Join(tempDir, "cmd", "zqk", "app")
	require.NoError(t, fileutil.MkdirAll(appDir, paths.DirPerm755))

	rootCmdsSrc := `package app
func setupCommands() {
	rootCmd.AddCommand(NewInlineCmd())
	rootCmd.AddCommand(custompkg.NewCustomCommand())
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(appDir, "root_commands.go"), []byte(rootCmdsSrc), paths.FilePerm644))

	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "valid_command.yaml"), []byte("name: valid\nshort: valid\n"), paths.FilePerm644))

	findings, err := CheckCommandSpecs(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)
}

func TestCheckReferenceIntegrity_AllDirsAndReqRefs_Cycle238(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Singular backlog_item directory
	bliSingularDir := filepath.Join(tempDir, ".zqk", "process", "backlog_item")
	require.NoError(t, fileutil.MkdirAll(bliSingularDir, paths.DirPerm755))

	reqItem := `kind: backlog_item
id: BLI-REQ-001
status: in_progress
requirement_refs:
  - REQ-001
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(bliSingularDir, "req.yaml"), []byte(reqItem), paths.FilePerm644))

	archivedItem := `kind: backlog_item
id: BLI-ARCHIVED-001
status: archived
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(bliSingularDir, "archived.yaml"), []byte(archivedItem), paths.FilePerm644))

	// 2. Datacell directory
	datacellDir := filepath.Join(tempDir, ".zqk", "datacell", "backlog_items")
	require.NoError(t, fileutil.MkdirAll(datacellDir, paths.DirPerm755))

	preliminaryItem := `kind: backlog_item
id: BLI-PRELIM-001
status: preliminary
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(datacellDir, "prelim.yaml"), []byte(preliminaryItem), paths.FilePerm644))

	findings, err := CheckReferenceIntegrity(tempDir, DefaultConfig())
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestCheckTreePolice_SymlinkAndExemptDocs_Cycle238(t *testing.T) {
	tempDir := t.TempDir()

	docsDir := filepath.Join(tempDir, "docs")
	require.NoError(t, fileutil.MkdirAll(docsDir, paths.DirPerm755))

	validContent := "# Comprehensive documentation guide exceeding minimum byte requirements safely"
	targetPath := filepath.Join(docsDir, "target.md")
	require.NoError(t, fileutil.WriteFile(targetPath, []byte(validContent), paths.FilePerm644))

	linkPath := filepath.Join(docsDir, "link.md")
	require.NoError(t, fileutil.Symlink(targetPath, linkPath))

	// CEF evaluation docs are exempt from stub check
	cefDir := filepath.Join(tempDir, "pkg", "quality", "codebase_evaluation")
	require.NoError(t, fileutil.MkdirAll(cefDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(cefDir, "tiny.md"), []byte("# T"), paths.FilePerm644))

	cfg := DefaultConfig()
	cfg.TreePolice.AllowedScripts = nil
	cfg.TreePolice.AllowedScriptPrefixes = nil
	cfg.TreePolice.ForbiddenPaths = nil
	cfg.TreePolice.ForbiddenFiles = nil

	findings, err := CheckTreePolice(tempDir, cfg)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestCheckDuplication_StatementSequenceMatch_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "sequence")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	seqCodeA := `package sequence
func RoutineAlpha() int {
	a := 10
	b := 20
	c := a + b
	d := c * 2
	e := d - 5
	return e
}
`
	seqCodeB := `package sequence
func RoutineBeta() int {
	a := 10
	b := 20
	c := a + b
	d := c * 2
	e := d - 5
	return e
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "a.go"), []byte(seqCodeA), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "b.go"), []byte(seqCodeB), paths.FilePerm644))

	cfg := &GatesConfig{
		Hygiene: HygieneConfig{
			CheckDups:        true,
			DupMinStatements: 3,
			DupMinLines:      3,
			GoScanDirs:       []string{"pkg/"},
		},
	}
	findings, err := CheckDuplication(tempDir, nil, cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)

	hasSeqDup := false
	for _, f := range findings {
		if f.CheckID == "hygiene/duplication" && f.Severity == SeverityWarn {
			hasSeqDup = true
			break
		}
	}
	assert.True(t, hasSeqDup, "expected statement sequence duplication finding")
}

func TestCheckDuplication_DefaultsAndNegativeBounds_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &GatesConfig{
		Hygiene: HygieneConfig{
			CheckDups:        true,
			DupMinStatements: -1,
			DupMinLines:      -1,
			DupExemptions:    nil,
		},
	}
	findings, err := CheckDuplication(tempDir, nil, cfg)
	require.NoError(t, err)
	assert.Empty(t, findings)

	nilCfgFindings, err := CheckDuplication(tempDir, []string{}, nil)
	require.NoError(t, err)
	assert.Empty(t, nilCfgFindings)
}

func TestCheckCLIBuilders_LiveRootDrift_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	cliDir := filepath.Join(tempDir, paths.ProjectDataDir, "cli")
	require.NoError(t, fileutil.MkdirAll(cliDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(
		filepath.Join(cliDir, "command_spec_coverage_baseline.json"),
		[]byte(`{"commands_without_specs":[],"specs_without_commands":[]}`),
		paths.FilePerm644,
	))

	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	cmdDir := filepath.Join(tempDir, "cmd", "zqk", "drift")
	require.NoError(t, fileutil.MkdirAll(cmdDir, paths.DirPerm755))
	driftSrc := `package drift
import "github.com/spf13/cobra"
func NewdriftCmd() *cobra.Command {
	return &cobra.Command{Use: "drift"}
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(cmdDir, "drift.go"), []byte(driftSrc), paths.FilePerm644))

	root := &cobra.Command{Use: "zqk"}
	driftCmd := &cobra.Command{Use: "drift"}
	root.AddCommand(driftCmd)

	findings, err := CheckCLIBuilders(tempDir, DefaultConfig(), root)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)

	hasViolation := false
	for _, f := range findings {
		if f.CheckID == "cli/builder_pattern_violation" {
			hasViolation = true
			break
		}
	}
	assert.True(t, hasViolation, "expected builder pattern violation on drift command")
}

func TestCheckPathsAndPerms_ExemptAndGenerated_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "sample")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	genSrc := "// Code generated by tool. DO NOT EDIT.\n" +
		"package sample\nimport \"os\"\n" +
		"func Gen() { _ = " + "os." + "WriteFile(\"f\", nil, 0644) }\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "gen.go"), []byte(genSrc), paths.FilePerm644))

	exemptSrc := "package sample\nimport \"os\"\n" +
		"func Exempt() { _ = " + "os." + "WriteFile(\"f\", nil, 0644) }\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "exempt.go"), []byte(exemptSrc), paths.FilePerm644))

	brokenSrc := `package sample broken(`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "broken.go"), []byte(brokenSrc), paths.FilePerm644))

	cfg := DefaultConfig()
	cfg.Hygiene.Exemptions = []string{"pkg/sample/exempt.go"}

	findings, err := CheckPathsAndPerms(tempDir, cfg)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestCheckGoroutines_NilConfigAndGenerated_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "async")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	genSrc := "// Code generated by tool. DO NOT EDIT.\n" +
		"package async\n" +
		"func GenAsync() { " + "go" + " func() {}() }\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "gen.go"), []byte(genSrc), paths.FilePerm644))

	brokenSrc := `package async broken(`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "broken.go"), []byte(brokenSrc), paths.FilePerm644))

	findings, err := CheckRawGoroutines(tempDir, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestCheckDuplication_EdgeCasesAndTypedMaps_Cycle238(t *testing.T) {
	nilDrift := auditMapKeyReferenceDrift(token.NewFileSet(), "nil.go", nil)
	assert.Nil(t, nilDrift)

	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "typed")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	typedSrc := `package typed
const (
	_ = iota
	KeyOne = "one"
	KeyTwo = "two"
	KeyThree = "three"
)
var MyTypedMap map[string]int = map[string]int{
	KeyOne: 1,
	KeyTwo: 2,
	KeyThree: 3,
}
func CheckTyped(k string) bool {
	if k == KeyOne || k == KeyTwo {
		return true
	}
	return false
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "typed.go"), []byte(typedSrc), paths.FilePerm644))

	genSrc := `// Code generated by tool. DO NOT EDIT.
package typed
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "gen.go"), []byte(genSrc), paths.FilePerm644))

	brokenSrc := `package typed broken(`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "broken.go"), []byte(brokenSrc), paths.FilePerm644))

	findings, err := CheckDuplication(tempDir, nil, DefaultConfig())
	require.NoError(t, err)
	assert.NotEmpty(t, findings)
}

func TestCheckCLINames_ExemptFilesAndNonExistent_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "sample")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	genSrc := `// Code generated by tool. DO NOT EDIT.
package sample
func Gen() { _ = "zqk agent run" }
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "gen.go"), []byte(genSrc), paths.FilePerm644))

	exemptSrc := `package sample
func Exempt() { _ = "zqk agent run" }
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "exempt.go"), []byte(exemptSrc), paths.FilePerm644))

	cleanStrSrc := "package sample\nfunc Clean() { _ = \"hello world\" }\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "cleanstr.go"), []byte(cleanStrSrc), paths.FilePerm644))

	cfg := DefaultConfig()
	cfg.Hygiene.CLIExemptFiles = []string{"pkg/sample/exempt.go"}

	files := []string{
		"pkg/sample/missing.go",
		"pkg/sample/gen.go",
		"pkg/sample/exempt.go",
		"pkg/sample/cleanstr.go",
	}
	findings, err := CheckCLINames(tempDir, files, cfg)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestCheckCommandSpecs_AnnotationTrueWhenSpecFalse_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	specContent := `name: anncmd
short: Short
requires_storage: false
requires_session: true
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "anncmd_command.yaml"), []byte(specContent), paths.FilePerm644))

	root := &cobra.Command{Use: "zqk"}
	annCmd := &cobra.Command{
		Use:   "anncmd",
		Short: "Short",
		Annotations: map[string]string{
			"zqk.requires_storage": "true",
			"zqk.requires_session": "false",
		},
	}
	root.AddCommand(annCmd)

	findings, err := CheckCommandSpecs(tempDir, DefaultConfig(), root)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)

	driftCount := 0
	for _, f := range findings {
		if f.CheckID == "command_spec/annotation_drift" {
			driftCount++
		}
	}
	assert.Equal(t, 2, driftCount)
}

func TestCheckCommandSpecs_NewCRUDCommandBuilder_Cycle238(t *testing.T) {
	fnSrc := `package test
func NewCrudCmd() {
	bldr := NewCRUDCommandBuilder("crud_target")
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", fnSrc, 0)
	require.NoError(t, err)

	fnCrud := f.Decls[0].(*ast.FuncDecl)
	assert.Equal(t, "crud_target", extractCommandNameFromFunc(fnCrud))
}

func TestCheckCommandSpecs_FallbackBaselinePopulated_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	cliDir := filepath.Join(tempDir, paths.ProjectDataDir, "cli")
	require.NoError(t, fileutil.MkdirAll(cliDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(
		filepath.Join(cliDir, "command_spec_coverage_baseline.json"),
		[]byte(`{"commands_without_specs":["custompkg"],"specs_without_commands":[]}`),
		paths.FilePerm644,
	))

	appDir := filepath.Join(tempDir, "cmd", "zqk", "app")
	require.NoError(t, fileutil.MkdirAll(appDir, paths.DirPerm755))
	rootCmdsSrc := `package app
func registerCommands() {
	cmdA := custompkg.Build()
	rootCmd.AddCommand(cmdA)
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(appDir, "root_commands.go"), []byte(rootCmdsSrc), paths.FilePerm644))

	findings, err := CheckCommandSpecs(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestCheckDuplication_ClosuresAndEmptyBodies_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "closures")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	src := `package closures
func External()

var TopClosure = func() {
	x := 1
	y := 2
	_ = x + y
}

func HostFunction() {
	short := func() { _ = 1 }
	_ = short
	nested := func() {
		a := 10
		b := 20
		c := 30
		d := a + b + c
		_ = d
	}
	nested()
}

func SelfRepeating() {
	p := 1
	q := 2
	r := p + q
	_ = r

	p = 1
	q = 2
	r = p + q
	_ = r
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "closures.go"), []byte(src), paths.FilePerm644))

	cfg := DefaultConfig()
	cfg.Hygiene.DupMinStatements = 3
	cfg.Hygiene.DupMinLines = 3
	findings, err := CheckDuplication(tempDir, nil, cfg)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestConfig_DirectoryPathError_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	cfg, err := LoadConfig(tempDir, tempDir)
	assert.Error(t, err)
	assert.Nil(t, cfg)
}

func TestRunner_ErrorPath_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	r := NewRunner(tempDir, DefaultConfig())
	r.Config.Hygiene.CheckPaths = false
	r.Config.Hygiene.CheckPerms = false
	r.Config.Hygiene.CheckCLINames = true
	r.Config.Hygiene.CLIExemptFiles = nil
	r.Config.Hygiene.Exemptions = nil

	_, err := r.Run(RunOptions{
		Suites: []string{"hygiene"},
		Files:  []string{tempDir},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "suite hygiene failed")
}

func TestCheckGoroutines_FilesBranches_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	emptyExemptCfg := &GatesConfig{}

	// Non-existent file in files slice
	findings, err := CheckRawGoroutines(tempDir, []string{"non_existent.go"}, emptyExemptCfg)
	require.NoError(t, err)
	assert.Empty(t, findings)

	// Directory in files slice causes ReadFile error
	_, err = CheckRawGoroutines(tempDir, []string{tempDir}, emptyExemptCfg)
	assert.Error(t, err)
}

func TestCheckReferenceIntegrity_CorruptYAML_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	bliDir := filepath.Join(tempDir, ".zqk", "process", "backlog_items")
	require.NoError(t, fileutil.MkdirAll(bliDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(bliDir, "bad.yaml"), []byte(":\n  - ["), paths.FilePerm644))

	findings, err := CheckReferenceIntegrity(tempDir, DefaultConfig())
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestCheckTestExec_CallForms_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "sample")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	src := `package sample
import "testing"
func TestVariousCalls(t *testing.T) {
	bareFunction()
	getObj().Method()
}
func bareFunction() {}
type Obj struct{}
func (o Obj) Method() {}
func getObj() Obj { return Obj{} }
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "calls_test.go"), []byte(src), paths.FilePerm644))

	findings, err := CheckTestSubprocessHygiene(tempDir, DefaultConfig())
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestCheckPayload_SensitiveAndPatternLeaks_Cycle238(t *testing.T) {
	cfg := &GatesConfig{
		Payload: PayloadConfig{
			RequiredModulePath:   "github.com/zqk-os/zqk",
			SensitivePatterns:    []string{"*.go"},
			GlobalGrepExemptions: []string{"nonexistent/*"},
			ProhibitedPatterns: []TokenRule{
				{
					ID:      "pkg_main_rule",
					Pattern: "package main",
					Message: "detected package main declaration",
					Scope:   []string{"cmd/zqk"},
				},
			},
		},
	}

	findings, err := CheckPayload("../..", cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)

	hasSensitive := false
	hasProhibited := false
	for _, f := range findings {
		if f.CheckID == "payload/sensitive-file-tracked" {
			hasSensitive = true
		}
		if f.CheckID == "payload/pkg_main_rule" {
			hasProhibited = true
		}
	}
	assert.True(t, hasSensitive, "expected sensitive tracked files finding")
	assert.True(t, hasProhibited, "expected prohibited pattern finding")
}

func TestCheckCLIBuilders_SubpkgPrefixAndEmptyName_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	cmdDir := filepath.Join(tempDir, "cmd", "zqk", "sub")
	require.NoError(t, fileutil.MkdirAll(cmdDir, paths.DirPerm755))

	src := `package sub
import "github.com/spf13/cobra"
func NewNoNameCmd() *cobra.Command {
	return nil
}
func NewDifferentCmd() *cobra.Command {
	return &cobra.Command{Use: "different"}
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(cmdDir, "sub.go"), []byte(src), paths.FilePerm644))

	findings, err := CheckCLIBuilders(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestCheckCommandSpecs_DirectCallAndSelectorInAddCommand_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	appDir := filepath.Join(tempDir, "cmd", "zqk", "app")
	require.NoError(t, fileutil.MkdirAll(appDir, paths.DirPerm755))

	rootCmdsSrc := `package app
func setup() {
	rootCmd.AddCommand(pkgbuilder.Create())
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(appDir, "root_commands.go"), []byte(rootCmdsSrc), paths.FilePerm644))

	findings, err := CheckCommandSpecs(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, findings)
	assert.Equal(t, "command_spec/missing", findings[0].CheckID)
}

func TestRunner_MoreHygieneErrorPaths_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	r := NewRunner(tempDir, DefaultConfig())

	// Test CheckRawGoroutines error propagation in runHygiene
	r.Config.Hygiene.CheckPaths = false
	r.Config.Hygiene.CheckPerms = false
	r.Config.Hygiene.CheckCLINames = false
	r.Config.Hygiene.CheckSubprocessHygiene = false
	r.Config.Hygiene.CheckFileutilHygiene = false
	r.Config.Hygiene.CheckCommandSpecs = false
	r.Config.Hygiene.CheckCLIBuilders = false
	r.Config.Hygiene.CheckReferenceIntegrity = false
	r.Config.Hygiene.CheckRawGoroutines = true
	r.Config.Hygiene.CheckDups = false

	_, err := r.Run(RunOptions{
		Suites: []string{"hygiene"},
		Files:  []string{tempDir},
	})
	assert.Error(t, err)

	// Test CheckDups error propagation in runHygiene
	r.Config.Hygiene.CheckRawGoroutines = false
	r.Config.Hygiene.CheckDups = true
	_, err = r.Run(RunOptions{
		Suites: []string{"hygiene"},
		Files:  []string{tempDir},
	})
	assert.Error(t, err)
}

func TestSpecs_NonYAMLFilesIgnored_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.CLISpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "notes.txt"), []byte("non yaml file"), paths.FilePerm644))

	findings, err := CheckCLIBuilders(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	assert.Empty(t, findings)

	specFindings, err := CheckCommandSpecs(tempDir, DefaultConfig(), nil)
	require.NoError(t, err)
	assert.Empty(t, specFindings)
}

func TestCheckTreePolice_ScriptFilterAndForbiddenRef_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	scriptsDir := filepath.Join(tempDir, "scripts")
	require.NoError(t, fileutil.MkdirAll(scriptsDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(scriptsDir, "README.txt"), []byte("non script"), paths.FilePerm644))

	cfg := DefaultConfig()
	cfg.TreePolice.AllowedScripts = []string{"allowed.sh"}
	findings, err := CheckTreePolice(tempDir, cfg)
	require.NoError(t, err)
	assert.Empty(t, findings)

	// Test forbidden script references using repo root
	repoCfg := DefaultConfig()
	repoCfg.TreePolice.ForbiddenScriptReferences = []string{"package vet"}
	repoCfg.TreePolice.GrepExemptions = []string{"pkg/other/*"}
	repoFindings, err := CheckTreePolice("../..", repoCfg)
	require.NoError(t, err)
	assert.NotEmpty(t, repoFindings)

	hasScriptRef := false
	for _, f := range repoFindings {
		if f.CheckID == "tree/forbidden-script-ref" {
			hasScriptRef = true
			break
		}
	}
	assert.True(t, hasScriptRef, "expected forbidden script ref finding")
}

func TestCheckTestExec_BrokenSyntaxInTestFile_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "sample")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	brokenTestSrc := `package sample broken(`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "broken_test.go"), []byte(brokenTestSrc), paths.FilePerm644))

	findings, err := CheckTestSubprocessHygiene(tempDir, DefaultConfig())
	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestRunner_SeverityWarnIncrementsTotalWarns_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "driftwarn")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	src := `package driftwarn
const (
	ActA = "a"
	ActB = "b"
	ActC = "c"
)
var ActMap = map[string]int{
	ActA: 1,
	ActB: 2,
	ActC: 3,
}
func Check(a string) bool {
	if a == ActA || a == ActB {
		return true
	}
	return false
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "drift.go"), []byte(src), paths.FilePerm644))

	r := NewRunner(tempDir, DefaultConfig())
	r.Config.Hygiene.CheckPaths = false
	r.Config.Hygiene.CheckPerms = false
	r.Config.Hygiene.CheckCLINames = false
	r.Config.Hygiene.CheckSubprocessHygiene = false
	r.Config.Hygiene.CheckFileutilHygiene = false
	r.Config.Hygiene.CheckCommandSpecs = false
	r.Config.Hygiene.CheckCLIBuilders = false
	r.Config.Hygiene.CheckReferenceIntegrity = false
	r.Config.Hygiene.CheckRawGoroutines = false
	r.Config.Hygiene.CheckDups = true

	rep, err := r.Run(RunOptions{Suites: []string{"hygiene"}})
	require.NoError(t, err)
	assert.True(t, rep.Passed)
	assert.Greater(t, rep.TotalWarns, 0)
	assert.Equal(t, 0, rep.TotalErrors)
}

func TestCheckDuplication_MultiCandidatesSameFileSort_Cycle238(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "multisort")
	require.NoError(t, fileutil.MkdirAll(pkgDir, paths.DirPerm755))

	srcA := `package multisort
func Alpha1() int {
	a := 1
	b := 2
	c := a + b
	d := c * 2
	e := d - 5
	return e
}
func Alpha2() int {
	m := 10
	n := 20
	o := m + n
	p := o * 2
	q := p - 5
	return q
}
`
	srcB := `package multisort
func Beta1() int {
	a := 1
	b := 2
	c := a + b
	d := c * 2
	e := d - 5
	return e
}
func Beta2() int {
	m := 10
	n := 20
	o := m + n
	p := o * 2
	q := p - 5
	return q
}
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "a.go"), []byte(srcA), paths.FilePerm644))
	require.NoError(t, fileutil.WriteFile(filepath.Join(pkgDir, "b.go"), []byte(srcB), paths.FilePerm644))

	cfg := &GatesConfig{
		Hygiene: HygieneConfig{
			CheckDups:        true,
			DupMinStatements: 3,
			DupMinLines:      3,
			GoScanDirs:       []string{"pkg/"},
		},
	}
	findings, err := CheckDuplication(tempDir, nil, cfg)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(findings), 2)
}
