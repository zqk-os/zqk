package traversal

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Standard error codes conforming to SPEC-ZPARQL-GRAPH-TRAVERSAL-GRAMMAR.
const (
	ErrCodeZPARQLSyntaxError          = "ERR_ZPARQL_SYNTAX_ERROR"
	ErrCodeZPARQLDisconnectedVariable = "ERR_ZPARQL_DISCONNECTED_VARIABLE"
	ErrCodeZPARQLInvalidEdgeType      = "ERR_ZPARQL_INVALID_EDGE_TYPE"
	ErrCodeZPARQLInvalidDepthRange    = "ERR_ZPARQL_INVALID_DEPTH_RANGE"
	ErrCodeZPARQLUndefinedProjection  = "ERR_ZPARQL_UNDEFINED_PROJECTION"
)

// TokenType represents lexical token types for ZPARQL.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdent
	TokenString
	TokenNumber
	TokenBool
	TokenNull
	TokenLParen   // (
	TokenRParen   // )
	TokenLBracket // [
	TokenRBracket // ]
	TokenLBrace   // {
	TokenRBrace   // }
	TokenColon    // :
	TokenComma    // ,
	TokenDot      // .
	TokenSemicolon // ;
	TokenMinus    // -
	TokenArrowR   // ->
	TokenArrowL   // <-
	TokenStar     // *
	TokenDotDot   // ..
	TokenEq       // = or ==
	TokenNotEq    // !=
	TokenLt       // <
	TokenLte      // <=
	TokenGt       // >
	TokenGte      // >=

	// Keywords
	TokenMatch
	TokenWhere
	TokenReturn
	TokenDistinct
	TokenOrderBy
	TokenAsc
	TokenDesc
	TokenLimit
	TokenOffset
	TokenAnd
	TokenOr
	TokenNot
	TokenContains
	TokenIn
	TokenExists
	TokenCount
	TokenCollect
	TokenMin
	TokenMax
	TokenAs
)

// Token represents a lexical token with source position.
type Token struct {
	Type   TokenType
	Value  string
	Line   int
	Column int
}

// Lexer tokenizes ZPARQL input text.
type Lexer struct {
	input []rune
	pos   int
	line  int
	col   int
}

// NewLexer creates a new ZPARQL lexer.
func NewLexer(input string) *Lexer {
	return &Lexer{
		input: []rune(input),
		pos:   0,
		line:  1,
		col:   1,
	}
}

func (l *Lexer) current() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *Lexer) peek(n int) rune {
	if l.pos+n >= len(l.input) {
		return 0
	}
	return l.input[l.pos+n]
}

func (l *Lexer) advance() rune {
	ch := l.current()
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *Lexer) skipWhitespaceAndComments() {
	for l.pos < len(l.input) {
		ch := l.current()
		if unicode.IsSpace(ch) {
			l.advance()
			continue
		}
		// Single-line comment // or --
		if ch == '/' && l.peek(1) == '/' {
			for l.pos < len(l.input) && l.current() != '\n' {
				l.advance()
			}
			continue
		}
		if ch == '-' && l.peek(1) == '-' {
			for l.pos < len(l.input) && l.current() != '\n' {
				l.advance()
			}
			continue
		}
		// Multi-line comment /* */
		if ch == '/' && l.peek(1) == '*' {
			l.advance()
			l.advance()
			for l.pos < len(l.input) {
				if l.current() == '*' && l.peek(1) == '/' {
					l.advance()
					l.advance()
					break
				}
				l.advance()
			}
			continue
		}
		break
	}
}

