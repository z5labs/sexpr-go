// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr_test

import (
	"fmt"
	"os"
	"strings"

	"github.com/z5labs/sexpr-go"
)

func ExampleTokenize() {
	src := `(add 1 2.5 "hi" #t) ; done`

	for tok, err := range sexpr.Tokenize(strings.NewReader(src)) {
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("%d:%d %s\n", tok.Pos.Line, tok.Pos.Column, tok)
	}

	// Output:
	// 1:1 LParen(()
	// 1:2 Symbol(add)
	// 1:6 Number(1)
	// 1:8 Number(2.5)
	// 1:12 String(hi)
	// 1:17 Bool(#t)
	// 1:19 RParen())
	// 1:21 Comment(; done)
}

func ExampleParse() {
	src := `(add 1 2.5 "hi" #t nil)`

	f, err := sexpr.Parse(strings.NewReader(src))
	if err != nil {
		fmt.Println(err)
		return
	}

	list := f.Nodes[0].(sexpr.List)
	for _, elem := range list.Elements {
		switch n := elem.(type) {
		case sexpr.Symbol:
			fmt.Printf("symbol %s\n", n.Value)
		case sexpr.Int:
			fmt.Printf("int %d\n", n.Value)
		case sexpr.Float:
			fmt.Printf("float %v\n", n.Value)
		case sexpr.String:
			fmt.Printf("string %q\n", n.Value)
		case sexpr.Bool:
			fmt.Printf("bool %t\n", n.Value)
		case sexpr.Nil:
			fmt.Println("nil")
		}
	}

	// Output:
	// symbol add
	// int 1
	// float 2.5
	// string "hi"
	// bool true
	// nil
}

func ExampleParse_errors() {
	_, err := sexpr.Parse(strings.NewReader(`(add 1`))
	if err != nil {
		fmt.Println(err)
	}

	_, err = sexpr.Parse(strings.NewReader(`(add 1 . )`))
	if err != nil {
		fmt.Println(err)
	}

	_, err = sexpr.Parse(strings.NewReader(`(add 12abc)`))
	if err != nil {
		fmt.Println(err)
	}

	_, err = sexpr.Parse(strings.NewReader(`(add "oops)`))
	if err != nil {
		fmt.Println(err)
	}

	// Output:
	// unexpected end of tokens at line 1, column 6, expected one of: RParen
	// unexpected token at line 1, column 10: RParen()), expected one of: LParen, Quote, Symbol, String, Number, Bool
	// invalid number literal "12abc" at line 1, column 6
	// unterminated string literal at line 1, column 6
}

func ExamplePrint() {
	f := &sexpr.File{
		Nodes: []sexpr.Node{
			sexpr.List{
				Elements: []sexpr.Node{
					sexpr.Symbol{Value: "add"},
					sexpr.Int{Value: 1},
					sexpr.Int{Value: 2},
				},
			},
			sexpr.List{
				Elements: []sexpr.Node{
					sexpr.Symbol{Value: "define"},
					sexpr.Symbol{Value: "a-name-long-enough-to-need-breaking"},
					sexpr.String{Value: "a value long enough to push past the line width"},
				},
			},
		},
	}

	if err := sexpr.Print(os.Stdout, f); err != nil {
		fmt.Println(err)
	}

	// Output:
	// (add 1 2)
	// (define
	//   a-name-long-enough-to-need-breaking
	//   "a value long enough to push past the line width")
}
