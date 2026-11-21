package community

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/zqk-os/zqk/pkg/strutil"
)

var (
	// ErrMalformedIntentSyntax indicates invalid lexical or grammatical intent syntax.
	ErrMalformedIntentSyntax = errors.New("malformed intent syntax")

	// ErrIntentCycleDetected indicates a self-dependency or cycle in the intent graph.
	ErrIntentCycleDetected = errors.New("intent dependency cycle detected")

	// ErrUnknownDependency indicates a reference to an undeclared step dependency.
	ErrUnknownDependency = errors.New("unknown intent dependency target")

	// ErrDuplicateIntentNode indicates a duplicate step or stage identifier.
	ErrDuplicateIntentNode = errors.New("duplicate intent node identifier")
)

// TokenKind identifies the lexical category of an intent token.
type TokenKind string

const (
	TokenGoal      TokenKind = "GOAL"
	TokenStage     TokenKind = "STAGE"
	TokenStep      TokenKind = "STEP"
	TokenParallel  TokenKind = "PARALLEL"
	TokenThen      TokenKind = "THEN"
	TokenDependsOn TokenKind = "DEPENDS_ON"
	TokenIdent     TokenKind = "IDENT"
	TokenString    TokenKind = "STRING"
	TokenArrow     TokenKind = "ARROW"
	TokenSemicolon TokenKind = "SEMICOLON"
	TokenComma     TokenKind = "COMMA"
	TokenLBrace    TokenKind = "LBRACE"
	TokenRBrace    TokenKind = "RBRACE"
	TokenLParen    TokenKind = "LPAREN"
	TokenRParen    TokenKind = "RPAREN"
	TokenEOF       TokenKind = "EOF"
)

var intentKeywords = map[string]TokenKind{
	"GOAL":       TokenGoal,
	"STAGE":      TokenStage,
	"STEP":       TokenStep,
	"PARALLEL":   TokenParallel,
	"THEN":       TokenThen,
	"DEPENDS_ON": TokenDependsOn,
}

var singleCharTokens = map[byte]TokenKind{
	';': TokenSemicolon,
	',': TokenComma,
	'{': TokenLBrace,
	'}': TokenRBrace,
	'(': TokenLParen,
	')': TokenRParen,
}

// IntentToken represents a single lexical unit scanned from an intent expression.
type IntentToken struct {
	Kind     TokenKind
	Literal  string
	Position int
	Line     int
	Column   int
}

// IntentNodeKind identifies the structural role of an AST node.
type IntentNodeKind string

const (
	NodeKindRoot     IntentNodeKind = "ROOT"
	NodeKindGoal     IntentNodeKind = "GOAL"
	NodeKindStage    IntentNodeKind = "STAGE"
	NodeKindStep     IntentNodeKind = "STEP"
	NodeKindParallel IntentNodeKind = "PARALLEL"
)

// IntentASTNode represents a parsed node in the declarative intent AST.
type IntentASTNode struct {
	ID        string
	Kind      IntentNodeKind
	Label     string
	DependsOn []string
	Children  []*IntentASTNode
}

// IntentStage represents a topologically ordered parallel execution wave.
type IntentStage struct {
	Index   int
	StepIDs []string
}

// IntentPlan represents a compiled, topologically ordered execution plan with a canonical digest.
type IntentPlan struct {
	Goal           string
	Root           *IntentASTNode
	Steps          map[string]*IntentASTNode
	OrderedStepIDs []string
	Stages         []IntentStage
	Digest         string
}

// IntentCompiler tokenizes, parses, validates, and synthesizes declarative intent expressions.
type IntentCompiler struct {
	mu           sync.RWMutex
	compileCount uint64
}

// NewIntentCompiler creates a thread-safe IntentCompiler instance.
func NewIntentCompiler() *IntentCompiler {
	return &IntentCompiler{}
}

// CompileCount returns the number of successful compilations performed by this compiler.
func (c *IntentCompiler) CompileCount() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.compileCount
}

// Compile tokenizes, parses, and synthesizes an intent expression into a deterministic IntentPlan.
func (c *IntentCompiler) Compile(input string) (*IntentPlan, error) {
	tokens, err := c.Tokenize(input)
	if err != nil {
		return nil, err
	}
	root, err := c.Parse(tokens)
	if err != nil {
		return nil, err
	}
	plan, err := c.Synthesize(root)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.compileCount++
	c.mu.Unlock()

	return plan, nil
}

