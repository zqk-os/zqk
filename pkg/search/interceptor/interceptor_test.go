package interceptor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/pipeline"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestIntercept_PositiveCases(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "pkg")
	if err := fileutil.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("failed to create pkg dir: %v", err)
	}
	cmdDir := filepath.Join(tmpDir, "cmd")
	if err := fileutil.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatalf("failed to create cmd dir: %v", err)
	}

	tests := []struct {
		name        string
		command     string
		cwd         string
		expectedCmd string
		astClass    bool
	}{
		{
			name:        "grep -r 'pattern' .",
			command:     `grep -r "pattern" .`,
			expectedCmd: `zqk grep "pattern"`,
		},
		{
			name:        "grep -rn 'pattern' pkg/",
			command:     `grep -rn "pattern" pkg/`,
			expectedCmd: `zqk grep "pattern" pkg`,
		},
		{
			name:        "rg 'pattern'",
			command:     `rg "pattern"`,
			expectedCmd: `zqk grep "pattern"`,
		},
		{
			name:        "grep -rnwi 'bar' pkg/",
			command:     `grep -rnwi "bar" pkg/`,
			expectedCmd: `zqk grep -i -w "bar" pkg`,
		},
		{
			name:        "git grep 'handler'",
			command:     `git grep 'handler'`,
			expectedCmd: `zqk grep "handler"`,
		},
		{
			name:        "rg -i 'error_timeout' cmd/",
			command:     `rg -i 'error_timeout' cmd/`,
			expectedCmd: `zqk grep -i "error_timeout" cmd`,
		},
		{
			name:        "grep with -e regex",
			command:     `grep -e "func.*Start" pkg/`,
			expectedCmd: `zqk grep -e "func.*Start" pkg`,
			astClass:    true,
		},
		{
			name:        "grep with -C context lines",
			command:     `grep -C 3 "pattern" .`,
			expectedCmd: `zqk grep -C 3 "pattern"`,
		},
		{
			name:        "grep with -m max count",
			command:     `grep -m 10 "pattern" .`,
			expectedCmd: `zqk grep -m 10 "pattern"`,
		},
		{
			name:        "rg with -t go",
			command:     `rg -t go "pattern"`,
			expectedCmd: `zqk grep --ext .go "pattern"`,
		},
		{
			name:        "rg with --hidden",
			command:     `rg --hidden "pattern"`,
			expectedCmd: `zqk grep --hidden "pattern"`,
		},
		{
			name:        "egrep pattern",
			command:     `egrep "foo|bar" .`,
			expectedCmd: `zqk grep -e "foo|bar"`,
		},
		{
			name:        "fgrep literal",
			command:     `fgrep "literal string" .`,
			expectedCmd: `zqk grep "literal string"`,
		},
		{
			name:        "grep with env var prefix",
			command:     `LC_ALL=C grep -r "pattern" .`,
			expectedCmd: `zqk grep "pattern"`,
		},
		{
			name:        "grep with -- long flags",
			command:     `grep --recursive --ignore-case --word-regexp "target" .`,
			expectedCmd: `zqk grep -i -w "target"`,
		},
		{
			name:        "grep with double dash separator",
			command:     `grep -r -- "-my-pattern" .`,
			expectedCmd: `zqk grep "-my-pattern"`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := Intercept(context.Background(), InterceptRequest{
				CommandLine: tt.command,
				Cwd:         tmpDir,
				ProjectRoot: tmpDir,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Intercepted {
				t.Fatalf("expected command to be intercepted, reason: %s", res.Reason)
			}
			if !res.Rewritten {
				t.Fatalf("expected Rewritten alias to be true")
			}
			if res.RewrittenCommand != tt.expectedCmd {
				t.Errorf("expected RewrittenCommand %q, got %q", tt.expectedCmd, res.RewrittenCommand)
			}
			if res.Replacement != tt.expectedCmd {
				t.Errorf("expected Replacement alias %q, got %q", tt.expectedCmd, res.Replacement)
			}
			if res.Reason != ReasonRewritten {
				t.Errorf("expected reason %q, got %q", ReasonRewritten, res.Reason)
			}
			expectedClass := ClassificationText
			if tt.astClass {
				expectedClass = ClassificationAST
			}
			if res.Classification != expectedClass {
				t.Errorf("expected classification %q, got %q", expectedClass, res.Classification)
			}
		})
	}
}

