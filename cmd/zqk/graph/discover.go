package graph

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewDiscoverCmd creates a new discover command
func NewDiscoverCmd() *cobra.Command {
	var depth int

	helpBuilder := clipkg.DynamicHelpBuilder(
		"Discover object relationships",
		"Visually explore the relationship tree for an object, highlighting dependencies, alignment, and references.",
		"",
		"This command traverses the graph to map out all connections to the target object,",
		"presenting them in an easily understandable tree format.",
	).
		AddExample("Discover all relationships up to depth 2", "%s graph discover BLI-123").
		AddExample("Explore deeper relationships", "%s graph discover GOAL-456 --depth 3")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewGraphDiscoverCommandBuilder(), &cobra.Command{
		Use:   "discover <id>",
		Short: "Discover and visualize object relationships",
		Args:  cobra.ExactArgs(1),
	})

	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		return runDiscover(cmd, args[0], depth)
	})

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().IntVarP(&depth, "depth", "d", 2, "Maximum traversal depth")

	return cmd
}

type discoverNode struct {
	id       string
	kind     string
	title    string
	children []*discoverNode
}

func runDiscover(cmd *cobra.Command, id string, maxDepth int) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return err
	}

	storage := proc.Storage()

	// Fetch the root object
	rootObj, err := storage.Read(proc.OperationContext(), proc.SecurityContext(), id)
	if err != nil {
		return errfmt.Newf("failed to read root object").Wrap(err)
	}

	rootKind, _ := rootObj[objects.FieldKeyKind].(string)
	rootTitle, _ := rootObj[objects.FieldKeyTitle].(string)

	rootNode := &discoverNode{
		id:    id,
		kind:  rootKind,
		title: rootTitle,
	}

	visited := make(map[string]bool)
	visited[id] = true

	// Build the tree recursively
	err = buildTree(proc, rootNode, 1, maxDepth, visited)
	if err != nil {
		return errfmt.Newf("failed to build relationship tree").Wrap(err)
	}

	// Render output
	var sb strings.Builder
	sb.WriteString("Relationship Discovery for:\n")

	renderTree(&sb, rootNode, "", true, true)

	return cli.WriteOutput(cmd, []byte(sb.String()))
}

func buildTree(proc *cli.Processor, node *discoverNode, currentDepth, maxDepth int, visited map[string]bool) error {
	if currentDepth > maxDepth {
		return nil
	}

	// Use GetNeighbors to find direct connections
	neighbors, err := proc.Storage().GetNeighbors(proc.OperationContext(), proc.SecurityContext(), node.id, "both")
	if err != nil {
		return err
	}

	for _, neighbor := range neighbors {
		nID, _ := neighbor[objects.FieldKeyID].(string)
		if nID == "" {
			continue
		}

		// Prevent infinite loops in cyclic graphs
		if visited[nID] {
			continue
		}
		visited[nID] = true

		nKind, _ := neighbor[objects.FieldKeyKind].(string)
		nTitle, _ := neighbor[objects.FieldKeyTitle].(string)

		childNode := &discoverNode{
			id:    nID,
			kind:  nKind,
			title: nTitle,
		}
		node.children = append(node.children, childNode)

		if err := buildTree(proc, childNode, currentDepth+1, maxDepth, visited); err != nil {
			return err
		}
	}

	return nil
}

func renderTree(sb *strings.Builder, node *discoverNode, prefix string, isLast bool, isRoot bool) {
	if isRoot {
		sb.WriteString(fmt.Sprintf("📦 [%s] %s: %s\n", node.kind, node.id, node.title))
	} else {
		marker := "├──"
		if isLast {
			marker = "└──"
		}
		sb.WriteString(fmt.Sprintf("%s%s [%s] %s: %s\n", prefix, marker, node.kind, node.id, node.title))
	}

	childPrefix := prefix
	if !isRoot {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, child := range node.children {
		renderTree(sb, child, childPrefix, i == len(node.children)-1, false)
	}
}
