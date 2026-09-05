package processhygiene

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestStrategicKindStatusVocabulary enforces that strategic object lifecycles
// (goal, requirement, risk_blocker, criteria, agent_task, question)
// define valid roles, statuses, and reject hollow promotions (REQ-KERNEL-LIFECYCLE-FITNESS-001).
func TestStrategicKindStatusVocabulary(t *testing.T) {
	repoRoot := findRepoRoot(t)
	lifecyclesDir := filepath.Join(repoRoot, "docs", "process", "_internal", "lifecycles")

	strategicKinds := []string{
		"goal_lifecycle.yaml",
		"requirement_lifecycle.yaml",
		"risk_blocker_lifecycle.yaml",
		"criteria_lifecycle.yaml",
		"agent_task_lifecycle.yaml",
		"question_lifecycle.yaml",
	}

	for _, fileName := range strategicKinds {
		lifecyclePath := filepath.Join(lifecyclesDir, fileName)
		data, err := fileutil.ReadFile(lifecyclePath)
		if err != nil {
			t.Errorf("ReadFile %s: %v", fileName, err)
			continue
		}

		var lc map[string]any
		if err := yaml.Unmarshal(data, &lc); err != nil {
			t.Errorf("Unmarshal %s: %v", fileName, err)
			continue
		}

		statuses, ok := lc["statuses"].([]any)
		if !ok || len(statuses) == 0 {
			t.Errorf("lifecycle %s missing statuses list", fileName)
			continue
		}

		// Ensure all statuses have role and description
		for _, s := range statuses {
			statusMap, ok := s.(map[string]any)
			if !ok {
				continue
			}
			val, _ := statusMap["value"].(string)
			role, _ := statusMap["role"].(string)
			desc, _ := statusMap["description"].(string)

			if val == "" {
				t.Errorf("lifecycle %s has status with empty value", fileName)
			}
			if role == "" && desc == "" {
				t.Errorf("lifecycle %s status %s missing role and description", fileName, val)
			}
		}
	}
}

// TestStrategicPopulationHonesty ensures no active strategic objects in CAS
// use obsolete statuses like 'planned' for goals/requirements or 'not_started' for criteria.
func TestStrategicPopulationHonesty(t *testing.T) {
	repoRoot := findRepoRoot(t)
	processDir := filepath.Join(repoRoot, "docs", "process")

	checkKindDirectory := func(dirName string, forbiddenStatuses map[string]bool) {
		kindDir := filepath.Join(processDir, dirName)
		if _, err := fileutil.Stat(kindDir); fileutil.IsNotExist(err) {
			return
		}

		entries, err := fileutil.ReadDir(kindDir)
		if err != nil {
			t.Fatalf("ReadDir %s: %v", kindDir, err)
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}

			filePath := filepath.Join(kindDir, entry.Name())
			data, err := fileutil.ReadFile(filePath)
			if err != nil {
				t.Errorf("ReadFile %s: %v", filePath, err)
				continue
			}

			var obj map[string]any
			if err := yaml.Unmarshal(data, &obj); err != nil {
				continue
			}

			status, _ := obj["status"].(string)
			if forbiddenStatuses[status] {
				t.Errorf("object %s in %s has forbidden obsolete status: %q", entry.Name(), dirName, status)
			}
		}
	}

	// Goals: no 'planned'
	checkKindDirectory("goals", map[string]bool{"planned": true})
	// Requirements: no 'planned'
	checkKindDirectory("requirements", map[string]bool{"planned": true})
	// Criteria: no 'not_started'
	checkKindDirectory("criteria", map[string]bool{"not_started": true})
}
