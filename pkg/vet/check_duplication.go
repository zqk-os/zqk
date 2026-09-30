package vet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	defaultDupMinStatements = 5
	defaultDupMinLines      = 8
)

var (
	commentLineRegex = regexp.MustCompile(`//.*$|/\*[\s\S]*?\*/`)
)

// FuncLocation records where a function was declared.
type FuncLocation struct {
	File  string
	Line  int
	Name  string
	Stmts int
	Lines int
}

type dupGroup struct {
	Stmts     int
	Lines     int
	Locations []FuncLocation
}

// StmtItem records a normalized statement and its line metrics.
type StmtItem struct {
	CleanText string
	Lines     int
	Line      int
}

// FuncSequence records a sequence of normalized statements in a function or closure.
type FuncSequence struct {
	File  string
	Name  string
	Line  int
	Stmts []StmtItem
}

// CheckDuplication scans Go source files for identical function bodies,
// enforcing DRY principles across packages and modules.
func CheckDuplication(projectRoot string, files []string, cfg *GatesConfig) ([]Finding, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if len(files) == 0 {
		var err error
		files, err = collectGoFiles(projectRoot, cfg.Hygiene.GoScanDirs)
		if err != nil {
			return nil, err
		}
	}

	minStmts := cfg.Hygiene.DupMinStatements
	if minStmts <= 0 {
		minStmts = defaultDupMinStatements
	}

	minLines := cfg.Hygiene.DupMinLines
	if minLines <= 0 {
		minLines = defaultDupMinLines
	}

	exemptions := append([]string{}, cfg.Hygiene.Exemptions...)
	exemptions = append(exemptions, cfg.Hygiene.DupExemptions...)
	if len(exemptions) == 0 {
		exemptions = []string{"*_test.go", "vendor/*", "*/testdata/*", "*/mock/*"}
	}

	fset := token.NewFileSet()
	bodyHashMap := make(map[string]*dupGroup)
	var allFuncSeqs []FuncSequence
	var driftFindings []Finding

	for _, rel := range files {
		if err := auditFileDuplication(fset, projectRoot, rel, exemptions, minStmts, minLines, bodyHashMap, &allFuncSeqs, &driftFindings); err != nil {
			return nil, err
		}
	}

	findings := collectDuplicationFindings(bodyHashMap)
	seqFindings := collectSequenceDuplicationFindings(allFuncSeqs, minStmts)
	findings = append(findings, seqFindings...)
	findings = append(findings, driftFindings...)
	return findings, nil
}

func auditFileDuplication(fset *token.FileSet, projectRoot, rel string, exemptions []string, minStmts, minLines int, bodyHashMap map[string]*dupGroup, allFuncSeqs *[]FuncSequence, driftFindings *[]Finding) error {
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(rel, "_test.go") ||
		strings.HasPrefix(rel, "vendor/") ||
		strings.Contains(rel, "/testdata/") ||
		strings.HasPrefix(rel, "testdata/") ||
		strings.Contains(rel, "/mock/") ||
		isPathExempt(rel, exemptions) {
		return nil
	}

	abs := rel
	if !filepath.IsAbs(rel) {
		abs = filepath.Join(projectRoot, rel)
	}

	src, err := fileutil.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if isGeneratedCode(src) {
		return nil
	}

	node, err := parser.ParseFile(fset, rel, src, 0)
	if err != nil {
		return nil
	}

	inspectBlock := func(name string, body *ast.BlockStmt, pos token.Pos, end token.Pos) {
		if body == nil {
			return
		}

		startPos := fset.Position(pos)
		endPos := fset.Position(end)

		if allFuncSeqs != nil {
			stmtItems := extractStmtItems(fset, body.List)
			if len(stmtItems) >= 2 {
				*allFuncSeqs = append(*allFuncSeqs, FuncSequence{
					File:  rel,
					Name:  name,
					Line:  startPos.Line,
					Stmts: stmtItems,
				})
			}
		}

		stmtCount := len(body.List)
		if stmtCount < minStmts {
			return
		}

		lineCount := endPos.Line - startPos.Line + 1
		if lineCount < minLines {
			return
		}

		normalized, err := normalizeFuncBody(body)
		if err != nil || normalized == "" {
			return
		}

		hashBytes := sha256.Sum256([]byte(normalized))
		hash := hex.EncodeToString(hashBytes[:])

		group, exists := bodyHashMap[hash]
		if !exists {
			group = &dupGroup{
				Stmts: stmtCount,
				Lines: lineCount,
			}
			bodyHashMap[hash] = group
		}

		group.Locations = append(group.Locations, FuncLocation{
			File:  rel,
			Line:  startPos.Line,
			Name:  name,
			Stmts: stmtCount,
			Lines: lineCount,
		})
	}

	var currentFunc string
	ast.Inspect(node, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			currentFunc = fn.Name.Name
			inspectBlock(fn.Name.Name, fn.Body, fn.Pos(), fn.End())
		case *ast.FuncLit:
			name := fmt.Sprintf("closure at L%d", fset.Position(fn.Pos()).Line)
			if currentFunc != "" {
				name = fmt.Sprintf("closure in %s at L%d", currentFunc, fset.Position(fn.Pos()).Line)
			}
			inspectBlock(name, fn.Body, fn.Pos(), fn.End())
		}
		return true
	})

	if driftFindings != nil {
		drifts := auditMapKeyReferenceDrift(fset, rel, node)
		*driftFindings = append(*driftFindings, drifts...)
	}

	return nil
}

