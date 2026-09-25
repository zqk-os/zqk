//go:build !linux && !darwin

package overseer

func setSubreaper() error {
	return nil
}

func isSubreaperSupported() bool {
	return false
}