func TestIntercept_StreamPipelineNegatives(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	negatives := []struct {
		name    string
		command string
	}{
		{
			name:    "ps aux pipe grep",
			command: "ps aux | grep foo",
		},
		{
			name:    "cat file pipe grep",
			command: "cat foo.txt | grep bar",
		},
		{
			name:    "zqk object list pipe grep",
			command: "zqk object list | grep ...",
		},
		{
			name:    "grep explicitly reading stdin with dash",
			command: "grep foo -",
		},
		{
			name:    "standard non-recursive grep with no files",
			command: "grep foo",
		},
		{
			name:    "grep pipe to another command",
			command: "grep foo . | sort",
		},
	}

	for _, tt := range negatives {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := Intercept(context.Background(), InterceptRequest{
				CommandLine: tt.command,
				Cwd:         tmpDir,
				ProjectRoot: tmpDir,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Intercepted {
				t.Fatalf("expected command NOT to be intercepted, got: %s", res.RewrittenCommand)
			}
			if res.Rewritten {
				t.Fatalf("expected Rewritten alias to be false")
			}
			if res.RewrittenCommand != tt.command {
				t.Errorf("expected untouched command %q, got %q", tt.command, res.RewrittenCommand)
			}
			if res.Reason != ReasonStreamFilter {
				t.Errorf("expected reason %q, got %q", ReasonStreamFilter, res.Reason)
			}
			if res.Classification != ClassificationBypass {
				t.Errorf("expected classification %q, got %q", ClassificationBypass, res.Classification)
			}
		})
	}
}

func TestIntercept_ExternalPathNegatives(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	negatives := []struct {
		name    string
		command string
		env     map[string]string
	}{
		{
			name:    "grep foo in /tmp/test.log",
			command: "grep foo /tmp/test.log",
		},
		{
			name:    "grep foo in /etc/hosts",
			command: "grep foo /etc/hosts",
		},
		{
			name:    "grep foo in ~/.config/foo.yaml",
			command: "grep foo ~/.config/foo.yaml",
			env:     map[string]string{"HOME": "/custom/home"},
		},
		{
			name:    "grep foo in home directory directly",
			command: "grep foo ~",
			env:     map[string]string{"HOME": "/custom/home"},
		},
		{
			name:    "grep foo parent traversal outside root",
			command: "grep foo ../outside/repo",
		},
	}

	for _, tt := range negatives {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := Intercept(context.Background(), InterceptRequest{
				CommandLine: tt.command,
				Cwd:         tmpDir,
				ProjectRoot: tmpDir,
				Env:         tt.env,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Intercepted {
				t.Fatalf("expected command NOT to be intercepted, got: %s", res.RewrittenCommand)
			}
			if res.Reason != ReasonExternalPath {
				t.Errorf("expected reason %q, got %q", ReasonExternalPath, res.Reason)
			}
		})
	}
}

func TestIntercept_NonCodeFileNegatives(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	negatives := []struct {
		name    string
		command string
	}{
		{
			name:    "grep in single log file",
			command: "grep foo test.log",
		},
		{
			name:    "grep in single csv file",
			command: "grep foo data.csv",
		},
		{
			name:    "grep in single tsv file",
			command: "grep foo metrics.tsv",
		},
		{
			name:    "grep in single out file",
			command: "grep foo run.out",
		},
		{
			name:    "grep in single hosts file",
			command: "grep foo hosts",
		},
	}

	for _, tt := range negatives {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := Intercept(context.Background(), InterceptRequest{
				CommandLine: tt.command,
				Cwd:         tmpDir,
				ProjectRoot: tmpDir,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Intercepted {
				t.Fatalf("expected non-code file search NOT to be intercepted, got: %s", res.RewrittenCommand)
			}
			if res.Reason != ReasonNonCodeFile {
				t.Errorf("expected reason %q, got %q", ReasonNonCodeFile, res.Reason)
			}
		})
	}
}

func TestIntercept_NonSearchAndEmptyCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		command        string
		expectedReason string
	}{
		{
			name:           "empty command line",
			command:        "",
			expectedReason: ReasonEmptyCommand,
		},
		{
			name:           "whitespace command line",
			command:        "   \t\n",
			expectedReason: ReasonEmptyCommand,
		},
		{
			name:           "ls command",
			command:        "ls -la",
			expectedReason: ReasonNotTargetSearchTool,
		},
		{
			name:           "find command",
			command:        `find . -name "*.go"`,
			expectedReason: ReasonNotTargetSearchTool,
		},
		{
			name:           "go test command",
			command:        "go test -v ./...",
			expectedReason: ReasonNotTargetSearchTool,
		},
		{
			name:           "git status command",
			command:        "git status",
			expectedReason: ReasonNotTargetSearchTool,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := Intercept(context.Background(), InterceptRequest{
				CommandLine: tt.command,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Intercepted {
				t.Fatalf("expected command NOT to be intercepted, got: %s", res.RewrittenCommand)
			}
			if res.Reason != tt.expectedReason {
				t.Errorf("expected reason %q, got %q", tt.expectedReason, res.Reason)
			}
		})
	}
}

