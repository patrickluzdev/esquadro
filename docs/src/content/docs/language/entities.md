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
| `money` | `currency.Amount` | `numeric(19,4)` and a `char(3)` currency |
| `file`, `image` | the file's id | a `uuid` to the [stored file](/esquadro/blueprints/reference/storage/) |
| `bool` | `bool` | `boolean` |
| `time` | `time.Time` | `timestamptz` |
| `date` | `time.Time` | `date` |
| `uuid` | `uuid.UUID` | `uuid` |
| `json` | `json.RawMessage` | `jsonb` |
| an enum | its Go type | a Postgres enum |

## Money

A `money` field is an exact amount and its currency (an ISO 4217 code).
Its currency is the app's, set once in the app block, unless a value comes
with another:

```esquadro
app shop {
  module "github.com/you/shop"
  currency BRL
}

entity Product {
  price money required min(0)
}
```

It is stored as two columns, `price` (`numeric(19,4)`: four places, for
prices finer than a cent) and `price_currency`, and travels in the API as
`{"amount": "19.90", "currency": "BRL"}`, the amount always written as
text, never as a JSON number. In Go it is a
[`currency.Amount`](https://pkg.go.dev/github.com/bojanz/currency). `min`,
`max` and `default` apply to the amount; a money field cannot be `unique`.

## Files

A `file` or an `image` field is a file kept by the
[storage](/esquadro/blueprints/reference/storage/) blueprint, which the app
must use: an image takes png, jpeg, webp, gif and avif up to 10MB, a file
any format up to 25MB, both changed in the blueprint's options or by the
field's `@storage`:

```esquadro
entity Product {
  photo  image required @storage(max "5MB")
  manual file  @storage(max "20MB", types [pdf], url_minutes 5)
}
```

A file field cannot be `unique`, have a `default`, `min`, `max` or a
`pattern`, nor be a job's argument or a report's column.

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
one of its values. A value can carry annotations after it, e.g. the label a
frontend shows: `ACTIVE @ui(label "Active")`.
