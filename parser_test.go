// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"iter"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestParser(src string) (*parser, func()) {
	next, stop := iter.Pull2(Tokenize(strings.NewReader(src)))
	return &parser{next: next, pos: Pos{Line: 1, Column: 1}}, stop
}

func TestParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      string
		expected []Node
	}{
		{
			name:     "empty input",
			src:      ``,
			expected: nil,
		},
		{
			name:     "input of only whitespace",
			src:      "  \n\t ",
			expected: nil,
		},
		{
			name: "a symbol",
			src:  `add`,
			expected: []Node{
				Symbol{Pos: Pos{Line: 1, Column: 1}, Value: "add"},
			},
		},
		{
			name: "a symbol of punctuation",
			src:  `->list`,
			expected: []Node{
				Symbol{Pos: Pos{Line: 1, Column: 1}, Value: "->list"},
			},
		},
		{
			name: "nil is its own node",
			src:  `nil`,
			expected: []Node{
				Nil{Pos: Pos{Line: 1, Column: 1}},
			},
		},
		{
			name: "a symbol which merely contains nil",
			src:  `nil?`,
			expected: []Node{
				Symbol{Pos: Pos{Line: 1, Column: 1}, Value: "nil?"},
			},
		},
		{
			name: "a string literal",
			src:  `"hello"`,
			expected: []Node{
				String{Pos: Pos{Line: 1, Column: 1}, Value: "hello"},
			},
		},
		{
			name: "an empty string literal",
			src:  `""`,
			expected: []Node{
				String{Pos: Pos{Line: 1, Column: 1}, Value: ""},
			},
		},
		{
			name: "integers",
			src:  `0 42 -1 +42`,
			expected: []Node{
				Int{Pos: Pos{Line: 1, Column: 1}, Value: 0},
				Int{Pos: Pos{Line: 1, Column: 3}, Value: 42},
				Int{Pos: Pos{Line: 1, Column: 6}, Value: -1},
				Int{Pos: Pos{Line: 1, Column: 9}, Value: 42},
			},
		},
		{
			name: "the integer bounds",
			src:  `9223372036854775807 -9223372036854775808`,
			expected: []Node{
				Int{Pos: Pos{Line: 1, Column: 1}, Value: 9223372036854775807},
				Int{Pos: Pos{Line: 1, Column: 21}, Value: -9223372036854775808},
			},
		},
		{
			name: "floats",
			src:  `1.5 -0.5 .5`,
			expected: []Node{
				Float{Pos: Pos{Line: 1, Column: 1}, Value: 1.5},
				Float{Pos: Pos{Line: 1, Column: 5}, Value: -0.5},
				Float{Pos: Pos{Line: 1, Column: 10}, Value: 0.5},
			},
		},
		{
			name: "floats with exponents",
			src:  `1e10 1.5e-3 1E+2`,
			expected: []Node{
				Float{Pos: Pos{Line: 1, Column: 1}, Value: 1e10},
				Float{Pos: Pos{Line: 1, Column: 6}, Value: 1.5e-3},
				Float{Pos: Pos{Line: 1, Column: 13}, Value: 100},
			},
		},
		{
			name: "booleans",
			src:  `#t #true #f #false`,
			expected: []Node{
				Bool{Pos: Pos{Line: 1, Column: 1}, Value: true},
				Bool{Pos: Pos{Line: 1, Column: 4}, Value: true},
				Bool{Pos: Pos{Line: 1, Column: 10}, Value: false},
				Bool{Pos: Pos{Line: 1, Column: 13}, Value: false},
			},
		},
		{
			name: "several top level datums of mixed kinds",
			src:  `a 1 1.5 "s" #t nil`,
			expected: []Node{
				Symbol{Pos: Pos{Line: 1, Column: 1}, Value: "a"},
				Int{Pos: Pos{Line: 1, Column: 3}, Value: 1},
				Float{Pos: Pos{Line: 1, Column: 5}, Value: 1.5},
				String{Pos: Pos{Line: 1, Column: 9}, Value: "s"},
				Bool{Pos: Pos{Line: 1, Column: 13}, Value: true},
				Nil{Pos: Pos{Line: 1, Column: 16}},
			},
		},
		{
			name: "datums across several lines",
			src: `a
  1
  #f`,
			expected: []Node{
				Symbol{Pos: Pos{Line: 1, Column: 1}, Value: "a"},
				Int{Pos: Pos{Line: 2, Column: 3}, Value: 1},
				Bool{Pos: Pos{Line: 3, Column: 3}, Value: false},
			},
		},
		{
			name: "line comments are skipped",
			src: `; leading
a ; trailing
b`,
			expected: []Node{
				Symbol{Pos: Pos{Line: 2, Column: 1}, Value: "a"},
				Symbol{Pos: Pos{Line: 3, Column: 1}, Value: "b"},
			},
		},
		{
			name: "block comments are skipped",
			src:  `#| c |# a #| d |# b`,
			expected: []Node{
				Symbol{Pos: Pos{Line: 1, Column: 9}, Value: "a"},
				Symbol{Pos: Pos{Line: 1, Column: 19}, Value: "b"},
			},
		},
		{
			name:     "a file of only comments",
			src:      `; nothing but a comment`,
			expected: nil,
		},
		{
			name: "an empty list",
			src:  `()`,
			expected: []Node{
				List{Pos: Pos{Line: 1, Column: 1}},
			},
		},
		{
			name: "a list of one element",
			src:  `(a)`,
			expected: []Node{
				List{Pos: Pos{Line: 1, Column: 1}, Elements: []Node{
					Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"},
				}},
			},
		},
		{
			name: "a list of mixed atoms",
			src:  `(a 1 "s" #t nil)`,
			expected: []Node{
				List{Pos: Pos{Line: 1, Column: 1}, Elements: []Node{
					Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"},
					Int{Pos: Pos{Line: 1, Column: 4}, Value: 1},
					String{Pos: Pos{Line: 1, Column: 6}, Value: "s"},
					Bool{Pos: Pos{Line: 1, Column: 10}, Value: true},
					Nil{Pos: Pos{Line: 1, Column: 13}},
				}},
			},
		},
		{
			name: "nested lists",
			src:  `(a (b c) d)`,
			expected: []Node{
				List{Pos: Pos{Line: 1, Column: 1}, Elements: []Node{
					Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"},
					List{Pos: Pos{Line: 1, Column: 4}, Elements: []Node{
						Symbol{Pos: Pos{Line: 1, Column: 5}, Value: "b"},
						Symbol{Pos: Pos{Line: 1, Column: 7}, Value: "c"},
					}},
					Symbol{Pos: Pos{Line: 1, Column: 10}, Value: "d"},
				}},
			},
		},
		{
			name: "lists nested only in other lists",
			src:  `((()))`,
			expected: []Node{
				List{Pos: Pos{Line: 1, Column: 1}, Elements: []Node{
					List{Pos: Pos{Line: 1, Column: 2}, Elements: []Node{
						List{Pos: Pos{Line: 1, Column: 3}},
					}},
				}},
			},
		},
		{
			name: "an empty list is not nil",
			src:  `(() nil)`,
			expected: []Node{
				List{Pos: Pos{Line: 1, Column: 1}, Elements: []Node{
					List{Pos: Pos{Line: 1, Column: 2}},
					Nil{Pos: Pos{Line: 1, Column: 5}},
				}},
			},
		},
		{
			name: "several top level lists",
			src:  `(a) (b)`,
			expected: []Node{
				List{Pos: Pos{Line: 1, Column: 1}, Elements: []Node{
					Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"},
				}},
				List{Pos: Pos{Line: 1, Column: 5}, Elements: []Node{
					Symbol{Pos: Pos{Line: 1, Column: 6}, Value: "b"},
				}},
			},
		},
		{
			name: "a list spanning several lines",
			src: `(add
  1
  2)`,
			expected: []Node{
				List{Pos: Pos{Line: 1, Column: 1}, Elements: []Node{
					Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "add"},
					Int{Pos: Pos{Line: 2, Column: 3}, Value: 1},
					Int{Pos: Pos{Line: 3, Column: 3}, Value: 2},
				}},
			},
		},
		{
			name: "comments inside a list are kept on the list",
			src: `(a ; note
 b)`,
			expected: []Node{
				List{
					Pos: Pos{Line: 1, Column: 1},
					Elements: []Node{
						Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"},
						Symbol{Pos: Pos{Line: 2, Column: 2}, Value: "b"},
					},
					Comments: []*Comment{
						{Pos: Pos{Line: 1, Column: 4}, Text: "; note"},
					},
				},
			},
		},
		{
			name: "a dotted pair",
			src:  `(a . b)`,
			expected: []Node{
				List{
					Pos:      Pos{Line: 1, Column: 1},
					Elements: []Node{Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"}},
					Tail:     Symbol{Pos: Pos{Line: 1, Column: 6}, Value: "b"},
				},
			},
		},
		{
			name: "an improper list of several elements",
			src:  `(a b . c)`,
			expected: []Node{
				List{
					Pos: Pos{Line: 1, Column: 1},
					Elements: []Node{
						Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"},
						Symbol{Pos: Pos{Line: 1, Column: 4}, Value: "b"},
					},
					Tail: Symbol{Pos: Pos{Line: 1, Column: 8}, Value: "c"},
				},
			},
		},
		{
			name: "a dotted pair of atoms of different kinds",
			src:  `(1 . "s")`,
			expected: []Node{
				List{
					Pos:      Pos{Line: 1, Column: 1},
					Elements: []Node{Int{Pos: Pos{Line: 1, Column: 2}, Value: 1}},
					Tail:     String{Pos: Pos{Line: 1, Column: 6}, Value: "s"},
				},
			},
		},
		{
			name: "a list as the tail",
			src:  `(a . (b c))`,
			expected: []Node{
				List{
					Pos:      Pos{Line: 1, Column: 1},
					Elements: []Node{Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"}},
					Tail: List{Pos: Pos{Line: 1, Column: 6}, Elements: []Node{
						Symbol{Pos: Pos{Line: 1, Column: 7}, Value: "b"},
						Symbol{Pos: Pos{Line: 1, Column: 9}, Value: "c"},
					}},
				},
			},
		},
		{
			name: "nil as the tail",
			src:  `(a . nil)`,
			expected: []Node{
				List{
					Pos:      Pos{Line: 1, Column: 1},
					Elements: []Node{Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"}},
					Tail:     Nil{Pos: Pos{Line: 1, Column: 6}},
				},
			},
		},
		{
			name: "a dotted pair nested in another",
			src:  `((a . b) . c)`,
			expected: []Node{
				List{
					Pos: Pos{Line: 1, Column: 1},
					Elements: []Node{
						List{
							Pos:      Pos{Line: 1, Column: 2},
							Elements: []Node{Symbol{Pos: Pos{Line: 1, Column: 3}, Value: "a"}},
							Tail:     Symbol{Pos: Pos{Line: 1, Column: 7}, Value: "b"},
						},
					},
					Tail: Symbol{Pos: Pos{Line: 1, Column: 12}, Value: "c"},
				},
			},
		},
		{
			name: "a proper list leaves the tail unset",
			src:  `(a b)`,
			expected: []Node{
				List{Pos: Pos{Line: 1, Column: 1}, Elements: []Node{
					Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"},
					Symbol{Pos: Pos{Line: 1, Column: 4}, Value: "b"},
				}, Tail: nil},
			},
		},
		{
			name: "a dotted pair spanning lines",
			src: `(a
  .
  b)`,
			expected: []Node{
				List{
					Pos:      Pos{Line: 1, Column: 1},
					Elements: []Node{Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"}},
					Tail:     Symbol{Pos: Pos{Line: 3, Column: 3}, Value: "b"},
				},
			},
		},
		{
			name: "a quoted symbol",
			src:  `'x`,
			expected: []Node{
				Quote{
					Pos:   Pos{Line: 1, Column: 1},
					Kind:  QuoteKindQuote,
					Datum: Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "x"},
				},
			},
		},
		{
			name: "a quasiquoted symbol",
			src:  "`x",
			expected: []Node{
				Quote{
					Pos:   Pos{Line: 1, Column: 1},
					Kind:  QuoteKindQuasiquote,
					Datum: Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "x"},
				},
			},
		},
		{
			name: "an unquoted symbol",
			src:  `,x`,
			expected: []Node{
				Quote{
					Pos:   Pos{Line: 1, Column: 1},
					Kind:  QuoteKindUnquote,
					Datum: Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "x"},
				},
			},
		},
		{
			name: "an unquote splicing form",
			src:  `,@x`,
			expected: []Node{
				Quote{
					Pos:   Pos{Line: 1, Column: 1},
					Kind:  QuoteKindUnquoteSplicing,
					Datum: Symbol{Pos: Pos{Line: 1, Column: 3}, Value: "x"},
				},
			},
		},
		{
			name: "stacked quotes",
			src:  `''x`,
			expected: []Node{
				Quote{
					Pos:  Pos{Line: 1, Column: 1},
					Kind: QuoteKindQuote,
					Datum: Quote{
						Pos:   Pos{Line: 1, Column: 2},
						Kind:  QuoteKindQuote,
						Datum: Symbol{Pos: Pos{Line: 1, Column: 3}, Value: "x"},
					},
				},
			},
		},
		{
			name: "mixed stacked macros",
			src:  "`,@x",
			expected: []Node{
				Quote{
					Pos:  Pos{Line: 1, Column: 1},
					Kind: QuoteKindQuasiquote,
					Datum: Quote{
						Pos:   Pos{Line: 1, Column: 2},
						Kind:  QuoteKindUnquoteSplicing,
						Datum: Symbol{Pos: Pos{Line: 1, Column: 4}, Value: "x"},
					},
				},
			},
		},
		{
			name: "a quoted list",
			src:  `'(1 2)`,
			expected: []Node{
				Quote{
					Pos:  Pos{Line: 1, Column: 1},
					Kind: QuoteKindQuote,
					Datum: List{Pos: Pos{Line: 1, Column: 2}, Elements: []Node{
						Int{Pos: Pos{Line: 1, Column: 3}, Value: 1},
						Int{Pos: Pos{Line: 1, Column: 5}, Value: 2},
					}},
				},
			},
		},
		{
			name: "a quasiquoted template with both unquote forms",
			src:  "`(a ,b ,@c)",
			expected: []Node{
				Quote{
					Pos:  Pos{Line: 1, Column: 1},
					Kind: QuoteKindQuasiquote,
					Datum: List{Pos: Pos{Line: 1, Column: 2}, Elements: []Node{
						Symbol{Pos: Pos{Line: 1, Column: 3}, Value: "a"},
						Quote{
							Pos:   Pos{Line: 1, Column: 5},
							Kind:  QuoteKindUnquote,
							Datum: Symbol{Pos: Pos{Line: 1, Column: 6}, Value: "b"},
						},
						Quote{
							Pos:   Pos{Line: 1, Column: 8},
							Kind:  QuoteKindUnquoteSplicing,
							Datum: Symbol{Pos: Pos{Line: 1, Column: 10}, Value: "c"},
						},
					}},
				},
			},
		},
		{
			name: "a quoted datum as a dotted pair tail",
			src:  `(a . 'b)`,
			expected: []Node{
				List{
					Pos:      Pos{Line: 1, Column: 1},
					Elements: []Node{Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "a"}},
					Tail: Quote{
						Pos:   Pos{Line: 1, Column: 6},
						Kind:  QuoteKindQuote,
						Datum: Symbol{Pos: Pos{Line: 1, Column: 7}, Value: "b"},
					},
				},
			},
		},
		{
			name: "a quoted empty list",
			src:  `'()`,
			expected: []Node{
				Quote{
					Pos:   Pos{Line: 1, Column: 1},
					Kind:  QuoteKindQuote,
					Datum: List{Pos: Pos{Line: 1, Column: 2}},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := Parse(strings.NewReader(tc.src))

			require.NoError(t, err)
			require.NotNil(t, file)
			require.Equal(t, tc.expected, file.Nodes)
		})
	}
}

func TestParseStringEscapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      string
		expected string
	}{
		{name: "escaped quote", src: `"a\"b"`, expected: "a\"b"},
		{name: "escaped backslash", src: `"a\\b"`, expected: "a\\b"},
		{name: "newline", src: `"a\nb"`, expected: "a\nb"},
		{name: "carriage return", src: `"a\rb"`, expected: "a\rb"},
		{name: "tab", src: `"a\tb"`, expected: "a\tb"},
		{name: "backspace", src: `"a\bb"`, expected: "a\bb"},
		{name: "form feed", src: `"a\fb"`, expected: "a\fb"},
		{name: "several escapes in one literal", src: `"\t\n\\"`, expected: "\t\n\\"},
		{name: "a literal newline is kept", src: "\"a\nb\"", expected: "a\nb"},
		{name: "non-ASCII text passes through", src: `"héllo"`, expected: "héllo"},
		{name: "unicode escape", src: `"\u00e9"`, expected: "é"},
		{name: "unicode escape with uppercase hex", src: `"\uABCD"`, expected: "ꯍ"},
		{name: "unicode escape of an ASCII character", src: `"\u0041"`, expected: "A"},
		{name: "unicode escape beside other escapes", src: `"\n\u00e9\t"`, expected: "\né\t"},
		{name: "the largest unicode escape", src: `"\uFFFF"`, expected: "\uFFFF"},
		{name: "the smallest unicode escape", src: `"\u0000"`, expected: "\u0000"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := Parse(strings.NewReader(tc.src))

			require.NoError(t, err)
			require.Len(t, file.Nodes, 1)
			require.Equal(t, String{Pos: Pos{Line: 1, Column: 1}, Value: tc.expected}, file.Nodes[0])
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name: "integer above the int64 range",
			src:  `9223372036854775808`,
			expectedErr: NumberRangeError{
				Pos:   Pos{Line: 1, Column: 1},
				Value: "9223372036854775808",
			},
		},
		{
			name: "integer below the int64 range",
			src:  `-9223372036854775809`,
			expectedErr: NumberRangeError{
				Pos:   Pos{Line: 1, Column: 1},
				Value: "-9223372036854775809",
			},
		},
		{
			name: "float above the float64 range",
			src:  `1e400`,
			expectedErr: NumberRangeError{
				Pos:   Pos{Line: 1, Column: 1},
				Value: "1e400",
			},
		},
		{
			name: "out of range number reports its own position",
			src: `1
  99999999999999999999`,
			expectedErr: NumberRangeError{
				Pos:   Pos{Line: 2, Column: 3},
				Value: "99999999999999999999",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := Parse(strings.NewReader(tc.src))

			require.Equal(t, tc.expectedErr, err)
			require.Nil(t, file)
		})
	}
}