func TestIntercept_IntentClassification(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		command        string
		expectedClass  string
		expectedCmdSub string
	}{
		{
			name:          "func keyword pattern",
			command:       `grep "func StartWorker" .`,
			expectedClass: ClassificationAST,
		},
		{
			name:          "func paren pattern",
			command:       `grep "func(" .`,
			expectedClass: ClassificationAST,
		},
		{
			name:          "type struct pattern",
			command:       `grep "type Engine struct" .`,
			expectedClass: ClassificationAST,
		},
		{
			name:          "type interface pattern",
			command:       `grep "type Store interface" .`,
			expectedClass: ClassificationAST,
		},
		{
			name:          "struct open pattern",
			command:       `grep "struct {" .`,
			expectedClass: ClassificationAST,
		},
		{
			name:          "interface open pattern",
			command:       `grep "interface {" .`,
			expectedClass: ClassificationAST,
		},
		{
			name:           "--kind flag",
			command:        `grep --kind struct "Engine" .`,
			expectedClass:  ClassificationAST,
			expectedCmdSub: "--kind struct",
		},
		{
			name:           "--recv flag",
			command:        `grep --recv Engine "Start" .`,
			expectedClass:  ClassificationAST,
			expectedCmdSub: "--recv Engine",
		},
		{
			name:          "plain text literal",
			command:       `grep "some random string" .`,
			expectedClass: ClassificationText,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := Intercept(context.Background(), InterceptRequest{
				CommandLine: tt.command,
				Cwd:         tmpDir,
				ProjectRoot: tmpDir,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Intercepted {
				t.Fatalf("expected command to be intercepted, got false, reason: %s", res.Reason)
			}
			if res.Classification != tt.expectedClass {
				t.Errorf("expected classification %q, got %q", tt.expectedClass, res.Classification)
			}
			if tt.expectedCmdSub != "" && !strings.Contains(res.RewrittenCommand, tt.expectedCmdSub) {
				t.Errorf("expected rewritten command to contain %q, got %q", tt.expectedCmdSub, res.RewrittenCommand)
			}
		})
	}
}

