package search

import "unicode/utf8"

// DefaultMaxTokens is the default token budget for search results (approx 4000 tokens).
const DefaultMaxTokens = 4000

// DefaultMaxMatches is the default upper bound on matches returned.
const DefaultMaxMatches = 100

// EstimateTokens calculates an approximate token count for a string.
// On average in technical code and prose, 1 token is roughly 3.7 to 4 UTF-8 bytes / characters.
// This fast heuristic avoids expensive BPE tokenization while remaining conservative.
func EstimateTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	runeCount := utf8.RuneCountInString(s)
	// Base estimation: 1 token per 3.5 characters + punctuation/whitespace bias
	tokens := (runeCount * 10) / 35
	if tokens == 0 {
		return 1
	}
	return tokens
}

// EstimateMatchTokens estimates token cost for a single Match struct including context.
func EstimateMatchTokens(m Match) int {
	chars := len(m.File) + len(m.LineContent) + len(m.SymbolKind) + len(m.SymbolName) + len(m.Receiver) + 30
	for _, c := range m.ContextBefore {
		chars += len(c) + 5
	}
	for _, c := range m.ContextAfter {
		chars += len(c) + 5
	}
	return (chars * 10) / 35
}