func TestParseUnbalancedLists(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name: "an unmatched opening parenthesis",
			src:  `(`,
			expectedErr: UnexpectedEndOfTokensError{
				Expected: []TokenType{TokenRParen},
				Pos:      Pos{Line: 1, Column: 1},
			},
		},
		{
			name: "a list left open after an element",
			src:  `(a`,
			expectedErr: UnexpectedEndOfTokensError{
				Expected: []TokenType{TokenRParen},
				Pos:      Pos{Line: 1, Column: 2},
			},
		},
		{
			name: "an outer list left open",
			src:  `(()`,
			expectedErr: UnexpectedEndOfTokensError{
				Expected: []TokenType{TokenRParen},
				Pos:      Pos{Line: 1, Column: 3},
			},
		},
		{
			name: "a list left open across lines",
			src: `(add
  1`,
			expectedErr: UnexpectedEndOfTokensError{
				Expected: []TokenType{TokenRParen},
				Pos:      Pos{Line: 2, Column: 3},
			},
		},
		{
			name: "a stray closing parenthesis",
			src:  `)`,
			expectedErr: UnexpectedTokenError{
				Expected: datumTokens,
				Actual:   Token{Pos: Pos{Line: 1, Column: 1}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a closing parenthesis after a datum",
			src:  `a)`,
			expectedErr: UnexpectedTokenError{
				Expected: datumTokens,
				Actual:   Token{Pos: Pos{Line: 1, Column: 2}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "one closing parenthesis too many",
			src:  `(a))`,
			expectedErr: UnexpectedTokenError{
				Expected: datumTokens,
				Actual:   Token{Pos: Pos{Line: 1, Column: 4}, Type: TokenRParen, Value: []byte(")")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := Parse(strings.NewReader(tc.src))

			require.Equal(t, tc.expectedErr, err)
			require.Nil(t, file)
		})
	}
}

func TestParseMalformedDottedPairs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name: "a dot with nothing before it",
			src:  `(. a)`,
			expectedErr: UnexpectedTokenError{
				Expected: datumTokens,
				Actual:   Token{Pos: Pos{Line: 1, Column: 2}, Type: TokenDot, Value: []byte(".")},
			},
		},
		{
			name: "a dot with nothing after it",
			src:  `(a . )`,
			expectedErr: UnexpectedTokenError{
				Expected: datumTokens,
				Actual:   Token{Pos: Pos{Line: 1, Column: 6}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a dot immediately before the closing parenthesis",
			src:  `(a .)`,
			expectedErr: UnexpectedTokenError{
				Expected: datumTokens,
				Actual:   Token{Pos: Pos{Line: 1, Column: 5}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "more than one datum after the dot",
			src:  `(a . b c)`,
			expectedErr: UnexpectedTokenError{
				Expected: []TokenType{TokenRParen},
				Actual:   Token{Pos: Pos{Line: 1, Column: 8}, Type: TokenSymbol, Value: []byte("c")},
			},
		},
		{
			name: "two dots in one list",
			src:  `(a . b . c)`,
			expectedErr: UnexpectedTokenError{
				Expected: []TokenType{TokenRParen},
				Actual:   Token{Pos: Pos{Line: 1, Column: 8}, Type: TokenDot, Value: []byte(".")},
			},
		},
		{
			name: "a lone dot in a list",
			src:  `(.)`,
			expectedErr: UnexpectedTokenError{
				Expected: datumTokens,
				Actual:   Token{Pos: Pos{Line: 1, Column: 2}, Type: TokenDot, Value: []byte(".")},
			},
		},
		{
			name: "input ending after the dot",
			src:  `(a .`,
			expectedErr: UnexpectedEndOfTokensError{
				Expected: datumTokens,
				Pos:      Pos{Line: 1, Column: 4},
			},
		},
		{
			name: "input ending after the tail",
			src:  `(a . b`,
			expectedErr: UnexpectedEndOfTokensError{
				Expected: []TokenType{TokenRParen},
				Pos:      Pos{Line: 1, Column: 6},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := Parse(strings.NewReader(tc.src))

			require.Equal(t, tc.expectedErr, err)
			require.Nil(t, file)
		})
	}
}

func TestParseComments(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		src              string
		expectedNodes    int
		expectedFile     []*Comment
		expectedListPath []int      // indexes to walk from file.Nodes to a list
		expectedList     []*Comment // comments expected on that list
	}{
		{
			name:          "a file of only comments",
			src:           "; one\n; two",
			expectedNodes: 0,
			expectedFile: []*Comment{
				{Pos: Pos{Line: 1, Column: 1}, Text: "; one"},
				{Pos: Pos{Line: 2, Column: 1}, Text: "; two"},
			},
		},
		{
			name:          "a file of only a block comment",
			src:           `#| nothing else |#`,
			expectedNodes: 0,
			expectedFile: []*Comment{
				{Pos: Pos{Line: 1, Column: 1}, Text: "#| nothing else |#"},
			},
		},
		{
			name:          "comments around a top level datum",
			src:           "; lead\na ; trail",
			expectedNodes: 1,
			expectedFile: []*Comment{
				{Pos: Pos{Line: 1, Column: 1}, Text: "; lead"},
				{Pos: Pos{Line: 2, Column: 3}, Text: "; trail"},
			},
		},
		{
			name:          "both comment styles at the top level",
			src:           `#| a |# x ; b`,
			expectedNodes: 1,
			expectedFile: []*Comment{
				{Pos: Pos{Line: 1, Column: 1}, Text: "#| a |#"},
				{Pos: Pos{Line: 1, Column: 11}, Text: "; b"},
			},
		},
		{
			name:             "comments inside a list stay on the list",
			src:              "(; first\n a b ; last\n)",
			expectedNodes:    1,
			expectedFile:     nil,
			expectedListPath: []int{0},
			expectedList: []*Comment{
				{Pos: Pos{Line: 1, Column: 2}, Text: "; first"},
				{Pos: Pos{Line: 2, Column: 6}, Text: "; last"},
			},
		},
		{
			name:             "a comment before a dotted pair tail",
			src:              "(a . ; before\n b)",
			expectedNodes:    1,
			expectedFile:     nil,
			expectedListPath: []int{0},
			expectedList: []*Comment{
				{Pos: Pos{Line: 1, Column: 6}, Text: "; before"},
			},
		},
		{
			name:          "a comment between a macro and its datum",
			src:           "' ; between\n x",
			expectedNodes: 1,
			expectedFile: []*Comment{
				{Pos: Pos{Line: 1, Column: 3}, Text: "; between"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := Parse(strings.NewReader(tc.src))

			require.NoError(t, err)
			require.Len(t, file.Nodes, tc.expectedNodes)
			require.Equal(t, tc.expectedFile, file.Comments)

			if tc.expectedListPath != nil {
				node := file.Nodes[tc.expectedListPath[0]]
				list, ok := node.(List)
				require.Truef(t, ok, "expected a list, got %T", node)
				require.Equal(t, tc.expectedList, list.Comments)
			}
		})
	}
}

func TestParseCommentsAreScopedToTheirContainer(t *testing.T) {
	t.Parallel()

	t.Run("will attach each comment to the list which encloses it", func(t *testing.T) {
		t.Parallel()

		file, err := Parse(strings.NewReader("; file\n((a ; inner\n) ; outer\n)"))
		require.NoError(t, err)

		require.Equal(t, []*Comment{
			{Pos: Pos{Line: 1, Column: 1}, Text: "; file"},
		}, file.Comments)

		outer, ok := file.Nodes[0].(List)
		require.True(t, ok)
		require.Equal(t, []*Comment{
			{Pos: Pos{Line: 3, Column: 3}, Text: "; outer"},
		}, outer.Comments)

		inner, ok := outer.Elements[0].(List)
		require.True(t, ok)
		require.Equal(t, []*Comment{
			{Pos: Pos{Line: 2, Column: 5}, Text: "; inner"},
		}, inner.Comments)
	})
}

func TestParseCommentsNeverBecomeNodes(t *testing.T) {
	t.Parallel()

	// Comment has no sexpr method, so a comment cannot be a Node at all. What
	// is worth checking is that draining them leaves the datum tree exactly as
	// it would be without them, rather than dropping or duplicating elements.
	testCases := []struct {
		name     string
		src      string
		stripped string
	}{
		{name: "leading and trailing", src: "; lead\na ; trail", stripped: "a"},
		{name: "inside a list", src: "(; a\n b ; c\n)", stripped: "(b)"},
		{name: "block comments", src: "#| x |# (#| y |# z)", stripped: "(z)"},
		{name: "around a tail", src: "(a . ; t\n b)", stripped: "(a . b)"},
		{name: "after a macro", src: "' ; m\n x", stripped: "'x"},
		{name: "between elements", src: "(a ; one\n b ; two\n c)", stripped: "(a b c)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			withComments, err := Parse(strings.NewReader(tc.src))
			require.NoError(t, err)

			without, err := Parse(strings.NewReader(tc.stripped))
			require.NoError(t, err)

			require.Equal(t, shapeOf(without.Nodes), shapeOf(withComments.Nodes))
		})
	}
}

// shapeOf renders the datum tree without positions or comments, so two parses
// of the same data can be compared regardless of where comments sat.
func shapeOf(nodes []Node) string {
	var out strings.Builder
	for i, n := range nodes {
		if i > 0 {
			out.WriteByte(' ')
		}
		writeShape(&out, n)
	}
	return out.String()
}

func writeShape(out *strings.Builder, n Node) {
	switch node := n.(type) {
	case List:
		out.WriteByte('(')
		for i, e := range node.Elements {
			if i > 0 {
				out.WriteByte(' ')
			}
			writeShape(out, e)
		}
		if node.Tail != nil {
			out.WriteString(" . ")
			writeShape(out, node.Tail)
		}
		out.WriteByte(')')
	case Quote:
		out.WriteString(node.Kind.String())
		out.WriteByte('<')
		writeShape(out, node.Datum)
		out.WriteByte('>')
	case Symbol:
		out.WriteString(node.Value)
	case String:
		out.WriteString(strconv.Quote(node.Value))
	case Int:
		out.WriteString(strconv.FormatInt(node.Value, 10))
	case Float:
		out.WriteString(strconv.FormatFloat(node.Value, 'g', -1, 64))
	case Bool:
		out.WriteString(strconv.FormatBool(node.Value))
	case Nil:
		out.WriteString("nil")
	}
}

func TestParseMalformedQuoteForms(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name: "a quote with nothing after it",
			src:  `'`,
			expectedErr: UnexpectedEndOfTokensError{
				Expected: datumTokens,
				Pos:      Pos{Line: 1, Column: 1},
			},
		},
		{
			name: "a quasiquote with nothing after it",
			src:  "`",
			expectedErr: UnexpectedEndOfTokensError{
				Expected: datumTokens,
				Pos:      Pos{Line: 1, Column: 1},
			},
		},
		{
			name: "an unquote splice with nothing after it",
			src:  `,@`,
			expectedErr: UnexpectedEndOfTokensError{
				Expected: datumTokens,
				Pos:      Pos{Line: 1, Column: 1},
			},
		},
		{
			name: "stacked quotes with nothing after them",
			src:  `''`,
			expectedErr: UnexpectedEndOfTokensError{
				Expected: datumTokens,
				Pos:      Pos{Line: 1, Column: 2},
			},
		},
		{
			name: "a quote immediately before a closing parenthesis",
			src:  `(')`,
			expectedErr: UnexpectedTokenError{
				Expected: datumTokens,
				Actual:   Token{Pos: Pos{Line: 1, Column: 3}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a quote before the close of a longer list",
			src:  `(a ')`,
			expectedErr: UnexpectedTokenError{
				Expected: datumTokens,
				Actual:   Token{Pos: Pos{Line: 1, Column: 5}, Type: TokenRParen, Value: []byte(")")},
			},
		},
		{
			name: "a quote before a dot",
			src:  `(a ' . b)`,
			expectedErr: UnexpectedTokenError{
				Expected: datumTokens,
				Actual:   Token{Pos: Pos{Line: 1, Column: 6}, Type: TokenDot, Value: []byte(".")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := Parse(strings.NewReader(tc.src))

			require.Equal(t, tc.expectedErr, err)
			require.Nil(t, file)
		})
	}
}

func TestParseQuoteDepth(t *testing.T) {
	t.Parallel()

	// A run of quote macros recurses exactly as a run of open parentheses does,
	// even though it does not look like nesting. Without counting quotes
	// against the bound this overflows the stack and takes the process with it,
	// so this test guards a crash rather than a wrong answer.
	t.Run("will bound a long run of quote macros", func(t *testing.T) {
		t.Parallel()

		_, err := Parse(strings.NewReader(strings.Repeat("'", 1_000_000) + "x"))

		require.IsType(t, MaxDepthExceededError{}, err)
	})

	t.Run("will accept a run of quotes up to the limit", func(t *testing.T) {
		t.Parallel()

		file, err := Parse(strings.NewReader(strings.Repeat("'", MaxDepth) + "x"))

		require.NoError(t, err)
		require.Len(t, file.Nodes, 1)
	})

	t.Run("will reject a run of quotes one beyond the limit", func(t *testing.T) {
		t.Parallel()

		_, err := Parse(strings.NewReader(strings.Repeat("'", MaxDepth+1) + "x"))

		require.IsType(t, MaxDepthExceededError{}, err)
	})
}

func TestQuoteKind(t *testing.T) {
	t.Parallel()

	t.Run("will name each kind", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "Quote", QuoteKindQuote.String())
		require.Equal(t, "Quasiquote", QuoteKindQuasiquote.String())
		require.Equal(t, "Unquote", QuoteKindUnquote.String())
		require.Equal(t, "UnquoteSplicing", QuoteKindUnquoteSplicing.String())
	})

	t.Run("will name every declared kind", func(t *testing.T) {
		t.Parallel()

		for k := QuoteKindQuote; k <= QuoteKindUnquoteSplicing; k++ {
			require.NotPanics(t, func() {
				require.NotEmpty(t, k.String())
			})
		}
	})

	t.Run("will panic on an unknown kind", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			_ = QuoteKind(-1).String()
		})
		require.Panics(t, func() {
			_ = (QuoteKindUnquoteSplicing + 1).String()
		})
	})
}

