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
	TokenNumber                   // e.g. 42, -0.5, 1.5e-3
	TokenDot                      // "." separating the halves of a dotted pair
	TokenBool                     // #t, #true, #f, or #false
	TokenQuote                    // a reader macro: ', `, , or ,@
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
	case TokenNumber:
		return "Number"
	case TokenDot:
		return "Dot"
	case TokenBool:
		return "Bool"
	case TokenQuote:
		return "Quote"
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

// InvalidNumberError is the error returned by the tokenizer when a lexeme is meant to be a number but is malformed.
type InvalidNumberError struct {
	Pos   Pos
	Value string
}

// Error implements the [error] interface.
func (e InvalidNumberError) Error() string {
	return fmt.Sprintf("invalid number literal %q at line %d, column %d", e.Value, e.Pos.Line, e.Pos.Column)
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
					case r == '\'':
						return tokenizeQuote(pos, "'")
					case r == '`':
						return tokenizeQuote(pos, "`")
					case r == ',':
						return tokenizeUnquote(pos)
					case isAtomRune(r):
						err = t.backup(pos)
						return yieldErrorOr(err, tokenizeAtom)
					default:
						return yieldErrorOr(UnexpectedCharacterError{Pos: pos, R: r}, nil)
					}
				},
			)
		},
	)
}

// atomPunctuation are the punctuation characters which may appear in an atom.
// '.' is included because it may be part of a symbol such as "..." and part of
// a number such as "1.5"; a lexeme consisting of nothing but '.' is the
// dotted-pair marker instead.
const atomPunctuation = `+-*/<>=!?:$%_&~^@.`

// isAtomRune reports whether r may appear in an atom, that is a symbol, a
// number, or the dotted-pair dot. [unicode.IsLetter] admits non-ASCII letters
// as well.
func isAtomRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(atomPunctuation, r)
}

// isASCIIDigit reports whether b is one of '0' to '9'. Number syntax is
// deliberately ASCII only, so a lexeme built from other digit runes stays a
// symbol rather than becoming a malformed number.
func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// looksNumeric reports whether the lexeme is an attempt at a number: after any
// leading signs it begins with a digit, or with a '.' followed by a digit.
//
// This is deliberately looser than [validNumber]. It is what separates "this
// was meant to be a number and is malformed" from "this is a symbol", so that
// "--1" is reported as a bad number while "->x2" stays a perfectly good symbol.
func looksNumeric(b []byte) bool {
	for len(b) > 0 && (b[0] == '+' || b[0] == '-') {
		b = b[1:]
	}
	if len(b) > 0 && b[0] == '.' {
		b = b[1:]
	}
	return len(b) > 0 && isASCIIDigit(b[0])
}

// validNumber reports whether the lexeme is a well formed number:
//
//	sign? ( digits ( '.' digits )? | '.' digits ) ( [eE] sign? digits )?
func validNumber(s string) bool {
	i := 0

	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}

	whole := 0
	for i < len(s) && isASCIIDigit(s[i]) {
		i++
		whole++
	}

	fraction := 0
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isASCIIDigit(s[i]) {
			i++
			fraction++
		}
		// A decimal point must be followed by at least one digit, so "1." is
		// not a number.
		if fraction == 0 {
			return false
		}
	}

	if whole == 0 && fraction == 0 {
		return false
	}

	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		exponent := 0
		for i < len(s) && isASCIIDigit(s[i]) {
			i++
			exponent++
		}
		if exponent == 0 {
			return false
		}
	}

	// Anything left over means the lexeme ran past the number, as in "123abc".
	return i == len(s)
}

// classifyAtom decides which kind of token a complete atom lexeme is.
func classifyAtom(value []byte) TokenType {
	switch {
	case string(value) == ".":
		return TokenDot
	case looksNumeric(value):
		return TokenNumber
	default:
		return TokenSymbol
	}
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

// tokenizeHash dispatches on the character following a '#': '|' opens a block
// comment and 't' or 'f' begins a boolean. Any other character, including none
// at all, is rejected. A new '#' form means a new case here.
func tokenizeHash(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		spellingPos := t.pos
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
		if r == 't' || r == 'f' {
			return tokenizeBool(pos, spellingPos, r)
		}

		// The dispatch itself is unrecognised, so the error is about the whole
		// '#' form rather than any one character within it.
		return yieldErrorOr(UnexpectedCharacterError{Pos: pos, R: '#'}, nil)
	}
}

