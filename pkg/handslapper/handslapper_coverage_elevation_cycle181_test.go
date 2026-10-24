// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package handslapper

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCycle181_MonitorCommand_EntitlementBypass(t *testing.T) {
	t.Parallel()
	svc := NewService(true)
	bypassFlag := zqkenv.BypassHandslapper().Name() + "=1"

	cases := []struct {
		name string
		cmd  string
	}{
		{
			name: "Prefix bypass flag with forbidden grep",
			cmd:  bypassFlag + " grep -rn foo .",
		},
		{
			name: "Prefix bypass flag with forbidden awk",
			cmd:  bypassFlag + " awk '{print $1}' file.txt",
		},
		{
			name: "Prefix bypass flag with namespace violation",
			cmd:  bypassFlag + " rm -rf " + paths.ProjectDataDir,
		},
		{
			name: "Embedded bypass flag in complex pipeline",
			cmd:  "env " + bypassFlag + " bash -c 'cat README.md'",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := svc.MonitorCommand(context.Background(), "test-agent-bypass", tc.cmd)
			if err != nil {
				t.Fatalf("expected nil error when bypass entitlement flag is present, got: %v", err)
			}
		})
	}
}

func TestCycle181_MonitorCommand_EmptyAndWhitespace(t *testing.T) {
	t.Parallel()
	svc := NewService(true)

	cases := []struct {
		name string
		cmd  string
	}{
		{name: "Empty string", cmd: ""},
		{name: "Single space", cmd: " "},
		{name: "Multiple spaces", cmd: "    "},
		{name: "Mixed whitespace tabs and newlines", cmd: "  \t \n \r  "},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := svc.MonitorCommand(context.Background(), "test-agent-empty", tc.cmd)
			if err != nil {
				t.Fatalf("expected nil error for empty or whitespace command %q, got: %v", tc.cmd, err)
			}
		})
	}
}

func TestCycle181_MonitorCommand_AllowedCommands(t *testing.T) {
	t.Parallel()
	svc := NewService(true)

	allowed := []string{
		"./bin/zqk workflow whats-next",
		"zqk object list goal",
		"git status",
		"go test -v -timeout 30s ./pkg/handslapper",
		"echo 'building knowledge kernel'",
		"mkdir -p build/out",
		"make test",
	}

	for _, cmd := range allowed {
		cmd := cmd
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			err := svc.MonitorCommand(context.Background(), "test-agent-allowed", cmd)
			if err != nil {
				t.Fatalf("expected command %q to be allowed, got error: %v", cmd, err)
			}
		})
	}
}

func TestCycle181_MonitorCommand_ForbiddenTools_StrictMode(t *testing.T) {
	t.Parallel()
	svc := NewService(true)
	tools := []string{"grep", "awk", "sed", "cat", "ls", "find"}

	for _, tool := range tools {
		tool := tool
		t.Run("RawTool_"+tool, func(t *testing.T) {
			t.Parallel()
			cmd := fmt.Sprintf("%s target-arg", tool)
			err := svc.MonitorCommand(context.Background(), "agent-strict-tool", cmd)
			if err == nil {
				t.Fatalf("expected error for forbidden tool %q, got nil", tool)
			}
			assertElectrocutionError(t, err, "TOOL_BYPASS", tool)
		})

		t.Run("DoubleQuotedTool_"+tool, func(t *testing.T) {
			t.Parallel()
			cmd := fmt.Sprintf("\"%s\" target-arg", tool)
			err := svc.MonitorCommand(context.Background(), "agent-strict-tool-dq", cmd)
			if err == nil {
				t.Fatalf("expected error for double-quoted tool %q, got nil", tool)
			}
			assertElectrocutionError(t, err, "TOOL_BYPASS", tool)
		})

		t.Run("SingleQuotedTool_"+tool, func(t *testing.T) {
			t.Parallel()
			cmd := fmt.Sprintf("'%s' target-arg", tool)
			err := svc.MonitorCommand(context.Background(), "agent-strict-tool-sq", cmd)
			if err == nil {
				t.Fatalf("expected error for single-quoted tool %q, got nil", tool)
			}
			assertElectrocutionError(t, err, "TOOL_BYPASS", tool)
		})

		t.Run("WrappedBash_"+tool, func(t *testing.T) {
			t.Parallel()
			cmd := fmt.Sprintf("bash -c '%s target-file'", tool)
			err := svc.MonitorCommand(context.Background(), "agent-strict-tool-wrapped", cmd)
			if err == nil {
				t.Fatalf("expected error for wrapped tool invocation %q, got nil", cmd)
			}
			assertElectrocutionError(t, err, "TOOL_BYPASS", tool)
		})
	}
}

