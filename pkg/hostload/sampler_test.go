package hostload

// setSamplerForTest replaces the process sampler. Nil restores the OS sampler on next use.
func setSamplerForTest(s Sampler) {
	samplerMu.Lock()
	defer samplerMu.Unlock()
	sampler = s
}
