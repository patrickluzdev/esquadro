---
title: Environment variables
description: The project's environments and variables, declared in the model, with files that keep each environment's values.
sidebar:
  order: 6
---

A project's configuration comes from environment variables, and every one of
them is **declared in the model**: blueprints declare those they read, and
`app.esq` declares the app's own. Everything else comes from the
declarations: the code that reads and checks them, `.env.example`, the files
of each environment, and `esquadro env`. With the
[env](/esquadro/blueprints/reference/env/) blueprint, which `esquadro new`
adds, a project has three environments: `development`, `staging` and
`production`.

## Environments

`APP_ENV` picks the environment; `development` is the default. In
`development` every variable has a value, so a new project runs as it is:
`task up`, `task run`, and nothing to configure. In the other environments,
the variables that have no value there must be set, or the app refuses to
start and says which:

```
config (production): DATABASE_URL is not set; STRIPE_KEY is not set
```

Every problem is in the message at once, and a secret's value never is.

## Declaring variables

```esquadro
app shop {
  module "github.com/acme/shop"

  env {
    /// The Stripe key.
    STRIPE_KEY string secret required
    /// The key that signs sessions.
    SESSION_KEY string secret generate(32)
    /// How long a request may take.
    REQUEST_TIMEOUT duration default("30s")
  }
}
```

A declaration is `NAME type modifiers`. The types are `string`, `int`,
`number`, `bool`, `list` (comma-separated) and `duration` (`30s`, `5m`).

| Modifier | |
|---|---|
| `default(v)` | the value in every environment |
| `development(v)`, `staging(v)`, `production(v)` | the value in one environment |
| `required` | set in every environment but development; `required(staging)` names them |
| `secret` | never shown: not in logs, errors or `esquadro env`; no value in the code, but in development |
| `generate(32)` | a secret esquadro generates for development, in `.env.development` |
| `prefixed` | the name starts with the app's: `POOL_MAX` is `SHOP_POOL_MAX` |
| `field(Name)` | the field of `config.Config` that holds it, when its name says otherwise |
| `when("{{...}}")` | in a blueprint, the variable exists only when the template renders to something other than empty or `false`: `when("{{if .Options.pepper}}true{{end}}")` |

Each blueprint's reference page lists the variables it declares. `APP_NAME`
names the app in its logs and telemetry (`shop` by default).

## Values by environment

Values that are not secrets can be set per environment in `app.esq`,
versioned with the code:

```esquadro
app shop {
  env production {
    SHOP_CORS_ORIGINS ["https://shop.com"]
    SHOP_RATE_LIMIT   300
  }
  env staging {
    SHOP_CORS_ORIGINS ["https://staging.shop.com"]
  }
}
```

A secret's value never goes in a `.esq` file: esquadro check refuses it.

## Where a value comes from

In order, the first that has one:

1. the process's environment (`export SHOP_RATE_LIMIT=500`, the platform's
   variables and secrets);
2. the environment's file, `.env.<environment>`, in the working directory;
3. the value `app.esq` gives that environment;
4. the declaration's value for that environment, then its default.

`go run ./cmd/shop env` shows each value and where it came from:

```
environment: production

APP_NAME         shop                 (default)
SHOP_RATE_LIMIT  300                  (production value)
DATABASE_URL     xxxxx                (.env.production)
STRIPE_KEY       xxxxx                (environment)
```

## The files

| File | | In git |
|---|---|---|
| `.env.example` | every variable, documented, with its values by environment: generated | yes |
| `.env.development`, `.env.staging`, `.env.production` | each environment's values, yours | no |

`esquadro gen` creates each environment's file once, and later adds the
variables the model declares since, commented, with their documentation. It
never changes a value of yours; a variable no declaration has any more gets
a note over it instead of being deleted. The files are readable by you only,
and `.gitignore` keeps them out of git.

## esquadro env

```sh
esquadro env list              # every variable: type, where required, values, who declares it
esquadro env check production  # .env.production against the model
```

`esquadro env check` says what an environment is missing, the values of the
wrong type and the variables nothing reads.

## Other environments

The env blueprint's `environments` option changes them; the first one is
where development happens:

```esquadro
blueprint env {
  environments ["development", "qa", "staging", "production"]
}
```
