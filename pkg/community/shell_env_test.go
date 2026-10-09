package community

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestShellEnv_FunctionalAcceptance(t *testing.T) {
	tmpDir := t.TempDir()

	testCases := []struct {
		name       string
		shell      ShellType
		configFile string
		binDir     string
		expectRule string
	}{
		{
			name:       "zsh_injection",
			shell:      ShellZsh,
			configFile: filepath.Join(tmpDir, ".zshrc"),
			binDir:     "/opt/zqk/bin",
			expectRule: "export PATH=\"/opt/zqk/bin:$PATH\"",
		},
		{
			name:       "bash_injection",
			shell:      ShellBash,
			configFile: filepath.Join(tmpDir, ".bashrc"),
			binDir:     "/usr/local/zqk/bin",
			expectRule: "export PATH=\"/usr/local/zqk/bin:$PATH\"",
		},
		{
			name:       "fish_injection",
			shell:      ShellFish,
			configFile: filepath.Join(tmpDir, "config.fish"),
			binDir:     filepath.Join("/Users/test", paths.ProjectDataDir, "bin"),
			expectRule: "fish_add_path " + filepath.Join("/Users/test", paths.ProjectDataDir, "bin"),
		},
		{
			name:       "posix_injection",
			shell:      ShellPosix,
			configFile: filepath.Join(tmpDir, ".profile"),
			binDir:     "/home/user/bin",
			expectRule: "export PATH=\"/home/user/bin:$PATH\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			profile := ShellProfile{
				Shell:      tc.shell,
				ConfigFile: tc.configFile,
				BinDir:     tc.binDir,
			}

			// Pre-populate config file with user settings
			initialContent := "# User configuration\nalias ll='ls -la'\n"
			if err := fileutil.WriteFile(tc.configFile, []byte(initialContent), paths.FilePerm644); err != nil {
				t.Fatalf("failed creating test config: %v", err)
			}

			// 1. Inject PATH
			res, err := InjectPath(profile)
			if err != nil {
				t.Fatalf("InjectPath failed: %v", err)
			}
			if !res.Modified {
				t.Errorf("expected res.Modified to be true")
			}
			if res.BackupFile == "" {
				t.Errorf("expected BackupFile to be created")
			}

			// 2. Verify content
			data, err := fileutil.ReadFile(tc.configFile)
			if err != nil {
				t.Fatalf("failed reading modified config: %v", err)
			}
			content := string(data)
			if !strings.Contains(content, tc.expectRule) {
				t.Errorf("expected config to contain %q, got: %s", tc.expectRule, content)
			}
			if !strings.Contains(content, "alias ll='ls -la'") {
				t.Errorf("expected user config to be preserved")
			}

			// 3. VerifyPathInjected helper
			injected, err := VerifyPathInjected(profile)
			if err != nil {
				t.Fatalf("VerifyPathInjected failed: %v", err)
			}
			if !injected {
				t.Errorf("expected VerifyPathInjected to return true")
			}

			// 4. Remove injection
			remRes, err := RemovePathInjection(profile)
			if err != nil {
				t.Fatalf("RemovePathInjection failed: %v", err)
			}
			if !remRes.Modified {
				t.Errorf("expected remRes.Modified to be true")
			}

			dataAfter, err := fileutil.ReadFile(tc.configFile)
			if err != nil {
				t.Fatalf("failed reading config after removal: %v", err)
			}
			contentAfter := string(dataAfter)
			if strings.Contains(contentAfter, tc.expectRule) {
				t.Errorf("expected snippet to be removed, but still present")
			}
			if !strings.Contains(contentAfter, "alias ll='ls -la'") {
				t.Errorf("expected original content to remain intact after removal")
			}
		})
	}
}

