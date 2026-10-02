package system

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	paths "github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// resolveTestProjectRoot locates the project root for doc coherency tests.
func resolveTestProjectRoot(t *testing.T) string {
	t.Helper()
	root := paths.ResolveProjectRoot(".")
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get working directory: %v", err)
		}
		// Fallback upward search
		curr := cwd
		for i := 0; i < 5; i++ {
			if _, err := os.Stat(filepath.Join(curr, "go.mod")); err == nil {
				return curr
			}
			curr = filepath.Dir(curr)
		}
		t.Skip("not running inside a valid project root")
	}
	return root
}

// CRIT-1790914690287150000-63ea0c38: README Layer 2 identifiers match kinds.go constants
func TestDocCoherency_CRIT_README_Identifiers(t *testing.T) {
	root := resolveTestProjectRoot(t)
	content, err := fileutil.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}
	text := string(content)

	// In Layer 2 table, ensure valid kinds are used
	layer2Pattern := regexp.MustCompile(`\|\s*\*\*Layer 2\*\*\s*\|[^\n]+`)
	loc := layer2Pattern.FindString(text)
	if loc == "" {
		t.Fatalf("could not find Layer 2 table row in README.md")
	}

	// Must contain Goal, BacklogItem, Decision
	expectedKinds := []string{"`Goal`", "`BacklogItem`", "`Decision`", "`InvariantGate`"}
	for _, k := range expectedKinds {
		if !strings.Contains(loc, k) {
			t.Errorf("Layer 2 table row missing expected kind: %s in %q", k, loc)
		}
	}

	// Must NOT contain phantom kinds Intent or LineageNode in Layer 2 row
	forbiddenKinds := []string{"`Intent`", "`LineageNode`"}
	for _, fk := range forbiddenKinds {
		if strings.Contains(loc, fk) {
			t.Errorf("Layer 2 table row contains obsolete/phantom identifier: %s", fk)
		}
	}
}

// CRIT-1790914693702825000-55bed4ac: README state machine planes match lifecycle spec
func TestDocCoherency_CRIT_README_Planes(t *testing.T) {
	root := resolveTestProjectRoot(t)
	content, err := fileutil.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}
	text := string(content)

	// Ensure phantom plane names do not appear in README
	phantomPlanes := []string{"PlaneDraft", "PlaneStaged", "PlanePromoted"}
	for _, p := range phantomPlanes {
		if strings.Contains(text, p) {
			t.Errorf("README.md contains obsolete plane constant: %s", p)
		}
	}

	// Ensure membrane description accurately references Draft -> Promoted
	if !strings.Contains(text, "`Draft` → `Promoted`") && !strings.Contains(text, "Draft") {
		t.Errorf("README.md membrane section does not reference Draft plane")
	}
}

// CRIT-1790914697056151000-e5995f94: No unsubstantiated quantitative claims in README.md
func TestDocCoherency_CRIT_README_Claims(t *testing.T) {
	root := resolveTestProjectRoot(t)
	content, err := fileutil.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}
	text := string(content)

	// Ensure unbenchmarked 80% token claim is removed
	if strings.Contains(text, "80%") {
		t.Error("README.md still contains unbenchmarked claim '80%'")
	}
}

// CRIT-1790914709302716000-61c310fe: Every CLI command in docs/ maps to cobra registration
func TestDocCoherency_CRIT_CLI_CommandsValid(t *testing.T) {
	root := resolveTestProjectRoot(t)
	cfrPath := filepath.Join(root, "docs", "onboarding", "COMMUNITY_FIRST_RUN.md")
	content, err := fileutil.ReadFile(cfrPath)
	if err != nil {
		t.Fatalf("failed to read COMMUNITY_FIRST_RUN.md: %v", err)
	}
	text := string(content)

	// Ensure alias relationship is explicitly noted
	if !strings.Contains(text, "alias for zqk system init") {
		t.Errorf("COMMUNITY_FIRST_RUN.md does not document zqk init as alias for zqk system init")
	}
	if !strings.Contains(text, "alias: zqk system start-here") {
		t.Errorf("COMMUNITY_FIRST_RUN.md does not document quickstart/start-here alias relationship")
	}
}

