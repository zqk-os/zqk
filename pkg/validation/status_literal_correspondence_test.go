package validation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Correspondence test: lifecycle vocabulary versus the status values code and fixtures actually
// write. A lifecycle defines which statuses a kind can hold; thousands of composite literals in
// this tree pair a kind with a status, and nothing checked that the pair is legal. The result was
// a steady supply of fixtures asserting statuses that do not exist - criteria in "not_started",
// requirement in "exploring" and "validated" - each of which fails at the storage boundary with a
// lifecycle error that reads like a bug in the code under test rather than a wrong fixture.
//
// This scans for literals that name both a kind and a status, resolves each through the same
// constant tables the code uses, and validates the pair with the loader production rejects with.
//
// Coverage boundary, stated so the counts below are not read as more than they are: a literal has to
// carry its own kind to be checkable here. Status-only update maps — the shape
// `Update(ctx, sec, id, map[string]any{FieldKeyStatus: x})`, where the kind comes from the id at
// runtime — are invisible to this scan, and two such sites in pkg/scheduler/convergence_engine.go
// were caught by the raw-status-literal lint instead, not by this test. Checking those needs the id
// traced to a kind, which is dataflow this does not attempt.

// constStrings maps identifier name to string value for constants whose name starts with prefix.
//
// known lets a value that is itself a constant reference resolve, which is required rather than
// optional here: objects.KindMilestone is `= kindnames.Milestone`, not a string literal, so a
// resolver that only accepted literals silently dropped every site written with the kind
// constants — that is, all the correctly-written ones — and reported a clean scan over the
// remainder.
func constStrings(t *testing.T, dir, prefix string, known map[string]string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			continue
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) || !strings.HasPrefix(name.Name, prefix) {
						continue
					}
					if v, ok := resolveStringExpr(vs.Values[i], known); ok {
						out[name.Name] = v
					}
				}
			}
		}
	}
	return out
}

// statusLiteralSite is one literal that names both a kind and a status.
type statusLiteralSite struct {
	file   string
	line   int
	kind   string
	status string
}

// resolveStringExpr returns the string an expression denotes, using the constant tables for
// identifiers. ok=false means the value is dynamic (a variable, a call, a field) and therefore
// not checkable here.
func resolveStringExpr(e ast.Expr, consts map[string]string) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		return s, err == nil
	case *ast.Ident:
		s, ok := consts[v.Name]
		return s, ok
	case *ast.SelectorExpr:
		s, ok := consts[v.Sel.Name]
		return s, ok
	default:
		return "", false
	}
}

// isKeyFor reports whether a map key expression names the given field, accepting the FieldKey
// constant (qualified or not) and the raw wire string.
func isKeyFor(e ast.Expr, fieldConst, wire string) bool {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return false
		}
		s, err := strconv.Unquote(v.Value)
		return err == nil && s == wire
	case *ast.Ident:
		return v.Name == fieldConst
	case *ast.SelectorExpr:
		return v.Sel.Name == fieldConst
	}
	return false
}

