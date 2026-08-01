package storage

// DeferredHashManagerCalculateHashForTest exposes (*DeferredHashManager).calculateHash for external tests.
func DeferredHashManagerCalculateHashForTest(dhm *DeferredHashManager, content []byte) string {
	return dhm.calculateHash(content)
}