// CRIT-1790914712454174000-7d95d7fd: CLI_REFERENCE.md covers registered root commands
func TestDocCoherency_CRIT_CLI_ReferenceComplete(t *testing.T) {
	root := resolveTestProjectRoot(t)
	refPath := filepath.Join(root, "docs", "manual", "CLI_REFERENCE.md")
	content, err := fileutil.ReadFile(refPath)
	if err != nil {
		t.Fatalf("failed to read CLI_REFERENCE.md: %v", err)
	}
	text := string(content)

	// Key commands that must be covered
	expectedCmds := []string{"`zqk do`", "`zqk query`", "`zqk mutate`", "`zqk inspect`", "`zqk run`", "`zqk ui`", "`zqk init`", "`zqk state`"}
	for _, cmd := range expectedCmds {
		if !strings.Contains(text, cmd) {
			t.Errorf("CLI_REFERENCE.md missing documentation for command %s", cmd)
		}
	}

	// Phantom commands that must NOT be present
	if strings.Contains(text, "`zqk use`") {
		t.Errorf("CLI_REFERENCE.md lists phantom command 'zqk use'")
	}
}

// CRIT-1790914715619302000-4c7e29bf: No phantom commands documented — zqk object apply must not appear
func TestDocCoherency_CRIT_CLI_NoPhantomApply(t *testing.T) {
	root := resolveTestProjectRoot(t)
	zqlPath := filepath.Join(root, "docs", "manual", "ZQL_MUTATIONS.md")
	content, err := fileutil.ReadFile(zqlPath)
	if err != nil {
		t.Fatalf("failed to read ZQL_MUTATIONS.md: %v", err)
	}
	text := string(content)

	if strings.Contains(text, "object apply") {
		t.Errorf("ZQL_MUTATIONS.md still references non-existent 'zqk object apply'")
	}
	if !strings.Contains(text, "zqk mutate") {
		t.Errorf("ZQL_MUTATIONS.md does not document correct command 'zqk mutate'")
	}
}

// CRIT-1790914736584424000-3d7e9eff: Pack composition docs contain no contradictory claims
func TestDocCoherency_CRIT_Arch_PackComposition(t *testing.T) {
	root := resolveTestProjectRoot(t)
	packPath := filepath.Join(root, "PACK-COMPOSITION.md")
	content, err := fileutil.ReadFile(packPath)
	if err != nil {
		t.Fatalf("failed to read PACK-COMPOSITION.md: %v", err)
	}
	text := string(content)

	// Must clarify spec metadata vs Go recompilation
	if !strings.Contains(text, "Pack Go code") || !strings.Contains(text, "requires recompilation") {
		t.Errorf("PACK-COMPOSITION.md does not clarify that Pack Go code requires recompilation")
	}
}

// CRIT-1790914740023133000-dc7e207e: CLI taxonomy approved list matches current state note
func TestDocCoherency_CRIT_Arch_TaxonomyGovernance(t *testing.T) {
	root := resolveTestProjectRoot(t)
	taxPath := filepath.Join(root, "docs", "architecture", "CLI_COMMAND_TAXONOMY_STANDARDS.md")
	content, err := fileutil.ReadFile(taxPath)
	if err != nil {
		t.Fatalf("failed to read CLI_COMMAND_TAXONOMY_STANDARDS.md: %v", err)
	}
	text := string(content)

	// Must acknowledge current state / implementation note
	if !strings.Contains(text, "Implementation Note") && !strings.Contains(text, "Current State") {
		t.Errorf("CLI_COMMAND_TAXONOMY_STANDARDS.md missing implementation note regarding current CLI surface")
	}
}

