package drifthotspots

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/astutil"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/kindnames"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// Risk levels for drift-prone patterns (highest first).
const (
	RiskCritical = "critical"
	RiskHigh     = "high"
	RiskMedium   = "medium"
	RiskLow      = "low"
)

// Category describes the kind of hotspot.
const (
	CategoryKindLiteralCompare   = "kind_literal_compare"
	CategoryKindLiteralAssign    = "kind_literal_assign"
	CategorySystemFieldKey       = "system_field_map_key"
	CategorySchemaVersionLiteral = "schema_version_literal"
	CategoryObjectStatusCompare  = "object_status_literal_compare"
	CategoryRepeatedString       = "repeated_string_literal"
	CategoryMagicNumberLiteral   = "magic_number_literal"
	CategoryRegistryRunMonitorID = "registry_run_monitor_id_literal"
	CategoryExecCommandLiteral   = "exec_command_literal"
	CategoryCobraFlagLiteral     = "cobra_flag_literal"
	CategoryListFilterStatus     = "list_filter_status_literal"
	CategoryListFilterSortBy     = "list_filter_sortby_literal"
	CategoryFilepathJoinSegment  = "filepath_join_segment_literal"
)

// Finding is one analyzer hit.
type Finding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Risk     string `json:"risk"`
	Category string `json:"category"`
	Literal  string `json:"literal"`
	Hint     string `json:"hint"`
}

// FileSummary aggregates findings for one file (sorted by count in BuildReport).
type FileSummary struct {
	File       string         `json:"file"`
	Count      int            `json:"count"`
	ByCategory map[string]int `json:"by_category"`
	ByRisk     map[string]int `json:"by_risk"`
}

// Report aggregates scan output for CLI/JSON.
type Report struct {
	RootDir                string         `json:"root_dir"`
	SpecsDir               string         `json:"specs_dir"`
	IncludeTests           bool           `json:"include_tests"`
	MinRiskFloor           string         `json:"min_risk_floor"`
	IncludeConstantBarrels bool           `json:"include_constant_barrels"`
	Findings               []Finding      `json:"findings"`
	CountByRisk            map[string]int `json:"count_by_risk"`
	CountByCategory        map[string]int `json:"count_by_category"`
	FileSummaries          []FileSummary  `json:"file_summaries"`
}

// Options configures the analyzer.
type Options struct {
	RootDir      string
	SpecsDir     string
	IncludeTests bool
	MinRisk      string // critical | high | medium | low (inclusive floor); empty = low
	ExtraKindSet map[string]struct{}
	// IncludeConstantBarrels when true, also scan lock_op_names.go and pkg/specbuilder/bldr_v2/*_constants.go
	// (default false: those files centralize literals by design).
	IncludeConstantBarrels bool
}

// loadKindSetForAnalyze returns the same map shape as kindnames.LoadKindNamesFromSpecsDir.
// When RootDir uses the default object_specs layout and a spec index is materialized, prefer
// objects.KindNamesFromSpecIndex so the kind set matches spec_index.json (spec origin plane).
func loadKindSetForAnalyze(opts Options) (map[string]struct{}, error) {
	defaultSpecs := filepath.Join(opts.RootDir, paths.ProcessInternalObjectSpecsDir)
	if filepath.Clean(opts.SpecsDir) == filepath.Clean(defaultSpecs) {
		if idx := objects.TryLoadSpecIndexForProjectRoot(opts.RootDir); idx != nil {
			if m, err := objects.KindNamesFromSpecIndex(idx); err == nil && len(m) > 0 {
				return m, nil
			}
		}
	}
	return kindnames.LoadKindNamesFromSpecsDir(opts.SpecsDir)
}

// Analyze walks RootDir for *.go (excluding vendor), parses each file, and reports drift hotspots.
func Analyze(opts Options) ([]Finding, error) {
	if opts.RootDir == emptyValue {
		return nil, errfmt.Errorf("root directory is empty")
	}
	kindSet, err := loadKindSetForAnalyze(opts)
	if err != nil {
		return nil, err
	}
	for k := range opts.ExtraKindSet {
		kindSet[k] = struct{}{}
	}

	minRank := minRiskFloorRank(opts.MinRisk)
	var out []Finding

	err = filepath.Walk(opts.RootDir, func(path string, info fileutil.FileInfo, walkErr error) error {
		return analyzeFile(path, info, walkErr, opts, minRank, kindSet, &out)
	})

	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Column < out[j].Column
	})

	return out, nil
}

