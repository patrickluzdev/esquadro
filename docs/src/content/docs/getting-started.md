---
title: Getting started
description: Install esquadro, create a project and run it.
---

Esquadro generates Go projects from a **model**: `.esq` files that describe
your modules, their entities, fields and relations. **Blueprints** decide what
the project is made of (a database, migrations, a JSON API...). Run
`esquadro gen` whenever the model changes; esquadro updates the code and
writes the database migration.

## Install

```sh
# macOS and Linux
brew install patrickluzdev/tap/esquadro

# Anywhere with Go
go install github.com/patrickluzdev/esquadro/cmd/esquadro@latest
```

Or download a binary from the
[releases](https://github.com/patrickluzdev/esquadro/releases), for macOS and
Linux, Intel and ARM.

:::note[Windows]
Windows is not supported yet.
:::

:::note[Downloaded binaries]
The binaries are not signed yet. On macOS, a binary downloaded with a browser
is blocked the first time; allow it with
`xattr -d com.apple.quarantine ./esquadro`. Homebrew and `go install` are not
affected.
:::

Check it works, and that you have what the projects need:

```sh
esquadro version
esquadro doctor
```

The projects esquadro generates need [Go](https://go.dev/dl),
[Task](https://taskfile.dev) to run their tasks, and, with a database,
[Docker](https://docs.docker.com/get-docker/) for it and the tests.
`esquadro doctor` checks each of them and says how to install what is
missing; in a project, it checks what that project needs.

## Create a project

A project starts from a **starter**, a set of blueprints with an example
model:

```sh
esquadro new -starter api shop
cd shop
```

| Starter | What it is |
|---|---|
| `api` | A JSON API on PostgreSQL, with its contract, docs, image and CI, and a catalog to change |
| `worker` | A background service on PostgreSQL: [jobs](/esquadro/language/jobs/) on a queue, some on a schedule, with its image and CI |
| `cli` | A command line tool: commands, configuration and CI, nothing else to start with |

Without `-starter`, `esquadro new shop` writes the smallest project, a
program with the `check` and `version` commands, and you
[add blueprints](#add-what-you-need) as you need them.

`esquadro new` writes the model in `esquadro/`, generates the project from it
and, with the `git` blueprint, makes it a repository with a first commit. The
`api` starter writes:

```
shop/
  esquadro/            the model: app.esq and catalog.esq, edit these
  cmd/shop/            the commands: serve (the default), migrate, check, version
  internal/catalog/    the module: its entities, rules and use cases
  internal/platform/   config, logging, the HTTP edge, Postgres
  migrations/          one migration per change in the model
  api/openapi.yaml     the API's contract
  compose.yaml         Postgres and Grafana for development (and the app, in the app profile)
  Dockerfile           the app's container image
  .github/workflows/   the CI: tests, the generated code in step, the image
```

Its module path comes from `git config github.user`
(`github.com/<you>/shop`), or `-module` sets it:
`esquadro new -module example.com/shop shop`.

## Run it

```sh
task up        # start Postgres and Grafana (compose.yaml) and wait until they are ready
task migrate   # create the tables
task run       # serve on http://localhost:8080
```

Open http://localhost:8080/docs: the API's documentation, where every
operation can be tried. Each request's trace, with its queries, is in
Grafana at http://localhost:3000 (see [Observability](/esquadro/observability/)). `task test` runs the generated tests, against a real
Postgres they start themselves.

## Change the model

Open `esquadro/catalog.esq` and add a field to `Product`:

```esquadro
entity Product {
  name  string  required min(2) max(120)
  sku   string  unique
  price decimal required min(0)
  ...
}
```

Then:

```sh
esquadro gen     # updates the code, writes a migration adding the column
task migrate
```

That is the whole loop. The [tutorial](/esquadro/tutorial/) goes through it
step by step, and [the language](/esquadro/language/files-and-modules/)
describes everything a model can say.

## Add what you need

Blueprints come and go with a command, whatever the project started from:

```sh
esquadro blueprint list          # every blueprint, and those the project uses
esquadro blueprint info worker   # what one does: options, slots, files
esquadro add worker              # add it (and what it requires), and generate
esquadro remove worker           # take it out: its generated files go too
```

`esquadro add` writes the blueprint in `esquadro/app.esq` with its options
commented out at their defaults, ready to change. See
[Blueprints](/esquadro/blueprints/overview/).
