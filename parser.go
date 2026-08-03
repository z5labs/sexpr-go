// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Node represents a node in the S-expression AST.
//
// The interface is sealed by an unexported method, so only this package can
// implement it and a type switch over the node types is exhaustive.
type Node interface {
	sexpr()
}

// Symbol represents a symbol such as add or ->list.
type Symbol struct {
	Pos   Pos
	Value string
}

func (Symbol) sexpr() {}

// String represents a string literal. Value holds the decoded text, with the
// escape sequences of the source resolved.
type String struct {
	Pos   Pos
	Value string
}

func (String) sexpr() {}

// Int represents an integer literal.
type Int struct {
	Pos   Pos
	Value int64
}

func (Int) sexpr() {}

// Float represents a floating point literal.
type Float struct {
	Pos   Pos
	Value float64
}

func (Float) sexpr() {}

// Bool represents a boolean literal, written #t, #true, #f, or #false.
type Bool struct {
	Pos   Pos
	Value bool
}

func (Bool) sexpr() {}

// Nil represents the empty value, written nil.
type Nil struct {
	Pos Pos
}

func (Nil) sexpr() {}

// List represents a parenthesized list. An empty list has no elements and is
// distinct from [Nil].
//
// Tail holds the datum after the dot of an improper list such as (a . b), and
// is nil for a proper list. Elements always holds what precedes the dot, so
// (a b . c) has two elements and a tail.
type List struct {
	Pos      Pos
	Elements []Node
	Tail     Node

	// Comments holds the comments written inside this list, in the order they
	// appear. They are attached to the list rather than to any one element.
	Comments []*Comment
}

func (List) sexpr() {}

// QuoteKind identifies which reader macro produced a [Quote].
type QuoteKind int

const (
	QuoteKindQuote           QuoteKind = iota // '
	QuoteKindQuasiquote                       // `
	QuoteKindUnquote                          // ,
	QuoteKindUnquoteSplicing                  // ,@
)

func (k QuoteKind) String() string {
	switch k {
	case QuoteKindQuote:
		return "Quote"
	case QuoteKindQuasiquote:
		return "Quasiquote"
	case QuoteKindUnquote:
		return "Unquote"
	case QuoteKindUnquoteSplicing:
		return "UnquoteSplicing"
	default:
		panic(fmt.Sprintf("unknown quote kind: %d", k))
	}
}

// Quote represents a datum written with one of the reader macro shorthands.
//
// The shorthand is kept rather than rewritten to (quote x), so that printing
// reproduces what was written.
type Quote struct {
	Pos   Pos
	Kind  QuoteKind
	Datum Node
}

func (Quote) sexpr() {}

// Comment represents a comment in the source.
type Comment struct {
	Pos  Pos
	Text string
}

// File represents a parsed S-expression source, which may hold any number of
// top level datums.
type File struct {
	Nodes    []Node
	Comments []*Comment
}

// MaxDepth is the deepest list nesting [Parse] accepts.
//
// Parsing is recursive descent, so nesting costs stack. Rather than let a
// pathological input exhaust it, anything deeper than this fails with a
// [MaxDepthExceededError].
const MaxDepth = 10_000

// MaxDepthExceededError is the error returned by the parser when the input nests deeper than [MaxDepth].
type MaxDepthExceededError struct {
	Pos   Pos
	Depth int
}

// Error implements the [error] interface.
func (e MaxDepthExceededError) Error() string {
	return fmt.Sprintf(
		"maximum nesting depth of %d exceeded at line %d, column %d",
		e.Depth,
		e.Pos.Line,
		e.Pos.Column,
	)
}

// UnexpectedEndOfTokensError is the error returned by the parser when it reaches the end of the tokens unexpectedly.
type UnexpectedEndOfTokensError struct {
	Expected []TokenType
	Pos      Pos
}

// Error implements the [error] interface.
func (e UnexpectedEndOfTokensError) Error() string {
	var expected []string
	for _, t := range e.Expected {
		expected = append(expected, t.String())
	}
	return fmt.Sprintf(
		"unexpected end of tokens at line %d, column %d, expected one of: %s",
		e.Pos.Line,
		e.Pos.Column,
		strings.Join(expected, ", "),
	)
}

// UnexpectedTokenError is the error returned by the parser when it encounters an unexpected token.
type UnexpectedTokenError struct {
	Expected []TokenType
	Actual   Token
}