type stringOccur struct {
	val string
	pos token.Position
}

func analyzeFile(path string, info fileutil.FileInfo, walkErr error, opts Options, minRank int, kindSet map[string]struct{}, out *[]Finding) error {
	if walkErr != nil {
		return walkErr
	}
	if info.IsDir() {
		base := info.Name()
		if base == "vendor" || base == ".git" {
			return filepath.SkipDir
		}
		return nil
	}
	if !strings.HasSuffix(path, ".go") {
		return nil
	}
	if !opts.IncludeTests && strings.HasSuffix(path, "_test.go") {
		return nil
	}
	if isGeneratedGoFile(path) {
		return nil
	}
	if !opts.IncludeConstantBarrels && IsConstantBarrelPath(path) {
		return nil
	}

	fset := token.NewFileSet()
	src, err := fileutil.ReadFile(path)
	if err != nil {
		return err
	}
	if isGeneratedSource(src) {
		return nil
	}

	parsed, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil // skip unparseable (e.g. build tags)
	}

	isTest := strings.HasSuffix(path, "_test.go")
	constantsGo := strings.HasSuffix(path, filepath.Join("pkg", "objects", "constants.go")) ||
		strings.HasSuffix(path, "pkg/objects/constants.go")
	kindnamesGo := strings.HasSuffix(path, filepath.Join("pkg", "kindnames", "kinds.go")) ||
		strings.HasSuffix(path, "pkg/kindnames/kinds.go")

	var repeatedStringCandidates []stringOccur
	var stringCounts = map[string]int{}

	inspectAST(parsed, fset, path, isTest, constantsGo, kindnamesGo, minRank, kindSet, out, &repeatedStringCandidates, stringCounts)

	for _, occ := range repeatedStringCandidates {
		if stringCounts[occ.val] < 2 {
			continue
		}
		f := Finding{
			File: path, Line: occ.pos.Line, Column: occ.pos.Column,
			Risk: RiskLow, Category: CategoryRepeatedString, Literal: occ.val,
			Hint: "repeated quoted literal in file; consider centralizing into a constant when it carries domain meaning",
		}
		if riskRank(f.Risk) >= minRank {
			*out = append(*out, f)
		}
	}
	return nil
}

