package search

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
)

// SupportedASTExtensions returns the canonical file extensions supported by
// ZQK's zero-dependency in-process polyglot AST engine (GH-1219).
func SupportedASTExtensions() []string {
	return []string{".go", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".py", ".rs"}
}

type polyglotSymbol struct {
	Kind     string
	Name     string
	Receiver string
	Line     int // 1-based
	Column   int // 1-based
}

var (
	// TypeScript / JavaScript declarations
	reTSClass     = regexp.MustCompile(`^\s*(?:export\s+(?:default\s+)?)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	reTSInterface = regexp.MustCompile(`^\s*(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	reTSType      = regexp.MustCompile(`^\s*(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*=`)
	reTSEnum      = regexp.MustCompile(`^\s*(?:export\s+)?(?:const\s+)?enum\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	reTSFunc      = regexp.MustCompile(`^\s*(?:export\s+(?:default\s+)?)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*\(`)
	reTSArrowFn   = regexp.MustCompile(`^\s*(?:export\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)(?:\s*:\s*[^=]+)?\s*=\s*(?:async\s+)?(?:\([^)]*\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*(?::\s*[^=]+)?=>`)
	reTSMethod    = regexp.MustCompile(`^\s+(?:(?:public|private|protected|static|readonly|override|abstract|async|get|set)\s+)*([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>]*>)?\s*\([^)]*\)\s*(?::\s*[^{]+)?\{`)

	// Python declarations
	rePyClass = regexp.MustCompile(`^(\s*)class\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:\(|:)`)
	rePyDef   = regexp.MustCompile(`^(\s*)(?:async\s+)?def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)

	// Rust declarations
	reRsStruct = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?struct\s+([A-Za-z_][A-Za-z0-9_]*)`)
	reRsEnum   = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?enum\s+([A-Za-z_][A-Za-z0-9_]*)`)
	reRsTrait  = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:unsafe\s+)?trait\s+([A-Za-z_][A-Za-z0-9_]*)`)
	reRsType   = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?type\s+([A-Za-z_][A-Za-z0-9_]*)`)
	reRsImpl   = regexp.MustCompile(`^\s*(?:unsafe\s+)?impl(?:<[^>]*>)?\s+(?:[A-Za-z_][A-Za-z0-9_:<>]*\s+for\s+)?([A-Za-z_][A-Za-z0-9_]*)`)
	reRsFn     = regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:const\s+)?(?:async\s+)?(?:unsafe\s+)?(?:extern\s+"[^"]+"\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)`)
)

var tsMethodKeywords = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"return": true, "else": true, "try": true, "finally": true, "do": true,
	"function": true, "class": true, "interface": true, "type": true,
}

func isPolyglotASTExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".py", ".rs":
		return true
	default:
		return false
	}
}

func polyglotASTSearchFile(filePath string, data []byte, opts SearchOptions) ([]Match, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	lines := bytes.Split(data, []byte("\n"))

	var syms []polyglotSymbol
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		syms = extractTypeScriptJSSymbols(lines)
	case ".py":
		syms = extractPythonSymbols(lines)
	case ".rs":
		syms = extractRustSymbols(lines)
	default:
		return nil, nil
	}

	var queryRegex *regexp.Regexp
	if opts.Regex && opts.Query != "" {
		re, err := regexp.Compile(opts.Query)
		if err != nil {
			return nil, err
		}
		queryRegex = re
	}

	numLines := len(lines)
	var matches []Match
	for _, sym := range syms {
		if !matchASTKind(opts.ASTKind, sym.Kind) {
			continue
		}
		if opts.ASTReceiver != "" && (sym.Receiver == "" || !matchASTReceiver(opts.ASTReceiver, sym.Receiver, opts.CaseInsensitive)) {
			continue
		}
		if !matchASTName(opts.Query, sym.Name, queryRegex, opts.CaseInsensitive) {
			continue
		}

		lineIdx := sym.Line - 1
		lineContent := ""
		if lineIdx >= 0 && lineIdx < numLines {
			lineContent = string(lines[lineIdx])
		}
		before, after := collectASTContext(lines, lineIdx, opts.ContextLines)

		matches = append(matches, Match{
			File:          filePath,
			Line:          sym.Line,
			Column:        sym.Column,
			EndLine:       sym.Line,
			LineContent:   lineContent,
			ContextBefore: before,
			ContextAfter:  after,
			SymbolKind:    sym.Kind,
			SymbolName:    sym.Name,
			Receiver:      sym.Receiver,
		})
	}
	return matches, nil
}

