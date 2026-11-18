---
title: Frontends
description: Single-page frontends of the app, in the same repository, with resources made from the model and screens of your own.
---

An app's frontends live in its repository, beside it: the model, the API, the
TypeScript client and the screens change in one commit. A frontend is a
blueprint used by name, the architecture first and its purpose after:

```esquadro
app shop {
  blueprint client_ts
  blueprint workspace            // the repository: one npm workspace
  blueprint spa admin {          // a single-page app, for the team
    domain "admin.shop.com"      // or, without one, under /admin
    roles  [admin, staff]        // who of the realm may use it
    locale pt_br                 // en or pt_br
  }
}
```

The project gets `web/admin`: React, Vite and TypeScript, the API client,
sign-in with the session cookie, a responsive layout with a light and a dark
theme, and the app's binary serves its build. `task web:install` installs its
packages, `task dev` runs the API and the frontend together, `task web:build`
builds it.

## Sign-in

Every page under `src/pages/_app` needs a session of the realm (`auth`, the
first realm by default) and, with `roles`, one of those roles. Without one,
the sign-in page opens, keeping the page asked for: after signing in, the
person is back on it. A session that ends while the frontend is open does the
same. Sign-in, the forgotten password and its link's page come ready.

## Resources

An entity marked `@admin(resource)` is a resource of the admin frontend: its
list, form and page.

```esquadro
entity Product {
  name  string  required @admin(label "Name")
  price decimal required @admin(label "Price")
  @admin(resource, label "Products", icon package)
}
```

- The list pages, filters, sorts and searches through the API, with all of it
  in the URL: a list can be shared, and Back works.
- The form has a field for each kind of field (text, numbers, money, dates,
  enums, relations), shows the API's refusals on their fields, changes the
  version it read (`If-Match`: a change someone made meanwhile is not
  overwritten), creates once however often it is sent (`Idempotency-Key`),
  and asks before leaving with unsaved changes.
- What the person may do (`can`) decides the buttons; the API decides again.

What a resource shows is **yours**, in `src/resources/<resource>.tsx`,
written once with the model's fields and left alone after:

```tsx
export default resource(meta, {
  table: { columns: [f.name, column(f.price, { cell: (p) => money(p.price) })], filters: [f.status] },
  form: [f.name, f.price, f.status],
  actions: [{ label: "Publish", run: (p) => api.request("POST", `/api/v1/products/${p.id}/publish`) }],
})
```

A field the model gains is in `meta.fields` at once; add it where you want it.

## Yours, and generated

What is generated is regenerated; what is yours is written once and left
alone.

| | Where | Whose |
|---|---|---|
| The model's fields, filters and sorts of each resource | `src/esquadro/` | generated |
| The kit: lists, forms, pages, sign-in | `src/esquadro/kit/` | generated |
| Each resource's columns, form and actions | `src/resources/` | yours |
| Pages of your own, and each resource's routes | `src/pages/` | yours: a file is a route |
| Components, the menu, the theme, the words | `src/components/`, `src/menu.ts`, `src/theme.css`, `src/messages.ts` | yours |
| How a field is shown or edited, by name or kind | `src/fields.ts` | yours |

## Where it runs

The app's binary serves the frontend's build (`serve app`, the only way so
far): on its domain, where the app's own paths (`/api`, `/identity`) still
reach the app, so the frontend calls the API without CORS; or under its path,
without one. The image builds the frontend with Node first. On a
[platform](/esquadro/production/), add the frontend's domain to the app's
`domains`.
