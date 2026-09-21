package authcred

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
)

var rbacSpecs stampmemo.Table[struct{}]

// RequireRBACSpecs reports whether account and role object specs exist (flat or domain bucket).
// Result is retained until the specs directory mtime changes.
func RequireRBACSpecs(projectRoot string) error {
	if strings.TrimSpace(projectRoot) == "" {
		return errfmt.Errorf("object specs directory missing")
	}
	specsDir := paths.ObjectSpecsDir(projectRoot)
	_, err := rbacSpecs.Load(projectRoot, stampmemo.Of(specsDir), func() (struct{}, error) {
		if _, err := paths.FindObjectSpecFile(specsDir, objects.KindAccount); err != nil {
			return struct{}{}, err
		}
		if _, err := paths.FindObjectSpecFile(specsDir, objects.KindRole); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}
