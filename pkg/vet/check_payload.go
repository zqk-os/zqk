package vet

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
)

// CheckPayload verifies required files, forbidden files, sensitive files, and pattern leaks.
func CheckPayload(root string, cfg *GatesConfig) ([]Finding, error) {
	var findings []Finding
	pl := cfg.Payload

	// 1. Verify required public files
	for _, req := range pl.RequiredArtifacts {
		fullPath := filepath.Join(root, req)
		if fi, err := os.Stat(fullPath); err != nil || fi.IsDir() {
			findings = append(findings, Finding{
				CheckID:  "payload/required-artifact-missing",
				Suite:    "payload",
				File:     req,
				Message:  "required public artifact missing",
				Severity: SeverityError,
			})
		}
	}

	// 2. Verify forbidden public files
	for _, forb := range pl.ForbiddenArtifacts {
		fullPath := filepath.Join(root, forb)
		if _, err := os.Stat(fullPath); err == nil {
			findings = append(findings, Finding{
				CheckID:  "payload/forbidden-artifact-present",
				Suite:    "payload",
				File:     forb,
				Message:  "forbidden or unconfigured public artifact present",
				Severity: SeverityError,
			})
		}
	}

	// 3. Verify module declaration in go.mod
	goModBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err == nil {
		if !strings.Contains(string(goModBytes), "module github.com/zqk-os/zqk") {
			findings = append(findings, Finding{
				CheckID:  "payload/invalid-module-path",
				Suite:    "payload",
				File:     "go.mod",
				Message:  "go.mod does not declare module github.com/zqk-os/zqk",
				Severity: SeverityError,
			})
		}
	}

	// 4. Sensitive files check
	sensitivePatterns := []string{
		".zqk/process/**", "docs/process/**",
		".zqk/keystore/**", ".zqk/state/**", "config/zqk-local.yaml",
		"*.pem", "*.key", "*.p12", "*.pfx",
	}
	sensitiveFiles, _ := checkSensitiveTrackedFiles(root, sensitivePatterns)
	for _, f := range sensitiveFiles {
		findings = append(findings, Finding{
			CheckID:  "payload/sensitive-file-tracked",
			Suite:    "payload",
			File:     f,
			Message:  "sensitive/private runtime material tracked in git repository",
			Severity: SeverityError,
		})
	}

	// 5. Prohibited pattern scanning using git grep
	for _, r := range pl.ProhibitedPatterns {
		var pathspecs []string
		for _, sc := range r.Scope {
			if sc == "**" || sc == "*" {
				pathspecs = append(pathspecs, ".")
			} else {
				pathspecs = append(pathspecs, sc)
			}
		}
		if len(pathspecs) == 0 {
			pathspecs = []string{"."}
		}

		for _, ex := range r.Exemptions {
			pathspecs = append(pathspecs, ":!"+ex)
		}
		// Always exempt pkg/vet/* and config/gates.yaml from self-matches
		pathspecs = append(pathspecs, ":!pkg/vet/*", ":!config/gates.yaml")

		matches, err := runGitGrep(root, r.Pattern, pathspecs)
		if err == nil {
			for _, m := range matches {
				findings = append(findings, Finding{
					CheckID:  "payload/" + r.ID,
					Suite:    "payload",
					File:     m.File,
					Line:     m.Line,
					Message:  r.Message + " [" + m.Content + "]",
					Severity: SeverityError,
				})
			}
		}
	}

	return findings, nil
}

func checkSensitiveTrackedFiles(root string, patterns []string) ([]string, error) {
	args := []string{"-C", root, "ls-files", "-z", "--"}
	args = append(args, patterns...)
	cmd := execwrap.Command("git", args...)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil, nil
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}
