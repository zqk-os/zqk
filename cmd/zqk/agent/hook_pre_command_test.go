package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/clihooks"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/testkit"
)

const reasonSearchIntercepted = "Intercepted search: redirected to token-budgeted zqk grep trigram engine"

func TestRunPreCommandHook_Rewrites(t *testing.T) {
	tests := []struct {
		name                string
		inputJSON           string
		expectedRewritten   bool
		expectedCommandLine string
	}{
		{
			name: "top level command_line with grep",
			inputJSON: `{
				"tool_name": "run_command",
				"command_line": "grep -rn 'foo' .",
				"cwd": "/path/to/project"
			}`,
			expectedRewritten:   true,
			expectedCommandLine: `zqk grep "foo"`,
		},
		{
			name: "top level CommandLine with grep",
			inputJSON: `{
				"tool_name": "run_command",
				"CommandLine": "grep -rn 'foo' .",
				"cwd": "/path/to/project"
			}`,
			expectedRewritten:   true,
			expectedCommandLine: `zqk grep "foo"`,
		},
		{
			name: "nested tool_args command",
			inputJSON: `{
				"tool_name": "run_command",
				"tool_args": {
					"command": "grep -rnwi 'bar' pkg/"
				},
				"cwd": "/path/to/project"
			}`,
			expectedRewritten:   true,
			expectedCommandLine: `zqk grep -i -w "bar" pkg`,
		},
		{
			name: "nested toolCall args CommandLine",
			inputJSON: `{
				"toolCall": {
					"name": "run_command",
					"args": {
						"CommandLine": "grep -rn 'handler' ."
					}
				},
				"workspacePaths": ["/path/to/project"]
			}`,
			expectedRewritten:   true,
			expectedCommandLine: `zqk grep "handler"`,
		},
		{
			name: "ripgrep command",
			inputJSON: `{
				"tool_name": "run_command",
				"command_line": "rg -i 'timeout' cmd/",
				"cwd": "/path/to/project"
			}`,
			expectedRewritten:   true,
			expectedCommandLine: `zqk grep -i "timeout" cmd`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.inputJSON)
			var out bytes.Buffer

			sec := pkgctx.NewSystemSecurityContext()
			err := runPreCommandHookWithDeps(context.Background(), sec, nil, "/path/to/project", "antigravity", in, &out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var hookOut preCommandHookOutput
			if err := json.Unmarshal(out.Bytes(), &hookOut); err != nil {
				t.Fatalf("failed to decode output JSON: %v, raw: %s", err, out.String())
			}

			if hookOut.Decision != "allow" {
				t.Errorf("expected decision 'allow', got %q", hookOut.Decision)
			}

			if tt.expectedRewritten {
				if hookOut.Overwrite == nil {
					t.Fatalf("expected overwrite map, got nil")
				}
				cmdLine, ok := hookOut.Overwrite["CommandLine"].(string)
				if !ok {
					t.Fatalf("expected CommandLine in overwrite, got %v", hookOut.Overwrite)
				}
				if cmdLine != tt.expectedCommandLine {
					t.Errorf("expected CommandLine %q, got %q", tt.expectedCommandLine, cmdLine)
				}
				if hookOut.Reason != reasonSearchIntercepted {
					t.Errorf("expected reason %q, got %q", reasonSearchIntercepted, hookOut.Reason)
				}
			} else {
				if hookOut.Overwrite != nil {
					t.Errorf("expected no overwrite, got %v", hookOut.Overwrite)
				}
			}
		})
	}
}

