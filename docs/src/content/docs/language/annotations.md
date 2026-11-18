---
title: Annotations and comments
description: Settings for one blueprint on one element, and comments that become docs.
sidebar:
  order: 4
---

## Annotations

An annotation gives a blueprint a setting for one element: the app, an
entity, a field, a relation, a [job](/esquadro/language/jobs/) or a value of
an enum (after it: `ACTIVE @admin(label "Active")`). It is named after the
blueprint:

```esquadro
entity Product {
  name   string @postgres(column product_name)
  status Status

  @postgres(index [name, status], table items)
}
```

Each blueprint declares the annotations it understands, with their keys and
types; esquadro checks them like everything else, and an annotation for a
blueprint the app does not use is an error. The keys of each blueprint are
in its [reference page](/esquadro/blueprints/overview/).

## Comments

`//` and `/* ... */` are comments. `///` before the app, an entity, an enum,
a job or a field is documentation: it goes into the generated Go code, and an
entity's also into the API's contract.

```esquadro
/// Something the store sells.
entity Product {
  /// Shown in lists.
  name string required
}
```
