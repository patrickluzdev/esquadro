package seed

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"

	"go.yaml.in/yaml/v3"
)

// rules are the rules a generated field may have.
var rules = []string{"sequence", "pick", "one_of", "int", "decimal", "money", "past", "future", "between", "trend", "any", "name", "email", "sentence", "words", "null"}

// Check reads the project's seeds/*.yaml against the model: lists, fields,
// enum values, aliases, rules, each at its file and line.
func Check(p *project.Project, _ *blueprint.Set) diag.List {
	var diags diag.List
	if len(p.Files) == 0 {
		return nil
	}
	root := filepath.Dir(filepath.Dir(p.Files[0]))
	var files []string
	for _, pattern := range []string{"*.yaml", "*.yml"} {
		m, _ := filepath.Glob(filepath.Join(root, "seeds", pattern))
		files = append(files, m...)
	}
	slices.Sort(files)
	lists := map[string]*project.Entity{}
	for _, e := range p.Entities {
		lists[e.Plural.Snake] = e
	}
	aliases := map[string]map[string]bool{}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		c := &checker{file: file, lists: lists, aliases: aliases, diags: &diags}
		c.document(src)
	}
	return diags
}

type checker struct {
	file    string
	lists   map[string]*project.Entity
	aliases map[string]map[string]bool
	diags   *diag.List
}

func (c *checker) at(n *yaml.Node) diag.Pos {
	return diag.Pos{File: c.file, Line: n.Line, Col: n.Column}
}

func (c *checker) errorf(n *yaml.Node, format string, args ...any) {
	c.diags.Errorf(c.at(n), c.at(n), format, args...)
}

func (c *checker) document(src []byte) {
	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		c.diags.Errorf(diag.Pos{File: c.file, Line: 1, Col: 1}, diag.Pos{File: c.file, Line: 1, Col: 1}, "%v", err)
		return
	}
	if len(root.Content) == 0 {
		return
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		c.errorf(top, "a seed file is lists by their names: categories:, products:")
		return
	}
	for i := 0; i+1 < len(top.Content); i += 2 {
		key, value := top.Content[i], top.Content[i+1]
		e, ok := c.lists[key.Value]
		if !ok {
			names := make([]string, 0, len(c.lists))
			for name := range c.lists {
				names = append(names, name)
			}
			slices.Sort(names)
			c.errorf(key, "%s is not a list of the model (%s)", key.Value, strings.Join(names, ", "))
			continue
		}
		switch value.Kind {
		case yaml.SequenceNode:
			for _, row := range value.Content {
				c.row(e, row)
			}
		case yaml.MappingNode:
			c.generated(e, value)
		default:
			c.errorf(value, "%s: its records ([...]) or generate: and fields:", key.Value)
		}
	}
}

// field is the field or relation a row names: a relation by its name or
// its key's (category, category_id).
func field(e *project.Entity, name string) (*project.Field, *project.Relation) {
	for _, f := range e.Fields {
		if f.Snake == name && (!f.Implicit || name == "created_at" || name == "updated_at") {
			return f, nil
		}
	}
	for _, r := range Keys(e) {
		if r.Snake == name || r.Snake+"_id" == name {
			return nil, r
		}
	}
	return nil, nil
}

func names(e *project.Entity) string {
	var out []string
	for _, f := range e.Fields {
		if !f.Implicit {
			out = append(out, f.Snake)
		}
	}
	for _, r := range Keys(e) {
		out = append(out, r.Snake)
	}
	return strings.Join(out, ", ")
}

