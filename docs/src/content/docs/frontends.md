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
    idle_minutes 30              // sign out after 30 minutes without use
  }
}
```

The project gets `web/admin`: React, Vite and TypeScript, the API client,
sign-in with the session cookie, a responsive layout with a light and a dark
theme, and the app's binary serves its build. `task web:install` installs its
packages, `task dev` runs the API and the frontend together, `task web:build`
builds it.

## The look

The frontend looks like a modern admin template: the Inter font, a vivid
blue, a light gray page with white rounded cards, filled fields, bold
buttons, tags tinted by their state, tables with small-capital headers. Its
colors and radii are tokens in `src/theme.css`, and its components are in
`src/components/ui`: both are yours to change.

- A list is one card: its title and actions, a wide search, Filters opening
  a panel, the table, and numbered pages with how many per page.
- A form is in cards (the resource's `sections`, or one), with Delete and
  Save in a bar at the foot of the screen (and Back, when editing).
- A record's page is a summary card (its picture, its fields, its actions)
  beside tabs of its related records.
- The sign-in page is a narrow column on white.

A record's picture sits beside its name in lists and atop its page. Until a
resource says where it is, it is the first letters of the name; `image`
gives its URL, and a record with none (or one that fails to load) keeps its
letters:

```tsx
export default resource(meta, {
  // ...
  image: (r) => r.photo_url as string | null,
})
```

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
`user-menu.tsx`, `site-header.tsx`, `src/pages/_app.tsx`), written once: to
change the layout later, change them, or take another block from
ui.shadcn.com/blocks. The header has, at the left, the sidebar's button, the
search (it opens the command palette, as Cmd+K does) and where the page is (a
breadcrumb); at the right, the account's avatar, whose menu has the account's
page, the theme (light, dark or the system's) and signing out.

In a list, `j` and `k` (or the arrows) move between records, `Enter` opens
one, `e` edits it and `x` selects it; on a record, `e` edits and `Esc` goes
back. `?` lists the shortcuts.

The command palette (Cmd+K, or Ctrl+K, or the header's Search button) goes
to any page, finds records of every searchable resource at once, creates a
record of any resource, changes the theme and signs out, all from the
keyboard.

## The dashboard

The first page shows the reports marked as widgets: a number, bars, a line
or a table, for a period (today, the last 7 or 30 days, all time) that
fills the report's time parameter, chosen at the right of its title. The
numbers sit side by side in an overview, each with its icon (`icon`) in a
colored circle; the charts and tables, each in a card, below them. A number
with an `x` (a day) is also drawn over it, under the numbers: a click on
another number with one draws that one instead.

```esquadro
report SalesPerDay {
  day   string  required
  total decimal required @admin(label "Sales")
  since time    param
  @admin(widget stat, x day, y total, icon dollar_sign, since since)
}
```

```esquadro
report ProductsByCategory {
  category string required @admin(label "Category")
  products int    required @admin(label "Products")
  since    time   param
  @admin(widget bar, x category, y products, label "Products by category", since since)
}
```

`widget` is `stat` (the `y` column summed), `bar` and `line` (`x` along,
`y` up) or `table` (every column); `check` tells a column or parameter it
does not have. The page is yours: place `<Dashboard />` where you want it,
with cards and charts of your own.

## Sign-in

Every page under `src/pages/_app` needs a session of the realm (`auth`, the
first realm by default) and, with `roles`, one of those roles. Without one,
the sign-in page opens, keeping the page asked for: after signing in, the
person is back on it. A session that ends while the frontend is open does the
same. Sign-in, the forgotten password and its link's page come ready.

With `idle_minutes`, the frontend signs out after that long without use
(in any of its tabs), warning a minute before, and comes back to the page
after signing in again.

The settings page (`/settings`, from the account's menu) changes the
password, lists the devices signed in and signs out of one or of all the
others, changes the email with a link to the new one, and picks the theme. What changes an account needs a recent
sign-in: when it is old, the page offers to sign in again and comes back.

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
    alone (`PATCH`) on the version shown (`If-Match`), showing the new value
    at once and going back to the old one if the API refuses it;
  - each record has a menu: View, Edit, your actions, Delete;
  - Columns shows and hides columns (`hidden` starts one hidden), and sets
    the rows' density, comfortable or compact (kept in the browser).
  - A date in the filters (`created_at`, which every resource can filter
    by, or a date field marked `@store(filter)`) is a period: today, the last
    7 or 30 days, this month, or a range picked in the calendar.
  - Views saves the list's search, filters and sorting under a name, to
    come back to (kept in the browser).
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
  When a list cannot be refreshed, it keeps what it had and says it may be
  out of date.
- Offline, a bar says so (and a notice when the connection is back, when
  the data reload); signing in or out in a tab does it in the others.

What a resource shows is **yours**, in `src/resources/<resource>.tsx`,
written once with the model's fields and left alone after:

```tsx
export default resource(meta, {
  table: {
    columns: [f.name, column(f.price, { editable: true }), column(f.status, { editable: true })],
    filters: [f.status, f.price],
  },
  form: [f.name, f.price, f.status],
  actions: [
    { label: "Publish", icon: Send, bulk: true, run: (p) => api.request("POST", `/api/v1/products/${p.id}/publish`) },
  ],
})
```

A field the model gains is in `meta.fields` at once; add it where you want it.

Everything a resource may say, each with a default that works without it,
is in [Customizing a frontend](#customizing-a-frontend).

A `money` field is shown with its currency (`R$ 1.234,56`), exactly: its
amount is text, never a float. Its input writes it as the language does,
with the currency's symbol and places; a list filters it by a range (from,
up to); a number card sums it exactly, one total per currency; a CSV gives
its amount and its currency in two columns.

## Customizing a frontend

Everything has a default that works without saying anything; each is
changed where it belongs: the model (`.esq`), the resource
(`src/resources/<resource>.tsx`), your fields (`src/fields.ts`), the look
(`src/theme.css`, `src/components/ui`) and your pages (`src/pages`).

### In the model

| Annotation | On | What | Default |
|---|---|---|---|
| `resource` | entity | It is a resource: its list, form and page | not one |
| `label`, `singular` | entity | What people call it, and one of it | its name, humanized |
| `menu`, `group`, `icon` | entity | Its menu entry, the section it is in, its Lucide icon | the label, none, a circle |
| `label` | field, relation | What people call it | its name, humanized |
| `label` | enum value | What people call the value | the value |
| `color` | enum value | Its dot: green, amber, red, blue, violet, sky, orange, gray | a guess from its name |
| `widget`, `x`, `y`, `label`, `since`, `icon` | report | A widget of the [dashboard](#the-dashboard) | not one |

Each annotation is named after the frontend: `@admin(...)` for
`blueprint spa admin`. The [blueprint's reference](/esquadro/blueprints/reference/spa/)
lists them, and its options (the domain, the realm and roles, the language,
the sidebar, the idle time).

### A resource

```tsx
export default resource(meta, {
  table: {
    columns: [f.name, column(f.price, { editable: true }), column(f.status, { label: "State", hidden: true })],
    filters: [f.status, f.price, f.created_at],
    search: true,
    sort: "-created_at",
    select: true,
  },
  form: [f.name, f.description, f.price, f.photo, f.status, f.category_id],
  sections: [
    { title: "Details", fields: [f.name, f.description, f.price] },
    { title: "Photo", fields: [f.photo], side: true },
    { title: "Filing", description: "Where it shows up.", fields: [f.status, f.category_id], side: true },
  ],
  show: [f.name, f.price, f.status, f.category_id, f.created_at],
  title: f.name,
  subtitle: (p) => `SKU ${p.sku}`,
  image: (p) => (p.photo as { url?: string } | null)?.url,
  relations: () => [relation(offers, "product_id", { label: "Offers" })],
  actions: [
    { label: "Publish", icon: Send, bulk: true, when: (p) => p.status === "DRAFT", run: (p) => api.request("POST", `/api/v1/products/${p.id}/publish`) },
    { label: "Archive", icon: Archive, destructive: true, confirm: "Archive it?", run: (p) => api.products.patch(p.id, { status: "ARCHIVED" }, { ifMatch: p }) },
  ],
})
```

| Option | What | Default |
|---|---|---|
| `table.columns` | The list's columns: a field, or `column(field, {label, cell, editable, hidden})` | the first fields but images |
| `table.filters` | The filters shown: only those the API has | the enums, booleans and money the API filters by, its relations, and `created_at` |
| `table.search`, `table.sort`, `table.select` | The search box, the first order, the checkboxes | as the API allows, `-created_at`, on |
| `form` | The fields of the form | every field |
| `sections` | The form's cards: `{title, description, fields, side}`; `side` puts one in the narrower column at the right | details at the left; media and filing at the right |
| `show` | The fields of a record's page | every field but images, and when it was created and changed |
| `title` | What a record is called, in titles, the breadcrumb and pickers | the first field |
| `subtitle` | A second line under each name in the list: a field or a function | the first relation |
| `image` | A record's picture, in lists and atop its page | its first image field, else its initials |
| `relations` | Lists of related records on its page | those the API filters by |
| `actions` | Your actions: `{label, icon, run, when, confirm, bulk, destructive}` | none |

### Fields of your own

`src/fields.ts` shows or edits a field your way, by its name (`price`) or by
its kind (`money`, `enum`, `image`), everywhere it appears:

```ts
export const fields: Record<string, FieldRenderer> = {
  price: { show: (v) => <b>{formatMoney(v)}</b> },
  color: { show: (v) => <span style={ { color: String(v) } }>●</span>, input: (v, onChange) => <input type="color" value={String(v ?? "")} onChange={(e) => onChange(e.target.value)} /> },
}
```

### The look

The colors, the radius and the font are tokens in `src/theme.css` (light
and dark); the components are shadcn/ui's, in `src/components/ui`, and the
layout's (sidebar, header, account menu) in `src/components`: all yours to
change. `npx shadcn add <component>` adds more.

The account menu, at the right of the header (`src/components/user-menu.tsx`),
shows who is signed in, links to the settings and signs out; add
your links (a profile, an activity log) to its `links`. The settings page's
sections (password, devices, email, theme) are in `src/pages/_app/settings.tsx`
(`/settings`): the place for every setting, each in a section of its own.

### Pages of your own

A file in `src/pages/_app/` is a page behind the sign-in. The kit's parts
are yours to use there: `PageHeader`, `Dashboard`, the charts, and
`Settings`, sections in a menu at the left, the one chosen kept in the URL,
which the settings page is made of. `ask` asks before acting, with an icon
of its `kind` (`info`, `success`, `warning`, `danger`; `destructive` is a
danger), a `description` and the button's name
(`await ask("Archive it?", { kind: "warning", description: "…", confirm: "Archive" })`),
and `toast` tells what happened (`toast("Archived")`, `toast(text, "error")`).

The settings page is made of:

```tsx
<Settings
  sections={[
    { key: "password", label: t("password"), icon: KeyRound, content: <PasswordCard /> },
    { key: "profile", label: "Profile", icon: User, content: <MyProfile /> },
  ]}
/>
```

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
