package cli_builders_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/specbuilder/cli_builders"
)

func TestOsmosisDemo(t *testing.T) {
	spec := cli_builders.CLISpec{
		Name:    "demo",
		Command: "go",
		Timeout: 5 * time.Second,
	}

	builder := cli_builders.NewCLIBuilder(spec, nil).
		WithArg("help").
		WithArg("build")

	fmt.Println("Running `go help build` through CLI Wrapper...")
	output, err := builder.Execute(context.Background())
	if err != nil {
		t.Fatalf("Failed to run command: %v", err)
	}

	fmt.Printf("Command exited cleanly (output len: %d bytes)\n", len(output))

	// Wait a moment for async aggregator to process the event
	time.Sleep(1 * time.Second)

	// Graceful shutdown to ensure logs flush
	cli_builders.GetKnowledgeAggregator().Stop()
}
