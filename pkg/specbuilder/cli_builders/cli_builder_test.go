package cli_builders

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestCLIBuilder_WrapsZQK(t *testing.T) {
	t.Setenv(zqkenv.TestBypassAuth(), "1")
	t.Setenv(zqkenv.APIKey(), "account:system")
	t.Setenv(zqkenv.TestBypassAuth(), "1")
	t.Setenv(zqkenv.APIKey(), "account:system")
	t.Setenv("MAIN_TEST_BYPASS_AUTH", "1")
	t.Setenv("MAIN_API_KEY", "account:system")
	// Execute the equivalent of `./bin/zqk object list policy`
	// Note: since this is run in the package dir, we reference the zqk binary relatively
	// or we can use "go run ../../../cmd/zqk"

	spec := CLISpec{
		Name:    "go_run_zqk",
		Command: "go",
		Timeout: 60 * time.Second,
	}

	builder := NewCLIBuilder(spec, nil).
		WithEnv(append(os.Environ(), "ZQK_ROOT="+t.TempDir())).
		WithArgs("run", "../../../cmd/zqk/main.go", "object", "list", "policy", "--limit", "1")

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
