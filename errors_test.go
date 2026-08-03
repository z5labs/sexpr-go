// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"go/ast"
	// Aliased because this package already declares a parser type.
	goparser "go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// errorCase pairs an error value with the message it must produce.
type errorCase struct {
	name     string
	err      error
	expected string
}

// exportedErrorCases is the one list of exported errors the tests work from.
//
// TestExportedErrorMessages pins each message; TestExportedErrorsAreAllCovered
// checks the list against the package source, so an error type added without a
// case here fails rather than quietly shipping an unchecked message.
func exportedErrorCases() []errorCase {
	return []errorCase{
		// Tokenizer.
		{
			name:     "UnexpectedCharacterError",
			err:      UnexpectedCharacterError{Pos: Pos{Line: 1, Column: 2}, R: '['},
			expected: "unexpected character '[' at line 1, column 2",
		},
		{
			name:     "UnterminatedCommentError",
			err:      UnterminatedCommentError{Pos: Pos{Line: 3, Column: 4}},
			expected: "unterminated block comment at line 3, column 4",
		},
		{
			name:     "UnterminatedStringError",
			err:      UnterminatedStringError{Pos: Pos{Line: 5, Column: 6}},
			expected: "unterminated string literal at line 5, column 6",
		},
		{
			name:     "InvalidNumberError",
			err:      InvalidNumberError{Pos: Pos{Line: 7, Column: 8}, Value: "1.2.3"},
			expected: `invalid number literal "1.2.3" at line 7, column 8`,
		},
		{
			name:     "InvalidEscapeError",
			err:      InvalidEscapeError{Pos: Pos{Line: 9, Column: 10}, R: 'q'},
			expected: `invalid escape sequence '\q' at line 9, column 10`,
		},

		// Parser.
		{
			name:     "MaxDepthExceededError",
			err:      MaxDepthExceededError{Pos: Pos{Line: 11, Column: 12}, Depth: 10000},
			expected: "maximum nesting depth of 10000 exceeded at line 11, column 12",
		},
		{
			name:     "UnexpectedEndOfTokensError",
			err:      UnexpectedEndOfTokensError{Expected: []TokenType{TokenRParen}, Pos: Pos{Line: 13, Column: 14}},
			expected: "unexpected end of tokens at line 13, column 14, expected one of: RParen",
		},
		{
			name: "UnexpectedTokenError",
			err: UnexpectedTokenError{
				Expected: []TokenType{TokenSymbol, TokenNumber},
				Actual:   Token{Pos: Pos{Line: 15, Column: 16}, Type: TokenLParen, Value: []byte("(")},
			},
			expected: "unexpected token at line 15, column 16: LParen((), expected one of: Symbol, Number",
		},
		{
			name:     "NumberRangeError",
			err:      NumberRangeError{Pos: Pos{Line: 17, Column: 18}, Value: "1e400"},
			expected: `number literal "1e400" out of range at line 17, column 18`,
		},

		// Printer.
		{
			name:     "UnsupportedNodeError with a node",
			err:      UnsupportedNodeError{Node: unknownNode{}},
			expected: "cannot print a node of type sexpr.unknownNode",
		},
		{
			name:     "UnsupportedNodeError with no node",
			err:      UnsupportedNodeError{},
			expected: "cannot print a nil node",
		},
		{
			name:     "NonFiniteFloatError",
			err:      NonFiniteFloatError{Value: math.Inf(-1)},
			expected: "cannot print the non-finite float -Inf",
		},
		{
			name:     "TailWithoutElementsError",
			err:      TailWithoutElementsError{Pos: Pos{Line: 19, Column: 20}},
			expected: "cannot print a list with a tail but no elements at line 19, column 20",
		},
		{
			name:     "InvalidSymbolError",
			err:      InvalidSymbolError{Pos: Pos{Line: 21, Column: 22}, Value: "a b"},
			expected: `cannot print "a b" as a symbol at line 21, column 22`,
		},

		// Sentinels.
		{
			name:     "ErrNilFile",
			err:      ErrNilFile,
			expected: "cannot print a nil file",
		},
		{
			name:     "ErrNilComment",
			err:      ErrNilComment,
			expected: "cannot print a nil comment",
		},
	}
}

// TestExportedErrorMessages pins the message of every exported error.
func TestExportedErrorMessages(t *testing.T) {
	t.Parallel()

	for _, tc := range exportedErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.expected, tc.err.Error())
		})
	}
}

// TestExportedErrorsAreAllCovered reads the package source for exported error
// types and requires each one to appear in [exportedErrorCases].
//
// Asserting against the source rather than against a second hand written list
// is what makes this catch a new error type: a list would only ever agree with
// whatever was last written into it.
func TestExportedErrorsAreAllCovered(t *testing.T) {
	t.Parallel()

	covered := make(map[string]bool)
	for _, tc := range exportedErrorCases() {
		typ := reflect.TypeOf(tc.err)
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}

		// The sentinels are unexported types behind exported vars; their
		// messages are pinned above but there is no declaration to match.
		if name := typ.Name(); token.IsExported(name) {
			covered[name] = true
		}
	}

	declared := declaredErrorTypes(t)
	require.NotEmpty(t, declared, "no exported error types found; the scan is broken")

	for _, name := range declared {
		require.Truef(t, covered[name], "%s has no case in exportedErrorCases", name)
	}
}

// declaredErrorTypes reports every exported type in the package which declares
// an Error method, found by parsing the non-test sources.
func declaredErrorTypes(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()

	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := goparser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "Error" || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}

			receiver := fn.Recv.List[0].Type
			if star, ok := receiver.(*ast.StarExpr); ok {
				receiver = star.X
			}

			ident, ok := receiver.(*ast.Ident)
			if !ok || !ident.IsExported() {
				continue
			}
			names = append(names, ident.Name)
		}
	}
	return names
}
