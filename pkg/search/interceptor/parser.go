package interceptor

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Parsing syntax constants.
const (
	charPipe        = '|'
	charSingleQuote = '\''
	charDoubleQuote = '"'
	charBackslash   = '\\'
	charSpace       = ' '
	charTab         = '\t'
	charNewline     = '\n'
	charEqual       = '='
	charDash        = '-'
	charSlash       = '/'

	strDashDash   = "--"
	strStdinDash  = "-"
	strParentDir  = ".."
	strHomeEnvVar = "HOME"
	strGit        = "git"
	strGrep       = "grep"
)

// Flag string constants.
const (
	flagShortR           = "-r"
	flagShortBigR        = "-R"
	flagLongRecursive    = "--recursive"
	flagShortN           = "-n"
	flagLongLineNumber   = "--line-number"
	flagShortI           = "-i"
	flagShortY           = "-y"
	flagLongIgnoreCase   = "--ignore-case"
	flagShortW           = "-w"
	flagLongWordRegexp   = "--word-regexp"
	flagShortE           = "-e"
	flagShortBigE        = "-E"
	flagLongRegexp       = "--regexp"
	flagLongExtended     = "--extended-regexp"
	flagShortF           = "-F"
	flagLongFixedStrings = "--fixed-strings"
	flagShortC           = "-C"
	flagLongContext      = "--context"
	flagShortM           = "-m"
	flagLongMaxCount     = "--max-count"
	flagShortT           = "-t"
	flagLongType         = "--type"
	flagLongHidden       = "--hidden"
	flagLongAst          = "--ast"
	flagLongKind         = "--kind"
	flagLongRecv         = "--recv"
	flagLongMaxTokens    = "--max-tokens" //nolint:gosec // G101: CLI flag name, not credential
	flagLongFormat       = "--format"
	flagLongExt          = "--ext"
	flagExtGo            = ".go"
)

// AST intent pattern fragments.
const (
	astKeywordFunc           = "func "
	astKeywordFuncParen      = "func("
	astKeywordType           = "type "
	astKeywordStruct         = "struct"
	astKeywordInterface      = "interface"
	astKeywordStructOpen     = "struct{"
	astKeywordStructSpace    = "struct {"
	astKeywordInterfaceOpen  = "interface{"
	astKeywordInterfaceSpace = "interface {"
)

// Non-code file extensions and names.
var nonCodeExtensions = map[string]struct{}{
	".log":  {},
	".txt":  {},
	".csv":  {},
	".tsv":  {},
	".tmp":  {},
	".bak":  {},
	".swp":  {},
	".out":  {},
	".pid":  {},
	".sock": {},
	".zip":  {},
	".tar":  {},
	".gz":   {},
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".gif":  {},
	".ico":  {},
	".pdf":  {},
}

var nonCodeFilenames = map[string]struct{}{
	"hosts":       {},
	"passwd":      {},
	".DS_Store":   {},
	"resolv.conf": {},
}

var astRegexPattern = regexp.MustCompile(`(?i)(^|\b)(func(\s+|\.\*|\().*|\btype\s+\w+\s+(struct|interface)\b|\bstruct\s*\{|\binterface\s*\{)`)

// PipelineSegment represents a single pipeline stage in a shell command.
type PipelineSegment struct {
	Raw    string
	Tokens []string
}

// ParsedSearchCommand holds normalized flags, search query, and target paths.
type ParsedSearchCommand struct {
	Tool                string
	Pattern             string
	TargetPaths         []string
	IgnoreCase          bool
	WordRegexp          bool
	Regex               bool
	ContextLines        int
	MaxCount            int
	MaxTokens           int
	Format              string
	TypeExtension       string
	Hidden              bool
	ASTMode             bool
	ASTKind             string
	ASTReceiver         string
	HasStdinDash        bool
	NoPathsProvided     bool
	HasUnsupportedFlags bool
	UnsupportedFlag     string
}

