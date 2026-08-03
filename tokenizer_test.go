// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"iter"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenizerErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name: "unexpected character at start of input",
			src:  `#t`,
			expectedErr: UnexpectedCharacterError{
				Pos: Pos{Line: 1, Column: 1},
				R:   '#',
			},
		},
		{
			name: "unexpected character on a later line",
			src: `(add
  a
  ,b)`,
			expectedErr: UnexpectedCharacterError{
				Pos: Pos{Line: 3, Column: 3},
				R:   ',',
			},
		},
		{
			name: "a symbol may not begin with a digit",
			src:  `1abc`,
			expectedErr: InvalidNumberError{
				Pos:   Pos{Line: 1, Column: 1},
				Value: "1abc",
			},
		},
		{
			name: "a number must be followed by a delimiter",
			src:  `123abc`,
			expectedErr: InvalidNumberError{
				Pos:   Pos{Line: 1, Column: 1},
				Value: "123abc",
			},
		},
		{
			name: "two decimal points",
			src:  `1.2.3`,
			expectedErr: InvalidNumberError{
				Pos:   Pos{Line: 1, Column: 1},
				Value: "1.2.3",
			},
		},
		{
			name: "exponent without digits",
			src:  `1e`,
			expectedErr: InvalidNumberError{
				Pos:   Pos{Line: 1, Column: 1},
				Value: "1e",
			},
		},
		{
			name: "exponent with a sign but no digits",
			src:  `1e+`,
			expectedErr: InvalidNumberError{
				Pos:   Pos{Line: 1, Column: 1},
				Value: "1e+",
			},
		},
		{
			name: "repeated sign",
			src:  `--1`,
			expectedErr: InvalidNumberError{
				Pos:   Pos{Line: 1, Column: 1},
				Value: "--1",
			},
		},
		{
			name: "trailing decimal point without a fraction",
			src:  `1.`,
			expectedErr: InvalidNumberError{
				Pos:   Pos{Line: 1, Column: 1},
				Value: "1.",
			},
		},
		{
			name: "invalid number reports its own position",
			src: `(add
  1
  2e)`,
			expectedErr: InvalidNumberError{
				Pos:   Pos{Line: 3, Column: 3},
				Value: "2e",
			},
		},
		{
			name: "unterminated block comment",
			src:  `#| never closed`,
			expectedErr: UnterminatedCommentError{
				Pos: Pos{Line: 1, Column: 1},
			},
		},
		{
			name: "unterminated nested block comment",
			src:  `#| outer #| inner |#`,
			expectedErr: UnterminatedCommentError{
				Pos: Pos{Line: 1, Column: 1},
			},
		},
		{
			name: "unterminated block comment reports the opening delimiter",
			src: `(a)
  #| dangling`,
			expectedErr: UnterminatedCommentError{
				Pos: Pos{Line: 2, Column: 3},
			},
		},
		{
			name: "block comment closed one level short",
			src:  `#|#||#`,
			expectedErr: UnterminatedCommentError{
				Pos: Pos{Line: 1, Column: 1},
			},
		},
		{
			name: "unterminated string literal",
			src:  `"hello`,
			expectedErr: UnterminatedStringError{
				Pos: Pos{Line: 1, Column: 1},
			},
		},
		{
			name: "unterminated string literal reports the opening quote",
			src: `(a)
  "dangling`,
			expectedErr: UnterminatedStringError{
				Pos: Pos{Line: 2, Column: 3},
			},
		},
		{
			name: "string ending on a trailing backslash",
			src:  `"abc\`,
			expectedErr: UnterminatedStringError{
				Pos: Pos{Line: 1, Column: 1},
			},
		},
		{
			name: "string ending mid unicode escape",
			src:  `"\u00`,
			expectedErr: UnterminatedStringError{
				Pos: Pos{Line: 1, Column: 1},
			},
		},
		{
			name: "unrecognized escape sequence",
			src:  `"\q"`,
			expectedErr: InvalidEscapeError{
				Pos: Pos{Line: 1, Column: 2},
				R:   'q',
			},
		},
		{
			name: "unicode escape with non-hex digits",
			src:  `"\uZZZZ"`,
			expectedErr: InvalidEscapeError{
				Pos: Pos{Line: 1, Column: 2},
				R:   'u',
			},
		},
		{
			name: "unicode escape with too few hex digits",
			src:  `"\u00e"`,
			expectedErr: InvalidEscapeError{
				Pos: Pos{Line: 1, Column: 2},
				R:   'u',
			},
		},
		{
			name: "escape error reports the backslash position",
			src:  `"ab\q"`,
			expectedErr: InvalidEscapeError{
				Pos: Pos{Line: 1, Column: 4},
				R:   'q',
			},
		},
		{
			name: "unknown hash dispatch",
			src:  `#x`,
			expectedErr: UnexpectedCharacterError{
				Pos: Pos{Line: 1, Column: 1},
				R:   '#',
			},
		},
		{
			name: "trailing hash at end of input",
			src:  `#`,
			expectedErr: UnexpectedCharacterError{
				Pos: Pos{Line: 1, Column: 1},
				R:   '#',
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			collect := func(seq iter.Seq2[Token, error]) ([]Token, error) {
				var tokens []Token
				for item, err := range seq {
					if err != nil {
						return tokens, err
					}
					tokens = append(tokens, item)
				}
				return tokens, nil
			}

			_, err := collect(Tokenize(strings.NewReader(tc.src)))

			require.Equal(t, tc.expectedErr, err)
		})
	}
}

