package dsl

import (
	"fmt"
	"strconv"
	"strings"
)

// Dump renders the syntax tree as indented text, one node per line, with the
// line:col of each declaration and member. It is meant for tests and
// debugging, not as a formatter.
func Dump(f *File) string {
	var b strings.Builder
	w := func(indent int, format string, args ...any) {
		b.WriteString(strings.Repeat("  ", indent))
		fmt.Fprintf(&b, format, args...)
		b.WriteByte('\n')
	}
	docs := func(indent int, doc []string) {
		for _, d := range doc {
			w(indent, "doc %s", strconv.Quote(d))
		}
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *AppDecl:
			w(0, "app %s @%s", d.Name.Name, pos(d.Span))
			docs(1, d.Doc)
			if d.Module != nil {
				w(1, "module %s", dumpValue(*d.Module))
			}
			for _, bp := range d.Blueprints {
				w(1, "blueprint %s @%s", dumpValue(bp.Ref), pos(bp.Span))
				for _, o := range bp.Options {
					w(2, "option %s %s", o.Key.Name, dumpValue(o.Value))
				}
			}
			for _, a := range d.Annotations {
				w(1, "%s", dumpAnnotation(a))
			}
		case *EnumDecl:
			w(0, "enum %s @%s", d.Name.Name, pos(d.Span))
			docs(1, d.Doc)
			for _, v := range d.Values {
				w(1, "value %s", v.Name.Name)
				docs(2, v.Doc)
			}
		case *EntityDecl:
			w(0, "entity %s @%s", d.Name.Name, pos(d.Span))
			docs(1, d.Doc)
			for _, o := range d.Options {
				w(1, "option %s %s", o.Key.Name, dumpValue(o.Value))
			}
			for _, m := range d.Members {
				switch m := m.(type) {
				case *Field:
					w(1, "field %s %s%s @%s", m.Name.Name, m.Type.Name, dumpMods(m.Modifiers, m.Annotations), pos(m.Span))
					docs(2, m.Doc)
				case *Relation:
					disp := ""
					if m.Display != nil {
						disp = " (" + m.Display.Name + ")"
					}
					w(1, "relation %s %s %s%s%s @%s", m.Name.Name, m.Kind, m.Target.Name, disp, dumpMods(m.Modifiers, m.Annotations), pos(m.Span))
					docs(2, m.Doc)
				case *Annotation:
					w(1, "%s", dumpAnnotation(m))
				}
			}
		}
	}
	for _, c := range f.Comments {
		w(0, "comment %s @%s", strconv.Quote(c.Text), pos(Span{Pos: c.Pos, End: c.End}))
	}
	return b.String()
}

func pos(s Span) string {
	return fmt.Sprintf("%d:%d-%d:%d", s.Pos.Line, s.Pos.Col, s.End.Line, s.End.Col)
}

func dumpValue(v Value) string {
	switch v.Kind {
	case StringValue:
		return strconv.Quote(v.Text)
	case ListValue:
		parts := make([]string, len(v.List))
		for i, e := range v.List {
			parts[i] = dumpValue(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case BoolValue:
		return "bool:" + v.Text
	case NumberValue:
		return "num:" + v.Text
	default:
		return v.Text
	}
}

func dumpMods(mods []*Modifier, anns []*Annotation) string {
	var b strings.Builder
	for _, m := range mods {
		b.WriteString(" " + m.Name.Name)
		if m.HasParens {
			parts := make([]string, len(m.Args))
			for i, a := range m.Args {
				parts[i] = dumpValue(a)
			}
			b.WriteString("(" + strings.Join(parts, ", ") + ")")
		}
	}
	for _, a := range anns {
		b.WriteString(" " + dumpAnnotation(a))
	}
	return b.String()
}

func dumpAnnotation(a *Annotation) string {
	parts := make([]string, len(a.Args))
	for i, arg := range a.Args {
		parts[i] = arg.Key.Name
		if arg.Value != nil {
			parts[i] += " " + dumpValue(*arg.Value)
		}
	}
	return "@" + a.Name.Name + "(" + strings.Join(parts, ", ") + ")"
}
