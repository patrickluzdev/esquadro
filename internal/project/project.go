// Package project turns a parsed .esq file into a validated, fully resolved
// project model: types and relation targets are linked, inverse relations
// are inferred, implicit fields are added and every name has all the
// spellings templates need. Blueprints only ever see this model.
package project

import (
	"fmt"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
)

// Project is the resolved model of the .esq files of a project.
type Project struct {
	Files []string
	// Kind is app or platform. In a platform, App is the platform block and
	// Servers and Hosted say where and what it runs; an app's declarations
	// are empty.
	Kind     Kind
	App      *App
	Servers  []*Server
	Hosted   []*Hosted
	Modules  []*Module
	Enums    []*Enum
	Entities []*Entity
	Jobs     []*Job
	Reports  []*Report
}

// Module is a part of the domain: the entities and enums of the files that
// declare "package <name>". It becomes one Go package.
type Module struct {
	Names
	Entities []*Entity
	Enums    []*Enum
	Jobs     []*Job
	Reports  []*Report
	Pos      diag.Pos
}

// Module returns the module with the given name, or nil.
func (p *Project) Module(name string) *Module {
	for _, m := range p.Modules {
		if m.Raw == name {
			return m
		}
	}
	return nil
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
	Doc    []string
	Module string
	// Currency is the app's currency (ISO 4217: BRL), the one of its money
	// unless a value says another; empty when the app sets none.
	Currency   string
	Blueprints []*BlueprintRef
	// Annotations are validated later against the blueprints' schemas.
	Annotations []*dsl.Annotation
	Ann         Ann
	// Env are the app's own environment variables, and EnvValues the values
	// it sets per environment, checked when blueprints are resolved.
	Env       []*EnvVar
	EnvValues []EnvValue
	Pos       diag.Pos
}

// SourceKind says where a blueprint comes from.
type SourceKind int

const (
	SourceBuiltin SourceKind = iota // blueprint app
	SourceLocal                     // blueprint "./blueprints/mine"
	SourceGit                       // blueprint "github.com/acme/x@v1.2.0"
)

// BlueprintRef is a blueprint used by the app. Options are kept as written;
// they are validated later against the blueprint's manifest.
type BlueprintRef struct {
	Ref string
	// Instance is the instance's name, for a blueprint used by name
	// (blueprint spa admin): it stands for the blueprint in options,
	// annotations (@admin) and templates. Empty otherwise.
	Instance     string
	InstanceSpan dsl.Span
	Source       SourceKind
	Options      []*dsl.Option
	Pos          diag.Pos
	// RefSpan is the span of the name or path, for error messages.
	RefSpan dsl.Span
	// Name and Values are filled when blueprints are resolved: the name the
	// manifest declares and the options with defaults.
	Name   string
	Values map[string]any
}

// Ann holds validated annotation values: blueprint name, then key, then
// value (string, int64, float64, bool, []string or []any). It is filled when
// blueprints are resolved; a blueprint's entry exists only when its
// annotation was written on that node.
type Ann map[string]map[string]any

// Enum is an enum type.
type Enum struct {
	Names
	Module *Module
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
	// Annotations are validated later against the blueprints' schemas.
	Annotations []*dsl.Annotation
	Ann         Ann
	Pos         diag.Pos
}

// Entity is a resolved entity.
type Entity struct {
	Names
	Module *Module
	Plural Names
	Doc    []string
	// Fields includes the implicit id, version, created_at and updated_at, in that
	// order around the declared fields.
	Fields []*Field
	// Relations includes both declared relations and the inverse side of
	// relations declared on other entities (Inferred is true for those).
	Relations   []*Relation
	Annotations []*dsl.Annotation
	Ann         Ann
	Pos         diag.Pos
}

// Job is work done in the background, declared with "job": its fields are
// its arguments, as the queue stores them.
type Job struct {
	Names
	Module      *Module
	Doc         []string
	Fields      []*Field
	Annotations []*dsl.Annotation
	Ann         Ann
	Pos         diag.Pos
}

