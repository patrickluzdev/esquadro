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

// AppDecl is an "app", "platform" or "server" block. An app project has
// one app block; a platform project has one platform block, its servers,
// and an app block for each app it runs, made of settings.
type AppDecl struct {
	// Keyword is "app", "platform" or "server"; empty means "app".
	Keyword string
	Doc     []string
	Name    Ident
	// Module is nil when not set. When set more than once, the parser keeps
	// the first and reports the rest.
	Module      *Value
	Blueprints  []*BlueprintRef
	Annotations []*Annotation
	// Settings are the other "key value" or "key" lines, such as the image
	// of an app a platform runs; which ones a block takes is checked when
	// the project is built.
	Settings []*Setting
	// Items keeps module, blueprints, settings and annotations in source
	// order.
	Items []AppItem
	Span
}

// Kind is the block's keyword: "app", "platform" or "server".
func (d *AppDecl) Kind() string {
	if d.Keyword == "" {
		return "app"
	}
	return d.Keyword
}

// Setting is a "key value" line of a block, or a bare "key", a flag.
type Setting struct {
	Key Ident
	// Value is nil for a flag.
	Value *Value
	Span
}

// AppItem is an item of the app block: *ModuleItem, *BlueprintRef,
// *EnvBlock, *Setting or *Annotation.
type AppItem interface {
	appItemNode()
	SourceSpan() Span
}

// ModuleItem is the "module <path>" line.
type ModuleItem struct {
	Path Value
	Span
}

// BlueprintRef is a "blueprint <name-or-path> [instance] { options }" line
// in the app.
type BlueprintRef struct {
	// Ref is a name (IdentValue) for built-in blueprints or a string for a
	// local path or git source.
	Ref Value
	// Instance is the name of the instance, for a blueprint used more than
	// once (blueprint spa admin); nil otherwise.
	Instance *Ident
	Options  []*Option
	Span
}

// EnumDecl is an "enum" block.
type EnumDecl struct {
	Doc    []string
	Name   Ident
	Values []*EnumValue
	Span
}

// EnumValue is one value of an enum, with the annotations after it.
type EnumValue struct {
	Doc         []string
	Name        Ident
	Annotations []*Annotation
}

// End is where the value ends: after its last annotation.
func (v *EnumValue) End() diag.Pos {
	if n := len(v.Annotations); n > 0 {
		return v.Annotations[n-1].End
	}
	return v.Name.End
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

// EnvDecl declares an environment variable: "env NAME type modifiers", in
// a blueprint manifest or in an app's env block.
type EnvDecl struct {
	Doc       []string
	Name      Ident
	Type      Ident
	Modifiers []*Modifier
	Span
}

// EnvBlock is an app's "env { declarations }", the variables of the app
// itself, or "env <environment> { NAME value ... }", values of variables in
// one environment.
type EnvBlock struct {
	// Environment is nil for a block of declarations.
	Environment *Ident
	Decls       []*EnvDecl
	Values      []*Option
	Span
}

// JobDecl is a "job" block: work to do in the background, whose fields are
// its arguments. It is written as an entity, without options, and its block
// may be left out when it has no arguments. Relations are parsed, to report
// them clearly, but are not valid.
type JobDecl struct {
	EntityDecl
	// Bare is true when the job was written without a block.
	Bare bool
}

// ReportDecl is a "report" block: numbers worked out by SQL of the
// project's own, whose fields are its columns and, marked param, its
// parameters. It is written as an entity, without options; relations are
// parsed, to report them clearly, but are not valid.
type ReportDecl struct {
	EntityDecl
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
func (*JobDecl) declNode()    {}

func (*ModuleItem) appItemNode()   {}
func (*BlueprintRef) appItemNode() {}
func (*Annotation) appItemNode()   {}
func (*EnvBlock) appItemNode()     {}
func (*Setting) appItemNode()      {}

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
// *DependsItem, *OptionDecl, *EnvDecl, *AnnotationDecl, *SlotItem,
// *GoRequireItem or *HookItem.
type ManifestItem interface {
	manifestItemNode()
	SourceSpan() Span
}

// VersionItem is `version "1.2.0"`.
type VersionItem struct {
	Version Value
	Span
}

// DependsItem is "requires a, b" (Keyword "requires"), "optional a, b"
// (Keyword "optional") or "for app, platform" (Keyword "for"): the kinds
// of project the blueprint is made for.
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
func (*EnvDecl) manifestItemNode()        {}
func (*AnnotationDecl) manifestItemNode() {}
func (*SlotItem) manifestItemNode()       {}
func (*GoRequireItem) manifestItemNode()  {}
func (*HookItem) manifestItemNode()       {}