func TestRunPreCommandHook_NegativeAndBoundaryCases(t *testing.T) {
	tests := []struct {
		name      string
		inputJSON string
	}{
		{
			name:      "empty input",
			inputJSON: "",
		},
		{
			name:      "whitespace only input",
			inputJSON: "   \n\t",
		},
		{
			name:      "malformed JSON input",
			inputJSON: "{invalid-json",
		},
		{
			name: "non-command tool",
			inputJSON: `{
				"tool_name": "read_file",
				"command_line": "grep -rn 'foo' .",
				"cwd": "/path/to/project"
			}`,
		},
		{
			name: "non-search command",
			inputJSON: `{
				"tool_name": "run_command",
				"command_line": "git status",
				"cwd": "/path/to/project"
			}`,
		},
		{
			name: "stream filter pipe",
			inputJSON: `{
				"tool_name": "run_command",
				"command_line": "cat log.txt | grep error",
				"cwd": "/path/to/project"
			}`,
		},
		{
			name: "external path",
			inputJSON: `{
				"tool_name": "run_command",
				"command_line": "grep root /etc/passwd",
				"cwd": "/path/to/project"
			}`,
		},
		{
			name: "empty command string",
			inputJSON: `{
				"tool_name": "run_command",
				"command_line": "   ",
				"cwd": "/path/to/project"
			}`,
		},
		{
			name: "missing command fields",
			inputJSON: `{
				"tool_name": "run_command",
				"cwd": "/path/to/project"
			}`,
		},
		{
			name: "find command passthrough",
			inputJSON: `{
				"tool_name": "run_command",
				"command_line": "find . -name '*.go'",
				"cwd": "/path/to/project"
			}`,
		},
		{
			name: "unsupported invert match flag",
			inputJSON: `{
				"tool_name": "run_command",
				"command_line": "grep -v 'error' .",
				"cwd": "/path/to/project"
			}`,
		},
		{
			name: "unsupported files with matches flag",
			inputJSON: `{
				"tool_name": "run_command",
				"command_line": "grep -l 'foo' .",
				"cwd": "/path/to/project"
			}`,
		},
		{
			name: "non-indexed git config path",
			inputJSON: `{
				"tool_name": "run_command",
				"command_line": "grep 'bare' .git/config",
				"cwd": "/path/to/project"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.inputJSON)
			var out bytes.Buffer

			sec := pkgctx.NewSystemSecurityContext()
			err := runPreCommandHookWithDeps(context.Background(), sec, nil, "/path/to/project", "antigravity", in, &out)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var hookOut preCommandHookOutput
			if err := json.Unmarshal(out.Bytes(), &hookOut); err != nil {
				t.Fatalf("failed to decode output JSON: %v, raw: %s", err, out.String())
			}

			if hookOut.Decision != "allow" {
				t.Errorf("expected decision 'allow', got %q", hookOut.Decision)
			}

			if hookOut.Overwrite != nil {
				t.Errorf("expected no overwrite for negative case, got %v", hookOut.Overwrite)
			}
		})
	}
}

func TestNewPreCommandHookCmd_Structure(t *testing.T) {
	cmd := NewPreCommandHookCmd()
	if cmd.Use != "pre-command" {
		t.Errorf("expected Use 'pre-command', got %q", cmd.Use)
	}

	hasInterceptAlias := false
	for _, alias := range cmd.Aliases {
		if alias == "intercept" {
			hasInterceptAlias = true
			break
		}
	}
	if !hasInterceptAlias {
		t.Errorf("expected 'intercept' alias on command, got %v", cmd.Aliases)
	}

	if cmd.Flags().Lookup("vendor") == nil {
		t.Errorf("expected --vendor flag on command")
	}
}

func TestNewPreCommandHookCmd_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "hook_test",
	})
	cmd := NewPreCommandHookCmd()
	inBuf := strings.NewReader(`{
		"tool_name": "run_command",
		"command_line": "grep -rn 'foo' .",
		"cwd": "` + proj.Root + `"
	}`)
	var outBuf bytes.Buffer
	cmd.SetIn(inBuf)
	cmd.SetOut(&outBuf)
	cmd.SetArgs([]string{"--vendor", "antigravity", "--project-root", proj.Root})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	var hookOut preCommandHookOutput
	if err := json.Unmarshal(outBuf.Bytes(), &hookOut); err != nil {
		t.Fatalf("failed to decode output JSON: %v, raw: %s", err, outBuf.String())
	}

	if hookOut.Decision != "allow" {
		t.Errorf("expected decision 'allow', got %q", hookOut.Decision)
	}
	if hookOut.Overwrite == nil {
		t.Fatalf("expected overwrite in output")
	}
	if hookOut.Overwrite["CommandLine"] != `zqk grep "foo"` {
		t.Errorf("expected rewritten CommandLine `zqk grep \"foo\"`, got %v", hookOut.Overwrite["CommandLine"])
	}
}

