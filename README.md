# Esquadro

Esquadro generates Go projects from a model. You describe your modules and
their entities in `.esq` files, pick **blueprints** for what the project
needs (a database, migrations, a JSON API...), and `esquadro gen` writes a
complete, idiomatic Go project. Change the model and run it again: esquadro
updates the code, writes the database migration and never overwrites what
you wrote by hand.

```
package catalog

enum Status {
  ACTIVE
  INACTIVE
}

/// Something the store sells.
entity Product {
  name     string  required min(2) max(120)
  price    decimal required min(0)
  status   Status  default(ACTIVE)
  category > Category required
  tags     <> Tag
}
```

From a model like this, the `api` starter's blueprints write a JSON API on
PostgreSQL: the OpenAPI contract and its server, each entity's use cases,
the migrations, Problem Details errors in the client's language, security
headers, a rate limit, tests against a real Postgres, a `compose.yaml` to run
it all locally, a container image and a CI. Blueprints are small and
optional: the `worker` and `cli` starters, or a project built blueprint by
blueprint, take only what they need.

The model also declares work for the background, run on a queue in
PostgreSQL by the `worker`:

```
/// Sends the welcome email to a new user.
job SendWelcomeEmail {
  user_id uuid required
}
```

## Install

```
brew install patrickluzdev/tap/esquadro                      # macOS and Linux
go install github.com/patrickluzdev/esquadro/cmd/esquadro@latest
```

Or download a binary for macOS or Linux from the
[releases](https://github.com/patrickluzdev/esquadro/releases). Windows is not
supported yet.

## Start a project

```
esquadro doctor                    # Go, Task, Docker: what is missing, and how to get it
esquadro new -starter api shop     # or -starter worker, -starter cli
cd shop
task up        # Postgres in Docker
task migrate
task run       # http://localhost:8080/docs
```

Then change the `.esq` files in `esquadro/` and run `esquadro gen`. Without
a starter, `esquadro new shop` writes the smallest project, and blueprints
come with `esquadro add`:

```
esquadro blueprint list            # every blueprint, and those the project uses
esquadro add worker                # add one, with what it requires
esquadro remove worker
esquadro blueprint new billing     # a blueprint of your own, with a test
```
 The
[documentation](https://patrickluzdev.github.io/esquadro/) has a tutorial,
the language, every blueprint and how to write your own.

## Editors

The [VS Code extension](https://marketplace.visualstudio.com/items?itemName=patrickluzdev.esquadro)
colors `.esq` files, shows errors as you type and formats on save. Other
editors can use the same language server: `esquadro lsp`.

## License

Esquadro is licensed under the [Apache License 2.0](LICENSE). The code it
generates is yours: use it under any license you choose.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
