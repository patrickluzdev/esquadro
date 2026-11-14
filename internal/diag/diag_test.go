package diag

import (
	"strings"
	"testing"
)

const src = "entity Produto {\n  categoria -> Categria (nome)\n\tpreço\tdecimal\n}\n"

func TestRenderOneWithSpanAndSuggestion(t *testing.T) {
	d := Diagnostic{
		Severity:   Error,
		Pos:        Pos{File: "loja.esq", Line: 2, Col: 16},
		End:        Pos{File: "loja.esq", Line: 2, Col: 24},
		Message:    `entidade "Categria" não existe`,
		Suggestion: `você quis dizer "Categoria"?`,
	}
	var b strings.Builder
	RenderOne(&b, d, []byte(src))
	want := `loja.esq:2:16: erro: entidade "Categria" não existe
  |
2 |   categoria -> Categria (nome)
  |                ^^^^^^^^
  = sugestão: você quis dizer "Categoria"?
`
	if b.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestRenderOneExpandsTabsAndCountsRunes(t *testing.T) {
	// "\tpreço\tdecimal": tab expands to 4 spaces; "decimal" starts at rune col 8.
	d := Diagnostic{
		Pos:     Pos{File: "x.esq", Line: 3, Col: 8},
		End:     Pos{File: "x.esq", Line: 3, Col: 15},
		Message: "tipo inválido",
	}
	var b strings.Builder
	RenderOne(&b, d, []byte(src))
	want := `x.esq:3:8: erro: tipo inválido
  |
3 |     preço   decimal
  |             ^^^^^^^
`
	if b.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestRenderOnePastEndOfLine(t *testing.T) {
	d := Diagnostic{Pos: Pos{File: "x.esq", Line: 1, Col: 17}, Message: "esperado '}'"}
	var b strings.Builder
	RenderOne(&b, d, []byte(src))
	if !strings.Contains(b.String(), "  |                 ^\n") {
		t.Fatalf("caret not placed after end of line:\n%s", b.String())
	}
}

func TestRenderWithoutSource(t *testing.T) {
	var b strings.Builder
	RenderOne(&b, Diagnostic{Pos: Pos{File: "a.esq", Line: 1, Col: 1}, Message: "m"}, nil)
	if b.String() != "a.esq:1:1: erro: m\n" {
		t.Fatalf("got %q", b.String())
	}
}

func TestListSummaryAndErr(t *testing.T) {
	var l List
	l.Warnf(Pos{File: "b.esq", Line: 2, Col: 1, Offset: 10}, Pos{}, "w")
	l.Errorf(Pos{File: "a.esq", Line: 1, Col: 1}, Pos{}, "e1")
	l.Errorf(Pos{File: "b.esq", Line: 1, Col: 1, Offset: 0}, Pos{}, "e2")
	if !l.HasErrors() || l.Err() == nil {
		t.Fatal("expected errors")
	}
	l.Sort()
	if l[0].Message != "e1" || l[1].Message != "e2" || l[2].Message != "w" {
		t.Fatalf("bad order: %v", l)
	}
	var b strings.Builder
	Render(&b, l, nil)
	if !strings.HasSuffix(b.String(), "2 erros, 1 aviso\n") {
		t.Fatalf("bad summary: %q", b.String())
	}

	var onlyWarn List
	onlyWarn.Warnf(Pos{}, Pos{}, "w")
	if onlyWarn.HasErrors() || onlyWarn.Err() != nil {
		t.Fatal("warnings must not count as errors")
	}
}
