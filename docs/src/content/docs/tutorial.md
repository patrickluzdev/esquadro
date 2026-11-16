---
title: Tutorial
description: Grow a shop's model step by step, and see what each change generates.
---

This tutorial starts from the `api` starter and grows the model of a shop: a new
field, a new entity, a many-to-many relation, a second module, and an option.
After each step you run the same two commands, and you will see what esquadro
changed.

You need esquadro, Go, Task and Docker (see
[Getting started](/esquadro/getting-started/)).

## 1. Start

```sh
esquadro new -starter api shop
cd shop
task up
task migrate
go run ./cmd/shop security create user you@example.com name=You -role admin -password-stdin
task run
```

The `security create` line makes the first admin; type a password of at least
12 characters and press Enter. Leave `task run` in a terminal; in another, sign
in, and the API answers:

```sh
TOKEN=$(curl -s localhost:8080/identity/user/sign-in -H 'content-type: application/json' \
  -d '{"email":"you@example.com","password":"your password","cookie":false}' | jq -r .session_token)
curl -X POST localhost:8080/api/v1/categories -H "authorization: Bearer $TOKEN" \
  -H 'content-type: application/json' -d '{"name":"Books"}'
```

Without `jq`, copy `session_token` from the sign-in's answer. Anyone may read
the catalog; only an admin changes categories, and staff too products, as the
`@security` lines of `esquadro/catalog.esq` say. The calls below send the same
`authorization` header, left out for short.

The model is in `esquadro/catalog.esq`: a `Category` and a `Product` with a
`Status`; who signs in is in `esquadro/accounts.esq`. After each step below, stop `task run` (Ctrl+C), run:

```sh
esquadro gen
task migrate
task run
```

and look at what changed with `git diff` (the first generation was
committed).

## 2. Add a field

Products get a stock keeping unit, unique and optional:

```esquadro
entity Product {
  name        string  required min(2) max(120)
  sku         string  unique
  description text
  ...
}
```

`esquadro gen` updates `internal/catalog/product.go` (the field and its
rule), the queries, the API's contract and its code, and writes
`migrations/00002_add_products_sku.sql`:

```sql
-- +goose Up
ALTER TABLE public.products ADD COLUMN sku text;
ALTER TABLE public.products ADD CONSTRAINT products_sku_key UNIQUE (sku);
```

A second product with the same `sku` is refused with `422 invalid_fields` and
the code `taken` for `sku`.

## 3. Add an entity

Products now have a brand:

```esquadro
/// Who makes a product.
entity Brand {
  name string required unique
}

entity Product {
  ...
  brand > Brand
}
```

`Brand` gets everything `Category` has: its type and rules, its use cases
(create, read, list, update, delete), its queries, its operations at
`/api/v1/brands` and its tests. `Product` gets an optional `brand_id`, and the
migration creates the table and the foreign key. Deleting a brand that
products point at answers `409 in_use`; since the key is optional, you could
write `on_delete(set_null)` to clear it instead.

`esquadro gen` warns that `Brand` has no `@security`: until it says who may,
whoever signed in may change it. Add `@security(read public, write user.admin)`
to it, as `Category` has.

## 4. Tag products

A product has many tags, and a tag many products:

```esquadro
entity Tag {
  label string required unique
}

entity Product {
  ...
  tags <> Tag
}
```

The migration creates a join table, and the API gets operations to link and
unlink them:

```sh
curl -X PUT    localhost:8080/api/v1/products/$PRODUCT/tags/$TAG
curl           localhost:8080/api/v1/products/$PRODUCT/tags
curl           localhost:8080/api/v1/tags/$TAG/products
curl -X DELETE localhost:8080/api/v1/products/$PRODUCT/tags/$TAG
```

## 5. A second module

Orders are another part of the business, so they get their own module, in
their own file, `esquadro/orders.esq`:

```esquadro
package orders

/// A purchase.
entity Order {
  number    string required unique
  placed_at time   default(now)
}

/// One product in an order.
entity OrderItem {
  quantity int required min(1)
  order    > Order required on_delete(cascade)
  product  > Product required
}
```

`internal/orders` is a package of its own, with its use cases and its
operations at `/api/v1/orders` and `/api/v1/order-items`. `OrderItem` points
at `Product`, in another module: the key is stored, but modules do not share
tables, so there is no foreign key across them, and `catalog` does not know
about orders.

To keep the orders' tables in a Postgres schema of their own, give the
`postgres` blueprint an option in `esquadro/app.esq`:

```esquadro
blueprint postgres {
  schemas [orders]
}
```

## 6. Answer in Portuguese

Errors come as Problem Details with a stable `code` and a readable `title`.
The `i18n` blueprint picks the title's language from `Accept-Language`:

```esquadro
blueprint i18n {
  languages ["en", "pt-BR"]
}
```

```sh
curl -H 'Accept-Language: pt-BR' localhost:8080/api/v1/nowhere
# {"title":"Nada foi encontrado neste endereço.","code":"not_found",...}
```

esquadro knows `pt-BR` and `es`. For another language, write its titles in
`internal/platform/edge/translations.go`, a file that is yours: esquadro wrote
it once and never changes it.

## 7. Your own code

Generated files are esquadro's; your code goes in files of your own, next to
them. A use case that is not CRUD, for example, in
`internal/catalog/discount.go`:

```go
package catalog

// Discounted is the product's price with a discount, never below zero.
func Discounted(p Product, percent int) decimal.Decimal { ... }
```

`esquadro gen` never touches it. And if you edit a generated file, the next
`esquadro gen` stops and tells you which one, instead of overwriting it; see
[Regenerating safely](/esquadro/regenerating/).

## Where next

- [The language](/esquadro/language/files-and-modules/): everything a model
  can say.
- [Blueprints](/esquadro/blueprints/overview/): what each one generates, and
  its options.
- [Writing a blueprint](/esquadro/writing-blueprints/): generate your own
  kind of code.
