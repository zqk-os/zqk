package interceptor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/pipeline"
)

// PipelineState flows through each stage of the search interception pipeline.
type PipelineState struct {
	Req                InterceptRequest
	Result             InterceptResult
	Halt               bool
	Segments           []PipelineSegment
	SearchSegmentIndex int
	ParsedCmd          *ParsedSearchCommand
}

// stageFunnelFilter filters incoming commands to only target grep, egrep, fgrep, rg, or git grep.
func stageFunnelFilter(_ *pipeline.Context, payload any) (any, error) {
	state, ok := payload.(*PipelineState)
	if !ok {
		return nil, errfmt.Errorf("funnel_filter: invalid payload type %T", payload)
	}
	if state.Halt {
		return state, nil
	}

	cmdLine := strings.TrimSpace(state.Req.CommandLine)
	if cmdLine == "" {
		state.Result.Intercepted = false
		state.Result.Rewritten = false
		state.Result.RewrittenCommand = ""
		state.Result.Replacement = ""
		state.Result.Reason = ReasonEmptyCommand
		state.Result.Classification = ClassificationBypass
		state.Halt = true
		return state, nil
	}

	segments, err := splitPipelineSegments(cmdLine)
	if err != nil {
		state.Result.Intercepted = false
		state.Result.Rewritten = false
		state.Result.RewrittenCommand = state.Req.CommandLine
		state.Result.Replacement = ""
		state.Result.Reason = ReasonNotTargetSearchTool
		state.Result.Classification = ClassificationBypass
		state.Halt = true
		return state, nil
	}

	foundTool := false
	for i, seg := range segments {
		exe, idx := extractExecutable(seg.Tokens)
		if isSupportedSearchTool(exe) {
			parsed, parseErr := parseSearchCommand(exe, seg.Tokens, idx)
			if parseErr != nil {
				continue
			}
			state.SearchSegmentIndex = i
			state.Segments = segments
			state.ParsedCmd = parsed
			foundTool = true
			break
		}
	}

	if !foundTool {
		state.Result.Intercepted = false
		state.Result.Rewritten = false
		state.Result.RewrittenCommand = state.Req.CommandLine
		state.Result.Replacement = ""
		state.Result.Reason = ReasonNotTargetSearchTool
		state.Result.Classification = ClassificationBypass
		state.Halt = true
		return state, nil
	}

	return state, nil
}

// stageStreamFilterDetector detects POSIX pipes where grep reads from stdin / acts as a stream filter.
func stageStreamFilterDetector(_ *pipeline.Context, payload any) (any, error) {
	state, ok := payload.(*PipelineState)
	if !ok {
		return nil, errfmt.Errorf("stream_filter_detector: invalid payload type %T", payload)
	}
	if state.Halt {
		return state, nil
	}

	// 1. If the search command is preceded by a pipe, it is reading from stdin.
	isStream := state.SearchSegmentIndex > 0

	// 2. If "-" is given as a target path, it explicitly reads stdin.
	if !isStream && state.ParsedCmd != nil && state.ParsedCmd.HasStdinDash {
		isStream = true
	}

	// 3. If standard grep without paths and without recursive flags is invoked, it reads stdin.
	if !isStream && state.ParsedCmd != nil && state.ParsedCmd.NoPathsProvided {
		switch state.ParsedCmd.Tool {
		case ToolGrep, ToolEGrep, ToolFGrep:
			isStream = true
		}
	}

	// 4. If grep's output is piped to another command (e.g. grep foo . | sort)
	if !isStream && len(state.Segments) > 1 {
		isStream = true
	}

	if isStream {
		state.Result.Intercepted = false
		state.Result.Rewritten = false
		state.Result.RewrittenCommand = state.Req.CommandLine
		state.Result.Replacement = ""
		state.Result.Reason = ReasonStreamFilter
		state.Result.Classification = ClassificationBypass
		state.Halt = true
		return state, nil
	}

	return state, nil
}

// stagePathBoundaryDetector rejects commands searching paths outside repo root or single non-code files.
func stagePathBoundaryDetector(_ *pipeline.Context, payload any) (any, error) {
	state, ok := payload.(*PipelineState)
	if !ok {
		return nil, errfmt.Errorf("path_boundary_detector: invalid payload type %T", payload)
	}
	if state.Halt {
		return state, nil
	}

	projectRoot := state.Req.ProjectRoot
	if projectRoot == "" {
		projectRoot = state.Req.Cwd
	}
	if projectRoot == "" {
		projectRoot = DotPath
	}

	cwd := state.Req.Cwd
	if cwd == "" {
		cwd = projectRoot
	}

	if state.ParsedCmd != nil && len(state.ParsedCmd.TargetPaths) > 0 {
		for _, tp := range state.ParsedCmd.TargetPaths {
			if isExternalPath(tp, cwd, projectRoot, state.Req.Env) {
				state.Result.Intercepted = false
				state.Result.Rewritten = false
				state.Result.RewrittenCommand = state.Req.CommandLine
				state.Result.Replacement = ""
				state.Result.Reason = ReasonExternalPath
				state.Result.Classification = ClassificationBypass
				state.Halt = true
				return state, nil
			}
		}

		if len(state.ParsedCmd.TargetPaths) == 1 {
			tp := state.ParsedCmd.TargetPaths[0]
			if isNonCodeTarget(tp, cwd) {
				state.Result.Intercepted = false
				state.Result.Rewritten = false
				state.Result.RewrittenCommand = state.Req.CommandLine
				state.Result.Replacement = ""
				state.Result.Reason = ReasonNonCodeFile
				state.Result.Classification = ClassificationBypass
				state.Halt = true
				return state, nil
			}
		}
	}

	return state, nil
}

