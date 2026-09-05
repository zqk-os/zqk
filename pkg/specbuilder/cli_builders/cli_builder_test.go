package cli_builders

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestCLIBuilder_WrapsZQK(t *testing.T) {
	t.Setenv(zqkenv.TestBypassAuth(), "1")
	t.Setenv(zqkenv.APIKey(), "ACC-1785920548450214012-68b850c0")
	// Execute the equivalent of `./bin/zqk object list policy`
	// Note: since this is run in the package dir, we reference the zqk binary relatively
	// or we can use "go run ../../../cmd/zqk"

	spec := CLISpec{
		Name:    "go_run_zqk",
		Command: "go",
		Timeout: 60 * time.Second,
	}

	testRoot := t.TempDir()
	builder := NewCLIBuilder(spec, nil).
		WithEnv(append(os.Environ(),
			"ZQK_ROOT="+testRoot,
			zqkenv.TestRoot()+"="+testRoot,
			zqkenv.TestBypassAuth()+"=1",
			zqkenv.APIKey()+"=ACC-1785920548450214012-68b850c0",
			"ZQK_SESSION_ID=",
			"ZQK_CONVERGENCE_SESSION_ID=",
		)).
		WithArgs("run", "../../../cmd/zqk/main.go", "object", "list", "policy", "--limit", "1")

	if _, err := fileutil.Stat("../../../cmd/zqk/main.go"); fileutil.IsNotExist(err) {
		t.Skip("Skipping test because cmd/zqk/main.go does not exist (likely in open-core candidate tree)")
	}

	output, err := builder.Execute(context.Background())
	if err != nil {
		t.Fatalf("Failed to execute zqk via CLIBuilder: %v", err)
	}

	outStr := string(output)
	if !strings.Contains(outStr, "ID") && !strings.Contains(outStr, "Kind") {
		t.Logf("Output: %s", outStr)
		t.Errorf("Expected 'ID' or 'Kind' in output indicating a successful list operation")
	}

	t.Logf("CLIBuilder successfully executed ZQK and captured output:\n%s", outStr)
}