func TestShellEnv_BoundaryAndErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("empty_fields_validation", func(t *testing.T) {
		_, err := InjectPath(ShellProfile{BinDir: "", ConfigFile: "/tmp/file"})
		if err == nil {
			t.Errorf("expected error on empty BinDir")
		}

		_, err = InjectPath(ShellProfile{BinDir: "/bin", ConfigFile: ""})
		if err == nil {
			t.Errorf("expected error on empty ConfigFile")
		}

		_, err = RemovePathInjection(ShellProfile{ConfigFile: ""})
		if err == nil {
			t.Errorf("expected error on empty ConfigFile in RemovePathInjection")
		}
	})

	t.Run("idempotency_no_duplicate_injection", func(t *testing.T) {
		cfgPath := filepath.Join(tmpDir, "idempotent.zshrc")
		profile := ShellProfile{
			Shell:      ShellZsh,
			ConfigFile: cfgPath,
			BinDir:     "/usr/local/bin",
		}

		res1, err := InjectPath(profile)
		if err != nil || !res1.Modified {
			t.Fatalf("first injection failed: err=%v, mod=%v", err, res1.Modified)
		}

		res2, err := InjectPath(profile)
		if err != nil {
			t.Fatalf("second injection failed: %v", err)
		}
		if res2.Modified {
			t.Errorf("expected second injection to not modify file")
		}
		if !res2.AlreadyConfigured {
			t.Errorf("expected AlreadyConfigured to be true")
		}

		// Count occurrences of marker
		data, _ := fileutil.ReadFile(cfgPath)
		count := strings.Count(string(data), markerStart)
		if count != 1 {
			t.Errorf("expected exactly 1 marker block, found %d", count)
		}
	})

	t.Run("update_existing_snippet_path", func(t *testing.T) {
		cfgPath := filepath.Join(tmpDir, "update_path.bashrc")
		profile1 := ShellProfile{
			Shell:      ShellBash,
			ConfigFile: cfgPath,
			BinDir:     "/old/path/bin",
		}
		profile2 := ShellProfile{
			Shell:      ShellBash,
			ConfigFile: cfgPath,
			BinDir:     "/new/path/bin",
		}

		if _, err := InjectPath(profile1); err != nil {
			t.Fatalf("profile1 injection failed: %v", err)
		}

		resUpdate, err := InjectPath(profile2)
		if err != nil {
			t.Fatalf("profile2 injection failed: %v", err)
		}
		if !resUpdate.Modified {
			t.Errorf("expected update to modify file")
		}

		data, _ := fileutil.ReadFile(cfgPath)
		content := string(data)
		if strings.Contains(content, "/old/path/bin") {
			t.Errorf("old path should have been replaced")
		}
		if !strings.Contains(content, "/new/path/bin") {
			t.Errorf("new path not found in updated file")
		}
	})

	t.Run("remove_nonexistent_file", func(t *testing.T) {
		profile := ShellProfile{
			Shell:      ShellZsh,
			ConfigFile: filepath.Join(tmpDir, "does_not_exist.rc"),
		}
		res, err := RemovePathInjection(profile)
		if err != nil {
			t.Fatalf("expected no error removing from nonexistent file: %v", err)
		}
		if res.Modified {
			t.Errorf("expected Modified to be false")
		}
	})

	t.Run("nested_parent_directory_creation", func(t *testing.T) {
		nestedPath := filepath.Join(tmpDir, "deep", "nested", "subfolder", "config.fish")
		profile := ShellProfile{
			Shell:      ShellFish,
			ConfigFile: nestedPath,
			BinDir:     "/opt/bin",
		}
		res, err := InjectPath(profile)
		if err != nil {
			t.Fatalf("failed injecting into nested path: %v", err)
		}
		if !res.Modified {
			t.Errorf("expected file to be created and modified")
		}
		if _, err := fileutil.Stat(nestedPath); err != nil {
			t.Errorf("nested config file was not created: %v", err)
		}
	})
}

func TestShellEnv_IntegrationAndConformance(t *testing.T) {
	t.Run("shell_detection", func(t *testing.T) {
		cases := []struct {
			input    string
			expected ShellType
		}{
			{"/bin/zsh", ShellZsh},
			{"/usr/local/bin/zsh", ShellZsh},
			{"/bin/bash", ShellBash},
			{"/usr/bin/bash", ShellBash},
			{"/opt/homebrew/bin/fish", ShellFish},
			{"/bin/sh", ShellPosix},
			{"/bin/dash", ShellPosix},
			{"unknown", ShellPosix},
		}

		for _, c := range cases {
			actual := DetectShell(c.input)
			if actual != c.expected {
				t.Errorf("DetectShell(%q) = %s; want %s", c.input, actual, c.expected)
			}
		}
	})

	t.Run("default_config_file_resolution", func(t *testing.T) {
		home := "/home/alice"
		if GetDefaultConfigFile(ShellZsh, home) != "/home/alice/.zshrc" {
			t.Errorf("unexpected zsh default config")
		}
		if GetDefaultConfigFile(ShellBash, home) != "/home/alice/.bashrc" {
			t.Errorf("unexpected bash default config")
		}
		if GetDefaultConfigFile(ShellFish, home) != "/home/alice/.config/fish/config.fish" {
			t.Errorf("unexpected fish default config")
		}
		if GetDefaultConfigFile(ShellPosix, home) != "/home/alice/.profile" {
			t.Errorf("unexpected posix default config")
		}
	})
}
