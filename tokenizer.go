// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Pos represents the position of a token in the input.
//
// Line is 1-based. Column is a 1-based byte offset into the line, so a
// multi-byte rune advances Column by its encoded width rather than by one.
type Pos struct {
	Line   int
	Column int
}

// Token represents a token in an S-expression.
type Token struct {
	Pos   Pos
	Type  TokenType
	Value []byte
}

func (t Token) String() string {
	return fmt.Sprintf("%s(%s)", t.Type, t.Value)
}

// TokenType represents the type of a token.
type TokenType int

const (
	TokenLParen  TokenType = iota // "("
	TokenRParen                   // ")"
	TokenSymbol                   // e.g. add, x, +, ->list
	TokenComment                  // e.g. ; comment or #| comment |#
	TokenString                   // e.g. "hello"
)

func (tt TokenType) String() string {
	switch tt {
	case TokenLParen:
		return "LParen"
	case TokenRParen:
		return "RParen"
	case TokenSymbol:
		return "Symbol"
	case TokenComment:
		return "Comment"
	case TokenString:
		return "String"
	default:
		panic(fmt.Sprintf("unknown token type: %d", tt))
	}
}

// Tokenize the S-expression defined in the given reader.
func Tokenize(r io.Reader) iter.Seq2[Token, error] {
	return func(yield func(Token, error) bool) {
		t := &tokenizer{
			pos: Pos{Line: 1, Column: 1},
			buf: bufio.NewReader(r),
		}

		for action := tokenizeSexpr; action != nil; {
			action = action(t, yield)
		}
	}
}

type tokenizer struct {
	// pos tracks the current position in the input for error reporting.
	pos Pos

	buf *bufio.Reader
}

func (t *tokenizer) next() (rune, error) {
	r, size, err := t.buf.ReadRune()
	if err != nil {
		return 0, err
	}
	t.pos.Column += size
	if r == '\n' {
		t.pos.Line++
		t.pos.Column = 1
	}
	return r, nil
}

func (t *tokenizer) backup(previousPos Pos) error {
	err := t.buf.UnreadRune()
	if err != nil {
		return err
	}
	t.pos = previousPos
	return nil
}

func (t *tokenizer) copyIf(buf *bytes.Buffer, cond func(rune) bool) error {
	for {
		r, size, err := t.buf.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF
			}
			return err
		}

		if !cond(r) {
			err = t.buf.UnreadRune()
			return err
		}

		_, err = buf.WriteRune(r)
		if err != nil {
			return err
		}

		t.pos.Column += size
		if r == '\n' {
			t.pos.Line++
			t.pos.Column = 1
		}
	}
}

func (t *tokenizer) copyUntil(dst *bytes.Buffer, delim []rune) error {
	// buf holds the trailing window of runes which may yet complete delim. Runes
	// leave the window into dst once they can no longer be part of a match.
	buf := make([]rune, 0, len(delim))

	for {
		r, size, err := t.buf.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				// The delimiter never arrived, so nothing still in the window can
				// be part of a match. Flush it so dst holds everything consumed.
				for _, pending := range buf {
					if _, werr := dst.WriteRune(pending); werr != nil {
						return werr
					}
				}
				return io.ErrUnexpectedEOF
			}
			return err
		}

		if len(buf) == len(delim) {
			popRune := buf[0]
			buf = buf[1:]

			_, err := dst.WriteRune(popRune)
			if err != nil {
				return err
			}
		}

		buf = append(buf, r)

		// Account for every rune consumed, including those completing delim, so
		// that pos still refers to the reader's offset once this returns.
		t.pos.Column += size
		if r == '\n' {
			t.pos.Line++
			t.pos.Column = 1
		}

		if slices.Equal(buf, delim) {
			return nil
		}
	}
}

// endsWith reports whether window's trailing runes are exactly delim.
func endsWith(window, delim []rune) bool {
	if len(window) < len(delim) {
		return false
	}
	return slices.Equal(window[len(window)-len(delim):], delim)
}

// copyNested copies a construct delimited by open and closing, assuming the
// caller has already consumed one opening delimiter, and stops once that
// opening delimiter's matching close is consumed. Nested open/closing pairs are
// counted rather than terminating the copy, which is why [tokenizer.copyUntil]
// cannot serve here. Everything read, including the final closing delimiter, is
// written to dst.
//
// open and closing may differ in length. They must not be suffixes of one
// another, since a match is decided on the trailing runes read so far.
func (t *tokenizer) copyNested(dst *bytes.Buffer, open, closing []rune) error {
	// window holds the trailing runes which may complete either delimiter, so it
	// must be wide enough for the longer of the two. It is reset after a match so
	// that a delimiter rune is never counted twice, letting runs like "#|#|" nest
	// as two separate opens.
	width := max(len(open), len(closing))
	window := make([]rune, 0, width)

	for depth := 1; ; {
		r, size, err := t.buf.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF
			}
			return err
		}

		if _, err := dst.WriteRune(r); err != nil {
			return err
		}

		t.pos.Column += size
		if r == '\n' {
			t.pos.Line++
			t.pos.Column = 1
		}

		window = append(window, r)
		if len(window) > width {
			window = window[len(window)-width:]
		}

		switch {
		case endsWith(window, open):
			depth++
			window = window[:0]
		case endsWith(window, closing):
			depth--
			window = window[:0]
			if depth == 0 {
				return nil
			}
		}
	}
}

