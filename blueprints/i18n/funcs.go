// Package i18n holds the Go code of the built-in i18n blueprint: the titles
// of the problems the built-in blueprints answer with, in the languages
// esquadro knows.
package i18n

import (
	"strings"
	"text/template"
)

// TemplateFuncs are the helpers the i18n blueprint's templates use.
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"problems":    func() []Problem { return Problems },
		"translation": func(lang, code string) string { t, _ := translation(lang, code); return t },
		"translated":  func(lang, code string) bool { _, ok := translation(lang, code); return ok },
		"english":     func(lang string) bool { base, _, _ := strings.Cut(lang, "-"); return strings.EqualFold(base, "en") },
	}
}

// Problem is a problem a built-in blueprint answers with: its code, the
// blueprint that writes it, and its title in English.
type Problem struct {
	Code      string
	Blueprint string
	Title     string
}

// Problems are the problems of the built-in blueprints, in the order of
// their status.
var Problems = []Problem{
	{"invalid_request", "rest", "The request does not match the API's contract."},
	{"not_found", "http", "Nothing was found at this address."},
	{"method_not_allowed", "rest", "This address does not take that method."},
	{"not_acceptable", "rest", "The API answers only in JSON."},
	{"in_use", "rest", "Other records still point at this one."},
	{"request_too_large", "rest", "The request body is too large."},
	{"unsupported_media_type", "rest", "Send the request body as application/json."},
	{"invalid_fields", "rest", "Some fields are not valid."},
	{"rate_limited", "rate_limit", "Too many requests; try again later."},
	{"internal", "http", "Something went wrong on our side."},
	{"not_implemented", "rest", "The operation is not implemented."},
}

// translations are the titles of Problems in the languages esquadro knows,
// written into a new project's translations.go.
var translations = map[string]map[string]string{
	"pt-BR": {
		"invalid_request":        "A requisição não segue o contrato da API.",
		"not_found":              "Nada foi encontrado neste endereço.",
		"method_not_allowed":     "Este endereço não aceita esse método.",
		"not_acceptable":         "A API responde apenas em JSON.",
		"in_use":                 "Outros registros ainda apontam para este.",
		"request_too_large":      "O corpo da requisição é grande demais.",
		"unsupported_media_type": "Envie o corpo da requisição como application/json.",
		"invalid_fields":         "Alguns campos não são válidos.",
		"rate_limited":           "Requisições demais; tente de novo mais tarde.",
		"internal":               "Algo deu errado do nosso lado.",
		"not_implemented":        "A operação não foi implementada.",
	},
	"es": {
		"invalid_request":        "La solicitud no sigue el contrato de la API.",
		"not_found":              "No se encontró nada en esta dirección.",
		"method_not_allowed":     "Esta dirección no acepta ese método.",
		"not_acceptable":         "La API responde solo en JSON.",
		"in_use":                 "Otros registros todavía apuntan a este.",
		"request_too_large":      "El cuerpo de la solicitud es demasiado grande.",
		"unsupported_media_type": "Envíe el cuerpo de la solicitud como application/json.",
		"invalid_fields":         "Algunos campos no son válidos.",
		"rate_limited":           "Demasiadas solicitudes; inténtelo más tarde.",
		"internal":               "Algo salió mal de nuestro lado.",
		"not_implemented":        "La operación no está implementada.",
	},
}

// translation is the title of a problem in a language, and whether esquadro
// knows it; the English title otherwise.
func translation(lang, code string) (string, bool) {
	if t, ok := translations[lang][code]; ok {
		return t, true
	}
	for _, p := range Problems {
		if p.Code == code {
			return p.Title, false
		}
	}
	return "", false
}
