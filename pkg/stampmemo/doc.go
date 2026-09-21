// Package stampmemo is a stamp-invalidated memo: the key is a stable identity,
// the stamp is the generation.
//
// There is no size limit, LRU, or TTL. Maps grow with distinct keys, not with
// stamp changes — Load overwrites the same entry when the stamp moves.
//
// Key by a closed set (project root, config path, kind, spec path). Do not key
// by an unbounded stream (object ids, request ids, every path from a walk).
// A daemon may hold the table for its lifetime; that is safe only while the
// key cardinality stays that closed set.
//
// Delete(key) after an out-of-band write whose mtime you do not want to wait
// for. Reset forgets every key when the key space itself is invalid (tests
// that chdir, SpecLoader.ClearCache). Reset is not a capacity valve.
package stampmemo