// Error implements the [error] interface.
func (e UnexpectedTokenError) Error() string {
	var expected []string
	for _, t := range e.Expected {
		expected = append(expected, t.String())
	}
	return fmt.Sprintf(
		"unexpected token at line %d, column %d: %s, expected one of: %s",
		e.Actual.Pos.Line,
		e.Actual.Pos.Column,
		e.Actual.String(),
		strings.Join(expected, ", "),
	)
}

// NumberRangeError is the error returned by the parser when a number literal is
// well formed but cannot be represented as an int64 or a float64.
type NumberRangeError struct {
	Pos   Pos
	Value string
}

// Error implements the [error] interface.
func (e NumberRangeError) Error() string {
	return fmt.Sprintf("number literal %q out of range at line %d, column %d", e.Value, e.Pos.Line, e.Pos.Column)
}

// Parse the S-expressions defined in the given reader.
func Parse(r io.Reader) (*File, error) {
	next, stop := iter.Pull2(Tokenize(r))
	defer stop()

	file := &File{}

	p := &parser{
		next: next,
		pos:  Pos{Line: 1, Column: 1},
	}

	var err error
	for action := parseFile; action != nil && err == nil; {
		action, err = action(p, file)
	}
	if err != nil {
		return nil, err
	}

	return file, nil
}

type parser struct {
	next    func() (Token, error, bool)
	pending *Token
	pos     Pos
}

// advance pulls the next token, comments included.
//
// Comments are not datums, so every site which expects one drains them first
// with [parser.collectComments]; that is what keeps them out of File.Nodes and
// List.Elements while still recording them.
func (p *parser) advance() (Token, error, bool) {
	tok, err, ok := p.next()
	if err != nil {
		return Token{}, err, false
	}
	if !ok {
		return Token{}, nil, false
	}
	return tok, nil, true
}

// collectComments drains the comment tokens at the current position into dst,
// leaving the reader on the next token which is not a comment.
//
// Comments land in the container being read, so dst is the enclosing [File] or
// [List]. Appending as they are met keeps them in position order.
func (p *parser) collectComments(dst *[]*Comment) error {
	for {
		tok, err, ok := p.peek()
		if err != nil {
			return err
		}
		if !ok || tok.Type != TokenComment {
			return nil
		}

		if _, err, _ = p.read(); err != nil {
			return err
		}

		*dst = append(*dst, &Comment{Pos: tok.Pos, Text: string(tok.Value)})
	}
}

func (p *parser) unread(tok Token) {
	p.pending = &tok
}

func (p *parser) read() (Token, error, bool) {
	if p.pending != nil {
		tok := *p.pending
		p.pending = nil
		p.pos = tok.Pos
		return tok, nil, true
	}

	tok, err, ok := p.advance()
	if ok {
		p.pos = tok.Pos
	}
	return tok, err, ok
}

func (p *parser) peek() (Token, error, bool) {
	if p.pending != nil {
		return *p.pending, nil, true
	}

	tok, err, ok := p.advance()
	if err != nil {
		return Token{}, err, false
	}
	if !ok {
		return Token{}, nil, false
	}

	p.pending = &tok
	return tok, nil, true
}

func (p *parser) expect(expected ...TokenType) (Token, error) {
	tok, err, ok := p.read()
	if err != nil {
		return Token{}, err
	}
	if !ok {
		return Token{}, p.unexpectedEndOfTokens(expected...)
	}

	if slices.Contains(expected, tok.Type) {
		return tok, nil
	}

	return Token{}, UnexpectedTokenError{
		Expected: expected,
		Actual:   tok,
	}
}

func (p *parser) unexpectedEndOfTokens(expected ...TokenType) UnexpectedEndOfTokensError {
	return UnexpectedEndOfTokensError{
		Expected: expected,
		Pos:      p.pos,
	}
}

type parserAction[T any] func(p *parser, t T) (parserAction[T], error)

// nilLiteral is the spelling which denotes the empty value. It tokenizes as a
// symbol, so the parser turns it into a [Nil] and the printer must refuse to
// write a [Symbol] carrying it.
const nilLiteral = "nil"

// datumTokens are the token types which may begin a datum. The dot of a dotted
// pair is absent because it separates two datums rather than starting one, and
// a closing parenthesis only ever ends a list.
var datumTokens = []TokenType{TokenLParen, TokenQuote, TokenSymbol, TokenString, TokenNumber, TokenBool}

