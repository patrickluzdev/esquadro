// Package api holds the Go code of the built-in api blueprint: helpers for
// its templates, which write the OpenAPI contract and the code between the
// contract and each module, and its ogen hook.
package api

import (
	"fmt"
	"slices"
	"strconv"
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
		"problems":         func() []Problem { return Problems },
		"translation":      func(lang, code string) string { t, _ := translation(lang, code); return t },
		"translated":       func(lang, code string) bool { _, ok := translation(lang, code); return ok },
		"english":          func(lang string) bool { base, _, _ := strings.Cut(lang, "-"); return strings.EqualFold(base, "en") },
		"strs":             strs,
		"goStrings":        goStrings,
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

// ---- edge ----

// Problem is a problem every generated API may answer with, apart from the
// titles each module writes for its records.
type Problem struct {
	Code   string
	Status int
	Name   string // of its Err variable in package edge
	Title  string
	Doc    string
}

// Problems are the problems of package edge, in the order of their status.
var Problems = []Problem{
	{"invalid_request", 400, "ErrInvalidRequest", "The request does not match the API's contract.", "a request that does not match the contract"},
	{"not_found", 404, "ErrNotFound", "Nothing was found at this address.", "an address with nothing behind it"},
	{"method_not_allowed", 405, "ErrMethodNotAllowed", "This address does not take that method.", "a method the address does not take"},
	{"not_acceptable", 406, "ErrNotAcceptable", "The API answers only in JSON.", "an Accept header that leaves out JSON"},
	{"in_use", 409, "", "Other records still point at this one.", ""},
	{"request_too_large", 413, "ErrRequestTooLarge", "The request body is too large.", "a body over the limit"},
	{"unsupported_media_type", 415, "ErrUnsupportedMediaType", "Send the request body as application/json.", "a body that is not JSON"},
	{"invalid_fields", 422, "", "Some fields are not valid.", ""},
	{"rate_limited", 429, "ErrRateLimited", "Too many requests; try again later.", "a client over the rate limit"},
	{"internal", 500, "ErrInternal", "Something went wrong on our side.", "anything else, logged on the server"},
	{"not_implemented", 501, "ErrNotImplemented", "The operation is not implemented.", "an operation of the contract nobody implemented"},
}

// translations are the titles of Problems in the languages esquadro knows,
// written into a new project's translations.go.
var translations = map[string]map[string]string{
	"pt-BR": {
		"invalid_request":        "A requisição não segue o contrato da API.",
		"not_found":              "Nada foi encontrado neste endereço.",
		"method_not_allowed":     "Este endereço não aceita esse método.",
		"not_acceptable":         "A API responde apenas em JSON.",
		"in_use":                 "Outros registros ainda apontam para este.",
		"request_too_large":      "O corpo da requisição é grande demais.",
		"unsupported_media_type": "Envie o corpo da requisição como application/json.",
		"invalid_fields":         "Alguns campos não são válidos.",
		"rate_limited":           "Requisições demais; tente de novo mais tarde.",
		"internal":               "Algo deu errado do nosso lado.",
		"not_implemented":        "A operação não foi implementada.",
	},
	"es": {
		"invalid_request":        "La solicitud no sigue el contrato de la API.",
		"not_found":              "No se encontró nada en esta dirección.",
		"method_not_allowed":     "Esta dirección no acepta ese método.",
		"not_acceptable":         "La API responde solo en JSON.",
		"in_use":                 "Otros registros todavía apuntan a este.",
		"request_too_large":      "El cuerpo de la solicitud es demasiado grande.",
		"unsupported_media_type": "Envíe el cuerpo de la solicitud como application/json.",
		"invalid_fields":         "Algunos campos no son válidos.",
		"rate_limited":           "Demasiadas solicitudes; inténtelo más tarde.",
		"internal":               "Algo salió mal de nuestro lado.",
		"not_implemented":        "La operación no está implementada.",
	},
}

// translation is the title of a problem in a language, and whether esquadro
// knows it; the English title otherwise.
func translation(lang, code string) (string, bool) {
	if t, ok := translations[lang][code]; ok {
		return t, true
	}
	for _, p := range Problems {
		if p.Code == code {
			return p.Title, false
		}
	}
	return "", false
}

// strs is a list option as strings.
func strs(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		out = append(out, fmt.Sprint(x))
	}
	return out
}

// goStrings is a Go []string literal of a list option.
func goStrings(v any) string {
	var b strings.Builder
	b.WriteString("[]string{")
	for i, s := range strs(v) {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(s))
	}
	b.WriteString("}")
	return b.String()
}
