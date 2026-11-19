// Package rest holds the Go code of the built-in rest blueprint: helpers for
// its templates, which write the OpenAPI contract and the code between the
// contract and each module, and its ogen hook.
package rest

import (
	"fmt"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"github.com/patrickluzdev/esquadro/blueprints/audit"
	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the helpers the api blueprint's templates use.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"jsonName":            jsonName,
		"paginationOf":        PaginationOf,
		"reportSchema":        ReportSchema,
		"reportIn":            ReportIn,
		"reportOut":           ReportOut,
		"reportRows":          ReportRows,
		"reportPublic":        ReportPublic,
		"reportSampleQuery":   ReportSampleQuery,
		"ifMatchRequired":     IfMatchRequired,
		"idempotencyRequired": IdempotencyRequired,
		"unpagedMax":          UnpagedMax,
		"apiFilters":          APIFilters,
		"sortsOf":             SortsOf,
		"searchFieldsOf":      store.SearchFields,
		"has":                 has,
		"accessLine":          accessLine,
		// policies reports whether the API asks the entities' policies:
		// when something signs in.
		"policies":     func(entities []*project.Entity) bool { return len(security.RealmsOf(entities)) > 0 },
		"keyInRequest": KeyInRequest,
		"whosOf":       security.WhosOf,
		// ownerField is the Go field of the owner of e's records: its
		// relation's key, or ID in a principal; empty without.
		"ownerField": func(entities []*project.Entity, e *project.Entity) string {
			o, ok := security.OwnerUsed(entities, e)
			switch {
			case !ok:
				return ""
			case o.Relation == nil:
				return "ID"
			}
			return o.Relation.Pascal + "ID"
		},
		// declaredLinks are e's many-to-many relations it links and unlinks.
		"declaredLinks": func(e *project.Entity) bool {
			for _, r := range e.Relations {
				if r.Kind == project.ManyToMany && !r.Inferred {
					return true
				}
			}
			return false
		},
		// ownerColumn reports whether e lists by owner.
		"ownerColumn": func(entities []*project.Entity, e *project.Entity) bool {
			_, ok := security.OwnerUsed(entities, e)
			return ok
		},
		// publicRead reports whether anyone may read e, signed in or not.
		"publicRead": func(entities []*project.Entity, e *project.Entity) bool {
			return slices.Contains(security.Access(entities, e, "read"), "public")
		},
		// claims reports whether e's policy gives new and updated records
		// their owner (Claim).
		"claims": func(entities []*project.Entity, e *project.Entity) bool {
			o, ok := security.OwnerUsed(entities, e)
			return ok && o.Relation != nil
		},
		"keyName":  KeyName,
		"ogenName": ogenName,
		"schema":   schema,
		"hasFiles": func(entities []*project.Entity) bool {
			for _, e := range entities {
				for _, f := range e.Fields {
					if f.IsFile() {
						return true
					}
				}
			}
			return false
		},
		"changeSchema":     changeSchema,
		"writeSchema":      writeSchema,
		"shortable":        shortable,
		"clearable":        Clearable,
		"includable":       Includable,
		"includedFrom":     includedFrom,
		"answerFields":     AnswerFields,
		"enumName":         enumName,
		"inRequest":        InRequest,
		"keys":             Keys,
		"needsType":        needsType,
		"apiFieldNames":    apiFieldNames,
		"canSampleAPI":     canSampleAPI,
		"singleAPI":        singleAPI,
		"apiParents":       apiParents,
		"apiSample":        apiSample,
		"apiSampleImports": apiSampleImports,
		"modulePaths":      modulePaths,
		"softDeleted":      store.SoftDeleted,
		"single":           store.Single,
		"idLinks":          store.IDLinks,
		"html":             HTML,
		"anyHTML": func(entities []*project.Entity) bool {
			for _, e := range entities {
				if slices.ContainsFunc(e.Fields, HTML) {
					return true
				}
			}
			return false
		},
		"audited":     audit.Audited,
		"auditedOf":   audit.AuditedOf,
		"auditAdmins": audit.Admins,
		"links": func(e *project.Entity) []*project.Relation {
			var out []*project.Relation
			for _, r := range e.Relations {
				if r.Kind == project.ManyToMany {
					out = append(out, r)
				}
			}
			return out
		},
		"linkParam": func(r *project.Relation) string {
			if r.Target == r.Entity {
				return "otherId"
			}
			return r.Target.Camel + "Id"
		},
		"linkTests": linkTests,
	}
}