func parseFile(p *parser, file *File) (parserAction[*File], error) {
	// Comments before a top level datum, and any trailing ones, belong to the
	// file.
	if err := p.collectComments(&file.Comments); err != nil {
		return nil, err
	}

	tok, err, ok := p.read()
	if err != nil {
		return nil, err
	}
	if !ok {
		// No tokens left, so the file is complete.
		return nil, nil
	}

	node, err := p.parseDatum(tok, 0, &file.Comments)
	if err != nil {
		return nil, err
	}

	file.Nodes = append(file.Nodes, node)

	// Keep going so that several top level datums are read.
	return parseFile, nil
}

// parseList reads the elements of a list up to its closing parenthesis. The
// opening parenthesis at pos has already been consumed, and depth is this
// list's own nesting level as counted by [parser.parseDatum].
func (p *parser) parseList(pos Pos, depth int) (Node, error) {
	if depth > MaxDepth {
		return nil, MaxDepthExceededError{Pos: pos, Depth: MaxDepth}
	}

	list := List{Pos: pos}
	for {
		// Comments inside the list belong to it, wherever in the list they sit.
		if err := p.collectComments(&list.Comments); err != nil {
			return nil, err
		}

		tok, err, ok := p.read()
		if err != nil {
			return nil, err
		}
		if !ok {
			// The list was opened but never closed.
			return nil, p.unexpectedEndOfTokens(TokenRParen)
		}

		if tok.Type == TokenRParen {
			return list, nil
		}

		if tok.Type == TokenDot {
			// A dot must separate two halves, so something has to precede it.
			if len(list.Elements) == 0 {
				return nil, UnexpectedTokenError{
					Expected: datumTokens,
					Actual:   tok,
				}
			}

			tail, err := p.parseTail(depth, &list.Comments)
			if err != nil {
				return nil, err
			}
			list.Tail = tail

			return list, nil
		}

		element, err := p.parseDatum(tok, depth, &list.Comments)
		if err != nil {
			return nil, err
		}
		list.Elements = append(list.Elements, element)
	}
}

// parseTail reads the single datum after a dot and the closing parenthesis
// which must follow it. The dot has already been consumed, and depth is that of
// the list being read, since the tail sits inside the same list.
func (p *parser) parseTail(depth int, comments *[]*Comment) (Node, error) {
	if err := p.collectComments(comments); err != nil {
		return nil, err
	}

	tok, err, ok := p.read()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.unexpectedEndOfTokens(datumTokens...)
	}

	// A closing parenthesis here means the tail is missing, as in "(a . )",
	// which parseDatum reports as the unexpected token it is.
	tail, err := p.parseDatum(tok, depth, comments)
	if err != nil {
		return nil, err
	}

	// Exactly one datum may follow the dot, so the list must end here.
	if err := p.collectComments(comments); err != nil {
		return nil, err
	}

	closing, err, ok := p.read()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, p.unexpectedEndOfTokens(TokenRParen)
	}
	if closing.Type != TokenRParen {
		return nil, UnexpectedTokenError{
			Expected: []TokenType{TokenRParen},
			Actual:   closing,
		}
	}

	return tail, nil
}

// quoteKinds maps a reader macro's literal text to its kind.
var quoteKinds = map[string]QuoteKind{
	"'":  QuoteKindQuote,
	"`":  QuoteKindQuasiquote,
	",":  QuoteKindUnquote,
	",@": QuoteKindUnquoteSplicing,
}

// parseQuote reads the datum a reader macro applies to. The macro token has
// already been read.
//
// Quotes count against depth even though they do not look like nesting: a run
// of them recurses just as a run of open parentheses does, so a long enough one
// would otherwise exhaust the stack.
func (p *parser) parseQuote(tok Token, depth int, comments *[]*Comment) (Node, error) {
	if depth > MaxDepth {
		return nil, MaxDepthExceededError{Pos: tok.Pos, Depth: MaxDepth}
	}

	// The tokenizer only emits the four accepted macros, so this is defensive.
	kind, ok := quoteKinds[string(tok.Value)]
	if !ok {
		return nil, UnexpectedTokenError{Expected: datumTokens, Actual: tok}
	}

	// A comment may sit between the macro and its datum.
	if err := p.collectComments(comments); err != nil {
		return nil, err
	}

	next, err, ok := p.read()
	if err != nil {
		return nil, err
	}
	if !ok {
		// The macro was written with nothing for it to apply to.
		return nil, p.unexpectedEndOfTokens(datumTokens...)
	}

	// A closing parenthesis here is reported by parseDatum as the unexpected
	// token it is.
	datum, err := p.parseDatum(next, depth, comments)
	if err != nil {
		return nil, err
	}

	return Quote{Pos: tok.Pos, Kind: kind, Datum: datum}, nil
}

