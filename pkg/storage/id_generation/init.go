package id_generation

const emptyValue = ""

func init() {
	// Register default strategies
	_ = RegisterStrategy(NewSequentialStrategy())
	_ = RegisterStrategy(NewUUIDStrategy())
}