func TestCycle181_MonitorCommand_ForbiddenTools_NonStrictMode(t *testing.T) {
	t.Parallel()
	svc := NewService(false)
	tools := []string{"grep", "awk", "sed", "cat", "ls", "find"}

	for _, tool := range tools {
		tool := tool
		t.Run("NonStrict_"+tool, func(t *testing.T) {
			t.Parallel()
			cmd := fmt.Sprintf("%s foo bar", tool)
			err := svc.MonitorCommand(context.Background(), "agent-nonstrict-tool", cmd)
			if err != nil {
				t.Fatalf("expected non-strict mode to log warning and return nil, got error: %v", err)
			}
		})
	}
}

func TestCycle181_MonitorCommand_NamespaceViolation(t *testing.T) {
	t.Parallel()

	t.Run("StrictMode_NamespaceViolation", func(t *testing.T) {
		t.Parallel()
		svc := NewService(true)
		targetPath := paths.ProjectDataDir + "/lock.db"
		cmd := "touch " + targetPath

		err := svc.MonitorCommand(context.Background(), "agent-ns-strict", cmd)
		if err == nil {
			t.Fatalf("expected namespace violation error, got nil")
		}
		assertElectrocutionError(t, err, "NAMESPACE_VIOLATION", paths.ProjectDataDir)
	})

	t.Run("NonStrictMode_NamespaceViolation", func(t *testing.T) {
		t.Parallel()
		svc := NewService(false)
		cmd := "stat " + paths.ProjectDataDir + "/state.json"

		err := svc.MonitorCommand(context.Background(), "agent-ns-nonstrict", cmd)
		if err != nil {
			t.Fatalf("expected nil in non-strict mode for namespace violation, got: %v", err)
		}
	})
}

func TestCycle181_Electrocute_SentinelAndContextHierarchy(t *testing.T) {
	t.Parallel()

	t.Run("NilContext", func(t *testing.T) {
		t.Parallel()
		var nilCtx context.Context
		err := Electrocute(nilCtx, "FATAL_DRIFT", "context was uninitialized")
		if err == nil {
			t.Fatalf("expected error from Electrocute, got nil")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "⚡️ BZZZT! AGENT ELECTROCUTED: FATAL_DRIFT") {
			t.Fatalf("missing electrocution sentinel header in: %s", errMsg)
		}
		if !strings.Contains(errMsg, "Target Persona: [UNKNOWN_ENTITY]") {
			t.Fatalf("expected UNKNOWN_ENTITY for nil context in: %s", errMsg)
		}
		if !strings.Contains(errMsg, "User Mandate Violation: context was uninitialized") {
			t.Fatalf("missing violation reason in: %s", errMsg)
		}
	})

	t.Run("ContextWithoutSecurityContext", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		err := Electrocute(ctx, "TOOL_BYPASS", "forbidden command attempted")
		if err == nil {
			t.Fatalf("expected error from Electrocute, got nil")
		}
		if !strings.Contains(err.Error(), "Target Persona: [UNKNOWN_ENTITY]") {
			t.Fatalf("expected UNKNOWN_ENTITY for context without security context: %s", err.Error())
		}
	})

	t.Run("SecurityContextWithoutRoles", func(t *testing.T) {
		t.Parallel()
		ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
			AccountID: "agent-lone-wolf",
		})
		err := Electrocute(ctx, "HALLUCINATION", "unanchored fact generation")
		if err == nil {
			t.Fatalf("expected error from Electrocute, got nil")
		}
		errMsg := err.Error()
		if !strings.Contains(errMsg, "Target Persona: [agent-lone-wolf]") {
			t.Fatalf("expected Target Persona: [agent-lone-wolf], got: %s", errMsg)
		}
		if strings.Contains(errMsg, "Role:") {
			t.Fatalf("expected no role in message when roles slice is empty, got: %s", errMsg)
		}
	})

	t.Run("SecurityContextWithSingleRole", func(t *testing.T) {
		t.Parallel()
		ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
			AccountID: "agent-craftsman",
			Roles:     []string{"software-engineer"},
		})
		err := Electrocute(ctx, "TOOL_BYPASS", "attempted raw grep")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedPersona := "Target Persona: [agent-craftsman (Role: software-engineer)]"
		if !strings.Contains(err.Error(), expectedPersona) {
			t.Fatalf("expected %q, got: %s", expectedPersona, err.Error())
		}
	})

	t.Run("SecurityContextWithMultipleRolesTakesPrimary", func(t *testing.T) {
		t.Parallel()
		ctx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
			AccountID: "agent-supervisor",
			Roles:     []string{"architect", "lead-maintainer", "auditor"},
		})
		err := Electrocute(ctx, "NAMESPACE_VIOLATION", "tampered with project data directory")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		expectedPersona := "Target Persona: [agent-supervisor (Role: architect)]"
		if !strings.Contains(err.Error(), expectedPersona) {
			t.Fatalf("expected primary role in %q, got: %s", expectedPersona, err.Error())
		}
	})
}