func extractTypeScriptJSSymbols(lines [][]byte) []polyglotSymbol {
	var syms []polyglotSymbol
	var currentClass string
	classDepth := 0
	braceDepth := 0

	for i, raw := range lines {
		line := string(raw)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		lineNo := i + 1
		if m := reTSClass.FindStringSubmatchIndex(line); m != nil {
			name := line[m[2]:m[3]]
			syms = append(syms, polyglotSymbol{Kind: "class", Name: name, Line: lineNo, Column: m[2] + 1})
			currentClass = name
			classDepth = braceDepth + 1
		} else if m := reTSInterface.FindStringSubmatchIndex(line); m != nil {
			syms = append(syms, polyglotSymbol{Kind: "interface", Name: line[m[2]:m[3]], Line: lineNo, Column: m[2] + 1})
		} else if m := reTSType.FindStringSubmatchIndex(line); m != nil {
			syms = append(syms, polyglotSymbol{Kind: "type", Name: line[m[2]:m[3]], Line: lineNo, Column: m[2] + 1})
		} else if m := reTSEnum.FindStringSubmatchIndex(line); m != nil {
			syms = append(syms, polyglotSymbol{Kind: "enum", Name: line[m[2]:m[3]], Line: lineNo, Column: m[2] + 1})
		} else if m := reTSFunc.FindStringSubmatchIndex(line); m != nil {
			syms = append(syms, polyglotSymbol{Kind: "func", Name: line[m[2]:m[3]], Line: lineNo, Column: m[2] + 1})
		} else if m := reTSArrowFn.FindStringSubmatchIndex(line); m != nil {
			syms = append(syms, polyglotSymbol{Kind: "func", Name: line[m[2]:m[3]], Line: lineNo, Column: m[2] + 1})
		} else if currentClass != "" && braceDepth == classDepth {
			if m := reTSMethod.FindStringSubmatchIndex(line); m != nil {
				name := line[m[2]:m[3]]
				if !tsMethodKeywords[name] {
					syms = append(syms, polyglotSymbol{Kind: "method", Name: name, Receiver: currentClass, Line: lineNo, Column: m[2] + 1})
				}
			}
		}

		braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
		if braceDepth < 0 {
			braceDepth = 0
		}
		if currentClass != "" && braceDepth < classDepth {
			currentClass = ""
			classDepth = 0
		}
	}
	return syms
}

func extractPythonSymbols(lines [][]byte) []polyglotSymbol {
	var syms []polyglotSymbol
	type classScope struct {
		name   string
		indent int
	}
	var stack []classScope

	for i, raw := range lines {
		line := string(raw)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		lineNo := i + 1
		if m := rePyClass.FindStringSubmatchIndex(line); m != nil {
			indent := len(line[m[2]:m[3]])
			name := line[m[4]:m[5]]
			for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
				stack = stack[:len(stack)-1]
			}
			syms = append(syms, polyglotSymbol{Kind: "class", Name: name, Line: lineNo, Column: m[4] + 1})
			stack = append(stack, classScope{name: name, indent: indent})
			continue
		}

		if m := rePyDef.FindStringSubmatchIndex(line); m != nil {
			indent := len(line[m[2]:m[3]])
			name := line[m[4]:m[5]]
			for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
				stack = stack[:len(stack)-1]
			}
			kind := "func"
			recv := ""
			if len(stack) > 0 {
				kind = "method"
				recv = stack[len(stack)-1].name
			}
			syms = append(syms, polyglotSymbol{Kind: kind, Name: name, Receiver: recv, Line: lineNo, Column: m[4] + 1})
		}
	}
	return syms
}

