---
title: Production
description: From a fresh Debian server to your apps deployed by their tags, with a platform made by esquadro.
---

An app made by esquadro publishes its image from its CI. A **platform** is
where those images run: a project of its own, made with
`esquadro new -starter platform`, that describes your servers and the
services your apps share, and generates the code that keeps them so:
Ansible for the servers, Docker Compose for the services and apps, Caddy for
HTTPS, OpenTofu for Cloudflare. It is ordinary code, in its own repository:
read it, run it, change what esquadro leaves to you.

One platform runs many apps: each is a few lines of its model, and each
deploys itself when you tag it.

```esquadro
platform acme {
  blueprint server
  blueprint caddy
  blueprint postgres
  blueprint apps
  blueprint observability {
    grafana_domain "grafana.acme.com"
  }
  blueprint backup
  blueprint cloudflare {
    account_id "your account's id"
  }
  blueprint github_actions
  blueprint git
}

/// The VPS.
server main {
  host "203.0.113.10"
}

/// The shop.
app shop {
  image   "ghcr.io/acme/shop"
  domains ["shop.com", "api.shop.com"]
  worker
  database
}
```

## What runs on the server

- **Caddy**, the only service on the internet (ports 80 and 443): each app's
  domains reach its container, over HTTPS. While an app is replaced, Caddy
  holds its requests for up to 30 seconds instead of failing them.
- **PostgreSQL**, with a database and a user for each app with `database`,
  which no other app can open. It publishes no port.
- **Grafana**, with each app's traces, metrics and logs and the server's
  CPU, memory, disk and network, behind its sign-in.
- **Backups**: every day, each database goes to an R2 bucket, kept for 14
  days.
- **The apps**, each with its worker, its production variables and its
  database's address, at the version their last deploy chose.

The server itself is Debian 13: an admin user who signs in with your key,
no root login and no passwords, a firewall that lets in SSH, HTTP and HTTPS
only, and security updates every day.

## The first time

You need a Debian 13 server you sign in to as root with your SSH key (your
provider's panel takes the key when it installs the system), your domains
on Cloudflare, and on your machine [Task](https://taskfile.dev), Ansible,
SOPS, age and OpenTofu.

```sh
esquadro new -starter platform acme
cd acme
# write the server's address and your apps in esquadro/platform.esq
esquadro gen
task deps            # the Ansible collections
task secrets:init    # your age key, and the secrets encrypted with it
task secrets:edit    # the Cloudflare token and the bucket's keys
task bootstrap       # root once, then everything else
```

`task secrets:init` makes your age key at `~/.config/sops/age/keys.txt`:
keep a copy in your password manager. The secrets are in `secrets/`,
encrypted: only that key reads them, so they go to git with the rest.

## An app's deploys

In the app, add the `deploy` blueprint: its CI, after publishing a tag's
image, signs in to the server with a key that can only deploy that app.

```sh
# in the platform: the key, straight to the app repository's secrets
task app:key APP=shop REPO=acme/shop
sops secrets/apps/shop.sops.env   # its production variables
task apply

# in the app
git tag v1.0.0 && git push origin v1.0.0
```

A deploy pulls the version, runs its migrations, replaces the app and its
worker, and waits for them to be healthy. When they are not, the version
before comes back and the CI fails with the reason. From the platform,
`task deploy APP=shop VERSION=v1.0.0` deploys or rolls back, `task status`
shows what runs, `task logs APP=shop` follows the logs.

Rolling back does not undo migrations: write them so that the version
before still works with the new schema (add a column first, stop using the
old one later).

## Every day

| Task | What it does |
|---|---|
| `task plan` | shows what `apply` would change on the server and on Cloudflare |
| `task apply` | changes it |
| `task status` | each app's version and health |
| `task db:backup`, `task db:restore` | a backup now, or one put back |
| `task grafana` | Grafana through SSH, without a domain |
| `task ssh` | signs in to the server |

The platform's CI checks the code (`esquadro gen -check`, `ansible-lint`,
`tofu validate`) and never touches the servers: `task apply` runs from
your machine, with your key.

## Your own changes

What esquadro generates, it regenerates. These are yours, written once:
`caddy/sites/*.caddy` (sites and redirects), `ansible/roles/local/`
(anything else on the server), `postgres/local.conf`, `tofu/local.tf` and
`Taskfile.yml`.
