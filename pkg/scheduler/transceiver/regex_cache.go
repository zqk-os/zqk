package transceiver

import (
	"regexp"
	"sync"
)

type cachedRegexpEntry struct {
	re  *regexp.Regexp
	err error
}

var regexpCache sync.Map // map[string]cachedRegexpEntry

func getCachedRegexp(pattern string) (*regexp.Regexp, error) {
	if v, ok := regexpCache.Load(pattern); ok {
		entry := v.(cachedRegexpEntry)
		return entry.re, entry.err
	}

	re, err := regexp.Compile(pattern)
	entry := cachedRegexpEntry{re: re, err: err}
	regexpCache.Store(pattern, entry)
	return re, err
}