func TestTokenizer(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      string
		expected []Token
	}{
		{
			name:     "empty input",
			src:      ``,
			expected: []Token{},
		},
		{
			name:     "input of only whitespace",
			src:      "  \n\t\n  ",
			expected: []Token{},
		},
		{
			name: "a simple call form",
			src:  `(add a b)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("add")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 1, Column: 8}, Type: TokenSymbol, Value: []byte("b")},
				{Pos: Pos{Line: 1, Column: 9}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "the empty list",
			src:  `()`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "nested lists",
			src:  `(a (b c) d)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenSymbol, Value: []byte("b")},
				{Pos: Pos{Line: 1, Column: 7}, Type: TokenSymbol, Value: []byte("c")},
				{Pos: Pos{Line: 1, Column: 8}, Type: TokenRParen, Value: []byte(")")},
				{Pos: Pos{Line: 1, Column: 10}, Type: TokenSymbol, Value: []byte("d")},
				{Pos: Pos{Line: 1, Column: 11}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a bare symbol terminated by end of input",
			src:  `add`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("add")},
			},
		},
		{
			name: "symbols made of punctuation",
			src:  `(+ - ->list <= x2)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("+")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenSymbol, Value: []byte("-")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenSymbol, Value: []byte("->list")},
				{Pos: Pos{Line: 1, Column: 13}, Type: TokenSymbol, Value: []byte("<=")},
				{Pos: Pos{Line: 1, Column: 16}, Type: TokenSymbol, Value: []byte("x2")},
				{Pos: Pos{Line: 1, Column: 18}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "every symbol punctuation character",
			src:  `(+ - * / < > = ! ? : $ % _ & ~ ^ @)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("+")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenSymbol, Value: []byte("-")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenSymbol, Value: []byte("*")},
				{Pos: Pos{Line: 1, Column: 8}, Type: TokenSymbol, Value: []byte("/")},
				{Pos: Pos{Line: 1, Column: 10}, Type: TokenSymbol, Value: []byte("<")},
				{Pos: Pos{Line: 1, Column: 12}, Type: TokenSymbol, Value: []byte(">")},
				{Pos: Pos{Line: 1, Column: 14}, Type: TokenSymbol, Value: []byte("=")},
				{Pos: Pos{Line: 1, Column: 16}, Type: TokenSymbol, Value: []byte("!")},
				{Pos: Pos{Line: 1, Column: 18}, Type: TokenSymbol, Value: []byte("?")},
				{Pos: Pos{Line: 1, Column: 20}, Type: TokenSymbol, Value: []byte(":")},
				{Pos: Pos{Line: 1, Column: 22}, Type: TokenSymbol, Value: []byte("$")},
				{Pos: Pos{Line: 1, Column: 24}, Type: TokenSymbol, Value: []byte("%")},
				{Pos: Pos{Line: 1, Column: 26}, Type: TokenSymbol, Value: []byte("_")},
				{Pos: Pos{Line: 1, Column: 28}, Type: TokenSymbol, Value: []byte("&")},
				{Pos: Pos{Line: 1, Column: 30}, Type: TokenSymbol, Value: []byte("~")},
				{Pos: Pos{Line: 1, Column: 32}, Type: TokenSymbol, Value: []byte("^")},
				{Pos: Pos{Line: 1, Column: 34}, Type: TokenSymbol, Value: []byte("@")},
				{Pos: Pos{Line: 1, Column: 35}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a form spanning several lines",
			src: `(add
  a
  b)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("add")},
				{Pos: Pos{Line: 2, Column: 3}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 3, Column: 3}, Type: TokenSymbol, Value: []byte("b")},
				{Pos: Pos{Line: 3, Column: 4}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "leading and trailing whitespace",
			src:  "\n  (a)\n\n",
			expected: []Token{
				{Pos: Pos{Line: 2, Column: 3}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 2, Column: 4}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 2, Column: 5}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "several top level forms",
			src:  `(a) (b)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 1, Column: 3}, Type: TokenRParen, Value: []byte(")")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenSymbol, Value: []byte("b")},
				{Pos: Pos{Line: 1, Column: 7}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			// Column is a byte offset, so the two byte 'λ' advances it by two.
			name: "symbols containing non-ASCII letters",
			src:  `(λ x)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("λ")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenSymbol, Value: []byte("x")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a symbol with a digit after a letter",
			src:  `x1`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("x1")},
			},
		},
		{
			name: "a line comment running to end of input",
			src:  `; hello`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("; hello")},
			},
		},
		{
			name: "a line comment terminated by a newline",
			src: `; first
(a)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("; first")},
				{Pos: Pos{Line: 2, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 2, Column: 2}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 2, Column: 3}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a line comment trailing a form",
			src:  `(a) ; done`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 1, Column: 3}, Type: TokenRParen, Value: []byte(")")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenComment, Value: []byte("; done")},
			},
		},
		{
			name: "an empty line comment",
			src:  `;`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte(";")},
			},
		},
		{
			name: "repeated semicolons belong to the comment",
			src:  `;;; heading`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte(";;; heading")},
			},
		},
		{
			name: "a block comment",
			src:  `#| c |#`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("#| c |#")},
			},
		},
		{
			name: "an empty block comment",
			src:  `#||#`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("#||#")},
			},
		},
		{
			name: "a nested block comment is one token",
			src:  `#| a #| b |# c |#`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("#| a #| b |# c |#")},
			},
		},
		{
			name: "a doubly nested block comment is one token",
			src:  `#| a #| b #| c |# d |# e |#`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("#| a #| b #| c |# d |# e |#")},
			},
		},
		{
			name: "adjacent nested block comments",
			src:  `#|#||#|#`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("#|#||#|#")},
			},
		},
		{
			name: "a block comment between forms",
			src:  `(a #| c |# b)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenComment, Value: []byte("#| c |#")},
				{Pos: Pos{Line: 1, Column: 12}, Type: TokenSymbol, Value: []byte("b")},
				{Pos: Pos{Line: 1, Column: 13}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "positions survive a multi-line block comment",
			src: `#| line one
   line two |#
(a)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("#| line one\n   line two |#")},
				{Pos: Pos{Line: 3, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 3, Column: 2}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 3, Column: 3}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "positions survive a multi-line nested block comment",
			src: `#| a
#| b
|# c |#
(x)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("#| a\n#| b\n|# c |#")},
				{Pos: Pos{Line: 4, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 4, Column: 2}, Type: TokenSymbol, Value: []byte("x")},
				{Pos: Pos{Line: 4, Column: 3}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "both comment styles together",
			src: `; one
