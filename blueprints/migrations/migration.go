package migrations

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/blueprints/postgres"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Migration is the SQL that moves a database from one version of the
// project to the next.
type Migration struct {
	// Up and Down are goose's two sections; Up is empty when nothing
	// changed.
	Up, Down string
	// Name is a short description of the change, for the file name, such as
	// "create_products" or "add_products_sku".
	Name string
	// Warnings are changes esquadro cannot write safely; they appear in the
	// SQL as TODO comments too.
	Warnings []string
}

// Diff writes the migration from previous to current. previous is nil for a
// project never generated.
func Diff(previous *project.Description, current project.Description) Migration {
	var old database
	if previous != nil {
		old = describe(*previous)
	}
	cur := describe(current)
	up := plan(old, cur)
	down := plan(cur, old)
	m := Migration{Up: up.sql(), Down: down.sql(), Name: up.name(), Warnings: up.todos}
	if previous == nil && m.Up != "" {
		m.Name = "init"
	}
	return m
}

// steps collects SQL statements by phase, so a migration creates what others
// depend on first and drops in the reverse order.
type steps struct {
	createSchemas, createTypes, addValues, createTables []string
	alterColumns, addConstraints, createIndexes         []string
	dropIndexes, dropConstraints, dropColumns           []string
	dropTables, dropTypes, dropSchemas                  []string
	todos                                               []string
	// labels describe the change for the file name, most significant first.
	labels []string
}

func (s *steps) todo(format string, args ...any) {
	s.todos = append(s.todos, fmt.Sprintf(format, args...))
}

func (s *steps) sql() string {
	var parts []string
	for _, group := range [][]string{
		s.createSchemas, s.createTypes, s.addValues, s.dropIndexes, s.dropConstraints,
		s.createTables, s.alterColumns, s.dropColumns, s.addConstraints, s.createIndexes,
		s.dropTables, s.dropTypes, s.dropSchemas,
	} {
		parts = append(parts, group...)
	}
	for _, t := range s.todos {
		parts = append(parts, "-- TODO: "+t)
	}
	return strings.Join(parts, "\n\n")
}

func (s *steps) name() string {
	var labels []string
	for _, l := range s.labels {
		if !slices.Contains(labels, l) {
			labels = append(labels, l)
		}
	}
	switch len(labels) {
	case 0:
		return "no_change"
	case 1, 2:
		return strings.Join(labels, "_and_")
	}
	return labels[0] + "_and_more"
}

