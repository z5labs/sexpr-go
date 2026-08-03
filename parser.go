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

// advance pulls the next token which carries a datum, skipping comments.
func (p *parser) advance() (Token, error, bool) {
	for {
		tok, err, ok := p.next()
		if err != nil {
			return Token{}, err, false
		}
		if !ok {
			return Token{}, nil, false
		}

		// A comment is not a datum, so it never reaches the parse actions.
		// Collecting comments into File.Comments is a later story.
		if tok.Type == TokenComment {
			continue
		}

		return tok, nil, true
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

// datumTokens are the token types which may begin a datum. Lists, dotted pairs,
// and quote forms extend this in later stories.
var datumTokens = []TokenType{TokenSymbol, TokenString, TokenNumber, TokenBool}

func parseFile(p *parser, file *File) (parserAction[*File], error) {
	tok, err, ok := p.read()
	if err != nil {
		return nil, err
	}
	if !ok {
		// No tokens left, so the file is complete.
		return nil, nil
	}

	node, err := parseDatum(tok)
	if err != nil {
		return nil, err
	}

	file.Nodes = append(file.Nodes, node)

	// Keep going so that several top level datums are read.
	return parseFile, nil
}

// parseDatum turns a single token into its node.
func parseDatum(tok Token) (Node, error) {
	switch tok.Type {
	case TokenSymbol:
		// nil is spelled like a symbol but denotes the empty value.
		if string(tok.Value) == "nil" {
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