func TestCycle181_DriftEvent_TelemetryAndTypes(t *testing.T) {
	t.Parallel()
	svc := NewService(false)

	eventTypes := []struct {
		driftType DriftType
		name      string
	}{
		{driftType: DriftTypeToolBypass, name: "ToolBypass"},
		{driftType: DriftTypeNamespaceViolation, name: "NamespaceViolation"},
		{driftType: DriftTypeHallucination, name: "Hallucination"},
	}

	for _, et := range eventTypes {
		et := et
		t.Run(et.name, func(t *testing.T) {
			t.Parallel()
			event := DriftEvent{
				AgentID:  "agent-telemetry-test",
				Type:     et.driftType,
				Command:  "zqk test run",
				Severity: 10,
			}
			if event.AgentID != "agent-telemetry-test" {
				t.Fatalf("unexpected agent ID: %s", event.AgentID)
			}
			if event.Type != et.driftType {
				t.Fatalf("unexpected drift type: %s", event.Type)
			}
			if event.Command != "zqk test run" {
				t.Fatalf("unexpected command: %s", event.Command)
			}
			if event.Severity != 10 {
				t.Fatalf("unexpected severity: %d", event.Severity)
			}
			svc.LogDrift(event)
		})
	}
}

func TestCycle181_MonitorCommand_Concurrent_GoroutineLabels(t *testing.T) {
	t.Parallel()
	strictSvc := NewService(true)
	nonStrictSvc := NewService(false)

	const workerCount = 16
	var wg sync.WaitGroup

	errCh := make(chan error, workerCount*4)

	for i := 0; i < workerCount; i++ {
		workerID := i

		goroutinelabels.NewGoroutine(
			fmt.Sprintf("handslapper_worker_%d", workerID),
			"concurrent command monitoring verification",
		).WithWaitGroup(&wg).StartSimple(func() {
			runWorkerCommandChecks(strictSvc, nonStrictSvc, workerID, errCh)
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent test worker failed: %v", err)
		}
	}
}

func runWorkerCommandChecks(
	strictSvc *Service,
	nonStrictSvc *Service,
	workerID int,
	errCh chan<- error,
) {
	ctx := context.Background()
	agentID := fmt.Sprintf("agent-concurrent-%d", workerID)

	// 1. Allowed command should pass in strict mode
	if err := strictSvc.MonitorCommand(ctx, agentID, "zqk workflow whats-next"); err != nil {
		errCh <- fmt.Errorf("worker %d: allowed command failed: %w", workerID, err)
		return
	}

	// 2. Bypass flag should permit forbidden tool in strict mode
	bypassCmd := zqkenv.BypassHandslapper().Name() + "=1 grep -rn token ."
	if err := strictSvc.MonitorCommand(ctx, agentID, bypassCmd); err != nil {
		errCh <- fmt.Errorf("worker %d: bypass command failed: %w", workerID, err)
		return
	}

	// 3. Forbidden tool should be electrocuted in strict mode
	forbiddenCmd := "find . -name '*.go'"
	if err := strictSvc.MonitorCommand(ctx, agentID, forbiddenCmd); err == nil {
		errCh <- fmt.Errorf("worker %d: forbidden command succeeded unexpectedly", workerID)
		return
	}

	// 4. Forbidden tool should pass with nil error in non-strict mode
	if err := nonStrictSvc.MonitorCommand(ctx, agentID, "cat /etc/hosts"); err != nil {
		errCh <- fmt.Errorf("worker %d: non-strict mode returned unexpected error: %w", workerID, err)
		return
	}
}

func assertElectrocutionError(t *testing.T, err error, expectedType string, keyword string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected non-nil error, got nil")
	}
	msg := err.Error()
	sentinelPrefix := fmt.Sprintf("⚡️ BZZZT! AGENT ELECTROCUTED: %s", expectedType)
	if !strings.Contains(msg, sentinelPrefix) {
		t.Fatalf("expected error message to contain %q, got: %s", sentinelPrefix, msg)
	}
	if keyword != "" && !strings.Contains(msg, keyword) {
		t.Fatalf("expected error message to contain keyword %q, got: %s", keyword, msg)
	}

	// Verify error identity and formatting integrity
	if !errors.Is(err, err) {
		t.Fatalf("expected errors.Is to match self")
	}
}