func inspectAST(parsed *ast.File, fset *token.FileSet, path string, isTest, constantsGo, kindnamesGo bool, minRank int, kindSet map[string]struct{}, out *[]Finding, repeatedStringCandidates *[]stringOccur, stringCounts map[string]int) {
	ast.Inspect(parsed, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		bl, ok := n.(*ast.BasicLit)
		if !ok {
			return true
		}
		// Path is innermost-first: path[0] is the smallest enclosing node, path[len-1] is *ast.File.
		pathNodes, ok := astutil.PathEnclosingInterval(parsed, bl.Pos(), bl.End())
		if !ok || len(pathNodes) < 2 {
			return true
		}
		if hasImportAncestor(pathNodes) {
			return true
		}
		pos := fset.Position(bl.Pos())

		if bl.Kind == token.INT || bl.Kind == token.FLOAT || bl.Kind == token.IMAG {
			if isMagicNumberLiteral(pathNodes, bl.Value) {
				risk := RiskLow
				if isTest {
					risk = RiskLow
				}
				f := Finding{
					File: path, Line: pos.Line, Column: pos.Column,
					Risk: risk, Category: CategoryMagicNumberLiteral, Literal: bl.Value,
					Hint: "consider named constants for repeated or domain-significant numeric literals",
				}
				if riskRank(f.Risk) >= minRank {
					*out = append(*out, f)
				}
			}
			return true
		}
		if bl.Kind != token.STRING {
			return true
		}
		val, err := strconv.Unquote(bl.Value)
		if err != nil {
			return true
		}

		// Schema version literals (prefer objects.DefaultSchemaVersion); compare via constant to avoid a probe literal here.
		if val == objects.DefaultSchemaVersion && !constantsGo {
			f := Finding{
				File: path, Line: pos.Line, Column: pos.Column,
				Risk:     RiskHigh,
				Category: CategorySchemaVersionLiteral,
				Literal:  val,
				Hint:     "prefer objects.DefaultSchemaVersion or a single spec-backed constant",
			}
			if isTest {
				f.Risk = RiskMedium
			}
			if riskRank(f.Risk) >= minRank {
				*out = append(*out, f)
			}
			return true
		}

		compareOrCase := kindLiteralInCompareOrCase(pathNodes)

		// Object status literals in compare/case tend to drift from centralized constants.
		// Catch both known status values and custom per-kind statuses when the comparison
		// clearly involves a status-like field/expression.
		if isStatusLiteralInStatusCompare(pathNodes, bl, val) && !constantsGo {
			risk := RiskHigh
			if isTest {
				risk = RiskMedium
			}
			f := Finding{
				File: path, Line: pos.Line, Column: pos.Column,
				Risk: risk, Category: CategoryObjectStatusCompare, Literal: val,
				Hint: "prefer pkg/objects ObjectStatus* constants for lifecycle status comparisons",
			}
			if riskRank(f.Risk) >= minRank {
				*out = append(*out, f)
			}
			return true
		}

		// Kind literals from spec ontology: highest drift when used in compare/switch case.
		if _, known := kindSet[val]; known {
			// Canonical definitions live in pkg/kindnames/kinds.go and pkg/objects/constants.go; do not flag those barrels.
			if constantsGo || kindnamesGo {
				return true
			}
			// Data-catalog files intentionally hold large mapping/fixture tables.
			// Keep compare/switch signals, but skip assignment-only noise there.
			if !compareOrCase && IsDataCatalogPath(path) {
				return true
			}
			// Specbuilder builder trees intentionally carry kind literals from spec/codegen shape.
			// Keep compare/switch signals, but skip assignment-only noise in these directories.
			if !compareOrCase && isSpecbuilderBuilderPath(path) {
				return true
			}
			cat := CategoryKindLiteralAssign
			risk := RiskMedium
			if compareOrCase {
				cat = CategoryKindLiteralCompare
				risk = RiskCritical
				if isTest {
					risk = RiskHigh
				}
			} else if isTest {
				risk = RiskLow
			}

			f := Finding{
				File: path, Line: pos.Line, Column: pos.Column,
				Risk: risk, Category: cat, Literal: val,
				Hint: "prefer kind from registry/spec cache or pkg/objects Kind* constant where generated",
			}
			if riskRank(f.Risk) >= minRank {
				*out = append(*out, f)
			}
			return true
		}

		// System field keys as map indices — spelling must match spec.
		if _, isSysField := SystemObjectFieldKeys[val]; isSysField && isMapIndexKey(pathNodes, bl) {
			risk := RiskMedium
			if isTest {
				risk = RiskLow
			}
			f := Finding{
				File: path, Line: pos.Line, Column: pos.Column,
				Risk: risk, Category: CategorySystemFieldKey, Literal: val,
				Hint: "consider a shared constant for object field keys if repeated; keys must match spec",
			}
			if riskRank(f.Risk) >= minRank {
				*out = append(*out, f)
			}
		}

		// Health/registry monitor IDs passed to .Run(..., ..., "monitor_id") should be constants.
		if isRegistryRunMonitorIDLiteral(pathNodes, bl) {
			f := Finding{
				File: path, Line: pos.Line, Column: pos.Column,
				Risk: RiskLow, Category: CategoryRegistryRunMonitorID, Literal: val,
				Hint: "prefer a named monitor-id constant instead of inline string in registry .Run call",
			}
			if riskRank(f.Risk) >= minRank {
				*out = append(*out, f)
			}
			return true
		}
		if isExecCommandLiteral(pathNodes, bl) {
			f := Finding{
				File: path, Line: pos.Line, Column: pos.Column,
				Risk: RiskLow, Category: CategoryExecCommandLiteral, Literal: val,
				Hint: "prefer command/arg constants for exec.Command/CommandContext literals in reusable command paths",
			}
			if riskRank(f.Risk) >= minRank {
				*out = append(*out, f)
			}
			return true
		}
		if isCobraFlagLiteral(pathNodes, bl) {
			f := Finding{
				File: path, Line: pos.Line, Column: pos.Column,
				Risk: RiskLow, Category: CategoryCobraFlagLiteral, Literal: val,
				Hint: "prefer constants for CLI flag names/help strings when repeated or policy-sensitive",
			}
			if riskRank(f.Risk) >= minRank {
				*out = append(*out, f)
			}
			return true
		}
		if isListFilterStatusLiteral(pathNodes, bl) {
			f := Finding{
				File: path, Line: pos.Line, Column: pos.Column,
				Risk: RiskLow, Category: CategoryListFilterStatus, Literal: val,
				Hint: "prefer named status constants in storage.ListFilter maps",
			}
			if riskRank(f.Risk) >= minRank {
				*out = append(*out, f)
			}
			return true
		}
		if isListFilterSortByLiteral(pathNodes, bl) {
			f := Finding{
				File: path, Line: pos.Line, Column: pos.Column,
				Risk: RiskLow, Category: CategoryListFilterSortBy, Literal: val,
				Hint: "prefer named constants for storage.ListFilter SortBy fields",
			}
			if riskRank(f.Risk) >= minRank {
				*out = append(*out, f)
			}
			return true
		}
		if isFilepathJoinSegmentLiteral(pathNodes, bl) {
			f := Finding{
				File: path, Line: pos.Line, Column: pos.Column,
				Risk: RiskLow, Category: CategoryFilepathJoinSegment, Literal: val,
				Hint: "prefer paths package constants or local consts for filepath.Join path segments",
			}
			if riskRank(f.Risk) >= minRank {
				*out = append(*out, f)
			}
			return true
		}

		if shouldTrackRepeatedStringLiteral(pathNodes, val) {
			*repeatedStringCandidates = append(*repeatedStringCandidates, stringOccur{val: val, pos: pos})
			stringCounts[val]++
		}

		return true
	})
}

