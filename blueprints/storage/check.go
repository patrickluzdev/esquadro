package storage

import (
	"slices"
	"strings"
	"time"

	"github.com/patrickluzdev/esquadro/internal/blueprint"
	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/project"
)

// Check checks the storage blueprint's options (sizes, formats, how keys
// are made, how often the cleanup runs) and each field's @storage: on a
// file or an image only, with a size and formats it can read.
func Check(p *project.Project, set *blueprint.Set) diag.List {
	var diags diag.List
	for _, b := range set.Blueprints {
		if b.Manifest.Name != "storage" {
			continue
		}
		opts := set.Options[b.Manifest.Name]
		at := func(key string) diag.Pos {
			for _, o := range b.Ref.Options {
				if o.Key.Name == key {
					return o.Pos
				}
			}
			return b.Ref.Pos
		}
		for _, key := range []string{"image_max", "file_max"} {
			if s, _ := opts[key].(string); s != "" {
				if _, err := ParseSize(s); err != nil {
					diags.Errorf(at(key), at(key), "%s: %v", key, err)
				}
			}
		}
		for _, key := range []string{"image_types", "file_types"} {
			for _, f := range strs(opts[key]) {
				if !Known(f) {
					diags.Errorf(at(key), at(key), "%s: %s is no format esquadro knows", key, f).
						Suggestion = "an extension (png, pdf, docx) or a MIME type (application/x-custom)"
				}
			}
		}
		if s, _ := opts["clean_every"].(string); s != "" {
			if d, err := time.ParseDuration(s); err != nil || d < time.Minute {
				diags.Errorf(at("clean_every"), at("clean_every"), "clean_every: %q is not a duration of a minute or more", s).
					Suggestion = "e.g. 1h or 30m"
			}
		}
		check := func(key string, allowed ...string) {
			if v, _ := opts[key].(string); v != "" && !slices.Contains(allowed, v) {
				diags.Errorf(at(key), at(key), "%s is %s or %s, not %s", key, strings.Join(allowed[:len(allowed)-1], ", "), allowed[len(allowed)-1], v)
			}
		}
		check("local", "disk", "minio")
		for _, d := range strs(opts["drivers"]) {
			if !slices.Contains([]string{"s3", "gcs", "azure"}, d) {
				diags.Errorf(at("drivers"), at("drivers"), "drivers: %s is not s3, gcs or azure", d)
			}
		}
		if l, _ := opts["local"].(string); l == "minio" && !slices.Contains(strs(opts["drivers"]), "s3") {
			diags.Errorf(at("local"), at("local"), "local minio speaks S3: drivers needs s3")
		}
		check("key_layout", "entity", "random")
		for _, key := range []string{"url_minutes", "upload_minutes", "pending_hours"} {
			if n, ok := opts[key].(int64); ok && n < 1 {
				diags.Errorf(at(key), at(key), "%s must be 1 or more", key)
			}
		}
	}
	for _, e := range p.Entities {
		for _, f := range e.Fields {
			a := f.Ann["storage"]
			if len(a) == 0 {
				continue
			}
			pos := f.Pos
			if !f.IsFile() {
				diags.Errorf(pos, pos, "@storage is for file and image fields; %s.%s is %s", e.Raw, f.Raw, f.Type).
					Suggestion = "make it a file or an image, or remove @storage"
				continue
			}
			if s, ok := a["max"].(string); ok {
				if _, err := ParseSize(s); err != nil {
					diags.Errorf(pos, pos, "%s.%s: max %v", e.Raw, f.Raw, err)
				}
			}
			for _, t := range strs(a["types"]) {
				if !Known(t) {
					diags.Errorf(pos, pos, "%s.%s: %s is no format esquadro knows", e.Raw, f.Raw, t).
						Suggestion = "an extension (png, pdf, docx) or a MIME type (application/x-custom)"
				} else if f.Type == project.TypeImage {
					for _, m := range MIMETypes([]string{t}) {
						if !strings.HasPrefix(m, "image/") {
							diags.Errorf(pos, pos, "%s.%s is an image, and %s is not an image format", e.Raw, f.Raw, t).
								Suggestion = "make the field a file to take other formats"
						}
					}
				}
			}
			if a["inline"] == true && a["attachment"] == true {
				diags.Errorf(pos, pos, "%s.%s is inline or attachment, not both", e.Raw, f.Raw)
			}
			if n, ok := a["url_minutes"].(int64); ok && n < 1 {
				diags.Errorf(pos, pos, "%s.%s: url_minutes must be 1 or more", e.Raw, f.Raw)
			}
		}
	}
	return diags
}
