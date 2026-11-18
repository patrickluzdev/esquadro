---
title: The trash
description: "Deleting a record puts it in the trash: restore it, delete it for good, and have it purged after some days."
---

By default, deleting a record removes it. For the entities where a wrong
click should not lose anything, mark the entity, and deleting puts the
record in the trash instead:

```esquadro
entity Product {
  name  string required
  ...
  @store(soft_delete)
}
```

Entities without the annotation keep deleting for good.

## What changes

- The table gains a `deleted_at` column; the migration adds it (and removing
  the annotation drops it).
- Reads, lists, counts, `include` and many-to-many lists leave out what is
  in the trash: a record there answers `404` like one that never existed.
- A unique field is unique among the records not in the trash: the name of a
  product in the trash can be given to a new one. Unique constraints become
  partial unique indexes of the same name.
- A record may not point at one in the trash: creating or changing it so
  answers `invalid_fields` with `not_found`, as for a record that does not
  exist.

## The records that point at it

Deleting follows the relation's `on_delete`, among the records not in the
trash:

| `on_delete` | Deleting the record |
|---|---|
| `restrict` (required relations, by default) | refused (`409 in_use`) while records not in the trash point at it |
| `cascade` | puts them in the trash with it, when their entity has a trash; refused when it has not, so nothing goes for good behind your back |
| `set_null` (optional relations, by default) | leaves them as they are: the relation points at a record in the trash |

Restoring a record restores those that went in with it. It is refused
(`invalid_fields`) when a record it points at is in the trash too
(`in_trash`), or when another took one of its unique values meanwhile
(`taken`).

## The API

| Operation | What it does |
|---|---|
| `DELETE /api/v1/products/{id}` | puts it in the trash (`204`), as before |
| `GET /api/v1/products/trash` | the trash, the last deleted first, by page, each with `deleted_at` |
| `POST /api/v1/products/{id}/restore` | takes it out, and answers it |
| `DELETE /api/v1/products/trash/{id}` | deletes it for good (only one in the trash) |
| `DELETE /api/v1/products/trash` | empties the trash: `{"purged": n}` |

Who may delete a record may restore it, see it in the trash and delete it
for good: the entity's `delete` rule, owners included.

The [TypeScript client](/esquadro/typescript-client/) has them under
`trash`:

```ts
await api.products.delete(id)
await api.products.trash.restore(id)
const page = await api.products.trash.list({ page: 1 })
await api.products.trash.purge(id)
await api.products.trash.empty()
```

In your code, the module has `DeleteProduct` (to the trash),
`RestoreProduct`, `ReadDeletedProduct`, `ListDeletedProducts`,
`PurgeProduct` and `PurgeDeletedProducts`.

## In the frontends

A [spa](/esquadro/frontends/) list of an entity with a trash gets a
**Trash** button, with how many records it holds. Deleting asks less (the
record can come back), and its notice has **Undo**. The trash
(`/products?trash=1`) lists what went there, with when, to **Restore** or
**Delete for good** one by one, or **Empty trash**; it says why a record
cannot come back (what it points at is in the trash too, or its unique value
was taken).

## Purging

What has been in the trash for 30 days is deleted for good (the `store`
blueprint's `purge_days` option; `0` keeps it there):

- with the [jobs](/esquadro/blueprints/reference/jobs/) blueprint, a job does
  it every day at 3:00;
- `<app> purge` does it by hand: `--days N` to choose how old (`0` empties
  the trash), `--dry-run` to only count.

A record other records still point at stays until they go. Files of a
record are released when it is deleted for good, not when it goes to the
trash.

Reports are your SQL: leave out what is in the trash with
`deleted_at IS NULL`.