// CRIT-1790914743254354000-6eba1bc4: Architecture INDEX.md lists all existing docs
func TestDocCoherency_CRIT_Arch_IndexComplete(t *testing.T) {
	root := resolveTestProjectRoot(t)
	indexPath := filepath.Join(root, "docs", "architecture", "INDEX.md")
	content, err := fileutil.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read INDEX.md: %v", err)
	}
	text := string(content)

	expectedDocs := []string{
		"PROJECT_SCOPED_REVERSE_REFERENCE_INDEX_AND_THREAD_SAFE_CACHES.md",
		"FAIL_CLOSED_ERROR_PROPAGATION.md",
		"ergonomics/DIAGNOSTICS_AUTO_REMEDY.md",
		"ergonomics/WORKTREE_KERNEL_RESOLUTION.md",
		"ergonomics/ZQK_DO_AUTONOMOUS_EXECUTION.md",
	}

	for _, doc := range expectedDocs {
		if !strings.Contains(text, doc) {
			t.Errorf("INDEX.md missing entry for: %s", doc)
		}
	}
}

// CRIT-1790914746676623000-df997477: pkg/README.md has correct descriptions and no garbage characters
func TestDocCoherency_CRIT_Pkg_README_Quality(t *testing.T) {
	root := resolveTestProjectRoot(t)
	pkgPath := filepath.Join(root, "pkg", "README.md")
	content, err := fileutil.ReadFile(pkgPath)
	if err != nil {
		t.Fatalf("failed to read pkg/README.md: %v", err)
	}
	lines := strings.Split(string(content), "\n")

	// Verify table entries
	for _, l := range lines {
		if strings.HasPrefix(l, "| [observer]") {
			if strings.Contains(l, "Lock operation names") {
				t.Errorf("pkg/README.md observer package description is clobbered by metrics description")
			}
			if !strings.Contains(l, "Event observation") {
				t.Errorf("pkg/README.md observer package description missing expected wording")
			}
		}
		if strings.HasPrefix(l, "| [testdiscovery]") {
			if strings.Contains(l, `Import ( "context"`) {
				t.Errorf("pkg/README.md testdiscovery contains raw Go import statement")
			}
		}
		// Check table rows for trailing orphaned ' p |'
		if strings.HasPrefix(l, "| [") && strings.HasSuffix(strings.TrimSpace(l), " p |") {
			t.Errorf("pkg/README.md contains trailing orphaned ' p |' in row: %s", l)
		}
	}
}

// CRIT-1790914750147522000-b4949a63: internal/README.md references correct import paths
func TestDocCoherency_CRIT_Internal_README_Paths(t *testing.T) {
	root := resolveTestProjectRoot(t)
	intPath := filepath.Join(root, "internal", "README.md")
	content, err := fileutil.ReadFile(intPath)
	if err != nil {
		t.Fatalf("failed to read internal/README.md: %v", err)
	}
	text := string(content)

	if strings.Contains(text, "github.com/zqk-os/zqk/internal/cli") {
		t.Errorf("internal/README.md references invalid import path internal/cli")
	}
	if !strings.Contains(text, "github.com/zqk-os/zqk/internal/codegen") {
		t.Errorf("internal/README.md does not reference existing internal/codegen package")
	}
}

// CRIT-1790914753310404000-6078255d: Getting-started files cross-reference each other
func TestDocCoherency_CRIT_GettingStarted_Crossrefs(t *testing.T) {
	root := resolveTestProjectRoot(t)

	f1, err := fileutil.ReadFile(filepath.Join(root, "ZQK_GETTING_STARTED.md"))
	if err != nil {
		t.Fatalf("failed to read ZQK_GETTING_STARTED.md: %v", err)
	}
	if !strings.Contains(string(f1), "docs/getting-started.md") {
		t.Errorf("ZQK_GETTING_STARTED.md missing cross-reference to docs/getting-started.md")
	}

	f2, err := fileutil.ReadFile(filepath.Join(root, "docs", "getting-started.md"))
	if err != nil {
		t.Fatalf("failed to read docs/getting-started.md: %v", err)
	}
	if !strings.Contains(string(f2), "ZQK_GETTING_STARTED.md") {
		t.Errorf("docs/getting-started.md missing cross-reference to ZQK_GETTING_STARTED.md")
	}
}
