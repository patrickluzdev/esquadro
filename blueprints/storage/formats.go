// Package storage is the storage blueprint's Go side: the formats a field
// takes, its largest size, and the checks of a project that uses it.
package storage

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// formats are the formats a field names by extension, with their MIME
// types (the first is the one a file of the format is served with).
var formats = map[string][]string{
	"png":  {"image/png"},
	"jpeg": {"image/jpeg"},
	"jpg":  {"image/jpeg"},
	"webp": {"image/webp"},
	"gif":  {"image/gif"},
	"avif": {"image/avif"},
	"svg":  {"image/svg+xml"},
	"ico":  {"image/x-icon", "image/vnd.microsoft.icon"},
	"pdf":  {"application/pdf"},
	"txt":  {"text/plain"},
	"csv":  {"text/csv"},
	"json": {"application/json"},
	"xml":  {"application/xml", "text/xml"},
	"zip":  {"application/zip"},
	"doc":  {"application/msword"},
	"docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	"xls":  {"application/vnd.ms-excel"},
	"xlsx": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	"ppt":  {"application/vnd.ms-powerpoint"},
	"pptx": {"application/vnd.openxmlformats-officedocument.presentationml.presentation"},
	"odt":  {"application/vnd.oasis.opendocument.text"},
	"ods":  {"application/vnd.oasis.opendocument.spreadsheet"},
	"mp3":  {"audio/mpeg"},
	"wav":  {"audio/wav", "audio/x-wav"},
	"ogg":  {"audio/ogg"},
	"mp4":  {"video/mp4"},
	"webm": {"video/webm"},
	"mov":  {"video/quicktime"},
}

// imageInline are the image formats safe to open in a browser: none of
// them runs script (SVG can, so it is always downloaded).
var imageInline = []string{"image/png", "image/jpeg", "image/webp", "image/gif", "image/avif"}

// Known reports whether a field may name a format: a known extension, or a
// MIME type (type/subtype).
func Known(format string) bool {
	if _, ok := formats[strings.ToLower(format)]; ok {
		return true
	}
	t, sub, ok := strings.Cut(format, "/")
	return ok && t != "" && sub != "" && !strings.ContainsAny(format, " ;,")
}

// MIMETypes are the MIME types of formats, in order and once each.
func MIMETypes(list []string) []string {
	var out []string
	for _, f := range list {
		types, ok := formats[strings.ToLower(f)]
		if !ok {
			types = []string{f}
		}
		for _, t := range types {
			if !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	return out
}

// ParseSize reads a size: 5MB, 500KB, 1GB or a number of bytes; units are
// powers of 1024.
func ParseSize(s string) (int64, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		mult   int64
	}{{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1}} {
		if rest, ok := strings.CutSuffix(t, u.suffix); ok {
			t, mult = strings.TrimSpace(rest), u.mult
			break
		}
	}
	n, err := strconv.ParseInt(t, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a size: write it like 5MB, 500KB or 1GB", s)
	}
	return n * mult, nil
}

// Rules are what a file field takes, its own or the blueprint's: its
// largest size in bytes, its MIME types (any when empty), how long a link
// to read it works, and whether it opens in the browser.
type Rules struct {
	Max        int64
	Types      []string
	URLMinutes int64
	Inline     bool
}

// RulesOf works out a field's rules from its @storage annotation and the
// blueprint's options; sizes and formats are already checked.
func RulesOf(f *project.Field, opts map[string]any) Rules {
	a := f.Ann["storage"]
	kind := "file"
	if f.Type == project.TypeImage {
		kind = "image"
	}
	r := Rules{URLMinutes: intOf(opts["url_minutes"], 60)}
	maxText, _ := opts[kind+"_max"].(string)
	if s, ok := a["max"].(string); ok {
		maxText = s
	}
	r.Max, _ = ParseSize(maxText)
	types := strs(opts[kind+"_types"])
	if t := strs(a["types"]); len(t) > 0 {
		types = t
	}
	r.Types = MIMETypes(types)
	if n, ok := a["url_minutes"].(int64); ok {
		r.URLMinutes = n
	}
	// Inline when asked, or for an image whose formats are all safe.
	switch {
	case a["attachment"] == true:
		r.Inline = false
	case a["inline"] == true:
		r.Inline = true
	default:
		r.Inline = f.Type == project.TypeImage && len(r.Types) > 0
		for _, t := range r.Types {
			if !slices.Contains(imageInline, t) {
				r.Inline = false
			}
		}
	}
	// SVG can run script: never in the browser.
	if slices.Contains(r.Types, "image/svg+xml") {
		r.Inline = false
	}
	return r
}

func intOf(v any, fallback int64) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	}
	return fallback
}

func strs(v any) []string {
	switch l := v.(type) {
	case []string:
		return l
	case []any:
		out := make([]string, 0, len(l))
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