// isHexDigit reports whether r is a hexadecimal digit in either case.
func isHexDigit(r rune) bool {
	switch {
	case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		return true
	default:
		return false
	}
}

// copyEscape consumes the escape sequence following a backslash and writes it
// to dst verbatim, backslash included. The backslash itself has already been
// consumed and escapePos is its position.
//
// Escapes are validated but not decoded — the parser turns them into their
// real runes, so the token keeps the source text.
func (t *tokenizer) copyEscape(dst *bytes.Buffer, escapePos Pos) error {
	esc, err := t.next()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		return err
	}

	switch esc {
	case '"', '\\', 'n', 'r', 't', 'b', 'f':
		dst.WriteRune('\\')
		dst.WriteRune(esc)
		return nil
	case 'u':
		// \u must be followed by exactly four hex digits.
		var digits [4]rune
		for i := range digits {
			d, err := t.next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return io.ErrUnexpectedEOF
				}
				return err
			}
			if !isHexDigit(d) {
				return InvalidEscapeError{Pos: escapePos, R: 'u'}
			}
			digits[i] = d
		}

		dst.WriteRune('\\')
		dst.WriteRune('u')
		for _, d := range digits {
			dst.WriteRune(d)
		}
		return nil
	default:
		return InvalidEscapeError{Pos: escapePos, R: esc}
	}
}

type tokenizerAction func(t *tokenizer, yield func(Token, error) bool) tokenizerAction

func yieldErrorOr(err error, next tokenizerAction) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		if err == nil {
			return next
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil
		}
		if !yield(Token{}, err) {
			return nil
		}
		return next
	}
}

func yieldTokenThen(tok Token, next tokenizerAction) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		if !yield(tok, nil) {
			return nil
		}
		return next
	}
}

func skipWhitespace(next tokenizerAction) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var buf bytes.Buffer
		err := t.copyIf(&buf, unicode.IsSpace)

		return yieldErrorOr(err, next)
	}
}

// UnexpectedCharacterError is the error returned by the tokenizer when it encounters an unexpected character in the input.
type UnexpectedCharacterError struct {
	Pos Pos
	R   rune
}

// Error implements the [error] interface.
func (e UnexpectedCharacterError) Error() string {
	return fmt.Sprintf("unexpected character '%c' at line %d, column %d", e.R, e.Pos.Line, e.Pos.Column)
}

// UnterminatedCommentError is the error returned by the tokenizer when a block comment is never closed.
type UnterminatedCommentError struct {
	Pos Pos
}

// Error implements the [error] interface.
func (e UnterminatedCommentError) Error() string {
	return fmt.Sprintf("unterminated block comment at line %d, column %d", e.Pos.Line, e.Pos.Column)
}

// UnterminatedStringError is the error returned by the tokenizer when a string literal is never closed.
type UnterminatedStringError struct {
	Pos Pos
}

// Error implements the [error] interface.
func (e UnterminatedStringError) Error() string {
	return fmt.Sprintf("unterminated string literal at line %d, column %d", e.Pos.Line, e.Pos.Column)
}

// InvalidEscapeError is the error returned by the tokenizer when a string literal contains an unrecognized escape sequence.
type InvalidEscapeError struct {
	Pos Pos
	R   rune
}

// Error implements the [error] interface.
func (e InvalidEscapeError) Error() string {
	return fmt.Sprintf("invalid escape sequence '\\%c' at line %d, column %d", e.R, e.Pos.Line, e.Pos.Column)
}

func tokenizeSexpr(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
	return skipWhitespace(
		func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
			pos := t.pos
			r, err := t.next()

			return yieldErrorOr(
				err,
				func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
					switch {
					case r == '(':
						return tokenizeLParen(pos)
					case r == ')':
						return tokenizeRParen(pos)
					case r == ';':
						return tokenizeLineComment(pos)
					case r == '#':
						return tokenizeHash(pos)
					case r == '"':
						return tokenizeString(pos)
					case isSymbolRune(r):
						err = t.backup(pos)
						return yieldErrorOr(err, tokenizeSymbol)
					default:
						return yieldErrorOr(UnexpectedCharacterError{Pos: pos, R: r}, nil)
					}
				},
			)
		},
	)
}

// symbolPunctuation are the punctuation characters which may appear in a symbol.
const symbolPunctuation = `+-*/<>=!?:$%_&~^@`

// isSymbolRune reports whether r may appear in a symbol. Symbols are made up of
// letters, digits, and the punctuation in symbolPunctuation. [unicode.IsLetter]
// admits non-ASCII letters as well.
func isSymbolRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(symbolPunctuation, r)
}

// startsWithNumber reports whether the lexeme begins with a sequence that forms
// a valid number, namely an optional sign followed by a digit. Such a lexeme is
// not a symbol.
func startsWithNumber(b []byte) bool {
	r, size := utf8.DecodeRune(b)
	if r == '+' || r == '-' {
		r, _ = utf8.DecodeRune(b[size:])
	}
	return unicode.IsDigit(r)
}

