package io

import (
	"crypto/sha256"
	"encoding/hex"
	"html"
	"regexp"
	"strings"
)

var (
	reScript   = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	reStyle    = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	reNav      = regexp.MustCompile(`(?is)<nav\b[^>]*>.*?</nav>`)
	reFooter   = regexp.MustCompile(`(?is)<footer\b[^>]*>.*?</footer>`)
	reHeader   = regexp.MustCompile(`(?is)<header\b[^>]*>.*?</header>`)
	reNoScript = regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript>`)
	reIframe   = regexp.MustCompile(`(?is)<iframe\b[^>]*>.*?</iframe>`)
	reSvg      = regexp.MustCompile(`(?is)<svg\b[^>]*>.*?</svg>`)
	reComment  = regexp.MustCompile(`(?is)<!--.*?-->`)

	// Headings
	reH1 = regexp.MustCompile(`(?is)<h1\b[^>]*>(.*?)</h1>`)
	reH2 = regexp.MustCompile(`(?is)<h2\b[^>]*>(.*?)</h2>`)
	reH3 = regexp.MustCompile(`(?is)<h3\b[^>]*>(.*?)</h3>`)
	reH4 = regexp.MustCompile(`(?is)<h4\b[^>]*>(.*?)</h4>`)
	reH5 = regexp.MustCompile(`(?is)<h5\b[^>]*>(.*?)</h5>`)
	reH6 = regexp.MustCompile(`(?is)<h6\b[^>]*>(.*?)</h6>`)

	// Formatting
	reA      = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']*)["'][^>]*>(.*?)</a>`)
	reStrong = regexp.MustCompile(`(?is)<(strong|b)\b[^>]*>(.*?)</(strong|b)>`)
	reEm     = regexp.MustCompile(`(?is)<(em|i)\b[^>]*>(.*?)</(em|i)>`)
	reCode   = regexp.MustCompile(`(?is)<code\b[^>]*>(.*?)</code>`)
	rePre    = regexp.MustCompile(`(?is)<pre\b[^>]*>(.*?)</pre>`)
	reLi     = regexp.MustCompile(`(?is)<li\b[^>]*>(.*?)</li>`)
	reBlock  = regexp.MustCompile(`(?is)<(p|div|section|article|tr|blockquote)\b[^>]*>`)
	reBr     = regexp.MustCompile(`(?is)<br\s*/?>`)
	reHr     = regexp.MustCompile(`(?is)<hr\s*/?>`)
	reAllTag = regexp.MustCompile(`(?is)<[^>]+>`)

	reMultiNL     = regexp.MustCompile(`\n{3,}`)
	reMultiSpaces = regexp.MustCompile(`[ \t]+`)
)

// ComputeSHA256 returns the lowercase hexadecimal SHA256 digest of data.
func ComputeSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ExtractCleanContent strips scripts, styles, navs, footers, headers, and extracts clean Markdown/plaintext.
func ExtractCleanContent(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}

	// 1. Strip non-content and layout chrome
	s := reScript.ReplaceAllString(input, "")
	s = reStyle.ReplaceAllString(s, "")
	s = reNav.ReplaceAllString(s, "")
	s = reFooter.ReplaceAllString(s, "")
	s = reHeader.ReplaceAllString(s, "")
	s = reNoScript.ReplaceAllString(s, "")
	s = reIframe.ReplaceAllString(s, "")
	s = reSvg.ReplaceAllString(s, "")
	s = reComment.ReplaceAllString(s, "")

	// 2. Pre-process blocks and preformatted code
	s = rePre.ReplaceAllStringFunc(s, func(m string) string {
		inner := rePre.FindStringSubmatch(m)
		if len(inner) > 1 {
			codeText := reAllTag.ReplaceAllString(inner[1], "")
			return "\n```\n" + strings.TrimSpace(codeText) + "\n```\n"
		}
		return m
	})

	// 3. Headings to Markdown
	s = reH1.ReplaceAllString(s, "\n\n# $1\n\n")
	s = reH2.ReplaceAllString(s, "\n\n## $1\n\n")
	s = reH3.ReplaceAllString(s, "\n\n### $1\n\n")
	s = reH4.ReplaceAllString(s, "\n\n#### $1\n\n")
	s = reH5.ReplaceAllString(s, "\n\n##### $1\n\n")
	s = reH6.ReplaceAllString(s, "\n\n###### $1\n\n")

	// 4. Links and text styling
	s = reA.ReplaceAllString(s, "[$2]($1)")
	s = reStrong.ReplaceAllString(s, "**$2**")
	s = reEm.ReplaceAllString(s, "*$2*")
	s = reCode.ReplaceAllString(s, "`$1`")
	s = reLi.ReplaceAllString(s, "\n* $1")

	// 5. Block dividers and breaks
	s = reBr.ReplaceAllString(s, "\n")
	s = reHr.ReplaceAllString(s, "\n---\n")
	s = reBlock.ReplaceAllString(s, "\n\n")

	// 6. Strip all remaining HTML tags
	s = reAllTag.ReplaceAllString(s, "")

	// 7. Unescape HTML entities
	s = html.UnescapeString(s)

	// 8. Clean up whitespace: trim each line, reduce excessive empty lines
	lines := strings.Split(s, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmed := strings.TrimRight(reMultiSpaces.ReplaceAllString(line, " "), " ")
		cleanedLines = append(cleanedLines, trimmed)
	}

	result := strings.Join(cleanedLines, "\n")
	result = reMultiNL.ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}
