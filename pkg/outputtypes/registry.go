package outputtypes

import "strings"

// Definition describes a standard output type produced by zqk.
type Definition struct {
	ID            string
	MIMEType      string
	FileExtension string
	LineDelimited bool
	Structured    bool
}

const (
	IDText    = "text"
	IDJSON    = "json"
	IDJSONL   = "jsonl"
	IDYAML    = "yaml"
	IDCSV     = "csv"
	IDTable   = "table"
	IDJSONRPC = "json-rpc"
	IDStream  = "stream"
	IDIDs     = "ids"
)

const (
	mimeTextUTF8       = "text/plain; charset=utf-8"
	mimeJSONUTF8       = "application/json; charset=utf-8"
	mimeNDJSONUTF8     = "application/x-ndjson; charset=utf-8"
	mimeYAMLUTF8       = "application/yaml; charset=utf-8"
	fileExtLog         = ".log"
	fileExtJSON        = ".json"
	fileExtJSONL       = ".jsonl"
	fileExtYAML        = ".yaml"
	fileExtCSV         = ".csv"
	fileExtText        = ".txt"
	mimeCSVUTF8        = "text/csv; charset=utf-8"
	normalizeAliasYML  = "yml"
	normalizeAliasNDJS = "ndjson"
)

var definitions = map[string]Definition{
	IDText: {
		ID:            IDText,
		MIMEType:      mimeTextUTF8,
		FileExtension: fileExtLog,
		LineDelimited: false,
		Structured:    false,
	},
	IDJSON: {
		ID:            IDJSON,
		MIMEType:      mimeJSONUTF8,
		FileExtension: fileExtJSON,
		LineDelimited: false,
		Structured:    true,
	},
	IDJSONL: {
		ID:            IDJSONL,
		MIMEType:      mimeNDJSONUTF8,
		FileExtension: fileExtJSONL,
		LineDelimited: true,
		Structured:    true,
	},
	IDYAML: {
		ID:            IDYAML,
		MIMEType:      mimeYAMLUTF8,
		FileExtension: fileExtYAML,
		LineDelimited: false,
		Structured:    true,
	},
	IDCSV: {
		ID:            IDCSV,
		MIMEType:      mimeCSVUTF8,
		FileExtension: fileExtCSV,
		LineDelimited: true,
		Structured:    true,
	},
	IDTable: {
		ID:            IDTable,
		MIMEType:      mimeTextUTF8,
		FileExtension: fileExtText,
		LineDelimited: false,
		Structured:    false,
	},
	IDJSONRPC: {
		ID:            IDJSONRPC,
		MIMEType:      mimeJSONUTF8,
		FileExtension: fileExtJSONL,
		LineDelimited: true,
		Structured:    true,
	},
	IDStream: {
		ID:            IDStream,
		MIMEType:      mimeJSONUTF8,
		FileExtension: fileExtJSONL,
		LineDelimited: true,
		Structured:    true,
	},
	IDIDs: {
		ID:            IDIDs,
		MIMEType:      mimeTextUTF8,
		FileExtension: fileExtText,
		LineDelimited: true,
		Structured:    false,
	},
}

// Normalize returns a canonical output type ID.
func Normalize(id string) string {
	v := strings.ToLower(strings.TrimSpace(id))
	switch v {
	case normalizeAliasNDJS:
		return IDJSONL
	case normalizeAliasYML:
		return IDYAML
	default:
		return v
	}
}

// Get returns a canonical output type definition.
func Get(id string) (Definition, bool) {
	def, ok := definitions[Normalize(id)]
	return def, ok
}
