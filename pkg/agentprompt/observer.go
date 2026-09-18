package agentprompt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/utils/sortutil"
)

const (
	observerHintTimeout = 8 * time.Second
	observerHintLimit   = 12
)

// ObserverContext represents the AST and structural graph context for an agent.
type ObserverContext struct {
	Result *observer.ExtractResult
}

// ExtractObserverContext extracts the structural AST graph from the given project directory.
func ExtractObserverContext(ctx context.Context, dir string) (*ObserverContext, error) {
	res, err := observer.ExtractFromDir(ctx, nil, dir, []observer.Extractor{
		observer.GoExtractor{},
	})
	if err != nil {
		return nil, err
	}

	return &ObserverContext{
		Result: res,
	}, nil
}

func observerToolPrefix() string {
	return brand.NamespacePrefix() + "_"
}

// GeneratePromptSection is the cheap always-on pointer: live tools, no census.
func (o *ObserverContext) GeneratePromptSection() string {
	return ObserverToolAccessSection()
}

// ObserverToolAccessSection tells execute-layer agents how to query live structure.
func ObserverToolAccessSection() string {
	p := observerToolPrefix()
	return "## Codebase Structural Context\n" +
		"Do not persist an AST census on the task object. Before inventing a file or package, call `" +
		p + "observer_search` with name= (symbol) and/or path= (pkg/... or cmd/...). Then `" +
		p + "read_code` the returned path. Never invent src/ or a new package.\n"
}

// RelevantObserverSection is a live, task-scoped AST hint plus tool access.
// It must never be persisted on agent_task.
func RelevantObserverSection(ctx context.Context, projectRoot, title, extra string) string {
	var sb strings.Builder
	sb.WriteString(ObserverToolAccessSection())
	if strings.TrimSpace(projectRoot) == "" {
		return sb.String()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	queryCtx, cancel := context.WithTimeout(ctx, observerHintTimeout)
	defer cancel()

	seen := make(map[string]struct{})
	var hits []observer.Entity
	path := observer.InferSourcePath(title + " " + extra)
	tokens := observer.SearchTokens(title + " " + extra)
	if path != "" {
		found, err := observer.Search(queryCtx, observer.Query{
			Root: projectRoot, Path: path, Limit: observerHintLimit,
		})
		if err == nil {
			hits = appendUniqueHits(hits, found, seen)
		}
	}
	for _, tok := range tokens {
		if queryCtx.Err() != nil || len(hits) >= observerHintLimit {
			break
		}
		found, err := observer.Search(queryCtx, observer.Query{
			Root: projectRoot, Name: tok, Path: path, Limit: observerHintLimit - len(hits),
		})
		if err != nil {
			continue
		}
		hits = appendUniqueHits(hits, found, seen)
	}
	if len(hits) == 0 {
		return sb.String()
	}
	sb.WriteString("\n### Task-relevant AST hits (live, not persisted)\n")
	for _, hit := range hits {
		sb.WriteString(observer.FormatHit(hit))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func appendUniqueHits(dst []observer.Entity, src []observer.Entity, seen map[string]struct{}) []observer.Entity {
	for _, e := range src {
		key := e.File + ":" + e.Name + ":" + e.Kind
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dst = append(dst, e)
		if len(dst) >= observerHintLimit {
			break
		}
	}
	return dst
}

// GeneratePromptSectionCensus inlines the top-complexity AST dump. Do not persist this on agent_task.
func (o *ObserverContext) GeneratePromptSectionCensus() string {
	if o.Result == nil || len(o.Result.Entities) == 0 {
		return "## Codebase Structural Context\nNo significant AST structures discovered.\n"
	}

	var sb strings.Builder
	sb.WriteString("## Codebase Structural Context (Top 30 Highest Complexity Entities)\n")
	sb.WriteString("The following high-complexity structural components (AST) were discovered in the codebase to assist your operations:\n\n")

	entities := make([]observer.Entity, len(o.Result.Entities))
	copy(entities, o.Result.Entities)

	sortutil.BubbleSort(entities, func(a, b observer.Entity) bool {
		return a.Complexity > b.Complexity
	})

	limit := 30
	if len(entities) < limit {
		limit = len(entities)
	}
	topEntities := entities[:limit]

	grouped := make(map[string][]observer.Entity)
	for _, entity := range topEntities {
		grouped[entity.File] = append(grouped[entity.File], entity)
	}

	var files []string
	for file := range grouped {
		files = append(files, file)
	}
	sortutil.BubbleSort(files, func(a, b string) bool {
		return a < b
	})

	for _, file := range files {
		fileEntities := grouped[file]
		sb.WriteString(fmt.Sprintf("### File: `%s`\n", file))
		for _, entity := range fileEntities {
			complexityStr := ""
			if entity.Complexity > 0 {
				complexityStr = fmt.Sprintf(" (Complexity: %d)", entity.Complexity)
				if entity.Complexity > 10 {
					complexityStr += " ⚠️ HIGH COMPLEXITY"
				}
			}
			sb.WriteString(fmt.Sprintf("- **%s** (`%s`): `%s` %s\n", entity.Name, entity.Kind, entity.Signature, complexityStr))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
