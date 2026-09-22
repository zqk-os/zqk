package agentonboard

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SkillSyncResult summarizes the projection of kernel skills into workspace roots
// and the pruning of unmanaged snowflakes.
type SkillSyncResult struct {
	Dests   []string `json:"dests,omitempty"`
	Linked  []string `json:"linked,omitempty"`
	Skipped []string `json:"skipped,omitempty"`
	Pruned  []string `json:"pruned,omitempty"`
}

// DefaultSkillDestinations returns the relative workspace destination paths for skill projection.
// Destinations are active if their parent marker exists on disk, or if allVendors is true.
// In headless mode (unless allVendors is set), no IDE skill destinations are activated.
func DefaultSkillDestinations(projectRoot string, detected []DetectedVendor, allVendors, headless bool) []string {
	if headless && !allVendors {
		return nil
	}
	var dests []string
	check := func(parentDir, dest string, vID VendorID) {
		if allVendors || pathExists(filepath.Join(projectRoot, filepath.FromSlash(parentDir))) || hasVendor(detected, vID) {
			dests = append(dests, filepath.ToSlash(dest))
		}
	}

	check(".cursor", ".cursor/skills", VendorIDE)
	check(".agent", ".agent/skills", VendorAgent)
	check(".ide", ".ide/skills", VendorIDE)
	check(".claude", ".claude/skills", VendorClaudeCode)

	return dests
}

func hasVendor(detected []DetectedVendor, id VendorID) bool {
	for _, d := range detected {
		if d.ID == id {
			return true
		}
	}
	return false
}

// SyncSkills synchronizes kernel skills from .zqk/skills into workspace skill roots
// (e.g. .agent/skills, .cursor/skills) as relative symlinks, and actively prunes
// any unmanaged snowflake files or directories that have no backing in the kernel.
func SyncSkills(projectRoot string, detected []DetectedVendor, allVendors, headless, dryRun, force bool) (*SkillSyncResult, error) {
	res := &SkillSyncResult{}
	kernelSkillsDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SkillsSubdir)
	if !pathExists(kernelSkillsDir) {
		return res, nil
	}

	// 1. Enumerate valid kernel skill packs
	entries, err := fileutil.ReadDir(kernelSkillsDir)
	if err != nil {
		return nil, errfmt.Newf("read kernel skills dir %s", kernelSkillsDir).Wrap(err)
	}

	validSkills := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		skillMD := filepath.Join(kernelSkillsDir, name, "SKILL.md")
		if pathExists(skillMD) {
			validSkills[name] = true
		}
	}

	// 2. Identify destination directories
	dests := DefaultSkillDestinations(projectRoot, detected, allVendors, headless)
	res.Dests = dests
	if len(dests) == 0 {
		return res, nil
	}

	var sortedSkills []string
	for s := range validSkills {
		sortedSkills = append(sortedSkills, s)
	}
	sort.Strings(sortedSkills)

	for _, destRel := range dests {
		destAbs := filepath.Join(projectRoot, filepath.FromSlash(destRel))

		// Step A: Prune unmanaged snowflakes in destAbs
		if pathExists(destAbs) {
			destEntries, err := fileutil.ReadDir(destAbs)
			if err == nil {
				for _, de := range destEntries {
					name := de.Name()
					if strings.HasPrefix(name, ".") {
						continue
					}
					if !validSkills[name] {
						// Unmanaged snowflake detected
						rel := filepath.ToSlash(filepath.Join(destRel, name))
						res.Pruned = append(res.Pruned, rel)
						if !dryRun {
							_ = fileutil.RemoveAll(filepath.Join(destAbs, name))
						}
					}
				}
			}
		}

		// Step B: Project valid kernel skills
		if len(sortedSkills) == 0 {
			continue
		}

		if !dryRun {
			if err := fileutil.EnsureDir(destAbs); err != nil {
				return nil, errfmt.Newf("ensure dest dir %s", destRel).Wrap(err)
			}
		}

		for _, skillName := range sortedSkills {
			rel := filepath.ToSlash(filepath.Join(destRel, skillName))
			targetRel, relErr := filepath.Rel(destAbs, filepath.Join(kernelSkillsDir, skillName))
			if relErr != nil {
				targetRel = "../../" + filepath.ToSlash(filepath.Join(paths.ProjectDataDir, paths.SkillsSubdir, skillName))
			} else {
				targetRel = filepath.ToSlash(targetRel)
			}
			linkAbs := filepath.Join(destAbs, skillName)

			// Check existing link/file state
			fi, lerr := os.Lstat(linkAbs)
			if lerr == nil {
				if fi.Mode()&os.ModeSymlink != 0 {
					curTarget, err := os.Readlink(linkAbs)
					if err == nil && filepath.Clean(curTarget) == filepath.Clean(targetRel) {
						res.Skipped = append(res.Skipped, rel)
						continue
					}
				}
				// Broken link or wrong target or non-symlink: update to symlink
				res.Linked = append(res.Linked, rel)
				if !dryRun {
					_ = fileutil.RemoveAll(linkAbs)
					if err := os.Symlink(targetRel, linkAbs); err != nil {
						return nil, errfmt.Newf("create skill symlink %s", rel).Wrap(err)
					}
				}
				continue
			}

			// New link
			res.Linked = append(res.Linked, rel)
			if !dryRun {
				if err := os.Symlink(targetRel, linkAbs); err != nil {
					return nil, errfmt.Newf("create skill symlink %s", rel).Wrap(err)
				}
			}
		}
	}

	return res, nil
}
