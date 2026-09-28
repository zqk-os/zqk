package cli_builders

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/testenvroot"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCLIBuilder_WrapsZQK(t *testing.T) {
	t.Setenv(zqkenv.TestBypassAuth().Name(), "1")
	t.Setenv(zqkenv.APIKey().Name(), "ACC-SYSTEM")
	// Execute the equivalent of `./bin/zqk object list policy`
	// Note: since this is run in the package dir, we reference the zqk binary relatively
	// or we can use "go run ../../../cmd/zqk"

	spec := CLISpec{
		Name:    "go_run_zqk",
		Command: "go",
		Timeout: 60 * time.Second,
	}

	testRoot := t.TempDir()
	_ = testenvroot.BootstrapRoot(testRoot, "../../../")
	builder := NewCLIBuilder(spec, nil).
		WithEnv(zqkenv.SubprocessEnvironWithTestRootAndExtras(testRoot,
			zqkenv.TestBypassAuth().Name()+"=1",
			zqkenv.APIKey().Name()+"=ACC-SYSTEM",
			"ZQK_SESSION_ID=",
			"ZQK_CONVERGENCE_SESSION_ID=",
		)).
		WithArgs("run", "../../../cmd/zqk", "object", "list", "policy", "--limit", "1")

	if _, err := fileutil.Stat("../../../cmd/zqk"); fileutil.IsNotExist(err) {
		t.Skip("Skipping test because cmd/zqk does not exist (likely in open-core candidate tree)")
	}

	output, err := builder.Execute(context.Background())
	if err != nil {
		t.Fatalf("Failed to execute zqk via CLIBuilder: %v", err)
	}

	outStr := string(output)
	if !strings.Contains(outStr, "ID") && !strings.Contains(outStr, "Kind") && !strings.Contains(outStr, "policy") {
		t.Logf("Output: %s", outStr)
		t.Errorf("Expected 'ID', 'Kind', or 'policy' in output indicating a successful list operation")
	}

	t.Logf("CLIBuilder successfully executed ZQK and captured output:\n%s", outStr)
}
