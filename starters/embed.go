// Package starters holds the projects esquadro new can start from, one
// folder per starter: about (one line on what it is for), app.esq.tmpl (the
// app, a text/template given the project's Name, Label, Module and Git) and
// .esq files copied as they are (an example model).
package starters

import "embed"

// FS contains the starters.
//
//go:embed api worker cli
var FS embed.FS
