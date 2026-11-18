---
title: Audit
description: "Who changed what, and when: each change of an audited record, kept with the change, and each record's history."
---

With the `audit` blueprint, each change of an audited record is kept: when,
which record, what was done (`create`, `update`, `delete`, `restore`,
`purge`), who did it, and each field that changed, before and after.

```esquadro
app shop {
  blueprint audit
}

entity Product {
  name  string required
  price money  required
  cost  money  @audit(secret)   // kept as "changed", without its values
  notes text   @audit(ignore)   // not kept
  @audit
}
```

## How it is kept

The store's use cases write the change in the same transaction as the
change itself: if the change does not stay, neither does its record, and the
other way around. Everything the generated code writes goes through them:
the API, the frontends, the seed, your jobs. An update that changes no field
is not kept. A change made by hand in SQL is not kept either.

Who made it is a person (with [security](/esquadro/security/), their name as
it was then), a service key (its name), or the system: the command or job
that made it. In your code, `audit.AsSystem(ctx, "import")` names it.

## Reading it

| Operation | What |
|---|---|
| `GET /api/v1/products/{id}/history` | the record's changes, the newest first, by page |
| `GET /audit?entity=&record_id=&actor_id=&action=&since=&until=` | every change, for those who administer |

```ts
const page = await api.products.history(id)
const all = await api.audit.list({ entity: "products", since: "2026-11-01" })
```

In your code: `audit.List(ctx, pool, audit.Filter{...}, page, perPage)`.

## In the frontends

A [spa](/esquadro/frontends/) record page of an audited entity has a
**History** tab, for those the `history` option lets see it: a timeline of
who did what and when, each field before and after, shown as the page shows
it (an amount formatted, an enum with its color, a related record by its
name). Those who administer have **Administration › Audit**: every change,
filtered by entity, action, period and person (in the URL), each opened to
its fields. The spa's `history false` and `audit_page false` leave them out.

## Options

| Option | Default | |
|---|---|---|
| `entities` | `marked` | `marked`: those with `@audit`; `all`: every entity but those with `@audit(off)` |
| `retention_days` | `365` | changes older than this are deleted, by a daily job with [jobs](/esquadro/blueprints/reference/jobs/), and by `<app> audit purge [--days N]`; `0` keeps them |
| `admins` | `[]` | who sees every change (`[user.admin]`); empty, those who administer the first realm's accounts |
| `history` | `update` | who sees a record's history: `update` (who may change it), `read` (who may read it) or `admins` |
