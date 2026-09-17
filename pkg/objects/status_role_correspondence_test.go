package objects

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Correspondence tests for the role plane: the cross-kind status vocabulary versus the code that
// answers role questions with hand-written label lists.
//
// Every one of the 321 statuses in this tree declares a role, so "which of this kind's statuses are
// shovel-ready?" is answerable from data. Until StatusesForRole existed there was no way to ask, so
// callers restated the answer as literal slices — and those slices drifted, in the one direction that
// never fails loudly:
//
//	contractchange listed priority_plan "ready"; no lifecycle has ever defined it.
//	ref_status_constraints required criteria "completed"; the status is "complete".
//	matrix_gate named verification_matrix "completed"; that lifecycle has draft/active/archived.
//	pplan_constants listed priority_plan "rejected"; that lifecycle has "cancelled".
//	remaining_open_count seed, criterion, and status_helpers listed backlog_item "rejected", an alias for
//	archived, so it never matches a persisted status.
//
// A label that does not exist cannot match, so an over-broad filter silently becomes a narrow one and
// a barrier silently becomes a no-op. That is the same failure the dead overlay barrier had.

func lifecyclesDirFromObjects(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir)
	if _, err := fileutil.Stat(dir); err != nil {
		t.Skipf("lifecycles dir unavailable from this working directory: %v", err)
	}
	return dir
}

// loadLifecycleYAML reads a lifecycle straight off disk rather than through LoadLifecycle, so these
// guards do not depend on loader cache state or on the global loader's root.
func loadLifecycleYAML(t *testing.T, dir, file string) *Lifecycle {
	t.Helper()
	raw, err := fileutil.ReadFile(filepath.Join(dir, file))
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var lc Lifecycle
	if err := yaml.Unmarshal(raw, &lc); err != nil {
		return nil
	}
	return &lc
}

func lifecycleFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".yaml") {
			rel, err := filepath.Rel(dir, path)
			if err == nil {
				out = append(out, rel)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read lifecycles dir: %v", err)
	}
	sort.Strings(out)
	return out
}

