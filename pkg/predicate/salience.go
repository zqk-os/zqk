package predicate

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// DefaultStopWords contains closed-class English stop words and universal punctuation.
var DefaultStopWords = map[string]struct{}{
	"a": {}, "about": {}, "above": {}, "after": {}, "again": {}, "against": {},
	"all": {}, "am": {}, "an": {}, "and": {}, "any": {}, "are": {}, "aren't": {},
	"as": {}, "at": {}, "be": {}, "because": {}, "been": {}, "before": {}, "being": {},
	"below": {}, "between": {}, "both": {}, "but": {}, "by": {}, "can": {}, "cannot": {},
	"could": {}, "couldn't": {}, "did": {}, "didn't": {}, "do": {}, "does": {}, "doesn't": {},
	"doing": {}, "don't": {}, "down": {}, "during": {}, "each": {}, "few": {}, "for": {},
	"from": {}, "further": {}, "had": {}, "hadn't": {}, "has": {}, "hasn't": {}, "have": {},
	"haven't": {}, "having": {}, "he": {}, "he'd": {}, "he'll": {}, "he's": {}, "her": {},
	"here": {}, "here's": {}, "hers": {}, "herself": {}, "him": {}, "himself": {}, "his": {},
	"how": {}, "how's": {}, "i": {}, "i'd": {}, "i'll": {}, "i'm": {}, "i've": {}, "if": {},
	"in": {}, "into": {}, "is": {}, "isn't": {}, "it": {}, "it's": {}, "its": {}, "itself": {},
	"let's": {}, "me": {}, "more": {}, "most": {}, "mustn't": {}, "my": {}, "myself": {},
	"no": {}, "nor": {}, "not": {}, "of": {}, "off": {}, "on": {}, "once": {}, "only": {},
	"or": {}, "other": {}, "ought": {}, "our": {}, "ours": {}, "ourselves": {}, "out": {},
	"over": {}, "own": {}, "same": {}, "shan't": {}, "she": {}, "she'd": {}, "she'll": {},
	"she's": {}, "should": {}, "shouldn't": {}, "so": {}, "some": {}, "such": {}, "than": {},
	"that": {}, "that's": {}, "the": {}, "their": {}, "theirs": {}, "them": {}, "themselves": {},
	"then": {}, "there": {}, "there's": {}, "these": {}, "they": {}, "they'd": {}, "they'll": {},
	"they're": {}, "they've": {}, "this": {}, "those": {}, "through": {}, "to": {}, "too": {},
	"under": {}, "until": {}, "up": {}, "very": {}, "was": {}, "wasn't": {}, "we": {},
	"we'd": {}, "we'll": {}, "we're": {}, "we've": {}, "were": {}, "weren't": {}, "what": {},
	"what's": {}, "when": {}, "when's": {}, "where": {}, "where's": {}, "which": {}, "while": {},
	"who": {}, "who's": {}, "whom": {}, "why": {}, "why's": {}, "with": {}, "won't": {},
	"would": {}, "wouldn't": {}, "you": {}, "you'd": {}, "you'll": {}, "you're": {}, "you've": {},
	// Process/meta words with near-zero discriminatory power across objects
	"ensure": {}, "properly": {}, "must": {}, "will": {}, "shall": {},
	"required": {}, "update": {}, "perform": {}, "verify": {}, "check": {},
}

