package compose

import (
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// goTypes maps field types to Go types. Enums use their own type.
var goTypes = map[project.Type]string{
	project.TypeString:  "string",
	project.TypeText:    "string",
	project.TypeInt:     "int",
	project.TypeInt64:   "int64",
	project.TypeDecimal: "decimal.Decimal",
	project.TypeBool:    "bool",
	project.TypeTime:    "time.Time",
	project.TypeDate:    "time.Time",
	project.TypeUUID:    "uuid.UUID",
	project.TypeJSON:    "json.RawMessage",
}

// goImportOf is the import each Go type needs.
var goImportOf = map[project.Type]string{
	project.TypeDecimal: "github.com/shopspring/decimal",
	project.TypeTime:    "time",
	project.TypeDate:    "time",
	project.TypeJSON:    "encoding/json",
	project.TypeUUID:    "uuid",
}

// goType returns the Go type of a field.
func goType(f *project.Field) string {
	if f.Type == project.TypeEnum && f.Enum != nil {
		return f.Enum.Pascal
	}
	return goTypes[f.Type]
}

// goImports returns, sorted, the imports the Go types of an entity's fields
// need.
func goImports(e *project.Entity) []string {
	var out []string
	for _, f := range e.Fields {
		if imp := goImportOf[f.Type]; imp != "" && !slices.Contains(out, imp) {
			out = append(out, imp)
		}
	}
	slices.Sort(out)
	return out
}

// importBlock writes a Go import declaration from import paths given as
// strings or string slices: empty ones are skipped, duplicates dropped, and
// the standard library comes first, in its own group, as goimports does. A
// path may carry a name: "ogenuuid github.com/google/uuid". It writes
// nothing when there are no imports.
func importBlock(paths ...any) string {
	var std, other []string
	add := func(p string) {
		if p == "" || slices.Contains(std, p) || slices.Contains(other, p) {
			return
		}
		path := p
		if _, after, aliased := strings.Cut(p, " "); aliased {
			path = after
		}
		if first, _, _ := strings.Cut(path, "/"); strings.Contains(first, ".") {
			other = append(other, p)
		} else {
			std = append(std, p)
		}
	}
	for _, p := range paths {
		switch p := p.(type) {
		case string:
			add(p)
		case []string:
			for _, x := range p {
				add(x)
			}
		}
	}
	if len(std)+len(other) == 0 {
		return ""
	}
	slices.Sort(std)
	slices.Sort(other)
	var b strings.Builder
	b.WriteString("import (\n")
	for i, group := range [][]string{std, other} {
		if i == 1 && len(std) > 0 && len(other) > 0 {
			b.WriteString("\n")
		}
		for _, p := range group {
			b.WriteString("\t")
			if alias, path, aliased := strings.Cut(p, " "); aliased {
				b.WriteString(alias)
				b.WriteString(" ")
				p = path
			}
			b.WriteString(strconv.Quote(p))
			b.WriteString("\n")
		}
	}
	b.WriteString(")\n")
	return b.String()
}

// goDoc writes a doc comment for a Go declaration named name, following Go's
// convention that it starts with the name: "Product is something the shop
// sells." from the .esq doc "Something the shop sells.". Without a doc, it
// uses fallback, which should already start with the name.
func goDoc(name string, doc []string, fallback string) string {
	if len(doc) == 0 {
		return "// " + fallback
	}
	lines := slices.Clone(doc)
	if first := lines[0]; !strings.HasPrefix(first, name+" ") {
		lines[0] = name + " is " + lowerFirst(first)
	}
	for i, l := range lines {
		lines[i] = strings.TrimRight("// "+l, " ")
	}
	return strings.Join(lines, "\n")
}

// lowerFirst lowers the first letter of s unless it starts an acronym
// ("HTTP server" stays as is).
func lowerFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	next, _ := utf8.DecodeRuneInString(s[size:])
	if size == 0 || unicode.IsUpper(next) {
		return s
	}
	return string(unicode.ToLower(r)) + s[size:]
}

// enumValues lists an enum's values as "A, B or C".
func enumValues(e *project.Enum) string {
	names := make([]string, len(e.Values))
	for i, v := range e.Values {
		names[i] = v.Raw
	}
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
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

// goStrings is a Go []string literal of a list option, e.g. []string{"a"}.
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

// goVersion is the release of the Go that built esquadro, without its
// patch: go1.27.3 is 1.27.
func goVersion() string {
	v := strings.TrimPrefix(runtime.Version(), "go")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return v
	}
	minor, _, _ := strings.Cut(parts[1], " ")
	return parts[0] + "." + minor
}