func tokenizeLParen(pos Pos) tokenizerAction {
	return yieldTokenThen(
		Token{Pos: pos, Type: TokenLParen, Value: []byte("(")},
		tokenizeSexpr,
	)
}

func tokenizeRParen(pos Pos) tokenizerAction {
	return yieldTokenThen(
		Token{Pos: pos, Type: TokenRParen, Value: []byte(")")},
		tokenizeSexpr,
	)
}

// blockCommentOpen and blockCommentClose delimit a nestable block comment.
var (
	blockCommentOpen  = []rune{'#', '|'}
	blockCommentClose = []rune{'|', '#'}
)

// tokenizeHash dispatches on the character following a '#'. Only block comments
// are recognised for now; the remaining hash forms land in a later story.
func tokenizeHash(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		r, err := t.next()
		if err != nil {
			// A trailing '#' is an incomplete dispatch rather than a clean end.
			if errors.Is(err, io.EOF) {
				return yieldErrorOr(UnexpectedCharacterError{Pos: pos, R: '#'}, nil)
			}
			return yieldErrorOr(err, nil)
		}

		if r == '|' {
			return tokenizeBlockComment(pos)
		}
		return yieldErrorOr(UnexpectedCharacterError{Pos: pos, R: '#'}, nil)
	}
}

// tokenizeLineComment scans a ';' comment through end of line. The ';' has
// already been consumed and is included in the token's value.
func tokenizeLineComment(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var comment bytes.Buffer
		comment.WriteRune(';')

		err := t.copyIf(&comment, func(r rune) bool {
			return r != '\n'
		})

		tok := Token{Pos: pos, Type: TokenComment, Value: comment.Bytes()}

		// A comment running to the end of input without a trailing newline is
		// still a complete comment.
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return yieldTokenThen(tok, nil)
		}

		return yieldErrorOr(
			err,
			yieldTokenThen(tok, skipWhitespace(tokenizeSexpr)),
		)
	}
}

// tokenizeBlockComment scans a nestable '#| ... |#' comment. The opening '#|'
// has already been consumed and both delimiters are included in the token's
// value.
func tokenizeBlockComment(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var comment bytes.Buffer
		comment.WriteString(string(blockCommentOpen))

		err := t.copyNested(&comment, blockCommentOpen, blockCommentClose)

		// Running out of input mid-comment is a real error rather than the clean
		// termination yieldErrorOr would otherwise read io.ErrUnexpectedEOF as.
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return yieldErrorOr(UnterminatedCommentError{Pos: pos}, nil)
		}

		return yieldErrorOr(
			err,
			yieldTokenThen(
				Token{Pos: pos, Type: TokenComment, Value: comment.Bytes()},
				skipWhitespace(tokenizeSexpr),
			),
		)
	}
}

// tokenizeString scans a double quoted string literal. The opening quote has
// already been consumed and neither quote appears in the token's value, which
// holds the source text with escape sequences left undecoded.
func tokenizeString(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		// Value stays non-nil so that an empty literal is an empty value rather
		// than an absent one.
		str := bytes.NewBuffer([]byte{})

		for {
			cur := t.pos
			r, err := t.next()
			if err != nil {
				// Running out of input mid-literal means the closing quote never
				// arrived, which is an error rather than a clean end.
				if errors.Is(err, io.EOF) {
					return yieldErrorOr(UnterminatedStringError{Pos: pos}, nil)
				}
				return yieldErrorOr(err, nil)
			}

			switch r {
			case '"':
				return yieldTokenThen(
					Token{Pos: pos, Type: TokenString, Value: str.Bytes()},
					skipWhitespace(tokenizeSexpr),
				)
			case '\\':
				if err := t.copyEscape(str, cur); err != nil {
					if errors.Is(err, io.ErrUnexpectedEOF) {
						return yieldErrorOr(UnterminatedStringError{Pos: pos}, nil)
					}
					return yieldErrorOr(err, nil)
				}
			default:
				// A literal newline is allowed; next has already advanced the line.
				str.WriteRune(r)
			}
		}
	}
}

func tokenizeSymbol(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
	pos := t.pos

	var sym bytes.Buffer
	err := t.copyIf(&sym, isSymbolRune)

	// A symbol may not begin with a sequence that forms a valid number. Number
	// literals are tokenized in a later story; until then such a lexeme is not
	// a token this tokenizer produces.
	if err == nil || errors.Is(err, io.ErrUnexpectedEOF) {
		if startsWithNumber(sym.Bytes()) {
			r, _ := utf8.DecodeRune(sym.Bytes())
			return yieldErrorOr(UnexpectedCharacterError{Pos: pos, R: r}, nil)
		}
	}

	tok := Token{Pos: pos, Type: TokenSymbol, Value: sym.Bytes()}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return yieldTokenThen(tok, nil)
	}

	return yieldErrorOr(
		err,
		yieldTokenThen(tok, skipWhitespace(tokenizeSexpr)),
	)
}