// Tokenize extracts lowercase alphanumeric words from text.
func Tokenize(text string) []string {
	var tokens []string
	var cur strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			cur.WriteRune(unicode.ToLower(r))
		} else {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// Stem applies a deterministic, simplified Porter-style stemmer to normalize words.
func Stem(w string) string {
	w = strings.ToLower(strings.TrimSpace(w))
	if len(w) <= 3 {
		return w
	}

	// Step 1: plural and past tense endings
	if strings.HasSuffix(w, "sses") {
		w = w[:len(w)-2]
	} else if strings.HasSuffix(w, "ies") {
		w = w[:len(w)-2]
	} else if strings.HasSuffix(w, "ss") {
		// keep ss
	} else if strings.HasSuffix(w, "s") && len(w) > 3 {
		w = w[:len(w)-1]
	}

	if strings.HasSuffix(w, "eed") {
		if len(w) > 4 {
			w = w[:len(w)-1]
		}
	} else if strings.HasSuffix(w, "ed") && len(w) > 4 {
		w = w[:len(w)-2]
	} else if strings.HasSuffix(w, "ing") && len(w) > 5 {
		w = w[:len(w)-3]
	}

	// Step 2: derivational suffixes
	if strings.HasSuffix(w, "ational") && len(w) > 8 {
		w = w[:len(w)-7]
	} else if strings.HasSuffix(w, "tional") && len(w) > 7 {
		w = w[:len(w)-6]
	} else if strings.HasSuffix(w, "ation") && len(w) > 6 {
		w = w[:len(w)-5]
	} else if (strings.HasSuffix(w, "tion") || strings.HasSuffix(w, "sion")) && len(w) > 6 {
		w = w[:len(w)-3] // "connection" -> "connect"
	} else if (strings.HasSuffix(w, "izer") || strings.HasSuffix(w, "iser")) && len(w) > 6 {
		w = w[:len(w)-4]
	} else if (strings.HasSuffix(w, "alism") || strings.HasSuffix(w, "ality")) && len(w) > 7 {
		w = w[:len(w)-5]
	} else if strings.HasSuffix(w, "ment") && len(w) > 6 {
		w = w[:len(w)-4]
	} else if strings.HasSuffix(w, "ness") && len(w) > 6 {
		w = w[:len(w)-4]
	} else if (strings.HasSuffix(w, "able") || strings.HasSuffix(w, "ible")) && len(w) > 6 {
		w = w[:len(w)-4]
	} else if strings.HasSuffix(w, "ate") && len(w) > 6 {
		w = w[:len(w)-3]
	} else if strings.HasSuffix(w, "at") && len(w) > 5 {
		w = w[:len(w)-2]
	}

	if strings.HasSuffix(w, "ic") && len(w) > 5 {
		w = w[:len(w)-2]
	}

	// Step 3: tidy trailing 'e' if long enough
	if strings.HasSuffix(w, "e") && len(w) > 4 {
		w = w[:len(w)-1]
	}

	return w
}

type termScore struct {
	stem  string
	score float64
}

// ExtractSalientStems computes sublinear term frequencies (1 + ln(count))
// for all non-stopwords in text, returning up to topK stems sorted by salience.
func ExtractSalientStems(text string, topK int) []string {
	tokens := Tokenize(text)
	counts := make(map[string]int)

	for _, tok := range tokens {
		if _, isStop := DefaultStopWords[tok]; isStop {
			continue
		}
		if len(tok) < 3 {
			continue
		}
		s := Stem(tok)
		if len(s) < 3 {
			continue
		}
		counts[s]++
	}

	if len(counts) == 0 {
		return nil
	}

	scores := make([]termScore, 0, len(counts))
	for stem, count := range counts {
		// Sublinear term frequency: 1 + ln(count)
		score := 1.0 + math.Log(float64(count))
		scores = append(scores, termScore{stem: stem, score: score})
	}

	sort.Slice(scores, func(i, j int) bool {
		if scores[i].score == scores[j].score {
			return scores[i].stem < scores[j].stem
		}
		return scores[i].score > scores[j].score
	})

	if topK > len(scores) || topK <= 0 {
		topK = len(scores)
	}

	result := make([]string, topK)
	for i := 0; i < topK; i++ {
		result[i] = scores[i].stem
	}
	return result
}

// CalculateTitleBodyOverlap computes the Szymkiewicz–Simpson overlap coefficient
// between title stems and the salient stems of the body text.
// Overlap = |TitleStems ∩ BodyStems| / min(|TitleStems|, 1)
func CalculateTitleBodyOverlap(title, body string, topK int) (overlap float64, sharedStems []string) {
	titleTokens := Tokenize(title)
	titleStems := make(map[string]struct{})
	for _, tok := range titleTokens {
		if _, isStop := DefaultStopWords[tok]; isStop {
			continue
		}
		if len(tok) < 3 {
			continue
		}
		titleStems[Stem(tok)] = struct{}{}
	}

	if len(titleStems) == 0 {
		return 0, nil
	}

	bodyStems := ExtractSalientStems(body, topK)
	if len(bodyStems) == 0 {
		return 0, nil
	}

	bodySet := make(map[string]struct{}, len(bodyStems))
	for _, s := range bodyStems {
		bodySet[s] = struct{}{}
	}

	for ts := range titleStems {
		if _, exists := bodySet[ts]; exists {
			sharedStems = append(sharedStems, ts)
		}
	}
	sort.Strings(sharedStems)

	overlap = float64(len(sharedStems)) / float64(len(titleStems))
	return overlap, sharedStems
}

// VerifyTitleBodyCohesion deterministically proves whether a title contains at least
// minSharedStems salient keywords from the body content.
func VerifyTitleBodyCohesion(title, body string, minSharedStems int) (bool, []string) {
	if minSharedStems <= 0 {
		minSharedStems = 1
	}
	_, shared := CalculateTitleBodyOverlap(title, body, 25)
	return len(shared) >= minSharedStems, shared
}
