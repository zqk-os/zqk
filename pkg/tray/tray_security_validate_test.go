// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package tray

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestValidateTrayConfig_EdgeCases(t *testing.T) {
	if err := validateTrayConfig(nil); err == nil {
		t.Fatal("expected error for nil tray config")
	}

	validCfg := &Config{
		Version: 1,
		Entries: []Entry{
			{Name: "status", Argv: []string{"system", "status"}},
		},
	}
	if err := validateTrayConfig(validCfg); err != nil {
		t.Fatalf("expected valid config to pass, got: %v", err)
	}

	invalidVersionCfg := &Config{
		Version: 0,
		Entries: []Entry{
			{Name: "status", Argv: []string{"system", "status"}},
		},
	}
	if err := validateTrayConfig(invalidVersionCfg); err == nil {
		t.Fatal("expected error for config with version 0")
	}

	emptyArgvCfg := &Config{
		Version: 1,
		Entries: []Entry{
			{Name: "status", Argv: []string{}},
		},
	}
	if err := validateTrayConfig(emptyArgvCfg); err == nil {
		t.Fatal("expected error for entry with empty argv")
	}

	invalidNameCfg := &Config{
		Version: 1,
		Entries: []Entry{
			{Name: "Invalid_Name!", Argv: []string{"status"}},
		},
	}
	if err := validateTrayConfig(invalidNameCfg); err == nil {
		t.Fatal("expected error for entry with invalid name pattern")
	}
}

func TestValidateEntries_EdgeCases(t *testing.T) {
	emptyNameEntries := []Entry{
		{Name: "   ", Argv: []string{"status"}},
	}
	if _, err := validateEntries(emptyNameEntries); err == nil {
		t.Fatal("expected error for entry with empty trimmed name")
	}

	emptyArgvEntries := []Entry{
		{Name: "valid-name", Argv: []string{}},
	}
	if _, err := validateEntries(emptyArgvEntries); err == nil {
		t.Fatal("expected error for entry with empty argv slice")
	}

	validEntries := []Entry{
		{Name: "  trimmed-name  ", Argv: []string{"status"}},
	}
	cleaned, err := validateEntries(validEntries)
	if err != nil {
		t.Fatalf("unexpected error validating entries: %v", err)
	}
	if cleaned[0].Name != "trimmed-name" {
		t.Fatalf("expected trimmed name, got %q", cleaned[0].Name)
	}
}

func TestMerge_EmptyNameSkipping(t *testing.T) {
	defaults := []Entry{
		{Name: "", Argv: []string{"invalid"}},
		{Name: "item-a", Argv: []string{"a"}},
	}
	user := []Entry{
		{Name: "", Argv: []string{"invalid-user"}},
		{Name: "item-b", Argv: []string{"b"}},
	}

	merged := Merge(defaults, user)
	if len(merged) != 2 {
		t.Fatalf("expected 2 valid merged entries, got %d", len(merged))
	}
	if merged[0].Name != "item-a" || merged[1].Name != "item-b" {
		t.Fatalf("unexpected merged entry names: %+v", merged)
	}
	if !merged[0].IsDefault {
		t.Fatal("expected item-a to have IsDefault=true")
	}
	if merged[1].IsDefault {
		t.Fatal("expected item-b to have IsDefault=false")
	}
}

