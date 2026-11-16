---
title: Deploying
description: The container image and the CI esquadro generates, for any kind of app.
---

Two blueprints take a project from your machine to production. They work for
any kind of app (an API, a worker, a command). The `api` and `worker`
starters have both, `cli` has the CI; `esquadro add docker` adds the image to
any project.

## The image: `docker`

The [docker](/esquadro/blueprints/reference/docker/) blueprint writes a
multi-stage `Dockerfile`: Go builds the binary, which is copied into a
minimal image (distroless: no shell, no package manager, not root). An API's
image is around 30 MB.

```sh
task docker:build                 # shop:dev
docker run shop:dev               # the app's default command (serve, for an API)
docker run shop:dev migrate       # any other command
docker run shop:dev version       # dev, or the version it was built with
```

The image's version comes from `docker build --build-arg VERSION=v1.2.0`; the
CI below passes the tag's.

The image runs in production mode (`SHOP_ENV=production`: JSON logs, no
`/docs`), and is configured by environment variables, as the app is
(`DATABASE_URL`, `PORT`...). With `http`, it exposes the port and checks its
own health (`shop health`, which asks `/healthz`).

With `compose`, the app is also a service of `compose.yaml`, in the `app`
profile, so that `task up` keeps starting only what the app needs:

```sh
docker compose --profile app up -d --build    # everything in containers
docker compose --profile app run --rm app migrate
```

Without `http` (a worker, a command), the image has no port and no health
check, and runs the command it is given: `docker run worker:dev check`.

## The CI: `github_actions`

The [github_actions](/esquadro/blueprints/reference/github_actions/)
blueprint writes `.github/workflows/ci.yml`. On every push and pull request
it runs `go vet`, the tests (GitHub's runners have Docker, so the tests
against Postgres run too), the build, and `esquadro gen -check`, with the
esquadro version recorded in `esquadro/lock.json`: it fails when someone
changed the model without regenerating, or edited generated files.

With `docker`, it also builds the image and, for tags `v*`, publishes it to
GitHub's container registry:

```sh
git tag v1.0.0 && git push origin v1.0.0
# ghcr.io/<owner>/<repo>:1.0.0, :1.0 and :latest
```

## Where to run it

Set `OTEL_EXPORTER_OTLP_ENDPOINT` to send its traces, metrics and logs
somewhere (see [Observability](/esquadro/observability/)).

The image runs anywhere that runs containers. Blueprints for specific
platforms (Fly.io, Kubernetes...) are planned; until then, point your
platform at the image and give it the environment variables of
`internal/platform/config/config.go`.
