package app

import (
	"regexp"
	"slices"
	"strings"
	"text/template"
)

// TemplateFuncs are the helpers the app blueprint's templates use.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{"commandNames": commandNames}
}

var caseName = regexp.MustCompile(`"([a-z][a-z0-9-]*)"`)

// commandNames lists the commands of the switch cases blueprints contribute
// to app.commands (case "", "serve": ... case "migrate": ...), sorted, for
// the message of a command that does not exist.
func commandNames(cases string) string {
	var names []string
	for line := range strings.SplitSeq(cases, "\n") {
		if l := strings.TrimSpace(line); strings.HasPrefix(l, "case ") {
			for _, m := range caseName.FindAllStringSubmatch(l, -1) {
				names = append(names, m[1])
			}
		}
	}
	names = append(names, "check")
	slices.Sort(names)
	return strings.Join(slices.Compact(names), ", ")
}
