package validate

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestCheckCommandCtorPattern_WrapAndPatchIsViolation(t *testing.T) {
	src := `package agent

import "github.com/spf13/cobra"

func NewValidateCmd() *cobra.Command {
	cmd := validate.NewValidateAgentCmd()
	cmd.Use = "validate"
	cmd.Short = "Validate codebase and optionally verify cryptographic stamp"
	return cmd
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "cmd/zqk/agent/validate.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = checkCommandCtorPattern("cmd/zqk/agent/validate.go", file)
	if err == nil {
		t.Fatal("expected wrap-and-patch constructor to fail AST command-ctor check")
	}
	if !strings.Contains(err.Error(), "NewValidateCmd wrapping NewValidateAgentCmd") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckCommandCtorPattern_CommandBuilderIsClean(t *testing.T) {
	src := `package agent

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

func NewValidateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentValidateCommandBuilder()
	cmd.RunE = cli.WithProcessor(RunValidateAgent)
	return cmd
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "cmd/zqk/agent/validate.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := checkCommandCtorPattern("cmd/zqk/agent/validate.go", file); err != nil {
		t.Fatalf("builder pattern should pass: %v", err)
	}
}

func TestCheckCommandCtorPattern_AddCommandIsNotAWrap(t *testing.T) {
	src := `package agent

import "github.com/spf13/cobra"

func NewAgentCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAgentCommandBuilder()
	cmd.AddCommand(NewValidateCmd())
	return cmd
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "cmd/zqk/agent/agent.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := checkCommandCtorPattern("cmd/zqk/agent/agent.go", file); err != nil {
		t.Fatalf("group AddCommand should pass: %v", err)
	}
}
