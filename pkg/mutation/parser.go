package mutation

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Standard error codes conforming to SPEC-ZQL-DECLARATIVE-MUTATION-GRAMMAR.
const (
	ErrCodeZQLSyntaxError        = "ERR_ZQL_SYNTAX_ERROR"
	ErrCodeZQLUnboundVariable    = "ERR_ZQL_UNBOUND_VARIABLE"
	ErrCodeZQLCircularDependency = "ERR_ZQL_CIRCULAR_DEPENDENCY"
	ErrCodeZQLInvalidKind        = "ERR_ZQL_INVALID_KIND"
	ErrCodeZQLUnresolvedPath     = "ERR_ZQL_UNRESOLVED_PATH"
)

// ZQLTokenType represents lexical token categories.
type ZQLTokenType int

const (
	ZQLTokenEOF ZQLTokenType = iota
	ZQLTokenIdent
	ZQLTokenVariable // $ident
	ZQLTokenString
	ZQLTokenNumber
	ZQLTokenBool
	ZQLTokenNull
	ZQLTokenEqual     // =
	ZQLTokenColon     // :
	ZQLTokenComma     // ,
	ZQLTokenDot       // .
	ZQLTokenSemicolon // ;
	ZQLTokenLBrace    // {
	ZQLTokenRBrace    // }
	ZQLTokenLBracket  // [
	ZQLTokenRBracket  // ]
	ZQLTokenStar      // *

	// Keywords
	ZQLTokenBegin
	ZQLTokenTransaction
	ZQLTokenIsolation
	ZQLTokenLevel
	ZQLTokenCommit
	ZQLTokenRollback
	ZQLTokenLet
	ZQLTokenUpsert
	ZQLTokenID
	ZQLTokenWith
	ZQLTokenReturning
	ZQLTokenDelete
	ZQLTokenCascade
	ZQLTokenRestrict
	ZQLTokenAs
)

// ZQLToken is a lexical token with source position.
type ZQLToken struct {
	Type   ZQLTokenType
	Value  string
	Line   int
	Column int
}

// ZQLLexer scans ZQL source text into tokens.
type ZQLLexer struct {
	input []rune
	pos   int
	line  int
	col   int
}

// NewZQLLexer creates a new lexer.
func NewZQLLexer(input string) *ZQLLexer {
	return &ZQLLexer{
		input: []rune(input),
		pos:   0,
		line:  1,
		col:   1,
	}
}

func (l *ZQLLexer) current() rune {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *ZQLLexer) peek(n int) rune {
	if l.pos+n >= len(l.input) {
		return 0
	}
	return l.input[l.pos+n]
}