type quoteState struct {
	inSingle bool
	inDouble bool
	escaped  bool
}

func (q *quoteState) isQuoted() bool {
	return q.inSingle || q.inDouble
}

// consumeEscapeOrQuote processes character c and returns true if c was consumed as an escape or quote transition.
func (q *quoteState) consumeEscapeOrQuote(c byte) (handled bool, isEscaped bool) {
	if q.escaped {
		q.escaped = false
		return true, true
	}
	if c == charBackslash && !q.inSingle {
		q.escaped = true
		return true, false
	}
	if c == charSingleQuote && !q.inDouble {
		q.inSingle = !q.inSingle
		return true, false
	}
	if c == charDoubleQuote && !q.inSingle {
		q.inDouble = !q.inDouble
		return true, false
	}
	return false, false
}

// splitPipelineSegments partitions a shell command line by unquoted POSIX pipe '|' operators.
func splitPipelineSegments(cmdLine string) ([]PipelineSegment, error) {
	trimmed := strings.TrimSpace(cmdLine)
	if trimmed == "" {
		return nil, nil
	}

	var segments []string
	var current strings.Builder
	qs := &quoteState{}
	n := len(trimmed)

	for i := 0; i < n; i++ {
		c := trimmed[i]
		if handled, _ := qs.consumeEscapeOrQuote(c); handled {
			current.WriteByte(c)
			continue
		}

		if c == charPipe && !qs.isQuoted() {
			// Check for logical OR '||'
			if i+1 < n && trimmed[i+1] == charPipe {
				current.WriteByte(c)
				current.WriteByte(trimmed[i+1])
				i++
				continue
			}
			segments = append(segments, current.String())
			current.Reset()
			continue
		}

		current.WriteByte(c)
	}

	if qs.isQuoted() {
		return nil, errfmt.Errorf("unclosed quote in command line: %s", cmdLine)
	}

	segments = append(segments, current.String())

	var result []PipelineSegment
	for _, seg := range segments {
		sTrim := strings.TrimSpace(seg)
		if sTrim == "" {
			continue
		}
		tokens, err := tokenizeShell(sTrim)
		if err != nil {
			return nil, err
		}
		result = append(result, PipelineSegment{
			Raw:    sTrim,
			Tokens: tokens,
		})
	}

	return result, nil
}

// tokenizeShell decomposes a single command segment into argv tokens.
func tokenizeShell(segment string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	qs := &quoteState{}
	inToken := false

	n := len(segment)
	for i := 0; i < n; i++ {
		c := segment[i]
		handled, isEscaped := qs.consumeEscapeOrQuote(c)
		if handled {
			if isEscaped {
				current.WriteByte(c)
				inToken = true
			} else if c != charBackslash {
				inToken = true
			}
			continue
		}
		if (c == charSpace || c == charTab || c == charNewline) && !qs.isQuoted() {
			if inToken {
				tokens = append(tokens, current.String())
				current.Reset()
				inToken = false
			}
			continue
		}
		current.WriteByte(c)
		inToken = true
	}

	if qs.isQuoted() {
		return nil, errfmt.Errorf("unclosed quote in segment: %s", segment)
	}
	if inToken {
		tokens = append(tokens, current.String())
	}
	return tokens, nil
}

// extractExecutable finds the command executable name and starting index after any env vars.
func extractExecutable(tokens []string) (string, int) {
	for i, tok := range tokens {
		if strings.ContainsRune(tok, charEqual) && !strings.HasPrefix(tok, strDashDash) && !strings.HasPrefix(tok, strStdinDash) {
			continue
		}
		base := filepath.Base(tok)
		if base == strGit && i+1 < len(tokens) && tokens[i+1] == strGrep {
			return ToolGitGrep, i
		}
		return base, i
	}
	return "", -1
}

