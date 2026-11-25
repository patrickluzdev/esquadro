---
title: The contract
description: One OpenAPI document with every route and rule, for clients and their generators.
---

`api/openapi.yaml` is the contract of the whole service, generated from the
model and served at `/api/openapi.yaml` (the page `/docs` shows it, with
[api_docs](/esquadro/blueprints/reference/api_docs/)). It is what a client,
generated or written by hand, builds on:

- **Every route, in full:** the API's (`/api/v1/products`) and, with
  [security](/esquadro/security/), the identity ones
  (`/identity/customer/sign-in`), under the server `/`.
- **The model's rules** on the bodies that create and change records
  (create, `PUT`, `PATCH`) and on sign-up: `minLength`, `maxLength`,
  `minimum`, `maximum` and `pattern`. A form can check them before sending.
  A `decimal`, which travels as text with `format: decimal`, carries its
  bounds as `x-minimum` and `x-maximum`, and in its description.
- **Money** as the schema `Money`: `{"amount": "19.90", "currency": "BRL"}`,
  the amount as text (up to four places, never a JSON number), the currency
  an ISO 4217 code, the app's when a request leaves it out (the `default` of
  the schema, when the app says one). An amount with
  more places is refused as `too_precise`, a currency that is no code as
  `invalid_value`; a list filters a money field by its amount
  (`price_min`, `price_max`).
- **Times** in RFC 3339, with the microseconds the database keeps: one that
  was read, sent back in a filter or in a `PUT`, is the one that is stored.
- Each list's filters, orders and pages, the fields `fields` may ask for, the
  relations `include` may answer, and every error code.
- What the person may do: `can` on each record (`update`, `delete`) and on
  each list (`create`), as the [policies](/esquadro/security/#policies)
  decide; every action without security.

## The rules, and the API that checks them

The API checks the same rules in the domain, and refuses with
`422 invalid_fields`, listing every field refused at once. So that it keeps
doing so, the server is generated from the contract's API part without the
model's rules: a client that skips checking still gets the full list.

- Text left empty means "no value": leave the field out, or send `null` in a
  `PATCH`. The rules apply to every value sent.
- A `pattern` that JavaScript reads differently from Go (flags such as
  `(?i)`, `\A`, `\z`, `\p{...}`) is left out of the contract, and
  `esquadro check` says so; the API still checks it.
- `unique` is not a rule a contract can state: the API answers `taken`.

## Operations of your own

Contribute them from a local blueprint to the `rest.paths` and
`rest.schemas` slots, with full paths (`/api/v1/reports`), and implement
them in the module.

## Calling from a browser

Pages on another origin call the API once it is listed in the
[http](/esquadro/blueprints/reference/http/) blueprint's `cors_origins`
option (`<APP>_CORS_ORIGINS` per environment), in every project with an HTTP
server.