#| two |# (a)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("; one")},
				{Pos: Pos{Line: 2, Column: 1}, Type: TokenComment, Value: []byte("#| two |#")},
				{Pos: Pos{Line: 2, Column: 11}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 2, Column: 12}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 2, Column: 13}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a line comment inside a block comment is not a delimiter",
			src:  `#| ; not a line comment |#`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("#| ; not a line comment |#")},
			},
		},
		{
			name: "a block comment opener inside a line comment is inert",
			src: `; #| not nested
(a)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("; #| not nested")},
				{Pos: Pos{Line: 2, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 2, Column: 2}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 2, Column: 3}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a string literal",
			src:  `"hello"`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte("hello")},
			},
		},
		{
			name: "an empty string literal",
			src:  `""`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte("")},
			},
		},
		{
			name: "every recognized escape is preserved raw",
			src:  `"\" \\ \n \r \t \b \f \u00e9"`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`\" \\ \n \r \t \b \f \u00e9`)},
			},
		},
		{
			name: "an escaped quote does not close the string",
			src:  `"say \"hi\"" x`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`say \"hi\"`)},
				{Pos: Pos{Line: 1, Column: 14}, Type: TokenSymbol, Value: []byte("x")},
			},
		},
		{
			name: "a trailing escaped backslash does not swallow the quote",
			src:  `"a\\" x`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`a\\`)},
				{Pos: Pos{Line: 1, Column: 7}, Type: TokenSymbol, Value: []byte("x")},
			},
		},
		{
			name: "a unicode escape with uppercase hex",
			src:  `"\uABCD"`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`\uABCD`)},
			},
		},
		{
			name: "a string inside a form",
			src:  `(f "hi")`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("f")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenString, Value: []byte("hi")},
				{Pos: Pos{Line: 1, Column: 8}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a literal newline inside a string advances the line",
			src: `"line one
line two"
(a)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte("line one\nline two")},
				{Pos: Pos{Line: 3, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 3, Column: 2}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 3, Column: 3}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "delimiters inside a string are inert",
			src:  `"(a) ; #| |#"`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte("(a) ; #| |#")},
			},
		},
		{
			name: "a string containing non-ASCII text",
			src:  `"héllo"`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte("héllo")},
			},
		},
		{
			name: "integers",
			src:  `(0 42 -1 +42)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenNumber, Value: []byte("0")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenNumber, Value: []byte("42")},
				{Pos: Pos{Line: 1, Column: 7}, Type: TokenNumber, Value: []byte("-1")},
				{Pos: Pos{Line: 1, Column: 10}, Type: TokenNumber, Value: []byte("+42")},
				{Pos: Pos{Line: 1, Column: 13}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "floating point numbers",
			src:  `(1.5 -0.5 0.0)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenNumber, Value: []byte("1.5")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenNumber, Value: []byte("-0.5")},
				{Pos: Pos{Line: 1, Column: 11}, Type: TokenNumber, Value: []byte("0.0")},
				{Pos: Pos{Line: 1, Column: 14}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "numbers with exponents",
			src:  `(1e10 1.5e-3 1E+7 2e0)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenNumber, Value: []byte("1e10")},
				{Pos: Pos{Line: 1, Column: 7}, Type: TokenNumber, Value: []byte("1.5e-3")},
				{Pos: Pos{Line: 1, Column: 14}, Type: TokenNumber, Value: []byte("1E+7")},
				{Pos: Pos{Line: 1, Column: 19}, Type: TokenNumber, Value: []byte("2e0")},
				{Pos: Pos{Line: 1, Column: 22}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a leading decimal point begins a number",
			src:  `(.5 -.5)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenNumber, Value: []byte(".5")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenNumber, Value: []byte("-.5")},
				{Pos: Pos{Line: 1, Column: 8}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a dot between datums is the dotted pair marker",
			src:  `(a . b)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenDot, Value: []byte(".")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenSymbol, Value: []byte("b")},
				{Pos: Pos{Line: 1, Column: 7}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a dot abutting a parenthesis is the dotted pair marker",
			src:  `(a .(b))`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenDot, Value: []byte(".")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenSymbol, Value: []byte("b")},
				{Pos: Pos{Line: 1, Column: 7}, Type: TokenRParen, Value: []byte(")")},
				{Pos: Pos{Line: 1, Column: 8}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a dot at end of input is the dotted pair marker",
			src:  `.`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenDot, Value: []byte(".")},
			},
		},
		{
			name: "a dot followed by a digit is part of a number",
			src:  `(a .5)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenNumber, Value: []byte(".5")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a dot elsewhere is part of a symbol",
			src:  `(a.b ... .foo x.)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("a.b")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenSymbol, Value: []byte("...")},
				{Pos: Pos{Line: 1, Column: 10}, Type: TokenSymbol, Value: []byte(".foo")},
				{Pos: Pos{Line: 1, Column: 15}, Type: TokenSymbol, Value: []byte("x.")},
				{Pos: Pos{Line: 1, Column: 17}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "signs and sign-led symbols are not numbers",
			src:  `(- + -e10 ->x2 e10)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("-")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenSymbol, Value: []byte("+")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenSymbol, Value: []byte("-e10")},
				{Pos: Pos{Line: 1, Column: 11}, Type: TokenSymbol, Value: []byte("->x2")},
				{Pos: Pos{Line: 1, Column: 16}, Type: TokenSymbol, Value: []byte("e10")},
				{Pos: Pos{Line: 1, Column: 19}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a dotted pair of numbers",
			src:  `(1 . 2)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenNumber, Value: []byte("1")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenDot, Value: []byte(".")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenNumber, Value: []byte("2")},
				{Pos: Pos{Line: 1, Column: 7}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a number terminated by end of input",
			src:  `42`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("42")},
			},
		},
		{
			name: "a non-ASCII digit stays a symbol",
			src:  `٣`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("٣")},
			},
		},
		{
			name: "parentheses immediately adjacent to symbols",
			src:  `((a)b)`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenLParen, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 3}, Type: TokenSymbol, Value: []byte("a")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenRParen, Value: []byte(")")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenSymbol, Value: []byte("b")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenRParen, Value: []byte(")")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			collect := func(seq iter.Seq2[Token, error]) ([]Token, error) {
				tokens := make([]Token, 0, len(tc.expected))
				for item, err := range seq {
					if err != nil {
						return tokens, err
					}
					t.Log(item)
					tokens = append(tokens, item)
				}
				return tokens, nil
			}

			tokens, err := collect(Tokenize(strings.NewReader(tc.src)))

			require.NoError(t, err)
			require.Equal(t, tc.expected, tokens)
		})
	}
}

