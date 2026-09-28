//go:build !zqk_omit_workpack

package objects

func init() {
	// Same relative directory as workpack.SpecDir. This package cannot import
	// workpack: that package already imports objects.
	AddModuleSpecRoot("packs/work/specs")
}
