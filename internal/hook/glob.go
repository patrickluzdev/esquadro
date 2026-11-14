package hook

import (
	"path"
	"strings"
)

// Match reports whether a slash-separated path matches a pattern. Each
// segment is matched with path.Match ("*", "?", "[...]"), and a "**" segment
// matches any number of segments, including none:
//
//	internal/*/sqlgen/**   matches internal/catalog/sqlgen/db.go
//	**/*.sql               matches queries.sql and db/x/queries.sql
func Match(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			for i := 0; i <= len(name); i++ {
				if matchSegments(rest, name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], name[0]); !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// MatchAny reports whether name matches one of the patterns.
func MatchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if Match(p, name) {
			return true
		}
	}
	return false
}