func TestTokenizerStopsEarly(t *testing.T) {
	t.Parallel()

	t.Run("will stop yielding once the consumer breaks", func(t *testing.T) {
		t.Parallel()

		var tokens []Token
		for tok, err := range Tokenize(strings.NewReader(`(add a b)`)) {
			require.NoError(t, err)
			tokens = append(tokens, tok)
			if len(tokens) == 2 {
				break
			}
		}

		require.Len(t, tokens, 2)
		require.Equal(t, TokenLParen, tokens[0].Type)
		require.Equal(t, TokenSymbol, tokens[1].Type)
	})
}

func TestTokenType(t *testing.T) {
	t.Parallel()

	t.Run("will render each token type as a name", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "LParen", TokenLParen.String())
		require.Equal(t, "RParen", TokenRParen.String())
		require.Equal(t, "Symbol", TokenSymbol.String())
	})

	t.Run("will panic on an unknown token type", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			_ = TokenType(-1).String()
		})
	})
}

func TestTokenString(t *testing.T) {
	t.Parallel()

	t.Run("will render the type and value", func(t *testing.T) {
		t.Parallel()

		tok := Token{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("add")}

		require.Equal(t, "Symbol(add)", tok.String())
	})
}

func TestUnexpectedCharacterError(t *testing.T) {
	t.Parallel()

	t.Run("will report the character and its position", func(t *testing.T) {
		t.Parallel()

		err := UnexpectedCharacterError{Pos: Pos{Line: 3, Column: 7}, R: '#'}

		require.Equal(t, "unexpected character '#' at line 3, column 7", err.Error())
	})
}