func TestParseDottedPairDepth(t *testing.T) {
	t.Parallel()

	// A tail sits inside the list which holds it, so it must count against the
	// same depth budget as an element. Nesting through tails alone would
	// otherwise escape the bound entirely.
	t.Run("will count nesting reached through a tail", func(t *testing.T) {
		t.Parallel()

		var src strings.Builder
		for range MaxDepth + 1 {
			src.WriteString("(a . ")
		}
		src.WriteString("x")
		for range MaxDepth + 1 {
			src.WriteString(")")
		}

		_, err := Parse(strings.NewReader(src.String()))

		require.IsType(t, MaxDepthExceededError{}, err)
	})
}

// nest builds input nested depth levels deep around the given body.
func nest(depth int, body string) string {
	return strings.Repeat("(", depth) + body + strings.Repeat(")", depth)
}

func TestParseNestingDepth(t *testing.T) {
	t.Parallel()

	t.Run("will parse input nested one thousand levels deep", func(t *testing.T) {
		t.Parallel()

		file, err := Parse(strings.NewReader(nest(1_000, "x")))

		require.NoError(t, err)
		require.Len(t, file.Nodes, 1)

		// Walk down to confirm the whole chain is there rather than trusting
		// that the top level node alone looks right.
		node := file.Nodes[0]
		for i := range 1_000 {
			list, ok := node.(List)
			require.Truef(t, ok, "expected a list at depth %d, got %T", i, node)
			require.Len(t, list.Elements, 1)
			node = list.Elements[0]
		}
		require.Equal(t, Symbol{Pos: Pos{Line: 1, Column: 1001}, Value: "x"}, node)
	})

	t.Run("will parse input nested exactly to the limit", func(t *testing.T) {
		t.Parallel()

		file, err := Parse(strings.NewReader(nest(MaxDepth, "x")))

		require.NoError(t, err)
		require.Len(t, file.Nodes, 1)
	})

	t.Run("will reject input nested one level beyond the limit", func(t *testing.T) {
		t.Parallel()

		file, err := Parse(strings.NewReader(nest(MaxDepth+1, "x")))

		require.Equal(t, MaxDepthExceededError{
			Pos:   Pos{Line: 1, Column: MaxDepth + 1},
			Depth: MaxDepth,
		}, err)
		require.Nil(t, file)
	})

	t.Run("will reject deep input without exhausting the stack", func(t *testing.T) {
		t.Parallel()

		// Far beyond the limit: the depth check must reject this long before
		// recursion could overflow.
		_, err := Parse(strings.NewReader(strings.Repeat("(", 1_000_000)))

		require.IsType(t, MaxDepthExceededError{}, err)
	})
}

