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
			name: "dotted pair dot is not yet a token",
			src:  `(a . b)`,
			expectedErr: UnexpectedCharacterError{
				Pos: Pos{Line: 1, Column: 4},
				R:   '.',
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
			name: "bare integer is not a symbol",
			src:  `123`,
			expectedErr: UnexpectedCharacterError{
				Pos: Pos{Line: 1, Column: 1},
				R:   '1',
			},
		},
		{
			name: "negative integer is not a symbol",
			src:  `-1`,
			expectedErr: UnexpectedCharacterError{
				Pos: Pos{Line: 1, Column: 1},
				R:   '-',
			},
		},
		{
			name: "explicitly positive integer is not a symbol",
			src:  `(+42)`,
			expectedErr: UnexpectedCharacterError{
				Pos: Pos{Line: 1, Column: 2},
				R:   '+',
			},
		},
		{
			name: "symbol may not begin with a digit",
			src:  `1abc`,
			expectedErr: UnexpectedCharacterError{
				Pos: Pos{Line: 1, Column: 1},
				R:   '1',
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
