package object

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/entitlements"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// FlagElevatedInternal is the object-group elevated access mode flag.
// It means privilege mode, not visibility: internal as a data filter.
// TRACK: BLI-1785930106857898000-94b9a5bc — retire parallel zqk-admin internal tree.
const FlagElevatedInternal = "internal"

// ElevatedInternalRequested reports whether object … --internal was set.
func ElevatedInternalRequested(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	v, err := cmd.Flags().GetBool(FlagElevatedInternal)
	if err != nil {
		// Persistent flag may live on parents.
		v, err = cmd.InheritedFlags().GetBool(FlagElevatedInternal)
		if err != nil {
			return false
		}
	}
	return v
}

// RequireElevatedInternal enforces license-or-admin gate when --internal is set.
// No-op when the flag is unset. Allowed when elevated_object entitlement passes
// or the executable is zqk-admin (DEC transitional carrier).
func RequireElevatedInternal(cmd *cobra.Command) error {
	if !ElevatedInternalRequested(cmd) {
		return nil
	}
	if isZqkAdminExecutable() {
		return nil
	}
	if err := entitlements.CheckEntitlementBundle(cmd.Context(), entitlements.BundleElevatedObject); err != nil {
		return errfmt.Newf("elevated object access (--internal) denied").Wrap(err)
	}
	return nil
}

func isZqkAdminExecutable() bool {
	base := strings.ToLower(strings.TrimSpace(brand.ExecutableName()))
	return base == "zqk-admin" || strings.HasSuffix(base, "zqk-admin.exe")
}

// ShouldSkipInternalKind reports whether all-kinds list/count should omit a kind.
// When elevated is true, internal kinds are included.
func ShouldSkipInternalKind(kind string, elevated bool) bool {
	if elevated {
		return false
	}
	return isInternalKind(kind)
}