func collectDuplicationFindings(bodyHashMap map[string]*dupGroup) []Finding {
	var findings []Finding
	var hashes []string
	for h := range bodyHashMap {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)

	for _, h := range hashes {
		group := bodyHashMap[h]
		if len(group.Locations) < 2 {
			continue
		}

		primary := group.Locations[0]
		for _, dup := range group.Locations[1:] {
			if dup.File == primary.File && dup.Line == primary.Line {
				continue
			}

			findings = append(findings, Finding{
				CheckID:  "hygiene/duplication",
				Suite:    "hygiene",
				Severity: SeverityWarn,
				File:     dup.File,
				Line:     dup.Line,
				Message: fmt.Sprintf(
					"duplicative function body in %q (%d statements, %d lines) is identical to %s:%d (%s); refactor into a shared abstraction (DRY)",
					dup.Name, dup.Stmts, dup.Lines, primary.File, primary.Line, primary.Name,
				),
			})
		}
	}
	return findings
}

func extractStmtItems(fset *token.FileSet, list []ast.Stmt) []StmtItem {
	var items []StmtItem
	for _, s := range list {
		var buf bytes.Buffer
		if err := format.Node(&buf, token.NewFileSet(), s); err != nil {
			continue
		}
		raw := commentLineRegex.ReplaceAllString(buf.String(), "")
		lines := strings.Split(raw, "\n")
		var cleaned []string
		for _, l := range lines {
			t := strings.TrimSpace(l)
			if t != "" {
				cleaned = append(cleaned, t)
			}
		}
		if len(cleaned) == 0 {
			continue
		}
		startPos := fset.Position(s.Pos())
		endPos := fset.Position(s.End())
		lineCount := endPos.Line - startPos.Line + 1
		if lineCount < 1 {
			lineCount = 1
		}
		items = append(items, StmtItem{
			CleanText: strings.Join(cleaned, " "),
			Lines:     lineCount,
			Line:      startPos.Line,
		})
	}
	return items
}

type kgramOccur struct {
	fnIdx   int
	stmtIdx int
}