// NextToken scans the next token.
func (l *Lexer) NextToken() (Token, error) {
	l.skipWhitespaceAndComments()

	if l.pos >= len(l.input) {
		return Token{Type: TokenEOF, Line: l.line, Column: l.col}, nil
	}

	startLine := l.line
	startCol := l.col
	ch := l.current()

	// Handle arrows and punctuation
	if ch == '<' {
		if l.peek(1) == '-' {
			l.advance()
			l.advance()
			return Token{Type: TokenArrowL, Value: "<-", Line: startLine, Column: startCol}, nil
		}
		if l.peek(1) == '=' {
			l.advance()
			l.advance()
			return Token{Type: TokenLte, Value: "<=", Line: startLine, Column: startCol}, nil
		}
		l.advance()
		return Token{Type: TokenLt, Value: "<", Line: startLine, Column: startCol}, nil
	}

	if ch == '-' {
		if l.peek(1) == '>' {
			l.advance()
			l.advance()
			return Token{Type: TokenArrowR, Value: "->", Line: startLine, Column: startCol}, nil
		}
		// Check if it's a negative number
		if unicode.IsDigit(l.peek(1)) {
			return l.scanNumber()
		}
		l.advance()
		return Token{Type: TokenMinus, Value: "-", Line: startLine, Column: startCol}, nil
	}

	if ch == '.' {
		if l.peek(1) == '.' {
			l.advance()
			l.advance()
			return Token{Type: TokenDotDot, Value: "..", Line: startLine, Column: startCol}, nil
		}
		l.advance()
		return Token{Type: TokenDot, Value: ".", Line: startLine, Column: startCol}, nil
	}

	if ch == '!' && l.peek(1) == '=' {
		l.advance()
		l.advance()
		return Token{Type: TokenNotEq, Value: "!=", Line: startLine, Column: startCol}, nil
	}

	if ch == '=' {
		l.advance()
		if l.current() == '=' {
			l.advance()
		}
		return Token{Type: TokenEq, Value: "=", Line: startLine, Column: startCol}, nil
	}

	if ch == '>' {
		if l.peek(1) == '=' {
			l.advance()
			l.advance()
			return Token{Type: TokenGte, Value: ">=", Line: startLine, Column: startCol}, nil
		}
		l.advance()
		return Token{Type: TokenGt, Value: ">", Line: startLine, Column: startCol}, nil
	}

	// Single-character delimiters
	switch ch {
	case '(':
		l.advance()
		return Token{Type: TokenLParen, Value: "(", Line: startLine, Column: startCol}, nil
	case ')':
		l.advance()
		return Token{Type: TokenRParen, Value: ")", Line: startLine, Column: startCol}, nil
	case '[':
		l.advance()
		return Token{Type: TokenLBracket, Value: "[", Line: startLine, Column: startCol}, nil
	case ']':
		l.advance()
		return Token{Type: TokenRBracket, Value: "]", Line: startLine, Column: startCol}, nil
	case '{':
		l.advance()
		return Token{Type: TokenLBrace, Value: "{", Line: startLine, Column: startCol}, nil
	case '}':
		l.advance()
		return Token{Type: TokenRBrace, Value: "}", Line: startLine, Column: startCol}, nil
	case ':':
		l.advance()
		return Token{Type: TokenColon, Value: ":", Line: startLine, Column: startCol}, nil
	case ',':
		l.advance()
		return Token{Type: TokenComma, Value: ",", Line: startLine, Column: startCol}, nil
	case ';':
		l.advance()
		return Token{Type: TokenSemicolon, Value: ";", Line: startLine, Column: startCol}, nil
	case '*':
		l.advance()
		return Token{Type: TokenStar, Value: "*", Line: startLine, Column: startCol}, nil
	case '"', '\'':
		return l.scanString(ch)
	}

	if unicode.IsDigit(ch) {
		return l.scanNumber()
	}

	if unicode.IsLetter(ch) || ch == '_' || ch == '$' {
		return l.scanIdent()
	}

	l.advance()
	return Token{}, fmt.Errorf("%s: unexpected character %q at line %d, column %d", ErrCodeZPARQLSyntaxError, ch, startLine, startCol)
}

func (l *Lexer) scanString(quote rune) (Token, error) {
	startLine := l.line
	startCol := l.col
	l.advance() // skip open quote

	var sb strings.Builder
	for l.pos < len(l.input) {
		ch := l.current()
		if ch == quote {
			l.advance() // skip close quote
			return Token{Type: TokenString, Value: sb.String(), Line: startLine, Column: startCol}, nil
		}
		if ch == '\\' {
			l.advance()
			if l.pos >= len(l.input) {
				break
			}
			escaped := l.advance()
			switch escaped {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case 'r':
				sb.WriteRune('\r')
			case '\\':
				sb.WriteRune('\\')
			case '"':
				sb.WriteRune('"')
			case '\'':
				sb.WriteRune('\'')
			default:
				sb.WriteRune(escaped)
			}
			continue
		}
		sb.WriteRune(ch)
		l.advance()
	}

	return Token{}, fmt.Errorf("%s: unclosed string literal starting at line %d, column %d", ErrCodeZPARQLSyntaxError, startLine, startCol)
}