// stageIntentClassifier classifies the search intent into Go AST structural vs raw text.
func stageIntentClassifier(_ *pipeline.Context, payload any) (any, error) {
	state, ok := payload.(*PipelineState)
	if !ok {
		return nil, errfmt.Errorf("intent_classifier: invalid payload type %T", payload)
	}
	if state.Halt {
		return state, nil
	}

	if state.ParsedCmd != nil && isASTIntent(state.ParsedCmd) {
		state.Result.Classification = ClassificationAST
	} else {
		state.Result.Classification = ClassificationText
	}

	return state, nil
}

// stageRewriteTransformer constructs the rewritten zqk grep command.
func stageRewriteTransformer(opts Options) pipeline.StageFunc {
	return func(pctx *pipeline.Context, payload any) (any, error) {
		state, ok := payload.(*PipelineState)
		if !ok {
			return nil, errfmt.Errorf("rewrite_transformer: invalid payload type %T", payload)
		}
		if state.Halt {
			return state, nil
		}

		bin := opts.BinaryName
		if bin == "" {
			bin = DefaultCLICommand
		}

		parts := []string{bin, GrepSubcommand}

		if state.ParsedCmd != nil {
			if state.ParsedCmd.IgnoreCase {
				parts = append(parts, flagShortI)
			}
			if state.ParsedCmd.WordRegexp {
				parts = append(parts, flagShortW)
			}
			if state.ParsedCmd.Regex {
				parts = append(parts, flagShortE)
			}
			if state.ParsedCmd.ContextLines > 0 {
				parts = append(parts, flagShortC, strconv.Itoa(state.ParsedCmd.ContextLines))
			}
			if state.ParsedCmd.MaxCount > 0 {
				parts = append(parts, flagShortM, strconv.Itoa(state.ParsedCmd.MaxCount))
			}
			if state.ParsedCmd.TypeExtension != "" {
				ext := state.ParsedCmd.TypeExtension
				if !strings.HasPrefix(ext, ".") {
					ext = "." + ext
				}
				parts = append(parts, flagLongExt, ext)
			}
			if state.ParsedCmd.Hidden {
				parts = append(parts, flagLongHidden)
			}
			if state.ParsedCmd.ASTKind != "" {
				parts = append(parts, flagLongKind, state.ParsedCmd.ASTKind)
			}
			if state.ParsedCmd.ASTReceiver != "" {
				parts = append(parts, flagLongRecv, state.ParsedCmd.ASTReceiver)
			}
		}

		if opts.MaxTokens > 0 {
			parts = append(parts, flagLongMaxTokens, strconv.Itoa(opts.MaxTokens))
		}

		pattern := ""
		if state.ParsedCmd != nil {
			pattern = state.ParsedCmd.Pattern
		}
		parts = append(parts, formatPattern(pattern))

		if state.ParsedCmd != nil && len(state.ParsedCmd.TargetPaths) > 0 {
			for _, tp := range state.ParsedCmd.TargetPaths {
				if tp == DotPath || tp == DotPath+string(charSlash) {
					continue
				}
				parts = append(parts, tp)
			}
		}

		rewritten := strings.Join(parts, string(charSpace))
		state.Result.Intercepted = true
		state.Result.Rewritten = true
		state.Result.RewrittenCommand = rewritten
		state.Result.Replacement = rewritten
		state.Result.Reason = ReasonRewritten

		if pctx != nil && pctx.Outcome != nil {
			pctx.Outcome[OutcomeKeyIntercepted] = state.Result.Intercepted
			pctx.Outcome[OutcomeKeyRewrittenCommand] = state.Result.RewrittenCommand
			pctx.Outcome[OutcomeKeyReason] = state.Result.Reason
			pctx.Outcome[OutcomeKeyClassification] = state.Result.Classification
		}

		return state, nil
	}
}

// formatPattern quotes the pattern safely.
func formatPattern(pattern string) string {
	return fmt.Sprintf("%q", pattern)
}