func TestIntercept_CustomOptions(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	in, err := NewInterceptor(nil,
		WithBinaryName("./bin/zqk"),
		WithMaxTokens(2500),
	)
	if err != nil {
		t.Fatalf("failed to create interceptor: %v", err)
	}

	res, err := in.Intercept(context.Background(), InterceptRequest{
		CommandLine: `grep -r "test" .`,
		Cwd:         tmpDir,
		ProjectRoot: tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Intercepted {
		t.Fatalf("expected interception, got false")
	}

	expectedPrefix := "./bin/zqk grep --max-tokens 2500"
	if !strings.HasPrefix(res.RewrittenCommand, expectedPrefix) {
		t.Errorf("expected command starting with %q, got %q", expectedPrefix, res.RewrittenCommand)
	}

	defIn := NewDefaultInterceptor()
	if defIn == nil {
		t.Fatalf("expected non-nil default interceptor")
	}
}

func TestIntercept_StageErrorAndEdgeCases(t *testing.T) {
	t.Parallel()

	// 1. Unclosed quote in command line
	res, err := Intercept(context.Background(), InterceptRequest{
		CommandLine: `grep "unclosed quote`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Intercepted {
		t.Errorf("expected unclosed quote command NOT to be intercepted")
	}
	if res.Reason != ReasonNotTargetSearchTool {
		t.Errorf("expected reason %q, got %q", ReasonNotTargetSearchTool, res.Reason)
	}

	// 2. Uninitialized Interceptor
	var nilIn *PipelineInterceptor
	_, err = nilIn.Intercept(context.Background(), InterceptRequest{})
	if err == nil {
		t.Fatalf("expected error on nil interceptor")
	}

	// 3. Stage type safety check: passing invalid payload directly to stages
	pctx := pipeline.NewContext(context.Background())
	if _, err := stageFunnelFilter(pctx, "invalid"); err == nil {
		t.Errorf("expected error passing string to stageFunnelFilter")
	}
	if _, err := stageStreamFilterDetector(pctx, "invalid"); err == nil {
		t.Errorf("expected error passing string to stageStreamFilterDetector")
	}
	if _, err := stagePathBoundaryDetector(pctx, "invalid"); err == nil {
		t.Errorf("expected error passing string to stagePathBoundaryDetector")
	}
	if _, err := stageIntentClassifier(pctx, "invalid"); err == nil {
		t.Errorf("expected error passing string to stageIntentClassifier")
	}
	transformer := stageRewriteTransformer(Options{})
	if _, err := transformer(pctx, "invalid"); err == nil {
		t.Errorf("expected error passing string to stageRewriteTransformer")
	}

	// 4. Halt propagation check
	haltState := &PipelineState{Halt: true}
	if out, err := stageFunnelFilter(pctx, haltState); err != nil || out != haltState {
		t.Errorf("expected halt state passed through unchanged in stageFunnelFilter")
	}
	if out, err := stageStreamFilterDetector(pctx, haltState); err != nil || out != haltState {
		t.Errorf("expected halt state passed through unchanged in stageStreamFilterDetector")
	}
	if out, err := stagePathBoundaryDetector(pctx, haltState); err != nil || out != haltState {
		t.Errorf("expected halt state passed through unchanged in stagePathBoundaryDetector")
	}
	if out, err := stageIntentClassifier(pctx, haltState); err != nil || out != haltState {
		t.Errorf("expected halt state passed through unchanged in stageIntentClassifier")
	}
	if out, err := transformer(pctx, haltState); err != nil || out != haltState {
		t.Errorf("expected halt state passed through unchanged in stageRewriteTransformer")
	}
}

func TestAdditionalCoverage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// 1. Long flags with '=' and space separated variants
	longFlagTests := []struct {
		name        string
		command     string
		expectedCmd string
	}{
		{
			name:        "long context with equal",
			command:     `grep --context=5 "pattern" .`,
			expectedCmd: `zqk grep -C 5 "pattern"`,
		},
		{
			name:        "long context with space",
			command:     `grep --context 4 "pattern" .`,
			expectedCmd: `zqk grep -C 4 "pattern"`,
		},
		{
			name:        "long max-count with equal",
			command:     `grep --max-count=20 "pattern" .`,
			expectedCmd: `zqk grep -m 20 "pattern"`,
		},
		{
			name:        "long max-count with space",
			command:     `grep --max-count 15 "pattern" .`,
			expectedCmd: `zqk grep -m 15 "pattern"`,
		},
		{
			name:        "long type with equal",
			command:     `rg --type=go "pattern"`,
			expectedCmd: `zqk grep --ext .go "pattern"`,
		},
		{
			name:        "long type with space",
			command:     `rg --type go "pattern"`,
			expectedCmd: `zqk grep --ext .go "pattern"`,
		},
		{
			name:        "long kind with equal",
			command:     `grep --kind=func "Start" .`,
			expectedCmd: `zqk grep --kind func "Start"`,
		},
		{
			name:        "long recv with equal",
			command:     `grep --recv=Engine "Start" .`,
			expectedCmd: `zqk grep --recv Engine "Start"`,
		},
		{
			name:        "long regexp with equal",
			command:     `grep --regexp="my-pattern" .`,
			expectedCmd: `zqk grep -e "my-pattern"`,
		},
		{
			name:        "long regexp with space",
			command:     `grep --regexp "my-pattern" .`,
			expectedCmd: `zqk grep -e "my-pattern"`,
		},
		{
			name:        "long fixed-strings",
			command:     `grep --fixed-strings "literal" .`,
			expectedCmd: `zqk grep "literal"`,
		},
		{
			name:        "short flag cluster attached context",
			command:     `grep -C5 "pattern" .`,
			expectedCmd: `zqk grep -C 5 "pattern"`,
		},
		{
			name:        "short flag cluster attached max-count",
			command:     `grep -m3 "pattern" .`,
			expectedCmd: `zqk grep -m 3 "pattern"`,
		},
		{
			name:        "short flag cluster attached type",
			command:     `rg -tgo "pattern"`,
			expectedCmd: `zqk grep --ext .go "pattern"`,
		},
		{
			name:        "short flag cluster attached regexp",
			command:     `grep -epattern .`,
			expectedCmd: `zqk grep -e "pattern"`,
		},
		{
			name:        "short flag with separate type",
			command:     `rg -t go "pattern"`,
			expectedCmd: `zqk grep --ext .go "pattern"`,
		},
	}

	for _, tt := range longFlagTests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res, err := Intercept(context.Background(), InterceptRequest{
				CommandLine: tt.command,
				Cwd:         tmpDir,
				ProjectRoot: tmpDir,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !res.Intercepted {
				t.Fatalf("expected intercepted command, reason: %s", res.Reason)
			}
			if res.RewrittenCommand != tt.expectedCmd {
				t.Errorf("expected %q, got %q", tt.expectedCmd, res.RewrittenCommand)
			}
		})
	}

	// 2. Real directory and code file target checking
	testFile := filepath.Join(tmpDir, "code.go")
	if err := fileutil.WriteFile(testFile, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("failed to create code.go: %v", err)
	}
	resCode, err := Intercept(context.Background(), InterceptRequest{
		CommandLine: `grep "package" code.go`,
		Cwd:         tmpDir,
		ProjectRoot: tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resCode.Intercepted {
		t.Errorf("expected searching code.go to be intercepted")
	}

	// 3. Target path with trailing slash directory
	resDirSlash, err := Intercept(context.Background(), InterceptRequest{
		CommandLine: `grep "foo" ./`,
		Cwd:         tmpDir,
		ProjectRoot: tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resDirSlash.Intercepted {
		t.Errorf("expected searching ./ to be intercepted")
	}

	// 4. Tokenizer quotes and escaped characters
	tokens, err := tokenizeShell(`echo "a\"b" 'cd' \e`)
	if err != nil {
		t.Fatalf("tokenizeShell unexpected error: %v", err)
	}
	if len(tokens) != 4 {
		t.Errorf("expected 4 tokens, got %d: %v", len(tokens), tokens)
	}

	// 5. Logical OR operator '||'
	segs, err := splitPipelineSegments(`true || grep "foo" .`)
	if err != nil {
		t.Fatalf("splitPipelineSegments unexpected error: %v", err)
	}
	if len(segs) != 1 {
		t.Errorf("expected 1 segment for logical OR, got %d", len(segs))
	}

	// 6. Direct helper checks for edge cases
	if !isExternalPath("/outside/root", tmpDir, tmpDir, nil) {
		t.Errorf("expected /outside/root to be external")
	}
	if isExternalPath("", tmpDir, tmpDir, nil) {
		t.Errorf("expected empty target path NOT to be external")
	}
	if isNonCodeTarget(".", tmpDir) {
		t.Errorf("expected '.' NOT to be non-code")
	}
	if isNonCodeTarget("dir/", tmpDir) {
		t.Errorf("expected 'dir/' NOT to be non-code")
	}

	// 7. Context cancellation in PipelineInterceptor.Intercept
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	in, err := NewInterceptor(nil)
	if err != nil {
		t.Fatalf("unexpected error creating interceptor: %v", err)
	}
	_, err = in.Intercept(canceledCtx, InterceptRequest{CommandLine: `grep "foo" .`})
	if err == nil {
		t.Errorf("expected error on canceled context")
	}

	// 8. NewInterceptor with nil option
	inNilOpt, err := NewInterceptor(nil, nil)
	if err != nil || inNilOpt == nil {
		t.Errorf("expected NewInterceptor with nil option to succeed")
	}

	// 9. Split pipeline and tokenizer boundary cases
	emptySegs, err := splitPipelineSegments("")
	if err != nil || len(emptySegs) != 0 {
		t.Errorf("expected empty segments for empty string")
	}
	doublePipeSegs, err := splitPipelineSegments("echo a | | grep b")
	if err != nil || len(doublePipeSegs) != 2 {
		t.Errorf("expected 2 non-empty segments, got %d", len(doublePipeSegs))
	}
	if _, err := tokenizeShell(`echo "unclosed`); err == nil {
		t.Errorf("expected error for unclosed quote in tokenizeShell")
	}

	// 10. Executable extractor with only env vars
	exe, idx := extractExecutable([]string{"A=1", "B=2"})
	if exe != "" || idx != -1 {
		t.Errorf("expected no executable found, got %q, %d", exe, idx)
	}

	// 11. isExternalPath with empty cwd
	if isExternalPath("rel/path", "", tmpDir, nil) {
		t.Errorf("expected rel/path under tmpDir NOT to be external")
	}
	if isExternalPath(tmpDir, tmpDir, tmpDir, nil) {
		t.Errorf("expected tmpDir NOT to be external to itself")
	}

	// 12. Package level Intercept helper execution
	resPkg, err := Intercept(context.Background(), InterceptRequest{
		CommandLine: `grep -r "test" .`,
		ProjectRoot: tmpDir,
	})
	if err != nil || !resPkg.Intercepted {
		t.Errorf("expected package-level Intercept to succeed")
	}
}
