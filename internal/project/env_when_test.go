package project

import (
	"strings"
	"testing"

	"github.com/patrickluzdev/esquadro/internal/dsl"
)

func TestEnvWhen(t *testing.T) {
	var msgs []string
	errorf := func(_ dsl.Span, format string, args ...any) { msgs = append(msgs, format) }
	decl := func(m *dsl.Modifier) *dsl.EnvDecl {
		return &dsl.EnvDecl{Name: dsl.Ident{Name: "X"}, Type: dsl.Ident{Name: "string"}, Modifiers: []*dsl.Modifier{m}}
	}
	when := func(v dsl.Value) *dsl.Modifier {
		return &dsl.Modifier{Name: dsl.Ident{Name: "when"}, HasParens: true, Args: []dsl.Value{v}}
	}
	v := EnvVarOf(decl(when(dsl.Value{Kind: dsl.StringValue, Text: `{{if .Options.on}}true{{end}}`})), errorf)
	if v == nil || v.When != `{{if .Options.on}}true{{end}}` || len(msgs) != 0 {
		t.Fatalf("%+v %v", v, msgs)
	}
	EnvVarOf(decl(when(dsl.Value{Kind: dsl.StringValue, Text: "true"})), errorf)
	if len(msgs) != 1 || !strings.Contains(msgs[0], "when needs a template") {
		t.Fatalf("%v", msgs)
	}
}
