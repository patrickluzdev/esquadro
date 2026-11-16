package project

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/patrickluzdev/esquadro/internal/dsl"
)

// EnvType is the type of an environment variable.
type EnvType string

// The types of environment variables.
const (
	EnvString   EnvType = "string"
	EnvInt      EnvType = "int"
	EnvNumber   EnvType = "number"
	EnvBool     EnvType = "bool"
	EnvList     EnvType = "list"
	EnvDuration EnvType = "duration"
)

// EnvTypes are the types an environment variable can have.
var EnvTypes = []EnvType{EnvString, EnvInt, EnvNumber, EnvBool, EnvList, EnvDuration}

// DefaultEnvironments are the environments of an app that does not say
// otherwise; the first one is where development happens.
var DefaultEnvironments = []string{"development", "staging", "production"}

// EnvVar is an environment variable of the project, declared by a
// blueprint's manifest or by the app: "env NAME type modifiers".
type EnvVar struct {
	// Name is the variable's name as declared; with Prefixed, the app's
	// prefix comes before it (SHOP_POOL_MAX).
	Name     string
	Prefixed bool
	Type     EnvType
	Doc      []string
	// Secret values never show: not in logs, not in errors, not in
	// esquadro's output.
	Secret bool
	// Required are the environments the variable must be set in.
	Required []string
	// Default is the value in every environment, and ByEnv the value in
	// one: string, int64, float64, bool or []string. A string may be a
	// template, rendered with the declaring blueprint's data.
	Default any
	ByEnv   map[string]any
	// Generate is, for a secret, the number of random bytes esquadro
	// generates for it in the development environment; 0 for none.
	Generate int
	// Field is the Go field of config.Config that holds it.
	Field string
	// Blueprint is the blueprint that declares it, empty for the app.
	Blueprint string
	Span      dsl.Span

	// Set when blueprints are resolved:

	// Full is the name with the app's prefix, when Prefixed.
	Full string
	// Values are the values the app sets per environment, in its
	// "env <environment> { ... }" blocks.
	Values map[string]any
}

