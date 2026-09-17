package mcp

import (
	"context"
	"encoding/json"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// ExtractActorContext extracts actor information (SecurityContext) from MCP request parameters
// if a "_meta" field is present. It returns a new context with the security context embedded,
// or the original context if no actor information is found.
func ExtractActorContext(ctx context.Context, params json.RawMessage) context.Context {
	if len(params) == 0 {
		return ctx
	}

	var paramsMap map[string]any
	if err := json.Unmarshal(params, &paramsMap); err != nil {
		return ctx
	}

	return ExtractActorContextFromArgs(ctx, paramsMap)
}

// ExtractActorContextFromArgs extracts actor information from a map of arguments.
func ExtractActorContextFromArgs(ctx context.Context, args map[string]any) context.Context {
	if args == nil {
		return ctx
	}

	metaRaw, exists := args["_meta"]
	if !exists {
		return ctx
	}

	metaMap, ok := metaRaw.(map[string]any)
	if !ok {
		return ctx
	}

	secCtx := InitializeSecurityContextFromMCP(metaMap)
	if secCtx != nil && secCtx.AccountID != "" {
		return pkgctx.WithSecurityContext(ctx, secCtx)
	}

	return ctx
}
