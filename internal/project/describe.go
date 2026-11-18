package project

// Description is the project as plain data, without pointers between its
// parts: what hooks receive as JSON and what is saved between generations to
// tell what changed. Entities, enums and fields refer to each other by the
// names written in the .esq files.
type Description struct {
	App      AppDescription      `json:"app"`
	Modules  []Names             `json:"modules"`
	Enums    []EnumDescription   `json:"enums"`
	Entities []EntityDescription `json:"entities"`
	Jobs     []JobDescription    `json:"jobs,omitempty"`
	Reports  []ReportDescription `json:"reports,omitempty"`
}

// JobDescription describes a job: its arguments are fields.
type JobDescription struct {
	Names
	Module string             `json:"module"`
	Fields []FieldDescription `json:"fields"`
	Ann    Ann                `json:"ann,omitempty"`
}

// ReportDescription describes a report: the columns of its rows and the
// parameters it takes.
type ReportDescription struct {
	Names
	Module  string             `json:"module"`
	Columns []FieldDescription `json:"columns"`
	Params  []FieldDescription `json:"params"`
	Ann     Ann                `json:"ann,omitempty"`
}

// AppDescription describes the app block.
type AppDescription struct {
	Names
	Module     string                 `json:"module"`
	Currency   string                 `json:"currency,omitempty"`
	Blueprints []BlueprintDescription `json:"blueprints"`
	Ann        Ann                    `json:"ann,omitempty"`
}

// BlueprintDescription is a blueprint the app uses, with its options.
type BlueprintDescription struct {
	Name    string         `json:"name"`
	Options map[string]any `json:"options,omitempty"`
}

// Options returns the options of the blueprint with the given name, or nil.
func (a AppDescription) Options(blueprint string) map[string]any {
	for _, b := range a.Blueprints {
		if b.Name == blueprint {
			return b.Options
		}
	}
	return nil
}

// EnumDescription describes an enum.
type EnumDescription struct {
	Names
	Module string  `json:"module"`
	Values []Names `json:"values"`
}

// EntityDescription describes an entity.
type EntityDescription struct {
	Names
	Module    string                `json:"module"`
	Plural    Names                 `json:"plural"`
	Fields    []FieldDescription    `json:"fields"`
	Relations []RelationDescription `json:"relations"`
	Ann       Ann                   `json:"ann,omitempty"`
}

// FieldDescription describes a field.
type FieldDescription struct {
	Names
	Type     Type     `json:"type"`
	Enum     string   `json:"enum,omitempty"`
	Required bool     `json:"required,omitempty"`
	Unique   bool     `json:"unique,omitempty"`
	Min      *Number  `json:"min,omitempty"`
	Max      *Number  `json:"max,omitempty"`
	Pattern  string   `json:"pattern,omitempty"`
	Default  *Default `json:"default,omitempty"`
	Implicit bool     `json:"implicit,omitempty"`
	Ann      Ann      `json:"ann,omitempty"`
}

// RelationDescription describes one side of a relation.
type RelationDescription struct {
	Names
	Kind      string `json:"kind"`
	Target    string `json:"target"`
	Display   string `json:"display,omitempty"`
	Required  bool   `json:"required,omitempty"`
	Inverse   string `json:"inverse,omitempty"`
	Inferred  bool   `json:"inferred,omitempty"`
	OwnsKey   bool   `json:"owns_key,omitempty"`
	OnDelete  string `json:"on_delete,omitempty"`
	JoinTable string `json:"join_table,omitempty"`
	Ann       Ann    `json:"ann,omitempty"`
}

// Describe returns the project as plain data.
func (p *Project) Describe() Description {
	d := Description{
		App:      AppDescription{Names: p.App.Names, Module: p.App.Module, Currency: p.App.Currency, Ann: p.App.Ann, Blueprints: []BlueprintDescription{}},
		Modules:  []Names{},
		Enums:    []EnumDescription{},
		Entities: []EntityDescription{},
	}
	for _, b := range p.App.Blueprints {
		if b.Name != "" {
			d.App.Blueprints = append(d.App.Blueprints, BlueprintDescription{Name: b.Name, Options: b.Values})
		}
	}
	for _, m := range p.Modules {
		d.Modules = append(d.Modules, m.Names)
	}
	for _, e := range p.Enums {
		ed := EnumDescription{Names: e.Names, Module: moduleName(e.Module), Values: []Names{}}
		for _, v := range e.Values {
			ed.Values = append(ed.Values, v.Names)
		}
		d.Enums = append(d.Enums, ed)
	}
	for _, e := range p.Entities {
		ed := EntityDescription{
			Names: e.Names, Module: moduleName(e.Module), Plural: e.Plural, Ann: e.Ann,
			Fields: []FieldDescription{}, Relations: []RelationDescription{},
		}
		for _, f := range e.Fields {
			ed.Fields = append(ed.Fields, describeField(f))
		}
		for _, r := range e.Relations {
			rd := RelationDescription{
				Names: r.Names, Kind: r.Kind.String(), Target: r.Target.Raw,
				Required: r.Required, Inferred: r.Inferred, JoinTable: r.JoinTable, Ann: r.Ann,
				OwnsKey: r.OwnsForeignKey(), OnDelete: r.OnDelete,
			}
			if r.Display != nil {
				rd.Display = r.Display.Raw
			}
			if r.Inverse != nil {
				rd.Inverse = r.Inverse.Raw
			}
			ed.Relations = append(ed.Relations, rd)
		}
		d.Entities = append(d.Entities, ed)
	}
	for _, j := range p.Jobs {
		jd := JobDescription{Names: j.Names, Module: moduleName(j.Module), Ann: j.Ann, Fields: []FieldDescription{}}
		for _, f := range j.Fields {
			jd.Fields = append(jd.Fields, describeField(f))
		}
		d.Jobs = append(d.Jobs, jd)
	}
	for _, r := range p.Reports {
		rd := ReportDescription{Names: r.Names, Module: moduleName(r.Module), Ann: r.Ann, Columns: []FieldDescription{}, Params: []FieldDescription{}}
		for _, f := range r.Columns {
			rd.Columns = append(rd.Columns, describeField(f))
		}
		for _, f := range r.Params {
			rd.Params = append(rd.Params, describeField(f))
		}
		d.Reports = append(d.Reports, rd)
	}
	return d
}

func describeField(f *Field) FieldDescription {
	fd := FieldDescription{
		Names: f.Names, Type: f.Type, Required: f.Required, Unique: f.Unique,
		Min: f.Min, Max: f.Max, Pattern: f.Pattern, Implicit: f.Implicit, Ann: f.Ann,
	}
	if f.Enum != nil {
		fd.Enum = f.Enum.Raw
	}
	if f.Default != nil {
		def := *f.Default
		def.EnumValue = nil // named by Text already
		fd.Default = &def
	}
	return fd
}

func moduleName(m *Module) string {
	if m == nil {
		return ""
	}
	return m.Raw
}