// BuildReport wraps findings with summary counts and per-file rollups.
func BuildReport(opts Options, findings []Finding) Report {
	r := Report{
		RootDir:                opts.RootDir,
		SpecsDir:               opts.SpecsDir,
		IncludeTests:           opts.IncludeTests,
		MinRiskFloor:           opts.MinRisk,
		IncludeConstantBarrels: opts.IncludeConstantBarrels,
		Findings:               findings,
		CountByRisk:            map[string]int{},
		CountByCategory:        map[string]int{},
	}
	if r.MinRiskFloor == emptyValue {
		r.MinRiskFloor = RiskLow
	}
	for _, f := range findings {
		r.CountByRisk[f.Risk]++
		r.CountByCategory[f.Category]++
	}
	byFile := map[string]*FileSummary{}
	for _, f := range findings {
		s := byFile[f.File]
		if s == nil {
			s = &FileSummary{File: f.File, ByCategory: map[string]int{}, ByRisk: map[string]int{}}
			byFile[f.File] = s
		}
		s.Count++
		s.ByCategory[f.Category]++
		s.ByRisk[f.Risk]++
	}
	r.FileSummaries = make([]FileSummary, 0, len(byFile))
	for _, s := range byFile {
		r.FileSummaries = append(r.FileSummaries, *s)
	}
	sort.Slice(r.FileSummaries, func(i, j int) bool {
		if r.FileSummaries[i].Count != r.FileSummaries[j].Count {
			return r.FileSummaries[i].Count > r.FileSummaries[j].Count
		}
		return r.FileSummaries[i].File < r.FileSummaries[j].File
	})
	return r
}

func riskRank(r string) int {
	switch r {
	case RiskCritical:
		return 4
	case RiskHigh:
		return 3
	case RiskMedium:
		return 2
	case RiskLow:
		return 1
	default:
		return 0
	}
}

func minRiskFloorRank(s string) int {
	if s == emptyValue {
		return 1
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 1
	}
}

func hasImportAncestor(path []ast.Node) bool {
	for _, n := range path {
		if _, ok := n.(*ast.ImportSpec); ok {
			return true
		}
	}
	return false
}

