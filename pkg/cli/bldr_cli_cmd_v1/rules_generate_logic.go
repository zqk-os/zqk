package bldr_cli_cmd_v1

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TargetHost represents a supported IDE or agent host.
type TargetHost string

const (
	HostAll      TargetHost = "all"
	HostCursor   TargetHost = "cursor"
	HostClaude   TargetHost = "claude"
	HostWindsurf TargetHost = "windsurf"
	HostAgents   TargetHost = "agents"
)

// DirectiveFile specifies a directive file path and its generator.
type DirectiveFile struct {
	Host    TargetHost
	RelPath string
	Content string
}

// RulesGenerateOptions configures the directives generation process.
type RulesGenerateOptions struct {
	Hosts  []string
	DryRun bool
	Force  bool
}

// RulesGenerateResult contains the outcome of generating rules files.
type RulesGenerateResult struct {
	Written  []string          `json:"written"`
	Skipped  []string          `json:"skipped"`
	Previews map[string]string `json:"previews,omitempty"`
}

// DirectiveContent returns the syntax-valid, token-conserving directive for a host.
func DirectiveContent(host TargetHost) string {
	switch host {
	case HostCursor:
		return `# ZQK Cursor Rules

## Code Search & AST Navigation (Sub-15ms zgrep)
- **Prefer zgrep over ripgrep or full codebase scans:** zgrep provides sub-15ms trigram indexing and Go AST structural queries saving >90% context tokens for Cursor:
  - Find structs: ` + "`" + `zgrep --ast --kind struct <Name>` + "`" + `
  - Find functions/methods: ` + "`" + `zgrep --ast --kind func <Name>` + "`" + `
  - Find receiver methods: ` + "`" + `zgrep --ast --recv <TypeName>` + "`" + `
  - Strictly token-budgeted output: ` + "`" + `zgrep --max-tokens 2000 -f json <query>` + "`" + `
- Never dump entire package directories into chat context; use targeted zgrep queries.

## Persistent Knowledge Kernel Rules & Done-Gates
- **Contract-Driven Verification:** Do not mark tasks, criteria, or backlog items complete without verification.
  - Run verification: ` + "`" + `zqk test run <test-id>` + "`" + ` or ` + "`" + `zqk do <bli-id> --verify` + "`" + `
  - Whats-Next Discovery: ` + "`" + `./bin/zqk workflow whats-next` + "`" + `
- **Anti-Hallucination Protocol:** Query the Knowledge Kernel (` + "`" + `zqk object get <id>` + "`" + `, ` + "`" + `zqk query` + "`" + `, ` + "`" + `zqk grep` + "`" + `) before generating assumptions or references. If an entity does not exist in the Knowledge Kernel or git repository, do not invent it.
- **Process Data Plane Isolation:** Never manually edit ` + "`" + `.zqk/process/*.yaml` + "`" + `. All mutations must proceed through the branded CLI (` + "`" + `zqk object` + "`" + `, ` + "`" + `zqk new` + "`" + `, ` + "`" + `zqk mutate` + "`" + `).
`
	case HostClaude:
		return `# ZQK Claude Code Directives

## Code Search & AST Navigation (Sub-15ms zgrep)
- **Prefer zgrep over raw shell grep or find:** zgrep provides sub-15ms trigram indexing and Go AST structural queries saving >90% context tokens for Claude Code:
  - Find structs: ` + "`" + `zgrep --ast --kind struct <Name>` + "`" + `
  - Find functions/methods: ` + "`" + `zgrep --ast --kind func <Name>` + "`" + `
  - Find receiver methods: ` + "`" + `zgrep --ast --recv <TypeName>` + "`" + `
  - Token-budgeted output: ` + "`" + `zgrep --max-tokens 2000 -f json <query>` + "`" + `
- Never dump unbounded search results into context; always use targeted zgrep AST queries.

## Persistent Knowledge Kernel Rules & Done-Gates
- **Contract-Driven Verification:** Do not mark tasks, criteria, or backlog items complete without verification.
  - Run verification: ` + "`" + `zqk test run <test-id>` + "`" + ` or ` + "`" + `zqk do <bli-id> --verify` + "`" + `
  - Whats-Next Discovery: ` + "`" + `./bin/zqk workflow whats-next` + "`" + `
- **Anti-Hallucination Protocol:** Query the Knowledge Kernel (` + "`" + `zqk object get <id>` + "`" + `, ` + "`" + `zqk query` + "`" + `, ` + "`" + `zqk grep` + "`" + `) before generating assumptions or references. If an entity does not exist in the Knowledge Kernel or git repository, do not invent it.
- **Process Data Plane Isolation:** Never manually edit ` + "`" + `.zqk/process/*.yaml` + "`" + `. All mutations must proceed through the branded CLI (` + "`" + `zqk object` + "`" + `, ` + "`" + `zqk new` + "`" + `, ` + "`" + `zqk mutate` + "`" + `).
`
	case HostWindsurf:
		return `# ZQK Windsurf Directives

## Code Search & AST Navigation (Sub-15ms zgrep)
- **Prefer zgrep over raw search:** zgrep provides sub-15ms trigram indexing and Go AST structural queries saving >90% context tokens:
  - Find structs: ` + "`" + `zgrep --ast --kind struct <Name>` + "`" + `
  - Find functions/methods: ` + "`" + `zgrep --ast --kind func <Name>` + "`" + `
  - Find receiver methods: ` + "`" + `zgrep --ast --recv <TypeName>` + "`" + `
  - Token-budgeted output: ` + "`" + `zgrep --max-tokens 2000 -f json <query>` + "`" + `

## Persistent Knowledge Kernel Rules & Done-Gates
- **Contract-Driven Verification:** Do not mark tasks, criteria, or backlog items complete without verification.
  - Run verification: ` + "`" + `zqk test run <test-id>` + "`" + ` or ` + "`" + `zqk do <bli-id> --verify` + "`" + `
  - Whats-Next Discovery: ` + "`" + `./bin/zqk workflow whats-next` + "`" + `
- **Anti-Hallucination Protocol:** Query the Knowledge Kernel (` + "`" + `zqk object get <id>` + "`" + `, ` + "`" + `zqk query` + "`" + `, ` + "`" + `zqk grep` + "`" + `) before generating assumptions or references. If an entity does not exist in the Knowledge Kernel or git repository, do not invent it.
- **Process Data Plane Isolation:** Never manually edit ` + "`" + `.zqk/process/*.yaml` + "`" + `. All mutations must proceed through the branded CLI (` + "`" + `zqk object` + "`" + `, ` + "`" + `zqk new` + "`" + `, ` + "`" + `zqk mutate` + "`" + `).
`
	case HostAgents:
		return `# ZQK Universal Agent Directives

## Code Search & AST Navigation (Sub-15ms zgrep)
- **Prefer zgrep over raw shell grep or find:** zgrep provides sub-15ms trigram indexing and Go AST structural queries saving >90% context tokens:
  - Find structs: ` + "`" + `zgrep --ast --kind struct <Name>` + "`" + `
  - Find functions/methods: ` + "`" + `zgrep --ast --kind func <Name>` + "`" + `
  - Find receiver methods: ` + "`" + `zgrep --ast --recv <TypeName>` + "`" + `
  - Token-budgeted output: ` + "`" + `zgrep --max-tokens 2000 -f json <query>` + "`" + `
- Using external grep dumps unbudgeted files into LLM contexts and increases token consumption.

## Persistent Knowledge Kernel Rules & Done-Gates
- **Contract-Driven Verification:** Do not mark tasks, criteria, or backlog items complete without verification.
  - Run verification: ` + "`" + `zqk test run <test-id>` + "`" + ` or ` + "`" + `zqk do <bli-id> --verify` + "`" + `
  - Whats-Next Discovery: ` + "`" + `./bin/zqk workflow whats-next` + "`" + `
- **Anti-Hallucination Protocol:** Query the Knowledge Kernel (` + "`" + `zqk object get <id>` + "`" + `, ` + "`" + `zqk query` + "`" + `, ` + "`" + `zqk grep` + "`" + `) before generating assumptions or references. If an entity does not exist in the Knowledge Kernel or git repository, do not invent it.
- **Process Data Plane Isolation:** Never manually edit ` + "`" + `.zqk/process/*.yaml` + "`" + `. All mutations must proceed through the branded CLI (` + "`" + `zqk object` + "`" + `, ` + "`" + `zqk new` + "`" + `, ` + "`" + `zqk mutate` + "`" + `).
`
	default:
		return ""
	}
}

