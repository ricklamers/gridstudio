package main

import "github.com/alecthomas/participle/lexer"

type Operator int

const (
	OpMul Operator = iota
	OpDiv
	OpAdd
	OpSub
)

var operatorMap = map[string]Operator{"+": OpAdd, "-": OpSub, "*": OpMul, "/": OpDiv}

func (o *Operator) Capture(s []string) error {
	*o = operatorMap[s[0]]
	return nil
}

type FORMULA struct {
	Expr *Expr `"="@@`
}

type Expr struct {
	Left  *Term     `@@`
	Right []*OpTerm `( @@ )?`
}

type Factor struct {
	Base     *Value `(@@)+`
	Exponent *Value `( "^" @@ )*`
}

type OpFactor struct {
	Operator Operator `@("*" | "/")`
	Factor   *Factor  `@@`
}

type Term struct {
	Left  *Factor     `@@`
	Right []*OpFactor `( @@ )?`
}

type OpTerm struct {
	Operator Operator `@("+" | "-")`
	Term     *Term    `@@`
}

type Function struct {
	Name          string `@Ident "("`
	FirstArg      *Expr  `@@`
	ArgumentList  *Expr  `("," @@)*`
	FinalArgument *Expr  `")"`
}

type AbsoluteRef struct {
	Ref string `@AbsoluteRef`
}

type RelativeRef struct {
	Ref string `@RelativeRef`
}

type RefRange struct {
	AbsoluteRefStart AbsoluteRef `@@ ":"`
	AbsoluteRefEnd   RelativeRef `@@`
	RelativeRefStart RelativeRef `| @@ ":"`
	RelativeRefEnd   RelativeRef `@@`
}

type Value struct {
	// Pos    lexer.Position
	String        *string      `  @String`
	Number        *float64     `| @Float`
	BoolTrue      *bool        `| @("TRUE")`
	BoolFalse     *bool        `| @("FALSE")`
	Subexpression *Expr        `| "(" @@ ")"`
	Function      *Function    `| @@`
	RefRange      *RefRange    `| @@`
	AbsoluteRef   *AbsoluteRef `| @@`
	RelativeRef   *RelativeRef `| @@`
}

var formulaLexer = lexer.Must(lexer.Regexp(
	`(?m)` +
		`(\s+)` +
		`|(^[#;].*$)` +
		`|(?P<RelativeFullRefRange>[a-zA-Z]+:[a-zA-Z]+)` +
		`|(?P<AbsoluteFullRefRange>(('[a-zA-Z]+[ a-zA-Z\d]*')|("[a-zA-Z]+[ a-zA-Z\d]*")|([a-zA-Z]+[a-zA-Z\d]*))\![a-zA-Z]+:[a-zA-Z]+)` +
		`|(?P<AbsoluteRef>(('[a-zA-Z]+[ a-zA-Z\d]*')|("[a-zA-Z]+[ a-zA-Z\d]*")|([a-zA-Z]+[a-zA-Z\d]*))\![a-zA-Z]+[\d]*)` +
		`|(?P<RelativeRef>[a-zA-Z]+[\d]+)` +
		`|(?P<String>("(?:\\.|[^"])*")|('(?:\\.|[^'])*'))` +
		`|(?P<Float>\-?\d+(?:\.\d+)?)` +
		`|(?P<Ident>[a-zA-Z][a-zA-Z_\d]*)` +
		`|(?P<Punct>[\+\-\*\/)(=,^:\!])`,
))