// TestEveryStatusDeclaresAKnownRole is the precondition the rest of the role plane rests on. Role is
// `omitempty`, so a status added without one silently reads as role "" — and every Role()-based
// barrier then treats it as none of shovel_ready, execution_locked, or terminal, which is the
// permissive answer in most call sites. A typo'd role does the same thing.
func TestEveryStatusDeclaresAKnownRole(t *testing.T) {
	t.Parallel()
	dir := lifecyclesDirFromObjects(t)
	known := make(map[string]bool, len(KnownLifecycleRoles()))
	for _, r := range KnownLifecycleRoles() {
		known[r] = true
	}

	checked := 0
	for _, file := range lifecycleFiles(t, dir) {
		lc := loadLifecycleYAML(t, dir, file)
		if lc == nil || lc.ObjectType == "" {
			continue
		}
		for _, s := range lc.Statuses {
			checked++
			role := strings.TrimSpace(s.Role)
			if role == "" {
				t.Errorf("%s status %q declares no role; Role() returns \"\" for it and every "+
					"role-based barrier reads that as neither ready nor locked nor terminal, which "+
					"is the permissive answer", lc.ObjectType, s.Value)
				continue
			}
			if !known[role] {
				t.Errorf("%s status %q declares role %q, which is not one of the "+
					"LifecycleRole* constants, so no caller can match it", lc.ObjectType, s.Value, role)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no statuses checked; this guard would be vacuous")
	}
	t.Logf("verified role on %d statuses", checked)
}

// TestStatusesForRole_agreesWithForwardRoleLookup pins the new reverse lookup against the forward one
// the kernel already trusted. If these two ever disagree, callers migrating off literal slices would
// get a different answer than the barrier that judges them.
func TestStatusesForRole_agreesWithForwardRoleLookup(t *testing.T) {
	dir := lifecyclesDirFromObjects(t)
	loader := NewLifecycleLoader(dir)
	checker := NewStatusChecker(loader)

	allRoles := KnownLifecycleRoles()

	pairs := 0
	for _, file := range lifecycleFiles(t, dir) {
		lc := loadLifecycleYAML(t, dir, file)
		if lc == nil || lc.ObjectType == "" {
			continue
		}
		kind := lc.ObjectType

		// Both sides must come from the loader. Comparing against the raw YAML status list
		// understates the kind: LoadLifecycle resolves `extends`, so verification_matrix reads as
		// 3 statuses on disk and 8 in effect. An earlier draft of this test compared the two and
		// reported a partition gap that was only inheritance.
		effective, err := loader.GetAllowedStatuses(kind)
		if err != nil {
			t.Errorf("%s GetAllowedStatuses: %v", kind, err)
			continue
		}

		var covered []string
		for _, role := range allRoles {
			got, err := loader.StatusesForRole(kind, role)
			if err != nil {
				t.Errorf("%s StatusesForRole(%s): %v", kind, role, err)
				continue
			}
			for _, status := range got {
				pairs++
				if fwd := checker.Role(kind, status); fwd != role {
					t.Errorf("%s: StatusesForRole(%s) returned %q but Role(%s, %s) says %q",
						kind, role, status, kind, status, fwd)
				}
			}
			covered = append(covered, got...)
		}

		// Partition: KnownLifecycleRoles must account for every effective status, or a caller
		// migrating off a literal slice would silently drop whatever falls outside them.
		if len(covered) != len(effective) {
			sort.Strings(covered)
			missing := make([]string, 0)
			inCovered := make(map[string]bool, len(covered))
			for _, s := range covered {
				inCovered[s] = true
			}
			for _, s := range effective {
				if !inCovered[s] {
					missing = append(missing, s)
				}
			}
			sort.Strings(missing)
			t.Errorf("%s has %d effective statuses but KnownLifecycleRoles account for %d; unclassified: %v",
				kind, len(effective), len(covered), missing)
		}
	}
	if pairs == 0 {
		t.Fatal("no kind/role pairs checked; this guard would be vacuous")
	}
	t.Logf("verified %d status/role pairs round-trip between StatusesForRole and Role", pairs)
}

// TestRoleProgressRank_coversTheOnLadderRolesOnly guards the ordering answer. The point of the ladder
// is that realign and halted have no position on it; if someone adds them to make a comparison
// compile, "is this further along?" starts returning a confident wrong answer for blocked work.
func TestRoleProgressRank_coversTheOnLadderRolesOnly(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		role     string
		wantOK   bool
		wantRank int
	}{
		{LifecycleRoleGrooming, true, 0},
		{LifecycleRoleShovelReady, true, 1},
		{LifecycleRoleExecutionLocked, true, 2},
		{LifecycleRoleTerminal, true, 3},
		{LifecycleRoleRealign, false, 0},
		{LifecycleRoleHalted, false, 0},
		{LifecycleRoleEnforced, false, 0},
		{"", false, 0},
		{"not_a_role", false, 0},
	} {
		rank, ok := RoleProgressRank(tc.role)
		if ok != tc.wantOK {
			t.Errorf("RoleProgressRank(%q) ok=%v, want %v", tc.role, ok, tc.wantOK)
		}
		if ok && rank != tc.wantRank {
			t.Errorf("RoleProgressRank(%q) = %d, want %d", tc.role, rank, tc.wantRank)
		}
	}

	// Strictly increasing, so callers can compare ranks rather than enumerate cases.
	ladder := []string{
		LifecycleRoleGrooming, LifecycleRoleShovelReady,
		LifecycleRoleExecutionLocked, LifecycleRoleTerminal,
	}
	prev := -1
	for _, role := range ladder {
		rank, ok := RoleProgressRank(role)
		if !ok {
			t.Fatalf("%s is on the ladder but has no rank", role)
		}
		if rank <= prev {
			t.Errorf("ladder is not strictly increasing at %s (rank %d after %d)", role, rank, prev)
		}
		prev = rank
	}
}

// kindsWithNoTerminalRole are registry entities rather than work: an organization, a division, and
// an account are active, inactive, or suspended, and none of those is "finished". They legitimately
// never reach the terminal rung, so requiring one of every lifecycle would be wrong.
//
// The list is a ceiling, not a permission. A work kind landing here means role-driven callers can
// never treat its objects as done, which is a real defect — so a new entry fails and must be
// justified rather than appended.
// Kind constants, not strings, so a rename cannot leave a stale entry silently excusing a work kind.
var kindsWithNoTerminalRole = map[string]bool{
	KindAccount:      true,
	KindDivision:     true,
	KindOrganization: true,
}

// TestEveryWorkLifecycleReachesTerminalAndHasAnOrigin checks the ladder is actually walked. A kind
// with no terminal cannot be completed by any role-driven caller, and a kind with no origin has no
// defined landing at creation.
func TestEveryWorkLifecycleReachesTerminalAndHasAnOrigin(t *testing.T) {
	t.Parallel()
	dir := lifecyclesDirFromObjects(t)
	loader := NewLifecycleLoader(dir)
	checked := 0
	for _, file := range lifecycleFiles(t, dir) {
		lc := loadLifecycleYAML(t, dir, file)
		if lc == nil || lc.ObjectType == "" {
			continue
		}
		kind := lc.ObjectType
		checked++

		terminals, err := loader.StatusesForRole(kind, LifecycleRoleTerminal)
		if err != nil {
			t.Errorf("%s StatusesForRole(terminal): %v", kind, err)
			continue
		}
		switch {
		case len(terminals) == 0 && !kindsWithNoTerminalRole[kind]:
			t.Errorf("%s has no status with role terminal, so no role-driven caller can treat one "+
				"of its objects as finished; if this kind is a registry entity rather than work, "+
				"add it to kindsWithNoTerminalRole with that reason", kind)
		case len(terminals) > 0 && kindsWithNoTerminalRole[kind]:
			t.Errorf("%s is listed in kindsWithNoTerminalRole but now has terminal statuses %v; "+
				"remove the exemption", kind, terminals)
		}

		hasOrigin := false
		for _, s := range lc.Statuses {
			if s.Origin {
				hasOrigin = true
			}
		}
		if !hasOrigin {
			t.Errorf("%s declares no origin status, so creation has no defined landing", kind)
		}
	}
	if checked == 0 {
		t.Fatal("no lifecycles checked; this guard would be vacuous")
	}
	t.Logf("verified ladder endpoints on %d lifecycles", checked)
}