// collectStatusLiteralSites walks Go sources under root for composite literals that set both a
// kind and a status.
func collectStatusLiteralSites(t *testing.T, root string, consts map[string]string) []statusLiteralSite {
	t.Helper()
	var sites []statusLiteralSite
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d fileutil.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "docs", ".zqk", "integration", "experiments":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(f, func(n ast.Node) bool {
			cl, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			var kind, status string
			var haveKind, haveStatus bool
			var pos token.Pos
			for _, elt := range cl.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				switch {
				case isKeyFor(kv.Key, "FieldKeyKind", objects.FieldKeyKind):
					if v, ok := resolveStringExpr(kv.Value, consts); ok {
						kind, haveKind = v, true
					}
				case isKeyFor(kv.Key, "FieldKeyStatus", objects.FieldKeyStatus):
					if v, ok := resolveStringExpr(kv.Value, consts); ok {
						status, haveStatus, pos = v, true, kv.Pos()
					}
				}
			}
			if haveKind && haveStatus {
				sites = append(sites, statusLiteralSite{
					file:   rel,
					line:   fset.Position(pos).Line,
					kind:   kind,
					status: status,
				})
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return sites
}

// yamlTopLevelValues returns the value of the given top-level key from every YAML file in dir.
//
// Lifecycles name their kind with object_type and object specs with ontology, so one reader serves
// both planes.
func yamlTopLevelValues(t *testing.T, dir, key string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	prefix := key + ":"
	err := filepath.WalkDir(dir, func(path string, d fileutil.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		raw, err := fileutil.ReadFile(path)
		if err != nil {
			return nil
		}
		for line := range strings.SplitSeq(string(raw), "\n") {
			if rest, ok := strings.CutPrefix(line, prefix); ok {
				if k := strings.TrimSpace(rest); k != "" {
					out[k] = true
				}
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	if len(out) == 0 {
		t.Fatalf("no %s values found under %s", key, dir)
	}
	return out
}

// checkableKinds returns the kinds whose statuses this scan may judge: every kind in the object
// spec registry, plus any kind a lifecycle declares directly.
//
// The membership test is the spec registry rather than "has its own lifecycle file", which is what
// this used and which was wrong in a way that hid real defects. 64 of the 127 kinds have no
// lifecycle file of their own; they resolve one through extends, exactly as production does when
// LoadLifecycle answers for them. Excluding them put half the ontology outside the scan, and the
// defects it was built to find were sitting in that half: watchdog_registration,
// scheduler_handler_binding, and remote_kernel all inherit base_object, whose ladder
// (proposed/approved/in_progress/implemented/archived/error) has no "active" — so every site
// writing them "active" was unreported here and failed at the storage boundary instead.
//
// The registry still excludes what the old filter was really defending against. LoadLifecycle
// falls back to a base lifecycle for any unknown name, so it happily judged ad-hoc test kinds and
// even the string "FieldKeyKind"; neither has a spec file, so neither is checkable now either.
func checkableKinds(t *testing.T, specsDir, lifecyclesDir string) map[string]bool {
	t.Helper()
	out := yamlTopLevelValues(t, specsDir, "ontology")
	for k := range yamlTopLevelValues(t, lifecyclesDir, "object_type") {
		out[k] = true
	}
	return out
}

// TestCheckableKinds_includesKindsThatInheritTheirLifecycle refuses a narrowing of the scan's
// membership test back to "declares its own lifecycle file".
//
// That narrowing is not hypothetical: it was the original implementation, added to stop
// LoadLifecycle's unknown-kind fallback from judging ad-hoc names, and it silently put 64 of 127
// kinds out of scope. The scan reported a clean result over the remainder while a dead barrier and
// two production creates sat in the excluded half. A guard that only checks the total kind count
// would not catch it either, since the count stays plausible — so this asserts the specific
// property, that a kind resolving its lifecycle through extends is still checkable.
func TestCheckableKinds_includesKindsThatInheritTheirLifecycle(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	lifecyclesDir := filepath.Join(root, paths.ProcessInternalLifecyclesDir)
	checkable := checkableKinds(t, filepath.Join(root, paths.ProcessInternalObjectSpecsDir), lifecyclesDir)
	ownLifecycle := yamlTopLevelValues(t, lifecyclesDir, "object_type")

	inherited := 0
	for kind := range checkable {
		if !ownLifecycle[kind] {
			inherited++
		}
	}
	if inherited == 0 {
		t.Error("no checkable kind inherits its lifecycle — membership has been narrowed back to " +
			"kinds with their own lifecycle file, which removes about half the ontology from the scan")
	}

	// Named kinds, so the property cannot be satisfied by an unrelated remainder. Each of these has
	// no lifecycle file and reached the storage boundary with an unknown-status error before the
	// scan could see it.
	for _, kind := range []string{"watchdog_registration", "scheduler_handler_binding", "remote_kernel"} {
		if ownLifecycle[kind] {
			continue // gained its own lifecycle; no longer an inheritance case
		}
		if !checkable[kind] {
			t.Errorf("%s inherits its lifecycle and is not checkable; the status literals written "+
				"for it would go unreported", kind)
		}
	}

	// The fallback the old filter defended against must still be excluded, or widening membership
	// just trades false negatives for false positives.
	for _, notAKind := range []string{"test_object", "FieldKeyKind"} {
		if checkable[notAKind] {
			t.Errorf("%q is checkable but is not a kind; LoadLifecycle's fallback would judge its "+
				"statuses against a vocabulary that was never its own", notAKind)
		}
	}
}

func TestStatusLiterals_matchTheLifecycleOfTheKindTheyAreWrittenWith(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	objectsDir := filepath.Join(root, "pkg", "objects")
	kindNamesDir := filepath.Join(root, "pkg", "kindnames")

	// kindnames first: it holds the string literals that pkg/objects' Kind* constants alias.
	consts := constStrings(t, kindNamesDir, "", nil)
	for _, prefix := range []string{"ObjectStatus", "Kind"} {
		for name, v := range constStrings(t, objectsDir, prefix, consts) {
			consts[name] = v
		}
	}
	// Guard the alias chain explicitly. Without this, a future refactor that moves kind constants
	// again would shrink the scan back to string-literal sites and still report success.
	for _, required := range []string{"KindMilestone", "KindBacklogItem", "KindCriteria", "ObjectStatusComplete"} {
		if consts[required] == "" {
			t.Fatalf("%s did not resolve to a value; the constant tables are incomplete and the "+
				"scan would silently skip every site written with it", required)
		}
	}

	loader := objects.NewLifecycleLoader(filepath.Join(root, paths.ProcessInternalLifecyclesDir))
	sites := collectStatusLiteralSites(t, root, consts)
	if len(sites) == 0 {
		t.Fatal("no kind+status literals found; this guard would be vacuous")
	}

	declared := checkableKinds(t,
		filepath.Join(root, paths.ProcessInternalObjectSpecsDir),
		filepath.Join(root, paths.ProcessInternalLifecyclesDir))

	type violation struct{ site statusLiteralSite }
	var violations []violation
	checked := 0
	noLifecycle := map[string]bool{}

	for _, s := range sites {
		if !declared[s.kind] {
			noLifecycle[s.kind] = true
			continue
		}
		valid, err := loader.IsValidStatus(s.kind, s.status)
		if err != nil {
			continue
		}
		checked++
		if !valid {
			violations = append(violations, violation{s})
		}
	}

	byPair := map[string]int{}
	for _, v := range violations {
		byPair[v.site.kind+" status="+v.site.status]++
	}
	pairs := make([]string, 0, len(byPair))
	for p := range byPair {
		pairs = append(pairs, p)
	}
	sort.Strings(pairs)

	t.Logf("scanned %d kind+status literals; %d checkable; %d unresolvable kinds (non-object 'kind' keys); %d violations across %d distinct pairs",
		len(sites), checked, len(noLifecycle), len(violations), len(byPair))
	for _, p := range pairs {
		t.Logf("  %s (%d sites)", p, byPair[p])
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].site.file != violations[j].site.file {
			return violations[i].site.file < violations[j].site.file
		}
		return violations[i].site.line < violations[j].site.line
	})

	// Counts, not membership: baselining a pair by name alone would let a twelfth
	// criteria=not_started fixture join the eleven already there without failing anything.
	counts := map[string]int{}
	sitesByPair := map[string][]string{}
	for _, v := range violations {
		key := v.site.kind + "=" + v.site.status
		counts[key]++
		sitesByPair[key] = append(sitesByPair[key], v.site.file+":"+strconv.Itoa(v.site.line))
	}
	for key, n := range counts {
		allowed, known := statusLiteralBaseline[key]
		if !known {
			t.Errorf("new illegal status literal %s at %s — that status is not in the kind's "+
				"lifecycle, so the object cannot hold it", key, strings.Join(sitesByPair[key], ", "))
			continue
		}
		if n > allowed {
			t.Errorf("%s grew from %d to %d sites (%s) — the baseline is a ceiling for existing "+
				"debt, not permission to add more", key, allowed, n, strings.Join(sitesByPair[key], ", "))
		}
	}
	for key, allowed := range statusLiteralBaseline {
		if n := counts[key]; n < allowed {
			t.Logf("%s is down to %d sites (baseline %d) — lower the baseline", key, n, allowed)
		}
	}
}
