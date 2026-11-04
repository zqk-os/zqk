package drifthotspots

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCycle261_BarrelsAndCatalogs exercises all branches of IsConstantBarrelPath and IsDataCatalogPath.
func TestCycle261_BarrelsAndCatalogs(t *testing.T) {
	barrelCases := []struct {
		path     string
		expected bool
	}{
		{"lock_op_names.go", true},
		{"path/to/lock_op_names.go", true},
		{"pkg/specbuilder/bldr_v2/entity_constants.go", true},
		{"pkg/specbuilder/bldr_v2/entity.go", false},
		{"pkg/specbuilder/other/entity_constants.go", false},
		{"random_file.go", false},
		{"", false},
	}
	for _, tc := range barrelCases {
		if got := IsConstantBarrelPath(tc.path); got != tc.expected {
			t.Errorf("IsConstantBarrelPath(%q) = %v; want %v", tc.path, got, tc.expected)
		}
	}

	catalogCases := []struct {
		path     string
		expected bool
	}{
		{"my_test_helper.go", true},
		{"pkg/sub/another_test_helper.go", true},
		{"pkg/objects/kind_mappings.go", true},
		{"pkg/validation/id_prefixes_config.go", true},
		{"pkg/validation/namespaces_config.go", true},
		{"pkg/specbuilder/bldr_config_v1/kind_mappings_config_builder.go", true},
		{"pkg/specbuilder/bldr_config_v1/id_prefixes_config_builder.go", true},
		{"pkg/specbuilder/bldr_config_v1/namespaces_config_builder.go", true},
		{"scripts/generate-readme-index.go", true},
		{"scripts/generate-process-readme-index.go", true},
		{"pkg/objects/other.go", false},
		{"pkg/cli/main.go", false},
	}
	for _, tc := range catalogCases {
		if got := IsDataCatalogPath(tc.path); got != tc.expected {
			t.Errorf("IsDataCatalogPath(%q) = %v; want %v", tc.path, got, tc.expected)
		}
	}
}

// TestCycle261_RiskRanks verifies rank mappings and min floor threshold defaults.
func TestCycle261_RiskRanks(t *testing.T) {
	if got := riskRank(RiskCritical); got != 4 {
		t.Fatalf("riskRank(critical) = %d; want 4", got)
	}
	if got := riskRank(RiskHigh); got != 3 {
		t.Fatalf("riskRank(high) = %d; want 3", got)
	}
	if got := riskRank(RiskMedium); got != 2 {
		t.Fatalf("riskRank(medium) = %d; want 2", got)
	}
	if got := riskRank(RiskLow); got != 1 {
		t.Fatalf("riskRank(low) = %d; want 1", got)
	}
	if got := riskRank("unknown"); got != 0 {
		t.Fatalf("riskRank(unknown) = %d; want 0", got)
	}

	if got := minRiskFloorRank(""); got != 1 {
		t.Fatalf("minRiskFloorRank(\"\") = %d; want 1", got)
	}
	if got := minRiskFloorRank(" critical "); got != 4 {
		t.Fatalf("minRiskFloorRank(critical) = %d; want 4", got)
	}
	if got := minRiskFloorRank("HIGH"); got != 3 {
		t.Fatalf("minRiskFloorRank(HIGH) = %d; want 3", got)
	}
	if got := minRiskFloorRank("Medium"); got != 2 {
		t.Fatalf("minRiskFloorRank(Medium) = %d; want 2", got)
	}
	if got := minRiskFloorRank("Low"); got != 1 {
		t.Fatalf("minRiskFloorRank(Low) = %d; want 1", got)
	}
	if got := minRiskFloorRank("fallback-unrecognized"); got != 1 {
		t.Fatalf("minRiskFloorRank(fallback) = %d; want 1", got)
	}
	if objects.DefaultSchemaVersion == "" {
		t.Fatal("expected DefaultSchemaVersion")
	}
}

