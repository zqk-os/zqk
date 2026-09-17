package skill

import (
	"fmt"
	"path/filepath"
	"time"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

type DriftIssue struct {
	SkillName   string
	IssueType   string // "MissingInPrimary", "MissingInSecondary", "MtimeMismatch"
	Description string
}

type Auditor struct {
	PrimaryDir   string
	SecondaryDir string
}

func NewDriftAuditor(projectRoot string) *Auditor {
	return &Auditor{
		PrimaryDir:   filepath.Join(projectRoot, ".zqk", "skills"),
		SecondaryDir: filepath.Join(projectRoot, "skills"),
	}
}

func (a *Auditor) Audit() ([]DriftIssue, error) {
	var issues []DriftIssue

	primarySkills, err := a.readSkills(a.PrimaryDir)
	if err != nil && !fileutil.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read primary skills: %w", err)
	}

	secondarySkills, err := a.readSkills(a.SecondaryDir)
	if err != nil && !fileutil.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read secondary skills: %w", err)
	}

	for name, primaryMtime := range primarySkills {
		secondaryMtime, exists := secondarySkills[name]
		if !exists {
			issues = append(issues, DriftIssue{
				SkillName:   name,
				IssueType:   "MissingInSecondary",
				Description: fmt.Sprintf("Skill %s is in %s but missing in %s", name, a.PrimaryDir, a.SecondaryDir),
			})
			continue
		}

		// Check mtime alignment, allow small tolerance if needed but default to equal
		if !primaryMtime.Equal(secondaryMtime) {
			issues = append(issues, DriftIssue{
				SkillName:   name,
				IssueType:   "MtimeMismatch",
				Description: fmt.Sprintf("Skill %s has mtime mismatch (primary: %v, secondary: %v)", name, primaryMtime, secondaryMtime),
			})
		}
	}

	for name := range secondarySkills {
		if _, exists := primarySkills[name]; !exists {
			issues = append(issues, DriftIssue{
				SkillName:   name,
				IssueType:   "MissingInPrimary",
				Description: fmt.Sprintf("Skill %s is in %s but missing in %s", name, a.SecondaryDir, a.PrimaryDir),
			})
		}
	}

	return issues, nil
}

func (a *Auditor) readSkills(dir string) (map[string]time.Time, error) {
	skills := make(map[string]time.Time)

	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return skills, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillMdPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		stat, err := fileutil.Stat(skillMdPath)
		if err == nil {
			skills[entry.Name()] = stat.ModTime()
		}
	}

	return skills, nil
}