var (
	envNameRE    = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	envNameLower = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// EnvValue is a value the app sets for a variable in one environment, in an
// "env <environment> { NAME value }" block.
type EnvValue struct {
	Environment dsl.Ident
	Name        dsl.Ident
	Value       dsl.Value
}

// envModifiers are the modifiers of an environment variable besides the
// environment names, which set its value there.
var envModifiers = []string{"default", "required", "secret", "generate", "prefixed", "field"}

// EnvVarOf builds a variable from its declaration, reporting what is wrong
// through errorf. Modifiers named after an environment, such as
// development("..."), are checked against the app's environments later.
func EnvVarOf(d *dsl.EnvDecl, errorf func(dsl.Span, string, ...any)) *EnvVar {
	v := &EnvVar{Name: d.Name.Name, Type: EnvType(d.Type.Name), Doc: d.Doc, Span: d.Span, ByEnv: map[string]any{}}
	if !envNameRE.MatchString(v.Name) {
		errorf(d.Name.Span, "%s is not an environment variable name: use upper case letters, digits and _, e.g. DATABASE_URL", v.Name)
		return nil
	}
	if !slices.Contains(EnvTypes, v.Type) {
		names := make([]string, len(EnvTypes))
		for i, t := range EnvTypes {
			names[i] = string(t)
		}
		errorf(d.Type.Span, "unknown variable type %q: the types are %s", d.Type.Name, strings.Join(names, ", "))
		return nil
	}
	ok := true
	seen := map[string]bool{}
	for _, m := range d.Modifiers {
		name := m.Name.Name
		if seen[name] {
			errorf(m.Name.Span, "%s is already set", name)
			ok = false
			continue
		}
		seen[name] = true
		one := func() (dsl.Value, bool) {
			if !m.HasParens || len(m.Args) != 1 {
				errorf(m.Span, "%s needs exactly one value", name)
				return dsl.Value{}, false
			}
			return m.Args[0], true
		}
		switch name {
		case "secret", "prefixed":
			if m.HasParens {
				errorf(m.Span, "%s takes no arguments", name)
				ok = false
			}
			v.Secret = v.Secret || name == "secret"
			v.Prefixed = v.Prefixed || name == "prefixed"
		case "required":
			if !m.HasParens || len(m.Args) == 0 {
				errorf(m.Span, "required needs the environments, e.g. required(staging, production)")
				ok = false
				continue
			}
			for _, a := range m.Args {
				if a.Kind != dsl.IdentValue {
					errorf(a.Span, "an environment is a name, without quotes")
					ok = false
					continue
				}
				v.Required = append(v.Required, a.Text)
			}
		case "generate":
			a, good := one()
			n, err := strconv.Atoi(a.Text)
			if !good || a.Kind != dsl.NumberValue || err != nil || n < 16 || n > 128 {
				if good {
					errorf(a.Span, "generate needs the number of random bytes, from 16 to 128, e.g. generate(32)")
				}
				ok = false
				continue
			}
			v.Generate = n
		case "field":
			a, good := one()
			if !good || a.Kind != dsl.IdentValue {
				if good {
					errorf(a.Span, "field needs the Go field's name, e.g. field(TelemetryEndpoint)")
				}
				ok = false
				continue
			}
			v.Field = a.Text
		default:
			// default, or the value in one environment.
			a, good := one()
			if !good {
				ok = false
				continue
			}
			val, msg := ConvertEnv(a, v.Type)
			if msg != "" {
				errorf(a.Span, "%s of %s: %s", name, v.Name, msg)
				ok = false
				continue
			}
			if name == "default" {
				v.Default = val
			} else {
				if !envNameLower.MatchString(name) {
					errorf(m.Name.Span, "unknown modifier %q: the modifiers are %s, or an environment's name", name, strings.Join(envModifiers, ", "))
					ok = false
					continue
				}
				v.ByEnv[name] = val
			}
		}
	}
	if v.Secret && v.Default != nil {
		errorf(d.Span, "%s is a secret: it cannot have a default, which would be in the code; give it a development value at most", v.Name)
		ok = false
	}
	if v.Generate > 0 && !v.Secret {
		errorf(d.Span, "generate is for secrets: mark %s secret", v.Name)
		ok = false
	}
	if !ok {
		return nil
	}
	return v
}

// ConvertEnv converts a value written in a .esq file to a variable's type:
// string, int64, float64, bool or []string.
func ConvertEnv(v dsl.Value, t EnvType) (any, string) {
	switch t {
	case EnvString:
		if v.Kind == dsl.StringValue {
			return v.Text, ""
		}
		return nil, "expected a quoted string"
	case EnvDuration:
		if v.Kind == dsl.StringValue {
			if _, err := time.ParseDuration(v.Text); err != nil {
				return nil, fmt.Sprintf("%q is not a duration, such as \"30s\" or \"5m\"", v.Text)
			}
			return v.Text, ""
		}
		return nil, `expected a quoted duration, such as "30s"`
	case EnvInt:
		if v.Kind == dsl.NumberValue {
			if n, err := strconv.ParseInt(v.Text, 10, 64); err == nil {
				return n, ""
			}
		}
		return nil, "expected a whole number"
	case EnvNumber:
		if v.Kind == dsl.NumberValue {
			if n, err := strconv.ParseFloat(v.Text, 64); err == nil {
				return n, ""
			}
		}
		return nil, "expected a number"
	case EnvBool:
		if v.Kind == dsl.BoolValue {
			return v.Text == "true", ""
		}
		return nil, "expected true or false"
	case EnvList:
		if v.Kind != dsl.ListValue {
			return nil, `expected a list, such as ["a", "b"]`
		}
		out := []string{}
		for _, e := range v.List {
			if e.Kind == dsl.ListValue {
				return nil, "a list holds plain values"
			}
			out = append(out, e.Text)
		}
		return out, ""
	}
	return nil, fmt.Sprintf("unknown type %q", t)
}

// FieldName is the Go field of a variable's name: DATABASE_URL is
// DatabaseURL, POOL_MAX is PoolMax.
func FieldName(name string) string {
	n, ok := makeNames(strings.ToLower(name))
	if !ok {
		return name
	}
	return n.Pascal
}