// plan lists the statements that turn from into to.
func plan(from, to database) *steps {
	s := &steps{}

	for _, sc := range to.schemas {
		if !slices.Contains(from.schemas, sc) {
			s.createSchemas = append(s.createSchemas, "CREATE SCHEMA "+sc+";")
		}
	}
	for _, sc := range from.schemas {
		if !slices.Contains(to.schemas, sc) {
			s.dropSchemas = append(s.dropSchemas, "DROP SCHEMA "+sc+";")
		}
	}

	for _, e := range to.enums {
		prev := findEnum(from, e.key)
		if prev != nil && prev.qualified() != e.qualified() {
			s.todo("enum type %s is now %s (its module changed schema): ALTER TYPE %s SET SCHEMA / RENAME TO, by hand",
				prev.qualified(), e.qualified(), prev.qualified())
			continue
		}
		if prev == nil {
			s.createTypes = append(s.createTypes, createType(e))
			s.labels = append(s.labels, "create_"+e.name)
			continue
		}
		for _, v := range e.values {
			if !slices.Contains(prev.values, v) {
				s.addValues = append(s.addValues, fmt.Sprintf("ALTER TYPE %s ADD VALUE %s;", e.qualified(), quoteLiteral(v)))
				s.labels = append(s.labels, "add_"+e.name+"_"+strings.ToLower(v))
			}
		}
		for _, v := range prev.values {
			if !slices.Contains(e.values, v) {
				s.todo("remove value %s from %s: Postgres cannot drop an enum value; recreate the type by hand", v, e.qualified())
			}
		}
	}
	for _, e := range from.enums {
		if findEnum(to, e.key) == nil {
			s.dropTypes = append(s.dropTypes, "DROP TYPE "+e.qualified()+";")
			s.labels = append(s.labels, "drop_"+e.name)
		}
	}

	for _, t := range to.tables {
		prev := findTable(from, t.key)
		if prev != nil && prev.qualified() != t.qualified() {
			s.todo("table %s is now %s: move it by hand (ALTER TABLE %s SET SCHEMA %s and/or RENAME TO %s) to keep its rows",
				prev.qualified(), t.qualified(), prev.qualified(), t.schema, t.name)
			continue
		}
		if prev == nil {
			s.createTables = append(s.createTables, createTable(t))
			for _, fk := range t.foreignKeys {
				s.addConstraints = append(s.addConstraints, addForeignKey(t, fk))
			}
			for _, ix := range t.indexes {
				s.createIndexes = append(s.createIndexes, createIndex(t, ix))
			}
			s.labels = append(s.labels, "create_"+t.name)
			continue
		}
		alterTable(s, *prev, t)
	}
	for _, t := range from.tables {
		if findTable(to, t.key) == nil {
			// Its foreign keys go first, so dropped tables that reference
			// each other can go in any order.
			for _, fk := range t.foreignKeys {
				s.dropConstraints = append(s.dropConstraints, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", t.qualified(), fk.name))
			}
			s.dropTables = append(s.dropTables, "DROP TABLE "+t.qualified()+";")
			s.labels = append(s.labels, "drop_"+t.name)
		}
	}
	return s
}

// alterTable compares two versions of a table.
func alterTable(s *steps, from, to table) {
	q := to.qualified()
	for _, c := range to.columns {
		prev := from.column(c.name)
		if prev == nil {
			stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", q, columnSQL(c))
			if c.notNull && c.def == "" {
				stmt += "\n-- Fails when the table has rows: give the column a default or fill it first."
			}
			s.alterColumns = append(s.alterColumns, stmt)
			if c.unique {
				s.addConstraints = append(s.addConstraints, addUnique(to, c))
			}
			s.labels = append(s.labels, "add_"+to.name+"_"+c.name)
			continue
		}
		if prev.sqlType != c.sqlType {
			s.todo("change %s.%s from %s to %s: write the conversion by hand", q, c.name, prev.sqlType, c.sqlType)
			continue
		}
		if prev.notNull != c.notNull {
			verb := "DROP"
			if c.notNull {
				verb = "SET"
			}
			s.alterColumns = append(s.alterColumns, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s NOT NULL;", q, postgres.Ident(c.name), verb))
			s.labels = append(s.labels, "change_"+to.name+"_"+c.name)
		}
		if prev.def != c.def {
			if c.def == "" {
				s.alterColumns = append(s.alterColumns, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", q, postgres.Ident(c.name)))
			} else {
				s.alterColumns = append(s.alterColumns, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", q, postgres.Ident(c.name), c.def))
			}
			s.labels = append(s.labels, "change_"+to.name+"_"+c.name)
		}
		if prev.unique != c.unique {
			if c.unique {
				s.addConstraints = append(s.addConstraints, addUnique(to, c))
			} else {
				s.dropConstraints = append(s.dropConstraints, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", q, uniqueName(to, c)))
			}
			s.labels = append(s.labels, "change_"+to.name+"_"+c.name)
		}
	}
	for _, c := range from.columns {
		if to.column(c.name) == nil {
			s.dropColumns = append(s.dropColumns, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", q, postgres.Ident(c.name)))
			s.labels = append(s.labels, "drop_"+to.name+"_"+c.name)
		}
	}

	for _, fk := range to.foreignKeys {
		if prev := findForeignKey(from, fk.name); prev == nil || *prev != fk {
			if prev != nil {
				s.dropConstraints = append(s.dropConstraints, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", q, fk.name))
			}
			s.addConstraints = append(s.addConstraints, addForeignKey(to, fk))
		}
	}
	for _, fk := range from.foreignKeys {
		if findForeignKey(to, fk.name) == nil {
			s.dropConstraints = append(s.dropConstraints, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", q, fk.name))
		}
	}

	for _, ix := range to.indexes {
		prev := findIndex(from, ix.name)
		if prev != nil && slices.Equal(prev.columns, ix.columns) && prev.unique == ix.unique {
			continue
		}
		if prev != nil {
			s.dropIndexes = append(s.dropIndexes, "DROP INDEX "+to.schema+"."+ix.name+";")
		}
		s.createIndexes = append(s.createIndexes, createIndex(to, ix))
		s.labels = append(s.labels, "index_"+to.name)
	}
	for _, ix := range from.indexes {
		if findIndex(to, ix.name) == nil {
			s.dropIndexes = append(s.dropIndexes, "DROP INDEX "+from.schema+"."+ix.name+";")
		}
	}
}

func createType(e enumType) string {
	values := make([]string, len(e.values))
	for i, v := range e.values {
		values[i] = quoteLiteral(v)
	}
	return fmt.Sprintf("CREATE TYPE %s AS ENUM (%s);", e.qualified(), strings.Join(values, ", "))
}

func createTable(t table) string {
	var lines []string
	for _, c := range t.columns {
		lines = append(lines, "    "+columnSQL(c))
	}
	lines = append(lines, "    PRIMARY KEY ("+idents(t.primaryKey)+")")
	for _, c := range t.columns {
		if c.unique {
			lines = append(lines, fmt.Sprintf("    CONSTRAINT %s UNIQUE (%s)", uniqueName(t, c), postgres.Ident(c.name)))
		}
	}
	return "CREATE TABLE " + t.qualified() + " (\n" + strings.Join(lines, ",\n") + "\n);"
}

func idents(names []string) string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = postgres.Ident(n)
	}
	return strings.Join(out, ", ")
}

func columnSQL(c column) string {
	s := postgres.Ident(c.name) + " " + c.sqlType
	if c.notNull {
		s += " NOT NULL"
	}
	if c.def != "" {
		s += " DEFAULT " + c.def
	}
	return s
}

func uniqueName(t table, c column) string { return t.name + "_" + c.name + "_key" }

func addUnique(t table, c column) string {
	return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s);", t.qualified(), uniqueName(t, c), postgres.Ident(c.name))
}

func addForeignKey(t table, fk foreignKey) string {
	return fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (id) ON DELETE %s;",
		t.qualified(), fk.name, postgres.Ident(fk.column), fk.refTable, fk.onDelete)
}

func createIndex(t table, ix index) string {
	kind := "INDEX"
	if ix.unique {
		kind = "UNIQUE INDEX"
	}
	return fmt.Sprintf("CREATE %s %s ON %s (%s);", kind, ix.name, t.qualified(), idents(ix.columns))
}

func findEnum(db database, key string) *enumType {
	for i := range db.enums {
		if db.enums[i].key == key {
			return &db.enums[i]
		}
	}
	return nil
}

func findTable(db database, key string) *table {
	for i := range db.tables {
		if db.tables[i].key == key {
			return &db.tables[i]
		}
	}
	return nil
}

func findForeignKey(t table, name string) *foreignKey {
	for i := range t.foreignKeys {
		if t.foreignKeys[i].name == name {
			return &t.foreignKeys[i]
		}
	}
	return nil
}

func findIndex(t table, name string) *index {
	for i := range t.indexes {
		if t.indexes[i].name == name {
			return &t.indexes[i]
		}
	}
	return nil
}

// File writes a migration as a goose SQL file.
func (m Migration) File() string {
	var b strings.Builder
	b.WriteString("-- Generated by esquadro from a change in the .esq files. Review it before\n")
	b.WriteString("-- applying it; esquadro never changes it again.\n\n")
	b.WriteString("-- +goose Up\n")
	b.WriteString(m.Up)
	b.WriteString("\n\n-- +goose Down\n")
	b.WriteString(m.Down)
	b.WriteString("\n")
	return b.String()
}

// MigrationsDir is where migrations live in the generated project.
const MigrationsDir = "migrations"

// FileName returns the path, relative to the project, of the file for a
// migration: the next number after the files already in migrations/, or the
// file that already holds exactly this migration, so a generation that
// failed half way and runs again does not write it twice. It returns "" when
// the migration changes nothing.
func FileName(projectDir string, m Migration) (string, error) {
	if m.Up == "" {
		return "", nil
	}
	entries, err := os.ReadDir(filepath.Join(projectDir, MigrationsDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	content := m.File()
	last := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(projectDir, MigrationsDir, name))
		if err != nil {
			return "", err
		}
		if string(data) == content {
			return MigrationsDir + "/" + name, nil
		}
		prefix, _, _ := strings.Cut(name, "_")
		if n, err := strconv.Atoi(prefix); err == nil {
			last = max(last, n)
		}
	}
	return fmt.Sprintf("%s/%s_%s.sql", MigrationsDir, itoa5(last+1), m.Name), nil
}
