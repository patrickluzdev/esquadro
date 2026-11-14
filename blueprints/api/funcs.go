// Package api holds the Go code of the built-in api blueprint: helpers for
// its templates, which write the OpenAPI contract and the code between the
// contract and each module, and its ogen hook.
package api

import (
	"fmt"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the helpers the api blueprint's templates use.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"jsonName":         jsonName,
		"keyName":          func(r *project.Relation) string { return r.Camel + "Id" },
		"ogenName":         ogenName,
		"schema":           schema,
		"enumName":         enumName,
		"inRequest":        inRequest,
		"keys":             keys,
		"needsType":        needsType,
		"apiFieldNames":    apiFieldNames,
		"canSampleAPI":     canSampleAPI,
		"apiParents":       apiParents,
		"apiSample":        apiSample,
		"apiSampleImports": apiSampleImports,
		"modulePaths":      modulePaths,
	}
}

// jsonName is a field's property name in the contract: camelCase, as the
// rest of a JSON API.
func jsonName(f *project.Field) string { return f.Camel }

// ogenInitialisms are the names ogen writes in capitals when they are the
// whole name or its first word ("id" is ID, but "productId" is ProductId).
var ogenInitialisms = []string{"id", "url", "uri", "api", "http", "https", "json", "xml", "uuid", "ip", "sql", "html"}

// ogenName is the Go name ogen gives a property or parameter.
func ogenName(name string) string {
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
	}
	return "{type: string}"
}

// inRequest lists the fields a NewX carries: the declared ones.
func inRequest(e *project.Entity) []*project.Field {
	var out []*project.Field
	for _, f := range e.Fields {
		if !f.Implicit {
			out = append(out, f)
		}
	}
	return out
}

// keys lists the relations whose key an entity holds.
func keys(e *project.Entity) []*project.Relation {
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
		for _, f := range inRequest(e) {
			add(f.Snake, jsonName(f))
		}
		for _, r := range keys(e) {
			add(r.Snake, r.Camel+"Id")
		}
	}
	slices.SortFunc(out, func(a, b FieldName) int { return strings.Compare(a.Domain, b.Domain) })
	return out
}

// ---- test data ----

// canSampleAPI reports whether esquadro can write a valid request for an
// entity: not when a required field has a pattern, nor when a required
// relation of its module needs such a record or loops back.
func canSampleAPI(e *project.Entity) bool { return sampleableAPI(e, map[*project.Entity]bool{}) }

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
	for _, r := range keys(e) {
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
		return `uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", ` + i + `))`
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
		for _, r := range keys(e) {
			if r.Required && r.Target.Module != e.Module {
				add("github.com/google/uuid")
			}
		}
		for _, f := range inRequest(e) {
			if !f.Required {
				continue
			}
			s := apiSample(f, "i")
			for prefix, imp := range map[string]string{
				"fmt.": "fmt", "strings.": "strings", "uuid.": "github.com/google/uuid",
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

// modulePaths lists the import paths of the modules with entities.
func modulePaths(appModule string, modules []*project.Module) []string {
	var out []string
	for _, m := range modules {
		if len(m.Entities) > 0 {
			out = append(out, appModule+"/internal/"+m.Snake)
		}
	}
	return out
}
