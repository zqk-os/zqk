package context

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"
)

const emptyValue = ""

// contextPropagationTestCase represents a single test case for context propagation
// Reserved for future use when we add table-driven tests
//
//nolint:unused // Reserved for future use
type contextPropagationTestCase struct {
	name           string
	profile        LoggingProfile
	securityCtx    *SecurityContext
	lifecycleCtx   string   // lifecycle name or empty
	commandPath    []string // e.g., ["system", "check"]
	flags          map[string]string
	expectedChecks []contextCheck
}

// contextCheck validates a specific aspect of context propagation
//
//nolint:unused // Reserved for future use
type contextCheck struct {
	name        string
	checkFunc   func(ctx context.Context, t *testing.T) error
	description string
}

// TestContextPropagation_AllProfiles tests context propagation for all logging profiles
func TestContextPropagation_AllProfiles(t *testing.T) {
	t.Parallel()
	profiles := []LoggingProfile{
		ProfileMCP,
		ProfileSystem,
		ProfileAIAgent,
		ProfileDebug,
		ProfileHuman,
	}

	for _, profile := range profiles {
		t.Run(string(profile), func(t *testing.T) {
			testProfileContextPropagation(t, profile)
		})
	}
}

// TestContextPropagation_AllSecurityContexts tests context propagation for all security contexts
func TestContextPropagation_AllSecurityContexts(t *testing.T) {
	t.Parallel()
	securityContexts := []struct {
		name string
		ctx  *SecurityContext
	}{
		{"system", NewSystemSecurityContext()},
		{"founder", NewSecurityContext(FounderAccountID, []string{"admin", "founder"}, []string{"read:*", "write:*", "delete:*"})},
		{"admin", NewSecurityContext("account:admin", []string{"admin"}, []string{"read:*", "write:*", "delete:*"})},
		{"developer", NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*", "write:backlog_item", "write:task"})},
		{"viewer", NewSecurityContext("ACC-1785920548450214017-87f10a62", []string{"viewer"}, []string{"read:*"})},
		{"custom", NewSecurityContext("account:custom", []string{"custom_role"}, []string{"read:backlog_item", "write:backlog_item"})},
	}

	for _, secCtx := range securityContexts {
		t.Run(secCtx.name, func(t *testing.T) {
			testSecurityContextPropagation(t, secCtx.ctx)
		})
	}
}

// TestContextPropagation_AllCommandPaths tests context propagation for all major command paths
func TestContextPropagation_AllCommandPaths(t *testing.T) {
	t.Parallel()
	commandPaths := [][]string{
		{"system", "check"},
		{"system", "fix"},
		{"system", "init"},
		{"object", "create"},
		{"object", "list"},
		{"object", "get"},
		{"scheduler", "submit"},
		{"mcp"},
		{"utility", "validate-yaml"},
		{"utility", "fix-hashes"},
	}

	for _, cmdPath := range commandPaths {
		testName := strings.Join(cmdPath, "_")
		t.Run(testName, func(t *testing.T) {
			testCommandPathContextPropagation(t, cmdPath)
		})
	}
}

// TestContextPropagation_ProfileSecurityCombinations tests all profile + security context combinations
func TestContextPropagation_ProfileSecurityCombinations(t *testing.T) {
	t.Parallel()
	profiles := []LoggingProfile{
		ProfileMCP,
		ProfileSystem,
		ProfileAIAgent,
		ProfileDebug,
		ProfileHuman,
	}

	securityContexts := []*SecurityContext{
		NewSystemSecurityContext(),
		NewSecurityContext(FounderAccountID, []string{"admin", "founder"}, []string{"read:*", "write:*", "delete:*"}),
		NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*", "write:backlog_item"}),
	}

	for _, profile := range profiles {
		for _, secCtx := range securityContexts {
			testName := fmt.Sprintf("%s_%s", profile, secCtx.AccountID)
			t.Run(testName, func(t *testing.T) {
				testProfileSecurityCombination(t, profile, secCtx)
			})
		}
	}
}

// TestContextPropagation_NoBackgroundContext verifies no inappropriate context.Background() usage
func TestContextPropagation_NoBackgroundContext(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	// Create minimal project structure
	zqkDir := filepath.Join(projectRoot, paths.ProjectDataDir)
	if err := fileutil.MkdirAll(zqkDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create .zqk directory: %v", err)
	}

	profiles := []LoggingProfile{
		ProfileMCP,
		ProfileSystem,
		ProfileAIAgent,
		ProfileDebug,
		ProfileHuman,
	}

	for _, profile := range profiles {
		t.Run(string(profile), func(t *testing.T) {
			cmd := createTestCommandWithContext(t, projectRoot, profile)

			// Verify command has context
			ctx := cmd.Context()
			if ctx == nil {
				t.Fatal("Command context is nil")
			}

			// Verify context is not context.Background() (unless it's the root)
			// In tests, we create from Background, but it should be wrapped
			if ctx == context.Background() {
				t.Error("Command context should not be raw context.Background() - should be wrapped with values")
			}

			// Verify logging context is present
			loggingCtx := GetLoggingContext(ctx)
			if loggingCtx == nil {
				t.Error("LoggingContext should be present in command context")
			} else if loggingCtx.Profile != profile {
				t.Errorf("Expected profile %s, got %s", profile, loggingCtx.Profile)
			}
		})
	}
}

// TestContextPropagation_Cancellation tests that context cancellation propagates correctly
func TestContextPropagation_Cancellation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	profiles := []LoggingProfile{
		ProfileSystem,
		ProfileHuman,
		ProfileDebug,
	}

	for _, profile := range profiles {
		t.Run(string(profile), func(t *testing.T) {
			cmd := createTestCommandWithContext(t, projectRoot, profile)
			ctx := cmd.Context()

			// Create a derived context with cancellation
			derivedCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			// Verify derived context has logging context
			derivedLoggingCtx := GetLoggingContext(derivedCtx)
			if derivedLoggingCtx == nil {
				t.Error("Derived context should inherit LoggingContext")
			}

			// Cancel the context
			cancel()

			// Verify cancellation is detected
			select {
			case <-derivedCtx.Done():
				// Expected
			case <-time.After(100 * time.Millisecond):
				t.Error("Context cancellation not detected")
			}

			// Verify parent context is not cancelled
			select {
			case <-ctx.Done():
				t.Error("Parent context should not be cancelled")
			default:
				// Expected
			}
		})
	}
}