// jsonName is a field's name in the API's JSON: snake_case.
func jsonName(f *project.Field) string { return f.Snake }

// KeyName is the name in the API's JSON of the key of a relation, the id of
// its record: category_id.
func KeyName(r *project.Relation) string { return r.Snake + "_id" }

// ogenInitialisms are the names ogen writes in capitals when they are the
// whole name or its first word ("id" is ID, but "productId" is ProductId).
var ogenInitialisms = []string{"id", "url", "uri", "api", "http", "https", "json", "xml", "uuid", "ip", "sql", "html"}

// ogenName is the Go name ogen gives a property or parameter: each word of
// a snake_case name capitalized, an initialism in capitals (category_id is
// CategoryID); in a camelCase one, only a first word that is an
// initialism.
func ogenName(name string) string {
	if strings.Contains(name, "_") {
		var b strings.Builder
		for word := range strings.SplitSeq(name, "_") {
			if word == "" {
				continue
			}
			if slices.Contains(ogenInitialisms, word) {
				b.WriteString(strings.ToUpper(word))
				continue
			}
			b.WriteString(strings.ToUpper(word[:1]))
			b.WriteString(word[1:])
		}
		return b.String()
	}
	end := len(name)
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			end = i
			break
		}
	}
	if first := name[:end]; slices.Contains(ogenInitialisms, first) {
		return strings.ToUpper(first) + name[end:]
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// enumName is an enum's schema in the contract: prefixed with its module,
// since two modules may declare an enum with the same name.
func enumName(e *project.Enum) string { return e.Module.Pascal + e.Pascal }

// schema writes a field's schema as a YAML flow mapping. Only types:
// the domain checks the rules and answers invalid_fields with every field at
// once, so the contract does not repeat them.
func schema(f *project.Field) string {
	switch f.Type {
	case project.TypeInt:
		return "{type: integer}"
	case project.TypeInt64:
		return "{type: integer, format: int64}"
	case project.TypeDecimal:
		return `{type: string, description: 'A decimal number written as a string, e.g. "19.90".'}`
	case project.TypeBool:
		return "{type: boolean}"
	case project.TypeTime:
		return "{type: string, format: date-time}"
	case project.TypeDate:
		return "{type: string, format: date}"
	case project.TypeUUID:
		return "{type: string, format: uuid}"
	case project.TypeJSON:
		return "{description: Any JSON value.}"
	case project.TypeEnum:
		return fmt.Sprintf("{$ref: \"#/components/schemas/%s\"}", enumName(f.Enum))
	case project.TypeMoney:
		return `{$ref: "#/components/schemas/Money"}`
	case project.TypeFile, project.TypeImage:
		return `{$ref: "#/components/schemas/StoredFile"}`
	}
	return "{type: string}"
}

// ruleMark ends each line of a model rule in the contract: the document
// ogen generates the server from leaves them out, so that the domain
// checks them and refuses with invalid_fields, every field at once.
const ruleMark = " # model rule"

// PortablePattern reports whether a Go pattern means the same in the
// ECMAScript of OpenAPI and JavaScript clients.
func PortablePattern(p string) bool {
	for _, s := range []string{"(?", `\A`, `\z`, `\p{`, `\P{`, `\Q`, "[[:", `\C`} {
		if strings.Contains(p, s) {
			return false
		}
	}
	return true
}

// rules are the lines of f's model rules in a write body's schema, at the
// given indent, each ending with ruleMark.
func rules(f *project.Field, indent int) []string {
	pad := strings.Repeat(" ", indent)
	var out []string
	line := func(format string, args ...any) { out = append(out, pad+fmt.Sprintf(format, args...)+ruleMark) }
	switch f.Type {
	case project.TypeString, project.TypeText:
		if f.Min != nil {
			line("minLength: %s", f.Min.Text)
		}
		if f.Max != nil {
			line("maxLength: %s", f.Max.Text)
		}
		if f.Pattern != "" && PortablePattern(f.Pattern) {
			line("pattern: '%s'", strings.ReplaceAll(f.Pattern, "'", "''"))
		}
	case project.TypeInt, project.TypeInt64:
		if f.Min != nil {
			line("minimum: %s", f.Min.Text)
		}
		if f.Max != nil {
			line("maximum: %s", f.Max.Text)
		}
	case project.TypeDecimal:
		if f.Min != nil {
			line("x-minimum: %q", f.Min.Text)
		}
		if f.Max != nil {
			line("x-maximum: %q", f.Max.Text)
		}
	}
	return out
}

// writeSchema is f's schema in a write body (create, PUT, or PATCH when
// changes): with its model rules, in block style when it has some. Rules
// apply only to texts and numbers, never to an enum's $ref.
func writeSchema(f *project.Field, changes bool) string {
	if f.IsFile() {
		// Written as the id of a file sent first, read as the file.
		s := "{type: string, format: uuid, description: 'The id of a file sent by POST /storage/uploads.'"
		if changes && Clearable(f) {
			s += ", nullable: true"
		}
		return s + "}"
	}
	rs := rules(f, 10)
	if len(rs) == 0 {
		if changes {
			return changeSchema(f)
		}
		return schema(f)
	}
	const pad = "          "
	var lines []string
	switch f.Type {
	case project.TypeInt:
		lines = append(lines, pad+"type: integer")
	case project.TypeInt64:
		lines = append(lines, pad+"type: integer", pad+"format: int64")
	default:
		lines = append(lines, pad+"type: string")
	}
	if f.Type == project.TypeDecimal {
		var bounds []string
		if f.Min != nil {
			bounds = append(bounds, "at least "+f.Min.Text)
		}
		if f.Max != nil {
			bounds = append(bounds, "at most "+f.Max.Text)
		}
		lines = append(lines, pad+`description: 'A decimal number written as a string, e.g. "19.90"; `+strings.Join(bounds, " and ")+".'")
	}
	if changes && Clearable(f) {
		lines = append(lines, pad+"nullable: true")
	}
	return "\n" + strings.Join(append(lines, rs...), "\n")
}

// shortable is the first text field of e with a minimum length of 2 or
// more, which "x" breaks; nil when there is none.
func shortable(e *project.Entity) *project.Field {
	for _, f := range InRequest(e) {
		if (f.Type == project.TypeString || f.Type == project.TypeText) && f.Min != nil && f.Min.Value >= 2 && f.Pattern == "" {
			return f
		}
	}
	return nil
}

// Clearable reports whether a PATCH may send f as null: a field without a
// default, which null empties (or, when required, is refused as such).
func Clearable(f *project.Field) bool { return f.Default == nil && f.Type != project.TypeJSON }

// changeSchema is f's schema in a PATCH body: null for a clearable one.
func changeSchema(f *project.Field) string {
	if !Clearable(f) {
		return schema(f)
	}
	if f.Type == project.TypeEnum {
		values := make([]string, len(f.Enum.Values))
		for i, v := range f.Enum.Values {
			values[i] = fmt.Sprintf("%q", v.Raw)
		}
		return fmt.Sprintf("{type: string, enum: [%s], nullable: true}", strings.Join(values, ", "))
	}
	if f.Type == project.TypeMoney {
		// nullable beside a $ref is ignored: the object, written out.
		return "{type: object, required: [amount], properties: {amount: {type: string}, currency: {type: string}}, nullable: true}"
	}
	s := schema(f)
	return strings.TrimSuffix(s, "}") + ", nullable: true}"
}

// Includable lists the relations a record's answer may include: those whose
// key it holds, and the many-to-many ones it declares.
func Includable(e *project.Entity) []*project.Relation {
	var out []*project.Relation
	for _, r := range e.Relations {
		if r.OwnsForeignKey() || (r.Kind == project.ManyToMany && !r.Inferred) {
			out = append(out, r)
		}
	}
	return out
}

// includedFrom lists, once each, the entities of other modules that m's
// records may include.
func includedFrom(m *project.Module) []*project.Entity {
	var out []*project.Entity
	for _, e := range m.Entities {
		for _, r := range Includable(e) {
			if r.Target.Module != m && !slices.Contains(out, r.Target) {
				out = append(out, r.Target)
			}
		}
	}
	return out
}

// AnswerFields are the names of a record's answer that fields may ask for:
// all but id and version, which always come.
func AnswerFields(e *project.Entity) []string {
	var out []string
	for _, f := range InRequest(e) {
		out = append(out, jsonName(f))
	}
	for _, r := range Keys(e) {
		out = append(out, KeyName(r))
	}
	return append(out, "created_at", "updated_at")
}

// InRequest lists the fields a NewX carries: the declared ones.
func InRequest(e *project.Entity) []*project.Field {
	var out []*project.Field
	for _, f := range e.Fields {
		if !f.Implicit {
			out = append(out, f)
		}
	}
	return out
}

// Keys lists the relations whose key an entity holds.
func Keys(e *project.Entity) []*project.Relation {
	var out []*project.Relation
	for _, r := range e.Relations {
		if r.OwnsForeignKey() {
			out = append(out, r)
		}
	}
	return out
}

// needsType reports whether a module has a field of the given type, for the
// imports of its HTTP code.
func needsType(m *project.Module, t string) bool {
	for _, e := range m.Entities {
		for _, f := range e.Fields {
			if string(f.Type) == t {
				return true
			}
		}
		// A money field's range filter reads decimals.
		if t == "decimal" {
			for _, f := range APIFilters(e) {
				if f.Go == "decimal.Decimal" {
					return true
				}
			}
		}
	}
	return false
}

// FieldName pairs a field's name in the domain's errors with its name in
// the contract.
type FieldName struct{ Domain, API string }

// apiFieldNames lists, once each, how the fields of a module's entities are
// named in the contract, so refusals name what the client sent.
func apiFieldNames(m *project.Module) []FieldName {
	seen := map[string]bool{}
	var out []FieldName
	add := func(domain, api string) {
		if !seen[domain] {
			seen[domain] = true
			out = append(out, FieldName{domain, api})
		}
	}
	for _, e := range m.Entities {
		for _, f := range InRequest(e) {
			add(f.Snake, jsonName(f))
		}
		for _, r := range Keys(e) {
			add(r.Snake, KeyName(r))
		}
	}
	slices.SortFunc(out, func(a, b FieldName) int { return strings.Compare(a.Domain, b.Domain) })
	return out
}

// ---- test data ----

// canSampleAPI reports whether esquadro can write a valid request to
// create a record of an entity: not when a required field has a pattern,
// nor when a required relation of its module needs such a record or loops
// back, nor for a single entity, which has no create.
func canSampleAPI(e *project.Entity) bool {
	return !store.Single(e) && sampleableAPI(e, map[*project.Entity]bool{})
}

// singleAPI reports whether an entity is single and esquadro can write a
// valid request to change it, for its test.
func singleAPI(e *project.Entity) bool {
	return store.Single(e) && sampleableAPI(e, map[*project.Entity]bool{})
}

func sampleableAPI(e *project.Entity, visiting map[*project.Entity]bool) bool {
	if visiting[e] {
		return false
	}
	visiting[e] = true
	defer delete(visiting, e)
	for _, f := range e.Fields {
		if f.Required && f.Pattern != "" {
			return false
		}
	}
	for _, r := range apiParents(e) {
		if !sampleableAPI(r.Target, visiting) {
			return false
		}
	}
	return true
}

// apiParents are the required relations to records of the same module,
// which a test creates first through the API.
func apiParents(e *project.Entity) []*project.Relation {
	var out []*project.Relation
	for _, r := range Keys(e) {
		if r.Required && r.Target.Module == e.Module {
			out = append(out, r)
		}
	}
	return out
}

// apiSample is a Go expression for a valid value of a required field in a
// request of the contract, with i telling records apart.
func apiSample(f *project.Field, i string) string {
	switch f.Type {
	case project.TypeString, project.TypeText:
		word := f.Snake
		digits := 1
		if f.Min != nil {
			digits = max(digits, int(f.Min.Value)-len(word)-1)
		}
		if f.Max != nil && float64(len(word)+1+digits) > f.Max.Value {
			n := int(f.Max.Value)
			if f.Min != nil {
				n = int(f.Min.Value)
			}
			return fmt.Sprintf("strings.Repeat(\"x\", %d)", max(n, 1))
		}
		return fmt.Sprintf("fmt.Sprintf(\"%s %%0%dd\", %s)", word, digits, i)
	case project.TypeUUID:
		return "ogenuuid.UUID(uuid.NewV7())"
	case project.TypeInt, project.TypeInt64, project.TypeDecimal:
		n := "1"
		if f.Min != nil {
			n = f.Min.Text
		} else if f.Max != nil && f.Max.Value < 1 {
			n = f.Max.Text
		}
		if f.Type == project.TypeDecimal {
			return `"` + n + `"`
		}
		return n
	case project.TypeMoney:
		n := "1"
		if f.Min != nil {
			n = f.Min.Text
		} else if f.Max != nil && f.Max.Value < 1 {
			n = f.Max.Text
		}
		return `apigen.Money{Amount: "` + n + `"}`
	case project.TypeBool:
		return "true"
	case project.TypeTime, project.TypeDate:
		return "time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)"
	case project.TypeJSON:
		return "jx.Raw(`{}`)"
	case project.TypeEnum:
		return "apigen." + enumName(f.Enum) + "(" + fmt.Sprintf("%q", f.Enum.Values[0].Raw) + ")"
	}
	return `""`
}

// apiSampleImports lists the imports the request samples of every entity
// need.
func apiSampleImports(entities []*project.Entity) []string {
	var out []string
	add := func(p string) {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	for _, e := range entities {
		if !canSampleAPI(e) {
			continue
		}
		for _, r := range Keys(e) {
			if r.Required && r.Target.Module != e.Module {
				add("uuid")
				add("ogenuuid github.com/google/uuid")
			}
		}
		for _, f := range InRequest(e) {
			if !f.Required {
				continue
			}
			s := apiSample(f, "i")
			for prefix, imp := range map[string]string{
				"fmt.": "fmt", "strings.": "strings", "uuid.": "uuid", "ogenuuid.": "ogenuuid github.com/google/uuid",
				"time.": "time", "jx.": "github.com/go-faster/jx",
			} {
				if strings.Contains(s, prefix) {
					add(imp)
				}
			}
		}
	}
	return out
}

// linkTests reports whether the API tests have a many-to-many relation to
// test: a declared one whose records esquadro can write.
func linkTests(entities []*project.Entity) bool {
	for _, e := range entities {
		if !canSampleAPI(e) {
			continue
		}
		for _, r := range e.Relations {
			if r.Kind == project.ManyToMany && !r.Inferred && (r.CrossesModules() || canSampleAPI(r.Target)) {
				return true
			}
		}
	}
	return false
}

// modulePaths lists the import paths of the modules with entities, and
// with reports too when reports.
func modulePaths(appModule string, modules []*project.Module, reports ...bool) []string {
	var out []string
	for _, m := range modules {
		if len(m.Entities) > 0 || (len(reports) > 0 && reports[0] && len(m.Reports) > 0) {
			out = append(out, appModule+"/internal/"+m.Snake)
		}
	}
	return out
}

// accessLine is who may do op on e, for the operation's description; empty
// when nothing signs in.
func accessLine(entities []*project.Entity, e *project.Entity, op string) string {
	rules := security.Access(entities, e, op)
	if rules == nil {
		return ""
	}
	return "Who may: " + security.AccessText(rules) + "."
}

// KeyInRequest reports whether a request that creates or replaces an e must
// carry r's key: a required relation, unless it is the owner's and create
// lets the owner in, who is then the record's owner.
func KeyInRequest(entities []*project.Entity, e *project.Entity, r *project.Relation) bool {
	if !r.Required {
		return false
	}
	o, ok := security.OwnerUsed(entities, e)
	return !(ok && o.Relation == r && security.RulesOf(entities, e, "create").Owner)
}

// HTML reports whether a field is formatted text, kept as HTML that the
// API cleans (@rest(html)).
func HTML(f *project.Field) bool {
	on, _ := f.Ann["rest"]["html"].(bool)
	return on
}
