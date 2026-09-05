package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
)

// ResolveSemanticArgument evaluates an incoming CLI argument.
// If the argument matches the strict formatting of a ZQK hash (e.g. PRI-1234, BLI-5678), it is returned directly.
// If it is a natural language title (e.g., "Phase 16"), it searches the local CAS for a matching title and returns the exact ID.
func (p *Processor) ResolveSemanticArgument(ctx context.Context, kind string, arg string) (string, error) {
	// If it is already a valid ZQK ID, return it directly.
	if validation.GetIDValidator().InferKindFromID(arg) != "" {
		return arg, nil
	}

	argLower := strings.ToLower(strings.TrimSpace(arg))

	// Search strategy: if kind is empty, we must list without a kind filter.
	// Note: Listing without a kind can be expensive in a very large CAS, but for Semantic CLI Routing
	// on human-scale arguments, it is an acceptable UX tradeoff. In production GraphRAG, this is indexed.
	// Setting a high Limit allows us to leverage the existing in-memory ListCache (which skips caching for Limit=0).
	filter := storage.ListFilter{Kind: kind, Limit: 10000}
	results, err := p.Storage().List(ctx, p.SecurityContext(), p.StorageContext(), filter)

	// If the backend strictly requires a kind and failed, and kind was empty, we can try to brute-force
	// the most common kinds for semantic routing (Priority Plans, Workstreams, Backlog Items).
	if err != nil && kind == "" {
		if results == nil {
			results = &storage.QueryResult{Objects: []map[string]any{}}
		}

		commonKinds := []string{
			objects.KindPriorityPlan,
			objects.KindWorkstream,
			objects.KindBacklogItem,
			objects.KindGoal,
			objects.KindRoadmap,
		}

		// Fallback to sequential scanning. Using Limit=10000 leverages the global list cache,
		// rendering concurrent fetching unnecessary and avoiding CAS semaphore contention.
		for _, ck := range commonKinds {
			filter.Kind = ck
			res, cerr := p.Storage().List(ctx, p.SecurityContext(), p.StorageContext(), filter)
			if cerr == nil && res != nil {
				results.Objects = append(results.Objects, res.Objects...)
			}
		}
	} else if err != nil {
		return "", err
	} // First pass: Exact match
	for _, obj := range results.Objects {
		title, ok := obj[objects.FieldKeyTitle].(string)
		if ok && strings.ToLower(strings.TrimSpace(title)) == argLower {
			if id, ok := obj[objects.FieldKeyID].(string); ok {
				return id, nil
			}
		}
	}

	// Second pass: Prefix/Contains match
	for _, obj := range results.Objects {
		title, ok := obj[objects.FieldKeyTitle].(string)
		if ok && strings.Contains(strings.ToLower(strings.TrimSpace(title)), argLower) {
			if id, ok := obj[objects.FieldKeyID].(string); ok {
				return id, nil
			}
		}
	}

	return "", fmt.Errorf("could not semantically resolve argument %q", arg)
}
