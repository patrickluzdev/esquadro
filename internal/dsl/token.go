// Package dsl implements the lexer and parser for .esq files and the syntax
// tree they produce.
//
// Grammar (informal; newlines end fields, settings and options):
//
//	File       = { AppDecl | EnumDecl | EntityDecl | JobDecl | ReportDecl } .
//	AppDecl    = "app" Ident "{" { AppItem } "}" .
//	AppItem    = "module" String
//	           | "blueprint" ( Ident | String ) [ "{" { Option } "}" ]
//	           | Annotation .
//	EnumDecl   = "enum" Ident "{" { Ident [ "," ] } "}" .
//	EntityDecl = "entity" Ident [ "(" Option { "," Option } ")" ] "{" { Member } "}" .
//	JobDecl    = "job" Ident [ "{" { Member } "}" ] .
//	ReportDecl = "report" Ident "{" { Member } "}" .
//	Member     = Field | Relation | Annotation .
//	Field      = Ident Ident { Modifier | Annotation } .
//	Relation   = Ident ( ">" | "-" | "<>" | "<" ) Ident [ "." Ident ] [ "(" Ident ")" ] { Modifier | Annotation } .
//	Modifier   = Ident [ "(" [ Value { "," Value } ] ")" ] .
//	Annotation = "@" Ident [ "(" AnnArg { "," AnnArg } ")" ] .
//	AnnArg     = Ident [ Value ] .
//	Option     = Ident Value .
//	Value      = String | Number | "true" | "false" | Ident | "[" [ Value { "," Value } ] "]" .
//
// Keywords are contextual: a field may be called "entity" or "module".
// Comments are "// ...", "/* ... */" and doc comments "/// ...", which attach
// to the declaration, enum value or member that follows them.
package dsl

import (
	"fmt"

	"github.com/patrickluzdev/esquadro/internal/diag"
)

// Kind is the kind of a token.
type Kind int

const (
	EOF Kind = iota
	Newline
	Name
	String
	Number
	DocComment
	LBrace // {
	RBrace // }
	LParen // (
	RParen // )
	LBrack // [
	RBrack // ]
	Comma  // ,
	At     // @
	Gt     // >  many-to-one
	Lt     // <  one-to-many
	Dash   // -  one-to-one
	LtGt   // <> many-to-many
	Dot    // .
	Illegal
)

var kindNames = [...]string{
	EOF:        "end of file",
	Newline:    "newline",
	Name:       "name",
	String:     "string",
	Number:     "number",
	DocComment: "doc comment",
	LBrace:     `"{"`,
	RBrace:     `"}"`,
	LParen:     `"("`,
	RParen:     `")"`,
	LBrack:     `"["`,
	RBrack:     `"]"`,
	Comma:      `","`,
	At:         `"@"`,
	Gt:         `">"`,
	Lt:         `"<"`,
	Dash:       `"-"`,
	LtGt:       `"<>"`,
	Dot:        `"."`,
	Illegal:    "invalid token",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Token is a lexical token. Text is the raw source text; Value is the decoded
// value for strings and the text without "///" for doc comments.
type Token struct {
	Kind  Kind
	Text  string
	Value string
	Pos   diag.Pos
	End   diag.Pos
	// For doc comments: the token before it, see Comment.After.
	after    Kind
	afterEnd int
}

// describe renders a token for error messages, e.g. `name "foo"`.
func (t Token) describe() string {
	switch t.Kind {
	case Name:
		return fmt.Sprintf("name %q", t.Text)
	case String:
		return "string " + t.Text
	case Number:
		return "number " + t.Text
	default:
		return t.Kind.String()
	}
}
