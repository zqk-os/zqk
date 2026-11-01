package prime

import "time"

const (
	// DefaultMaxPrimingDuration is the target boundary (<5s) from CRIT-1793495698371444000-1c17aaef.
	DefaultMaxPrimingDuration = 5 * time.Second

	// ASTSymbolsCacheFile is the cached AST index file name under .zqk/cache/.
	ASTSymbolsCacheFile = "ast_symbols.json"

	// TrigramIndexCacheFile is the cached trigram index file name under .zqk/cache/.
	TrigramIndexCacheFile = "trigram.idx"

	// PrimeManifestFile stores the metadata manifest of the last prime run.
	PrimeManifestFile = "prime_manifest.json"

	// FormatText specifies human-readable terminal output.
	FormatText = "text"

	// FormatJSON specifies JSON output format.
	FormatJSON = "json"

	// KindStruct indicates a Go struct symbol.
	KindStruct = "struct"

	// KindInterface indicates a Go interface symbol.
	KindInterface = "interface"

	// KindFunc indicates a Go function symbol.
	KindFunc = "func"

	// KindMethod indicates a Go method symbol.
	KindMethod = "method"

	// KindType indicates a Go type definition or alias.
	KindType = "type"

	// KindConst indicates a Go constant declaration.
	KindConst = "const"

	// KindVar indicates a Go variable declaration.
	KindVar = "var"

	// LockDir is the subdirectory for temporary operation locks inside .zqk.
	LockDir = "lock"

	// PrimeLockFile is the lock file name stored inside .zqk/lock/.
	PrimeLockFile = "prime.lock"

	// TargetDurationDesc describes the performance SLA.
	TargetDurationDesc = "<5.0s"
)