// TestCycle261_ASTHelperPredicates tests lower-level AST predicates directly.
func TestCycle261_ASTHelperPredicates(t *testing.T) {
	fset := token.NewFileSet()
	src := `package testpkg

import "fmt"

const ConstThreshold = 42

type Sample struct {
	TagField string ` + "`json:\"tag_val\"`" + `
}

func helper(st string) {
	var m = map[string]any{"id": 1}
	_ = m["id"]
	switch st {
	case "pending":
		fmt.Println(st)
	}
	if st == "draft" {
	}
	if "in_progress" == st {
	}
	SetField("id", 10)
	sampleObj.SetField("kind", "goal")
	otherCall("id")
}
`
	file, err := parser.ParseFile(fset, "sample.go", src, 0)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Verify exprHasStatusSignal
	if exprHasStatusSignal(nil) {
		t.Fatal("exprHasStatusSignal(nil) should be false")
	}

	identSt := &ast.Ident{Name: "st"}
	if !exprHasStatusSignal(identSt) {
		t.Fatal("exprHasStatusSignal(st) should be true")
	}
	identStatus := &ast.Ident{Name: "currentStatus"}
	if !exprHasStatusSignal(identStatus) {
		t.Fatal("exprHasStatusSignal(currentStatus) should be true")
	}
	identOther := &ast.Ident{Name: "otherVar"}
	if exprHasStatusSignal(identOther) {
		t.Fatal("exprHasStatusSignal(otherVar) should be false")
	}

	selStatus := &ast.SelectorExpr{X: &ast.Ident{Name: "obj"}, Sel: &ast.Ident{Name: "ItemStatus"}}
	if !exprHasStatusSignal(selStatus) {
		t.Fatal("exprHasStatusSignal(selStatus) should be true")
	}
	selRecursive := &ast.SelectorExpr{X: &ast.Ident{Name: "userStatus"}, Sel: &ast.Ident{Name: "Value"}}
	if !exprHasStatusSignal(selRecursive) {
		t.Fatal("exprHasStatusSignal(selRecursive) should be true")
	}

	idxStatus := &ast.IndexExpr{
		X:     &ast.Ident{Name: "m"},
		Index: &ast.BasicLit{Kind: token.STRING, Value: `"status"`},
	}
	if !exprHasStatusSignal(idxStatus) {
		t.Fatal("exprHasStatusSignal(idxStatus) should be true")
	}

	callStatus := &ast.CallExpr{
		Fun: &ast.Ident{Name: "getStatus"},
	}
	if !exprHasStatusSignal(callStatus) {
		t.Fatal("exprHasStatusSignal(callStatus) should be true")
	}

	// isSetFieldFun
	if !isSetFieldFun(&ast.Ident{Name: "SetField"}) {
		t.Fatal("isSetFieldFun(SetField) should be true")
	}
	if isSetFieldFun(&ast.Ident{Name: "OtherFun"}) {
		t.Fatal("isSetFieldFun(OtherFun) should be false")
	}
	if !isSetFieldFun(&ast.SelectorExpr{X: &ast.Ident{Name: "obj"}, Sel: &ast.Ident{Name: "SetField"}}) {
		t.Fatal("isSetFieldFun(obj.SetField) should be true")
	}
	if isSetFieldFun(&ast.SelectorExpr{X: &ast.Ident{Name: "obj"}, Sel: &ast.Ident{Name: "GetField"}}) {
		t.Fatal("isSetFieldFun(obj.GetField) should be false")
	}
	if isSetFieldFun(&ast.SelectorExpr{X: &ast.Ident{Name: "obj"}, Sel: nil}) {
		t.Fatal("isSetFieldFun(nil Sel) should be false")
	}
	if isSetFieldFun(&ast.BasicLit{Kind: token.STRING, Value: `"foo"`}) {
		t.Fatal("isSetFieldFun(BasicLit) should be false")
	}

	_ = file
}