func TestMaxDepthExceededError(t *testing.T) {
	t.Parallel()

	t.Run("will report the limit and position", func(t *testing.T) {
		t.Parallel()

		err := MaxDepthExceededError{Pos: Pos{Line: 2, Column: 9}, Depth: 10_000}

		require.Equal(t, "maximum nesting depth of 10000 exceeded at line 2, column 9", err.Error())
	})
}

func TestParseRejectsTokensWithoutADatum(t *testing.T) {
	t.Parallel()

	// A closing parenthesis and a dot only ever appear while a list is being
	// read, so neither can begin a datum. Every other token type now can.
	testCases := []struct {
		name string
		src  string
		tok  Token
	}{
		{
			name: "a closing parenthesis",
			src:  `)`,
			tok:  Token{Pos: Pos{Line: 1, Column: 1}, Type: TokenRParen, Value: []byte(")")},
		},
		{
			name: "a dotted pair marker",
			src:  `.`,
			tok:  Token{Pos: Pos{Line: 1, Column: 1}, Type: TokenDot, Value: []byte(".")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse(strings.NewReader(tc.src))

			require.Equal(t, UnexpectedTokenError{Expected: datumTokens, Actual: tc.tok}, err)
		})
	}
}

func TestParsePropagatesTokenizerErrors(t *testing.T) {
	t.Parallel()

	t.Run("will return the tokenizer's error unchanged", func(t *testing.T) {
		t.Parallel()

		file, err := Parse(strings.NewReader(`a ]`))

		require.Equal(t, UnexpectedCharacterError{Pos: Pos{Line: 1, Column: 3}, R: ']'}, err)
		require.Nil(t, file)
	})

	t.Run("will return a malformed number error from the tokenizer", func(t *testing.T) {
		t.Parallel()

		_, err := Parse(strings.NewReader(`1.2.3`))

		require.Equal(t, InvalidNumberError{Pos: Pos{Line: 1, Column: 1}, Value: "1.2.3"}, err)
	})
}

