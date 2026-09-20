package provider

// QueryBuilder provides a fluent API for constructing queries across backends
type QueryBuilder interface {
	Match(pattern string) QueryBuilder
	Where(condition string) QueryBuilder
	Return(fields ...string) QueryBuilder
	Limit(n int) QueryBuilder
	OrderBy(field string, direction string) QueryBuilder
	Build() Query
}

// QueryTranslator translates queries between different query languages
type QueryTranslator interface {
	Translate(query Query, targetLanguage QueryLanguage) (Query, error)
}