// Tokenize converts a declarative intent expression into a typed IntentToken stream.
func (c *IntentCompiler) Tokenize(input string) ([]IntentToken, error) {
	if err := strutil.ErrorOnBlank(input, "intent expression cannot be blank"); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedIntentSyntax, err)
	}

	var tokens []IntentToken
	line, col := 1, 1
	i := 0
	for i < len(input) {
		ch := input[i]
		if ch == '\n' {
			line++
			col = 1
			i++
			continue
		}
		if unicode.IsSpace(rune(ch)) {
			col++
			i++
			continue
		}
		if ch == '-' && i+1 < len(input) && input[i+1] == '>' {
			tokens = append(tokens, IntentToken{Kind: TokenArrow, Literal: "->", Position: i, Line: line, Column: col})
			i += 2
			col += 2
			continue
		}
		if kind, ok := singleCharTokens[ch]; ok {
			tokens = append(tokens, IntentToken{Kind: kind, Literal: string(ch), Position: i, Line: line, Column: col})
			i++
			col++
			continue
		}
		if ch == '"' {
			tok, nextIdx, nextCol, err := scanQuotedString(input, i, line, col)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = nextIdx
			col = nextCol
			continue
		}
		if isIdentStart(ch) {
			tok, nextIdx, nextCol := scanIdentOrKeyword(input, i, line, col)
			tokens = append(tokens, tok)
			i = nextIdx
			col = nextCol
			continue
		}
		return nil, fmt.Errorf("%w: unexpected character %q at line %d column %d", ErrMalformedIntentSyntax, ch, line, col)
	}

	tokens = append(tokens, IntentToken{Kind: TokenEOF, Position: len(input), Line: line, Column: col})
	return tokens, nil
}

func scanQuotedString(input string, start, line, col int) (IntentToken, int, int, error) {
	var b strings.Builder
	i := start + 1
	curCol := col + 1
	for i < len(input) {
		ch := input[i]
		if ch == '\n' {
			return IntentToken{}, 0, 0, fmt.Errorf("%w: unterminated string literal at line %d column %d", ErrMalformedIntentSyntax, line, col)
		}
		if ch == '\\' && i+1 < len(input) {
			i++
			curCol++
			b.WriteByte(input[i])
			i++
			curCol++
			continue
		}
		if ch == '"' {
			lit := b.String()
			if err := strutil.ErrorOnBlank(lit, "quoted literal cannot be blank"); err != nil {
				return IntentToken{}, 0, 0, fmt.Errorf("%w: %v at line %d column %d", ErrMalformedIntentSyntax, err, line, col)
			}
			return IntentToken{Kind: TokenString, Literal: lit, Position: start, Line: line, Column: col}, i + 1, curCol + 1, nil
		}
		b.WriteByte(ch)
		i++
		curCol++
	}
	return IntentToken{}, 0, 0, fmt.Errorf("%w: unterminated string literal at line %d column %d", ErrMalformedIntentSyntax, line, col)
}

func scanIdentOrKeyword(input string, start, line, col int) (IntentToken, int, int) {
	i := start
	for i < len(input) && isIdentPart(input[i]) {
		i++
	}
	lit := input[start:i]
	upper := strings.ToUpper(lit)
	kind := TokenIdent
	if kw, ok := intentKeywords[upper]; ok {
		kind = kw
		lit = upper
	}
	return IntentToken{
		Kind:     kind,
		Literal:  lit,
		Position: start,
		Line:     line,
		Column:   col,
	}, i, col + (i - start)
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9') || ch == '-' || ch == '.'
}

// Parse builds a structured IntentASTNode tree from a validated token stream.
func (c *IntentCompiler) Parse(tokens []IntentToken) (*IntentASTNode, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("%w: empty token stream", ErrMalformedIntentSyntax)
	}
	p := &intentParser{
		tokens: tokens,
		steps:  make(map[string]*IntentASTNode),
		stages: make(map[string]struct{}),
	}
	return p.parseRoot()
}