func (l *Lexer) scanNumber() (Token, error) {
	startLine := l.line
	startCol := l.col
	var sb strings.Builder

	if l.current() == '-' {
		sb.WriteRune(l.advance())
	}

	for l.pos < len(l.input) && (unicode.IsDigit(l.current()) || l.current() == '.') {
		if l.current() == '.' && l.peek(1) == '.' {
			// Don't consume if range quantifier ..
			break
		}
		sb.WriteRune(l.advance())
	}

	return Token{Type: TokenNumber, Value: sb.String(), Line: startLine, Column: startCol}, nil
}

func (l *Lexer) scanIdent() (Token, error) {
	startLine := l.line
	startCol := l.col
	var sb strings.Builder

	for l.pos < len(l.input) {
		ch := l.current()
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '-' || ch == '$' {
			sb.WriteRune(l.advance())
		} else {
			break
		}
	}

	val := sb.String()
	upper := strings.ToUpper(val)

	tokenType := TokenIdent
	switch upper {
	case "MATCH":
		tokenType = TokenMatch
	case "WHERE":
		tokenType = TokenWhere
	case "RETURN":
		tokenType = TokenReturn
	case "DISTINCT":
		tokenType = TokenDistinct
	case "ORDER":
		// check if next token is BY
		tokenType = TokenOrderBy
	case "ASC":
		tokenType = TokenAsc
	case "DESC":
		tokenType = TokenDesc
	case "LIMIT":
		tokenType = TokenLimit
	case "OFFSET":
		tokenType = TokenOffset
	case "AND":
		tokenType = TokenAnd
	case "OR":
		tokenType = TokenOr
	case "NOT":
		tokenType = TokenNot
	case "CONTAINS":
		tokenType = TokenContains
	case "IN":
		tokenType = TokenIn
	case "EXISTS":
		tokenType = TokenExists
	case "COUNT":
		tokenType = TokenCount
	case "COLLECT":
		tokenType = TokenCollect
	case "MIN":
		tokenType = TokenMin
	case "MAX":
		tokenType = TokenMax
	case "AS":
		tokenType = TokenAs
	case "TRUE", "FALSE":
		tokenType = TokenBool
	case "NULL":
		tokenType = TokenNull
	}

	return Token{Type: tokenType, Value: val, Line: startLine, Column: startCol}, nil
}

// Parser parses ZPARQL query string into QueryAST.
type Parser struct {
	tokens  []Token
	pos     int
	boundVars map[string]bool
}

// NewParser creates a new Parser.
func NewParser(input string) (*Parser, error) {
	lexer := NewLexer(input)
	var tokens []Token
	for {
		tok, err := lexer.NextToken()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.Type == TokenEOF {
			break
		}
	}
	return &Parser{
		tokens:    tokens,
		pos:       0,
		boundVars: make(map[string]bool),
	}, nil
}

func (p *Parser) current() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peek() Token {
	if p.pos+1 >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos+1]
}

