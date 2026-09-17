package object

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// wireExecForTest wires a spawned CLI child to an isolated project AND gives it the harness
// identity. Tests must use this rather than zqkenv.WireExecForIsolatedProject directly.
//
// The production helper isolates paths but passes HOME through, so a child with no explicit key
// falls back to $HOME/.zqk/credentials, authenticates as whichever developer is logged in, and then
// fails resolving that session id against the empty temp store ("unauthorized: session ZQK-nnnn not
// found"). Whether a test passed therefore depended on who was logged in.
//
// Spawn sites used to spell this as two lines -- the wire call followed by an EnvWithTestRoot
// override -- and omitting the second line was silent. That is what broke TestFieldsCommand while
// identical-looking tests beside it passed. One call cannot be half-written.
func wireExecForTest(cmd *exec.Cmd, testRoot string) {
	if cmd == nil {
		return
	}
	cmd.Dir = testRoot
	cmd.Env = EnvWithTestRoot(testRoot)
}

// TestSpawnSitesCarryHarnessIdentity refuses a direct zqkenv.WireExecForIsolatedProject call from a
// test in this package.
//
// That helper isolates paths but passes HOME through, so a spawned CLI child with no explicit key
// falls back to $HOME/.zqk/credentials, authenticates as whichever developer is logged in, and then
// fails resolving that session id against the empty temp store: "unauthorized: session ZQK-nnnn not
// found". Whether a test passed depended on who was logged in and whether they had a live session.
//
// The correct spelling used to be two lines -- the wire call plus an EnvWithTestRoot override -- and
// 27 of the package's spawn sites were missing the second one while identical-looking neighbours had
// it. A guard is warranted because the failure mode is silent at the call site: the code reads fine,
// compiles, and only misbehaves on a machine with a logged-in developer. wireExecForTest cannot be
// half-written the way the two-line idiom could.
func TestSpawnSitesCarryHarnessIdentity(t *testing.T) {
	t.Parallel()

	const forbidden = "zqkenv.WireExecForIsolatedProject"

	entries, err := fileutil.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	self := filepath.Base(mustSelfName())
	var offenders []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") || name == self {
			continue
		}
		data, err := fileutil.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			code, _, _ := strings.Cut(line, "//")
			if strings.Contains(code, forbidden) {
				offenders = append(offenders, name+":"+itoa(i+1))
			}
		}
	}

	if len(offenders) > 0 {
		t.Errorf("these test spawn sites call %s directly, which authenticates the child as the "+
			"logged-in developer instead of the harness; use wireExecForTest instead:\n  %s",
			forbidden, strings.Join(offenders, "\n  "))
	}
}

func mustSelfName() string { return "spawn_identity_guard_test.go" }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
