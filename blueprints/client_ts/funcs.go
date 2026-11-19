// Package clientts is the built-in client_ts blueprint: a TypeScript
// client of the API, generated from the model.
package clientts

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"text/template"

	"github.com/patrickluzdev/esquadro/blueprints/audit"
	"github.com/patrickluzdev/esquadro/blueprints/rest"
	"github.com/patrickluzdev/esquadro/blueprints/security"
	"github.com/patrickluzdev/esquadro/blueprints/store"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// TemplateFuncs are the helpers the client_ts blueprint's templates use.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"packageName":  PackageName,
		"inRequest":    rest.InRequest,
		"inAnswer":     rest.InAnswer,
		"isTotal":      store.IsTotal,
		"keys":         rest.Keys,
		"keyName":      rest.KeyName,
		"keyInRequest": rest.KeyInRequest,
		"includable":   rest.Includable,
		"answerFields": rest.AnswerFields,
		"apiFilters":   rest.APIFilters,
		"sortsOf":      rest.SortsOf,
		"paginationOf": rest.PaginationOf,
		"clearable":    rest.Clearable,
		"realms":       security.RealmsOf,
		"realmAdmins":  security.Admins,
		"tsType":       tsType,
		"reportSample": func(f *project.Field) string {
			switch f.Type {
			case project.TypeInt, project.TypeInt64:
				return "1"
			case project.TypeDecimal:
				return `"1"`
			case project.TypeMoney:
				return `{ amount: "1", currency: "USD" }`
			case project.TypeBool:
				return "true"
			case project.TypeTime:
				return `"2026-01-01T00:00:00Z"`
			case project.TypeDate:
				return `"2026-01-01"`
			case project.TypeUUID:
				return "crypto.randomUUID()"
			case project.TypeEnum:
				return strconv.Quote(f.Enum.Values[0].Raw)
			}
			return `"x"`
		},
		"reportTS": func(f *project.Field) string {
			if f.Type == project.TypeEnum {
				return "string"
			}
			return tsType(f)
		},
		"tsFilter":    tsFilter,
		"tsString":    strconv.Quote,
		"tsPattern":   tsPattern,
		"enumName":    func(e *project.Enum) string { return e.Module.Pascal + e.Pascal },
		"softDeleted": store.SoftDeleted,
		"single":      store.Single,
		"importable":  store.Importable,
		"importBy":    store.ImportBy,
		"idLinks":     store.IDLinks,
		"audited":     audit.Audited,
		"auditedOf":   audit.AuditedOf,
		"manyToMany": func(e *project.Entity) []*project.Relation {
			var out []*project.Relation
			for _, r := range e.Relations {
				if r.Kind == project.ManyToMany {
					out = append(out, r)
				}
			}
			return out
		},
		"isMany":     func(r *project.Relation) bool { return r.Kind == project.ManyToMany },
		"has":        slices.Contains[[]string],
		"sample":     sample,
		"sampleable": sampleable,
		"bound":      func(n *project.Number) string { return n.Text },
		"checks":     checks,
		"scopeOf": func(pkg string) string {
			scope, _, _ := strings.Cut(pkg, "/")
			return scope
		},
		"keyChecks": keyChecks,
	}
}

// PackageName is the npm package of the client: the package option, else
// @<scope>/<app>-client, the scope from the option or the module's GitHub
// owner; "" when there is none.
func PackageName(app *project.App, opts map[string]any) string {
	if p, _ := opts["package"].(string); p != "" {
		return p
	}
	scope, _ := opts["scope"].(string)
	if scope == "" {
		if rest, ok := strings.CutPrefix(app.Module, "github.com/"); ok {
			scope, _, _ = strings.Cut(rest, "/")
		}
	}
	if scope == "" {
		return ""
	}
	return "@" + strings.ToLower(scope) + "/" + app.Kebab + "-client"
}

// tsType is the TypeScript type of a field's value, as the JSON carries it.
func tsType(f *project.Field) string {
	switch f.Type {
	case project.TypeInt, project.TypeInt64:
		return "number"
	case project.TypeBool:
		return "boolean"
	case project.TypeJSON:
		return "unknown"
	case project.TypeEnum:
		return f.Enum.Module.Pascal + f.Enum.Pascal
	case project.TypeMoney:
		return "Money"
	case project.TypeFile, project.TypeImage:
		return "StoredFile"
	}
	return "string"
}

// tsFilter is the TypeScript type of a list's filter parameter.
func tsFilter(f rest.APIFilter, e *project.Entity) string {
	if f.Array {
		for _, fl := range e.Fields {
			if fl.Pascal == f.Field && fl.Enum != nil {
				return fl.Enum.Module.Pascal + fl.Enum.Pascal + "[]"
			}
		}
		return "string[]"
	}
	switch f.Go {
	case "int", "int64":
		return "number"
	case "bool":
		return "boolean"
	}
	return "string"
}

// tsPattern is a field's pattern as a JavaScript regular expression
// literal, or "" when JavaScript reads it differently from Go.
func tsPattern(f *project.Field) string {
	if f.Pattern == "" || !rest.PortablePattern(f.Pattern) {
		return ""
	}
	return "/" + strings.ReplaceAll(f.Pattern, "/", `\/`) + "/"
}

// sampleable reports whether the generated tests can write an e: no
// required relation, and no required field with a pattern.
func sampleable(e *project.Entity) bool {
	if store.Single(e) {
		return false // it has no create
	}
	for _, r := range rest.Keys(e) {
		if r.Required {
			return false
		}
	}
	for _, f := range rest.InRequest(e) {
		if f.Required && (f.Pattern != "" || f.Type == project.TypeJSON || f.IsFile()) {
			return false
		}
	}
	return true
}