// isSupportedSearchTool reports whether tool is one of grep, egrep, fgrep, rg, or git grep.
func isSupportedSearchTool(tool string) bool {
	switch tool {
	case ToolGrep, ToolEGrep, ToolFGrep, ToolRG, ToolRipgrep, ToolGitGrep:
		return true
	default:
		return false
	}
}

// parseSearchCommand inspects argv tokens starting from toolIndex to build ParsedSearchCommand.
func parseSearchCommand(tool string, tokens []string, toolIndex int) (*ParsedSearchCommand, error) {
	parsed := &ParsedSearchCommand{
		Tool: tool,
	}

	if tool == ToolEGrep {
		parsed.Regex = true
	}

	skip := 1
	if tool == ToolGitGrep {
		skip = 2
	}
	args := tokens[toolIndex+skip:]
	var positional []string
	n := len(args)

	for i := 0; i < n; i++ {
		arg := args[i]
		if arg == strDashDash {
			// All remaining args are positional
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, strDashDash) {
			i = parseLongFlag(arg, args, i, n, parsed)
			continue
		}
		if strings.HasPrefix(arg, strStdinDash) && len(arg) > 1 {
			i = parseShortFlagCluster(arg, args, i, n, parsed)
			continue
		}
		positional = append(positional, arg)
	}

	// Positional arguments resolution:
	// If pattern was not populated via -e / --regexp:
	if parsed.Pattern == "" {
		if len(positional) > 0 {
			parsed.Pattern = positional[0]
			for _, p := range positional[1:] {
				parsed.TargetPaths = append(parsed.TargetPaths, filepath.Clean(p))
			}
		}
	} else {
		for _, p := range positional {
			parsed.TargetPaths = append(parsed.TargetPaths, filepath.Clean(p))
		}
	}

	if len(parsed.TargetPaths) == 0 {
		parsed.NoPathsProvided = true
	} else {
		for _, p := range parsed.TargetPaths {
			if p == strStdinDash {
				parsed.HasStdinDash = true
				break
			}
		}
	}

	return parsed, nil
}

func extractLongFlagValue(flagValue string, hasEq bool, args []string, i, n int) (string, int) {
	if !hasEq && i+1 < n {
		return args[i+1], i + 1
	}
	return flagValue, i
}

func parseLongSearchFlags(flagName, flagValue string, hasEq bool, args []string, i, n int, parsed *ParsedSearchCommand) (int, bool) {
	switch flagName {
	case flagLongIgnoreCase:
		parsed.IgnoreCase = true
		return i, true
	case flagLongWordRegexp:
		parsed.WordRegexp = true
		return i, true
	case flagLongExtended:
		parsed.Regex = true
		return i, true
	case flagLongFixedStrings:
		parsed.Regex = false
		return i, true
	case flagLongHidden:
		parsed.Hidden = true
		return i, true
	case flagLongAst:
		parsed.ASTMode = true
		return i, true
	case flagLongRegexp:
		parsed.Regex = true
		val, nextI := extractLongFlagValue(flagValue, hasEq, args, i, n)
		parsed.Pattern = val
		return nextI, true
	case flagLongKind:
		val, nextI := extractLongFlagValue(flagValue, hasEq, args, i, n)
		parsed.ASTKind = val
		parsed.ASTMode = true
		return nextI, true
	case flagLongRecv:
		val, nextI := extractLongFlagValue(flagValue, hasEq, args, i, n)
		parsed.ASTReceiver = val
		parsed.ASTMode = true
		return nextI, true
	case flagLongRecursive, flagLongLineNumber:
		// Supported standard grep flags, handled implicitly by zqk grep
		return i, true
	default:
		return i, false
	}
}