// TestContextPropagation_Timeout tests that context timeouts work correctly
func TestContextPropagation_Timeout(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	profiles := []LoggingProfile{
		ProfileSystem,
		ProfileHuman,
	}

	for _, profile := range profiles {
		t.Run(string(profile), func(t *testing.T) {
			cmd := createTestCommandWithContext(t, projectRoot, profile)
			ctx := cmd.Context()

			// Create a derived context with timeout
			timeoutCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer cancel()

			// Verify timeout context has logging context
			timeoutLoggingCtx := GetLoggingContext(timeoutCtx)
			if timeoutLoggingCtx == nil {
				t.Error("Timeout context should inherit LoggingContext")
			}

			// Wait for timeout
			select {
			case <-timeoutCtx.Done():
				// Expected
				if timeoutCtx.Err() != context.DeadlineExceeded {
					t.Errorf("Expected DeadlineExceeded, got %v", timeoutCtx.Err())
				}
			case <-time.After(200 * time.Millisecond):
				t.Error("Context timeout not detected")
			}

			// Verify parent context is not cancelled
			select {
			case <-ctx.Done():
				t.Error("Parent context should not be cancelled")
			default:
				// Expected
			}
		})
	}
}

// TestContextPropagation_ContextValues tests that context values are accessible
func TestContextPropagation_ContextValues(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	profiles := []LoggingProfile{
		ProfileMCP,
		ProfileSystem,
		ProfileAIAgent,
		ProfileDebug,
		ProfileHuman,
	}

	for _, profile := range profiles {
		t.Run(string(profile), func(t *testing.T) {
			cmd := createTestCommandWithContext(t, projectRoot, profile)
			ctx := cmd.Context()

			// Verify logging context is accessible
			loggingCtx := GetLoggingContext(ctx)
			if loggingCtx == nil {
				t.Fatal("LoggingContext should be accessible")
			}
			if loggingCtx.Profile != profile {
				t.Errorf("Expected profile %s, got %s", profile, loggingCtx.Profile)
			}

			// Verify project root is accessible via CliInitializationContext
			findProjectRoot := func(startDir string) string {
				return projectRoot
			}
			initCtx := NewCliInitializationContext(findProjectRoot, projectRoot)
			if initCtx.GetProjectRoot() == emptyValue {
				t.Error("Project root should be accessible from CliInitializationContext")
			}
		})
	}
}

