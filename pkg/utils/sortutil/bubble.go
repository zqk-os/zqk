package sortutil

// BubbleSort performs an in-place bubble sort on the provided slice.
// It uses the less function to determine the order of elements.
// This is provided as a zero-dependency sorting utility for small slices.
func BubbleSort[T any](slice []T, less func(a, b T) bool) {
	n := len(slice)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if less(slice[j], slice[i]) {
				slice[i], slice[j] = slice[j], slice[i]
			}
		}
	}
}