func parseLongFilterFlags(flagName, flagValue string, hasEq bool, args []string, i, n int, parsed *ParsedSearchCommand) (int, bool) {
	switch flagName {
	case flagLongContext:
		val, nextI := extractLongFlagValue(flagValue, hasEq, args, i, n)
		if c, err := strconv.Atoi(val); err == nil {
			parsed.ContextLines = c
		}
		return nextI, true
	case flagLongMaxCount:
		val, nextI := extractLongFlagValue(flagValue, hasEq, args, i, n)
		if m, err := strconv.Atoi(val); err == nil {
			parsed.MaxCount = m
		}
		return nextI, true
	case flagLongType, flagLongExt:
		val, nextI := extractLongFlagValue(flagValue, hasEq, args, i, n)
		parsed.TypeExtension = val
		return nextI, true
	case flagLongMaxTokens:
		val, nextI := extractLongFlagValue(flagValue, hasEq, args, i, n)
		if t, err := strconv.Atoi(val); err == nil {
			parsed.MaxTokens = t
		}
		return nextI, true
	case flagLongFormat:
		val, nextI := extractLongFlagValue(flagValue, hasEq, args, i, n)
		parsed.Format = val
		return nextI, true
	default:
		return i, false
	}
}

func parseLongFlag(arg string, args []string, i, n int, parsed *ParsedSearchCommand) int {
	flagName := arg
	flagValue := ""
	hasEq := false
	if eqIdx := strings.IndexRune(arg, charEqual); eqIdx != -1 {
		flagName = arg[:eqIdx]
		flagValue = arg[eqIdx+1:]
		hasEq = true
	}

	if nextI, ok := parseLongSearchFlags(flagName, flagValue, hasEq, args, i, n, parsed); ok {
		return nextI
	}
	if nextI, ok := parseLongFilterFlags(flagName, flagValue, hasEq, args, i, n, parsed); ok {
		return nextI
	}

	parsed.HasUnsupportedFlags = true
	parsed.UnsupportedFlag = flagName
	return i
}

func parseShortFlagCluster(arg string, args []string, i, n int, parsed *ParsedSearchCommand) int {
	cluster := arg[1:]
	for j := 0; j < len(cluster); j++ {
		ch := cluster[j]
		switch ch {
		case 'i', 'y':
			parsed.IgnoreCase = true
		case 'w':
			parsed.WordRegexp = true
		case 'E':
			parsed.Regex = true
		case 'F':
			parsed.Regex = false
		case 'r', 'R', 'n':
			// Supported standard grep recursive/line flags, handled implicitly by zqk grep
		case 'e':
			parsed.Regex = true
			if j+1 < len(cluster) {
				parsed.Pattern = cluster[j+1:]
				return i
			} else if i+1 < n {
				i++
				parsed.Pattern = args[i]
				return i
			}
		case 'f':
			valStr := ""
			if j+1 < len(cluster) {
				valStr = cluster[j+1:]
				parsed.Format = valStr
				return i
			} else if i+1 < n {
				i++
				parsed.Format = args[i]
				return i
			}
		case 'C':
			valStr := ""
			if j+1 < len(cluster) {
				valStr = cluster[j+1:]
				if c, err := strconv.Atoi(valStr); err == nil {
					parsed.ContextLines = c
				}
				return i
			} else if i+1 < n {
				i++
				valStr = args[i]
				if c, err := strconv.Atoi(valStr); err == nil {
					parsed.ContextLines = c
				}
				return i
			}
		case 'm':
			valStr := ""
			if j+1 < len(cluster) {
				valStr = cluster[j+1:]
				if m, err := strconv.Atoi(valStr); err == nil {
					parsed.MaxCount = m
				}
				return i
			} else if i+1 < n {
				i++
				valStr = args[i]
				if m, err := strconv.Atoi(valStr); err == nil {
					parsed.MaxCount = m
				}
				return i
			}
		case 't':
			if j+1 < len(cluster) {
				parsed.TypeExtension = cluster[j+1:]
				return i
			} else if i+1 < n {
				i++
				parsed.TypeExtension = args[i]
				return i
			}
		default:
			parsed.HasUnsupportedFlags = true
			parsed.UnsupportedFlag = string(ch)
		}
	}
	return i
}

