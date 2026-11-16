---
title: Blueprints
description: What blueprints are, the built-in ones, and how they combine.
---

A **blueprint** is one capability of a project: a folder of templates, a
manifest (`blueprint.esq`) that declares its options and how it relates to
others, and, sometimes, hooks that run a tool (sqlc, ogen, goose). The app
lists the blueprints it is made of:

```esquadro
app shop {
  module "github.com/acme/shop"

  blueprint app
  blueprint domain
  blueprint http
  blueprint rate_limit {
    per_minute 60
  }
}
```

They are small on purpose: each does one thing, so a project takes what it
needs and drops a line to lose a capability. The built-in ones:

| Group | Blueprints |
|---|---|
| The program and how it runs | [app](reference/app/), [domain](reference/domain/), [http](reference/http/), [worker](reference/worker/), [jobs](reference/jobs/) |
| PostgreSQL | [postgres](reference/postgres/), [migrations](reference/migrations/), [store](reference/store/), [testcontainers](reference/testcontainers/), [compose](reference/compose/) |
| The JSON API | [rest](reference/rest/), [api_docs](reference/api_docs/), [cors](reference/cors/), [security_headers](reference/security_headers/), [rate_limit](reference/rate_limit/), [compression](reference/compression/), [i18n](reference/i18n/) |
| Shipping it | [docker](reference/docker/), [github_actions](reference/github_actions/) (see [Deploying](/esquadro/deploying/)) |
| The repository | [git](reference/git/) |

`esquadro new -starter api` starts from all of them but `worker` and `jobs`; the
`worker` and `cli` starters take fewer, and `esquadro new` without a starter
takes only `app` (see [Getting started](/esquadro/getting-started/#create-a-project)).

## Adding and removing

```sh
esquadro blueprint list            # every blueprint; * marks those the app uses
esquadro blueprint info rate_limit # what it does, its options, slots and files
esquadro add rate_limit            # add it, with what it requires, and generate
esquadro remove rate_limit         # take it out, and generate
```

`esquadro add` writes each blueprint in the app block with its options
commented out at their defaults, and adds the blueprints it requires first,
saying which:

```esquadro
blueprint http {
  // port 8080
  // trust_proxy false
}
blueprint rate_limit {
  // per_minute 120
}
```

It works with built-in names, folders (`./blueprints/billing`) and git
sources. `esquadro remove` refuses to take out a blueprint another one
requires, and says which blueprints no one requires any more. Generating then
deletes the files the removed blueprint wrote, unless you edited them (as
`esquadro gen` always does); the files it wrote once for you stay. Both
change `app.esq` in place, keeping your comments, and `-no-gen` stops before
generating.

## How they combine

- **`requires`**: a blueprint that needs another. `store` requires `postgres`;
  leaving `postgres` out is an error that says so.
- **`optional`**: a blueprint that works with another when the app has it.
  `postgres` adds its service to `compose.yaml` only with `compose`;
  `rate_limit` answers in the client's language only with `i18n`.
- **Slots**: the points where a blueprint lets others add code, such as
  `http.middleware` or `app.commands`. That is how `cors` puts itself in the
  HTTP server's chain without the server knowing it.

## Configuring them

Options go in the app block and have a default that suits development. The
ones read when the service starts (a port, a rate limit) can be changed in
production by an environment variable, named in each blueprint's reference:
`SHOP_RATE_LIMIT=300`.

## Where they come from

```esquadro
blueprint rest                                            // built in
blueprint "./blueprints/billing"                          // a folder of yours, in esquadro/
blueprint "github.com/acme/esquadro-stripe@v1.2.0"        // a git repository
blueprint "https://git.acme.com/bp.git//stripe@main"      // a folder of any git repository
```

A blueprint from git stays at the commit `esquadro/lock.json` records until
`esquadro blueprint update` (all of them, or those named) moves it to the
commit its ref points to now. If it has hooks, esquadro asks before running
them, once per commit. See [Writing a blueprint](/esquadro/writing-blueprints/).
