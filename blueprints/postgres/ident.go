// Package postgres holds the Go code of the built-in postgres blueprint:
// what the blueprints that write SQL (migrations, store) share.
package postgres

// reserved are Postgres keywords that cannot name a column or table without
// quotes; esquadro quotes them wherever it writes SQL.
var reserved = map[string]bool{
	"all": true, "analyse": true, "analyze": true, "and": true, "any": true, "array": true,
	"as": true, "asc": true, "asymmetric": true, "both": true, "case": true, "cast": true,
	"check": true, "collate": true, "column": true, "constraint": true, "create": true,
	"current_catalog": true, "current_date": true, "current_role": true, "current_time": true,
	"current_timestamp": true, "current_user": true, "default": true, "deferrable": true,
	"desc": true, "distinct": true, "do": true, "else": true, "end": true, "except": true,
	"false": true, "fetch": true, "for": true, "foreign": true, "from": true, "grant": true,
	"group": true, "having": true, "in": true, "initially": true, "intersect": true, "into": true,
	"lateral": true, "leading": true, "limit": true, "localtime": true, "localtimestamp": true,
	"not": true, "null": true, "offset": true, "on": true, "only": true, "or": true, "order": true,
	"placing": true, "primary": true, "references": true, "returning": true, "select": true,
	"session_user": true, "some": true, "symmetric": true, "system_user": true, "table": true,
	"then": true, "to": true, "trailing": true, "true": true, "union": true, "unique": true,
	"user": true, "using": true, "variadic": true, "when": true, "where": true, "window": true,
	"with": true,
}

// Ident writes a column or table name for SQL, quoted when it is a keyword.
func Ident(name string) string {
	if reserved[name] {
		return `"` + name + `"`
	}
	return name
}