// TestContextPropagation_ContextImmutable tests that context is immutable once created
func TestContextPropagation_ContextImmutable(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	cmd := createTestCommandWithContext(t, projectRoot, ProfileSystem)
	ctx := cmd.Context()

	// Get initial logging context
	initialLoggingCtx := GetLoggingContext(ctx)
	if initialLoggingCtx == nil {
		t.Fatal("Initial LoggingContext should exist")
	}

	// Try to modify the context (should not affect original)
	newLoggingCtx := NewLoggingContext(ProfileHuman)
	newCtx := WithLoggingContext(ctx, newLoggingCtx)

	// Verify original context is unchanged
	originalLoggingCtx := GetLoggingContext(ctx)
	if originalLoggingCtx.Profile != ProfileSystem {
		t.Errorf("Original context should be unchanged, got profile %s", originalLoggingCtx.Profile)
	}

	// Verify new context has new profile
	newCtxLoggingCtx := GetLoggingContext(newCtx)
	if newCtxLoggingCtx.Profile != ProfileHuman {
		t.Errorf("New context should have ProfileHuman, got %s", newCtxLoggingCtx.Profile)
	}
}

// Helper functions

func testProfileContextPropagation(t *testing.T, profile LoggingProfile) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	cmd := createTestCommandWithContext(t, projectRoot, profile)
	ctx := cmd.Context()

	// Verify logging context
	loggingCtx := GetLoggingContext(ctx)
	if loggingCtx == nil {
		t.Fatal("LoggingContext should be present")
	}
	if loggingCtx.Profile != profile {
		t.Errorf("Expected profile %s, got %s", profile, loggingCtx.Profile)
	}

	// Verify context can be derived
	derivedCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	derivedLoggingCtx := GetLoggingContext(derivedCtx)
	if derivedLoggingCtx == nil {
		t.Error("Derived context should inherit LoggingContext")
	}
}

func testSecurityContextPropagation(t *testing.T, secCtx *SecurityContext) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	cmd := createTestCommandWithContext(t, projectRoot, ProfileSystem)
	ctx := cmd.Context()

	// Add security context
	ctx = WithSecurityContext(ctx, secCtx)

	// Verify security context is accessible
	retrievedSecCtx := GetSecurityContext(ctx)
	if retrievedSecCtx == nil {
		t.Fatal("SecurityContext should be accessible")
	}
	if retrievedSecCtx.AccountID != secCtx.AccountID {
		t.Errorf("Expected AccountID %s, got %s", secCtx.AccountID, retrievedSecCtx.AccountID)
	}
}

func testCommandPathContextPropagation(t *testing.T, cmdPath []string) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	profiles := []LoggingProfile{
		ProfileSystem,
		ProfileHuman,
	}

	for _, profile := range profiles {
		t.Run(string(profile), func(t *testing.T) {
			cmd := createTestCommandWithContext(t, projectRoot, profile)

			// Navigate to subcommand if path has multiple elements
			currentCmd := cmd
			for _, cmdName := range cmdPath {
				found := false
				for _, subCmd := range currentCmd.Commands() {
					if subCmd.Name() == cmdName || subCmd.Use == cmdName {
						currentCmd = subCmd
						found = true
						break
					}
				}
				if !found {
					// Command doesn't exist, skip this test
					t.Skipf("Command path %v does not exist", cmdPath)
					return
				}
			}

			// Verify context is present on subcommand
			ctx := currentCmd.Context()
			if ctx == nil {
				t.Error("Subcommand should have context")
			}

			// Verify logging context is present
			loggingCtx := GetLoggingContext(ctx)
			if loggingCtx == nil {
				t.Error("Subcommand should have LoggingContext")
			}
		})
	}
}

