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
	{"bad_request", "security", "The request is not valid JSON of the expected shape."},
	{"invalid_redirect", "security", "The app may not send people there."},
	{"invalid_link", "security", "The link is not valid, or it expired."},
	{"unauthorized", "security", "Sign in to do this."},
	{"invalid_credentials", "security", "The email or the password is wrong."},
	{"forbidden", "security", "You may not do this."},
	{"csrf", "security", "The request did not come from the app."},
	{"email_not_verified", "security", "Verify your email first."},
	{"signup_closed", "security", "Accounts are not open to sign-up."},
	{"reauthentication_required", "security", "Sign in again to do this."},
	{"not_found", "http", "Nothing was found at this address."},
	{"method_not_allowed", "rest", "This address does not take that method."},
	{"not_acceptable", "rest", "The API answers only in JSON."},
	{"in_use", "rest", "Other records still point at this one."},
	{"idempotency_in_progress", "rest", "A request with this Idempotency-Key is still running: try again shortly."},
	{"email_taken", "security", "Another account has this email."},
	{"last_credential", "security", "This is the only way left to sign in."},
	{"request_too_large", "rest", "The request body is too large."},
	{"unsupported_media_type", "rest", "Send the request body as application/json."},
	{"precondition_failed", "rest", "The record changed since you read it: read it again."},
	{"idempotency_key_reused", "rest", "This Idempotency-Key was used for another request."},
	{"invalid_fields", "rest", "Some fields are not valid."},
	{"too_many_records", "rest", "Too many records: turn pagination on for them."},
	{"import_refused", "rest", "Some rows are not valid: nothing was imported."},
	{"too_many_rows", "rest", "An import takes 5000 rows at most."},
	{"precondition_required", "rest", "Send If-Match with the ETag of the record you read."},
	{"idempotency_key_required", "rest", "Send an Idempotency-Key, a new random UUID for each operation."},
	{"rate_limited", "rate_limit", "Too many requests; try again later."},
	{"too_many_attempts", "security", "Too many attempts. Try again later."},
	{"too_soon", "security", "This email was sent a short while ago: it may be sent again later."},
	{"internal", "http", "Something went wrong on our side."},
	{"not_ready", "http", "The service is not ready."},
	{"not_implemented", "rest", "The operation is not implemented."},
	{"provider_not_configured", "security", "Signing in this way is not set up."},
}

