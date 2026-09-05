package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/observer"
)

const (
	observerSearchToolSuffix = "observer_search"
	observerSearchMaxHits    = 20
)

// RegisterObserverTools exposes live Go AST search so agents can locate real
// symbols instead of inventing files. Do not persist a census on agent_task.
func RegisterObserverTools(server *Server) {
	server.RegisterTool(
		GetToolName(observerSearchToolSuffix),
		"Search the live Go AST for functions, methods, types, and structs. Call this BEFORE inventing a file or package. Pass name (symbol substring) and/or path (pkg/... or cmd/... file or dir). Returns file, line, kind, and signature. Then read_code the returned path.",
		map[string]any{
			objects.FieldKeyType: "object",
			"properties": map[string]any{
				objects.FieldKeyName: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Symbol substring (e.g. BranchRef, UnpaidLookupStreak)",
				},
				objects.FieldKeyPath: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Optional pkg/ or cmd/ file or directory to scope the walk",
				},
				objects.FieldKeyKind: map[string]any{
					objects.FieldKeyType:        "string",
					objects.FieldKeyDescription: "Optional entity kind: function, method, type, interface, struct",
				},
				mcpToolArgKeyLimit: map[string]any{
					objects.FieldKeyType:        "number",
					objects.FieldKeyDescription: "Maximum hits (default 20, max 50)",
				},
			},
		},
		nil,
	)
}

// HandleObserverSearch runs a scoped AST query against the sandbox root.
func HandleObserverSearch(ctx context.Context, root string, args map[string]any) (any, error) {
	name, _ := args[objects.FieldKeyName].(string)
	path, _ := args[objects.FieldKeyPath].(string)
	kind, _ := args[objects.FieldKeyKind].(string)
	if strings.TrimSpace(name) == "" && strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("observer_search requires name or path")
	}
	q := observer.Query{
		Root:  root,
		Name:  name,
		Path:  path,
		Kind:  kind,
		Limit: observerSearchLimit(args),
	}
	hits, err := observer.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return "No AST hits. Try a shorter name or a pkg/ / cmd/ path, then read_code the real file.", nil
	}
	lines := make([]string, 0, len(hits))
	for _, hit := range hits {
		lines = append(lines, observer.FormatHit(hit))
	}
	return strings.Join(lines, "\n"), nil
}

func observerSearchLimit(args map[string]any) int {
	switch v := args[mcpToolArgKeyLimit].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return observerSearchMaxHits
	}
}

func (s *Server) handleObserverSearchTool(ctx context.Context, args map[string]any) (any, error) {
	return HandleObserverSearch(ctx, s.fileSandboxRoot(), args)
}
