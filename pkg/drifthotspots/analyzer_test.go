package drifthotspots

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestAnalyze_SyntheticKindsAndSystemKeys(t *testing.T) {
	tmp := t.TempDir()
	specs := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specs, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(specs, "bi.yaml"), []byte("ontology: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(tmp, "src")
	if err := fileutil.MkdirAll(srcDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	goSrc := `package p

func f() {
	var x string
	var m map[string]any
	if x == "backlog_item" {
	}
	_ = m["kind"]
}
`
	if err := fileutil.WriteFile(filepath.Join(srcDir, "sample.go"), []byte(goSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	findings, err := Analyze(Options{
		RootDir:  srcDir,
		SpecsDir: specs,
		MinRisk:  "low",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d: %#v", len(findings), findings)
	}
	var sawCompare, sawSysKey bool
	for _, f := range findings {
		switch f.Category {
		case CategoryKindLiteralCompare:
			if f.Literal == "backlog_item" && f.Risk == RiskCritical {
				sawCompare = true
			}
		case CategorySystemFieldKey:
			if f.Literal == "kind" && f.Risk == RiskMedium {
				sawSysKey = true
			}
		}
	}
	if !sawCompare || !sawSysKey {
		t.Fatalf("missing expected categories: compare=%v syskey=%v findings=%#v", sawCompare, sawSysKey, findings)
	}
}

func TestAnalyze_MinRiskFiltersLow(t *testing.T) {
	tmp := t.TempDir()
	specs := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specs, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(specs, "bi.yaml"), []byte("ontology: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(tmp, "src")
	if err := fileutil.MkdirAll(srcDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	goSrc := `package p

func f() {
	_ = map[string]any{"kind": 1}
}
`
	if err := fileutil.WriteFile(filepath.Join(srcDir, "sample.go"), []byte(goSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// Composite literal key "kind" — not a map index on variable; expect no system_field hit from index rule.
	findings, err := Analyze(Options{RootDir: srcDir, SpecsDir: specs, MinRisk: "high"})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings at min-risk high, got %#v", findings)
	}
}

func TestAnalyze_SkipsKindLiteralsInObjectsConstants(t *testing.T) {
	tmp := t.TempDir()
	specs := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specs, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(specs, "bi.yaml"), []byte("ontology: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	objDir := filepath.Join(tmp, "pkg", "objects")
	if err := fileutil.MkdirAll(objDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	constantsGo := `package objects

const KindBacklogItem = "backlog_item"
`
	if err := fileutil.WriteFile(filepath.Join(objDir, "constants.go"), []byte(constantsGo), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	findings, err := Analyze(Options{RootDir: tmp, SpecsDir: specs, MinRisk: "low"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if strings.Contains(f.File, "pkg/objects/constants.go") && f.Category == CategoryKindLiteralAssign {
			t.Fatalf("expected no kind_literal_assign in pkg/objects/constants.go, got %#v", f)
		}
	}
}

func TestAnalyze_SkipsKindLiteralsInKindnamesKinds(t *testing.T) {
	tmp := t.TempDir()
	specs := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specs, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(specs, "bi.yaml"), []byte("ontology: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	knDir := filepath.Join(tmp, "pkg", "kindnames")
	if err := fileutil.MkdirAll(knDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	kindsGo := `package kindnames

const BacklogItem = "backlog_item"
`
	if err := fileutil.WriteFile(filepath.Join(knDir, "kinds.go"), []byte(kindsGo), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	findings, err := Analyze(Options{RootDir: tmp, SpecsDir: specs, MinRisk: "low"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if strings.Contains(f.File, "pkg/kindnames/kinds.go") && f.Category == CategoryKindLiteralAssign {
			t.Fatalf("expected no kind_literal_assign in pkg/kindnames/kinds.go, got %#v", f)
		}
	}
}

func TestAnalyze_SkipsKindAssignInSpecbuilderBuilders(t *testing.T) {
	tmp := t.TempDir()
	specs := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specs, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(specs, "bi.yaml"), []byte("ontology: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	builderDir := filepath.Join(tmp, "pkg", "specbuilder", "bldr_v2")
	if err := fileutil.MkdirAll(builderDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	src := `package bldr_v2

func f() string {
	kind := "backlog_item"
	return kind
}
`
	if err := fileutil.WriteFile(filepath.Join(builderDir, "sample.go"), []byte(src), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	findings, err := Analyze(Options{RootDir: tmp, SpecsDir: specs, MinRisk: "low"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if strings.Contains(filepath.ToSlash(f.File), "/pkg/specbuilder/bldr_v2/sample.go") && f.Category == CategoryKindLiteralAssign {
			t.Fatalf("expected no kind_literal_assign in specbuilder builder path, got %#v", f)
		}
	}
}

func TestAnalyze_ObjectStatusLiteralCompare(t *testing.T) {
	tmp := t.TempDir()
	specs := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specs, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(specs, "bi.yaml"), []byte("ontology: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(tmp, "src")
	if err := fileutil.MkdirAll(srcDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	goSrc := `package p

func f(st string) {
	if st == "completed" {
	}
	if st == "error" {
	}
}
`
	if err := fileutil.WriteFile(filepath.Join(srcDir, "sample.go"), []byte(goSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	findings, err := Analyze(Options{
		RootDir:  srcDir,
		SpecsDir: specs,
		MinRisk:  "low",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d: %#v", len(findings), findings)
	}
	var sawCompleted, sawError bool
	for _, f := range findings {
		if f.Category != CategoryObjectStatusCompare || f.Risk != RiskHigh {
			t.Fatalf("unexpected finding: %#v", f)
		}
		if f.Literal == "completed" {
			sawCompleted = true
		}
		if f.Literal == "error" {
			sawError = true
		}
	}
	if !sawCompleted || !sawError {
		t.Fatalf("missing status findings: completed=%v error=%v findings=%#v", sawCompleted, sawError, findings)
	}
}

func TestAnalyze_RepeatedStringAndMagicNumberLiterals(t *testing.T) {
	tmp := t.TempDir()
	specs := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specs, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(specs, "bi.yaml"), []byte("ontology: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(tmp, "src")
	if err := fileutil.MkdirAll(srcDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	goSrc := `package p

func f() {
	a := "dup_value"
	b := "dup_value"
	_ = a
	_ = b
	_ = 42
	_ = 0x2A
}
`
	if err := fileutil.WriteFile(filepath.Join(srcDir, "sample.go"), []byte(goSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	findings, err := Analyze(Options{
		RootDir:  srcDir,
		SpecsDir: specs,
		MinRisk:  "low",
	})
	if err != nil {
		t.Fatal(err)
	}

	var sawRepeated, sawMagicDecimal, sawMagicHex bool
	for _, f := range findings {
		if f.Category == CategoryRepeatedString && f.Literal == "dup_value" {
			sawRepeated = true
		}
		if f.Category == CategoryMagicNumberLiteral && f.Literal == "42" {
			sawMagicDecimal = true
		}
		if f.Category == CategoryMagicNumberLiteral && f.Literal == "0x2A" {
			sawMagicHex = true
		}
	}
	if !sawRepeated || !sawMagicDecimal || !sawMagicHex {
		t.Fatalf(
			"missing expected findings: repeated=%v decimal=%v hex=%v findings=%#v",
			sawRepeated,
			sawMagicDecimal,
			sawMagicHex,
			findings,
		)
	}
}

func TestAnalyze_RepeatedStringSkipsLoggingKeysAndCosmeticSuffix(t *testing.T) {
	tmp := t.TempDir()
	specs := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specs, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(specs, "bi.yaml"), []byte("ontology: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(tmp, "src")
	if err := fileutil.MkdirAll(srcDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	goSrc := `package p
import "github.com/lanceman/zqk/pkg/logging"
func f() {
	_ = logging.String("object_id", "x")
	_ = logging.String("object_id", "y")
	_ = "..."
	_ = "..."
	_ = ".yaml"
	_ = ".yaml"
	_ = "meaningful_literal"
	_ = "meaningful_literal"
}
`
	if err := fileutil.WriteFile(filepath.Join(srcDir, "sample.go"), []byte(goSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	findings, err := Analyze(Options{RootDir: srcDir, SpecsDir: specs, MinRisk: "low"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if f.Category != CategoryRepeatedString {
			continue
		}
		if f.Literal == "object_id" || f.Literal == "..." || f.Literal == ".yaml" {
			t.Fatalf("unexpected noisy repeated-string finding: %#v", f)
		}
	}
}

func TestAnalyze_RegistryRunMonitorIDLiteral(t *testing.T) {
	tmp := t.TempDir()
	specs := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specs, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(specs, "bi.yaml"), []byte("ontology: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(tmp, "src")
	if err := fileutil.MkdirAll(srcDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	goSrc := `package p

func f(ctx any, root string) {
	healthcheck.DefaultRegistry.Run(ctx, root, "object_volume")
}
`
	if err := fileutil.WriteFile(filepath.Join(srcDir, "sample.go"), []byte(goSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	findings, err := Analyze(Options{RootDir: srcDir, SpecsDir: specs, MinRisk: "low"})
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, f := range findings {
		if f.Category == CategoryRegistryRunMonitorID && f.Literal == "object_volume" {
			saw = true
			break
		}
	}
	if !saw {
		t.Fatalf("expected %s finding for object_volume, got %#v", CategoryRegistryRunMonitorID, findings)
	}
}

func TestAnalyze_ReportedLiteralPatterns(t *testing.T) {
	tmp := t.TempDir()
	specs := filepath.Join(tmp, "specs")
	if err := fileutil.MkdirAll(specs, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(specs, "bi.yaml"), []byte("ontology: backlog_item\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(tmp, "src")
	if err := fileutil.MkdirAll(srcDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	goSrc := `package p

import (
	"os/exec"
	"path/filepath"
)

type flagsAPI struct{}
func (f *flagsAPI) BoolVar(dst *bool, name string, value bool, usage string) {}
type cmd struct{}
func (c *cmd) Flags() *flagsAPI { return &flagsAPI{} }

func f(projectRoot string) {
	var b bool
	c := &cmd{}
	c.Flags().BoolVar(&b, "pull", false, "Pull changes from remote")
	_ = exec.Command("git", "status", "--short")
	_ = filepath.Join(projectRoot, paths.ProjectDataDir, "system-health", "tier1-latest.json")
	_ = struct{
		Filters map[string]any
		SortBy string
	}{Filters: map[string]any{"status":"active"}, SortBy: "active_order"}
}
`
	if err := fileutil.WriteFile(filepath.Join(srcDir, "sample.go"), []byte(goSrc), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	findings, err := Analyze(Options{RootDir: srcDir, SpecsDir: specs, MinRisk: "low"})
	if err != nil {
		t.Fatal(err)
	}
	need := map[string]bool{
		CategoryExecCommandLiteral:  false,
		CategoryCobraFlagLiteral:    false,
		CategoryFilepathJoinSegment: false,
		CategoryListFilterSortBy:    false,
	}
	for _, f := range findings {
		if _, ok := need[f.Category]; ok {
			need[f.Category] = true
		}
	}
	for cat, ok := range need {
		if !ok {
			t.Fatalf("expected category %s in findings, got %#v", cat, findings)
		}
	}
}
