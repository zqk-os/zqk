package community

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// ReleaseAsset describes a downloadable binary artifact and its cryptographic digest.
type ReleaseAsset struct {
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

// ReleaseManifest represents a published release with version metadata and platform assets.
type ReleaseManifest struct {
	Version     string                  `json:"version"`
	ReleaseDate string                  `json:"release_date"`
	Notes       string                  `json:"notes"`
	Assets      map[string]ReleaseAsset `json:"assets"`
}

// UpgradeConfig holds runtime configuration for the upgrade checker daemon.
type UpgradeConfig struct {
	CurrentVersion   string
	ManifestURL      string
	TargetBinaryPath string
	CheckInterval    time.Duration
	HTTPClient       *http.Client
	AutoUpgrade      bool
}

// DaemonStatus represents the observed state of the upgrade daemon.
type DaemonStatus struct {
	Running         bool             `json:"running"`
	CurrentVersion  string           `json:"current_version"`
	LastCheck       time.Time        `json:"last_check"`
	UpdateAvailable bool             `json:"update_available"`
	LatestRelease   *ReleaseManifest `json:"latest_release,omitempty"`
}

// UpgradeDaemon manages automated version checking and binary self-upgrades.
type UpgradeDaemon struct {
	cfg             UpgradeConfig
	mu              sync.RWMutex
	running         bool
	stopCh          chan struct{}
	lastCheck       time.Time
	latestRelease   *ReleaseManifest
	updateAvailable bool
}

// NewUpgradeDaemon constructs a new UpgradeDaemon instance with validated configuration.
func NewUpgradeDaemon(cfg UpgradeConfig) (*UpgradeDaemon, error) {
	if strings.TrimSpace(cfg.CurrentVersion) == "" {
		return nil, fmt.Errorf("current version is required")
	}
	if strings.TrimSpace(cfg.ManifestURL) == "" {
		return nil, fmt.Errorf("manifest URL is required")
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = 1 * time.Hour
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	return &UpgradeDaemon{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}, nil
}

// CheckForUpdate queries the remote manifest URL and determines if a newer version is available.
func (d *UpgradeDaemon) CheckForUpdate(ctx context.Context) (*ReleaseManifest, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.cfg.ManifestURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create manifest request: %w", err)
	}

	resp, err := d.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("manifest request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("manifest server responded with status %d", resp.StatusCode)
	}

	// Protect against unbounded reads (1MB limit)
	limitedReader := io.LimitReader(resp.Body, 1024*1024)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, false, fmt.Errorf("failed reading manifest body: %w", err)
	}

	var manifest ReleaseManifest
	if err := json.Unmarshal(bodyBytes, &manifest); err != nil {
		return nil, false, fmt.Errorf("invalid manifest JSON: %w", err)
	}

	if strings.TrimSpace(manifest.Version) == "" {
		return nil, false, fmt.Errorf("manifest missing version field")
	}

	isNewer := isVersionNewer(d.cfg.CurrentVersion, manifest.Version)

	d.mu.Lock()
	d.lastCheck = time.Now().UTC()
	d.latestRelease = &manifest
	d.updateAvailable = isNewer
	d.mu.Unlock()

	return &manifest, isNewer, nil
}

// DownloadAndVerify downloads the binary asset for the specified platform and verifies its SHA256 digest.
func (d *UpgradeDaemon) DownloadAndVerify(ctx context.Context, platform string) ([]byte, error) {
	d.mu.RLock()
	manifest := d.latestRelease
	d.mu.RUnlock()

	if manifest == nil {
		return nil, fmt.Errorf("no release manifest cached; run CheckForUpdate first")
	}

	asset, ok := manifest.Assets[platform]
	if !ok {
		return nil, fmt.Errorf("platform %q not supported in release %s", platform, manifest.Version)
	}

	if strings.TrimSpace(asset.DownloadURL) == "" {
		return nil, fmt.Errorf("asset for platform %q has empty download URL", platform)
	}
	if strings.TrimSpace(asset.SHA256) == "" {
		return nil, fmt.Errorf("asset for platform %q has empty SHA256 checksum", platform)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create asset request: %w", err)
	}

	resp, err := d.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asset download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asset download failed with status %d", resp.StatusCode)
	}

	hasher := sha256.New()
	var buf bytes.Buffer
	tee := io.TeeReader(resp.Body, hasher)

	// Max 256MB binary size limit
	if _, err := io.Copy(&buf, io.LimitReader(tee, 256*1024*1024)); err != nil {
		return nil, fmt.Errorf("failed downloading asset payload: %w", err)
	}

	computedHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(computedHash, asset.SHA256) {
		return nil, fmt.Errorf("checksum mismatch: expected %s, got %s", asset.SHA256, computedHash)
	}

	return buf.Bytes(), nil
}