func (p *Parser) advance() Token {
	tok := p.current()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func (p *Parser) expect(tt TokenType, expectedDesc string) (Token, error) {
	cur := p.current()
	if cur.Type != tt {
		return Token{}, fmt.Errorf("%s: line %d, column %d: expected %s, found %q",
			ErrCodeZPARQLSyntaxError, cur.Line, cur.Column, expectedDesc, cur.Value)
	}
	return p.advance(), nil
}

// Parse parses the entire ZPARQL query.
func (p *Parser) Parse() (*QueryAST, error) {
	if p.current().Type != TokenMatch {
		return nil, fmt.Errorf("%s: line %d, column %d: query must start with MATCH, found %q",
			ErrCodeZPARQLSyntaxError, p.current().Line, p.current().Column, p.current().Value)
	}
	p.advance() // consume MATCH

	ast := &QueryAST{
		Version:  "1.0.0",
		Type:     "ZPARQLQuery",
		Patterns: make([]PathPattern, 0),
		Returns:  make([]ProjectionItem, 0),
	}

	// Parse one or more path patterns separated by comma
	for {
		pattern, err := p.parsePathPattern()
		if err != nil {
			return nil, err
		}
		ast.Patterns = append(ast.Patterns, pattern)

		if p.current().Type == TokenComma {
			p.advance()
			continue
		}
		break
	}

	// Optional WHERE clause
	if p.current().Type == TokenWhere {
		p.advance()
		whereExpr, err := p.parseBooleanExpr()
		if err != nil {
			return nil, err
		}
		ast.Where = whereExpr
	}

	// Mandatory RETURN clause
	if p.current().Type != TokenReturn {
		return nil, fmt.Errorf("%s: line %d, column %d: expected RETURN clause, found %q",
			ErrCodeZPARQLSyntaxError, p.current().Line, p.current().Column, p.current().Value)
	}
	p.advance() // consume RETURN

	distinctAll := false
	if p.current().Type == TokenDistinct {
		distinctAll = true
		p.advance()
	}

	// Parse projection items
	for {
		proj, err := p.parseProjectionItem()
		if err != nil {
			return nil, err
		}
		if distinctAll {
			proj.Distinct = true
		}
		ast.Returns = append(ast.Returns, proj)

		if p.current().Type == TokenComma {
			p.advance()
			continue
		}
		break
	}

	// Optional ORDER BY clause
	if p.current().Type == TokenOrderBy {
		p.advance()
		if strings.ToUpper(p.current().Value) == "BY" {
			p.advance()
		}
		for {
			orderItem, err := p.parseOrderByItem()
			if err != nil {
				return nil, err
			}
			ast.OrderBy = append(ast.OrderBy, orderItem)
			if p.current().Type == TokenComma {
				p.advance()
				continue
			}
			break
		}
	}

	// Optional LIMIT and OFFSET
	if p.current().Type == TokenLimit {
		p.advance()
		numTok, err := p.expect(TokenNumber, "integer limit")
		if err != nil {
			return nil, err
		}
		lim, _ := strconv.Atoi(numTok.Value)
		ast.Limit = lim

		if p.current().Type == TokenOffset {
			p.advance()
			offTok, err := p.expect(TokenNumber, "integer offset")
			if err != nil {
				return nil, err
			}
			off, _ := strconv.Atoi(offTok.Value)
			ast.Offset = off
		}
	}

	// Optional trailing semicolon
	if p.current().Type == TokenSemicolon {
		p.advance()
	}

	// Validate projections are bound
	for _, ret := range ast.Returns {
		checkVar := ret.Variable
		if checkVar != "" && !p.boundVars[checkVar] {
			return nil, fmt.Errorf("%s: variable %q in RETURN was never bound in MATCH",
				ErrCodeZPARQLUndefinedProjection, checkVar)
		}
	}

	return ast, nil
}

func (p *Parser) parsePathPattern() (PathPattern, error) {
	pattern := PathPattern{
		Nodes: make([]NodePattern, 0),
		Edges: make([]EdgePattern, 0),
	}

	// Start with node
	node, err := p.parseNodePattern()
	if err != nil {
		return pattern, err
	}
	pattern.Nodes = append(pattern.Nodes, node)

	// Chain edge and node
	for {
		if p.isEdgeStart() {
			edge, err := p.parseEdgePattern()
			if err != nil {
				return pattern, err
			}
			pattern.Edges = append(pattern.Edges, edge)

			nextNode, err := p.parseNodePattern()
			if err != nil {
				return pattern, err
			}
			pattern.Nodes = append(pattern.Nodes, nextNode)
		} else {
			break
		}
	}

	return pattern, nil
}

func (p *Parser) parseNodePattern() (NodePattern, error) {
	node := NodePattern{
		Filters: make(map[string]any),
	}

	_, err := p.expect(TokenLParen, "'(' at start of node pattern")
	if err != nil {
		return node, err
	}

	// Variable name if present
	if p.current().Type == TokenIdent {
		node.Variable = p.current().Value
		p.boundVars[node.Variable] = true
		p.advance()
	}

	// Kind name if present :kind
	if p.current().Type == TokenColon {
		p.advance()
		kindTok, err := p.expect(TokenIdent, "kind name after ':'")
		if err != nil {
			return node, err
		}
		node.Kind = kindTok.Value
	}

	// Property filters { key: val, ... }
	if p.current().Type == TokenLBrace {
		p.advance()
		for {
			if p.current().Type == TokenRBrace {
				p.advance()
				break
			}
			keyTok := p.advance()
			if keyTok.Type != TokenIdent && keyTok.Type != TokenString {
				return node, fmt.Errorf("%s: line %d, column %d: expected property key identifier, got %q",
					ErrCodeZPARQLSyntaxError, keyTok.Line, keyTok.Column, keyTok.Value)
			}
			_, err := p.expect(TokenColon, "':' after property key")
			if err != nil {
				return node, err
			}
			val, err := p.parseLiteralValue()
			if err != nil {
				return node, err
			}
			node.Filters[keyTok.Value] = val

			if p.current().Type == TokenComma {
				p.advance()
				continue
			}
			if p.current().Type == TokenRBrace {
				p.advance()
				break
			}
			return node, fmt.Errorf("%s: line %d, column %d: expected ',' or '}', got %q",
				ErrCodeZPARQLSyntaxError, p.current().Line, p.current().Column, p.current().Value)
		}
	}

	_, err = p.expect(TokenRParen, "')' at end of node pattern")
	if err != nil {
		return node, err
	}

	return node, nil
}

func (p *Parser) isEdgeStart() bool {
	cur := p.current().Type
	return cur == TokenMinus || cur == TokenArrowL
}

func (p *Parser) parseEdgePattern() (EdgePattern, error) {
	edge := EdgePattern{
		MinDepth: 1,
		MaxDepth: 1,
	}

	cur := p.current()
	if cur.Type == TokenArrowL { // <-[...]
		p.advance() // consume <-
		_, err := p.expect(TokenLBracket, "'[' after '<-'")
		if err != nil {
			return edge, err
		}
		if err := p.parseEdgeSpec(&edge); err != nil {
			return edge, err
		}
		_, err = p.expect(TokenRBracket, "']' to close edge pattern")
		if err != nil {
			return edge, err
		}
		_, err = p.expect(TokenMinus, "'-' after ']' in incoming edge")
		if err != nil {
			return edge, err
		}
		edge.Direction = DirectionIncoming
		return edge, nil
	}

	if cur.Type == TokenMinus {
		p.advance() // consume -
		_, err := p.expect(TokenLBracket, "'[' after '-' in edge pattern")
		if err != nil {
			return edge, err
		}
		if err := p.parseEdgeSpec(&edge); err != nil {
			return edge, err
		}
		_, err = p.expect(TokenRBracket, "']' to close edge pattern")
		if err != nil {
			return edge, err
		}

		if p.current().Type == TokenArrowR {
			p.advance() // consume ->
			edge.Direction = DirectionOutgoing
		} else if p.current().Type == TokenMinus {
			p.advance() // consume -
			edge.Direction = DirectionUndirected
		} else {
			return edge, fmt.Errorf("%s: line %d, column %d: expected '->' or '-' after ']', got %q",
				ErrCodeZPARQLSyntaxError, p.current().Line, p.current().Column, p.current().Value)
		}
		return edge, nil
	}

	return edge, fmt.Errorf("%s: line %d, column %d: unexpected edge start token %q",
		ErrCodeZPARQLSyntaxError, cur.Line, cur.Column, cur.Value)
}

func (p *Parser) parseEdgeSpec(edge *EdgePattern) error {
	// [var:edge_type*min..max]
	if p.current().Type == TokenIdent {
		edge.Variable = p.current().Value
		p.boundVars[edge.Variable] = true
		p.advance()
	}

	if p.current().Type == TokenColon {
		p.advance()
		typeTok, err := p.expect(TokenIdent, "edge type after ':'")
		if err != nil {
			return err
		}
		edge.EdgeType = typeTok.Value
	}

	// Quantifier: * or *1..3 or *..3 or *1..
	if p.current().Type == TokenStar {
		p.advance() // consume *
		minD := 1
		maxD := 16

		if p.current().Type == TokenNumber {
			minD, _ = strconv.Atoi(p.current().Value)
			maxD = minD
			p.advance()
		}

		if p.current().Type == TokenDotDot {
			p.advance() // consume ..
			if p.current().Type == TokenNumber {
				maxD, _ = strconv.Atoi(p.current().Value)
				p.advance()
			} else {
				maxD = 16
			}
		}

		if maxD < minD {
			return fmt.Errorf("%s: max depth (%d) cannot be less than min depth (%d)",
				ErrCodeZPARQLInvalidDepthRange, maxD, minD)
		}

		edge.MinDepth = minD
		edge.MaxDepth = maxD
	}

	return nil
}

func (p *Parser) parseBooleanExpr() (Expr, error) {
	left, err := p.parsePredicate()
	if err != nil {
		return nil, err
	}

	for p.current().Type == TokenAnd || p.current().Type == TokenOr {
		op := strings.ToUpper(p.current().Value)
		p.advance()
		right, err := p.parsePredicate()
		if err != nil {
			return nil, err
		}
		left = LogicalExpr{
			Op:    op,
			Left:  left,
			Right: right,
		}
	}

	return left, nil
}

func (p *Parser) parsePredicate() (Expr, error) {
	if p.current().Type == TokenNot {
		p.advance()
		inner, err := p.parsePredicate()
		if err != nil {
			return nil, err
		}
		return NotExpr{Inner: inner}, nil
	}

	if p.current().Type == TokenLParen {
		p.advance()
		expr, err := p.parseBooleanExpr()
		if err != nil {
			return nil, err
		}
		_, err = p.expect(TokenRParen, "')' to close boolean expression")
		if err != nil {
			return nil, err
		}
		return expr, nil
	}

	if p.current().Type == TokenExists {
		p.advance()
		_, err := p.expect(TokenLParen, "'(' after EXISTS")
		if err != nil {
			return nil, err
		}
		pattern, err := p.parsePathPattern()
		if err != nil {
			return nil, err
		}
		_, err = p.expect(TokenRParen, "')' to close EXISTS")
		if err != nil {
			return nil, err
		}
		return ExistsExpr{Pattern: pattern}, nil
	}

	// Comparison expression: prop comparison_op literal
	prop, err := p.parsePropertyRef()
	if err != nil {
		return nil, err
	}

	opTok := p.current()
	var op ComparisonOp
	switch opTok.Type {
	case TokenEq:
		op = OpEq
	case TokenNotEq:
		op = OpNotEq
	case TokenLt:
		op = OpLt
	case TokenLte:
		op = OpLte
	case TokenGt:
		op = OpGt
	case TokenGte:
		op = OpGte
	case TokenContains:
		op = OpContains
	case TokenIn:
		op = OpIn
	default:
		return nil, fmt.Errorf("%s: line %d, column %d: expected comparison operator, got %q",
			ErrCodeZPARQLSyntaxError, opTok.Line, opTok.Column, opTok.Value)
	}
	p.advance()

	val, err := p.parseLiteralValue()
	if err != nil {
		return nil, err
	}

	return ComparisonExpr{
		Property: prop,
		Op:       op,
		Value:    val,
	}, nil
}

func (p *Parser) parsePropertyRef() (PropertyRef, error) {
	varTok, err := p.expect(TokenIdent, "variable name in property reference")
	if err != nil {
		return PropertyRef{}, err
	}
	_, err = p.expect(TokenDot, "'.' after variable name")
	if err != nil {
		return PropertyRef{}, err
	}
	fieldTok, err := p.expect(TokenIdent, "field name after '.'")
	if err != nil {
		return PropertyRef{}, err
	}

	return PropertyRef{
		Variable: varTok.Value,
		Field:    fieldTok.Value,
	}, nil
}

func (p *Parser) parseLiteralValue() (any, error) {
	tok := p.current()
	switch tok.Type {
	case TokenString:
		p.advance()
		return tok.Value, nil
	case TokenNumber:
		p.advance()
		if strings.Contains(tok.Value, ".") {
			return strconv.ParseFloat(tok.Value, 64)
		}
		return strconv.Atoi(tok.Value)
	case TokenBool:
		p.advance()
		return strings.EqualFold(tok.Value, "true"), nil
	case TokenNull:
		p.advance()
		return nil, nil
	case TokenLBracket: // list literal e.g. ["a", "b"]
		p.advance()
		items := make([]any, 0)
		for {
			if p.current().Type == TokenRBracket {
				p.advance()
				break
			}
			item, err := p.parseLiteralValue()
			if err != nil {
				return nil, err
			}
			items = append(items, item)
			if p.current().Type == TokenComma {
				p.advance()
				continue
			}
			if p.current().Type == TokenRBracket {
				p.advance()
				break
			}
			return nil, fmt.Errorf("%s: line %d, column %d: expected ',' or ']', got %q",
				ErrCodeZPARQLSyntaxError, p.current().Line, p.current().Column, p.current().Value)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("%s: line %d, column %d: expected literal value, got %q",
			ErrCodeZPARQLSyntaxError, tok.Line, tok.Column, tok.Value)
	}
}

func (p *Parser) parseProjectionItem() (ProjectionItem, error) {
	item := ProjectionItem{}

	// Check if aggregate function
	cur := p.current()
	if cur.Type == TokenCount || cur.Type == TokenCollect || cur.Type == TokenMin || cur.Type == TokenMax {
		item.Aggregate = strings.ToUpper(cur.Value)
		p.advance()
		_, err := p.expect(TokenLParen, "'(' after aggregate function")
		if err != nil {
			return item, err
		}

		if p.current().Type == TokenStar {
			item.Expression = "*"
			p.advance()
		} else if p.peek().Type == TokenDot {
			prop, err := p.parsePropertyRef()
			if err != nil {
				return item, err
			}
			item.Variable = prop.Variable
			item.Property = prop.Field
			item.Expression = fmt.Sprintf("%s.%s", prop.Variable, prop.Field)
		} else if p.current().Type == TokenIdent {
			item.Variable = p.current().Value
			item.Expression = item.Variable
			p.advance()
		}

		_, err = p.expect(TokenRParen, "')' to close aggregate function")
		if err != nil {
			return item, err
		}
	} else if p.peek().Type == TokenDot {
		// Property reference
		prop, err := p.parsePropertyRef()
		if err != nil {
			return item, err
		}
		item.Variable = prop.Variable
		item.Property = prop.Field
		item.Expression = fmt.Sprintf("%s.%s", prop.Variable, prop.Field)
	} else if cur.Type == TokenIdent {
		item.Variable = cur.Value
		item.Expression = cur.Value
		p.advance()
	} else {
		return item, fmt.Errorf("%s: line %d, column %d: invalid projection expression %q",
			ErrCodeZPARQLSyntaxError, cur.Line, cur.Column, cur.Value)
	}

	// Optional AS alias
	if p.current().Type == TokenAs {
		p.advance()
		aliasTok, err := p.expect(TokenIdent, "alias name after AS")
		if err != nil {
			return item, err
		}
		item.Alias = aliasTok.Value
	}

	return item, nil
}

func (p *Parser) parseOrderByItem() (OrderByItem, error) {
	item := OrderByItem{Direction: "ASC"}

	if p.peek().Type == TokenDot {
		prop, err := p.parsePropertyRef()
		if err != nil {
			return item, err
		}
		item.Property = fmt.Sprintf("%s.%s", prop.Variable, prop.Field)
	} else if p.current().Type == TokenIdent {
		item.Property = p.current().Value
		p.advance()
	}

	if p.current().Type == TokenAsc {
		item.Direction = "ASC"
		p.advance()
	} else if p.current().Type == TokenDesc {
		item.Direction = "DESC"
		p.advance()
	}

	return item, nil
}

// ParseZPARQL parses a raw ZPARQL query string into QueryAST.
func ParseZPARQL(query string) (*QueryAST, error) {
	p, err := NewParser(query)
	if err != nil {
		return nil, err
	}
	return p.Parse()
}
