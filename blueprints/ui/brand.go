package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Brand is a project's identity, in a folder of its own (brand "brand" in a
// frontend's options): its theme.json (colors, fonts, text roles, radii,
// buttons, motion, the pictures of the states) and the files it names
// (logo, favicon, fonts). The folder is embedded in the app, which serves
// it at /brand/.
type Brand struct {
	// Dir is the folder, relative to the project: brand.
	Dir string
	// Root is the project's folder.
	Root  string
	Theme BrandTheme
}

// BrandTheme is a brand's theme.json.
type BrandTheme struct {
	// Colors are by role: accent, on_accent, accent_edge, accent_text,
	// accent_soft, ink, muted, page, surface, line, neutral, neutral_edge,
	// ok, ok_soft, error, error_soft, warning, warning_soft, info,
	// info_soft. Any other name is a color of its own (brand, sale), as
	// --color-<name>.
	Colors map[string]string `json:"colors"`
	// Fonts are the display (titles) and body fonts, served by the
	// app.
	Fonts map[string]BrandFont `json:"fonts"`
	// Text are the text roles, by names of the project's own, each a
	// class .t-<name>; the kit's titles and texts carry .t-section,
	// .t-block, .t-item and .t-text.
	Text map[string]TextRole `json:"text"`
	// Radius are the corners: sm, md, lg, in px.
	Radius map[string]float64 `json:"radius"`
	// Buttons is flat (by default) or raised: a 2 px edge at the bottom
	// that sinks when pressed.
	Buttons string `json:"buttons"`
	// Control is the height of buttons and fields, in px.
	Control float64 `json:"control"`
	// Motion are named durations and curves: enter, exit.
	Motion map[string]Motion `json:"motion"`
	// States are the pictures of the states: empty, loading, success,
	// error, not_found, forbidden, gone; files of the brand folder.
	States map[string]string `json:"states"`
	// Identity is what names the site: its logos, the folder whose files
	// are also served at the root (favicon.ico, site.webmanifest), the
	// shared links' default picture and the browser's color.
	Identity BrandIdentity `json:"identity"`
}

// BrandFont is a font: its family and its files, by weight ("400",
// "700", or "100 900" for a variable font).
type BrandFont struct {
	Family   string            `json:"family"`
	Files    map[string]string `json:"files"`
	Fallback string            `json:"fallback"`
}

// TextRole is a text role: its font (display or body), weight, size in px,
// line height and letter spacing in em.
type TextRole struct {
	Font     string  `json:"font"`
	Weight   int     `json:"weight"`
	Size     float64 `json:"size"`
	Line     float64 `json:"line"`
	Tracking float64 `json:"tracking"`
	Color    string  `json:"color"`
}

// Motion is a duration (ms) and a CSS easing curve.
type Motion struct {
	Duration int    `json:"duration"`
	Curve    string `json:"curve"`
}

// BrandIdentity is what names the site, and its emails.
type BrandIdentity struct {
	Logo string `json:"logo"`
	// LogoEmail is the logo the emails show, a PNG, JPEG or GIF: email
	// clients do not show SVG. Without it, the emails show Logo when it is
	// one of those, else the app's name.
	LogoEmail  string `json:"logo_email"`
	LogoOnDark string `json:"logo_on_dark"`
	Symbol     string `json:"symbol"`
	Icons      string `json:"icons"`
	OGImage    string `json:"og_image"`
	ThemeColor string `json:"theme_color"`
}

// ColorRoles are the colors a theme names by role, in order.
var ColorRoles = []string{"accent", "on_accent", "accent_edge", "accent_text", "accent_soft", "ink", "muted", "page", "surface",
	"line", "neutral", "neutral_edge", "ok", "ok_soft", "error", "error_soft", "warning", "warning_soft", "info", "info_soft"}

// roleName is the name of a text role or a color of its own.
var roleName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// StateNames are the states a theme may picture.
var StateNames = []string{"empty", "loading", "success", "error", "not_found", "forbidden", "gone"}

