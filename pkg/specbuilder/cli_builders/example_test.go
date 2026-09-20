package cli_builders_test

import (
	"context"
	"fmt"
	"log"

	"github.com/zqk-os/zqk/pkg/specbuilder/cli_builders"
)

// ExampleCLIBuilder demonstrates how the CLI wrapper can be used
// to wrap the zqk CLI itself, automatically capturing execution metrics
// and logging them into the Knowledge Kernel via FluentEvent.
func ExampleCLIBuilder() {
	// Let's assume we want to run `./bin/zqk object list` or `./zqk object list`
	// This command will list all objects, while automatically capturing
	// execution duration and metrics to the Knowledge Kernel.
	spec := cli_builders.CLISpec{
		Name:    "zqk_cli",
		Command: "./zqk",
	}

	output, err := cli_builders.NewCLIBuilder(spec, nil).
		WithArg("object").
		WithArg("list").
		Execute(context.Background())

	if err != nil {
		log.Fatalf("Failed to execute zqk via wrapper: %v", err)
	}

	fmt.Printf("Successfully executed zqk command. Output size: %d bytes\n", len(output))

	// Output logging is handled internally by the builder:
	// - pre-execution metrics logged
	// - post-execution success/failure metrics logged
}
