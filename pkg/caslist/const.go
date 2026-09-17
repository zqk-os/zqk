package caslist

// TRACK: TDE-1789637124469147000-98c2f37f — community one-shot inventory must
// use the on-disk per-kind CAS index. Remove this package when zcom is a thin
// client of a warm kernel that already paid cobra/spec bring-up.

const (
	processDirName     = ".zqk/process"
	indexFilePrefix    = "."
	indexFileSuffix    = ".index"
	yamlExt            = ".yaml"
	hexHashLen         = 64
	formatJSON         = "json"
	formatYAML         = "yaml"
	formatTable        = "table"
	defaultListFormat  = formatTable
	defaultGetFormat   = formatJSON
	namespaceScopeMode = "community_project"
	verbList           = "list"
	verbGet            = "get"
	verbCount          = "count"
	cmdObject          = "object"
	envZcomProjectRoot = "ZCOM_PROJECT_ROOT"
	envZqkProjectRoot  = "ZQK_PROJECT_ROOT"
	metaKeyIsolation   = "namespace_isolation_active"
	metaKeyScopeMode   = "namespace_scope_mode"
	metaKeyReturned    = "returned_count"
	metaKeyTotal       = "total_count"
	metaKeyCASIndex    = "cas_index"
	outKeyObjects      = "objects"
	outKeyMeta         = "meta"
	outKeyCount        = "count"
	outKeyCountsByKind = "counts_by_kind"
	fieldID            = "id"
	fieldKind          = "kind"
	fieldStatus        = "status"
	fieldTitle         = "title"
	internalDirName    = "_internal"
	kindDocEntry       = "doc_entry"
	kindObjectSpec     = "object_spec"
)