func TestLoadTray_FilesystemEdgeCases(t *testing.T) {
	t.Run("non_existent_tray_yaml_returns_defaults", func(t *testing.T) {
		tmp := t.TempDir()
		zqkDir := filepath.Join(tmp, paths.ProjectDataDir)
		if err := fileutil.EnsureDir(zqkDir); err != nil {
			t.Fatal(err)
		}
		entries, err := Load(tmp)
		if err != nil {
			t.Fatalf("expected load to succeed with defaults, got: %v", err)
		}
		if len(entries) == 0 {
			t.Fatal("expected default entries, got empty slice")
		}
		for _, e := range entries {
			if !e.IsDefault {
				t.Fatalf("expected default entry %q to have IsDefault=true", e.Name)
			}
		}
	})

	t.Run("directory_at_tray_yaml_path_returns_read_error", func(t *testing.T) {
		tmp := t.TempDir()
		userPath := datacell.TrayYAMLPath(tmp)
		if err := fileutil.EnsureDir(userPath); err != nil {
			t.Fatal(err)
		}
		_, err := Load(tmp)
		if err == nil {
			t.Fatal("expected error when tray.yaml is a directory")
		}
	})

	t.Run("malformed_yaml_returns_parse_error", func(t *testing.T) {
		tmp := t.TempDir()
		userPath := datacell.TrayYAMLPath(tmp)
		if err := fileutil.EnsureDir(filepath.Dir(userPath)); err != nil {
			t.Fatal(err)
		}
		if err := fileutil.WriteSecureFile(userPath, []byte(":::[invalid yaml")); err != nil {
			t.Fatal(err)
		}
		_, err := Load(tmp)
		if err == nil {
			t.Fatal("expected parse error for malformed yaml")
		}
	})
}

func TestSecurity_IsPrivilegedArgv_TokenMatch(t *testing.T) {
	twoWordTokens := []string{
		"object delete",
		"system shutdown",
		"system purge",
		"io reap",
	}

	for _, token := range twoWordTokens {
		t.Run("token_"+token, func(t *testing.T) {
			isPriv, matched := IsPrivilegedArgv([]string{token})
			if !isPriv {
				t.Fatalf("expected IsPrivilegedArgv to detect token %q", token)
			}
			if matched != token {
				t.Fatalf("expected matched token %q, got %q", token, matched)
			}
		})
	}

	// Verify unprivileged command returns false
	isPriv, token := IsPrivilegedArgv([]string{"workflow", "whats-next", "--json"})
	if isPriv {
		t.Fatalf("expected whats-next to be unprivileged, got token %q", token)
	}

	// Verify nil / empty argv returns false
	if isPrivEmpty, _ := IsPrivilegedArgv(nil); isPrivEmpty {
		t.Fatal("expected nil argv to be unprivileged")
	}
	if isPrivZero, _ := IsPrivilegedArgv([]string{}); isPrivZero {
		t.Fatal("expected empty argv to be unprivileged")
	}
}

func TestSecurity_Crypto_BoundaryBranches(t *testing.T) {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// 130-char hex with "04" prefix
	rawHex := fmt.Sprintf("%064x%064x", privKey.PublicKey.X, privKey.PublicKey.Y)
	prefixedHex := "04" + rawHex
	parsedKey, err := ParsePublicKeyHex(prefixedHex)
	if err != nil {
		t.Fatalf("ParsePublicKeyHex with 04 prefix failed: %v", err)
	}
	if parsedKey.X.Cmp(privKey.PublicKey.X) != 0 || parsedKey.Y.Cmp(privKey.PublicKey.Y) != 0 {
		t.Fatal("parsed key coordinates do not match original")
	}

	// Invalid Y coordinate (valid X, non-hex Y)
	invalidYHex := strings.Repeat("a", 64) + strings.Repeat("z", 64)
	if _, err := ParsePublicKeyHex(invalidYHex); err == nil {
		t.Fatal("expected error for invalid Y coordinate in public key hex")
	}

	// VerifyEntry with invalid signature hex
	entry := &Entry{Name: "test-cmd", Argv: []string{"status"}, Signature: "not-a-valid-hex-signature!"}
	if err := VerifyEntry(entry, rawHex); err == nil {
		t.Fatal("expected error decoding non-hex signature")
	}

	// VerifyEntry with invalid public key
	entry.Signature = "aabbccddeeff"
	if err := VerifyEntry(entry, "invalid-pub-hex"); err == nil {
		t.Fatal("expected error parsing invalid public key")
	}
}

