package community

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpgradeDaemon_FunctionalAcceptance(t *testing.T) {
	binaryPayload := []byte("#!/bin/sh\necho upgraded binary\n")
	hasher := sha256.New()
	hasher.Write(binaryPayload)
	expectedSHA := hex.EncodeToString(hasher.Sum(nil))

	var serverURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest.json":
			manifest := ReleaseManifest{
				Version:     "v2.0.0",
				ReleaseDate: "2026-09-18T00:00:00Z",
				Notes:       "Major performance and community features release",
				Assets: map[string]ReleaseAsset{
					"darwin-arm64": {
						DownloadURL: serverURL + "/downloads/zqk-darwin-arm64",
						SHA256:      expectedSHA,
						Size:        int64(len(binaryPayload)),
					},
					"linux-amd64": {
						DownloadURL: serverURL + "/downloads/zqk-linux-amd64",
						SHA256:      expectedSHA,
						Size:        int64(len(binaryPayload)),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(manifest)
		case "/downloads/zqk-darwin-arm64", "/downloads/zqk-linux-amd64":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(binaryPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	serverURL = srv.URL

	tmpDir := t.TempDir()
	targetBinary := filepath.Join(tmpDir, "bin", "zqk")

	cfg := UpgradeConfig{
		CurrentVersion:   "v1.0.0",
		ManifestURL:      serverURL + "/releases/latest.json",
		TargetBinaryPath: targetBinary,
		CheckInterval:    50 * time.Millisecond,
		HTTPClient:       srv.Client(),
	}

	daemon, err := NewUpgradeDaemon(cfg)
	if err != nil {
		t.Fatalf("failed to create upgrade daemon: %v", err)
	}

	ctx := context.Background()

	// 1. Check for update
	manifest, isNewer, err := daemon.CheckForUpdate(ctx)
	if err != nil {
		t.Fatalf("CheckForUpdate failed: %v", err)
	}
	if !isNewer {
		t.Fatalf("expected isNewer to be true, got false")
	}
	if manifest.Version != "v2.0.0" {
		t.Errorf("expected version v2.0.0, got %s", manifest.Version)
	}

	// 2. Download and verify binary
	downloaded, err := daemon.DownloadAndVerify(ctx, "darwin-arm64")
	if err != nil {
		t.Fatalf("DownloadAndVerify failed: %v", err)
	}
	if string(downloaded) != string(binaryPayload) {
		t.Errorf("downloaded binary payload mismatch")
	}

	// 3. Apply upgrade
	if err := daemon.ApplyUpgrade(downloaded, targetBinary); err != nil {
		t.Fatalf("ApplyUpgrade failed: %v", err)
	}

	info, err := os.Stat(targetBinary)
	if err != nil {
		t.Fatalf("upgraded binary not found: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("expected target binary to be executable, mode: %v", info.Mode())
	}
}

func TestUpgradeDaemon_BoundaryAndErrorHandling(t *testing.T) {
	t.Run("config_validation_errors", func(t *testing.T) {
		_, err := NewUpgradeDaemon(UpgradeConfig{
			CurrentVersion: "",
			ManifestURL:    "http://example.com",
		})
		if err == nil {
			t.Errorf("expected error for empty current version")
		}

		_, err = NewUpgradeDaemon(UpgradeConfig{
			CurrentVersion: "v1.0.0",
			ManifestURL:    "",
		})
		if err == nil {
			t.Errorf("expected error for empty manifest URL")
		}
	})

	t.Run("network_and_server_failures", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		daemon, err := NewUpgradeDaemon(UpgradeConfig{
			CurrentVersion: "v1.0.0",
			ManifestURL:    srv.URL + "/releases/latest.json",
			HTTPClient:     srv.Client(),
		})
		if err != nil {
			t.Fatalf("failed creating daemon: %v", err)
		}

		_, _, err = daemon.CheckForUpdate(context.Background())
		if err == nil {
			t.Errorf("expected error when server responds 500")
		}
	})

	t.Run("invalid_manifest_json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not valid json"))
		}))
		defer srv.Close()

		daemon, err := NewUpgradeDaemon(UpgradeConfig{
			CurrentVersion: "v1.0.0",
			ManifestURL:    srv.URL + "/releases/latest.json",
			HTTPClient:     srv.Client(),
		})
		if err != nil {
			t.Fatalf("failed creating daemon: %v", err)
		}

		_, _, err = daemon.CheckForUpdate(context.Background())
		if err == nil {
			t.Errorf("expected error for malformed json")
		}
	})

	t.Run("unsupported_platform", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			manifest := ReleaseManifest{
				Version: "v2.0.0",
				Assets: map[string]ReleaseAsset{
					"linux-amd64": {DownloadURL: "http://example.com/bin", SHA256: "abc"},
				},
			}
			_ = json.NewEncoder(w).Encode(manifest)
		}))
		defer srv.Close()

		daemon, _ := NewUpgradeDaemon(UpgradeConfig{
			CurrentVersion: "v1.0.0",
			ManifestURL:    srv.URL,
			HTTPClient:     srv.Client(),
		})
		_, _, _ = daemon.CheckForUpdate(context.Background())

		_, err := daemon.DownloadAndVerify(context.Background(), "freebsd-arm")
		if err == nil {
			t.Errorf("expected error for unsupported platform")
		}
	})

	t.Run("checksum_mismatch_rejection", func(t *testing.T) {
		binaryPayload := []byte("corrupted payload")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/manifest.json" {
				manifest := ReleaseManifest{
					Version: "v2.0.0",
					Assets: map[string]ReleaseAsset{
						"linux-amd64": {
							DownloadURL: r.Host + "/bin",
							SHA256:      "0000000000000000000000000000000000000000000000000000000000000000",
						},
					},
				}
				_ = json.NewEncoder(w).Encode(manifest)
			} else {
				_, _ = w.Write(binaryPayload)
			}
		}))
		defer srv.Close()

		daemon, _ := NewUpgradeDaemon(UpgradeConfig{
			CurrentVersion: "v1.0.0",
			ManifestURL:    srv.URL + "/manifest.json",
			HTTPClient:     srv.Client(),
		})

		// Force the asset download URL to hit local httptest server
		daemon.latestRelease = &ReleaseManifest{
			Version: "v2.0.0",
			Assets: map[string]ReleaseAsset{
				"linux-amd64": {
					DownloadURL: srv.URL + "/bin",
					SHA256:      "0000000000000000000000000000000000000000000000000000000000000000",
				},
			},
		}

		_, err := daemon.DownloadAndVerify(context.Background(), "linux-amd64")
		if err == nil {
			t.Errorf("expected error on checksum mismatch")
		}
	})

	t.Run("apply_upgrade_empty_payload", func(t *testing.T) {
		daemon, _ := NewUpgradeDaemon(UpgradeConfig{
			CurrentVersion: "v1.0.0",
			ManifestURL:    "http://example.com",
		})
		err := daemon.ApplyUpgrade(nil, "/tmp/bin")
		if err == nil {
			t.Errorf("expected error when applying empty payload")
		}
	})
}

