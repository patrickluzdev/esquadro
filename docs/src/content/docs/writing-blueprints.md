---
title: Writing a blueprint
description: Templates, slots and hooks for your own kind of generated code.
---

A blueprint is a folder. Writing one needs no Go: templates use Go's
[text/template](https://pkg.go.dev/text/template), and the manifest is
written in the same language as the model.

## Start one

In a project:

```sh
esquadro blueprint new billing
```

writes a working blueprint in `esquadro/blueprints/billing/`, adds it to the
app, and gives it a test project. It has an option, a slot, a template for
the whole project, one for each entity, one written once for the user, and
code added to the app's Taskfile: change them to make it yours. Run
`esquadro gen` to see what it generates in the project.

```
billing/
  blueprint.esq          the manifest
  templates/             mirrors the generated project
    internal/{{.Module.Snake}}/invoice.go.tmpl
    docs/billing.md.once.tmpl
  slots/                 code added to other blueprints' slots
    http.routes.tmpl
  hooks/                 programs run before or after generating (optional)
    stamp
```

`testdata/` holds its [test project](#testing) (`esquadro blueprint new`
writes it). Use it from a project with a path relative to `esquadro/`:

```esquadro
blueprint "./blueprints/billing" {
  currency "EUR"
}
```

## When to write a new one

A blueprint is **one capability a project turns on or off as a unit**. Write a
new one when it makes sense to want it without the others, when it brings a
library or a tool of its own, or when it changes for a reason of its own.
Merge into one when two are always used together, are configured together,
or one needs to know the inside of the other.

## The manifest

```esquadro
/// Invoices for each order: what this blueprint does, for its users.
blueprint billing {
  version "0.1.0"
  requires domain, store
  optional i18n

  /// Currency of the invoices.
  option currency string default("BRL")
  /// Days to pay.
  option due_days int default(30)

  annotation entity {
    /// Field that holds the amount to invoice.
    amount field
  }

  /// Lines of an invoice's footer.
  slot "billing.footer"

  go_require "github.com/shopspring/decimal" "v1.4.0"

  hook post-gen "hooks/stamp" writes "billing/stamp.txt"
}
```

| Line | |
|---|---|
| `version` | the blueprint's version, `1.2.3` |
| `requires a, b` | blueprints the app must use too |
| `optional a, b` | blueprints it works with when the app has them: it comes after them, and its contributions to their slots are dropped without them |
| `option name type mods` | an option: type `string`, `int`, `number`, `bool`, `name` or `list`; `default(v)` or `required` |
| `annotation app\|entity\|field\|relation { key type }` | the keys of `@billing(...)` on that element; key types also `field` and `fields` (names of the entity's fields) |
| `slot "billing.x"` | a point where other blueprints add code; its name starts with the blueprint's |
| `go_require "module" "version"` | a Go module the generated code needs at least |
| `hook pre-gen\|post-gen "path"` | a program to run (see [Hooks](#hooks)) |

`///` comments are the blueprint's documentation: keep them, they are what
users read.

## Templates

Every file under `templates/` is rendered to the same path in the project.
`.tmpl` files are templates; others are copied as they are.

- **Per element:** a path that uses `.Entity`, `.Enum`, `.Module` or
  `.Relation` is rendered once for each of them:
  `internal/{{.Entity.Module.Snake}}/{{.Entity.Snake}}_invoice.go.tmpl`.
- **The user's:** `name.once.tmpl` is written once; esquadro never changes it
  again.
- **Conditional:** a template whose output is only whitespace writes no file.
- `.go` files are formatted with gofmt.

A template sees:

| | |
|---|---|
| `.App` | the app: `.Label`, `.Kebab`, `.Snake`, `.Module` (the Go module) |
| `.Modules`, `.Entities`, `.Enums` | everything in the model |
| `.Module`, `.Entity`, `.Enum`, `.Relation` | the element of a per-element template |
| `.Options` | this blueprint's options, defaults included |

An entity has its `.Fields` and `.Relations`; names come in every form
(`.Pascal`, `.Camel`, `.Snake`, `.Kebab`, `.Label`, `.Plural.Pascal`...).

Functions, besides text/template's:

| | |
|---|---|
| `slot "name" .` | the contributions to a slot, joined by lines |
| `slotParts "name" .` | the same, as a list, to join your own way |
| `uses "name"` | whether the app uses a blueprint |
| `optionsOf "name"` | another blueprint's options |
| `ann .` | this blueprint's annotation values on an element |
| `importBlock "fmt" (list...)` | a Go import block, standard library first, without duplicates |
| `goType .`, `goDoc`, `quote`, `rawQuote` | Go types, doc comments, string literals |
| `lower`, `upper`, `join`, `contains`, `hasPrefix`, `trimPrefix`, `indent` | text |
| `strs`, `goStrings` | a list option as strings, or as a Go `[]string` literal |

## Slots

A slot is a point where others add code. The blueprint that owns it opens it
in a template:

```go
func (d dependencies) routes() (http.Handler, error) {
	mux := http.NewServeMux()
{{- with slot "http.routes" .}}
{{.}}
{{- end}}
	...
```

Another blueprint contributes with a file named after the slot,
`slots/http.routes.tmpl` (or several, in `slots/http.routes/`):

```go
	mux.HandleFunc("GET /invoices", invoices(d))
```

Contributions are rendered with the caller's data and the contributor's own
options, in the order of the app's blueprints. Contributing to a slot nobody
declares is an error, unless the slot's blueprint is `optional` and the app
does not use it. The [reference](/esquadro/blueprints/overview/) lists every
built-in slot.

## Hooks

A hook is a program the blueprint ships, for what templates cannot do: run a
code generator, write a file computed from the model.

- **`pre-gen`** runs before writing, and may add files to the generation (its
  output's `files`).
- **`post-gen`** runs after writing. It declares the files it may change:
  `writes "glob"` for files it generates (tracked like any generated file) and
  `edits "glob"` for files that stay the user's (like `go.mod`). Hooks that
  write run first, then those that edit, then those that change no file.

A hook reads JSON on stdin, runs in the project's folder, and answers JSON on
stdout:

```json
{
  "event": "post-gen",
  "blueprint": "billing",
  "project": { "app": {}, "modules": [], "entities": [], "enums": [] },
  "previous": null,
  "options": { "currency": "BRL", "due_days": 30 },
  "blueprints": { "postgres": { "pool_max": 10 } },
  "files": ["internal/catalog/product.go"],
  "go_requires": [],
  "project_dir": "/home/me/shop",
  "dry_run": false
}
```

```json
{
  "messages": [{ "level": "info", "text": "stamped" }],
  "files": [{ "path": "billing/stamp.txt", "content": "...", "once": false }]
}
```

`previous` is the model of the last generation (`null` the first time): what
a hook compares with to write, for example, a migration.

## Testing

A blueprint's test project is in `testdata/project/`: an `esquadro/` folder
with an app that uses the blueprint, written `blueprint "../../.."` (its
folder, from there), and a small model. `testdata/expected/` holds the files
the blueprint generates for it.

```sh
esquadro blueprint test billing          # or a folder: esquadro blueprint test ./path
esquadro blueprint test -update billing  # the new output is right: make it the expected one
```

`esquadro blueprint test` generates the test project in a temporary folder
and compares the files the blueprint wrote with the expected ones, showing
the differences. Run it in the blueprint's CI too. The files other
blueprints write, with the code yours adds to their slots, are not compared:
check those in the project with `esquadro gen -dry-run`.

## Publishing

Push the blueprint to a git repository, in its root or a folder, and tag
versions:

```esquadro
blueprint "github.com/acme/esquadro-billing@v0.1.0"
blueprint "github.com/acme/blueprints/billing@v0.1.0"
```

Users get the commit the tag names, pinned in their `esquadro/lock.json` until
`esquadro blueprint update`. If the blueprint has hooks, esquadro shows them and
asks before running them, once per commit: keep them short and readable.