func TestTray_SimulatedLifecycleAndDispatcher(t *testing.T) {
	// Simulated Desktop Tray state model
	type TrayState string
	const (
		StateInitializing TrayState = "initializing"
		StateReady        TrayState = "ready"
		StateDispatched   TrayState = "dispatched"
		StateTerminated   TrayState = "terminated"
	)

	type ActionRequest struct {
		ItemName string
		Argv     []string
		Signed   bool
	}

	type TraySim struct {
		mu      sync.Mutex
		state   TrayState
		actions []string
		entries []Entry
	}

	sim := &TraySim{
		state: StateInitializing,
		entries: []Entry{
			{Name: "safe-status", Description: "Show Status", Argv: []string{"workflow", "whats-next"}},
			{Name: "dangerous-purge", Description: "Purge System", Argv: []string{"system", "purge"}},
		},
	}

	// State transition to ready
	sim.mu.Lock()
	sim.state = StateReady
	sim.mu.Unlock()

	dispatchAction := func(req ActionRequest) (string, error) {
		sim.mu.Lock()
		defer sim.mu.Unlock()

		isPriv, token := IsPrivilegedArgv(req.Argv)
		if isPriv && !req.Signed {
			return "", fmt.Errorf("action denied: privileged command %q requires cryptographic signature", token)
		}

		line := FormatExplainLine("zqk", req.Argv)
		sim.actions = append(sim.actions, line)
		sim.state = StateDispatched
		return line, nil
	}

	// 1. Unprivileged action succeeds
	line, err := dispatchAction(ActionRequest{
		ItemName: "safe-status",
		Argv:     []string{"workflow", "whats-next"},
		Signed:   false,
	})
	if err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}
	if line != "zqk workflow whats-next" {
		t.Fatalf("unexpected command line: %q", line)
	}

	// 2. Unsigned privileged action blocked
	_, err = dispatchAction(ActionRequest{
		ItemName: "dangerous-purge",
		Argv:     []string{"system", "purge"},
		Signed:   false,
	})
	if err == nil {
		t.Fatal("expected security block for unsigned privileged action")
	}

	// 3. Signed privileged action allowed
	line, err = dispatchAction(ActionRequest{
		ItemName: "dangerous-purge",
		Argv:     []string{"system", "purge"},
		Signed:   true,
	})
	if err != nil {
		t.Fatalf("unexpected dispatch error for signed action: %v", err)
	}
	if line != "zqk system purge" {
		t.Fatalf("unexpected command line: %q", line)
	}

	// Terminate
	sim.mu.Lock()
	sim.state = StateTerminated
	sim.mu.Unlock()
}

func TestTray_ConcurrentOperations(t *testing.T) {
	const concurrency = 8
	var wg sync.WaitGroup
	var completedCount atomic.Int64

	for i := 0; i < concurrency; i++ {
		workerID := i
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_tray_concurrent", fmt.Sprintf("worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()

			// Concurrently validate schemas
			cfg := &Config{
				Version: 1,
				Entries: []Entry{
					{Name: fmt.Sprintf("cmd-%d", workerID), Argv: []string{"status", fmt.Sprintf("--id=%d", workerID)}},
				},
			}
			if err := validateTrayConfig(cfg); err != nil {
				t.Errorf("worker %d schema validation error: %v", workerID, err)
				return
			}

			// Concurrently check privileged tokens
			isPriv, _ := IsPrivilegedArgv([]string{"workflow", "whats-next"})
			if isPriv {
				t.Errorf("worker %d unexpected privilege detection", workerID)
				return
			}

			// Concurrently query embedded entries
			entries, err := Load("")
			if err != nil {
				t.Errorf("worker %d load error: %v", workerID, err)
				return
			}
			if len(entries) == 0 {
				t.Errorf("worker %d empty entries", workerID)
				return
			}

			completedCount.Add(1)
		})
	}

	wg.Wait()
	if completedCount.Load() != concurrency {
		t.Fatalf("expected %d completed workers, got %d", concurrency, completedCount.Load())
	}
}

