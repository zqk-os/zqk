package id_generation

const emptyValue = ""

func init() {
	// Register default strategies
	RegisterStrategy(NewSequentialStrategy())
	RegisterStrategy(NewUUIDStrategy())
}