// roleDefaults are the colors of a role a theme leaves out: the neutral
// look the frontends have without a brand.
var roleDefaults = map[string]string{
	"accent": "#2a85ff", "on_accent": "#ffffff", "accent_edge": "", "accent_text": "#1d5fbf", "accent_soft": "#eaf2ff",
	"ink": "#0a0a0a", "muted": "#71717a", "page": "#ffffff", "surface": "#ffffff", "line": "#e4e4e7", "neutral": "#f4f4f5",
	"neutral_edge": "#e4e4e7", "ok": "#15803d", "ok_soft": "#ecfdf3", "error": "#dc2626",
	"error_soft": "#fef2f2", "warning": "#a16207", "warning_soft": "#fefce8", "info": "#1d4ed8", "info_soft": "#eff6ff",
}

// BrandOf is the brand of a frontend whose options name its folder, read
// from the project the app.esq at appFile is in; nil without one.
func BrandOf(appFile string, opts map[string]any) (*Brand, error) {
	dir, _ := opts["brand"].(string)
	dir = strings.Trim(dir, "/")
	if dir == "" || appFile == "" {
		return nil, nil
	}
	root := filepath.Dir(filepath.Dir(appFile))
	b := &Brand{Dir: dir, Root: root}
	data, err := os.ReadFile(filepath.Join(root, dir, "theme.json"))
	if err != nil {
		return nil, fmt.Errorf("brand %s: %w", dir, err)
	}
	if err := json.Unmarshal(data, &b.Theme); err != nil {
		return nil, fmt.Errorf("brand %s: theme.json: %w", dir, err)
	}
	return b, nil
}

// Problems are what is wrong with a brand: colors that are none, roles
// and states that do not exist, files that are not in its folder.
func (b *Brand) Problems() []string {
	var out []string
	t := b.Theme
	for name, c := range t.Colors {
		if !hexColor.MatchString(c) {
			out = append(out, fmt.Sprintf("colors.%s %q is not a color: #rgb or #rrggbb", name, c))
		}
		if !roleName.MatchString(name) {
			out = append(out, fmt.Sprintf("colors.%s: a color's name is lowercase, as accent or casa", name))
		}
	}
	for name, f := range t.Fonts {
		if name != "display" && name != "body" {
			out = append(out, fmt.Sprintf("fonts.%s: the fonts are display and body", name))
		}
		if f.Family == "" {
			out = append(out, fmt.Sprintf("fonts.%s has no family", name))
		}
		for weight, file := range f.Files {
			if !regexp.MustCompile(`^\d{3}( \d{3})?$`).MatchString(weight) {
				out = append(out, fmt.Sprintf("fonts.%s.files: %q is not a weight: 400, or 100 900 for a variable font", name, weight))
			}
			out = append(out, b.missing("fonts."+name+".files."+weight, file)...)
		}
	}
	for name, r := range t.Text {
		if !roleName.MatchString(name) {
			out = append(out, fmt.Sprintf("text.%s: a role's name is lowercase, as section or caption", name))
		}
		if r.Font != "" && r.Font != "display" && r.Font != "body" {
			out = append(out, fmt.Sprintf("text.%s.font %q is display or body", name, r.Font))
		}
		if r.Size <= 0 {
			out = append(out, fmt.Sprintf("text.%s has no size", name))
		}
	}
	for name := range t.Radius {
		if name != "sm" && name != "md" && name != "lg" {
			out = append(out, fmt.Sprintf("radius.%s: the radii are sm, md and lg", name))
		}
	}
	if t.Buttons != "" && t.Buttons != "flat" && t.Buttons != "raised" {
		out = append(out, fmt.Sprintf("buttons %q is flat or raised", t.Buttons))
	}
	for name, file := range t.States {
		if !slices.Contains(StateNames, name) {
			out = append(out, fmt.Sprintf("states.%s is not a state: %s", name, strings.Join(StateNames, ", ")))
		}
		out = append(out, b.missing("states."+name, file)...)
	}
	id := t.Identity
	for key, file := range map[string]string{"logo": id.Logo, "logo_email": id.LogoEmail, "logo_on_dark": id.LogoOnDark, "symbol": id.Symbol, "og_image": id.OGImage} {
		if file != "" {
			out = append(out, b.missing("identity."+key, file)...)
		}
	}
	if id.LogoEmail != "" && emailImageType(id.LogoEmail) == "" {
		out = append(out, fmt.Sprintf("identity.logo_email %s is not a PNG, JPEG or GIF, the pictures email clients show", id.LogoEmail))
	}
	if id.Icons != "" {
		if st, err := os.Stat(filepath.Join(b.Root, b.Dir, id.Icons)); err != nil || !st.IsDir() {
			out = append(out, fmt.Sprintf("identity.icons: %s is not a folder of %s", id.Icons, b.Dir))
		}
	}
	if id.ThemeColor != "" && !hexColor.MatchString(id.ThemeColor) {
		out = append(out, fmt.Sprintf("identity.theme_color %q is not a color", id.ThemeColor))
	}
	sort.Strings(out)
	return out
}