func kindLiteralInCompareOrCase(path []ast.Node) bool {
	for _, n := range path {
		switch x := n.(type) {
		case *ast.BinaryExpr:
			if x.Op == token.EQL || x.Op == token.NEQ {
				return true
			}
		case *ast.CaseClause:
			return true
		}
	}
	return false
}

func isSpecbuilderBuilderPath(path string) bool {
	unix := filepath.ToSlash(path)
	return strings.Contains(unix, "/pkg/specbuilder/bldr_v2/") ||
		strings.Contains(unix, "/pkg/specbuilder/bldr_lifecycle_v1/") ||
		strings.Contains(unix, "/pkg/specbuilder/bldr_profile_v1/")
}

func isStatusLiteralInStatusCompare(path []ast.Node, lit *ast.BasicLit, val string) bool {
	if strings.TrimSpace(val) == emptyValue {
		return false
	}
	known := val == objects.ObjectStatusArchived || val == objects.ObjectStatusCompleted || val == objects.ObjectStatusAggregated
	if known && kindLiteralInCompareOrCase(path) {
		return true
	}
	for _, n := range path {
		be, ok := n.(*ast.BinaryExpr)
		if !ok || (be.Op != token.EQL && be.Op != token.NEQ) {
			continue
		}
		if be.X == lit && exprHasStatusSignal(be.Y) {
			return true
		}
		if be.Y == lit && exprHasStatusSignal(be.X) {
			return true
		}
	}
	for i, n := range path {
		cc, ok := n.(*ast.CaseClause)
		if !ok {
			continue
		}
		containsLit := false
		for _, e := range cc.List {
			if e == lit {
				containsLit = true
				break
			}
		}
		if !containsLit {
			continue
		}
		for j := i + 1; j < len(path); j++ {
			if sw, ok := path[j].(*ast.SwitchStmt); ok && exprHasStatusSignal(sw.Tag) {
				return true
			}
		}
	}
	return false
}

func exprHasStatusSignal(e ast.Expr) bool {
	if e == nil {
		return false
	}
	switch x := e.(type) {
	case *ast.Ident:
		n := strings.ToLower(x.Name)
		return strings.Contains(n, "status") || n == "st"
	case *ast.SelectorExpr:
		if strings.Contains(strings.ToLower(x.Sel.Name), "status") {
			return true
		}
		return exprHasStatusSignal(x.X)
	case *ast.IndexExpr:
		if bl, ok := x.Index.(*ast.BasicLit); ok && bl.Kind == token.STRING {
			if v, err := strconv.Unquote(bl.Value); err == nil && strings.Contains(strings.ToLower(v), "status") {
				return true
			}
		}
		return exprHasStatusSignal(x.X)
	case *ast.CallExpr:
		return exprHasStatusSignal(x.Fun)
	default:
		return false
	}
}

func isMapIndexKey(path []ast.Node, lit *ast.BasicLit) bool {
	if len(path) < 2 || path[0] != lit {
		return false
	}
	parent := path[1]
	ix, ok := parent.(*ast.IndexExpr)
	if !ok {
		return false
	}
	return ix.Index == lit
}