// Report is numbers the project's own SQL works out: the columns of its
// rows and the parameters a request passes it, each with a model type.
type Report struct {
	Names
	Module      *Module
	Doc         []string
	Columns     []*Field
	Params      []*Field
	Annotations []*dsl.Annotation
	Ann         Ann
	Pos         diag.Pos
}

// Field returns the job's argument with the given name as written, or nil.
func (j *Job) Field(name string) *Field {
	for _, f := range j.Fields {
		if f.Raw == name {
			return f
		}
	}
	return nil
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
	// TypeMoney is an exact amount with its currency (ISO 4217): stored as
	// both, the currency the app's by default.
	TypeMoney Type = "money"
	// TypeFile and TypeImage are a file in the storage blueprint's store,
	// any kind or an image; the record keeps its id.
	TypeFile  Type = "file"
	TypeImage Type = "image"
	TypeBool  Type = "bool"
	TypeTime  Type = "time"
	TypeDate  Type = "date"
	TypeUUID  Type = "uuid"
	TypeJSON  Type = "json"
	TypeEnum  Type = "enum"
)

var builtinTypes = []Type{
	TypeString, TypeText, TypeInt, TypeInt64, TypeDecimal, TypeMoney,
	TypeFile, TypeImage, TypeBool, TypeTime, TypeDate, TypeUUID, TypeJSON,
}

// IsFile reports whether a field holds a file (file or image).
func (f *Field) IsFile() bool { return f.Type == TypeFile || f.Type == TypeImage }

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
	// Implicit is true for id, version, created_at and updated_at.
	Implicit    bool
	Annotations []*dsl.Annotation
	Ann         Ann
	Pos         diag.Pos
}

// Number is a numeric literal as written plus its value.
type Number struct {
	Text  string  `json:"text"`
	Value float64 `json:"value"`
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
	Kind DefaultKind `json:"kind"`
	// Text is the literal (decoded string, number as written, true/false),
	// "now", or the enum value name as written.
	Text      string     `json:"text"`
	EnumValue *EnumValue `json:"-"`
}

var defaultKindNames = [...]string{"literal", "now", "enum"}

// MarshalText writes the kind as a word, so descriptions read well.
func (k DefaultKind) MarshalText() ([]byte, error) {
	if int(k) < len(defaultKindNames) {
		return []byte(defaultKindNames[k]), nil
	}
	return nil, fmt.Errorf("unknown default kind %d", int(k))
}

// UnmarshalText reads a kind written by MarshalText.
func (k *DefaultKind) UnmarshalText(b []byte) error {
	for i, n := range defaultKindNames {
		if n == string(b) {
			*k = DefaultKind(i)
			return nil
		}
	}
	return fmt.Errorf("unknown default kind %q", b)
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
	JoinTable string
	// OnDelete is what deleting the target does to this side's rows:
	// "cascade", "restrict" or "set_null"; empty for the default (restrict
	// when required, set_null otherwise). Only on the side that owns the key.
	OnDelete    string
	Annotations []*dsl.Annotation
	Ann         Ann
	Pos         diag.Pos
}

// CrossesModules reports whether the relation points to an entity of another
// module. Such relations have no inverse: a module does not know who points
// at it.
func (r *Relation) CrossesModules() bool {
	return r.Entity.Module != r.Target.Module
}

// IDs names the ids a many-to-many relation is written with: TagIDs and
// tag_ids for tags of Tag, and the relation's own name otherwise (for
// related of Product: RelatedIDs, related_ids).
func (r *Relation) IDs() Names {
	base := r.Names
	if r.Target != nil && r.Snake == r.Target.Plural.Snake {
		base = r.Target.Names
	}
	return Names{
		Raw: base.Raw + "IDs", Label: base.Label + " IDs", Pascal: base.Pascal + "IDs",
		Camel: base.Camel + "IDs", Snake: base.Snake + "_ids", Kebab: base.Kebab + "-ids",
	}
}

// OwnsForeignKey reports whether this side stores the foreign key column:
// the many side of a many-to-one, and the declared side of a one-to-one.
func (r *Relation) OwnsForeignKey() bool {
	return r.Kind == ManyToOne || (r.Kind == OneToOne && !r.Inferred)
}
