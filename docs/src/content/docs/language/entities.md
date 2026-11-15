---
title: Entities and fields
description: Entities, their fields, types, rules and enums.
sidebar:
  order: 2
---

An entity is something the module stores and works with. Each line is a
field: its name, its type, and rules.

```esquadro
/// Something the store sells.
entity Product {
  name     string  required min(2) max(120)
  sku      string  unique pattern("^[A-Z]{2}-[0-9]+$")
  price    decimal required min(0)
  stock    int     default(0) min(0)
  status   Status  default(ACTIVE)
  launched date    default(now)
  details  json
}
```

Every entity also has an `id` (a UUID v7, which sorts by creation time),
`created_at` and `updated_at`; they are not written.

## Types

| Type | Go | Postgres |
|---|---|---|
| `string` | `string` | `text` |
| `text` | `string` | `text` (long text) |
| `int` | `int` | `integer` |
| `int64` | `int64` | `bigint` |
| `decimal` | `decimal.Decimal` | `numeric` |
| `bool` | `bool` | `boolean` |
| `time` | `time.Time` | `timestamptz` |
| `date` | `time.Time` | `date` |
| `uuid` | `uuid.UUID` | `uuid` |
| `json` | `json.RawMessage` | `jsonb` |
| an enum | its Go type | a Postgres enum |

## Rules

| Rule | Meaning |
|---|---|
| `required` | must have a value; the column is `NOT NULL` |
| `unique` | no two records share it |
| `min(n)`, `max(n)` | the length for text, the value for numbers |
| `pattern("re")` | text must match the regular expression |
| `default(v)` | the value when none is given; `now` for `time` and `date` |

Rules are checked in Go when a record is created or changed, and refused
fields come back all at once: in the API as
[`invalid_fields`](/esquadro/blueprints/reference/rest/) with a code per field
(`required`, `too_short`, `too_long`, `too_small`, `too_large`, `pattern`,
`invalid_value`, `not_found`, `taken`).

## Enums

```esquadro
enum Status {
  ACTIVE
  INACTIVE
  SOLD_OUT
}
```

An enum is a type of its module; a field uses it by name, and its default is
one of its values.
