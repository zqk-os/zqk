package quick

import (
	"os"

	"github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/object"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/quick"
	"github.com/spf13/cobra"
)

const emptyValue = ""

// NewQuickBacklogItemCmd creates the quick backlog-item command from spec-driven builder; RunE reads flags and creates via object create.
func NewQuickBacklogItemCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewQuickBacklogItemCommandBuilder()
	cmd.RunE = runQuickBacklogItem
	return cmd
}

func runQuickBacklogItem(cmd *cobra.Command, _ []string) error {
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
		objects.FieldKeyKind:        objects.KindBacklogItem,
		objects.FieldKeyTitle:       titleOut,
		objects.FieldKeyDescription: body,
	}
	return object.RunCreateWithData(cmd, objects.KindBacklogItem, objData)
}

func readTitleAndBody(filePath, content, titleOverride string) (body, title string, err error) {
	if filePath != emptyValue {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", "", errfmt.Newf("read file").Wrap(err)
		}
		parsed := quick.ParseFileContent(data)
		title = parsed.Title
		body = parsed.Body
	} else if content != emptyValue {
		parsed := quick.ParseMarkdownOrText(content)
		title = parsed.Title
		body = parsed.Body
	}
	if titleOverride != emptyValue {
		title = titleOverride
	}
	return body, title, nil
}
