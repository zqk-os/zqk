package printer

// TablePrintable defines an interface for objects that can format themselves as tables.
// This aligns with ZQK's object-first principles, allowing objects to define their own representation.
type TablePrintable interface {
	FormatTable() ([]byte, error)
}

// DataWrapper allows an object to provide the underlying data for structured formats (JSON/YAML).
// This is used when a wrapper object implements TablePrintable but contains underlying data
// that should be used directly for structured formats.
type DataWrapper interface {
	Unwrap() any
}