// TestCycle261_AnalyzerFileFiltering verifies analyzeFile skips and edge cases.
func TestCycle261_AnalyzerFileFiltering(t *testing.T) {
	opts := Options{
		RootDir:                t.TempDir(),
		SpecsDir:               t.TempDir(),
		IncludeTests:           false,
		IncludeConstantBarrels: false,
		MinRisk:                "low",
	}

	// 1. walkErr != nil
	testErr := fmt.Errorf("walk synthetic error")
	var findings []Finding
	err := analyzeFile("any_path", nil, testErr, opts, 1, nil, &findings)
	if err != testErr {
		t.Fatalf("expected testErr, got: %v", err)
	}

	// 2. info.IsDir() base == "vendor" or ".git"
	vendorDir := &mockFileInfo{isDir: true, name: "vendor"}
	if err := analyzeFile("vendor", vendorDir, nil, opts, 1, nil, &findings); err != filepath.SkipDir {
		t.Fatalf("expected filepath.SkipDir for vendor, got %v", err)
	}
	gitDir := &mockFileInfo{isDir: true, name: ".git"}
	if err := analyzeFile(".git", gitDir, nil, opts, 1, nil, &findings); err != filepath.SkipDir {
		t.Fatalf("expected filepath.SkipDir for .git, got %v", err)
	}
	regularDir := &mockFileInfo{isDir: true, name: "regular"}
	if err := analyzeFile("regular", regularDir, nil, opts, 1, nil, &findings); err != nil {
		t.Fatalf("expected nil for regular dir, got %v", err)
	}

	// 3. Non-.go file
	txtFile := &mockFileInfo{isDir: false, name: "notes.txt"}
	if err := analyzeFile("notes.txt", txtFile, nil, opts, 1, nil, &findings); err != nil {
		t.Fatalf("expected nil for non-go, got %v", err)
	}

	// 4. Test file when IncludeTests is false
	testFile := &mockFileInfo{isDir: false, name: "sample_test.go"}
	if err := analyzeFile("sample_test.go", testFile, nil, opts, 1, nil, &findings); err != nil {
		t.Fatalf("expected nil for excluded test, got %v", err)
	}

	// 5. Generated go files
	genFile1 := &mockFileInfo{isDir: false, name: "zz_generated.go"}
	if err := analyzeFile("zz_generated.go", genFile1, nil, opts, 1, nil, &findings); err != nil {
		t.Fatalf("expected nil for zz_generated, got %v", err)
	}
	genFile2 := &mockFileInfo{isDir: false, name: "generated_types.go"}
	if err := analyzeFile("generated_types.go", genFile2, nil, opts, 1, nil, &findings); err != nil {
		t.Fatalf("expected nil for generated_, got %v", err)
	}

	// 6. Constant barrels when IncludeConstantBarrels is false
	barrelFile := &mockFileInfo{isDir: false, name: "lock_op_names.go"}
	if err := analyzeFile("lock_op_names.go", barrelFile, nil, opts, 1, nil, &findings); err != nil {
		t.Fatalf("expected nil for excluded barrel, got %v", err)
	}

	// 7. Non-existent file on disk returns ReadFile error
	missingFile := &mockFileInfo{isDir: false, name: "missing.go"}
	if err := analyzeFile(filepath.Join(opts.RootDir, "missing.go"), missingFile, nil, opts, 1, nil, &findings); err == nil {
		t.Fatal("expected ReadFile error for non-existent file")
	}

	// 8. Generated source content
	genSrcPath := filepath.Join(opts.RootDir, "gen_src.go")
	if err := fileutil.WriteFile(genSrcPath, []byte("// Code generated by generator; DO NOT EDIT.\npackage p\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	genSrcInfo := &mockFileInfo{isDir: false, name: "gen_src.go"}
	if err := analyzeFile(genSrcPath, genSrcInfo, nil, opts, 1, nil, &findings); err != nil {
		t.Fatalf("expected nil for generated content, got %v", err)
	}

	// 9. Unparseable file content (syntax error) is skipped gracefully
	badSrcPath := filepath.Join(opts.RootDir, "syntax_error.go")
	if err := fileutil.WriteFile(badSrcPath, []byte("package 12345 invalid syntax\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	badSrcInfo := &mockFileInfo{isDir: false, name: "syntax_error.go"}
	if err := analyzeFile(badSrcPath, badSrcInfo, nil, opts, 1, nil, &findings); err != nil {
		t.Fatalf("expected nil (skipped) for unparseable file, got %v", err)
	}
}

// TestCycle261_InspectASTComprehensive exercises all category finders and branch conditions in inspectAST.
func TestCycle261_InspectASTComprehensive(t *testing.T) {
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	// Write spec to create known kinds
	if err := fileutil.WriteFile(filepath.Join(specsDir, "goal.yaml"), []byte("ontology: goal\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "src")
	if err := fileutil.MkdirAll(srcDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	// File 1: non-test file exercising all categories
	comprehensiveSrc := `package comprehensive

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

const DeclaredConst = 999

type RegistryMock struct{}
func (r *RegistryMock) Run(ctx context.Context, interval int, monitorID string) {}

func DummyLogger() {
	var cmd cobra.Command
	var b bool
	var s string
	var i int
	cmd.Flags().BoolVar(&b, "bool-flag", false, "help")
	cmd.Flags().BoolVarP(&b, "bool-flag-p", "p", false, "help")
	cmd.Flags().StringVar(&s, "str-flag", "", "help")
	cmd.Flags().StringVarP(&s, "str-flag-p", "s", "", "help")
	cmd.Flags().IntVar(&i, "int-flag", 0, "help")
	cmd.Flags().IntVarP(&i, "int-flag-p", "i", 0, "help")

	_ = exec.Command("git", "status")
	_ = exec.CommandContext(context.Background(), "docker", "ps")

	var reg RegistryMock
	reg.Run(context.Background(), 10, "health_monitor_alpha")

	_ = filepath.Join("project_root", "subdir_a", "subdir_b")

	_ = storage.ListFilter{
		SortBy: "name_field",
		Filters: map[string]any{
			objects.FieldKeyStatus: "archived",
		},
	}

	var rawMap = map[string]any{"id": "abc"}
	_ = rawMap["id"]
	_ = rawMap["schema_version"]

	var ver = "2.0.0"
	_ = ver

	var statusVal = "archived"
	if statusVal == "archived" {
		fmt.Println("archived")
	}
	var st = "active"
	if st == "draft" {
	}
	if "in_progress" == st {
	}

	switch st {
	case "completed":
		fmt.Println("done")
	case "pending":
		fmt.Println("pending")
	}

	var k = "goal"
	if k == "goal" {
	}

	// Magic numbers
	var magicInt = 8080
	var magicFloat = 3.14159
	var magicImag = 4.5i
	_ = magicInt
	_ = magicFloat
	_ = magicImag

	// Repeated strings (repeated 3 times)
	fmt.Println("repeated_sample_domain_string")
	fmt.Println("repeated_sample_domain_string")
	fmt.Println("repeated_sample_domain_string")
}
`
	compFilePath := filepath.Join(srcDir, "comprehensive.go")
	if err := fileutil.WriteFile(compFilePath, []byte(comprehensiveSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// File 2: test file exercising the isTest risk downgrades
	testSrc := `package comprehensive_test

import (
	"testing"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestSomething(t *testing.T) {
	var ver = "2.0.0"
	_ = ver
	var k = "goal"
	if k == "goal" {
	}
	var rawMap = map[string]any{"id": "1"}
	_ = rawMap["id"]

	var st = "draft"
	if st == "draft" {
	}
	var num = 404
	_ = num
}
`
	testFilePath := filepath.Join(srcDir, "comprehensive_test.go")
	if err := fileutil.WriteFile(testFilePath, []byte(testSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// File 3: Specbuilder builder path (kind literals assignment skipped, but compare flagged)
	bldrDir := filepath.Join(srcDir, "pkg", "specbuilder", "bldr_v2")
	if err := fileutil.MkdirAll(bldrDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	bldrSrc := `package bldr_v2

func BuildEntities() {
	var assignKind = "goal"
	_ = assignKind
	if assignKind == "goal" {
	}
}
`
	bldrFilePath := filepath.Join(bldrDir, "entity_builder.go")
	if err := fileutil.WriteFile(bldrFilePath, []byte(bldrSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// File 4: Data catalog path (assignment skipped, compare flagged)
	catalogDir := filepath.Join(srcDir, "pkg", "objects")
	if err := fileutil.MkdirAll(catalogDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	catalogSrc := `package objects

func CatalogData() {
	var assignKind = "goal"
	_ = assignKind
	if assignKind == "goal" {
	}
}
`
	catalogFilePath := filepath.Join(catalogDir, "kind_mappings.go")
	if err := fileutil.WriteFile(catalogFilePath, []byte(catalogSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// File 5: Constants barrel (pkg/objects/constants.go) where kind & status & schema literals are skipped
	barrelConstantsSrc := `package objects

const (
	MySchema = "2.0.0"
	MyKind = "goal"
	MyStatus = "completed"
)
`
	barrelConstantsPath := filepath.Join(catalogDir, "constants.go")
	if err := fileutil.WriteFile(barrelConstantsPath, []byte(barrelConstantsSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// Run Analyze across all files
	findings, err := Analyze(Options{
		RootDir:                tmp,
		SpecsDir:               specsDir,
		IncludeTests:           true,
		IncludeConstantBarrels: true,
		MinRisk:                "low",
		ExtraKindSet:           map[string]struct{}{"custom_extra_kind": {}},
	})
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected multiple findings, got 0")
	}

	catSeen := map[string]int{}
	for _, f := range findings {
		catSeen[f.Category]++
	}

	expectedCategories := []string{
		CategoryCobraFlagLiteral,
		CategoryExecCommandLiteral,
		CategoryRegistryRunMonitorID,
		CategoryFilepathJoinSegment,
		CategoryListFilterSortBy,
		CategoryListFilterStatus,
		CategorySystemFieldKey,
		CategorySchemaVersionLiteral,
		CategoryObjectStatusCompare,
		CategoryKindLiteralCompare,
		CategoryKindLiteralAssign,
		CategoryMagicNumberLiteral,
		CategoryRepeatedString,
	}

	for _, expCat := range expectedCategories {
		if catSeen[expCat] == 0 {
			t.Errorf("category %q was not observed in findings: %#v", expCat, catSeen)
		}
	}

	// Build report and verify aggregation and sort stability
	rep := BuildReport(Options{RootDir: tmp, MinRisk: "low"}, findings)
	if len(rep.Findings) != len(findings) {
		t.Errorf("report findings mismatch: %d vs %d", len(rep.Findings), len(findings))
	}
	if len(rep.FileSummaries) == 0 {
		t.Error("expected non-empty FileSummaries in report")
	}
	// Verify sorted descending by count
	for i := 1; i < len(rep.FileSummaries); i++ {
		if rep.FileSummaries[i].Count > rep.FileSummaries[i-1].Count {
			t.Errorf("FileSummaries not sorted descending: %d > %d", rep.FileSummaries[i].Count, rep.FileSummaries[i-1].Count)
		}
	}
}

// TestCycle261_AnalyzeValidationAndErrors verifies error branches in Analyze and loadKindSetForAnalyze.
func TestCycle261_AnalyzeValidationAndErrors(t *testing.T) {
	// Empty RootDir error
	if _, err := Analyze(Options{RootDir: ""}); err == nil {
		t.Fatal("expected error for empty RootDir")
	}

	// Invalid specs dir error
	if _, err := Analyze(Options{
		RootDir:  t.TempDir(),
		SpecsDir: filepath.Join(t.TempDir(), "non_existent_specs"),
	}); err == nil {
		t.Fatal("expected error for non-existent specs dir")
	}

	// Test defaultSpecs match branch in loadKindSetForAnalyze
	tmp := t.TempDir()
	defaultSpecsPath := filepath.Join(tmp, paths.ProcessInternalObjectSpecsDir)
	_ = fileutil.MkdirAll(defaultSpecsPath, paths.DirPerm755)
	_, _ = loadKindSetForAnalyze(Options{
		RootDir:  tmp,
		SpecsDir: defaultSpecsPath,
	})

	// Test filepath.Walk error when root directory is an inaccessible file or walk encounters error
	unreadableFile := filepath.Join(tmp, "unreadable_root")
	if err := fileutil.WriteFile(unreadableFile, []byte("data"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	// Passing a regular file as RootDir when it encounters walk issues or unreadable subdirs
	subDir := filepath.Join(tmp, "locked_dir")
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(subDir, 0000)
	defer os.Chmod(subDir, 0755)

	// In some operating systems / users chmod 0000 causes Walk to return an error
	_, _ = Analyze(Options{
		RootDir:  subDir,
		SpecsDir: defaultSpecsPath,
	})
}

// TestCycle261_FieldKeyLiteralFixerDetailed tests FixFieldKeyLiteralsInFile and WalkAndFixFieldKeyLiterals.
func TestCycle261_FieldKeyLiteralFixerDetailed(t *testing.T) {
	tmpRepo := t.TempDir()
	_ = fileutil.WriteFile(filepath.Join(tmpRepo, "go.mod"), []byte("module testrepo\ngo 1.22\n"), paths.FilePerm644)

	wireToConst := map[string]string{
		"id":             "FieldKeyID",
		"kind":           "FieldKeyKind",
		"status":         "FieldKeyStatus",
		"schema_version": "FieldKeySchemaVersion",
	}

	// 1. Syntax error
	if _, _, err := FixFieldKeyLiteralsInFile("bad.go", []byte("package 123 syntax error"), wireToConst, ""); err == nil {
		t.Fatal("expected error on syntax error")
	}

	// 2. No replacement needed
	noChangeSrc := []byte("package p\nfunc f() { x := 1; _ = x }\n")
	resSrc, count, err := FixFieldKeyLiteralsInFile("p.go", noChangeSrc, wireToConst, "p")
	if err != nil || count != 0 || string(resSrc) != string(noChangeSrc) {
		t.Fatalf("expected 0 fixes for untouched source, got %d, err: %v", count, err)
	}

	// 3. Various fix sites: map index, map literal key, SetField call, obj.SetField call
	srcToFix := `package demo

type SampleObj struct{}
func (s *SampleObj) SetField(k string, v any) {}
func SetField(k string, v any) {}

func Run() {
	var m = map[string]any{
		"id": 100,
		"schema_version": "2.0.0",
	}
	m["kind"] = "test"
	_ = m["status"]

	SetField("id", 200)

	var obj SampleObj
	obj.SetField("kind", "goal")
}
`
	demoPath := filepath.Join(tmpRepo, "pkg", "demo", "demo.go")
	fixedSrc, count, err := FixFieldKeyLiteralsInFile(demoPath, []byte(srcToFix), wireToConst, "demo")
	if err != nil {
		t.Fatalf("FixFieldKeyLiteralsInFile failed: %v", err)
	}
	if count < 6 {
		t.Fatalf("expected at least 6 fixes, got %d", count)
	}
	fixedStr := string(fixedSrc)
	if !strings.Contains(fixedStr, "objects.FieldKeyID") {
		t.Errorf("missing objects.FieldKeyID in fixed source: %s", fixedStr)
	}
	if !strings.Contains(fixedStr, "objects.FieldKeyKind") {
		t.Errorf("missing objects.FieldKeyKind in fixed source: %s", fixedStr)
	}
	if !strings.Contains(fixedStr, "objects.FieldKeyStatus") {
		t.Errorf("missing objects.FieldKeyStatus in fixed source: %s", fixedStr)
	}
	if !strings.Contains(fixedStr, "objects.FieldKeySchemaVersion") {
		t.Errorf("missing objects.FieldKeySchemaVersion in fixed source: %s", fixedStr)
	}

	// 4. effectivePkg == "objects" replaces with identifier without "objects." prefix
	objPath := filepath.Join(tmpRepo, "pkg", "objects", "demo.go")
	fixedObjectsPkg, countObj, err := FixFieldKeyLiteralsInFile(objPath, []byte(srcToFix), wireToConst, "objects")
	if err != nil {
		t.Fatalf("FixFieldKeyLiteralsInFile for objects pkg failed: %v", err)
	}
	if countObj != count {
		t.Fatalf("expected %d fixes in objects pkg, got %d", count, countObj)
	}
	if strings.Contains(string(fixedObjectsPkg), "objects.FieldKeyID") {
		t.Errorf("objects pkg should not contain 'objects.' qualifier, got:\n%s", string(fixedObjectsPkg))
	}
	if !strings.Contains(string(fixedObjectsPkg), "FieldKeyID") {
		t.Errorf("objects pkg should contain 'FieldKeyID', got:\n%s", string(fixedObjectsPkg))
	}

	// 5. Test WalkAndFixFieldKeyLiterals with various configurations
	fieldKeysPath := filepath.Join(tmpRepo, "pkg", "objects", "field_keys.go")
	if err := fileutil.MkdirAll(filepath.Dir(fieldKeysPath), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	fkContent := `package objects

const (
	FieldKeyID = "id"
	FieldKeyKind = "kind"
	FieldKeyStatus = "status"
)
`
	if err := fileutil.WriteFile(fieldKeysPath, []byte(fkContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// File under pkg/service to fix
	svcFile := filepath.Join(tmpRepo, "pkg", "service", "svc.go")
	if err := fileutil.MkdirAll(filepath.Dir(svcFile), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	svcSrc := `package service

func Work() {
	var m = map[string]any{"id": 1}
	_ = m["id"]
}
`
	if err := fileutil.WriteFile(svcFile, []byte(svcSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// File with syntax error
	badFile := filepath.Join(tmpRepo, "pkg", "service", "broken.go")
	if err := fileutil.WriteFile(badFile, []byte("package service syntax error !!!"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// File in excluded prefix
	otherFile := filepath.Join(tmpRepo, "cmd", "tool", "tool.go")
	if err := fileutil.MkdirAll(filepath.Dir(otherFile), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(otherFile, []byte("package main\nfunc main() {}\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// Test Walk with empty RepoRoot
	if _, err := WalkAndFixFieldKeyLiterals(WalkFieldKeyFixConfig{}); err == nil {
		t.Fatal("expected error on empty RepoRoot")
	}

	// Test Walk with invalid field keys path
	if _, err := WalkAndFixFieldKeyLiterals(WalkFieldKeyFixConfig{
		RepoRoot:      tmpRepo,
		FieldKeysPath: filepath.Join(tmpRepo, "non_existent.go"),
	}); err == nil {
		t.Fatal("expected error on invalid field keys path")
	}

	// Test Walk with explicit FilePaths and Write=true
	res, err := WalkAndFixFieldKeyLiterals(WalkFieldKeyFixConfig{
		RepoRoot:      tmpRepo,
		FieldKeysPath: fieldKeysPath,
		Write:         true,
		IncludeTests:  true,
		FilePaths: []string{
			"pkg/service/svc.go",
			"pkg/service/broken.go",
			"non_existent.go",
			"cmd/tool/tool.go",
		},
		PathPrefixes: []string{"pkg/service"},
	})
	if err != nil {
		t.Fatalf("WalkAndFixFieldKeyLiterals failed: %v", err)
	}
	if res.FilesScanned != 2 {
		t.Errorf("expected 2 files scanned under prefix, got %d", res.FilesScanned)
	}
	if res.FilesChanged != 1 {
		t.Errorf("expected 1 file changed, got %d", res.FilesChanged)
	}
	if len(res.Errors) == 0 {
		t.Error("expected errors logged for broken.go, got 0")
	}

	// Verify svc.go was written
	writtenSrc, _ := fileutil.ReadFile(svcFile)
	if !strings.Contains(string(writtenSrc), "objects.FieldKeyID") {
		t.Errorf("expected objects.FieldKeyID written to disk in svc.go: %s", string(writtenSrc))
	}
}

// TestCycle261_ConcurrencyStress ensures concurrent invocations of Analyze, BuildReport, and Fixer are data-race free.
func TestCycle261_ConcurrencyStress(t *testing.T) {
	tmp := t.TempDir()
	// Write go.mod in tmp so imports.Process has an immediate module boundary
	_ = fileutil.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module sample\ngo 1.22\n"), paths.FilePerm644)

	specsDir := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(specsDir, "sample.yaml"), []byte("ontology: sample_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "src")
	if err := fileutil.MkdirAll(srcDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	sampleSrc := `package sample

import "github.com/zqk-os/zqk/pkg/objects"

func Do() {
	var m = map[string]any{"id": 1, "kind": "sample_item"}
	if m["kind"] == "sample_item" {
	}
	_ = objects.FieldKeyID
}
`
	sampleFilePath := filepath.Join(srcDir, "sample.go")
	if err := fileutil.WriteFile(sampleFilePath, []byte(sampleSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	wireToConst := map[string]string{
		"id":   "FieldKeyID",
		"kind": "FieldKeyKind",
	}

	var wg sync.WaitGroup
	workers := 8
	iterations := 4

	for w := 0; w < workers; w++ {
		wg.Add(1)
		workerID := w
		goroutinelabels.NewGoroutine("test_cycle261_concurrency_stress", fmt.Sprintf("worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// 1. Analyze
				findings, err := Analyze(Options{
					RootDir:  srcDir,
					SpecsDir: specsDir,
					MinRisk:  "low",
				})
				if err != nil {
					t.Errorf("worker %d Analyze failed: %v", workerID, err)
					return
				}

				// 2. BuildReport
				rep := BuildReport(Options{RootDir: srcDir, MinRisk: "low"}, findings)
				if len(rep.Findings) != len(findings) {
					t.Errorf("worker %d BuildReport findings length mismatch", workerID)
					return
				}

				// 3. FixFieldKeyLiteralsInFile
				_, _, err = FixFieldKeyLiteralsInFile(sampleFilePath, []byte(sampleSrc), wireToConst, "sample")
				if err != nil {
					t.Errorf("worker %d FixFieldKeyLiteralsInFile failed: %v", workerID, err)
					return
				}

				// 4. Barrel Predicates
				_ = IsConstantBarrelPath("pkg/specbuilder/bldr_v2/foo_constants.go")
				_ = IsDataCatalogPath("pkg/objects/kind_mappings.go")
			}
		})
	}
	wg.Wait()
}

// TestCycle261_ASTPredicateEdgeCases directly exercises individual branches of AST helper functions.
func TestCycle261_ASTPredicateEdgeCases(t *testing.T) {
	// 1. isFilepathJoinSegmentLiteral edge cases
	lit1 := &ast.BasicLit{Kind: token.STRING, Value: `"first"`}
	lit2 := &ast.BasicLit{Kind: token.STRING, Value: `"second"`}
	joinCall := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: &ast.Ident{Name: "filepath"}, Sel: &ast.Ident{Name: "Join"}},
		Args: []ast.Expr{lit1, lit2},
	}
	pathFirst := []ast.Node{lit1, joinCall}
	pathSecond := []ast.Node{lit2, joinCall}

	if isFilepathJoinSegmentLiteral(pathFirst, lit1) {
		t.Error("first segment in filepath.Join should not be flagged")
	}
	if !isFilepathJoinSegmentLiteral(pathSecond, lit2) {
		t.Error("second segment in filepath.Join should be flagged")
	}

	// Call not filepath.Join
	cleanCall := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: &ast.Ident{Name: "filepath"}, Sel: &ast.Ident{Name: "Clean"}},
		Args: []ast.Expr{lit1},
	}
	if isFilepathJoinSegmentLiteral([]ast.Node{lit1, cleanCall}, lit1) {
		t.Error("filepath.Clean should not be flagged as filepath.Join")
	}

	otherPkgCall := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: &ast.Ident{Name: "strings"}, Sel: &ast.Ident{Name: "Join"}},
		Args: []ast.Expr{lit1, lit2},
	}
	if isFilepathJoinSegmentLiteral([]ast.Node{lit2, otherPkgCall}, lit2) {
		t.Error("strings.Join should not be flagged as filepath.Join")
	}

	// 2. isLoggingFieldKeyLiteral edge cases
	logLitKey := &ast.BasicLit{Kind: token.STRING, Value: `"request_id"`}
	logLitVal := &ast.BasicLit{Kind: token.STRING, Value: `"req_123"`}
	logCall := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: &ast.Ident{Name: "logging"}, Sel: &ast.Ident{Name: "String"}},
		Args: []ast.Expr{logLitKey, logLitVal},
	}
	if !isLoggingFieldKeyLiteral([]ast.Node{logLitKey, logCall}, "request_id") {
		t.Error("logging.String first arg with underscore should be recognized as logging key")
	}
	if isLoggingFieldKeyLiteral([]ast.Node{logLitVal, logCall}, "req_123") {
		t.Error("logging.String second arg should not be recognized as logging key")
	}

	otherLogFun := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: &ast.Ident{Name: "logging"}, Sel: &ast.Ident{Name: "Info"}},
		Args: []ast.Expr{logLitKey},
	}
	if isLoggingFieldKeyLiteral([]ast.Node{logLitKey, otherLogFun}, "request_id") {
		t.Error("logging.Info should not be recognized as logging field key literal")
	}

	otherPkgLog := &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: &ast.Ident{Name: "other"}, Sel: &ast.Ident{Name: "String"}},
		Args: []ast.Expr{logLitKey, logLitVal},
	}
	if isLoggingFieldKeyLiteral([]ast.Node{logLitKey, otherPkgLog}, "request_id") {
		t.Error("other.String should not be recognized as logging field key literal")
	}

	// 3. isFieldKeyLiteralFixSite edge cases
	fixLit := &ast.BasicLit{Kind: token.STRING, Value: `"id"`}
	valLit := &ast.BasicLit{Kind: token.STRING, Value: `"val"`}

	// KeyValueExpr where lit is Value, not Key
	kvVal := &ast.KeyValueExpr{Key: &ast.Ident{Name: "k"}, Value: fixLit}
	if isFieldKeyLiteralFixSite([]ast.Node{fixLit, kvVal}, fixLit) {
		t.Error("KeyValueExpr value should not be fix site")
	}

	// CallExpr where lit is not 1st arg
	setFieldCall := &ast.CallExpr{
		Fun:  &ast.Ident{Name: "SetField"},
		Args: []ast.Expr{valLit, fixLit},
	}
	if isFieldKeyLiteralFixSite([]ast.Node{fixLit, setFieldCall}, fixLit) {
		t.Error("SetField 2nd arg should not be fix site")
	}

	// Default unknown parent
	if isFieldKeyLiteralFixSite([]ast.Node{fixLit, &ast.ReturnStmt{}}, fixLit) {
		t.Error("ReturnStmt should not be fix site")
	}

	// 4. loadKindSetForAnalyze with module root
	repoRoot, err := paths.ModuleRootFromPath(".")
	if err == nil {
		defaultSpecsPath := filepath.Join(repoRoot, paths.ProcessInternalObjectSpecsDir)
		m, err := loadKindSetForAnalyze(Options{
			RootDir:  repoRoot,
			SpecsDir: defaultSpecsPath,
		})
		if err == nil && len(m) > 0 {
			if _, ok := m["goal"]; !ok {
				t.Logf("kinds loaded: %d kinds", len(m))
			}
		}
	}
}

// mockFileInfo implements fileutil.FileInfo for test cases.
type mockFileInfo struct {
	name  string
	isDir bool
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 100 }
func (m *mockFileInfo) Mode() os.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() any           { return nil }
