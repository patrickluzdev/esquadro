package i18n

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// tagPattern is a language tag the way browsers send it in Accept-Language
// and read it in Content-Language: a language, and maybe a region (en,
// pt-BR, es-419).
var tagPattern = regexp.MustCompile(`^[a-z]{2,3}(-([A-Z]{2}|[0-9]{3}))?$`)

// Check refuses languages that are not language tags: a translation is
// found by its tag, and Content-Language answers with it.
func Check(_ *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	bp := set.Get("i18n")
	if bp == nil {
		return nil
	}
	pos := bp.Ref.Pos
	for _, o := range bp.Ref.Options {
		if o.Key.Name == "languages" {
			pos = o.Pos
		}
	}
	languages, _ := set.Options["i18n"]["languages"].([]any)
	for _, l := range languages {
		lang, _ := l.(string)
		if tagPattern.MatchString(lang) {
			continue
		}
		d := diags.Errorf(pos, pos, "language %q is not a language tag", lang)
		if tag := TagOf(lang); tagPattern.MatchString(tag) {
			d.Suggestion = fmt.Sprintf("write %q", tag)
		}
	}
	return diags
}

// TagOf writes a language the way a tag is written: pt_br and PT-br are
// pt-BR.
func TagOf(lang string) string {
	base, region, ok := strings.Cut(strings.ReplaceAll(lang, "_", "-"), "-")
	if !ok {
		return strings.ToLower(base)
	}
	return strings.ToLower(base) + "-" + strings.ToUpper(region)
}