func extractRustSymbols(lines [][]byte) []polyglotSymbol {
	var syms []polyglotSymbol
	var currentImpl string
	implDepth := 0
	braceDepth := 0

	for i, raw := range lines {
		line := string(raw)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		lineNo := i + 1
		if m := reRsStruct.FindStringSubmatchIndex(line); m != nil {
			syms = append(syms, polyglotSymbol{Kind: "struct", Name: line[m[2]:m[3]], Line: lineNo, Column: m[2] + 1})
		} else if m := reRsEnum.FindStringSubmatchIndex(line); m != nil {
			syms = append(syms, polyglotSymbol{Kind: "enum", Name: line[m[2]:m[3]], Line: lineNo, Column: m[2] + 1})
		} else if m := reRsTrait.FindStringSubmatchIndex(line); m != nil {
			syms = append(syms, polyglotSymbol{Kind: "interface", Name: line[m[2]:m[3]], Line: lineNo, Column: m[2] + 1})
		} else if m := reRsType.FindStringSubmatchIndex(line); m != nil {
			syms = append(syms, polyglotSymbol{Kind: "type", Name: line[m[2]:m[3]], Line: lineNo, Column: m[2] + 1})
		} else if m := reRsImpl.FindStringSubmatchIndex(line); m != nil {
			currentImpl = line[m[2]:m[3]]
			implDepth = braceDepth + 1
		} else if m := reRsFn.FindStringSubmatchIndex(line); m != nil {
			name := line[m[2]:m[3]]
			kind := "func"
			recv := ""
			if currentImpl != "" && braceDepth >= implDepth {
				kind = "method"
				recv = currentImpl
			}
			syms = append(syms, polyglotSymbol{Kind: kind, Name: name, Receiver: recv, Line: lineNo, Column: m[2] + 1})
		}

		braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
		if braceDepth < 0 {
			braceDepth = 0
		}
		if currentImpl != "" && braceDepth < implDepth {
			currentImpl = ""
			implDepth = 0
		}
	}
	return syms
}

func matchASTName(query, name string, queryRegex *regexp.Regexp, caseInsensitive bool) bool {
	if query == "" {
		return true
	}
	if queryRegex != nil {
		return queryRegex.MatchString(name)
	}
	if caseInsensitive {
		return strings.Contains(strings.ToLower(name), strings.ToLower(query))
	}
	return strings.Contains(name, query)
}

var astKindAliases = map[string]map[string]bool{
	"func":      {"method": true},
	"type":      {"struct": true, "interface": true, "class": true, "enum": true, "trait": true},
	"class":     {"struct": true},
	"struct":    {"class": true},
	"trait":     {"interface": true},
	"interface": {"trait": true},
}

func matchASTKind(astKind, kind string) bool {
	if astKind == "" || strings.EqualFold(astKind, "any") {
		return true
	}
	target := strings.ToLower(strings.TrimSpace(astKind))
	kind = strings.ToLower(kind)
	if target == kind {
		return true
	}
	return astKindAliases[target][kind]
}

func matchASTReceiver(target, recv string, caseInsensitive bool) bool {
	target = strings.TrimPrefix(target, "*")
	recv = strings.TrimPrefix(recv, "*")
	if caseInsensitive {
		return strings.EqualFold(recv, target)
	}
	return recv == target
}

func collectASTContext(lines [][]byte, lineIdx, contextLines int) ([]string, []string) {
	if contextLines <= 0 {
		return nil, nil
	}
	numLines := len(lines)
	from := lineIdx - contextLines
	if from < 0 {
		from = 0
	}
	var before []string
	for i := from; i < lineIdx; i++ {
		before = append(before, string(lines[i]))
	}
	to := lineIdx + contextLines + 1
	if to > numLines {
		to = numLines
	}
	var after []string
	for i := lineIdx + 1; i < to; i++ {
		after = append(after, string(lines[i]))
	}
	return before, after
}