func TestUnterminatedCommentError(t *testing.T) {
	t.Parallel()

	t.Run("will report the position of the opening delimiter", func(t *testing.T) {
		t.Parallel()

		err := UnterminatedCommentError{Pos: Pos{Line: 2, Column: 5}}

		require.Equal(t, "unterminated block comment at line 2, column 5", err.Error())
	})
}

func TestInvalidNumberError(t *testing.T) {
	t.Parallel()

	t.Run("will report the lexeme and its position", func(t *testing.T) {
		t.Parallel()

		err := InvalidNumberError{Pos: Pos{Line: 3, Column: 8}, Value: "1.2.3"}

		require.Equal(t, `invalid number literal "1.2.3" at line 3, column 8`, err.Error())
	})
}

func TestValidNumber(t *testing.T) {
	t.Parallel()

	valid := []string{
		"0", "42", "-1", "+42", "007",
		"1.5", "-0.5", "0.0", ".5", "-.5", "+.5",
		"1e10", "1E10", "1e+10", "1e-10", "1.5e-3", "2e0", ".5e2",
	}
	for _, s := range valid {
		t.Run("accepts "+s, func(t *testing.T) {
			t.Parallel()
			require.True(t, validNumber(s))
		})
	}

	invalid := []string{
		"", "-", "+", ".", "..", "--1", "++1", "-+1",
		"1.", "1.2.3", "1e", "1e+", "1e-", "1ee1", "1e1.5",
		"123abc", "1a", "abc", "e10", "0x10", "1_000",
	}
	for _, s := range invalid {
		t.Run("rejects "+s, func(t *testing.T) {
			t.Parallel()
			require.False(t, validNumber(s))
		})
	}
}