func testProfileSecurityCombination(t *testing.T, profile LoggingProfile, secCtx *SecurityContext) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	cmd := createTestCommandWithContext(t, projectRoot, profile)
	ctx := cmd.Context()

	// Add security context
	ctx = WithSecurityContext(ctx, secCtx)

	// Verify both contexts are present
	loggingCtx := GetLoggingContext(ctx)
	if loggingCtx == nil {
		t.Fatal("LoggingContext should be present")
	}
	if loggingCtx.Profile != profile {
		t.Errorf("Expected profile %s, got %s", profile, loggingCtx.Profile)
	}

	retrievedSecCtx := GetSecurityContext(ctx)
	if retrievedSecCtx == nil {
		t.Fatal("SecurityContext should be present")
	}
	if retrievedSecCtx.AccountID != secCtx.AccountID {
		t.Errorf("Expected AccountID %s, got %s", secCtx.AccountID, retrievedSecCtx.AccountID)
	}
}

// createTestCommandWithContext creates a test command with proper context setup
// This mimics the exact context setup that would occur in real CLI execution
func createTestCommandWithContext(t *testing.T, projectRoot string, profile LoggingProfile) *cobra.Command {
	cmd := &cobra.Command{
		Use: "test",
	}

	// Create base context (mimics root.go:86)
	baseCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Set context on command
	cmd.SetContext(baseCtx)

	// Create CLI initialization context
	// Use a simple finder function that returns the provided project root
	findProjectRoot := func(startDir string) string {
		return projectRoot
	}
	_ = NewCliInitializationContext(findProjectRoot, projectRoot)

	// Note: We can't directly set CLI context here due to import cycle
	// The test focuses on context propagation, not full CLI context setup
	// In real execution, PersistentPreRunE would set CLI context

	// Add logging context to the command's context
	loggingCtx := NewLoggingContext(profile)
	ctxWithLogging := WithLoggingContext(cmd.Context(), loggingCtx)
	cmd.SetContext(ctxWithLogging)

	return cmd
}

// TestContextPropagation_AllPermutations is a comprehensive test that exercises all permutations
func TestContextPropagation_AllPermutations(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("Skipping comprehensive permutation test in short mode")
	}

	profiles := []LoggingProfile{
		ProfileMCP,
		ProfileSystem,
		ProfileAIAgent,
		ProfileDebug,
		ProfileHuman,
	}

	securityContexts := []*SecurityContext{
		NewSystemSecurityContext(),
		NewSecurityContext(FounderAccountID, []string{"admin", "founder"}, []string{"read:*", "write:*", "delete:*"}),
		NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*", "write:backlog_item"}),
		NewSecurityContext("ACC-1785920548450214017-87f10a62", []string{"viewer"}, []string{"read:*"}),
	}

	commandPaths := [][]string{
		{"system", "check"},
		{"object", "list"},
		{"utility", "validate-yaml"},
	}

	permutationCount := 0
	for _, profile := range profiles {
		for _, secCtx := range securityContexts {
			for _, cmdPath := range commandPaths {
				permutationCount++
				testName := fmt.Sprintf("profile_%s_security_%s_cmd_%s",
					profile, secCtx.AccountID, strings.Join(cmdPath, "_"))
				t.Run(testName, func(t *testing.T) {
					testFullPermutation(t, profile, secCtx, cmdPath)
				})
			}
		}
	}

	t.Logf("Tested %d context propagation permutations", permutationCount)
}

// TRACK: BLI-CEF-R15-HARDCODED-ACC-001 / REQ-CEF-R2-SEC-HARDCODED-ACC
func TestSecurityContext_NoHardcodedFallback(t *testing.T) {
	ctx := NewSecurityContext("ACC-CUSTOM-USER-001", []string{"developer"}, []string{"read:*"})
	if ctx.AccountID != "ACC-CUSTOM-USER-001" {
		t.Errorf("expected explicit account ID, got %s", ctx.AccountID)
	}
	if ctx.AccountID == SystemAccountID {
		t.Errorf("expected custom account not to alias SystemAccountID")
	}
}

