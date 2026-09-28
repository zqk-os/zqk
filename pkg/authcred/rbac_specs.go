package authcred

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
)

// rbacSpecs is keyed by project root. Stamp is the account+role spec files.
var rbacSpecs stampmemo.Table[struct{}]

// RequireRBACSpecs reports whether account and role object specs exist.
// The kernel tree wins. A linked pack directory is used when the kernel has no file.
// Result is retained until the specs directory or a registered pack spec root changes.
func RequireRBACSpecs(projectRoot string) error {
	if strings.TrimSpace(projectRoot) == "" {
		return errfmt.Errorf("object specs directory missing")
	}
	specsDir := paths.ObjectSpecsDir(projectRoot)
	stamp := stampmemo.Combine(stampmemo.Of(specsDir), stampmemo.OfAll(objects.ExtraSpecRoots()...))
	_, err := rbacSpecs.Load(projectRoot, stamp, func() (struct{}, error) {
		if _, err := objects.FindRegisteredSpecFile(specsDir, objects.KindAccount); err != nil {
			return struct{}{}, err
		}
		if _, err := objects.FindRegisteredSpecFile(specsDir, objects.KindRole); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}
