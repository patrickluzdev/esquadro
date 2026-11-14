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
	// Raw is the source text of the token, e.g. "\"a\\n\"" or "`a\n`" for
	// strings, so the formatter can keep the original quoting. Empty for lists.
	Raw  string
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
	// Module is nil when not set. When set more than once, the parser keeps
	// the first and reports the rest.
	Module      *Value
	Blueprints  []*BlueprintRef
	Annotations []*Annotation
	// Items keeps module, blueprints and annotations in source order.
	Items []AppItem
	Span
}

// AppItem is an item of the app block: *ModuleItem, *BlueprintRef or
// *Annotation.
type AppItem interface {
	appItemNode()
	SourceSpan() Span
}

// ModuleItem is the "module <path>" line.
type ModuleItem struct {
	Path Value
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

// RelationKind is the sign of a relation, as in DBML.
type RelationKind int

const (
	// ManyToOne is ">": this entity holds the foreign key.
	ManyToOne RelationKind = iota
	// OneToOne is "-": this entity holds a unique foreign key.
	OneToOne
	// ManyToMany is "<>": a join table.
	ManyToMany
	// OneToMany is "<": the other side of a ">" on the target.
	OneToMany
)

func (k RelationKind) String() string {
	return [...]string{">", "-", "<>", "<"}[k]
}

// Relation is "name > Target (displayField) modifiers... annotations...".
// For "<", Target may name the relation on the other side: "< Order.buyer".
type Relation struct {
	Doc         []string
	Name        Ident
	Kind        RelationKind
	Target      Ident
	TargetField *Ident
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

func (*ModuleItem) appItemNode()   {}
func (*BlueprintRef) appItemNode() {}
func (*Annotation) appItemNode()   {}

func (*Field) memberNode()      {}
func (*Relation) memberNode()   {}
func (*Annotation) memberNode() {}

// BlueprintDecl is the "blueprint" block of a blueprint manifest
// (blueprint.esq).
type BlueprintDecl struct {
	Doc   []string
	Name  Ident
	Items []ManifestItem
	Span
}

// ManifestItem is a line of a blueprint manifest: *VersionItem,
// *DependsItem, *OptionDecl, *AnnotationDecl, *SlotItem, *GoRequireItem or
// *HookItem.
type ManifestItem interface {
	manifestItemNode()
	SourceSpan() Span
}

// VersionItem is `version "1.2.0"`.
type VersionItem struct {
	Version Value
	Span
}

// DependsItem is "requires a, b" (Keyword "requires") or "after a, b"
// (Keyword "after").
type DependsItem struct {
	Keyword Ident
	Names   []Ident
	Span
}

// OptionDecl declares an option of the blueprint: "option name type mods".
type OptionDecl struct {
	Doc       []string
	Name      Ident
	Type      Ident
	Modifiers []*Modifier
	Span
}

// AnnotationDecl declares the keys accepted by the blueprint's annotation on
// one target: "annotation entity { key type mods ... }".
type AnnotationDecl struct {
	Doc    []string
	Target Ident
	Keys   []*AnnotationKeyDecl
	Span
}

// AnnotationKeyDecl is one key of an annotation declaration.
type AnnotationKeyDecl struct {
	Doc       []string
	Name      Ident
	Type      Ident
	Modifiers []*Modifier
	Span
}

// SlotItem declares an extension point that the blueprint's templates open:
// `slot "platform.routes"`.
type SlotItem struct {
	Doc  []string
	Name Value
	Span
}

// GoRequireItem is a Go module the generated project needs:
// `go_require "github.com/jackc/pgx/v5" "v5.7.0"`.
type GoRequireItem struct {
	Module  Value
	Version Value
	Span
}

// HookItem is `hook post-gen "hooks/sqlc" writes "internal/db/sqlc/**"`.
type HookItem struct {
	Event Ident
	Path  Value
	// Writes and Edits are a string or a list of strings; nil when absent.
	Writes *Value
	Edits  *Value
	Span
}

func (*BlueprintDecl) declNode() {}
func (*PackageDecl) declNode()   {}

// PackageDecl is "package <name>": the module the file's entities and enums
// belong to.
type PackageDecl struct {
	Name Ident
	Span
}

func (*VersionItem) manifestItemNode()    {}
func (*DependsItem) manifestItemNode()    {}
func (*OptionDecl) manifestItemNode()     {}
func (*AnnotationDecl) manifestItemNode() {}
func (*SlotItem) manifestItemNode()       {}
func (*GoRequireItem) manifestItemNode()  {}
func (*HookItem) manifestItemNode()       {}
