package security

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var (
	// ErrSecurityGateViolation indicates one or more security or leak violations occurred.
	ErrSecurityGateViolation = errors.New("security gate violation")

	// Regular expressions for secret detection
	awsKeyRegex     = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	githubPatRegex  = regexp.MustCompile(`\bghp_[0-9a-zA-Z]{36}\b`)
	privateKeyRegex = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
	slackHookRegex  = regexp.MustCompile(`https://hooks\.slack\.com/services/T[0-9a-zA-Z]+/B[0-9a-zA-Z]+/[0-9a-zA-Z]+`)
	studioIDRegex   = regexp.MustCompile(`\b(BLI|REQ|DEC|PRI|ATK|CRIT|TEST|GOAL|CVS)-[0-9]{13,}-[a-f0-9]{6,}\b`)
)

// Violation represents a specific security rule breach in a release candidate payload.
type Violation struct {
	Rule     string `json:"rule"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

// AuditOptions configures payload auditing behavior.
type AuditOptions struct {
	CheckSecrets      bool
	CheckStudioLitter bool
	CheckStudioIDs    bool
	CheckScriptBudget bool
	MaxProductScripts int
	IgnorePrefixes    []string
}

// DefaultAuditOptions returns standard strict options for public candidate releases.
func DefaultAuditOptions() AuditOptions {
	return AuditOptions{
		CheckSecrets:      true,
		CheckStudioLitter: true,
		CheckStudioIDs:    true,
		CheckScriptBudget: true,
		MaxProductScripts: 2,
		IgnorePrefixes: []string{
			".git",
			"vendor",
			".oc-sun-quarantine",
			".zqk",
		},
	}
}

// AuditReport aggregates all findings from a payload audit.
type AuditReport struct {
	ScannedFiles int         `json:"scanned_files"`
	Violations   []Violation `json:"violations"`
	Passed       bool        `json:"passed"`
}

// AuditPayload inspects a directory tree for leaked secrets, forbidden studio artifacts,
// and excessive script budgets according to fail-closed release candidate policies.
func AuditPayload(root string, opts AuditOptions) (*AuditReport, error) {
	cleanRoot := filepath.Clean(root)
	info, err := fileutil.Stat(cleanRoot)
	if err != nil {
		return nil, fmt.Errorf("stat target root %q: %w", cleanRoot, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("target root %q is not a directory", cleanRoot)
	}

	report := &AuditReport{
		Violations: make([]Violation, 0),
		Passed:     true,
	}

	forbiddenPaths := []string{
		"cmd/zqk-admin",
		"cmd/codegen_runner",
		"cmd/pattern-cli",
		".cursor",
		".agents",
	}

	for _, rel := range forbiddenPaths {
		target := filepath.Join(cleanRoot, rel)
		if _, err := fileutil.Stat(target); err == nil {
			report.Violations = append(report.Violations, Violation{
				Rule:     "FORBIDDEN_STUDIO_PATH",
				Path:     rel,
				Evidence: "studio-only artifact present in release payload",
			})
		}
	}

	// Script budget check (G11): same extras rule as scripts/open-core/police-community-tree.sh.
	// TRACK: keep AuditPayload extras in lockstep with police.
	if opts.CheckScriptBudget {
		scriptsDir := filepath.Join(cleanRoot, "scripts")
		if sInfo, err := fileutil.Stat(scriptsDir); err == nil && sInfo.IsDir() {
			count := 0
			_ = filepath.WalkDir(scriptsDir, func(p string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() {
					return nil
				}
				ext := filepath.Ext(p)
				if ext != ".sh" && ext != ".py" {
					return nil
				}
				rel, relErr := filepath.Rel(scriptsDir, p)
				if relErr != nil {
					rel = d.Name()
				}
				if g11AllowlistedScript(rel) {
					return nil
				}
				count++
				return nil
			})
			if count > opts.MaxProductScripts {
				report.Violations = append(report.Violations, Violation{
					Rule:     "SCRIPT_BUDGET_EXCEEDED",
					Path:     "scripts",
					Evidence: fmt.Sprintf("scripts/ contains %d extra product scripts (budget cap is %d)", count, opts.MaxProductScripts),
				})
			}
		}
	}

	// Recursive file scan
	err = filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(cleanRoot, path)
		if err != nil {
			rel = path
		}

		// Check ignored prefixes
		for _, prefix := range opts.IgnorePrefixes {
			if rel == prefix || strings.HasPrefix(rel, prefix+string(filepath.Separator)) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if d.IsDir() {
			return nil
		}

		report.ScannedFiles++

		// Disallow compiled test binaries
		if strings.HasSuffix(d.Name(), ".test") {
			report.Violations = append(report.Violations, Violation{
				Rule:     "TRACKED_TEST_BINARY",
				Path:     rel,
				Evidence: "compiled test binary detected",
			})
			return nil
		}

		// Skip binary files and vendor
		data, err := fileutil.ReadFile(path)
		if err != nil {
			return nil
		}
		if isBinary(data) {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		for idx, line := range lines {
			lineNum := idx + 1

			// Secret detections
			if opts.CheckSecrets {
				if awsKeyRegex.MatchString(line) {
					report.Violations = append(report.Violations, Violation{
						Rule:     "AWS_KEY_LEAK",
						Path:     rel,
						Line:     lineNum,
						Evidence: "detected AWS access key identifier",
					})
				}
				if githubPatRegex.MatchString(line) {
					report.Violations = append(report.Violations, Violation{
						Rule:     "GITHUB_PAT_LEAK",
						Path:     rel,
						Line:     lineNum,
						Evidence: "detected GitHub Personal Access Token",
					})
				}
				if privateKeyRegex.MatchString(line) {
					report.Violations = append(report.Violations, Violation{
						Rule:     "PRIVATE_KEY_LEAK",
						Path:     rel,
						Line:     lineNum,
						Evidence: "detected private key block",
					})
				}
				if slackHookRegex.MatchString(line) {
					report.Violations = append(report.Violations, Violation{
						Rule:     "SLACK_WEBHOOK_LEAK",
						Path:     rel,
						Line:     lineNum,
						Evidence: "detected Slack incoming webhook URL",
					})
				}
			}

			// Studio ID leaks (e.g. BLI-1234567890123-abcdef)
			if opts.CheckStudioIDs && studioIDRegex.MatchString(line) {
				report.Violations = append(report.Violations, Violation{
					Rule:     "LONG_STUDIO_ID_LEAK",
					Path:     rel,
					Line:     lineNum,
					Evidence: "long studio kernel-ID in product source",
				})
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk payload %q: %w", cleanRoot, err)
	}

	report.Passed = len(report.Violations) == 0
	return report, nil
}

// VerifyLicenseIntegrity asserts that LICENSE exists, is non-empty, and matches recognized open-source requirements.
func VerifyLicenseIntegrity(root string) error {
	cleanRoot := filepath.Clean(root)
	licPath := filepath.Join(cleanRoot, "LICENSE")
	data, err := fileutil.ReadFile(licPath)
	if err != nil {
		return fmt.Errorf("read LICENSE: %w", err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("LICENSE file is empty")
	}

	// Must contain standard open source copyright or terms
	content := string(data)
	if !strings.Contains(content, "Apache License") && !strings.Contains(content, "MIT License") {
		return errors.New("LICENSE does not declare Apache or MIT license")
	}

	noticePath := filepath.Join(cleanRoot, "NOTICE")
	if nData, err := fileutil.ReadFile(noticePath); err == nil {
		if len(bytes.TrimSpace(nData)) == 0 {
			return errors.New("NOTICE file exists but is empty")
		}
	}

	return nil
}

// VulnAuditResult represents the summary of a static vulnerability scan.
type VulnAuditResult struct {
	VulnerabilitiesCount int           `json:"vulnerabilities_count"`
	Executed             bool          `json:"executed"`
	Duration             time.Duration `json:"duration"`
	Output               string        `json:"output"`
}

// AuditStaticVulnerabilities invokes govulncheck if available on system, ensuring 0 symbol-level CVEs.
func AuditStaticVulnerabilities(ctx context.Context, projectRoot, targetPkg string) (*VulnAuditResult, error) {
	var binPath string
	candidates := []string{
		"govulncheck",
		filepath.Join(os.Getenv("HOME"), "go", "bin", "govulncheck"),
	}

	for _, cand := range candidates {
		if p, err := exec.LookPath(cand); err == nil {
			binPath = p
			break
		}
		if _, err := fileutil.Stat(cand); err == nil {
			binPath = cand
			break
		}
	}

	if binPath == "" {
		return &VulnAuditResult{
			Executed: false,
			Output:   "govulncheck binary not found on PATH or ~/go/bin",
		}, nil
	}

	start := time.Now()
	cmd := execwrap.CommandContext(ctx, binPath, targetPkg)
	cmd.Dir = projectRoot

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	duration := time.Since(start)
	combined := outBuf.String() + "\n" + errBuf.String()

	res := &VulnAuditResult{
		Executed: true,
		Duration: duration,
		Output:   combined,
	}

	if runErr != nil {
		return res, fmt.Errorf("govulncheck failed: %w (output: %s)", runErr, combined)
	}

	if strings.Contains(combined, "Your code is affected by") {
		re := regexp.MustCompile(`Your code is affected by (\d+) vulnerabilities`)
		m := re.FindStringSubmatch(combined)
		if len(m) > 1 && m[1] != "0" {
			var count int
			_, _ = fmt.Sscanf(m[1], "%d", &count)
			res.VulnerabilitiesCount = count
			return res, fmt.Errorf("affected by %d vulnerabilities", count)
		}
	}

	return res, nil
}

// g11AllowlistedScript reports whether rel (path under scripts/) is a living
// community SKU script, matching police-community-tree.sh MUST_NOT extras.
func g11AllowlistedScript(rel string) bool {
	base := filepath.Base(rel)
	switch base {
	case "check-public-release-payload.sh",
		"install-public-push-guard.sh",
		"package-community.sh",
		"install.sh",
		"generate-openvex.sh",
		"build-bootstrap-archive.sh":
		return true
	}
	n := filepath.ToSlash(rel)
	for _, prefix := range []string{
		"open-core/",
		"starter_kernel_graph/",
		"onboarding_roadmap/",
		"default_agent_skills/",
		"default_policies/",
		"demos/",
	} {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

func isBinary(data []byte) bool {
	if len(data) > 1024 {
		data = data[:1024]
	}
	return bytes.IndexByte(data, 0) != -1
}
