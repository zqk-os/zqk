package system

import (
	"testing"

	"github.com/zqk-os/zqk/internal/cli"

	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/spf13/cobra"
)

func TestRunAutoFixIssuesViaPipeline_NilCtx(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "test"}
	obj := &parser.ParsedObject{}

	_, err := RunAutoFixIssuesViaPipeline(nil, cmd, obj, "file.yaml", "backlog_item", nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected error for nil ctx")
	}
}

func TestRunAutoFixIssuesViaPipeline_NilCmd(t *testing.T) {
	t.Parallel()

	ctx := cli.ContextForProjectRoot("").WithProfile("system")
	obj := &parser.ParsedObject{}

	_, err := RunAutoFixIssuesViaPipeline(ctx, nil, obj, "file.yaml", "backlog_item", nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected error for nil cmd")
	}
}

func TestRunAutoFixIssuesViaPipeline_NilObj(t *testing.T) {
	t.Parallel()

	ctx := cli.ContextForProjectRoot("").WithProfile("system")
	cmd := &cobra.Command{Use: "test"}

	_, err := RunAutoFixIssuesViaPipeline(ctx, cmd, nil, "file.yaml", "backlog_item", nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected error for nil obj")
	}
}