func TestParserPrimitives(t *testing.T) {
	t.Parallel()

	t.Run("will read tokens in order", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser(`a b`)
		defer stop()

		tok, err, ok := p.read()
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, []byte("a"), tok.Value)

		tok, err, ok = p.read()
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, []byte("b"), tok.Value)

		_, err, ok = p.read()
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("will peek without consuming", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser(`a b`)
		defer stop()

		peeked, err, ok := p.peek()
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, []byte("a"), peeked.Value)

		// Peeking again returns the same token.
		again, _, _ := p.peek()
		require.Equal(t, peeked, again)

		read, _, _ := p.read()
		require.Equal(t, peeked, read)

		next, _, _ := p.read()
		require.Equal(t, []byte("b"), next.Value)
	})

	t.Run("will peek past the end of input", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser(``)
		defer stop()

		_, err, ok := p.peek()
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("will hand back an unread token", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser(`a b`)
		defer stop()

		first, _, _ := p.read()
		p.unread(first)

		again, err, ok := p.read()
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, first, again)

		next, _, _ := p.read()
		require.Equal(t, []byte("b"), next.Value)
	})

	t.Run("will return comments rather than skipping them", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser("; one\na")
		defer stop()

		first, _, _ := p.read()
		require.Equal(t, TokenComment, first.Type)
		require.Equal(t, []byte("; one"), first.Value)
	})

	t.Run("will drain leading comments", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser("; one\n#| two |# a #| three |# b")
		defer stop()

		var comments []*Comment
		require.NoError(t, p.collectComments(&comments))
		require.Equal(t, []*Comment{
			{Pos: Pos{Line: 1, Column: 1}, Text: "; one"},
			{Pos: Pos{Line: 2, Column: 1}, Text: "#| two |#"},
		}, comments)

		// Draining stops at the first token which is not a comment.
		tok, _, _ := p.read()
		require.Equal(t, TokenSymbol, tok.Type)
		require.Equal(t, []byte("a"), tok.Value)
	})

	t.Run("will drain nothing when no comment is next", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser(`a ; later`)
		defer stop()

		var comments []*Comment
		require.NoError(t, p.collectComments(&comments))
		require.Empty(t, comments)

		tok, _, _ := p.read()
		require.Equal(t, TokenSymbol, tok.Type)
	})

	t.Run("will drain nothing at end of input", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser(``)
		defer stop()

		var comments []*Comment
		require.NoError(t, p.collectComments(&comments))
		require.Empty(t, comments)
	})

	t.Run("will surface a tokenizer error while draining", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser(`]`)
		defer stop()

		var comments []*Comment
		err := p.collectComments(&comments)
		require.Equal(t, UnexpectedCharacterError{Pos: Pos{Line: 1, Column: 1}, R: ']'}, err)
	})

	t.Run("will accept an expected token type", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser(`a`)
		defer stop()

		tok, err := p.expect(TokenSymbol, TokenNumber)
		require.NoError(t, err)
		require.Equal(t, []byte("a"), tok.Value)
	})

	t.Run("will reject an unexpected token type", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser(`(`)
		defer stop()

		_, err := p.expect(TokenSymbol, TokenNumber)
		require.Equal(t, UnexpectedTokenError{
			Expected: []TokenType{TokenSymbol, TokenNumber},
			Actual:   Token{Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
		}, err)
	})

	t.Run("will report the end of tokens with the last position seen", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser(`a`)
		defer stop()

		_, err := p.expect(TokenSymbol)
		require.NoError(t, err)

		_, err = p.expect(TokenSymbol)
		require.Equal(t, UnexpectedEndOfTokensError{
			Expected: []TokenType{TokenSymbol},
			Pos:      Pos{Line: 1, Column: 1},
		}, err)
	})

	t.Run("will surface a tokenizer error from expect", func(t *testing.T) {
		t.Parallel()

		p, stop := newTestParser(`]`)
		defer stop()

		_, err := p.expect(TokenSymbol)
		require.Equal(t, UnexpectedCharacterError{Pos: Pos{Line: 1, Column: 1}, R: ']'}, err)
	})
}

