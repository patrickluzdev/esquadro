---
title: Related records, fields and PATCH
description: Read a record with what it points at, only the fields a screen shows, and change only what changed.
---

The generated API serves screens without extra calls or extra data, and
lets a screen change a record without undoing what another one changed.

## include

A record may come with the records it points at:

```sh
GET /api/v1/orders?include=customer,products
```

```json
{
  "id": "0198…",
  "number": "A-17",
  "customer_id": "0197…",
  "customer": { "id": "0197…", "name": "Carla", "version": 2, ... },
  "products": [ { "id": "0196…", "name": "Emma", ... } ],
  "version": 3
}
```

- Every relation a record declares may be included: the ones whose key it
  holds (`customer`) and its many-to-many ones (`products`), one level deep.
- A list includes for the whole page at once: one query per relation, not
  one per record.
- A many-to-many relation answers up to 100 records each; past them,
  `products_more` is `true`: read the rest with `/orders/{id}/products`.
- [Policies](/esquadro/security/#policies) apply: a related record the
  person may not read is left out, as if it were not there.
- A relation the record does not have answers `400`.

## fields

A `GET` answers only the fields asked for, and always `id` and `version`:

```sh
GET /api/v1/products?fields=name,price
GET /api/v1/orders/0198…?include=customer&fields=number,customer.name
```

A field of an included record is named after its relation
(`customer.name`); asking for it without including the relation answers
`400`, as does a field the record does not have.

The contract therefore says every field of a record's answer may be
absent, except `id` and `version`. The database still reads the whole row:
`fields` saves what travels, not what is read.

## PATCH

`PUT` replaces a record with the one sent. `PATCH` changes only the fields
sent:

```sh
PATCH /api/v1/products/0198…
{"price": "13.00"}
```

- A field left out stays as it is.
- `null` empties a field that may be empty; for a required one, it answers
  `422` (`required`). A field with a default does not take `null`: send
  the value.
- The whole record, with the changes, must pass the model's rules.
- Many-to-many relations change through their own routes, not `PATCH`.

With [If-Match](/esquadro/safe-writes/), the record must still be the
version read. Without it, a change someone made in between is kept: the
changes apply again on top of it, so two screens changing different fields
at once both win.

In your code, the module's use case takes the same changes (`c` is the
catalog module):

```go
price := decimal.RequireFromString("13.00")
updated, err := c.PatchProduct(ctx, id, 0, catalog.ProductChanges{Price: &price}, nil)
```

## Many-to-many, written with the record

A many-to-many relation can be written with the record itself: create,
update and patch take its ids, named after the target (`tag_ids` for
`tags <> Tag`, or the relation's name otherwise), and change the links in
the same transaction as the record. Left out, the links stay as they are;
`[]` unlinks them all; an id that is no record refuses the whole change
(`invalid_fields`, `not_found`), so nothing is written halfway.

```sh
POST  /api/v1/products         {"name": "Webcam", "tag_ids": ["0198…", "0198…"]}
PATCH /api/v1/products/0198…   {"tag_ids": []}
```

The links' own routes (`PUT`/`DELETE /products/{id}/tags/{tagId}`) still add
or remove one at a time. In a [frontend](/esquadro/frontends/)'s form, the
relation is a field of chips with a search to add or take some off.