func (b *Brand) missing(key, file string) []string {
	if file == "" || strings.Contains(file, "..") || path.IsAbs(file) {
		return []string{fmt.Sprintf("%s %q is not a file of %s", key, file, b.Dir)}
	}
	if _, err := os.Stat(filepath.Join(b.Root, b.Dir, filepath.FromSlash(file))); err != nil {
		return []string{fmt.Sprintf("%s: %s is not in %s", key, file, b.Dir)}
	}
	return nil
}

// Color is a color of the brand: a role's, with its default, or one of its
// own; "" for none.
func (b *Brand) Color(name string) string {
	if c, ok := b.Theme.Colors[name]; ok {
		return c
	}
	return roleDefaults[name]
}

// EmailLogo is the logo the emails show, a file of the brand folder, and
// its media type: identity.logo_email, or identity.logo when it is a PNG,
// JPEG or GIF; "" without one, when the emails show the app's name.
func (b *Brand) EmailLogo() (file, mediaType string) {
	for _, f := range []string{b.Theme.Identity.LogoEmail, b.Theme.Identity.Logo} {
		if t := emailImageType(f); t != "" {
			return f, t
		}
	}
	return "", ""
}

// emailImageType is the media type of a picture every email client shows,
// by its extension; "" for any other (an SVG).
func emailImageType(file string) string {
	switch strings.ToLower(path.Ext(file)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	}
	return ""
}