func (l *ZQLLexer) advance() rune {
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

func (l *ZQLLexer) skipWhitespaceAndComments() {
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
		// Multi-line comment /* ... */
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

// NextToken scans the next ZQL token.
func (l *ZQLLexer) NextToken() (ZQLToken, error) {
	l.skipWhitespaceAndComments()

	if l.pos >= len(l.input) {
		return ZQLToken{Type: ZQLTokenEOF, Line: l.line, Column: l.col}, nil
	}

	startLine := l.line
	startCol := l.col
	ch := l.current()

	// Variable identifier $var
	if ch == '$' {
		l.advance() // consume $
		var sb strings.Builder
		for l.pos < len(l.input) {
			c := l.current()
			if unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_' || c == '-' {
				sb.WriteRune(l.advance())
			} else {
				break
			}
		}
		if sb.Len() == 0 {
			return ZQLToken{}, fmt.Errorf("%s: line %d, column %d: expected variable identifier after '$'",
				ErrCodeZQLSyntaxError, startLine, startCol)
		}
		return ZQLToken{Type: ZQLTokenVariable, Value: sb.String(), Line: startLine, Column: startCol}, nil
	}

	// Punctuation
	switch ch {
	case '=':
		l.advance()
		return ZQLToken{Type: ZQLTokenEqual, Value: "=", Line: startLine, Column: startCol}, nil
	case ':':
		l.advance()
		return ZQLToken{Type: ZQLTokenColon, Value: ":", Line: startLine, Column: startCol}, nil
	case ',':
		l.advance()
		return ZQLToken{Type: ZQLTokenComma, Value: ",", Line: startLine, Column: startCol}, nil
	case '.':
		l.advance()
		return ZQLToken{Type: ZQLTokenDot, Value: ".", Line: startLine, Column: startCol}, nil
	case ';':
		l.advance()
		return ZQLToken{Type: ZQLTokenSemicolon, Value: ";", Line: startLine, Column: startCol}, nil
	case '{':
		l.advance()
		return ZQLToken{Type: ZQLTokenLBrace, Value: "{", Line: startLine, Column: startCol}, nil
	case '}':
		l.advance()
		return ZQLToken{Type: ZQLTokenRBrace, Value: "}", Line: startLine, Column: startCol}, nil
	case '[':
		l.advance()
		return ZQLToken{Type: ZQLTokenLBracket, Value: "[", Line: startLine, Column: startCol}, nil
	case ']':
		l.advance()
		return ZQLToken{Type: ZQLTokenRBracket, Value: "]", Line: startLine, Column: startCol}, nil
	case '*':
		l.advance()
		return ZQLToken{Type: ZQLTokenStar, Value: "*", Line: startLine, Column: startCol}, nil
	case '"', '\'':
		return l.scanString(ch)
	}

	// Numbers
	if unicode.IsDigit(ch) || (ch == '-' && unicode.IsDigit(l.peek(1))) {
		return l.scanNumber()
	}

	// Identifiers and Keywords
	if unicode.IsLetter(ch) || ch == '_' {
		return l.scanIdent()
	}

	l.advance()
	return ZQLToken{}, fmt.Errorf("%s: line %d, column %d: unexpected character %q",
		ErrCodeZQLSyntaxError, startLine, startCol, ch)
}

func (l *ZQLLexer) scanString(quote rune) (ZQLToken, error) {
	startLine := l.line
	startCol := l.col
	l.advance() // skip open quote

	var sb strings.Builder
	for l.pos < len(l.input) {
		ch := l.current()
		if ch == quote {
			l.advance()
			return ZQLToken{Type: ZQLTokenString, Value: sb.String(), Line: startLine, Column: startCol}, nil
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

	return ZQLToken{}, fmt.Errorf("%s: unclosed string literal starting at line %d, column %d",
		ErrCodeZQLSyntaxError, startLine, startCol)
}

func (l *ZQLLexer) scanNumber() (ZQLToken, error) {
	startLine := l.line
	startCol := l.col
	var sb strings.Builder

	if l.current() == '-' {
		sb.WriteRune(l.advance())
	}

	for l.pos < len(l.input) && (unicode.IsDigit(l.current()) || l.current() == '.') {
		sb.WriteRune(l.advance())
	}

	return ZQLToken{Type: ZQLTokenNumber, Value: sb.String(), Line: startLine, Column: startCol}, nil
}

func (l *ZQLLexer) scanIdent() (ZQLToken, error) {
	startLine := l.line
	startCol := l.col
	var sb strings.Builder

	for l.pos < len(l.input) {
		ch := l.current()
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '-' {
			sb.WriteRune(l.advance())
		} else {
			break
		}
	}

	val := sb.String()
	upper := strings.ToUpper(val)

	tokenType := ZQLTokenIdent
	switch upper {
	case "BEGIN":
		tokenType = ZQLTokenBegin
	case "TRANSACTION":
		tokenType = ZQLTokenTransaction
	case "ISOLATION":
		tokenType = ZQLTokenIsolation
	case "LEVEL":
		tokenType = ZQLTokenLevel
	case "COMMIT":
		tokenType = ZQLTokenCommit
	case "ROLLBACK":
		tokenType = ZQLTokenRollback
	case "LET":
		tokenType = ZQLTokenLet
	case "UPSERT":
		tokenType = ZQLTokenUpsert
	case "ID":
		tokenType = ZQLTokenID
	case "WITH":
		tokenType = ZQLTokenWith
	case "RETURNING":
		tokenType = ZQLTokenReturning
	case "DELETE":
		tokenType = ZQLTokenDelete
	case "CASCADE":
		tokenType = ZQLTokenCascade
	case "RESTRICT":
		tokenType = ZQLTokenRestrict
	case "AS":
		tokenType = ZQLTokenAs
	case "TRUE", "FALSE":
		tokenType = ZQLTokenBool
	case "NULL":
		tokenType = ZQLTokenNull
	}

	return ZQLToken{Type: tokenType, Value: val, Line: startLine, Column: startCol}, nil
}

// ZQLParser parses a sequence of ZQL tokens into a ZQLProgram AST.
type ZQLParser struct {
	tokens []ZQLToken
	pos    int
}

// NewZQLParser initializes a ZQL parser.
func NewZQLParser(input string) (*ZQLParser, error) {
	lexer := NewZQLLexer(input)
	var tokens []ZQLToken
	for {
		tok, err := lexer.NextToken()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, tok)
		if tok.Type == ZQLTokenEOF {
			break
		}
	}
	return &ZQLParser{
		tokens: tokens,
		pos:    0,
	}, nil
}

func (p *ZQLParser) current() ZQLToken {
	if p.pos >= len(p.tokens) {
		return ZQLToken{Type: ZQLTokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *ZQLParser) advance() ZQLToken {
	tok := p.current()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func (p *ZQLParser) expect(tt ZQLTokenType, expectedDesc string) (ZQLToken, error) {
	cur := p.current()
	if cur.Type != tt {
		return ZQLToken{}, fmt.Errorf("%s: line %d, column %d: expected %s, found %q",
			ErrCodeZQLSyntaxError, cur.Line, cur.Column, expectedDesc, cur.Value)
	}
	return p.advance(), nil
}

// Parse processes the entire ZQL program and resolves variable dependencies.
func (p *ZQLParser) Parse() (*ZQLProgram, error) {
	program := &ZQLProgram{
		Version:    "1.0.0",
		Type:       "Program",
		Statements: make([]Statement, 0),
	}

	for p.current().Type != ZQLTokenEOF {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		program.Statements = append(program.Statements, stmt)
	}

	// Topological sort and variable resolution via Kahn's algorithm
	resolvedStmts, err := ResolveVariableDependencies(program.Statements)
	if err != nil {
		return nil, err
	}
	program.Statements = resolvedStmts

	return program, nil
}

func (p *ZQLParser) parseStatement() (Statement, error) {
	cur := p.current()
	switch cur.Type {
	case ZQLTokenBegin:
		return p.parseBeginTransaction()
	case ZQLTokenCommit:
		return p.parseCommitTransaction()
	case ZQLTokenRollback:
		return p.parseRollbackTransaction()
	case ZQLTokenLet:
		return p.parseLetStatement()
	case ZQLTokenUpsert:
		return p.parseUpsertStatement("")
	case ZQLTokenDelete:
		return p.parseDeleteStatement()
	default:
		return Statement{}, fmt.Errorf("%s: line %d, column %d: unexpected statement token %q",
			ErrCodeZQLSyntaxError, cur.Line, cur.Column, cur.Value)
	}
}

func (p *ZQLParser) parseBeginTransaction() (Statement, error) {
	tok := p.advance() // consume BEGIN
	stmt := Statement{
		NodeType:       StmtBeginTransaction,
		Line:           tok.Line,
		Column:         tok.Column,
		IsolationLevel: IsolationStagedSnapshot, // default
	}

	if p.current().Type == ZQLTokenTransaction {
		p.advance()
	}

	if p.current().Type == ZQLTokenIsolation {
		p.advance() // consume ISOLATION
		_, err := p.expect(ZQLTokenLevel, "LEVEL after ISOLATION")
		if err != nil {
			return stmt, err
		}
		levelTok := p.advance()
		stmt.IsolationLevel = IsolationMode(levelTok.Value)
	}

	_, err := p.expect(ZQLTokenSemicolon, "';' at end of BEGIN statement")
	if err != nil {
		return stmt, err
	}

	return stmt, nil
}

func (p *ZQLParser) parseCommitTransaction() (Statement, error) {
	tok := p.advance() // consume COMMIT
	stmt := Statement{
		NodeType: StmtCommitTransaction,
		Line:     tok.Line,
		Column:   tok.Column,
	}

	if p.current().Type == ZQLTokenTransaction {
		p.advance()
	}

	_, err := p.expect(ZQLTokenSemicolon, "';' at end of COMMIT statement")
	if err != nil {
		return stmt, err
	}
	return stmt, nil
}

func (p *ZQLParser) parseRollbackTransaction() (Statement, error) {
	tok := p.advance() // consume ROLLBACK
	stmt := Statement{
		NodeType: StmtRollbackTransaction,
		Line:     tok.Line,
		Column:   tok.Column,
	}

	if p.current().Type == ZQLTokenTransaction {
		p.advance()
	}

	_, err := p.expect(ZQLTokenSemicolon, "';' at end of ROLLBACK statement")
	if err != nil {
		return stmt, err
	}
	return stmt, nil
}

func (p *ZQLParser) parseLetStatement() (Statement, error) {
	letTok := p.advance() // consume LET
	varTok, err := p.expect(ZQLTokenVariable, "variable identifier ($var)")
	if err != nil {
		return Statement{}, err
	}

	_, err = p.expect(ZQLTokenEqual, "'=' after variable identifier")
	if err != nil {
		return Statement{}, err
	}

	var expr ZQLExpression
	var upsertStmt *UpsertExpr

	if p.current().Type == ZQLTokenUpsert {
		upsert, err := p.parseUpsertExpression(varTok.Value)
		if err != nil {
			return Statement{}, err
		}
		upsertStmt = &upsert
		expr = upsert
	} else {
		valExpr, err := p.parseExpression()
		if err != nil {
			return Statement{}, err
		}
		expr = valExpr
	}

	_, err = p.expect(ZQLTokenSemicolon, "';' at end of LET statement")
	if err != nil {
		return Statement{}, err
	}

	return Statement{
		NodeType:     StmtLet,
		Line:         letTok.Line,
		Column:       letTok.Column,
		VariableName: varTok.Value,
		Expression:   expr,
		Upsert:       upsertStmt,
	}, nil
}

func (p *ZQLParser) parseUpsertStatement(bindAs string) (Statement, error) {
	tok := p.current()
	upsert, err := p.parseUpsertExpression(bindAs)
	if err != nil {
		return Statement{}, err
	}

	_, err = p.expect(ZQLTokenSemicolon, "';' at end of UPSERT statement")
	if err != nil {
		return Statement{}, err
	}

	return Statement{
		NodeType: StmtUpsert,
		Line:     tok.Line,
		Column:   tok.Column,
		Upsert:   &upsert,
	}, nil
}

func (p *ZQLParser) parseUpsertExpression(bindAs string) (UpsertExpr, error) {
	p.advance() // consume UPSERT
	upsert := UpsertExpr{
		BindAs: bindAs,
	}

	// Kind identifier (e.g. priority_plan, backlog_item)
	if p.current().Type == ZQLTokenIdent {
		upsert.Kind = p.advance().Value
	}

	// Optional ID clause: ID "ID-123" or ID $plan.id
	if p.current().Type == ZQLTokenID {
		p.advance() // consume ID
		idExpr, err := p.parseExpression()
		if err != nil {
			return upsert, err
		}
		upsert.ID = idExpr
	}

	// Optional WITH keyword
	if p.current().Type == ZQLTokenWith {
		p.advance()
	}

	// Object literal payload
	payload, err := p.parseObjectLiteral()
	if err != nil {
		return upsert, err
	}
	upsert.Payload = payload

	// Optional RETURNING clause
	if p.current().Type == ZQLTokenReturning {
		p.advance() // consume RETURNING
		if p.current().Type == ZQLTokenStar {
			p.advance()
			upsert.Returning = []string{"*"}
		} else {
			for {
				if p.current().Type != ZQLTokenIdent && p.current().Type != ZQLTokenID {
					return upsert, fmt.Errorf("%s: line %d, column %d: expected field name in RETURNING clause, found %q",
						ErrCodeZQLSyntaxError, p.current().Line, p.current().Column, p.current().Value)
				}
				identTok := p.advance()
				upsert.Returning = append(upsert.Returning, identTok.Value)
				if p.current().Type == ZQLTokenComma {
					p.advance()
					continue
				}
				break
			}
		}
	}

	return upsert, nil
}

func (p *ZQLParser) parseDeleteStatement() (Statement, error) {
	delTok := p.advance() // consume DELETE
	stmt := Statement{
		NodeType:    StmtDelete,
		Line:        delTok.Line,
		Column:      delTok.Column,
		CascadeMode: "RESTRICT",
	}

	if p.current().Type == ZQLTokenIdent {
		stmt.DeleteKind = p.advance().Value
	}

	idExpr, err := p.parseExpression()
	if err != nil {
		return stmt, err
	}
	stmt.DeleteID = idExpr

	if p.current().Type == ZQLTokenCascade {
		stmt.CascadeMode = "CASCADE"
		p.advance()
	} else if p.current().Type == ZQLTokenRestrict {
		stmt.CascadeMode = "RESTRICT"
		p.advance()
	}

	_, err = p.expect(ZQLTokenSemicolon, "';' at end of DELETE statement")
	if err != nil {
		return stmt, err
	}

	return stmt, nil
}

func (p *ZQLParser) parseExpression() (ZQLExpression, error) {
	cur := p.current()
	switch cur.Type {
	case ZQLTokenVariable:
		return p.parseVariableRef()
	case ZQLTokenLBrace:
		return p.parseObjectLiteral()
	case ZQLTokenLBracket:
		return p.parseListLiteral()
	case ZQLTokenString:
		p.advance()
		return LiteralExpr{Value: cur.Value}, nil
	case ZQLTokenNumber:
		p.advance()
		if strings.Contains(cur.Value, ".") {
			f, _ := strconv.ParseFloat(cur.Value, 64)
			return LiteralExpr{Value: f}, nil
		}
		n, _ := strconv.Atoi(cur.Value)
		return LiteralExpr{Value: n}, nil
	case ZQLTokenBool:
		p.advance()
		return LiteralExpr{Value: strings.EqualFold(cur.Value, "true")}, nil
	case ZQLTokenNull:
		p.advance()
		return LiteralExpr{Value: nil}, nil
	default:
		return nil, fmt.Errorf("%s: line %d, column %d: unexpected expression token %q",
			ErrCodeZQLSyntaxError, cur.Line, cur.Column, cur.Value)
	}
}

func (p *ZQLParser) parseVariableRef() (VariableRefExpr, error) {
	varTok := p.advance() // consume $var
	ref := VariableRefExpr{
		VariableName: varTok.Value,
		Path:         make([]string, 0),
	}

	for p.current().Type == ZQLTokenDot {
		p.advance() // consume .
		if p.current().Type != ZQLTokenIdent && p.current().Type != ZQLTokenID {
			return ref, fmt.Errorf("%s: line %d, column %d: expected path segment after '.', found %q",
				ErrCodeZQLSyntaxError, p.current().Line, p.current().Column, p.current().Value)
		}
		segTok := p.advance()
		ref.Path = append(ref.Path, segTok.Value)
	}

	return ref, nil
}

func (p *ZQLParser) parseObjectLiteral() (ObjectLiteralExpr, error) {
	obj := ObjectLiteralExpr{
		Fields: make(map[string]ZQLExpression),
	}

	_, err := p.expect(ZQLTokenLBrace, "'{' at start of object literal")
	if err != nil {
		return obj, err
	}

	for {
		if p.current().Type == ZQLTokenRBrace {
			p.advance()
			break
		}

		keyTok := p.advance()
		if keyTok.Type != ZQLTokenIdent && keyTok.Type != ZQLTokenID && keyTok.Type != ZQLTokenString {
			return obj, fmt.Errorf("%s: line %d, column %d: expected object key, found %q",
				ErrCodeZQLSyntaxError, keyTok.Line, keyTok.Column, keyTok.Value)
		}

		_, err := p.expect(ZQLTokenColon, "':' after object field key")
		if err != nil {
			return obj, err
		}

		valExpr, err := p.parseExpression()
		if err != nil {
			return obj, err
		}
		obj.Fields[keyTok.Value] = valExpr

		if p.current().Type == ZQLTokenComma {
			p.advance()
			continue
		}
		if p.current().Type == ZQLTokenRBrace {
			p.advance()
			break
		}
		return obj, fmt.Errorf("%s: line %d, column %d: expected ',' or '}' in object literal, found %q",
			ErrCodeZQLSyntaxError, p.current().Line, p.current().Column, p.current().Value)
	}

	return obj, nil
}

func (p *ZQLParser) parseListLiteral() (ListLiteralExpr, error) {
	list := ListLiteralExpr{
		Items: make([]ZQLExpression, 0),
	}

	_, err := p.expect(ZQLTokenLBracket, "'[' at start of list literal")
	if err != nil {
		return list, err
	}

	for {
		if p.current().Type == ZQLTokenRBracket {
			p.advance()
			break
		}

		item, err := p.parseExpression()
		if err != nil {
			return list, err
		}
		list.Items = append(list.Items, item)

		if p.current().Type == ZQLTokenComma {
			p.advance()
			continue
		}
		if p.current().Type == ZQLTokenRBracket {
			p.advance()
			break
		}
		return list, fmt.Errorf("%s: line %d, column %d: expected ',' or ']' in list literal, found %q",
			ErrCodeZQLSyntaxError, p.current().Line, p.current().Column, p.current().Value)
	}

	return list, nil
}

// ResolveVariableDependencies builds statement dependency DAG and sorts via Kahn's algorithm.
func ResolveVariableDependencies(stmts []Statement) ([]Statement, error) {
	var begins []Statement
	var endings []Statement
	var body []Statement

	for _, s := range stmts {
		switch s.NodeType {
		case StmtBeginTransaction:
			begins = append(begins, s)
		case StmtCommitTransaction, StmtRollbackTransaction:
			endings = append(endings, s)
		default:
			body = append(body, s)
		}
	}

	if len(body) == 0 {
		var res []Statement
		res = append(res, begins...)
		res = append(res, endings...)
		return res, nil
	}

	// 1. Collect Defined variables per statement in body
	defMap := make(map[string]int) // variable_name -> statement index in body
	for i, s := range body {
		if s.NodeType == StmtLet && s.VariableName != "" {
			defMap[s.VariableName] = i
		}
	}

	// 2. Build dependency graph: stmt i depends on stmt j
	inDegrees := make([]int, len(body))
	adjList := make([][]int, len(body))

	for i, s := range body {
		refs := extractReferencedVariables(s)
		for _, ref := range refs {
			defIdx, exists := defMap[ref]
			if !exists {
				return nil, fmt.Errorf("%s: variable $%s referenced at line %d, column %d was never defined",
					ErrCodeZQLUnboundVariable, ref, s.Line, s.Column)
			}
			if defIdx == i {
				// Self-referencing cycle
				return nil, fmt.Errorf("%s: circular reference on variable $%s",
					ErrCodeZQLCircularDependency, ref)
			}
			adjList[defIdx] = append(adjList[defIdx], i)
			inDegrees[i]++
		}
	}

	// 3. Kahn's Algorithm
	var queue []int
	for i := range body {
		if inDegrees[i] == 0 {
			queue = append(queue, i)
		}
	}

	var ordered []Statement
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		ordered = append(ordered, body[u])

		for _, v := range adjList[u] {
			inDegrees[v]--
			if inDegrees[v] == 0 {
				queue = append(queue, v)
			}
		}
	}

	if len(ordered) < len(body) {
		return nil, fmt.Errorf("%s: variable dependency cycle detected among statements",
			ErrCodeZQLCircularDependency)
	}

	res := make([]Statement, 0, len(begins)+len(ordered)+len(endings))
	res = append(res, begins...)
	res = append(res, ordered...)
	res = append(res, endings...)
	return res, nil
}

func extractReferencedVariables(s Statement) []string {
	var refs []string
	seen := make(map[string]bool)

	var walkExpr func(e ZQLExpression)
	walkExpr = func(e ZQLExpression) {
		if e == nil {
			return
		}
		switch x := e.(type) {
		case VariableRefExpr:
			if !seen[x.VariableName] {
				seen[x.VariableName] = true
				refs = append(refs, x.VariableName)
			}
		case ObjectLiteralExpr:
			for _, val := range x.Fields {
				walkExpr(val)
			}
		case ListLiteralExpr:
			for _, item := range x.Items {
				walkExpr(item)
			}
		case UpsertExpr:
			walkExpr(x.ID)
			walkExpr(x.Payload)
		}
	}

	walkExpr(s.Expression)
	if s.Upsert != nil {
		walkExpr(s.Upsert.ID)
		walkExpr(s.Upsert.Payload)
	}
	walkExpr(s.DeleteID)

	return refs
}

// ParseZQL parses a ZQL mutation script into a validated, topologically resolved AST.
func ParseZQL(script string) (*ZQLProgram, error) {
	p, err := NewZQLParser(script)
	if err != nil {
		return nil, err
	}
	return p.Parse()
}