type intentParser struct {
	tokens []IntentToken
	pos    int
	steps  map[string]*IntentASTNode
	stages map[string]struct{}
}

func (p *intentParser) peek() IntentToken {
	if p.pos >= len(p.tokens) {
		return IntentToken{Kind: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *intentParser) advance() IntentToken {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

func (p *intentParser) match(kind TokenKind) bool {
	if p.peek().Kind == kind {
		p.advance()
		return true
	}
	return false
}

func (p *intentParser) parseRoot() (*IntentASTNode, error) {
	root := &IntentASTNode{
		ID:   "root",
		Kind: NodeKindRoot,
	}

	for p.peek().Kind != TokenEOF {
		if p.match(TokenSemicolon) {
			continue
		}
		node, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if node != nil {
			root.Children = append(root.Children, node)
		}
		if p.peek().Kind != TokenEOF && p.peek().Kind != TokenSemicolon {
			tok := p.peek()
			return nil, fmt.Errorf("%w: unexpected token %q (%s) at line %d column %d", ErrMalformedIntentSyntax, tok.Literal, tok.Kind, tok.Line, tok.Column)
		}
		p.match(TokenSemicolon)
	}

	if len(p.steps) == 0 {
		return nil, fmt.Errorf("%w: intent expression must declare at least one STEP", ErrMalformedIntentSyntax)
	}

	return root, nil
}

func (p *intentParser) parseStatement() (*IntentASTNode, error) {
	tok := p.peek()
	switch tok.Kind {
	case TokenGoal:
		return p.parseGoal()
	case TokenStage:
		return p.parseStage()
	case TokenStep:
		return p.parseStepChain()
	case TokenParallel:
		return p.parseParallel()
	default:
		return nil, fmt.Errorf("%w: expected GOAL, STAGE, STEP, or PARALLEL, got %q at line %d column %d", ErrMalformedIntentSyntax, tok.Literal, tok.Line, tok.Column)
	}
}

func (p *intentParser) parseGoal() (*IntentASTNode, error) {
	p.advance()
	tok := p.peek()
	if tok.Kind != TokenString && tok.Kind != TokenIdent {
		return nil, fmt.Errorf("%w: GOAL requires a string or identifier at line %d column %d", ErrMalformedIntentSyntax, tok.Line, tok.Column)
	}
	p.advance()

	goalNode := &IntentASTNode{
		ID:    "goal",
		Kind:  NodeKindGoal,
		Label: tok.Literal,
	}
	if p.match(TokenArrow) {
		if p.peek().Kind == TokenEOF || p.peek().Kind == TokenSemicolon {
			return nil, fmt.Errorf("%w: expected statement after '->' at line %d column %d", ErrMalformedIntentSyntax, p.peek().Line, p.peek().Column)
		}
		child, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if child != nil {
			goalNode.Children = append(goalNode.Children, child)
		}
	}
	return goalNode, nil
}

func (p *intentParser) parseStage() (*IntentASTNode, error) {
	p.advance()
	idTok := p.peek()
	if idTok.Kind != TokenIdent {
		return nil, fmt.Errorf("%w: STAGE requires an identifier at line %d column %d", ErrMalformedIntentSyntax, idTok.Line, idTok.Column)
	}
	p.advance()
	if _, exists := p.stages[idTok.Literal]; exists {
		return nil, fmt.Errorf("%w: duplicate stage %q", ErrDuplicateIntentNode, idTok.Literal)
	}
	p.stages[idTok.Literal] = struct{}{}

	stageNode := &IntentASTNode{
		ID:   idTok.Literal,
		Kind: NodeKindStage,
	}
	if !p.match(TokenLBrace) {
		return nil, fmt.Errorf("%w: STAGE %q requires '{' block at line %d column %d", ErrMalformedIntentSyntax, idTok.Literal, p.peek().Line, p.peek().Column)
	}
	for p.peek().Kind != TokenRBrace {
		if p.peek().Kind == TokenEOF {
			return nil, fmt.Errorf("%w: unclosed '{' in STAGE %q", ErrMalformedIntentSyntax, idTok.Literal)
		}
		if p.match(TokenSemicolon) {
			continue
		}
		child, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if child != nil {
			stageNode.Children = append(stageNode.Children, child)
		}
		p.match(TokenSemicolon)
	}
	p.advance()
	return stageNode, nil
}

func (p *intentParser) parseParallel() (*IntentASTNode, error) {
	p.advance()
	parNode := &IntentASTNode{
		ID:   fmt.Sprintf("parallel_%d", p.pos),
		Kind: NodeKindParallel,
	}
	if !p.match(TokenLBrace) {
		return nil, fmt.Errorf("%w: PARALLEL requires '{' block at line %d column %d", ErrMalformedIntentSyntax, p.peek().Line, p.peek().Column)
	}
	for p.peek().Kind != TokenRBrace {
		if p.peek().Kind == TokenEOF {
			return nil, fmt.Errorf("%w: unclosed '{' in PARALLEL block", ErrMalformedIntentSyntax)
		}
		if p.match(TokenSemicolon) {
			continue
		}
		child, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if child != nil {
			parNode.Children = append(parNode.Children, child)
		}
		p.match(TokenSemicolon)
	}
	p.advance()
	return parNode, nil
}

func (p *intentParser) parseStepChain() (*IntentASTNode, error) {
	first, err := p.parseSingleStep("")
	if err != nil {
		return nil, err
	}
	prevID := first.ID
	if !p.match(TokenThen) {
		return first, nil
	}

	chain := &IntentASTNode{
		ID:       fmt.Sprintf("chain_%s", first.ID),
		Kind:     NodeKindStage,
		Children: []*IntentASTNode{first},
	}
	for {
		if p.peek().Kind != TokenStep {
			tok := p.peek()
			return nil, fmt.Errorf("%w: expected STEP after THEN at line %d column %d", ErrMalformedIntentSyntax, tok.Line, tok.Column)
		}
		nextNode, err := p.parseSingleStep(prevID)
		if err != nil {
			return nil, err
		}
		chain.Children = append(chain.Children, nextNode)
		prevID = nextNode.ID
		if !p.match(TokenThen) {
			break
		}
	}
	return chain, nil
}

func (p *intentParser) parseSingleStep(implicitDep string) (*IntentASTNode, error) {
	p.advance()
	idTok := p.peek()
	if idTok.Kind != TokenIdent {
		return nil, fmt.Errorf("%w: STEP requires an identifier at line %d column %d", ErrMalformedIntentSyntax, idTok.Line, idTok.Column)
	}
	p.advance()

	var label string
	if p.peek().Kind == TokenString {
		label = p.advance().Literal
	}

	var deps []string
	if implicitDep != "" {
		deps = append(deps, implicitDep)
	}

	hasDependsOn := false
	if p.match(TokenDependsOn) {
		hasDependsOn = true
		parsedDeps, err := p.parseDependencyList()
		if err != nil {
			return nil, err
		}
		deps = append(deps, parsedDeps...)
	}

	if existing, exists := p.steps[idTok.Literal]; exists {
		if !hasDependsOn || label != "" || implicitDep != "" {
			return nil, fmt.Errorf("%w: duplicate step %q", ErrDuplicateIntentNode, idTok.Literal)
		}
		existing.DependsOn = mergeUniqueStrings(existing.DependsOn, deps)
		return nil, nil
	}

	node := &IntentASTNode{
		ID:        idTok.Literal,
		Kind:      NodeKindStep,
		Label:     label,
		DependsOn: mergeUniqueStrings(nil, deps),
	}
	p.steps[node.ID] = node
	return node, nil
}

func (p *intentParser) parseDependencyList() ([]string, error) {
	inParens := p.match(TokenLParen)
	var deps []string
	for {
		tok := p.peek()
		if tok.Kind != TokenIdent {
			return nil, fmt.Errorf("%w: expected dependency identifier at line %d column %d", ErrMalformedIntentSyntax, tok.Line, tok.Column)
		}
		deps = append(deps, p.advance().Literal)
		if !p.match(TokenComma) {
			break
		}
	}
	if inParens && !p.match(TokenRParen) {
		tok := p.peek()
		return nil, fmt.Errorf("%w: expected ')' at line %d column %d", ErrMalformedIntentSyntax, tok.Line, tok.Column)
	}
	return deps, nil
}

func mergeUniqueStrings(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, s := range base {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	for _, s := range extra {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// Synthesize validates dependencies, detects cycles, and builds a topologically ordered IntentPlan.
func (c *IntentCompiler) Synthesize(root *IntentASTNode) (*IntentPlan, error) {
	if root == nil {
		return nil, fmt.Errorf("%w: nil AST root", ErrMalformedIntentSyntax)
	}

	steps := make(map[string]*IntentASTNode)
	var goal string
	collectASTNodes(root, steps, &goal)

	if len(steps) == 0 {
		return nil, fmt.Errorf("%w: no executable steps in AST", ErrMalformedIntentSyntax)
	}

	inDegree, dependents, err := buildStepGraph(steps)
	if err != nil {
		return nil, err
	}

	stages, ordered, err := topoSortStages(inDegree, dependents)
	if err != nil {
		return nil, err
	}

	digest := computeIntentPlanDigest(goal, steps, stages)
	return &IntentPlan{
		Goal:           goal,
		Root:           root,
		Steps:          steps,
		OrderedStepIDs: ordered,
		Stages:         stages,
		Digest:         digest,
	}, nil
}

func collectASTNodes(node *IntentASTNode, steps map[string]*IntentASTNode, goal *string) {
	if node == nil {
		return
	}
	collectors := map[IntentNodeKind]func(*IntentASTNode){
		NodeKindGoal: func(n *IntentASTNode) {
			if *goal == "" {
				*goal = n.Label
			}
		},
		NodeKindStep: func(n *IntentASTNode) {
			steps[n.ID] = n
		},
	}
	if fn, ok := collectors[node.Kind]; ok {
		fn(node)
	}
	for _, child := range node.Children {
		collectASTNodes(child, steps, goal)
	}
}

func buildStepGraph(steps map[string]*IntentASTNode) (map[string]int, map[string][]string, error) {
	inDegree := make(map[string]int, len(steps))
	dependents := make(map[string][]string, len(steps))
	for id := range steps {
		inDegree[id] = 0
	}

	for id, step := range steps {
		for _, dep := range step.DependsOn {
			if err := strutil.ErrorOnBlank(dep, "dependency identifier cannot be blank"); err != nil {
				return nil, nil, fmt.Errorf("%w: %v", ErrMalformedIntentSyntax, err)
			}
			if dep == id {
				return nil, nil, fmt.Errorf("%w: step %q depends on itself", ErrIntentCycleDetected, id)
			}
			if _, ok := steps[dep]; !ok {
				return nil, nil, fmt.Errorf("%w: step %q depends on missing step %q", ErrUnknownDependency, id, dep)
			}
			inDegree[id]++
			dependents[dep] = append(dependents[dep], id)
		}
	}
	return inDegree, dependents, nil
}

func topoSortStages(inDegree map[string]int, dependents map[string][]string) ([]IntentStage, []string, error) {
	total := len(inDegree)
	ordered := make([]string, 0, total)
	var stages []IntentStage

	for len(ordered) < total {
		var ready []string
		for id, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			return nil, nil, fmt.Errorf("%w: cyclic dependency among remaining steps", ErrIntentCycleDetected)
		}
		sort.Strings(ready)

		stages = append(stages, IntentStage{
			Index:   len(stages),
			StepIDs: ready,
		})
		for _, id := range ready {
			inDegree[id] = -1
			ordered = append(ordered, id)
			for _, next := range dependents[id] {
				inDegree[next]--
			}
		}
	}

	return stages, ordered, nil
}

func computeIntentPlanDigest(goal string, steps map[string]*IntentASTNode, stages []IntentStage) string {
	var b strings.Builder
	b.WriteString("goal:")
	b.WriteString(goal)
	b.WriteByte('\n')

	ids := make([]string, 0, len(steps))
	for id := range steps {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		step := steps[id]
		deps := append([]string(nil), step.DependsOn...)
		sort.Strings(deps)
		b.WriteString("step:")
		b.WriteString(id)
		b.WriteByte('|')
		b.WriteString(step.Label)
		b.WriteByte('|')
		b.WriteString(strings.Join(deps, ","))
		b.WriteByte('\n')
	}

	for _, st := range stages {
		b.WriteString(fmt.Sprintf("stage:%d:%s\n", st.Index, strings.Join(st.StepIDs, ",")))
	}

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
