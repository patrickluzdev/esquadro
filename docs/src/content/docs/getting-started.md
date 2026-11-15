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
[releases](https://github.com/patrickluzdev/esquadro/releases): `.tar.gz`
for macOS and Linux, `.zip` for Windows, for Intel and ARM.

:::note[Downloaded binaries]
The binaries are not signed yet. On macOS, a binary downloaded with a browser
is blocked the first time; allow it with
`xattr -d com.apple.quarantine ./esquadro`. On Windows, SmartScreen may warn
before the first run: choose **More info → Run anyway**. Homebrew and
`go install` are not affected.
:::

Check it works:

```sh
esquadro version
```

The projects esquadro generates need [Go](https://go.dev/dl),
[Task](https://taskfile.dev) to run their tasks, and
[Docker](https://docs.docker.com/get-docker/) for the local database and the
tests.

## Create a project

```sh
esquadro new shop
cd shop
```

`esquadro new` writes the model in `esquadro/`, generates the project from it
and, with the `git` blueprint, makes it a repository with a first commit:

```
shop/
  esquadro/            the model: app.esq and catalog.esq, edit these
  cmd/shop/            the commands: serve (the default), migrate, check
  internal/catalog/    the module: its entities, rules and use cases
  internal/platform/   config, logging, the HTTP edge, Postgres
  migrations/          one migration per change in the model
  api/openapi.yaml     the API's contract
  compose.yaml         Postgres for development
```

Its module path comes from `git config github.user`
(`github.com/<you>/shop`), or `-module` sets it:
`esquadro new -module example.com/shop shop`.

## Run it

```sh
task up        # start Postgres (compose.yaml) and wait until it is ready
task migrate   # create the tables
task run       # serve on http://localhost:8080
```

Open http://localhost:8080/docs: the API's documentation, where every
operation can be tried. `task test` runs the generated tests, against a real
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