// KnownDirectives returns the canonical table of directive files per host.
func KnownDirectives() []DirectiveFile {
	return []DirectiveFile{
		{Host: HostCursor, RelPath: ".cursorrules", Content: DirectiveContent(HostCursor)},
		{Host: HostClaude, RelPath: "CLAUDE.md", Content: DirectiveContent(HostClaude)},
		{Host: HostWindsurf, RelPath: ".windsurfrules", Content: DirectiveContent(HostWindsurf)},
		{Host: HostAgents, RelPath: ".agents/AGENTS.md", Content: DirectiveContent(HostAgents)},
	}
}

// ResolveDirectives filters the directives table based on requested hosts.
func ResolveDirectives(hosts []string) []DirectiveFile {
	all := KnownDirectives()
	if len(hosts) == 0 {
		return all
	}
	wantsAll := false
	wanted := make(map[TargetHost]bool)
	for _, h := range hosts {
		parts := strings.Split(h, ",")
		for _, p := range parts {
			norm := strings.ToLower(strings.TrimSpace(p))
			if norm == "all" || norm == "" {
				wantsAll = true
			} else {
				wanted[TargetHost(norm)] = true
			}
		}
	}
	if wantsAll {
		return all
	}
	var filtered []DirectiveFile
	for _, d := range all {
		if wanted[d.Host] {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// GenerateRules writes or previews multi-host IDE directive files.
func GenerateRules(projectRoot string, opts RulesGenerateOptions) (*RulesGenerateResult, error) {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		root = paths.FindNearestProjectRoot(".")
		if root == "" {
			var err error
			root, err = os.Getwd()
			if err != nil {
				return nil, errfmt.Newf("resolve project root").Wrap(err)
			}
		}
	}

	targets := ResolveDirectives(opts.Hosts)
	res := &RulesGenerateResult{
		Written:  make([]string, 0, len(targets)),
		Skipped:  make([]string, 0, len(targets)),
		Previews: make(map[string]string),
	}

	for _, d := range targets {
		absPath := filepath.Join(root, filepath.FromSlash(d.RelPath))
		_, statErr := fileutil.Stat(absPath)
		exists := statErr == nil

		if opts.DryRun {
			res.Previews[d.RelPath] = d.Content
			if exists && !opts.Force {
				res.Skipped = append(res.Skipped, d.RelPath)
			} else {
				res.Written = append(res.Written, d.RelPath)
			}
			continue
		}

		if exists && !opts.Force {
			res.Skipped = append(res.Skipped, d.RelPath)
			continue
		}

		dir := filepath.Dir(absPath)
		if err := fileutil.EnsureDir(dir); err != nil {
			return nil, errfmt.Newf("ensure directory for %s", d.RelPath).Wrap(err)
		}

		if err := fileutil.WriteSecureFile(absPath, []byte(d.Content)); err != nil {
			return nil, errfmt.Newf("write directive file %s", d.RelPath).Wrap(err)
		}
		res.Written = append(res.Written, d.RelPath)
	}

	return res, nil
}