// isNonIndexedPath determines if targetPath targets directories or files excluded from repository indexing (e.g. .git/).
func isNonIndexedPath(targetPath, cwd, projectRoot string) bool {
	if targetPath == "" || targetPath == DotPath {
		return false
	}
	clean := filepath.Clean(targetPath)
	slashPath := filepath.ToSlash(clean)
	parts := strings.Split(slashPath, "/")
	for _, part := range parts {
		if part == ".git" || part == ".gemini" || part == ".cursor" || part == ".claude" || part == ".idea" || part == ".vscode" || part == ".cache" {
			return true
		}
	}
	return false
}

// isExternalPath determines if targetPath resides outside the projectRoot directory.
func isExternalPath(targetPath, cwd, projectRoot string, env map[string]string) bool {
	if targetPath == "" || targetPath == DotPath {
		return false
	}

	cleanTarget := targetPath

	// Handle ~/ and ~
	if strings.HasPrefix(cleanTarget, TildePrefix) || cleanTarget == TildeOnly {
		homeDir := ""
		if env != nil {
			homeDir = env[strHomeEnvVar]
		}
		if homeDir == "" {
			if h, err := fileutil.UserHomeDir(); err == nil {
				homeDir = h
			}
		}
		if homeDir != "" {
			if cleanTarget == TildeOnly {
				cleanTarget = homeDir
			} else {
				cleanTarget = filepath.Join(homeDir, cleanTarget[2:])
			}
		} else {
			return true
		}
	}

	absProjectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return true
	}
	absProjectRoot = filepath.Clean(absProjectRoot)

	absTarget := cleanTarget
	if !filepath.IsAbs(absTarget) {
		absCwd := cwd
		if absCwd == "" {
			absCwd = projectRoot
		}
		c, err := filepath.Abs(absCwd)
		if err != nil {
			return true
		}
		absTarget = filepath.Join(c, absTarget)
	}
	absTarget = filepath.Clean(absTarget)

	rel, err := filepath.Rel(absProjectRoot, absTarget)
	if err != nil {
		return true
	}
	if rel == strParentDir || strings.HasPrefix(rel, strParentDir+string(filepath.Separator)) {
		return true
	}

	return false
}

// isNonCodeTarget checks if targetPath points to a single non-code file.
func isNonCodeTarget(targetPath, cwd string) bool {
	if targetPath == "" || targetPath == DotPath || strings.HasSuffix(targetPath, string(charSlash)) {
		return false
	}

	base := filepath.Base(targetPath)
	if _, ok := nonCodeFilenames[base]; ok {
		return true
	}

	ext := strings.ToLower(filepath.Ext(targetPath))
	if _, ok := nonCodeExtensions[ext]; ok {
		return true
	}

	fullPath := targetPath
	if !filepath.IsAbs(fullPath) && cwd != "" {
		fullPath = filepath.Join(cwd, targetPath)
	}

	fi, err := fileutil.Stat(fullPath)
	if err == nil {
		if fi.IsDir() {
			return false
		}
		if ext == "" {
			return false
		}
	}

	return false
}

// isASTIntent reports whether the query pattern indicates Go AST structural intent.
func isASTIntent(cmd *ParsedSearchCommand) bool {
	if cmd.ASTMode || cmd.ASTKind != "" || cmd.ASTReceiver != "" {
		return true
	}
	p := cmd.Pattern
	if p == "" {
		return false
	}

	if strings.Contains(p, astKeywordFunc) || strings.Contains(p, astKeywordFuncParen) {
		return true
	}
	if strings.Contains(p, astKeywordType) && (strings.Contains(p, astKeywordStruct) || strings.Contains(p, astKeywordInterface)) {
		return true
	}
	if strings.Contains(p, astKeywordStructSpace) || strings.Contains(p, astKeywordStructOpen) {
		return true
	}
	if strings.Contains(p, astKeywordInterfaceSpace) || strings.Contains(p, astKeywordInterfaceOpen) {
		return true
	}

	return astRegexPattern.MatchString(p)
}