// boolSpellings are the accepted spellings of a boolean literal after its '#',
// ordered longest first so that [boolPrefixLen] finds the longest valid prefix
// rather than stopping at a shorter one which also matches.
var boolSpellings = []string{"false", "true", "t", "f"}

// boolPrefixLen returns the length of the longest accepted spelling which s
// begins with, or zero if it begins with none of them. Anything beyond that
// length is trailing junk, as in "#tx".
func boolPrefixLen(s string) int {
	for _, spelling := range boolSpellings {
		if strings.HasPrefix(s, spelling) {
			return len(spelling)
		}
	}
	return 0
}

// tokenizeBool scans a boolean literal. The '#' and the leading 't' or 'f' have
// already been consumed; pos is the position of the '#', spellingPos that of
// the character after it, and first that character.
//
// The whole atom run is scanned so that a boolean must be delimited: "#tx" is
// rejected rather than read as "#t" followed by the symbol "x".
func tokenizeBool(pos, spellingPos Pos, first rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var spelling bytes.Buffer
		spelling.WriteRune(first)

		err := t.copyIf(&spelling, isAtomRune)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return yieldErrorOr(err, nil)
		}

		word := spelling.String()
		if n := boolPrefixLen(word); n != len(word) {
			// Point at where the trailing junk starts. Every accepted spelling is
			// ASCII, so the prefix length is also its column width.
			junk, _ := utf8.DecodeRuneInString(word[n:])
			return yieldErrorOr(
				UnexpectedCharacterError{
					Pos: Pos{Line: spellingPos.Line, Column: spellingPos.Column + n},
					R:   junk,
				},
				nil,
			)
		}

		tok := Token{Pos: pos, Type: TokenBool, Value: append([]byte("#"), word...)}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return yieldTokenThen(tok, nil)
		}

		return yieldErrorOr(
			err,
			yieldTokenThen(tok, skipWhitespace(tokenizeSexpr)),
		)
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

// tokenizeQuote yields a reader macro token carrying the macro's literal text.
//
// The tokenizer is deliberately permissive here: a macro with no datum after it
// is still a complete token. Reporting the missing datum is the parser's job,
// since only the parser knows what counts as one.
func tokenizeQuote(pos Pos, macro string) tokenizerAction {
	return yieldTokenThen(
		Token{Pos: pos, Type: TokenQuote, Value: []byte(macro)},
		tokenizeSexpr,
	)
}

// tokenizeUnquote scans "," or ",@". The comma has already been consumed, so
// this needs a single character of lookahead to tell the two apart, and must
// put that character back when it turns out not to be '@'.
func tokenizeUnquote(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		lookaheadPos := t.pos
		r, err := t.next()
		if err != nil {
			// A trailing comma is still a complete unquote token.
			if errors.Is(err, io.EOF) {
				return tokenizeQuote(pos, ",")
			}
			return yieldErrorOr(err, nil)
		}

		if r == '@' {
			return tokenizeQuote(pos, ",@")
		}

		// Not part of the macro, so the next token gets to see it.
		err = t.backup(lookaheadPos)
		return yieldErrorOr(err, tokenizeQuote(pos, ","))
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

// tokenizeAtom scans a maximal run of atom characters and then decides what it
// is. Scanning first is what makes "123abc" a single malformed number rather
// than a number followed by a symbol, and what lets a lone "." be recognised as
// the dotted-pair marker without lookahead.
func tokenizeAtom(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
	pos := t.pos

	var atom bytes.Buffer
	err := t.copyIf(&atom, isAtomRune)
	value := atom.Bytes()

	// Only judge the lexeme once it is complete; a read failure is reported as is.
	if err == nil || errors.Is(err, io.ErrUnexpectedEOF) {
		if looksNumeric(value) && !validNumber(string(value)) {
			return yieldErrorOr(InvalidNumberError{Pos: pos, Value: string(value)}, nil)
		}
	}

	tok := Token{Pos: pos, Type: classifyAtom(value), Value: value}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return yieldTokenThen(tok, nil)
	}

	return yieldErrorOr(
		err,
		yieldTokenThen(tok, skipWhitespace(tokenizeSexpr)),
	)
}
