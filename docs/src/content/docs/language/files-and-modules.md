---
title: Files and modules
description: How a model is spread over .esq files, the app block and modules.
sidebar:
  order: 1
---

A model is the set of `.esq` files in a project's `esquadro/` folder. Their
names and order do not matter, and relations cross files freely.

## The app

Exactly one file has the `app` block: the project's name, its Go module and
the blueprints it is made of, in order.

```esquadro
/// The shop's backend.
app shop {
  module "github.com/acme/shop"

  blueprint app
  blueprint domain
  blueprint http {
    port 9090
  }
}
```

A blueprint is written by name for the built-in ones, `"./path"` for a local
folder (relative to `esquadro/`), or as a git source such as
`"github.com/acme/blueprints/stripe@v1.2.0"`. Options go between braces; each
blueprint's page in the [reference](/esquadro/blueprints/overview/) lists
them.

## Modules

Every other file starts with `package` and belongs to that module. A module
becomes a Go package (`internal/catalog`) with its entities and use cases;
several files may share a module.

```esquadro
package catalog

entity Product {
  name string required
}
```

Modules keep to themselves: an enum is used only in its own module, and a
module does not know who points at it (see
[relations](/esquadro/language/relations/#across-modules)).

## Names

Names use ASCII letters, digits and `_`. Write them as you like:
`orderItem`, `order_item` and `OrderItem` are the same name, which esquadro
writes as Go and SQL expect (`OrderItem`, `order_items`, `orderItem`...).
Plurals are English and follow the last word (`OrderItem` → `order_items`,
`Person` → `people`); `(plural "...")` after an entity's name sets it.

## Formatting

`esquadro fmt` writes every `.esq` file in one style: two-space indentation
and aligned columns. `esquadro fmt -w` rewrites the files; the
[VS Code extension](/esquadro/editors/) does it on save.