func TestRunPreCommandHook_TokenBudgetingAndFormat(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "hook_token_test",
	})

	t.Run("via CLI flags", func(t *testing.T) {
		cmd := NewPreCommandHookCmd()
		inBuf := strings.NewReader(`{
			"tool_name": "run_command",
			"command_line": "grep -rn 'foo' .",
			"cwd": "` + proj.Root + `"
		}`)
		var outBuf bytes.Buffer
		cmd.SetIn(inBuf)
		cmd.SetOut(&outBuf)
		cmd.SetArgs([]string{"--vendor", "antigravity", "--project-root", proj.Root, "--max-tokens", "2000", "--format", "json"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("command execution failed: %v", err)
		}

		var hookOut preCommandHookOutput
		if err := json.Unmarshal(outBuf.Bytes(), &hookOut); err != nil {
			t.Fatalf("failed to decode output JSON: %v, raw: %s", err, outBuf.String())
		}

		if hookOut.Overwrite == nil {
			t.Fatalf("expected overwrite in output")
		}
		expectedCmd := `zqk grep --max-tokens 2000 -f json "foo"`
		if hookOut.Overwrite["CommandLine"] != expectedCmd {
			t.Errorf("expected rewritten CommandLine %q, got %v", expectedCmd, hookOut.Overwrite["CommandLine"])
		}
	})

	t.Run("via payload max_tokens and format", func(t *testing.T) {
		in := strings.NewReader(`{
			"tool_name": "run_command",
			"command_line": "grep -rn 'handler' .",
			"cwd": "` + proj.Root + `",
			"max_tokens": 1500,
			"format": "json"
		}`)
		var out bytes.Buffer

		sec := pkgctx.NewSystemSecurityContext()
		err := runPreCommandHookWithDeps(context.Background(), sec, nil, proj.Root, "antigravity", in, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var hookOut preCommandHookOutput
		if err := json.Unmarshal(out.Bytes(), &hookOut); err != nil {
			t.Fatalf("failed to decode output JSON: %v, raw: %s", err, out.String())
		}

		if hookOut.Overwrite == nil {
			t.Fatalf("expected overwrite in output")
		}
		expectedCmd := `zqk grep --max-tokens 1500 -f json "handler"`
		if hookOut.Overwrite["CommandLine"] != expectedCmd {
			t.Errorf("expected rewritten CommandLine %q, got %v", expectedCmd, hookOut.Overwrite["CommandLine"])
		}
	})

	t.Run("via nested tool_args max_tokens", func(t *testing.T) {
		in := strings.NewReader(`{
			"tool_name": "run_command",
			"tool_args": {
				"command": "grep -rn 'query' .",
				"max_tokens": 1200
			},
			"cwd": "` + proj.Root + `"
		}`)
		var out bytes.Buffer

		sec := pkgctx.NewSystemSecurityContext()
		err := runPreCommandHookWithDeps(context.Background(), sec, nil, proj.Root, "antigravity", in, &out)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var hookOut preCommandHookOutput
		if err := json.Unmarshal(out.Bytes(), &hookOut); err != nil {
			t.Fatalf("failed to decode output JSON: %v, raw: %s", err, out.String())
		}

		if hookOut.Overwrite == nil {
			t.Fatalf("expected overwrite in output")
		}
		expectedCmd := `zqk grep --max-tokens 1200 "query"`
		if hookOut.Overwrite["CommandLine"] != expectedCmd {
			t.Errorf("expected rewritten CommandLine %q, got %v", expectedCmd, hookOut.Overwrite["CommandLine"])
		}
	})
}

func TestRunPreCommandHook_CliHooksDisablement(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "hook_disable_test",
	})

	prof := clihooks.NewProfile(proj.Root)
	if err := prof.Load(); err != nil {
		t.Fatalf("failed to load hook profile: %v", err)
	}

	// Disable agent search interceptor
	if err := prof.SetEnabled(clihooks.HookAgentSearchInterceptor, false); err != nil {
		t.Fatalf("failed to disable hook: %v", err)
	}
	if err := prof.Save(); err != nil {
		t.Fatalf("failed to save hook profile: %v", err)
	}

	in := strings.NewReader(`{
		"tool_name": "run_command",
		"command_line": "grep -rn 'foo' .",
		"cwd": "` + proj.Root + `"
	}`)
	var out bytes.Buffer

	sec := pkgctx.NewSystemSecurityContext()
	err := runPreCommandHookWithDeps(context.Background(), sec, nil, proj.Root, "antigravity", in, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hookOut preCommandHookOutput
	if err := json.Unmarshal(out.Bytes(), &hookOut); err != nil {
		t.Fatalf("failed to decode output JSON: %v, raw: %s", err, out.String())
	}

	if hookOut.Decision != "allow" {
		t.Errorf("expected decision 'allow', got %q", hookOut.Decision)
	}
	if hookOut.Overwrite != nil {
		t.Errorf("expected no overwrite when hook is disabled in clihooks profile, got %v", hookOut.Overwrite)
	}

	// Re-enable hook and verify interception works
	if err := prof.SetEnabled(clihooks.HookAgentSearchInterceptor, true); err != nil {
		t.Fatalf("failed to re-enable hook: %v", err)
	}
	if err := prof.Save(); err != nil {
		t.Fatalf("failed to save hook profile: %v", err)
	}

	in2 := strings.NewReader(`{
		"tool_name": "run_command",
		"command_line": "grep -rn 'foo' .",
		"cwd": "` + proj.Root + `"
	}`)
	var out2 bytes.Buffer
	if err := runPreCommandHookWithDeps(context.Background(), sec, nil, proj.Root, "antigravity", in2, &out2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hookOut2 preCommandHookOutput
	if err := json.Unmarshal(out2.Bytes(), &hookOut2); err != nil {
		t.Fatalf("failed to decode output JSON: %v, raw: %s", err, out2.String())
	}
	if hookOut2.Overwrite == nil || hookOut2.Overwrite["CommandLine"] != `zqk grep "foo"` {
		t.Errorf("expected rewritten command when re-enabled, got %v", hookOut2.Overwrite)
	}
}
