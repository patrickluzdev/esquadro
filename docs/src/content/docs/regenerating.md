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
| marked `.once`, and you changed it | merges its template's changes into yours, as `git merge` does |
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

## Your .once files get esquadro's changes too

A `.once` file is yours, and esquadro still brings its template's changes
to it: it keeps what the template wrote last time in `esquadro/base/` (as `<file>.base`)
(commit it), and merges what changed since into your file, line by line.

- Changes in different places: both are kept, and `esquadro gen` lists the
  file as `merge`.
- Changes to the same lines: a conflict, marked as git marks them (with
  the lines as they were, as git's `diff3` style does); your editor offers
  to keep one side, the other or both. `esquadro gen` stops,
  and `esquadro gen -check` fails, until the markers are gone.

```text
<<<<<<< yours
  { to: "/promotions", label: "Promotions" },
||||||| base
  { to: "/products", label: "Products" },
=======
  { to: "/products", label: t("products") },
>>>>>>> esquadro
```

`esquadro gen -dry-run` shows each merge before it is written.

### In VS Code

The [Esquadro extension](https://marketplace.visualstudio.com/items?itemName=patrickluzdev.esquadro)
adds a panel to Source Control with what `esquadro gen` would do, file by
file, and opens a conflict in the merge editor in three columns: the base,
yours and esquadro's. It reads `esquadro gen -dry-run -json`, which other
editors and tools can read as well: each file's path, its `change` (create,
update, merge, delete, keep or conflict), `markers` when it has conflicts
to resolve, and, with `-out dir`, its new content under `dir`.

## Upgrading esquadro

`esquadro/lock.json` records the esquadro that generated the project last.
A newer release does not generate it in silence: `esquadro gen` stops and
asks for `esquadro upgrade`, which shows what each version since changes
for the project (what breaks, what is new, how to migrate), then generates
it as `esquadro gen` does, your `.once` files getting the changes merged in.
An older release than the project's refuses to generate it.

```sh
esquadro upgrade -dry-run   # the notes, and what would change, writing nothing
esquadro upgrade            # the notes, then generate
```

Upgrade in a branch of its own, run the project's tests, and merge it then.
A build of esquadro's own code (not a release) only notes the difference.

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