func TestParserErrorMessages(t *testing.T) {
	t.Parallel()

	t.Run("will describe an unexpected token", func(t *testing.T) {
		t.Parallel()

		err := UnexpectedTokenError{
			Expected: []TokenType{TokenSymbol, TokenNumber},
			Actual:   Token{Pos: Pos{Line: 2, Column: 5}, Type: TokenLParen, Value: []byte("(")},
		}

		require.Equal(t, "unexpected token at line 2, column 5: LParen((), expected one of: Symbol, Number", err.Error())
	})

	t.Run("will describe the end of tokens", func(t *testing.T) {
		t.Parallel()

		err := UnexpectedEndOfTokensError{
			Expected: []TokenType{TokenRParen},
			Pos:      Pos{Line: 3, Column: 7},
		}

		require.Equal(t, "unexpected end of tokens at line 3, column 7, expected one of: RParen", err.Error())
	})

	t.Run("will describe a number out of range", func(t *testing.T) {
		t.Parallel()

		err := NumberRangeError{Pos: Pos{Line: 1, Column: 4}, Value: "1e400"}

		require.Equal(t, `number literal "1e400" out of range at line 1, column 4`, err.Error())
	})
}

// Every atom node type implements Node. These fail to compile rather than
// fail at run time if one stops satisfying the interface.
var (
	_ Node = Symbol{}
	_ Node = String{}
	_ Node = Int{}
	_ Node = Float{}
	_ Node = Bool{}
	_ Node = Nil{}
	_ Node = List{}
	_ Node = Quote{}
)

