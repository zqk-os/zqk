package quick

import (
	"github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/object"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewQuickDecisionCmd creates the quick decision command from spec-driven builder; RunE reads flags and creates via object create.
func NewQuickDecisionCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewQuickDecisionCommandBuilder()
	cmd.RunE = runQuickDecision
	return cmd
}

func runQuickDecision(cmd *cobra.Command, _ []string) error {
	file, _ := cmd.Flags().GetString("file")
	content, _ := cmd.Flags().GetString("content")
	title, _ := cmd.Flags().GetString("title")
	body, titleOut, err := readTitleAndBody(file, content, title)
	if err != nil {
		return err
	}
	if titleOut == emptyValue {
		return errfmt.Errorf("title is required (provide --file, --content with a first line or heading, or --title)")
	}
	objData := map[string]any{
		objects.FieldKeyKind:    objects.KindDecision,
		objects.FieldKeyTitle:   titleOut,
		objects.FieldKeyContext: body,
	}
	return object.RunCreateWithData(cmd, objects.KindDecision, objData)
}