func TestSecurity_ShellInjectionAndPathTraversalBoundaries(t *testing.T) {
	adversarialArgvs := [][]string{
		{"status", "; rm -rf /"},
		{"run", "| cat /etc/passwd"},
		{"exec", "$(whoami) arg"},
		{"eval", "`id` arg"},
		{"pipeline", "&& malicious_command"},
		{"read", "../../../../etc/shadow with spaces"},
		{"quote", "argument with 'single' quote"},
		{"quote", `argument with "double" quote`},
	}

	for idx, argv := range adversarialArgvs {
		t.Run(fmt.Sprintf("adversarial_%d", idx), func(t *testing.T) {
			// FormatExplainLine quotes arguments containing whitespace or quotes
			line := FormatExplainLine("zqk", argv)
			if !strings.Contains(line, `"`) {
				t.Fatalf("expected quote in formatted explain line for adversarial argv: %s", line)
			}

			// CanonicalPayload and digest deterministically handle adversarial strings
			payload, err := CanonicalPayload(fmt.Sprintf("test-%d", idx), argv)
			if err != nil {
				t.Fatalf("CanonicalPayload failed on adversarial input: %v", err)
			}
			if len(payload) == 0 {
				t.Fatal("expected non-empty canonical payload")
			}

			digest, err := ComputeArgvDigest(fmt.Sprintf("test-%d", idx), argv)
			if err != nil {
				t.Fatalf("ComputeArgvDigest failed: %v", err)
			}
			if digest == [32]byte{} {
				t.Fatal("expected non-zero digest for adversarial input")
			}
		})
	}

	// Verify unquoted formatting for tokens without spaces/quotes
	cleanLine := FormatExplainLine("zqk", []string{"workflow", "whats-next", "--json"})
	if cleanLine != "zqk workflow whats-next --json" {
		t.Fatalf("unexpected clean line format: %q", cleanLine)
	}
}

func TestTray_MenuConstructionAndIconResolution(t *testing.T) {
	tmp := t.TempDir()
	iconPath := filepath.Join(tmp, "tray_icon.png")
	corruptIconPath := filepath.Join(tmp, "corrupt_icon.png")
	missingIconPath := filepath.Join(tmp, "missing_icon.png")

	// Write mock valid icon bytes
	validIconData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if err := fileutil.WriteSecureFile(iconPath, validIconData); err != nil {
		t.Fatalf("failed to write mock icon: %v", err)
	}

	// Write corrupt icon bytes
	corruptData := []byte{0x00, 0x01, 0x02}
	if err := fileutil.WriteSecureFile(corruptIconPath, corruptData); err != nil {
		t.Fatalf("failed to write corrupt icon: %v", err)
	}

	type MenuItem struct {
		ID       string
		Title    string
		Tooltip  string
		Icon     string
		Disabled bool
	}

	resolveIcon := func(path string) ([]byte, error) {
		data, err := fileutil.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read icon %s: %w", path, err)
		}
		if len(data) < 8 || string(data[:4]) != "\x89PNG" {
			return nil, fmt.Errorf("invalid icon magic header: %s", path)
		}
		return data, nil
	}

	// 1. Valid icon resolution
	data, err := resolveIcon(iconPath)
	if err != nil || len(data) != 8 {
		t.Fatalf("expected valid icon data, got: %v", err)
	}

	// 2. Corrupt icon resolution error
	if _, err := resolveIcon(corruptIconPath); err == nil {
		t.Fatal("expected error resolving corrupt icon header")
	}

	// 3. Missing icon resolution error
	if _, err := resolveIcon(missingIconPath); err == nil {
		t.Fatal("expected error resolving missing icon")
	}

	// Menu construction from entries
	entries := []Entry{
		{Name: "scheduler-status", Description: "Scheduler Status", Argv: []string{"scheduler", "status"}},
		{Name: "workflow-next", Description: "Whats Next", Argv: []string{"workflow", "whats-next"}},
	}

	menuItems := make([]MenuItem, 0, len(entries))
	for _, e := range entries {
		menuItems = append(menuItems, MenuItem{
			ID:      e.Name,
			Title:   e.Description,
			Tooltip: FormatExplainLine("zqk", e.Argv),
			Icon:    iconPath,
		})
	}

	if len(menuItems) != 2 {
		t.Fatalf("expected 2 menu items, got %d", len(menuItems))
	}
	if menuItems[0].Tooltip != "zqk scheduler status" {
		t.Fatalf("unexpected tooltip: %q", menuItems[0].Tooltip)
	}
}
