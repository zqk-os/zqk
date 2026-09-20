package when

// PtrOr returns the dereferenced pointer value if non-nil, otherwise it returns the provided default value.
func PtrOr[T any](ptr *T, def T) T {
	if ptr != nil {
		return *ptr
	}
	return def
}

// PtrOrElse returns the dereferenced pointer value if non-nil, otherwise it executes fn and returns its result.
func PtrOrElse[T any](ptr *T, fn func() T) T {
	if ptr != nil {
		return *ptr
	}
	if fn != nil {
		return fn()
	}
	var zero T
	return zero
}

// Ptr returns a pointer to the given value.
func Ptr[T any](v T) *T {
	return &v
}
