package query

import (
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/traversal"
)

// FormatVisualizer formats query results as graph edge path trees or visual relationships.
func FormatVisualizer(result *traversal.QueryResult, ast *traversal.QueryAST) string {
	var sb strings.Builder
	sb.WriteString("=== ZPARQL Graph Path Visualization ===\n")
	if result == nil || len(result.Rows) == 0 {
		sb.WriteString("No graph matches found to visualize.\n")
		return sb.String()
	}

	hasEdges := false
	if ast != nil {
		for _, p := range ast.Patterns {
			if len(p.Edges) > 0 {
				hasEdges = true
				break
			}
		}
	}

	if hasEdges && ast != nil {
		formatPathPatternResults(&sb, result, ast)
	} else {
		formatNodeListResults(&sb, result)
	}

	fmt.Fprintf(&sb, "\n(Visualized %d path(s))\n", len(result.Rows))
	return sb.String()
}

func formatPathPatternResults(sb *strings.Builder, result *traversal.QueryResult, ast *traversal.QueryAST) {
	for rowIdx, row := range result.Rows {
		fmt.Fprintf(sb, "\n[%d] Path:\n", rowIdx+1)

		for _, p := range ast.Patterns {
			if len(p.Nodes) == 0 {
				continue
			}

			for i := 0; i < len(p.Nodes); i++ {
				node := p.Nodes[i]
				nodeVal := findNodeValue(row, node.Variable)
				kindStr := node.Kind
				if kindStr == "" {
					kindStr = "node"
				}

				indent := strings.Repeat("    ", i)
				if i == 0 {
					fmt.Fprintf(sb, "  ● (%s:%s %v)\n", node.Variable, kindStr, nodeVal)
				} else {
					edge := p.Edges[i-1]
					edgeType := edge.EdgeType
					if edgeType == "" {
						edgeType = "related"
					}
					var arrow string
					switch edge.Direction {
					case traversal.DirectionIncoming:
						arrow = fmt.Sprintf("◄──[:%s]──", edgeType)
					case traversal.DirectionUndirected:
						arrow = fmt.Sprintf("───[:%s]───", edgeType)
					default:
						arrow = fmt.Sprintf("───[:%s]──►", edgeType)
					}
					fmt.Fprintf(sb, "  %s%s (%s:%s %v)\n", indent, arrow, node.Variable, kindStr, nodeVal)
				}
			}
		}

		// Print any other projected return values for this row
		otherVals := make([]string, 0)
		for _, h := range result.Headers {
			if v, ok := row[h]; ok && v != nil {
				otherVals = append(otherVals, fmt.Sprintf("%s=%v", h, v))
			}
		}
		if len(otherVals) > 0 {
			fmt.Fprintf(sb, "      └─ Properties: %s\n", strings.Join(otherVals, ", "))
		}
	}
}

func formatNodeListResults(sb *strings.Builder, result *traversal.QueryResult) {
	for _, row := range result.Rows {
		primaryID := ""
		for _, h := range result.Headers {
			if strings.HasSuffix(h, ".id") || strings.EqualFold(h, "id") {
				if val := row[h]; val != nil {
					primaryID = fmt.Sprintf("%v", val)
					break
				}
			}
		}
		if primaryID == "" && len(result.Headers) > 0 {
			primaryID = fmt.Sprintf("%v", row[result.Headers[0]])
		}

		fmt.Fprintf(sb, "  ● [%s]\n", primaryID)
		for _, h := range result.Headers {
			if val := row[h]; val != nil {
				fmt.Fprintf(sb, "      ├─ %s: %v\n", h, val)
			}
		}
	}
}

func findNodeValue(row map[string]any, variable string) any {
	if variable == "" {
		return ""
	}
	idKey := variable + ".id"
	if v, ok := row[idKey]; ok && v != nil {
		return v
	}
	titleKey := variable + ".title"
	if v, ok := row[titleKey]; ok && v != nil {
		return v
	}
	for k, v := range row {
		if strings.HasPrefix(k, variable+".") && v != nil {
			return v
		}
	}
	return variable
}