// translations are the titles of Problems in the languages esquadro knows,
// written into a new project's translations.go.
var translations = map[string]map[string]string{
	"pt-BR": {
		"invalid_request":           "A requisição não segue o contrato da API.",
		"not_found":                 "Nada foi encontrado neste endereço.",
		"method_not_allowed":        "Este endereço não aceita esse método.",
		"not_acceptable":            "A API responde apenas em JSON.",
		"in_use":                    "Outros registros ainda apontam para este.",
		"request_too_large":         "O corpo da requisição é grande demais.",
		"unsupported_media_type":    "Envie o corpo da requisição como application/json.",
		"invalid_fields":            "Alguns campos não são válidos.",
		"idempotency_in_progress":   "Um pedido com esta Idempotency-Key ainda está rodando: tente de novo em instantes.",
		"idempotency_key_reused":    "Esta Idempotency-Key já foi usada em outro pedido.",
		"idempotency_key_required":  "Envie uma Idempotency-Key, um UUID aleatório novo para cada operação.",
		"precondition_failed":       "O registro mudou desde que você o leu: leia de novo.",
		"precondition_required":     "Envie If-Match com o ETag do registro que você leu.",
		"too_many_records":          "Registros demais: ligue a paginação para eles.",
		"import_refused":            "Algumas linhas não são válidas: nada foi importado.",
		"too_many_rows":             "Uma importação aceita no máximo 5000 linhas.",
		"rate_limited":              "Requisições demais; tente de novo mais tarde.",
		"internal":                  "Algo deu errado do nosso lado.",
		"not_ready":                 "O serviço ainda não está pronto.",
		"not_implemented":           "A operação não foi implementada.",
		"bad_request":               "A requisição não é um JSON válido no formato esperado.",
		"invalid_redirect":          "O app não pode mandar ninguém para esse endereço.",
		"invalid_link":              "O link não é válido ou expirou.",
		"unauthorized":              "Entre na sua conta para fazer isso.",
		"invalid_credentials":       "O e-mail ou a senha estão errados.",
		"forbidden":                 "Você não pode fazer isso.",
		"csrf":                      "A requisição não veio do app.",
		"email_not_verified":        "Verifique seu e-mail primeiro.",
		"signup_closed":             "O cadastro não está aberto.",
		"reauthentication_required": "Entre de novo na sua conta para fazer isso.",
		"email_taken":               "Outra conta já usa este e-mail.",
		"last_credential":           "Esta é a única forma de entrar que sobrou.",
		"too_many_attempts":         "Tentativas demais. Tente de novo mais tarde.",
		"too_soon":                  "Este e-mail foi enviado há pouco: ele pode ser enviado de novo mais tarde.",
		"provider_not_configured":   "Entrar desta forma não está configurado.",
	},
	"es": {
		"invalid_request":           "La solicitud no sigue el contrato de la API.",
		"not_found":                 "No se encontró nada en esta dirección.",
		"method_not_allowed":        "Esta dirección no acepta ese método.",
		"not_acceptable":            "La API responde solo en JSON.",
		"in_use":                    "Otros registros todavía apuntan a este.",
		"request_too_large":         "El cuerpo de la solicitud es demasiado grande.",
		"unsupported_media_type":    "Envíe el cuerpo de la solicitud como application/json.",
		"invalid_fields":            "Algunos campos no son válidos.",
		"idempotency_in_progress":   "Una solicitud con esta Idempotency-Key todavía se está ejecutando: inténtelo en un momento.",
		"idempotency_key_reused":    "Esta Idempotency-Key ya se usó en otra solicitud.",
		"idempotency_key_required":  "Envíe una Idempotency-Key, un UUID aleatorio nuevo para cada operación.",
		"precondition_failed":       "El registro cambió desde que lo leyó: léalo de nuevo.",
		"precondition_required":     "Envíe If-Match con el ETag del registro que leyó.",
		"too_many_records":          "Demasiados registros: active la paginación para ellos.",
		"import_refused":            "Algunas filas no son válidas: no se importó nada.",
		"too_many_rows":             "Una importación acepta 5000 filas como máximo.",
		"rate_limited":              "Demasiadas solicitudes; inténtelo más tarde.",
		"internal":                  "Algo salió mal de nuestro lado.",
		"not_ready":                 "El servicio todavía no está listo.",
		"not_implemented":           "La operación no está implementada.",
		"bad_request":               "La solicitud no es un JSON válido con la forma esperada.",
		"invalid_redirect":          "La app no puede enviar a nadie a esa dirección.",
		"invalid_link":              "El enlace no es válido o caducó.",
		"unauthorized":              "Inicie sesión para hacer esto.",
		"invalid_credentials":       "El correo o la contraseña son incorrectos.",
		"forbidden":                 "No puede hacer esto.",
		"csrf":                      "La solicitud no vino de la app.",
		"email_not_verified":        "Verifique su correo primero.",
		"signup_closed":             "El registro no está abierto.",
		"reauthentication_required": "Inicie sesión de nuevo para hacer esto.",
		"email_taken":               "Otra cuenta ya usa este correo.",
		"last_credential":           "Es la única forma de entrar que queda.",
		"too_many_attempts":         "Demasiados intentos. Inténtelo más tarde.",
		"too_soon":                  "Este correo se envió hace poco: se podrá enviar de nuevo más tarde.",
		"provider_not_configured":   "Entrar de esta forma no está configurado.",
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
