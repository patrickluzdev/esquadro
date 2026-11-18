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
    sidebar icon                 // icon, inset or dashboard
  }
}
```

The project gets `web/admin`: React, Vite and TypeScript, the API client,
sign-in with the session cookie, a responsive layout with a light and a dark
theme, and the app's binary serves its build. `task web:install` installs its
packages, `task dev` runs the API and the frontend together, `task web:build`
builds it.

## The layout

The frontend is built with [shadcn/ui](https://ui.shadcn.com): its
components are copied into `src/components/ui`, yours to change, and
`npx shadcn add <component>` adds more. The sidebar is one of shadcn/ui's
blocks, chosen by `sidebar`:

| `sidebar` | Block | What it is like |
|---|---|---|
| `icon` (default) | sidebar-07 | It collapses to its icons; groups open and close |
| `inset` | sidebar-08 | The page is an inset card beside it |
| `dashboard` | dashboard-01 | It slides away, wider, beside an inset page |

Its files are yours (`src/components/app-sidebar.tsx`, `nav-main.tsx`,
`nav-user.tsx`, `site-header.tsx`, `src/pages/_app.tsx`), written once: to
change the layout later, change them, or take another block from
ui.shadcn.com/blocks. The header shows where the page is (a breadcrumb), and
the account's menu has the theme (light, dark or the system's) and signing
out.

The command palette (Cmd+K, or Ctrl+K, or the header's Search button) goes
to any page, finds records of every searchable resource at once, creates a
record of any resource, changes the theme and signs out, all from the
keyboard.

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
  @admin(resource, label "Products", singular "Product", icon package, group "Catalog")
}
```

- `label` names the resource, its fields, its relations and the values of
  its enums in the frontend's language (`ACTIVE @admin(label "Ativo")`). With a `locale` other than `en`, `esquadro check`
  warns of each field of a resource without one, which would show its name
  in English.
- `group` puts the resource in a section of the menu, with the other
  resources of the same group.
- The list pages, filters, sorts and searches through the API, with all of it
  in the URL: a list can be shared, and Back works.
- The form has a field for each kind of field (text, numbers, money, dates,
  enums, relations), shows the API's refusals on their fields, changes the
  version it read (`If-Match`: a change someone made meanwhile is not
  overwritten), creates once however often it is sent (`Idempotency-Key`),
  and asks before leaving with unsaved changes.
- The search searches as people type (after a pause), finds words by
  their start ("tecl" finds "Teclado"), keeps `"exact phrases"` and
  `-excluded` words, clears with its button or Esc, and is reached with `/`;
  the words found are marked in the list. Only the filters the API has (a
  field marked `@store(filter)`) are shown.
- The list is a data table (TanStack Table, in shadcn/ui's way):
  - checkboxes select records, and a bar acts on all of them: Delete, and
    your actions marked `bulk`, each record in turn, telling how many went
    through and failed;
  - a column marked `editable` is changed in the list itself: a choice or a
    checkbox saves at once, text on Enter (Esc gives up), changing that field
    alone (`PATCH`) on the version shown (`If-Match`);
  - each record has a menu: View, Edit, your actions, Delete;
  - Columns shows and hides columns (`hidden` starts one hidden).
  - A date in the filters (`created_at`, which every resource can filter
    by, or a date field marked `@store(filter)`) is a period: today, the last
    7 or 30 days, this month, or a range picked in the calendar.
  - Export saves what the search and filters find as a CSV file (the
    records selected, from the selection's bar): the columns shown, as people
    read them, with `;` and a byte-order mark where the language's
    spreadsheets expect them.
- A record's page lists the records of other resources that point at it
  (a category's products), each with the list's search, filters and actions;
  its New starts with the record (`?category_id=`). They are the resource's
  `relations: () => [relation(products, "category_id")]`, written from the
  model when the API filters by the key.
- A new record's form starts with the model's defaults.
- Each value on a record's page copies to the clipboard (a button on
  hover), as each record's id does from its menu in the list.
- Duplicate (on a record's page and in its menu) opens the form to create
  one with its values, what it is called marked as a copy; nothing is saved
  until the person saves.
- What the person may do (`can`) decides the buttons; the API decides again.
- Every page has the same states: a skeleton of what comes while it first
  loads; what was there, dimmed under a moving bar, while it loads again;
  empty, with a button to create the first; nothing found, with a button to
  clear the search and filters; and an error, in the frontend's words, with
  Try again (and, for a record not found or not allowed, Back to the list).
  When a list cannot be refreshed, it keeps what it had and says so.

What a resource shows is **yours**, in `src/resources/<resource>.tsx`,
written once with the model's fields and left alone after:

```tsx
export default resource(meta, {
  table: {
    columns: [f.name, column(f.price, { cell: (p) => money(p.price) }), column(f.status, { editable: true })],
    filters: [f.status],
  },
  form: [f.name, f.price, f.status],
  actions: [
    { label: "Publish", icon: Send, bulk: true, run: (p) => api.request("POST", `/api/v1/products/${p.id}/publish`) },
  ],
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