func isRegistryRunMonitorIDLiteral(path []ast.Node, lit *ast.BasicLit) bool {
	if len(path) < 2 || path[0] != lit {
		return false
	}
	call, ok := path[1].(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "Run" {
		return false
	}
	if len(call.Args) < 3 || call.Args[2] != lit {
		return false
	}
	return true
}

func isExecCommandLiteral(path []ast.Node, lit *ast.BasicLit) bool {
	if len(path) < 2 || path[0] != lit {
		return false
	}
	call, ok := path[1].(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return false
	}
	if sel.Sel.Name != "Command" && sel.Sel.Name != "CommandContext" {
		return false
	}
	xid, ok := sel.X.(*ast.Ident)
	if !ok || xid.Name != "exec" {
		return false
	}
	for _, a := range call.Args {
		if a == lit {
			return true
		}
	}
	return false
}

func isCobraFlagLiteral(path []ast.Node, lit *ast.BasicLit) bool {
	if len(path) < 2 || path[0] != lit {
		return false
	}
	call, ok := path[1].(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return false
	}
	switch sel.Sel.Name {
	case "BoolVar", "BoolVarP", "StringVar", "StringVarP", "IntVar", "IntVarP":
		for _, a := range call.Args {
			if a == lit {
				return true
			}
		}
	}
	return false
}

func isListFilterStatusLiteral(path []ast.Node, lit *ast.BasicLit) bool {
	for i, n := range path {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok || kv.Value != lit {
			continue
		}
		// value of map entry under Filters map
		if i+1 < len(path) {
			if _, ok := path[i+1].(*ast.CompositeLit); ok {
				for j := i + 2; j < len(path); j++ {
					parentKV, ok := path[j].(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if keyID, ok := parentKV.Key.(*ast.Ident); ok && keyID.Name == "Filters" {
						if entryKey, ok := kv.Key.(*ast.SelectorExpr); ok {
							if xid, ok := entryKey.X.(*ast.Ident); ok && xid.Name == "objects" && entryKey.Sel != nil && entryKey.Sel.Name == "FieldKeyStatus" {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

func isListFilterSortByLiteral(path []ast.Node, lit *ast.BasicLit) bool {
	for _, n := range path {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok || kv.Value != lit {
			continue
		}
		if keyID, ok := kv.Key.(*ast.Ident); ok && keyID.Name == "SortBy" {
			return true
		}
	}
	return false
}

func isFilepathJoinSegmentLiteral(path []ast.Node, lit *ast.BasicLit) bool {
	if len(path) < 2 || path[0] != lit {
		return false
	}
	call, ok := path[1].(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "Join" {
		return false
	}
	xid, ok := sel.X.(*ast.Ident)
	if !ok || xid.Name != "filepath" {
		return false
	}
	// only flag non-first segments (first arg often projectRoot variable)
	for idx, a := range call.Args {
		if a == lit {
			return idx > 0
		}
	}
	return false
}

func hasConstAncestor(path []ast.Node) bool {
	for _, n := range path {
		if gd, ok := n.(*ast.GenDecl); ok && gd.Tok == token.CONST {
			return true
		}
	}
	return false
}

func shouldTrackRepeatedStringLiteral(path []ast.Node, val string) bool {
	v := strings.TrimSpace(val)
	if v == emptyValue || len(v) < 3 {
		return false
	}
	// Repeated logger field keys and cosmetic suffixes are noisy and low signal.
	if v == "..." || v == ".yaml" || isLoggingFieldKeyLiteral(path, v) {
		return false
	}
	// Skip struct tags and import strings; imports already skipped via hasImportAncestor.
	for _, n := range path {
		if _, ok := n.(*ast.Field); ok {
			// Struct tags are encoded as string BasicLits on fields.
			if strings.Contains(v, ":") && strings.Contains(v, "\"") {
				return false
			}
		}
	}
	return true
}

func isLoggingFieldKeyLiteral(path []ast.Node, v string) bool {
	if len(path) < 2 {
		return false
	}
	call, ok := path[1].(*ast.CallExpr)
	if !ok {
		return false
	}
	for idx, arg := range call.Args {
		if arg != path[0] {
			continue
		}
		if idx != 0 {
			return false
		}
		break
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return false
	}
	if sel.Sel.Name != "String" && sel.Sel.Name != "Any" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	if ident.Name != "logging" {
		return false
	}
	return strings.Contains(v, "_")
}

func isMagicNumberLiteral(path []ast.Node, literalText string) bool {
	if hasConstAncestor(path) {
		return false
	}
	v := strings.TrimSpace(strings.TrimSuffix(literalText, "i"))
	// Ignore most-common sentinels to reduce noise.
	switch v {
	case "0", "1", "-1":
		return false
	}
	return true
}

func isGeneratedGoFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, "zz_generated") || strings.HasPrefix(base, "generated_")
}

func isGeneratedSource(src []byte) bool {
	head := src
	if len(head) > 2048 {
		head = head[:2048]
	}
	s := string(head)
	return strings.Contains(s, "Code generated") || strings.Contains(s, "DO NOT EDIT")
}
