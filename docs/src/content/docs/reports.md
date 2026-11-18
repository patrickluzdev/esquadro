---
title: Reports
description: Numbers worked out by SQL of your own, served by the API and typed in the client like everything else.
---

A report answers a question about many records at once: clicks per niche,
sales per day, the average price per category. You declare its shape in the
model and write its SQL; esquadro makes the rest: the use case, the route,
the contract, who may read it, the TypeScript client's function and a test.
Anything PostgreSQL does (joins, subqueries, window functions, `CASE`)
fits, so a hard report costs what an easy one does.

## Declaring it

```esquadro
package analytics

/// Products per category, and their average price.
report ProductsByCategory {
  category      string  required
  products      int     required
  average_price decimal
  since         time    param
  status        string  param
  @security(read [user.admin, user.staff])
}
```

- Each field is a **column** of the rows, with a model type; one not marked
  `required` may be `null` (an average of nothing).
- A column can be one of the model's enums (`status Status`): its values are
  then the enum's, and a frontend shows their labels.
- A field marked **`param`** is a **parameter**, passed in the query;
  `required` makes it compulsory.
- [`@security(read ...)`](/esquadro/security/) says who may read it:
  `public`, a realm, or `realm.role`. With security, a report must say.
- `@rest(unpaged_max 5000)` lets it answer more rows than the API's
  default, 1000.

## Writing its SQL

The first time it generates, esquadro writes
`internal/<module>/reports/<name>.sql`, which is then yours: it never
touches it again.

```sql
-- name: ProductsByCategory :many
SELECT c.name::text AS category,
       count(p.id)::bigint AS products,
       round(avg(p.price), 2)::numeric AS average_price
FROM categories c
LEFT JOIN products p ON p.category_id = c.id
  AND (sqlc.narg(since)::timestamptz IS NULL OR p.created_at >= sqlc.narg(since)::timestamptz)
  AND (sqlc.narg(status)::text IS NULL OR p.status::text = sqlc.narg(status)::text)
GROUP BY c.name
ORDER BY products DESC
LIMIT sqlc.arg(row_limit);
```

- Keep the `-- name:` line, and a column for each declared one, **cast to
  its type** (`::bigint` for `int`, `::numeric` for `decimal`,
  `::timestamptz` for `time`, `::text` for text).
- A `money` column is two: its amount (`::numeric`) and its currency
  (`::text`, named `<column>_currency`). Group by the currency, so that
  amounts of two currencies are never added up:
  `sum(price)::numeric AS total, price_currency::text AS total_currency ...
  GROUP BY price_currency`. A parameter cannot be money.
- Parameters are `sqlc.arg(name)::type` when required, `sqlc.narg(name)::type`
  when not: they are values, never pasted into the SQL.
- **Keep `LIMIT sqlc.arg(row_limit)`**: it caps the rows. Past the cap the
  report answers `422 too_many_records` instead of some rows as if they were
  all.

SQL that does not run (a typo, a column a migration removed) makes
`esquadro gen` fail, at its file and line. A column whose type is not the
declared one is reported when the report runs, naming the column.

## Using it

```sh
GET /api/v1/reports/products-by-category?since=2026-11-01T00:00:00Z
→ { "items": [ { "category": "Casa", "products": 2, "average_price": "15" } ] }
```

```ts
const rows = await api.reports.productsByCategory({ since: "2026-11-01T00:00:00Z" })
```

```go
rows, err := a.ProductsByCategory(ctx, analytics.ProductsByCategoryQuery{Since: &since})
```

The use case is the module's like any other: a job can send the numbers
every morning. From code, `Limit` asks for another cap.