// parseDatum turns tok, and any tokens belonging with it, into a node.
//
// depth counts the enclosing constructs which parse by recursing: lists and
// reader macros both do, so both add a level. A construct which reads a datum
// without recursing, such as a dotted pair's tail, shares the level of whatever
// holds it. The count exists to bound recursion, so the question for a new
// construct is whether it recurses, not whether it looks nested.
func (p *parser) parseDatum(tok Token, depth int, comments *[]*Comment) (Node, error) {
	switch tok.Type {
	case TokenLParen:
		return p.parseList(tok.Pos, depth+1)
	case TokenQuote:
		return p.parseQuote(tok, depth+1, comments)
	case TokenSymbol:
		// nil is spelled like a symbol but denotes the empty value.
		if string(tok.Value) == nilLiteral {
			return Nil{Pos: tok.Pos}, nil
		}
		return Symbol{Pos: tok.Pos, Value: string(tok.Value)}, nil
	case TokenString:
		value, err := decodeString(tok.Pos, tok.Value)
		if err != nil {
			return nil, err
		}
		return String{Pos: tok.Pos, Value: value}, nil
	case TokenNumber:
		return parseNumber(tok)
	case TokenBool:
		return Bool{Pos: tok.Pos, Value: isTrueLiteral(tok.Value)}, nil
	default:
		return nil, UnexpectedTokenError{
			Expected: datumTokens,
			Actual:   tok,
		}
	}
}

// isTrueLiteral reports whether a boolean lexeme is one of the true spellings.
// The tokenizer only ever emits the four accepted spellings.
func isTrueLiteral(value []byte) bool {
	switch string(value) {
	case "#t", "#true":
		return true
	default:
		return false
	}
}

// isFloatLexeme reports whether a number lexeme denotes a floating point value,
// which is so when it carries a fraction or an exponent.
func isFloatLexeme(lexeme string) bool {
	return strings.ContainsAny(lexeme, ".eE")
}

func parseNumber(tok Token) (Node, error) {
	lexeme := string(tok.Value)

	if isFloatLexeme(lexeme) {
		value, err := strconv.ParseFloat(lexeme, 64)
		if err != nil {
			return nil, numberError(tok.Pos, lexeme, err)
		}
		return Float{Pos: tok.Pos, Value: value}, nil
	}

	value, err := strconv.ParseInt(lexeme, 10, 64)
	if err != nil {
		return nil, numberError(tok.Pos, lexeme, err)
	}
	return Int{Pos: tok.Pos, Value: value}, nil
}

// numberError separates a value which does not fit its type from one which is
// not a number at all. The tokenizer rejects malformed lexemes, so only the
// range case is reachable through [Parse].
func numberError(pos Pos, lexeme string, err error) error {
	if errors.Is(err, strconv.ErrRange) {
		return NumberRangeError{Pos: pos, Value: lexeme}
	}
	return InvalidNumberError{Pos: pos, Value: lexeme}
}

// decodeString resolves the escape sequences the tokenizer left in place.
//
// The tokenizer has already validated them, so the error paths here are
// defensive; they report the position of the literal rather than of the
// offending escape.
func decodeString(pos Pos, raw []byte) (string, error) {
	var out strings.Builder
	out.Grow(len(raw))

	for i := 0; i < len(raw); {
		if raw[i] != '\\' {
			r, size := utf8.DecodeRune(raw[i:])
			out.WriteRune(r)
			i += size
			continue
		}

		i++
		if i >= len(raw) {
			return "", InvalidEscapeError{Pos: pos, R: '\\'}
		}

		switch raw[i] {
		case '"':
			out.WriteByte('"')
		case '\\':
			out.WriteByte('\\')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'u':
			i++
			if i+4 > len(raw) {
				return "", InvalidEscapeError{Pos: pos, R: 'u'}
			}
			// Four hex digits never exceed 0xFFFF, so a 16 bit parse states the
			// real bound and keeps the conversion to rune, an int32, in range.
			code, err := strconv.ParseUint(string(raw[i:i+4]), 16, 16)
			if err != nil {
				return "", InvalidEscapeError{Pos: pos, R: 'u'}
			}
			out.WriteRune(rune(code))
			i += 4
			continue
		default:
			r, _ := utf8.DecodeRune(raw[i:])
			return "", InvalidEscapeError{Pos: pos, R: r}
		}

		i++
	}

	return out.String(), nil
}