func TestNumberError(t *testing.T) {
	t.Parallel()

	// Only the range branch is reachable through Parse, since the tokenizer
	// rejects malformed lexemes before the parser sees them. Both are covered
	// here so the distinction does not rot.
	t.Run("will report a value which does not fit its type as out of range", func(t *testing.T) {
		t.Parallel()

		_, parseErr := strconv.ParseInt("9223372036854775808", 10, 64)
		err := numberError(Pos{Line: 1, Column: 1}, "9223372036854775808", parseErr)

		require.Equal(t, NumberRangeError{Pos: Pos{Line: 1, Column: 1}, Value: "9223372036854775808"}, err)
	})

	t.Run("will report a value which is not a number as malformed", func(t *testing.T) {
		t.Parallel()

		_, parseErr := strconv.ParseInt("abc", 10, 64)
		err := numberError(Pos{Line: 2, Column: 4}, "abc", parseErr)

		require.Equal(t, InvalidNumberError{Pos: Pos{Line: 2, Column: 4}, Value: "abc"}, err)
	})
}

func TestDecodeStringRejectsBadInput(t *testing.T) {
	t.Parallel()

	// The tokenizer validates escapes, so these are defensive paths which
	// Parse cannot reach. They are exercised directly to keep them honest.
	testCases := []struct {
		name        string
		raw         string
		expectedErr error
	}{
		{
			name:        "a trailing backslash",
			raw:         `a\`,
			expectedErr: InvalidEscapeError{Pos: Pos{Line: 1, Column: 1}, R: '\\'},
		},
		{
			name:        "an unrecognized escape",
			raw:         `a\q`,
			expectedErr: InvalidEscapeError{Pos: Pos{Line: 1, Column: 1}, R: 'q'},
		},
		{
			name:        "a truncated unicode escape",
			raw:         `a\u00`,
			expectedErr: InvalidEscapeError{Pos: Pos{Line: 1, Column: 1}, R: 'u'},
		},
		{
			name:        "a unicode escape with non-hex digits",
			raw:         `a\uZZZZ`,
			expectedErr: InvalidEscapeError{Pos: Pos{Line: 1, Column: 1}, R: 'u'},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := decodeString(Pos{Line: 1, Column: 1}, []byte(tc.raw))

			require.Equal(t, tc.expectedErr, err)
		})
	}
}