func TestLooksNumeric(t *testing.T) {
	t.Parallel()

	// looksNumeric decides whether a malformed lexeme is reported as a bad
	// number or accepted as a symbol, so both answers matter.
	numeric := []string{"0", "42", "-1", "+42", ".5", "-.5", "--1", "1.2.3", "123abc", "1e"}
	for _, s := range numeric {
		t.Run("treats "+s+" as a number", func(t *testing.T) {
			t.Parallel()
			require.True(t, looksNumeric([]byte(s)))
		})
	}

	symbolic := []string{"-", "+", "...", "->x2", "x2", "a.b", "-e10", "e10", ".foo", "abc"}
	for _, s := range symbolic {
		t.Run("treats "+s+" as a symbol", func(t *testing.T) {
			t.Parallel()
			require.False(t, looksNumeric([]byte(s)))
		})
	}
}

func TestUnterminatedStringError(t *testing.T) {
	t.Parallel()

	t.Run("will report the position of the opening quote", func(t *testing.T) {
		t.Parallel()

		err := UnterminatedStringError{Pos: Pos{Line: 4, Column: 9}}

		require.Equal(t, "unterminated string literal at line 4, column 9", err.Error())
	})
}

func TestInvalidEscapeError(t *testing.T) {
	t.Parallel()

	t.Run("will report the escape and its position", func(t *testing.T) {
		t.Parallel()

		err := InvalidEscapeError{Pos: Pos{Line: 2, Column: 6}, R: 'q'}

		require.Equal(t, `invalid escape sequence '\q' at line 2, column 6`, err.Error())
	})
}