func TestUpgradeDaemon_IntegrationAndConformance(t *testing.T) {
	t.Run("semver_comparison_matrix", func(t *testing.T) {
		cases := []struct {
			current   string
			candidate string
			expected  bool
		}{
			{"v1.0.0", "v1.0.1", true},
			{"v1.0.0", "v1.1.0", true},
			{"v1.0.0", "v2.0.0", true},
			{"v1.2.3", "v1.2.3", false},
			{"v2.0.0", "v1.9.9", false},
			{"1.0.0", "1.0.1", true},
			{"v1.5.0-beta", "v1.5.1", true},
			{"v1.5.1", "v1.5.0", false},
		}

		for _, c := range cases {
			actual := isVersionNewer(c.current, c.candidate)
			if actual != c.expected {
				t.Errorf("isVersionNewer(%q, %q) = %v; want %v", c.current, c.candidate, actual, c.expected)
			}
		}
	})

	t.Run("daemon_lifecycle", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			manifest := ReleaseManifest{
				Version: "v1.0.0",
			}
			_ = json.NewEncoder(w).Encode(manifest)
		}))
		defer srv.Close()

		daemon, err := NewUpgradeDaemon(UpgradeConfig{
			CurrentVersion: "v1.0.0",
			ManifestURL:    srv.URL,
			CheckInterval:  20 * time.Millisecond,
			HTTPClient:     srv.Client(),
		})
		if err != nil {
			t.Fatalf("failed to create daemon: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := daemon.Start(ctx); err != nil {
			t.Fatalf("failed to start daemon: %v", err)
		}

		// Double start should fail
		if err := daemon.Start(ctx); err == nil {
			t.Errorf("expected error on repeated start")
		}

		status := daemon.Status()
		if !status.Running {
			t.Errorf("expected daemon to report running: true")
		}

		time.Sleep(50 * time.Millisecond)

		if err := daemon.Stop(); err != nil {
			t.Fatalf("failed to stop daemon: %v", err)
		}

		status = daemon.Status()
		if status.Running {
			t.Errorf("expected daemon to report running: false after stop")
		}
	})
}
