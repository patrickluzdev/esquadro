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
			w(0, "%s %s @%s", d.Kind(), d.Name.Name, pos(d.Span))
			docs(1, d.Doc)
			if d.Module != nil {
				w(1, "module %s", dumpValue(*d.Module))
			}
			for _, bp := range d.Blueprints {
				if bp.Instance != nil {
					w(1, "blueprint %s %s @%s", dumpValue(bp.Ref), bp.Instance.Name, pos(bp.Span))
				} else {
					w(1, "blueprint %s @%s", dumpValue(bp.Ref), pos(bp.Span))
				}
				for _, o := range bp.Options {
					w(2, "option %s %s", o.Key.Name, dumpValue(o.Value))
				}
			}
			for _, s := range d.Settings {
				if s.Value == nil {
					w(1, "setting %s @%s", s.Key.Name, pos(s.Span))
				} else {
					w(1, "setting %s %s @%s", s.Key.Name, dumpValue(*s.Value), pos(s.Span))
				}
			}
			for _, a := range d.Annotations {
				w(1, "%s", dumpAnnotation(a))
			}
			for _, it := range d.Items {
				b, ok := it.(*EnvBlock)
				if !ok {
					continue
				}
				env := ""
				if b.Environment != nil {
					env = " " + b.Environment.Name
				}
				w(1, "env%s @%s", env, pos(b.Span))
				for _, e := range b.Decls {
					w(2, "var %s %s%s", e.Name.Name, e.Type.Name, dumpMods(e.Modifiers, nil))
					docs(3, e.Doc)
				}
				for _, o := range b.Values {
					w(2, "value %s %s", o.Key.Name, dumpValue(o.Value))
				}
			}
		case *EnumDecl:
			w(0, "enum %s @%s", d.Name.Name, pos(d.Span))
			docs(1, d.Doc)
			for _, v := range d.Values {
				w(1, "value %s", v.Name.Name)
				docs(2, v.Doc)
			}
		case *PackageDecl:
			w(0, "package %s @%s", d.Name.Name, pos(d.Span))
		case *BlueprintDecl:
			w(0, "blueprint %s @%s", d.Name.Name, pos(d.Span))
			docs(1, d.Doc)
			for _, it := range d.Items {
				switch it := it.(type) {
				case *VersionItem:
					w(1, "version %s", dumpValue(it.Version))
				case *DependsItem:
					names := make([]string, len(it.Names))
					for i, n := range it.Names {
						names[i] = n.Name
					}
					w(1, "%s %s", it.Keyword.Name, strings.Join(names, ", "))
				case *OptionDecl:
					w(1, "option %s %s%s @%s", it.Name.Name, it.Type.Name, dumpMods(it.Modifiers, nil), pos(it.Span))
					docs(2, it.Doc)
				case *EnvDecl:
					w(1, "env %s %s%s @%s", it.Name.Name, it.Type.Name, dumpMods(it.Modifiers, nil), pos(it.Span))
					docs(2, it.Doc)
				case *AnnotationDecl:
					w(1, "annotation %s @%s", it.Target.Name, pos(it.Span))
					docs(2, it.Doc)
					for _, k := range it.Keys {
						w(2, "key %s %s%s", k.Name.Name, k.Type.Name, dumpMods(k.Modifiers, nil))
						docs(3, k.Doc)
					}
				case *SlotItem:
					w(1, "slot %s", dumpValue(it.Name))
					docs(2, it.Doc)
				case *GoRequireItem:
					w(1, "go_require %s %s", dumpValue(it.Module), dumpValue(it.Version))
				case *HookItem:
					writes := ""
					if it.Writes != nil {
						writes = " writes " + dumpValue(*it.Writes)
					}
					if it.Edits != nil {
						writes += " edits " + dumpValue(*it.Edits)
					}
					w(1, "hook %s %s%s", it.Event.Name, dumpValue(it.Path), writes)
				}
			}
		case *EntityDecl, *JobDecl, *ReportDecl:
			keyword, ent := "entity", (*EntityDecl)(nil)
			switch d := d.(type) {
			case *EntityDecl:
				ent = d
			case *JobDecl:
				keyword, ent = "job", &d.EntityDecl
			case *ReportDecl:
				keyword, ent = "report", &d.EntityDecl
			}
			w(0, "%s %s @%s", keyword, ent.Name.Name, pos(ent.Span))
			docs(1, ent.Doc)
			for _, o := range ent.Options {
				w(1, "option %s %s", o.Key.Name, dumpValue(o.Value))
			}
			for _, m := range ent.Members {
				switch m := m.(type) {
				case *Field:
					w(1, "field %s %s%s @%s", m.Name.Name, m.Type.Name, dumpMods(m.Modifiers, m.Annotations), pos(m.Span))
					docs(2, m.Doc)
				case *Relation:
					disp := ""
					if m.TargetField != nil {
						disp = "." + m.TargetField.Name
					}
					if m.Display != nil {
						disp += " (" + m.Display.Name + ")"
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
		b.WriteString(" ")
		b.WriteString(m.Name.Name)
		if m.HasParens {
			parts := make([]string, len(m.Args))
			for i, a := range m.Args {
				parts[i] = dumpValue(a)
			}
			b.WriteString("(")
			b.WriteString(strings.Join(parts, ", "))
			b.WriteString(")")
		}
	}
	for _, a := range anns {
		b.WriteString(" ")
		b.WriteString(dumpAnnotation(a))
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
