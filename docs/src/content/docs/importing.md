---
title: Importing
description: "Records from a CSV file, in the admin panel or by the API: all or none, each through the rules of a create, and imported again by a key without duplicates."
---

Some records come from elsewhere: an affiliate program's report of
commissions, a list of coupons, a spreadsheet of products. Mark the entity
and its records may be imported in a batch:

```esquadro
/// A commission an affiliate program reported.
entity Commission {
  external_id string required unique   // the order's id in the store
  ordered_on  date   required
  status      CommissionStatus required
  amount      money  required
  niche       > Niche
  @store(import_by external_id)        // importing again replaces by it
  @admin(resource)
}

entity Coupon {
  code    string required
  percent int    required min(1) max(90)
  @store(import)                       // only creates
}
```

- `@store(import)` lets records be imported: each row is created.
- `@store(import_by field)` also lets them be imported, and a row whose
  `field` is that of a record already there replaces it instead: the same
  report imported again updates the commissions' status and duplicates
  none. The field must be `required` and `unique`, and a string, text,
  int, uuid or date.

## All or none

An import runs in one transaction. Each row goes through what a create
(or, replacing, a change) goes through: the model's rules, `@security`,
the business rules of `<entity>_rules.go`, audit and notifications. A
refused row does not stop the others from being checked, so every problem
comes back at once, and nothing is stored. An import takes 5000 rows at
most.

## In the admin panel

A resource of an entity that takes imports has an **Import** button next to
Export, for whoever may create its records:

1. **The file:** a CSV, with the names of its columns in its first line; the
   separator (`;` or `,`) is found, and a byte-order mark is fine.
2. **The columns:** each is given the field of its name or label, in any
   case and without accents; the others are left to choose, or ignore.
   What was chosen is remembered for the resource, so next week's report
   comes matched.
3. **The preview:** the first rows, and what is wrong with them: a required
   field empty, a number or date that cannot be read, a value no option
   has. Nothing is sent while there is a problem; **Download the rows with
   problems** gives them back as a CSV, with a column saying what is
   wrong, to fix and import again.

Values are read as the panel's language writes them, as Export does:
`1.234,56` and `18/11/2026` (or `18 de nov. de 2026`) in pt-BR,
`1,234.56` and `11/18/2026` in English, and ISO dates always. A choice may
be its value or its label; a yes or no, `sim`/`não`, `yes`/`no`, `1`/`0`
or `x`. Money takes an amount column and, optionally, its currency's
(`Price (Currency)`, as Export writes it). A relation may be a record's
id, or the text that shows it (`Livros` for a category shown by its name),
when the related resource is in the panel: a text no record shows, or more
than one does, is a problem of the preview.

So a list exported, changed in a spreadsheet and imported again comes back
as it was, but for the changes.

## By the API

```http
POST /api/v1/commissions/import
Idempotency-Key: 6f1c…

{ "rows": [ { "external_id": "SHP-1", "ordered_on": "2026-11-18", … }, … ] }
```

Each row is what `POST /api/v1/commissions` takes. The answer is
`{ "created": 120, "updated": 34 }`, or `422 import_refused`, whose
`errors` give each refused field with its `row` (from 1):

```json
{ "code": "import_refused", "errors": [ { "row": 3, "field": "amount", "code": "too_small" } ] }
```

More than 5000 rows answer `422 too_many_rows`. The request's body counts
against `MAX_BODY_KB` (1024 by default): raise it if your files are large.
The TypeScript client has `api.commissions.import(rows)`, and the module
`ImportCommissions(ctx, rows, guard)`.

## Generated tests

The module's tests import rows, import them again (with `import_by`,
which replaces them), and check that a refused row stores none, also in a
transaction that commits anyway; the API's, the same through the
contract, and the limit of rows. The panel's reading of files (separators,
quotes, numbers and dates in the language, a list exported and imported
again) is tested in `src/esquadro/kit/csv.test.ts`.
