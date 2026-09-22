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

	// 3. Check for archived docs in configured directories
	docDirs := tp.ArchivedDocDirs
	if len(docDirs) == 0 {
		docDirs = []string{"docs"}
	}
	docNames := tp.ArchivedDocNames
	if len(docNames) == 0 {
		docNames = []string{"archive", "_archive"}
	}
	docNameSet := make(map[string]struct{})
	for _, n := range docNames {
		docNameSet[n] = struct{}{}
	}

	for _, dir := range docDirs {
		dirPath := filepath.Join(root, dir)
		if fi, err := os.Stat(dirPath); err == nil && fi.IsDir() {
			_ = filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
				if err != nil || !d.IsDir() {
					return nil
				}
				if _, ok := docNameSet[d.Name()]; ok {
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
	}

	// 4. Check extra scripts in scripts/ (G11 rule)
	if len(tp.AllowedScripts) > 0 || len(tp.AllowedScriptPrefixes) > 0 {
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

				if isAllowedScript(relSlash, name, tp.AllowedScripts, tp.AllowedScriptPrefixes) {
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
	}

	// 5. Scan repo for forbidden studio script references
	grepExempts := tp.GrepExemptions
	if len(grepExempts) == 0 {
		grepExempts = []string{"scripts/open-core/police-community-tree.sh", "pkg/vet/*", "config/gates.yaml"}
	}
	var pathspecs []string
	pathspecs = append(pathspecs, ".")
	for _, ex := range grepExempts {
		pathspecs = append(pathspecs, ":!"+ex)
	}

	for _, pattern := range tp.ForbiddenScriptReferences {
		matches, err := runGitGrep(root, pattern, pathspecs)
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

func isAllowedScript(relSlash, name string, allowedScripts, allowedPrefixes []string) bool {
	for _, s := range allowedScripts {
		if name == s || relSlash == s {
			return true
		}
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
