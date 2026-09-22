package vet

import (
	"bufio"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
)

// CheckTreePolice inspects the repo tree for forbidden paths, files, archive leaks, and script references.
func CheckTreePolice(root string, cfg *GatesConfig) ([]Finding, error) {
	var findings []Finding
	tp := cfg.TreePolice

	// 1. Check forbidden directories/paths
	for _, relPath := range tp.ForbiddenPaths {
		fullPath := filepath.Join(root, relPath)
		if _, err := os.Stat(fullPath); err == nil {
			findings = append(findings, Finding{
				CheckID:  "tree/forbidden-path",
				Suite:    "tree_police",
				File:     relPath,
				Message:  "forbidden path present in public tree",
				Severity: SeverityError,
			})
		}
	}

	// 2. Check forbidden files
	for _, relFile := range tp.ForbiddenFiles {
		fullFile := filepath.Join(root, relFile)
		if _, err := os.Stat(fullFile); err == nil {
			findings = append(findings, Finding{
				CheckID:  "tree/forbidden-file",
				Suite:    "tree_police",
				File:     relFile,
				Message:  "forbidden file present in repository",
				Severity: SeverityError,
			})
		}
	}

	// 3. Check for archived docs in candidate docs directory
	docsDir := filepath.Join(root, "docs")
	if fi, err := os.Stat(docsDir); err == nil && fi.IsDir() {
		_ = filepath.WalkDir(docsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			name := d.Name()
			if name == "archive" || name == "_archive" {
				rel, _ := filepath.Rel(root, path)
				findings = append(findings, Finding{
					CheckID:  "tree/archived-docs",
					Suite:    "tree_police",
					File:     rel,
					Message:  "archived docs must not be present in public candidate docs",
					Severity: SeverityError,
				})
			}
			return nil
		})
	}

	// 4. Check extra scripts in scripts/ (G11 rule from police-community-tree.sh)
	scriptsDir := filepath.Join(root, "scripts")
	if fi, err := os.Stat(scriptsDir); err == nil && fi.IsDir() {
		_ = filepath.WalkDir(scriptsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(name, ".sh") && !strings.HasSuffix(name, ".py") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			relSlash := filepath.ToSlash(rel)

			if isAllowedCommunityScript(relSlash, name) {
				return nil
			}

			findings = append(findings, Finding{
				CheckID:  "tree/extra-script",
				Suite:    "tree_police",
				File:     rel,
				Message:  "extra unapproved script present in scripts/ (must be in open-core or standard overlays)",
				Severity: SeverityError,
			})
			return nil
		})
	}

	// 5. Scan repo for forbidden studio script references
	for _, pattern := range tp.ForbiddenScriptReferences {
		matches, err := runGitGrep(root, pattern, []string{
			".",
			":!scripts/open-core/police-community-tree.sh",
			":!pkg/vet/*",
			":!config/gates.yaml",
		})
		if err == nil {
			for _, m := range matches {
				findings = append(findings, Finding{
					CheckID:  "tree/forbidden-script-ref",
					Suite:    "tree_police",
					File:     m.File,
					Line:     m.Line,
					Message:  "references prohibited studio script (" + pattern + "): " + m.Content,
					Severity: SeverityError,
				})
			}
		}
	}

	return findings, nil
}

func isAllowedCommunityScript(relSlash, name string) bool {
	switch name {
	case "package-community.sh", "install.sh", "generate-openvex.sh",
		"check-hardcoded-paths-and-perms-repo.sh", "check-cli-name-literals-repo.sh",
		"build-bootstrap-archive.sh":
		return true
	}

	allowedPrefixes := []string{
		"scripts/open-core/",
		"scripts/starter_kernel_graph/",
		"scripts/onboarding_roadmap/",
		"scripts/default_agent_skills/",
		"scripts/default_policies/",
		"scripts/demos/",
	}

	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(relSlash, prefix) {
			return true
		}
	}
	return false
}

type grepMatch struct {
	File    string
	Line    int
	Content string
}

func runGitGrep(root, pattern string, pathspecs []string) ([]grepMatch, error) {
	args := []string{"-C", root, "grep", "-n", "-E", pattern, "--"}
	args = append(args, pathspecs...)
	cmd := execwrap.Command("git", args...)
	out, _ := cmd.Output()
	if len(out) == 0 {
		return nil, nil
	}

	var matches []grepMatch
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 3)
		if len(parts) >= 3 {
			lNum, _ := strconv.Atoi(parts[1])
			matches = append(matches, grepMatch{
				File:    parts[0],
				Line:    lNum,
				Content: strings.TrimSpace(parts[2]),
			})
		}
	}
	return matches, nil
}
