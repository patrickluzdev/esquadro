package dsl

import "github.com/patrickluzdev/esquadro/internal/diag"

// Span is the source range of a node: [Pos, End).
type Span struct {
	Pos diag.Pos
	End diag.Pos
}

// File is a parsed .esq file.
type File struct {
	Name     string
	Decls    []Decl
	Comments []Comment
}

// Decl is a top-level declaration: *AppDecl, *EnumDecl or *EntityDecl.
type Decl interface {
	declNode()
	SourceSpan() Span
}

// Ident is a name with its position.
type Ident struct {
	Name string
	Span
}

// ValueKind is the kind of a literal value.
type ValueKind int

const (
	StringValue ValueKind = iota
	NumberValue
	BoolValue
	IdentValue
	ListValue
)

// Value is a literal: string, number, bool, name or list.
type Value struct {
	Kind ValueKind
	// Text is the decoded string, the number as written, "true"/"false" or
	// the name. Empty for lists.
	Text string
	List []Value
	Span
}

// Option is a "key value" pair, used in blueprint option blocks and entity
// options.
type Option struct {
	Key   Ident
	Value Value
	Span
}

// Annotation is "@name(key value, flag, ...)", used to pass blueprint-specific
// settings on apps, entities and fields.
type Annotation struct {
	Name Ident
	Args []AnnotationArg
	Span
}

// AnnotationArg is a key with an optional value. A key without a value is a
// flag.
type AnnotationArg struct {
	Key   Ident
	Value *Value
	Span
}

// AppDecl is the "app" block.
type AppDecl struct {
	Doc  []string
	Name Ident
	// Module and Lang are nil when not set. When set more than once, the
	// parser keeps the first and reports the rest.
	Module      *Value
	Lang        *Ident
	Blueprints  []*BlueprintRef
	Annotations []*Annotation
	Span
}

// BlueprintRef is a "blueprint <name-or-path> { options }" line in the app.
type BlueprintRef struct {
	// Ref is a name (IdentValue) for built-in blueprints or a string for a
	// local path or git source.
	Ref     Value
	Options []*Option
	Span
}

// EnumDecl is an "enum" block.
type EnumDecl struct {
	Doc    []string
	Name   Ident
	Values []*EnumValue
	Span
}

// EnumValue is one value of an enum.
type EnumValue struct {
	Doc  []string
	Name Ident
}

// EntityDecl is an "entity" block.
type EntityDecl struct {
	Doc         []string
	Name        Ident
	Options     []*Option
	Fields      []*Field
	Relations   []*Relation
	Annotations []*Annotation
	// Members keeps fields, relations and annotations in source order.
	Members []Member
	Span
}

// Member is a member of an entity: *Field, *Relation or *Annotation.
type Member interface {
	memberNode()
}

// Field is "name type modifiers... annotations...".
type Field struct {
	Doc         []string
	Name        Ident
	Type        Ident
	Modifiers   []*Modifier
	Annotations []*Annotation
	Span
}

// RelationKind is the arrow used by a relation.
type RelationKind int

const (
	// ManyToOne is "->": a foreign key on this entity. Becomes one-to-one with
	// the unique modifier.
	ManyToOne RelationKind = iota
	// ManyToMany is "<->": a join table.
	ManyToMany
)

func (k RelationKind) String() string {
	if k == ManyToMany {
		return "<->"
	}
	return "->"
}

// Relation is "name -> Target (displayField) modifiers... annotations...".
type Relation struct {
	Doc         []string
	Name        Ident
	Kind        RelationKind
	Target      Ident
	Display     *Ident
	Modifiers   []*Modifier
	Annotations []*Annotation
	Span
}

// Modifier is a validation or flag on a field or relation, e.g. required,
// min(3), default(ACTIVE).
type Modifier struct {
	Name Ident
	Args []Value
	// HasParens records whether "()" was written, so min and min() differ.
	HasParens bool
	Span
}

// SourceSpan returns the node's source range.
func (s Span) SourceSpan() Span { return s }

func (*AppDecl) declNode()    {}
func (*EnumDecl) declNode()   {}
func (*EntityDecl) declNode() {}

func (*Field) memberNode()      {}
func (*Relation) memberNode()   {}
func (*Annotation) memberNode() {}