func (c *checker) row(e *project.Entity, row *yaml.Node) {
	if row.Kind != yaml.MappingNode {
		c.errorf(row, "a record is its fields: { name: ... }")
		return
	}
	named := map[string]bool{}
	for i := 0; i+1 < len(row.Content); i += 2 {
		k, v := row.Content[i], row.Content[i+1]
		if k.Value == "$id" {
			if c.aliases[e.Plural.Snake] == nil {
				c.aliases[e.Plural.Snake] = map[string]bool{}
			}
			c.aliases[e.Plural.Snake][v.Value] = true
			continue
		}
		f, r := field(e, k.Value)
		switch {
		case r != nil:
			named[r.Snake] = true
			if strings.HasPrefix(v.Value, "$") && !c.aliases[r.Target.Plural.Snake][v.Value[1:]] {
				c.errorf(v, "%s is no alias of %s declared before ($id: %s)", v.Value, r.Target.Plural.Snake, v.Value[1:])
			}
		case f != nil:
			named[f.Snake] = true
			c.value(f, v)
		default:
			c.errorf(k, "%s has no field %s (%s)", e.Plural.Snake, k.Value, names(e))
		}
	}
	for _, f := range e.Fields {
		if f.Required && !f.Implicit && f.Default == nil && !f.IsFile() && !named[f.Snake] {
			c.errorf(row, "%s: this record has no %s, which is required", e.Plural.Snake, f.Snake)
		}
	}
	for _, r := range Keys(e) {
		if r.Required && !named[r.Snake] {
			c.errorf(row, "%s: this record has no %s, which is required", e.Plural.Snake, r.Snake)
		}
	}
}

// value checks a written value against its field's type.
func (c *checker) value(f *project.Field, v *yaml.Node) {
	if v.Tag == "!!null" {
		return
	}
	switch f.Type {
	case project.TypeEnum:
		for _, ev := range f.Enum.Values {
			if ev.Raw == v.Value {
				return
			}
		}
		var values []string
		for _, ev := range f.Enum.Values {
			values = append(values, ev.Raw)
		}
		c.errorf(v, "%s is not one of %s", v.Value, strings.Join(values, ", "))
	case project.TypeInt, project.TypeInt64:
		if _, err := strconv.ParseInt(v.Value, 10, 64); err != nil || v.Kind != yaml.ScalarNode {
			c.errorf(v, "%s is a whole number", f.Snake)
		}
	case project.TypeBool:
		if v.Tag != "!!bool" {
			c.errorf(v, "%s is true or false", f.Snake)
		}
	case project.TypeTime, project.TypeDate:
		ok := false
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", time.DateOnly} {
			if _, err := time.Parse(layout, v.Value); err == nil {
				ok = true
			}
		}
		if !ok {
			c.errorf(v, "%s is a date, as 2026-10-01 or 2026-10-01T10:00:00Z", f.Snake)
		}
	case project.TypeMoney:
		if v.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(v.Content); i += 2 {
				if k := v.Content[i].Value; k != "amount" && k != "currency" {
					c.errorf(v.Content[i], `money is "199.90", or { amount: "199.90", currency: BRL }`)
				}
			}
		}
	case project.TypeFile, project.TypeImage:
		c.errorf(v, "%s is a file: a seed does not send files", f.Snake)
	}
}

func (c *checker) generated(e *project.Entity, block *yaml.Node) {
	hasCount := false
	for i := 0; i+1 < len(block.Content); i += 2 {
		k, v := block.Content[i], block.Content[i+1]
		switch k.Value {
		case "generate":
			hasCount = true
			if n, err := strconv.Atoi(v.Value); err != nil || n < 1 {
				c.errorf(v, "generate is how many records: generate: 100")
			}
		case "fields":
			if v.Kind != yaml.MappingNode {
				c.errorf(v, "fields are the rules of the fields: fields: { total: { money: 10..500 } }")
				continue
			}
			for j := 0; j+1 < len(v.Content); j += 2 {
				name, rule := v.Content[j], v.Content[j+1]
				f, r := field(e, name.Value)
				if f == nil && r == nil {
					c.errorf(name, "%s has no field %s (%s)", e.Plural.Snake, name.Value, names(e))
					continue
				}
				if rule.Kind == yaml.MappingNode {
					for m := 0; m+1 < len(rule.Content); m += 2 {
						if key := rule.Content[m]; !slices.Contains(rules, key.Value) {
							c.errorf(key, "%s is no rule (%s)", key.Value, strings.Join(rules, ", "))
						}
					}
				} else if f != nil && rule.Kind == yaml.ScalarNode {
					c.value(f, rule)
				}
			}
		default:
			c.errorf(k, "%s: a list is its records ([...]) or generate: and fields:", k.Value)
		}
	}
	if !hasCount {
		c.errorf(block, "%s: generate: how many records", e.Plural.Snake)
	}
}
