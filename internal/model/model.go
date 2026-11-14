// Package model turns a parsed .esq file into a validated, fully resolved
// project model: types and relation targets are linked, inverse relations
// are inferred, implicit fields are added and every name has all the
// spellings templates need. Blueprints only ever see this model.
package model

import (
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
)

// Project is the resolved model of a .esq file.
type Project struct {
	File     string
	App      *App
	Enums    []*Enum
	Entities []*Entity
}

// Entity returns the entity with the given name as written, or nil.
func (p *Project) Entity(name string) *Entity {
	for _, e := range p.Entities {
		if e.Raw == name {
			return e
		}
	}
	return nil
}

// Enum returns the enum with the given name as written, or nil.
func (p *Project) Enum(name string) *Enum {
	for _, e := range p.Enums {
		if e.Raw == name {
			return e
		}
	}
	return nil
}

// App is the "app" block.
type App struct {
	Names
	Doc        []string
	Module     string
	Blueprints []*BlueprintRef
	// Annotations are validated later against the blueprints' schemas.
	Annotations []*dsl.Annotation
	Pos         diag.Pos
}

// SourceKind says where a blueprint comes from.
type SourceKind int

const (
	SourceBuiltin SourceKind = iota // blueprint base
	SourceLocal                     // blueprint "./blueprints/mine"
	SourceGit                       // blueprint "github.com/acme/x@v1.2.0"
)

// BlueprintRef is a blueprint used by the app. Options are kept as written;
// they are validated later against the blueprint's manifest.
type BlueprintRef struct {
	Ref     string
	Source  SourceKind
	Options []*dsl.Option
	Pos     diag.Pos
}

// Enum is an enum type.
type Enum struct {
	Names
	Doc    []string
	Values []*EnumValue
	Pos    diag.Pos
}

// Value returns the value with the given name as written, or nil.
func (e *Enum) Value(name string) *EnumValue {
	for _, v := range e.Values {
		if v.Raw == name {
			return v
		}
	}
	return nil
}

// EnumValue is one value of an enum.
type EnumValue struct {
	Names
	Doc []string
}

// Entity is a resolved entity.
type Entity struct {
	Names
	Plural Names
	Doc    []string
	// Fields includes the implicit id, created_at and updated_at, in that
	// order around the declared fields.
	Fields []*Field
	// Relations includes both declared relations and the inverse side of
	// relations declared on other entities (Inferred is true for those).
	Relations   []*Relation
	Annotations []*dsl.Annotation
	Pos         diag.Pos
}

// Field returns the field with the given name as written, or nil.
func (e *Entity) Field(name string) *Field {
	for _, f := range e.Fields {
		if f.Raw == name {
			return f
		}
	}
	return nil
}

// Type is the type of a field.
type Type string

const (
	TypeString  Type = "string"
	TypeText    Type = "text"
	TypeInt     Type = "int"
	TypeInt64   Type = "int64"
	TypeDecimal Type = "decimal"
	TypeBool    Type = "bool"
	TypeTime    Type = "time"
	TypeDate    Type = "date"
	TypeUUID    Type = "uuid"
	TypeJSON    Type = "json"
	TypeEnum    Type = "enum"
)

var builtinTypes = []Type{
	TypeString, TypeText, TypeInt, TypeInt64, TypeDecimal,
	TypeBool, TypeTime, TypeDate, TypeUUID, TypeJSON,
}

// Field is a resolved field.
type Field struct {
	Names
	Doc  []string
	Type Type
	// Enum is set when Type is TypeEnum.
	Enum     *Enum
	Required bool
	Unique   bool
	// Min and Max are lengths for string/text and values for numbers.
	Min, Max *Number
	// Pattern is a Go (RE2) regular expression, empty when not set.
	Pattern string
	Default *Default
	// Implicit is true for id, created_at and updated_at.
	Implicit    bool
	Annotations []*dsl.Annotation
	Pos         diag.Pos
}

// Number is a numeric literal as written plus its value.
type Number struct {
	Text  string
	Value float64
}

// DefaultKind is the kind of a default value.
type DefaultKind int

const (
	DefaultLiteral DefaultKind = iota // a string, number or bool literal
	DefaultNow                        // now, for time and date fields
	DefaultEnum                       // an enum value
)

// Default is a field's default value.
type Default struct {
	Kind DefaultKind
	// Text is the literal (decoded string, number as written, true/false),
	// "now", or the enum value name as written.
	Text      string
	EnumValue *EnumValue
}

// RelationKind is the kind of a relation, seen from the entity that has it.
type RelationKind int

const (
	ManyToOne  RelationKind = iota // declared with ->; foreign key on this entity
	OneToOne                       // declared with -> unique, or its inverse
	OneToMany                      // inverse of ManyToOne
	ManyToMany                     // declared with <->, or its inverse
)

func (k RelationKind) String() string {
	return [...]string{"many-to-one", "one-to-one", "one-to-many", "many-to-many"}[k]
}

// Relation is one side of a relation between two entities.
type Relation struct {
	Names
	Doc    []string
	Kind   RelationKind
	Entity *Entity // the entity this side belongs to
	Target *Entity
	// Display is the target field shown when listing or picking the target,
	// e.g. Category.name.
	Display *Field
	// Required means the foreign key cannot be null. Only on the declared
	// side of ManyToOne and OneToOne.
	Required bool
	Inverse  *Relation
	// Inferred is true for the side that was not written in the file.
	Inferred bool
	// JoinTable is the snake_case join table name for ManyToMany, the same on
	// both sides.
	JoinTable   string
	Annotations []*dsl.Annotation
	Pos         diag.Pos
}

// OwnsForeignKey reports whether this side stores the foreign key column.
func (r *Relation) OwnsForeignKey() bool {
	return !r.Inferred && (r.Kind == ManyToOne || r.Kind == OneToOne)
}
