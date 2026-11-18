---
title: TypeScript client
description: A typed client of the API for the fronts that call it, published as an npm package with each version.
---

The [client_ts](/esquadro/blueprints/reference/client_ts/) blueprint
generates, in `clients/ts`, a TypeScript client of the project's API: for
the fronts that call it (Astro, Next, any JavaScript), usually in other
repositories. It needs nothing but `fetch`, so it runs in browsers, Node,
Bun and edge runtimes.

```esquadro
blueprint client_ts            // @<owner>/<app>-client, owner from the module
blueprint client_ts {
  scope "acme"                 // when the module is not on GitHub
  registry npmjs               // instead of GitHub Packages
}
```

Each project has its own package, as it has its own image: the client of
that API, with its records and rules. With
[github_actions](/esquadro/blueprints/reference/github_actions/), each tag
`v*` publishes it as the tag's version (`v1.4.0`, `1.4.0`): to GitHub
Packages with the workflow's own token, or to npmjs by trusted publishing,
without a token kept anywhere.

## Signing in

```ts
import { createClient } from "@acme/shop-client"

// In a browser, with a realm's session cookie:
const api = createClient({ baseUrl: "https://api.shop.com", auth: { cookie: "customer" } })
await api.identity.customer.signIn({ email, password })

// On a server (Next, Astro), a client per request, with that person's token:
const api = createClient({ baseUrl: process.env.API_URL!, token })
```

- **Cookies** work when the page and the API share a site
  (`app.shop.com` and `api.shop.com`, with the page's origin in the
  [http](/esquadro/blueprints/reference/http/) blueprint's `cors_origins`).
  The client keeps the session's CSRF token and sends it with each change.
- **Tokens** work from anywhere: `signIn` on a client without a cookie
  answers the `session_token`, to keep where the app keeps secrets.
- A server never shares one client between people.

## Records

```ts
const page = await api.products.list({ status: ["ACTIVE"], sort: "-price", include: ["category"] })
const p = await api.products.get(id, { fields: ["name", "price"] }) // typed with just those
await api.products.patch(p.id, { price: "13.00" }, { ifMatch: p })  // only if not changed since
for await (const x of api.products.all()) { ... }
```

The types follow the model: a list's filters and orders, the fields
[`fields`](/esquadro/related-records/) may ask for and the type that
answers, the relations `include` adds. Data keeps the API's snake_case
names; the client's own options are camelCase. `create` sends an
[Idempotency-Key](/esquadro/safe-writes/) (pass the same `idempotencyKey`
when retrying); `ifMatch` takes the record read. Each record's `can`, and
each list's, say what the person may do.

## Errors and forms

```ts
import { isApiError, validate } from "@acme/shop-client"

const problems = validate.newProduct(form) // the model's rules, with the API's codes
try {
  await api.products.create(form)
} catch (e) {
  if (isApiError(e, "invalid_fields")) show(e.fields)
}
```

An operation the project writes by hand, outside the model, is
`api.request(method, path, { query, body })`, untyped.

## Its tests

`go test ./cmd/<app>` runs the client's tests (`clients/ts/test`) against
the API, with Node 22.18 or later, which runs TypeScript as it is; without
it, they are skipped. The CI also type-checks the client.

## Cancelling a read

Every read takes a signal last, to cancel it when it is no longer wanted:
a screen left, a search typed over. A cancelled read rejects with an
`AbortError` (not an `ApiError`), and is never sent again.

```ts
const controller = new AbortController()
const page = api.products.list({ q: "webcam" }, { signal: controller.signal })
controller.abort()
```

With TanStack Query, pass the `signal` it gives each query: it cancels the
queries no screen uses any more.

```ts
useQuery({ queryKey: ["products", q], queryFn: ({ signal }) => api.products.list({ q }, { signal }) })
```

Changes (create, update, patch, delete) take no signal: the server may have
made one before it learns the request was cancelled.