func collectSequenceDuplicationFindings(allFuncSeqs []FuncSequence, minStmts int) []Finding {
	minSeqStmts := 3
	if minStmts < minSeqStmts && minStmts > 0 {
		minSeqStmts = minStmts
	}

	kgramMap := make(map[string][]kgramOccur)
	for i, fnSeq := range allFuncSeqs {
		n := len(fnSeq.Stmts)
		for j := 0; j < n; j++ {
			if j+3 <= n {
				kgramKey := fnSeq.Stmts[j].CleanText + "\n" + fnSeq.Stmts[j+1].CleanText + "\n" + fnSeq.Stmts[j+2].CleanText
				kgramMap[kgramKey] = append(kgramMap[kgramKey], kgramOccur{fnIdx: i, stmtIdx: j})
			} else if j+2 <= n && fnSeq.Stmts[j].Lines+fnSeq.Stmts[j+1].Lines >= 5 {
				kgramKey := "2L:" + fnSeq.Stmts[j].CleanText + "\n" + fnSeq.Stmts[j+1].CleanText
				kgramMap[kgramKey] = append(kgramMap[kgramKey], kgramOccur{fnIdx: i, stmtIdx: j})
			}
		}
	}

	type matchCandidate struct {
		file1     string
		line1     int
		name1     string
		file2     string
		line2     int
		name2     string
		stmtCount int
		lineCount int
	}

	var candidates []matchCandidate
	reported := make(map[string]bool)

	for _, occurs := range kgramMap {
		if len(occurs) < 2 {
			continue
		}
		for a := 0; a < len(occurs); a++ {
			for b := a + 1; b < len(occurs); b++ {
				o1 := occurs[a]
				o2 := occurs[b]
				if o1.fnIdx == o2.fnIdx && o1.stmtIdx == o2.stmtIdx {
					continue
				}
				if o1.fnIdx > o2.fnIdx || (o1.fnIdx == o2.fnIdx && o1.stmtIdx > o2.stmtIdx) {
					o1, o2 = o2, o1
				}
				f1 := allFuncSeqs[o1.fnIdx]
				f2 := allFuncSeqs[o2.fnIdx]

				start1 := o1.stmtIdx
				start2 := o2.stmtIdx
				for start1 > 0 && start2 > 0 && f1.Stmts[start1-1].CleanText == f2.Stmts[start2-1].CleanText {
					start1--
					start2--
				}

				end1 := o1.stmtIdx
				end2 := o2.stmtIdx
				for end1 < len(f1.Stmts) && end2 < len(f2.Stmts) && f1.Stmts[end1].CleanText == f2.Stmts[end2].CleanText {
					end1++
					end2++
				}

				seqLen := end1 - start1
				if o1.fnIdx == o2.fnIdx && start1+seqLen > start2 {
					continue
				}

				totalLines := 0
				for k := start2; k < end2; k++ {
					totalLines += f2.Stmts[k].Lines
				}

				if seqLen < minSeqStmts && !(seqLen >= 2 && totalLines >= 5) {
					continue
				}

				if seqLen == len(f1.Stmts) && seqLen == len(f2.Stmts) {
					continue
				}

				repKey := fmt.Sprintf("%s:%d::%s:%d", f1.File, f1.Stmts[start1].Line, f2.File, f2.Stmts[start2].Line)
				if reported[repKey] {
					continue
				}
				reported[repKey] = true

				candidates = append(candidates, matchCandidate{
					file1:     f1.File,
					line1:     f1.Stmts[start1].Line,
					name1:     f1.Name,
					file2:     f2.File,
					line2:     f2.Stmts[start2].Line,
					name2:     f2.Name,
					stmtCount: seqLen,
					lineCount: totalLines,
				})
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].file2 != candidates[j].file2 {
			return candidates[i].file2 < candidates[j].file2
		}
		return candidates[i].line2 < candidates[j].line2
	})

	var findings []Finding
	for _, c := range candidates {
		findings = append(findings, Finding{
			CheckID:  "hygiene/duplication",
			Suite:    "hygiene",
			Severity: SeverityWarn,
			File:     c.file2,
			Line:     c.line2,
			Message: fmt.Sprintf(
				"duplicative statement sequence in %q (%d statements, %d lines) is identical to %s:%d (%s); refactor into a shared abstraction (DRY)",
				c.name2, c.stmtCount, c.lineCount, c.file1, c.line1, c.name1,
			),
		})
	}
	return findings
}

func normalizeFuncBody(body *ast.BlockStmt) (string, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), body); err != nil {
		return "", err
	}

	raw := buf.String()
	raw = commentLineRegex.ReplaceAllString(raw, "")

	lines := strings.Split(raw, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	return strings.Join(cleaned, "\n"), nil
}

