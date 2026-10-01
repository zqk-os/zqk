package integrity

import (
	"path/filepath"
	"strings"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// LegacyCustomRulesGate asserts Go customRuleValidators bodies stay deleted (compose-only).
func LegacyCustomRulesGate(projectRoot string) (ok bool, note string) {
	p := filepath.Join(projectRoot, "pkg", "validation", "go_validator_custom_rules.go")
	if _, err := fileutil.Stat(p); err == nil {
		return false, "pkg/validation/go_validator_custom_rules.go must stay deleted (use compose overlays)"
	}
	customGo := filepath.Join(projectRoot, "pkg", "validation", "go_validator_custom.go")
	b, err := fileutil.ReadFile(customGo)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return true, "binary distribution or workspace without Go validation sources"
		}
		return false, err.Error()
	}
	s := string(b)
	if strings.Contains(s, "customRuleValidators") {
		return false, "legacy customRuleValidators map present in go_validator_custom.go"
	}
	return true, "composed overlays only (no Go custom rule bodies)"
}

// MembraneCoverageGate checks mutator files reference kernelcas Run* (string scan).
func MembraneCoverageGate(projectRoot string) (ok bool, gaps []string) {
	pathsToCheck := []string{
		filepath.Join("pkg", "storage", "object_storage_file_create.go"),
		filepath.Join("pkg", "storage", "object_storage_file_update.go"),
		filepath.Join("pkg", "storage", "object_storage_file_delete.go"),
		filepath.Join("pkg", "storage", "object_storage_file_transaction.go"),
		filepath.Join("pkg", "storage", "object_storage_graph_crud.go"),
		filepath.Join("cmd", "zqk", "system", "state_restore.go"),
		filepath.Join("cmd", "zqk", "system", "check_impl_output.go"),
	}
	firstFile := filepath.Join(projectRoot, pathsToCheck[0])
	if _, err := fileutil.Stat(firstFile); fileutil.IsNotExist(err) {
		return true, nil
	}
	needles := []string{"kernelcas.Run", "denyCoreKernelHardDelete", "kernelcas.IsCommit"}
	for _, rel := range pathsToCheck {
		full := filepath.Join(projectRoot, rel)
		b, err := fileutil.ReadFile(full)
		if err != nil {
			gaps = append(gaps, rel+": missing")
			continue
		}
		s := string(b)
		hit := false
		for _, n := range needles {
			if strings.Contains(s, n) {
				hit = true
				break
			}
		}
		if !hit {
			gaps = append(gaps, rel)
		}
	}
	return len(gaps) == 0, gaps
}