// FontStack is the font-family of a font (display or body) for a page
// that loads no files of the brand, as an email: its family, then its
// fallback or the system's fonts; the system's alone without one. Display
// falls back to body.
func (b *Brand) FontStack(name string) string {
	const system = "-apple-system, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"
	f, ok := b.Theme.Fonts[name]
	if !ok && name == "display" {
		f, ok = b.Theme.Fonts["body"]
	}
	if !ok || f.Family == "" {
		return system
	}
	fallback := f.Fallback
	if fallback == "" {
		fallback = system
	}
	return "'" + strings.NewReplacer("'", "", `\`, "", ";", "", "<", "", ">", "").Replace(f.Family) + "', " + fallback
}

// IconFiles are the files of the icons folder, served at the site's root
// too (favicon.ico, site.webmanifest, the apple-touch-icon): their names.
func (b *Brand) IconFiles() []string {
	if b.Theme.Identity.Icons == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(b.Root, b.Dir, b.Theme.Identity.Icons))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out
}

// HasIcon reports whether the icons folder has a file.
func (b *Brand) HasIcon(name string) bool { return slices.Contains(b.IconFiles(), name) }

// ThemeColor is the browser's color: the identity's, or the accent.
func (b *Brand) ThemeColor() string {
	if b.Theme.Identity.ThemeColor != "" {
		return b.Theme.Identity.ThemeColor
	}
	return b.Color("accent")
}

// px writes a number of pixels.
func px(n float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", n), "0"), ".") + "px"
}

// CSS is the brand's tokens and the look of the kit's pieces, for a
// frontend that serves the brand's files at base (/brand/): the colors by
// role and its own, as variables and as the names the kit's classes use;
// the fonts; the text roles (.t-<role>); radii; motion; and buttons,
// fields, chips, cards, notices and the progress bar.
func (b *Brand) CSS(base string) string {
	var w strings.Builder
	t := b.Theme
	for _, name := range []string{"display", "body"} {
		f, ok := t.Fonts[name]
		if !ok {
			continue
		}
		weights := make([]string, 0, len(f.Files))
		for wt := range f.Files {
			weights = append(weights, wt)
		}
		sort.Strings(weights)
		for _, wt := range weights {
			file := f.Files[wt]
			format := "truetype"
			switch strings.ToLower(path.Ext(file)) {
			case ".woff2":
				format = "woff2"
			case ".woff":
				format = "woff"
			case ".otf":
				format = "opentype"
			}
			fmt.Fprintf(&w, "@font-face { font-family: %q; src: url(%q) format(%q); font-weight: %s; font-style: normal; font-display: swap; }\n",
				f.Family, base+file, format, wt)
		}
	}
	w.WriteString("\n:root {\n")
	for _, role := range ColorRoles {
		if c := b.Color(role); c != "" {
			fmt.Fprintf(&w, "  --brand-%s: %s;\n", strings.ReplaceAll(role, "_", "-"), c)
		}
	}
	own := make([]string, 0)
	for name := range t.Colors {
		if !slices.Contains(ColorRoles, name) {
			own = append(own, name)
		}
	}
	sort.Strings(own)
	for _, name := range own {
		fmt.Fprintf(&w, "  --brand-%s: %s;\n", strings.ReplaceAll(name, "_", "-"), t.Colors[name])
	}
	for _, name := range []string{"display", "body"} {
		if f, ok := t.Fonts[name]; ok {
			fallback := f.Fallback
			if fallback == "" {
				fallback = "ui-sans-serif, system-ui, sans-serif"
			}
			fmt.Fprintf(&w, "  --brand-font-%s: %q, %s;\n", name, f.Family, fallback)
		}
	}
	radius := map[string]float64{"sm": 6, "md": 10, "lg": 14}
	for k, v := range t.Radius {
		radius[k] = v
	}
	for _, k := range []string{"sm", "md", "lg"} {
		fmt.Fprintf(&w, "  --brand-radius-%s: %s;\n", k, px(radius[k]))
	}
	control := t.Control
	if control == 0 {
		control = 40
	}
	fmt.Fprintf(&w, "  --control: %s;\n", px(control))
	motions := make([]string, 0, len(t.Motion))
	for name := range t.Motion {
		motions = append(motions, name)
	}
	sort.Strings(motions)
	for _, name := range motions {
		m := t.Motion[name]
		fmt.Fprintf(&w, "  --motion-%s: %dms;\n  --ease-%s: %s;\n", name, m.Duration, name, m.Curve)
	}
	// The names the kit's utility classes read (bg-primary, border-border).
	w.WriteString("  --radius: var(--brand-radius-md);\n")
	w.WriteString("  --background: var(--brand-page);\n  --foreground: var(--brand-ink);\n  --muted: var(--brand-neutral);\n  --muted-foreground: var(--brand-muted);\n")
	w.WriteString("  --border: var(--brand-line);\n  --destructive: var(--brand-error);\n  --primary: var(--brand-accent);\n  --primary-foreground: var(--brand-on-accent);\n  --ring: var(--brand-accent-text);\n")
	w.WriteString("}\n\n")

	w.WriteString("@theme inline {\n")
	for _, role := range ColorRoles {
		if role == "muted" {
			continue // muted is the kit's background of quiet things: text-muted-foreground is this one
		}
		fmt.Fprintf(&w, "  --color-%s: var(--brand-%s);\n", strings.ReplaceAll(role, "_", "-"), strings.ReplaceAll(role, "_", "-"))
	}
	for _, name := range own {
		fmt.Fprintf(&w, "  --color-%s: var(--brand-%s);\n", strings.ReplaceAll(name, "_", "-"), strings.ReplaceAll(name, "_", "-"))
	}
	w.WriteString("}\n\n")
	// Tailwind's own tokens, with their values: font-display, rounded-lg.
	w.WriteString("@theme {\n")
	for _, name := range []string{"display", "body"} {
		if _, ok := t.Fonts[name]; ok {
			fmt.Fprintf(&w, "  --font-%s: var(--brand-font-%s);\n", name, name)
		}
	}
	for _, k := range []string{"sm", "md", "lg"} {
		fmt.Fprintf(&w, "  --radius-%s: %s;\n", k, px(radius[k]))
	}
	w.WriteString("}\n\n")

	// The kit's pieces. Outside any layer, so that they win over the
	// utility classes the kit's markup also carries.
	body := "inherit"
	if _, ok := t.Fonts["body"]; ok {
		body = "var(--brand-font-body)"
	}
	display := body
	if _, ok := t.Fonts["display"]; ok {
		display = "var(--brand-font-display)"
	}
	fmt.Fprintf(&w, "body { font-family: %s; background: var(--brand-page); color: var(--brand-ink); }\n", body)
	roles := make([]string, 0, len(t.Text))
	for name := range t.Text {
		roles = append(roles, name)
	}
	sort.Strings(roles)
	for _, role := range roles {
		r := t.Text[role]
		font := body
		if r.Font == "display" {
			font = display
		}
		line := r.Line
		if line == 0 {
			line = 1.4
		}
		weight := r.Weight
		if weight == 0 {
			weight = 400
		}
		fmt.Fprintf(&w, ".t-%s { font-family: %s; font-weight: %d; font-size: %s; line-height: %s;", role, font, weight, px(r.Size),
			strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", line), "0"), "."))
		if r.Tracking != 0 {
			fmt.Fprintf(&w, " letter-spacing: %sem;", strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", r.Tracking), "0"), "."))
		}
		if r.Color != "" {
			fmt.Fprintf(&w, " color: var(--brand-%s);", strings.ReplaceAll(r.Color, "_", "-"))
		}
		w.WriteString(" }\n")
	}
	raised := t.Buttons == "raised"
	edge := func(c string) string {
		if !raised {
			return ""
		}
		return fmt.Sprintf(" box-shadow: inset 0 -2px 0 %s;", c)
	}
	accentEdge := "var(--brand-accent-edge)"
	if b.Color("accent_edge") == "" {
		accentEdge = "rgb(0 0 0 / .18)"
	}
	fmt.Fprintf(&w, `.btn { display: inline-flex; align-items: center; justify-content: center; gap: 7px; min-height: var(--control); padding: 0 16px; border: 0; border-radius: var(--brand-radius-md); font: 700 14px/1 %[1]s; white-space: nowrap; cursor: pointer; text-decoration: none; transition: transform var(--motion-press, 80ms); }
.btn:active { transform: translateY(%[2]s); }
.btn-primary { background: var(--brand-accent); color: var(--brand-on-accent);%[3]s }
.btn-secondary { background: var(--brand-neutral); color: var(--brand-ink);%[4]s }
.btn-icon { background: var(--brand-neutral); color: var(--brand-ink); width: var(--control); padding: 0;%[4]s }
.btn-wide { width: 100%%; }
.btn:disabled { opacity: .6; cursor: default; transform: none; }
.field { height: var(--control); width: 100%%; border: 0; border-radius: var(--brand-radius-md); background: var(--brand-surface); color: var(--brand-ink); padding: 0 14px; font: 400 15px/1 %[1]s; box-shadow: inset 0 0 0 1.5px var(--brand-neutral-edge); outline: none; }
.field:focus, .field:focus-within { box-shadow: inset 0 0 0 1.5px var(--brand-accent-text), 0 0 0 3px var(--brand-accent-soft); }
.field[aria-invalid="true"] { box-shadow: inset 0 0 0 1.5px var(--brand-error), 0 0 0 3px var(--brand-error-soft); }
.chip { display: inline-flex; align-items: center; gap: 6px; border-radius: var(--brand-radius-sm); padding: 5px 11px; font: 600 13px/1.2 %[1]s; background: var(--brand-surface); color: var(--brand-ink); box-shadow: inset 0 0 0 1.5px var(--brand-line); text-decoration: none; }
.chip[aria-current], .chip.is-on { background: var(--brand-accent-soft); color: var(--brand-accent-text); }
.card { background: var(--brand-surface); border-radius: var(--brand-radius-lg); box-shadow: 0 0 0 1px var(--brand-line); }
.notice { display: flex; gap: 10px; align-items: flex-start; border-radius: var(--brand-radius-md); padding: 10px 12px; font: 500 13px/1.4 %[1]s; }
.notice-ok { background: var(--brand-ok-soft); color: var(--brand-ok); } .notice-error { background: var(--brand-error-soft); color: var(--brand-error); }
.notice-warning { background: var(--brand-warning-soft); color: var(--brand-warning); } .notice-info { background: var(--brand-info-soft); color: var(--brand-info); }
.progress { width: 100%%; height: 6px; border-radius: 99px; background: var(--brand-neutral); overflow: hidden; }
.progress > i { display: block; height: 100%%; background: var(--brand-accent-text); border-radius: 99px; }
.btn.hidden, .chip.hidden, .notice.hidden, .progress.hidden { display: none; }
.state-picture { width: 140px; height: auto; margin: 0 auto 16px; display: block; }
`, body, map[bool]string{true: "2px", false: "1px"}[raised], edge(accentEdge), edge("var(--brand-neutral-edge)"))
	return w.String()
}