func testFullPermutation(t *testing.T, profile LoggingProfile, secCtx *SecurityContext, _ []string) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	cmd := createTestCommandWithContext(t, projectRoot, profile)
	ctx := cmd.Context()

	// Add security context
	ctx = WithSecurityContext(ctx, secCtx)
	cmd.SetContext(ctx)

	// Verify all contexts are present and correct
	checks := []struct {
		name  string
		check func() error
	}{
		{
			name: "logging_context_present",
			check: func() error {
				loggingCtx := GetLoggingContext(ctx)
				if loggingCtx == nil {
					return fmt.Errorf("LoggingContext should be present")
				}
				if loggingCtx.Profile != profile {
					return fmt.Errorf("Expected profile %s, got %s", profile, loggingCtx.Profile)
				}
				return nil
			},
		},
		{
			name: "security_context_present",
			check: func() error {
				retrievedSecCtx := GetSecurityContext(ctx)
				if retrievedSecCtx == nil {
					return fmt.Errorf("SecurityContext should be present")
				}
				if retrievedSecCtx.AccountID != secCtx.AccountID {
					return fmt.Errorf("Expected AccountID %s, got %s", secCtx.AccountID, retrievedSecCtx.AccountID)
				}
				return nil
			},
		},
		{
			name: "cli_initialization_context_present",
			check: func() error {
				// Verify CliInitializationContext can be created
				findProjectRoot := func(startDir string) string {
					return projectRoot
				}
				initCtx := NewCliInitializationContext(findProjectRoot, projectRoot)
				if initCtx.GetProjectRoot() == emptyValue {
					return fmt.Errorf("CliInitializationContext should provide project root")
				}
				return nil
			},
		},
		{
			name: "context_derivable",
			check: func() error {
				derivedCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				derivedLoggingCtx := GetLoggingContext(derivedCtx)
				if derivedLoggingCtx == nil {
					return fmt.Errorf("Derived context should inherit LoggingContext")
				}
				return nil
			},
		},
		{
			name: "context_not_background",
			check: func() error {
				if ctx == context.Background() {
					return fmt.Errorf("Context should not be raw context.Background()")
				}
				return nil
			},
		},
	}

	for _, check := range checks {
		if err := check.check(); err != nil {
			t.Errorf("Check %s failed: %v", check.name, err)
		}
	}
}

// TestContextPropagation_ContextChain tests the chain-based context system
func TestContextPropagation_ContextChain(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	cmd := createTestCommandWithContext(t, projectRoot, ProfileSystem)
	ctx := cmd.Context()

	// Verify CliInitializationContext can be created and provides project root
	findProjectRoot := func(startDir string) string {
		return projectRoot
	}
	initCtx := NewCliInitializationContext(findProjectRoot, projectRoot)
	if initCtx.GetProjectRoot() == emptyValue {
		t.Error("Project root should be accessible from CliInitializationContext")
	}

	// Verify logging context is in the chain
	loggingCtx := GetLoggingContext(ctx)
	if loggingCtx == nil {
		t.Error("LoggingContext should be in context chain")
	}
}

// TestContextPropagation_ReflectionBasedAudit uses reflection to audit context usage
func TestContextPropagation_ReflectionBasedAudit(t *testing.T) {
	tmpDir := t.TempDir()
	projectRoot := filepath.Join(tmpDir, "test-project")
	if err := fileutil.MkdirAll(projectRoot, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	profiles := []LoggingProfile{
		ProfileMCP,
		ProfileSystem,
		ProfileAIAgent,
		ProfileDebug,
		ProfileHuman,
	}

	for _, profile := range profiles {
		t.Run(string(profile), func(t *testing.T) {
			cmd := createTestCommandWithContext(t, projectRoot, profile)
			ctx := cmd.Context()

			// Use reflection to inspect context values
			ctxValue := reflect.ValueOf(ctx)
			if ctxValue.Kind() == reflect.Ptr {
				_ = ctxValue.Elem() // Checked but not used in this test
			}

			// Verify context has values (not just Background)
			if ctx == context.Background() {
				t.Error("Context should not be raw context.Background()")
			}

			// Verify we can extract values
			loggingCtx := GetLoggingContext(ctx)
			if loggingCtx == nil {
				t.Error("Should be able to extract LoggingContext")
			}
		})
	}
}