// sample is a TypeScript expression of a valid value of f for the record
// numbered i, which tells unique values apart.
func sample(f *project.Field, i int) string {
	switch f.Type {
	case project.TypeInt, project.TypeInt64:
		n := float64(i)
		if f.Min != nil && n < f.Min.Value {
			n = f.Min.Value
		}
		if f.Max != nil && n > f.Max.Value {
			n = f.Max.Value
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	case project.TypeDecimal:
		n := float64(i)
		if f.Min != nil && n < f.Min.Value {
			n = f.Min.Value
		}
		if f.Max != nil && n > f.Max.Value {
			n = f.Max.Value
		}
		return strconv.Quote(strconv.FormatFloat(n, 'f', 2, 64))
	case project.TypeMoney:
		n := float64(i)
		if f.Min != nil && n < f.Min.Value {
			n = f.Min.Value
		}
		if f.Max != nil && n > f.Max.Value {
			n = f.Max.Value
		}
		return `{ amount: ` + strconv.Quote(strconv.FormatFloat(n, 'f', 2, 64)) + ` }`
	case project.TypeBool:
		return "true"
	case project.TypeTime:
		return `new Date().toISOString()`
	case project.TypeDate:
		return `"2026-01-0` + strconv.Itoa(1+i%9) + `"`
	case project.TypeUUID:
		return "crypto.randomUUID()"
	case project.TypeEnum:
		return strconv.Quote(f.Enum.Values[0].Raw)
	case project.TypeJSON:
		return "{}"
	}
	s := fmt.Sprintf("%s %d", f.Pascal, i)
	if f.Min != nil && float64(len(s)) < f.Min.Value {
		s += strings.Repeat("x", int(f.Min.Value)-len(s))
	}
	if f.Max != nil && float64(len(s)) > f.Max.Value {
		s = s[:int(f.Max.Value)]
	}
	return strconv.Quote(s)
}

// checks is the TypeScript that checks the value v of f as the domain
// does, calling refuse with the API's codes; changes is whether v comes
// from a PATCH, where an absent value changes nothing.
func checks(f *project.Field, v string, changes bool) string {
	name := strconv.Quote(f.Snake)
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, "  "+format+"\n", args...) }
	if f.Required && f.Default == nil {
		if changes {
			line(`if (%s === null) refuse(%s, "required")`, v, name)
		} else {
			line(`if (%s === undefined || %s === null || (%s as unknown) === "") refuse(%s, "required")`, v, v, v, name)
		}
	}
	switch f.Type {
	case project.TypeString, project.TypeText:
		if f.Min != nil {
			line(`if (typeof %s === "string" && %s !== "" && [...%s].length < %s) refuse(%s, "too_short")`, v, v, v, f.Min.Text, name)
		}
		if f.Max != nil {
			line(`if (typeof %s === "string" && [...%s].length > %s) refuse(%s, "too_long")`, v, v, f.Max.Text, name)
		}
		if re := tsPattern(f); re != "" {
			line(`if (typeof %s === "string" && %s !== "" && !%s.test(%s)) refuse(%s, "pattern")`, v, v, re, v, name)
		}
	case project.TypeInt, project.TypeInt64:
		if f.Min != nil {
			line(`if (typeof %s === "number" && %s < %s) refuse(%s, "too_small")`, v, v, f.Min.Text, name)
		}
		if f.Max != nil {
			line(`if (typeof %s === "number" && %s > %s) refuse(%s, "too_large")`, v, v, f.Max.Text, name)
		}
	case project.TypeDecimal:
		line(`if (typeof %s === "string" && !/^-?\d+(\.\d+)?$/.test(%s)) refuse(%s, "invalid_value")`, v, v, name)
		if f.Min != nil {
			line(`else if (typeof %s === "string" && Number(%s) < %s) refuse(%s, "too_small")`, v, v, f.Min.Text, name)
		}
		if f.Max != nil {
			line(`else if (typeof %s === "string" && Number(%s) > %s) refuse(%s, "too_large")`, v, v, f.Max.Text, name)
		}
	case project.TypeMoney:
		a := v + "?.amount"
		line(`if (%s !== undefined && %s !== null && !/^-?\d+(\.\d{1,4})?$/.test(%s)) refuse(%s, /^-?\d+\.\d+$/.test(%s) ? "too_precise" : "invalid_value")`, v, v, a, name, a)
		if f.Min != nil {
			line(`else if (typeof %s === "string" && Number(%s) < %s) refuse(%s, "too_small")`, a, a, f.Min.Text, name)
		}
		if f.Max != nil {
			line(`else if (typeof %s === "string" && Number(%s) > %s) refuse(%s, "too_large")`, a, a, f.Max.Text, name)
		}
	case project.TypeEnum:
		line(`if (typeof %s === "string" && (%s as string) !== "" && !(%sValues as readonly string[]).includes(%s)) refuse(%s, "invalid_value")`,
			v, v, f.Enum.Module.Pascal+f.Enum.Pascal, v, name)
	}
	return b.String()
}

// keyChecks is the TypeScript that checks the key of r: required in a
// request that creates or replaces when the request must send it, and not
// null in a PATCH when the relation is required.
func keyChecks(r *project.Relation, v string, inRequest, changes bool) string {
	name := strconv.Quote(rest.KeyName(r))
	switch {
	case changes && r.Required:
		return fmt.Sprintf("  if (%s === null) refuse(%s, \"required\")\n", v, name)
	case !changes && inRequest:
		return fmt.Sprintf("  if (%s === undefined || %s === \"\") refuse(%s, \"required\")\n", v, v, name)
	}
	return ""
}
