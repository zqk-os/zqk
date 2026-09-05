package functional

// MapGetPtr looks up a key in a map of pointers and returns the value.
// It returns nil if the key is not present or the value is nil.
func MapGetPtr[K comparable, V any](m map[K]*V, key K) *V {
	if m == nil {
		return nil
	}
	return m[key]
}

// MapHasPtr checks if a key is present in a map of pointers AND the value is not nil.
func MapHasPtr[K comparable, V any](m map[K]*V, key K) bool {
	return MapGetPtr(m, key) != nil
}

// MapGetOrDefault returns the value for the key if it exists, otherwise the default value.
func MapGetOrDefault[K comparable, V any](m map[K]V, key K, def V) V {
	if m == nil {
		return def
	}
	if v, ok := m[key]; ok {
		return v
	}
	return def
}

// MapKeys returns a slice of keys from the map.
func MapKeys[K comparable, V any](m map[K]V) []K {
	if m == nil {
		return nil
	}
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// MapValues returns a slice of values from the map.
func MapValues[K comparable, V any](m map[K]V) []V {
	if m == nil {
		return nil
	}
	values := make([]V, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}