// ApplyUpgrade performs an atomic in-place file replacement for the target binary.
func (d *UpgradeDaemon) ApplyUpgrade(binaryData []byte, targetPath string) error {
	if len(binaryData) == 0 {
		return fmt.Errorf("cannot apply upgrade with empty binary payload")
	}
	if strings.TrimSpace(targetPath) == "" {
		targetPath = d.cfg.TargetBinaryPath
	}
	if strings.TrimSpace(targetPath) == "" {
		return fmt.Errorf("target binary path is required")
	}

	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory %s: %w", targetDir, err)
	}

	tmpFile, err := os.CreateTemp(targetDir, "zqk-upgrade-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file for upgrade: %w", err)
	}
	tmpName := tmpFile.Name()

	cleanUp := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}

	if _, err := tmpFile.Write(binaryData); err != nil {
		cleanUp()
		return fmt.Errorf("failed writing binary payload to temp file: %w", err)
	}

	if err := tmpFile.Chmod(0755); err != nil {
		cleanUp()
		return fmt.Errorf("failed to chmod temp binary: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to flush temp binary: %w", err)
	}

	if err := os.Rename(tmpName, targetPath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to atomically replace %s: %w", targetPath, err)
	}

	return nil
}

// Start launches the background polling loop.
func (d *UpgradeDaemon) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("upgrade daemon is already running")
	}

	d.running = true
	d.stopCh = make(chan struct{})
	d.mu.Unlock()

	goroutinelabels.NewGoroutine("community.upgrade_daemon", "background CLI version update checker").StartSimple(func() {
		ticker := time.NewTicker(d.cfg.CheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-d.stopCh:
				return
			case <-ticker.C:
				_, isNewer, err := d.CheckForUpdate(ctx)
				if err == nil && isNewer && d.cfg.AutoUpgrade && d.cfg.TargetBinaryPath != "" {
					// In auto mode, download and apply
					goos := "linux"
					goarch := "amd64"
					platform := fmt.Sprintf("%s-%s", goos, goarch)
					if payload, err := d.DownloadAndVerify(ctx, platform); err == nil {
						_ = d.ApplyUpgrade(payload, d.cfg.TargetBinaryPath)
					}
				}
			}
		}
	})

	return nil
}

// Stop terminates the background update checking loop.
func (d *UpgradeDaemon) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return nil
	}

	d.running = false
	close(d.stopCh)
	return nil
}

// Status returns a point-in-time snapshot of the daemon's state.
func (d *UpgradeDaemon) Status() DaemonStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var clonedRelease *ReleaseManifest
	if d.latestRelease != nil {
		copied := *d.latestRelease
		clonedRelease = &copied
	}

	return DaemonStatus{
		Running:         d.running,
		CurrentVersion:  d.cfg.CurrentVersion,
		LastCheck:       d.lastCheck,
		UpdateAvailable: d.updateAvailable,
		LatestRelease:   clonedRelease,
	}
}

// isVersionNewer compares semver strings (e.g. "v1.2.3" vs "v1.3.0").
// Returns true if candidate is strictly greater than current.
func isVersionNewer(current, candidate string) bool {
	cNorm := strings.TrimPrefix(strings.TrimSpace(current), "v")
	candNorm := strings.TrimPrefix(strings.TrimSpace(candidate), "v")

	cParts := strings.Split(cNorm, ".")
	candParts := strings.Split(candNorm, ".")

	maxLen := len(cParts)
	if len(candParts) > maxLen {
		maxLen = len(candParts)
	}

	for i := 0; i < maxLen; i++ {
		var cVal, candVal int
		if i < len(cParts) {
			numPart := strings.Split(cParts[i], "-")[0]
			cVal, _ = strconv.Atoi(numPart)
		}
		if i < len(candParts) {
			numPart := strings.Split(candParts[i], "-")[0]
			candVal, _ = strconv.Atoi(numPart)
		}

		if candVal > cVal {
			return true
		}
		if candVal < cVal {
			return false
		}
	}

	return false
}
