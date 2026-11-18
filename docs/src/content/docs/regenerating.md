---
title: Regenerating safely
description: How esquadro keeps the generated code in step with the model without losing your work.
---

`esquadro gen` can run as often as you like. It compares what it would write
with what it wrote last time, recorded in `esquadro/lock.json`, and with what
is on disk:

| A generated file... | `esquadro gen`... |
|---|---|
| you did not touch | updates it, or deletes it when the model no longer needs it |
| you edited | stops before writing anything, and lists it |
| marked `.once` (README, Taskfile.yml, translations.go...), as it wrote it | updates it: you never changed it, so it follows its template |
| marked `.once`, and you changed it | leaves it: it is yours |
| a file of yours, not generated | never looks at it |

```sh
esquadro gen -dry-run      # what would change, with diffs, writing nothing
esquadro gen -check        # exit 1 if anything would change: for CI
esquadro gen -force        # overwrite the generated files you edited
esquadro explain <file>    # which blueprint and template wrote a file
```

Keep your code in files of your own next to the generated ones: a use case in
`internal/catalog/discount.go`, a route in a file of `cmd/<app>/`. When a
blueprint has a slot for it, contribute there from a
[local blueprint](/esquadro/writing-blueprints/).

## Migrations

Each `esquadro gen` that changes the database writes a new migration in
`migrations/`, from the difference between this model and the last one
(`esquadro/model.json`). A migration is yours once written: review it, edit
it, and esquadro never changes it again. Changes it cannot write safely (a
column whose type changes, an enum value removed) are written as a `TODO` in
the migration and reported.

## Commit the model

`esquadro/` holds the model and esquadro's state (`lock.json`, `model.json`):
commit it with the code, so the next `esquadro gen`, on any machine, knows
what came before.
