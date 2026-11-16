package compose

import (
	"slices"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// envFuncs are the template helpers for environment variables.
func envFuncs(optionsOf func(string) map[string]any) map[string]any {
	return map[string]any{
		"envGoType": envGoType,
		"envLoader": envLoader,
		"envValues": envValues,
		"envText":   envText,
		"envValue":  envValue,
		"envUses":   envUses,
		// composePort is the host port compose gives a service, as its
		// ports option says ("postgres:5433"), or fallback.
		"composePort": func(service, fallback string) string {
			for _, p := range strs(optionsOf("compose")["ports"]) {
				if after, ok := strings.CutPrefix(p, service+":"); ok {
					return after
				}
			}
			return fallback
		},
	}
}

// envGoType is the Go type of a variable's field in config.Config.
func envGoType(v *project.EnvVar) string {
	return map[project.EnvType]string{
		project.EnvString: "string", project.EnvInt: "int", project.EnvNumber: "float64",
		project.EnvBool: "bool", project.EnvList: "[]string", project.EnvDuration: "time.Duration",
	}[v.Type]
}

// envLoader is the loader method that reads a variable of its type.
func envLoader(v *project.EnvVar) string {
	return map[project.EnvType]string{
		project.EnvString: "str", project.EnvInt: "integer", project.EnvNumber: "float",
		project.EnvBool: "boolean", project.EnvList: "items", project.EnvDuration: "duration",
	}[v.Type]
}

// envUses reports whether any variable has one of the types.
func envUses(vars []*project.EnvVar, types ...string) bool {
	return slices.ContainsFunc(vars, func(v *project.EnvVar) bool { return slices.Contains(types, string(v.Type)) })
}

// envText is a value as an environment variable holds it: a list is
// comma-separated.
func envText(val any) string {
	switch val := val.(type) {
	case nil:
		return ""
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'g', -1, 64)
	case []string:
		return strings.Join(val, ",")
	case []any:
		return strings.Join(strs(val), ",")
	}
	return ""
}

// envValue is a variable's value in an environment, as text: the app's
// value there, else the declared one there, else the declared default;
// empty when it has none.
func envValue(v *project.EnvVar, env string) string {
	if val, ok := v.Values[env]; ok {
		return envText(val)
	}
	if val, ok := v.ByEnv[env]; ok {
		return envText(val)
	}
	return envText(v.Default)
}

// envValues is the Go literal of a variable's values from the model: by
// environment, and under "" in every environment; nil when it has none.
func envValues(v *project.EnvVar, envs []string) string {
	var parts []string
	if v.Default != nil {
		parts = append(parts, `"": `+strconv.Quote(envText(v.Default)))
	}
	for _, e := range envs {
		val, ok := v.Values[e]
		if !ok {
			val, ok = v.ByEnv[e]
		}
		if ok {
			parts = append(parts, strconv.Quote(e)+": "+strconv.Quote(envText(val)))
		}
	}
	if len(parts) == 0 {
		return "nil"
	}
	return "map[string]string{" + strings.Join(parts, ", ") + "}"
}