func TestTokenizerCopyNested(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		src         string
		expected    string
		expectedPos Pos
		expectedErr error
	}{
		{
			name:        "stops at the matching close",
			src:         ` a |# rest`,
			expected:    " a |#",
			expectedPos: Pos{Line: 1, Column: 6},
		},
		{
			name:        "counts a nested pair rather than closing early",
			src:         ` #| b |# c |# rest`,
			expected:    " #| b |# c |#",
			expectedPos: Pos{Line: 1, Column: 14},
		},
		{
			name:        "treats adjacent opens as separate levels",
			src:         `#||#|# rest`,
			expected:    "#||#|#",
			expectedPos: Pos{Line: 1, Column: 7},
		},
		{
			name:        "tracks lines across the construct",
			src:         "a\nb |# rest",
			expected:    "a\nb |#",
			expectedPos: Pos{Line: 2, Column: 5},
		},
		{
			name:        "reports unexpected EOF when never closed",
			src:         ` a `,
			expected:    " a ",
			expectedPos: Pos{Line: 1, Column: 4},
			expectedErr: io.ErrUnexpectedEOF,
		},
		{
			name:        "reports unexpected EOF when closed one level short",
			src:         ` #| b |# `,
			expected:    " #| b |# ",
			expectedPos: Pos{Line: 1, Column: 10},
			expectedErr: io.ErrUnexpectedEOF,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tk := &tokenizer{
				pos: Pos{Line: 1, Column: 1},
				buf: bufio.NewReader(strings.NewReader(tc.src)),
			}

			var dst bytes.Buffer
			err := tk.copyNested(&dst, blockCommentOpen, blockCommentClose)

			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expected, dst.String())
			require.Equal(t, tc.expectedPos, tk.pos)
		})
	}
}

func TestTokenizerCopyNestedUnevenDelimiters(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      string
		open     []rune
		closing  []rune
		expected string
	}{
		{
			name:     "closing longer than open",
			open:     []rune{'<'},
			closing:  []rune{'-', '>'},
			src:      `a <b-> c-> rest`,
			expected: "a <b-> c->",
		},
		{
			name:     "open longer than closing",
			open:     []rune{'<', '-'},
			closing:  []rune{'>'},
			src:      `a <-b> c> rest`,
			expected: "a <-b> c>",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tk := &tokenizer{
				pos: Pos{Line: 1, Column: 1},
				buf: bufio.NewReader(strings.NewReader(tc.src)),
			}

			var dst bytes.Buffer
			err := tk.copyNested(&dst, tc.open, tc.closing)

			require.NoError(t, err)
			require.Equal(t, tc.expected, dst.String())
		})
	}
}

// errReader fails every read with a non-EOF error.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestTokenizerReaderErrors(t *testing.T) {
	t.Parallel()

	t.Run("will surface a read failure to the consumer", func(t *testing.T) {
		t.Parallel()

		readErr := errors.New("boom")

		var got error
		for _, err := range Tokenize(errReader{err: readErr}) {
			if err != nil {
				got = err
				break
			}
		}

		require.ErrorIs(t, got, readErr)
	})
}

func TestTokenizerCopyUntil(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		src         string
		delim       []rune
		expected    string
		expectedPos Pos
		expectedErr error
	}{
		{
			name:        "copies everything before the delimiter",
			src:         `abc*/rest`,
			delim:       []rune{'*', '/'},
			expected:    "abc",
			expectedPos: Pos{Line: 1, Column: 6},
		},
		{
			name:        "copies nothing when the delimiter is immediate",
			src:         `*/rest`,
			delim:       []rune{'*', '/'},
			expected:    "",
			expectedPos: Pos{Line: 1, Column: 3},
		},
		{
			name:        "flushes what it consumed when the delimiter is absent",
			src:         `abc`,
			delim:       []rune{'*', '/'},
			expected:    "abc",
			expectedPos: Pos{Line: 1, Column: 4},
			expectedErr: io.ErrUnexpectedEOF,
		},
		{
			name:        "tracks the delimiter across a newline",
			src:         "a\nb*/rest",
			delim:       []rune{'*', '/'},
			expected:    "a\nb",
			expectedPos: Pos{Line: 2, Column: 4},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tk := &tokenizer{
				pos: Pos{Line: 1, Column: 1},
				buf: bufio.NewReader(strings.NewReader(tc.src)),
			}

			var dst bytes.Buffer
			err := tk.copyUntil(&dst, tc.delim)

			if tc.expectedErr != nil {
				require.ErrorIs(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.expected, dst.String())
			// pos must still line up with the reader once copyUntil returns,
			// otherwise every token after a block comment is reported early.
			require.Equal(t, tc.expectedPos, tk.pos)
		})
	}
}