func auditMapKeyReferenceDrift(fset *token.FileSet, rel string, file *ast.File) []Finding {
	if file == nil {
		return nil
	}

	constStrings := make(map[string]string)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for j, name := range valSpec.Names {
				if j < len(valSpec.Values) {
					if lit, ok := valSpec.Values[j].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if val, err := strconv.Unquote(lit.Value); err == nil {
							constStrings[name.Name] = val
						}
					}
				}
			}
		}
	}

	type mapInfo struct {
		name string
		keys map[string]bool
	}
	var declaredMaps []mapInfo

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, val := range valSpec.Values {
				comp, ok := val.(*ast.CompositeLit)
				if !ok {
					continue
				}
				isStringKeyMap := false
				if mapType, ok := comp.Type.(*ast.MapType); ok {
					if ident, ok := mapType.Key.(*ast.Ident); ok && ident.Name == "string" {
						isStringKeyMap = true
					}
				} else if valSpec.Type != nil {
					if mapType, ok := valSpec.Type.(*ast.MapType); ok {
						if ident, ok := mapType.Key.(*ast.Ident); ok && ident.Name == "string" {
							isStringKeyMap = true
						}
					}
				}
				if !isStringKeyMap {
					continue
				}

				var name string
				if i < len(valSpec.Names) {
					name = valSpec.Names[i].Name
				}
				if name == "" {
					continue
				}

				keys := make(map[string]bool)
				for _, elt := range comp.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					switch kNode := kv.Key.(type) {
					case *ast.BasicLit:
						if kNode.Kind == token.STRING {
							if k, err := strconv.Unquote(kNode.Value); err == nil && k != "" {
								keys[k] = true
							}
						}
					case *ast.Ident:
						if val, ok := constStrings[kNode.Name]; ok && val != "" {
							keys[val] = true
						}
					}
				}

				if len(keys) >= 3 {
					declaredMaps = append(declaredMaps, mapInfo{
						name: name,
						keys: keys,
					})
				}
			}
		}
	}

	if len(declaredMaps) == 0 {
		return nil
	}

	var findings []Finding

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		for _, m := range declaredMaps {
			indexesMap := false
			matchedEqKeys := make(map[string]bool)
			matchedReturnKeys := make(map[string]bool)

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if n == nil {
					return true
				}
				if id, ok := n.(*ast.Ident); ok && id.Name == m.name {
					indexesMap = true
				}
				if bin, ok := n.(*ast.BinaryExpr); ok {
					if bin.Op == token.EQL || bin.Op == token.NEQ {
						checkExpr := func(e ast.Expr) {
							switch x := e.(type) {
							case *ast.BasicLit:
								if x.Kind == token.STRING {
									if s, err := strconv.Unquote(x.Value); err == nil && m.keys[s] {
										matchedEqKeys[s] = true
									}
								}
							case *ast.Ident:
								if s, ok := constStrings[x.Name]; ok && m.keys[s] {
									matchedEqKeys[s] = true
								}
							}
						}
						checkExpr(bin.X)
						checkExpr(bin.Y)
					}
				}
				if ret, ok := n.(*ast.ReturnStmt); ok {
					for _, res := range ret.Results {
						switch x := res.(type) {
						case *ast.BasicLit:
							if x.Kind == token.STRING {
								if s, err := strconv.Unquote(x.Value); err == nil && m.keys[s] {
									matchedReturnKeys[s] = true
								}
							}
						case *ast.Ident:
							if s, ok := constStrings[x.Name]; ok && m.keys[s] {
								matchedReturnKeys[s] = true
							}
						}
					}
				}
				return true
			})

			if !indexesMap {
				if len(matchedEqKeys) >= 2 || len(matchedReturnKeys) >= 3 {
					var matched []string
					for k := range matchedEqKeys {
						matched = append(matched, k)
					}
					for k := range matchedReturnKeys {
						if !matchedEqKeys[k] {
							matched = append(matched, k)
						}
					}
					sort.Strings(matched)

					findings = append(findings, Finding{
						CheckID:  "hygiene/duplication",
						Suite:    "hygiene",
						File:     rel,
						Line:     fset.Position(fn.Pos()).Line,
						Severity: SeverityWarn,
						Message: fmt.Sprintf("function %s duplicates references to keys of map %s (%s); separate equality branches create drift opportunities, use map lookup or canonical table (DRY)",
							fn.Name.Name, m.name, strings.Join(matched, ", ")),
					})
				}
			}
		}
	}

	return findings
}
