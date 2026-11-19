// Package ui is what the frontends of an app share: the annotation @ui (how
// people call the model's parts, their icons and colors), the labels a
// frontend shows, the theme's colors and the checks of the frontends'
// options, alone and together.
package ui

import (
	"maps"

	"github.com/patrickluzdev/esquadro/internal/project"
)

// Ann is what a frontend (instance, e.g. admin) knows of a node: @ui's
// values, and its own annotation's over them.
func Ann(a project.Ann, instance string) map[string]any {
	ui, own := a["ui"], a[instance]
	if ui == nil {
		return own
	}
	out := maps.Clone(ui)
	maps.Copy(out, own)
	return out
}

// Label is what a frontend calls a node with these annotations: its label,
// or fallback.
func Label(a project.Ann, instance, fallback string) string {
	if l, _ := Ann(a, instance)["label"].(string); l != "" {
		return l
	}
	return fallback
}

// Labeled reports whether a frontend has a label for a node.
func Labeled(a project.Ann, instance string) bool {
	l, _ := Ann(a, instance)["label"].(string)
	return l != ""
}
