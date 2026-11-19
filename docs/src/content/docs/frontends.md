---
title: Frontends
description: "The app's frontends, in the same repository: single-page panels and sites rendered on the server, made from the model, with screens of your own."
---

An app's frontends live in its repository, beside it: the model, the API, the
TypeScript client and the screens change in one commit. A frontend is a
blueprint used by name, the architecture first and its purpose after:

```esquadro
app shop {
  blueprint client_ts
  blueprint workspace            // the repository: one npm workspace
  blueprint ui                   // what every frontend shares
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

## What every frontend shares

An app may have several frontends: a panel for the team, a portal for its
customers, a public site. What they all show the same way is said once, by
the `ui` blueprint (`esquadro add spa admin` adds it):

```esquadro
entity Product {
  name   string  required @ui(label "Name")
  status Status           @ui(label "Status") @portal(label "Availability")
  @ui(label "Products", singular "Product", icon package)
  @admin(resource, group "Catalog")
}

enum Status {
  ACTIVE   @ui(label "Active", color green)
  INACTIVE @ui(label "Inactive")
}
```

| `@ui` | On | What | Default |
|---|---|---|---|
| `label`, `singular` | entity | What people call it, and one of it | its name, humanized |
| `icon` | entity | Its Lucide icon: `tag`, `users`, `shopping_cart` | a circle |
| `label` | field, relation, report column | What people call it | its name, humanized |
| `format` | field, report column | `percent` (0.42 as 42%) or `duration` (seconds as 03:45) | the value as is |
| `label`, `color` | enum value | What people call the value; its dot: green, amber, red, blue, violet, sky, orange, gray | the value; a guess from its name |

- **One frontend's own words.** Each frontend has its annotation, named
  after it (`@admin` for `blueprint spa admin`), for what is only its own
  (a resource, a menu group, a widget), and a `label` or `singular` there
  wins over `@ui`'s, in that frontend only.
- **The same options**, meaning the same in every frontend: `domain`,
  `path`, `auth`, `roles`, `locale`, `serve`, `primary`, `primary_dark`,
  `radius`; `esquadro check` checks them alike.
- **Who signs in** is the security blueprint's: `@security(principal,
  signup open, providers [password, google])` makes an entity sign in (a
  realm), with the sign-up and the providers it allows; a frontend's `auth`
  only chooses the realm, and shows the sign-up and the providers that
  realm has.
- **The same words**, in its language, for what every frontend says
  (signing in, the account, errors, empty lists, pages, fields), and **the
  same theme** from the same options: the app's frontends look alike by
  default.
- **Where each answers:** on its `domain`, or under its `path` on the
  app's (`/<name>` by default). Two frontends cannot answer at the same
  place, nor one under the app's own routes (`/api`, the security's
  `/identity`, `/healthz`, `/readyz`, `/docs`, `/.well-known`): `esquadro
  check` says which. On a [platform](/esquadro/production/), add each
  frontend's domain to the app's `domains`.

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

## Dashboards

The reports marked as widgets make dashboards: the first page's, and one
page for each `dashboard` the reports name, in a Dashboards group of the
menu (`/dashboards/sales`). Each has a period at the right of its title
(today, the last 7, 30 or 90 days, this month, all time) that fills the
reports' time parameters. The period and the number drawn are in the URL
(`?period=7d&kpi=orders-per-day`): a dashboard's link opens it as it was.

```esquadro
report RevenuePerDay {
  day     string required @ui(label "Day")
  revenue money  required @ui(label "Revenue")
  since   time   param
  until   time   param
  @admin(widget stat, x day, y revenue, icon dollar_sign, since since, until until, width two_thirds, dashboard "Sales")
}

report RevenueByChannel {
  channel Channel required
  revenue money   required
  since   time    param
  @admin(widget donut, x channel, y revenue, label "Revenue by channel", since since, dashboard "Sales")
}
```

| Widget | What it shows | Needs |
|---|---|---|
| `stat` | A number in the overview, with its icon; with `x`, a click draws it over time; with `until`, how it changed from the period before | `y` |
| `area`, `line` | Curves over `x` (days, months), a series per `y` column or per value of `series`, with a legend | `x`, `y` |
| `bar` | Bars, grouped with several series; across, when the names are long | `x`, `y` |
| `combo` | Bars of the first `y`, a line of the second, on two scales | `x`, `y [bars, line]` |
| `donut` | Each name's share, the total in the middle, the percentages under it | `x`, `y` |
| `progress` | Each name with a bar of its share of the total (or of its `target`) | `x`, `y` |
| `goal` | A ring of `y` over `target`: how far along | `y`, `target` |
| `top` | A ranking: picture (`image`) or initials, name, second line (`subtitle`), value and change (`delta`) | `x`, `y` |
| `table` | Every column: enums with their dots, money exact, percentages with a bar | — |
| `radar` | Scores on a radar, numbered under it, colored by how high | `x`, `y` |
| `timeline` | What happened: who (`subtitle`), what (`y`) and when (`x`) | `x`, `y` |

| Option | What | Default |
|---|---|---|
| `x`, `y`, `series` | The names, the numbers (one column or a list), the column splitting the rows into series | — |
| `since`, `until` | The time parameters the period fills; with `until`, a number compares with the period before | — |
| `total` | How a number is reckoned from its rows: `sum`, `last` (a running total), `average`, `max` | `sum` |
| `target` | The goal (`goal`), or what is 100% (`progress`) | — |
| `image`, `subtitle`, `delta` | A ranking's picture, second line and change | — |
| `width` | `third`, `half`, `two_thirds`, `full` | the charts two thirds, numbers and tables full, the rest a third |
| `rows` | How many rows a ranking, list, timeline or table shows | 6 (8 for a table) |
| `dashboard` | The dashboard it is on | the first page's |
| `icon`, `label` | A number's Lucide icon; what the dashboard calls it | a chart; the report's name |

A column's `@ui(format percent)` shows `0.42` as `42%` (with a bar in a
table, on a 0–100% scale on a radar), and `format duration` shows seconds as
`03:45`. An enum's colors (`@ui(color green)`) color its slices, series
and dots. The numbers are the reports': a dashboard asks each once for the
period (and once more for the period before, with `until`), and keeps them a
minute.

The pages are yours: `src/pages/_app/index.tsx` and
`src/pages/_app/dashboards/$slug.tsx`. Each widget is in the kit
(`SeriesChart`, `DonutChart`, `ProgressList`, `GoalRing`, `TopList`,
`RadarScores`, `Timeline`, `ReportTable`) for pages of your own, drawn with
shadcn/ui's charts.

## Sign-in

Every page under `src/pages/_app` needs a session of the realm (`auth`, the
first realm by default) and, with `roles`, one of those roles. Without one,
the sign-in page opens, keeping the page asked for: after signing in, the
person is back on it. A session that ends while the frontend is open does the
same. Sign-in, the forgotten password and its link's page come ready.

With `idle_minutes`, the frontend signs out after that long without use
(in any of its tabs), warning a minute before, and comes back to the page
after signing in again.

The pages before signing in (signing in, a forgotten password, a new one,
an email confirmed, no access) share one frame, `src/components/auth-frame.tsx`:
the brand, or how it ended (`tone`: success, error, info, forbidden), a
title, a hint and a way back to signing in. A link that expired says so.

When the security blueprint declares `services`, Administration has a
Service keys page too, for those who administer them (the `service_admins`
option): the service roles in cards, with what each may do as the rules
say; the keys, with their roles, status (active, expired, revoked), when
they expire and when they were last used; New key, which shows the key
once, to copy; and Revoke. The lifetimes it offers are the security
blueprint's `service_key_choices`, within `service_key_max_days`. The spa's
`service_keys false` leaves the page out; it is
`src/pages/_app/service-keys.tsx`, yours.

When the panel's realm signs in with providers
(`@security(providers [password, google, github])`), the sign-in page has
their buttons below the form ("or continue with"), or only them without a
password. Back from the provider, the person goes on to the page asked
for; when it failed, the sign-in page says why (no account with the email
and sign-up closed, the provider's account linked to someone else, an
account with the email to connect from the settings). The settings have
Connected accounts, to connect or disconnect a provider, never the last
way to sign in. The spa's `social_login false` leaves the buttons out.

Dates and times are picked in a calendar, in the frontend's language,
with Today and Clear (and Now and the hours and minutes, for a time); a
many-to-many relation is a field of chips with a search (see
[Related records](/esquadro/related-records/)).

A `text` field marked `@rest(html)` is formatted text: the form edits it
with a rich text editor (bold, italic, headings, lists, quotes, code,
links), the record's page shows it formatted, and lists show it as plain
text. The API keeps only what such an editor makes, and drops the rest
(scripts, styles, handlers, `javascript:` links) when it is written, so the
stored text is safe for any client; the frontend cleans it again when it
shows it.

```esquadro
entity Product {
  description text @rest(html)
}
```

With the [notifications](/esquadro/notifications/) blueprint, the header has
a bell with how many notices are unread, the last ones in a popover (a
click leads to the record and marks it read) and a page with all of them.

With the [audit](/esquadro/audit/) blueprint, an audited record's page has a
History tab, and Administration an Audit page with every change.

The lists of entities with a trash (`@store(soft_delete)`) have a Trash
button; deleting there can be undone, and the trash restores or deletes for
good: see [The trash](/esquadro/trash/).

The realms a frontend administers (whose entity lets its own realm
write it, `@security(write user.admin)`) each have a page under
Administration in the menu, for those who may: the realm's roles in cards,
with their permissions (as the model declares them); the accounts, with
their roles, status (active, invited, email not confirmed, disabled) and
when they were last seen, searched and filtered in the URL; Invite, which
emails a link to choose the password (opening the reset page greeted,
`?invite=1`); and, on each, Change roles, Send the invitation again, Sign
out everywhere, Disable and Delete, never on one's own account. Its page
is `src/pages/_app/accounts/$slug.tsx`, yours; the entity's
`@<frontend>(label, singular)` names it.

The settings page (`/settings`, from the account's menu) changes the
password, lists the devices signed in and signs out of one or of all the
others, changes the email with a link to the new one, and picks the theme. What changes an account needs a recent
sign-in: when it is old, the page offers to sign in again and comes back.

## Resources

An entity marked `@admin(resource)` is a resource of the admin frontend: its
list, form and page.

```esquadro
entity Product {
  name  string  required @ui(label "Name")
  price decimal required @ui(label "Price")
  @ui(label "Products", singular "Product", icon package)
  @admin(resource, group "Catalog")
}
```

- `@ui`'s labels name the resource, its fields, its relations and the
  values of its enums in the frontend's language (`ACTIVE @ui(label
  "Ativo")`). With a `locale` other than `en`, `esquadro check` warns of
  each field of a resource without one, which would show its name in
  English.
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
  - The filters are in a drawer at the right (or a dialog, or a bar beside
    the search: `filterLayout`), changed in a draft and applied at once;
    those set show as chips under the search, each with an X, and Clear
    all. An enum's values are checked, several at once, each with its dot;
    yes or no is three buttons; a related record is searched by its name;
    money is a range, from and up to, with the currency's symbol.
  - Tabs above the list (`tabs: f.status`) filter by an enum's value, or
    yes or no, each with how many records it keeps; a funnel in a column's
    header (`columnFilters: true`) filters by that column.
  - A date in the filters (`created_at`, which every resource can filter
    by, or a date field marked `@store(filter)`) is a period: today, the last
    7 or 30 days, this month, or a range picked in the calendar.
  - Save filter, beside the filters set, keeps the list's search, filters
    and order under a name ("Active and expensive"); Saved filters, beside
    Filters once there is one, applies it again in one click, and marks the
    one applied (kept in this browser).
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

Labels, icons, formats and colors are `@ui`'s ([above](#what-every-frontend-shares)).
The frontend's own annotation adds:

| Annotation | On | What | Default |
|---|---|---|---|
| `resource` | entity | It is a resource: its list, form and page | not one |
| `menu`, `group` | entity | Its menu entry, the section it is in | the label, none |
| `label`, `singular` | entity, field, relation, enum value | What this frontend calls it, over `@ui`'s | `@ui`'s |
| `widget`, `x`, `y`, `series`, `since`, `until`, `total`, `width`, `dashboard`… | report | A widget of a [dashboard](#dashboards) | not one |

Each annotation is named after the frontend: `@admin(...)` for
`blueprint spa admin`. The [blueprint's reference](/esquadro/blueprints/reference/spa/)
lists them, and its options (the domain, the realm and roles, the language,
the sidebar, where the lists' filters are, the idle time).

### A resource

```tsx
export default resource(meta, {
  table: {
    columns: [f.name, column(f.price, { editable: true }), column(f.status, { label: "State", hidden: true })],
    filters: [f.status, f.price, f.created_at],
    filterLayout: "drawer",
    tabs: f.status,
    columnFilters: true,
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
| `table.filterLayout` | Where the filters are: `drawer`, `dialog` or `bar` | the spa's `filters` option, `drawer` |
| `table.tabs` | Tabs above the list by an enum or a yes or no field, with each value's count | none |
| `table.columnFilters` | A funnel in the header of each column the list filters by | off |
| `table.search`, `table.sort`, `table.select` | The search box, the first order, the checkboxes | as the API allows, `-created_at`, on |
| `form` | The fields of the form | every field |
| `sections` | The form's cards: `{title, description, fields, side}`; `side` puts one in the narrower column at the right | details at the left; media and filing at the right |
| `show` | The fields of a record's page, in its Details tab (in the form's sections, when it has them) | every field but images, and when it was created and changed |
| `summary` | The fields beside the picture, atop the buttons | the first four short ones but the title |
| `title` | What a record is called, in titles, the breadcrumb and pickers | the first field |
| `subtitle` | A second line under each name in the list: a field or a function | the first relation |
| `image` | A record's picture, in lists and atop its page | its first image field, else its initials |
| `relations` | Lists of related records on its page | those the API filters by |
| `actions` | Your actions: `{label, icon, key, patch, ask, copy, run, when, confirm, bulk, destructive}` | none |
| `queue` | A queue beside the table: `{filter, card: {image, title, fields}, label}` | none |

### Queues and actions

Some lists are worked rather than read: offers to approve or skip, posts to
moderate, messages to copy and mark as sent. A resource's `queue` puts the
records of a filter beside its table, in a **Queue** tab (the browser
remembers which one was open), as cards: one in focus, `j`/`k` (or the
arrows) to move, `Enter` to open, and each action's key to run it. A record
an action takes out of the filter leaves, and the next takes its place; the
tab counts what is left.

```tsx
export default resource(meta, {
  // ...
  queue: {
    filter: { status: ["PENDING"] },
    card: { image: f.image_url, title: f.title, fields: [f.price, f.category_id] },
  },
  actions: [
    { label: "Home",  key: "1", patch: { status: "APPROVED", niche_id: niches.home }, ask: [f.hook] },
    { label: "Skip",  key: "s", patch: { status: "SKIPPED" }, bulk: true },
    { label: "Copy",  key: "c", copy: (o) => String(o.message) },
    { label: "Picture", key: "p", copyImage: (o) => String(o.image_url) },
  ],
})
```

An action does any of these, in this order:

- `copy`: copies the text a function makes of the record;
- `copyImage`: copies the image at the address a function makes of the
  record, as an image, to paste into a chat or a post ("Image copied."). It goes on
  the clipboard as PNG, the one type browsers take there; another is drawn
  as PNG first. A browser that cannot copy images says so and offers to
  download it (no download, and the action stops there);
- `patch`: changes those fields, by `PATCH` with `If-Match`, so a business
  rule's refusal shows as its text, and a record someone changed is read
  again. With `ask`, a small dialog asks for some fields of the form first,
  with their rules, and sends them along;
- `run`: your own code.

`key` runs it on the record in focus in the queue, or open on its page; a
key given to two actions of a resource is said in the console in
development. An action that changes fields shows only on records the
person may change. In the table, `bulk` runs it on the records selected,
when it asks and copies nothing (text or image).

The filter of a queue is the API's filters: a field marked
`@store(filter)`.

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

The primary color and how round the corners are come from the spa's
options, in the model:

```esquadro
blueprint spa admin {
  primary      "#3d3d3d"
  primary_dark "#e5e5e5"
  radius       "0.5rem"
}
```

The text on the primary is white, or near black where white would not
read; in the dark theme the primary is `primary_dark`, by default the
primary itself, or a light gray when it is too dark to stand out there.
They are written to `src/esquadro/theme.gen.css`, regenerated with the
model, which `src/theme.css` imports first: a token set there wins.

The colors, the radius and the font are tokens in `src/theme.css` (light
and dark); the components are shadcn/ui's, in `src/components/ui`, and the
layout's (sidebar, header, account menu) in `src/components`: all yours to
change. `npx shadcn add <component>` adds more.

The account menu, at the right of the header (`src/components/user-menu.tsx`),
shows who is signed in, links to the settings and signs out; add
your links (a profile, an activity log) to its `links`. The settings page's
sections (password, devices, email, theme) are in `src/pages/_app/settings.tsx`
(`/settings`): the place for every setting, each in a section of its own.

### Icons

The panel draws its icons by name, from one set: `src/esquadro/icons.gen.ts`,
regenerated with the model. It has the icons the kit draws (the trash, the
chevrons, the sidebar's...) and those the model names (`@ui(icon package)`
on a resource, a number's `icon` on a dashboard), by their
[Lucide](https://lucide.dev/icons) names in PascalCase (`Trash2`,
`ShoppingCart`), each Lucide's by default. `src/icons.tsx` is yours: an
icon given there is drawn instead wherever that one is, in the kit, in the
menu and in your files.

```tsx
// src/icons.tsx
import { createLucideIcon, PackageOpen } from "lucide-react"
import type { IconOverrides } from "./esquadro/icons.gen.ts"

// An icon of your own, drawn like Lucide's: 24 by 24, with its stroke.
const Bin = createLucideIcon("bin", [["path", { d: "M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13" }]])

export const icons: IconOverrides = { Package: PackageOpen, Trash2: Bin }
```

An icon is a Lucide icon, one made with `createLucideIcon`, or any
component that takes Lucide's props (`className`, `size`). Your files draw
from the same set, so that a change there reaches them too:
`import { Trash2 } from "@/esquadro/icons.gen.ts"`, or `iconOf("shopping_cart")`
for a name as the model writes it. A file that imports from `lucide-react`
itself still works (it stays a dependency), but draws Lucide's icon whatever
`src/icons.tsx` says: so does the account menu (`src/components/user-menu.tsx`),
whose links, and their icons, are yours to add. An icon the model names must be a Lucide name; the
panel only bundles the icons of the set.

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
| Components and the theme | `src/components/`, `src/theme.css` | yours |
| The menu: it starts from the model's entries (`src/esquadro/menu.gen.ts`, generated), to add, reorder or leave out | `src/menu.ts` | yours |
| The words: only those you change, and your own; the rest are the kit's (`src/esquadro/kit/messages.ts`) | `src/messages.ts` | yours |
| The sign-in and account pages: they show the kit's (`src/esquadro/kit/pages/`), or a page of yours | `src/pages/login.tsx`, `src/pages/_app/settings.tsx`... | yours |
| How a field is shown or edited, by name or kind | `src/fields.ts` | yours |
| The icons: the set, by name, is the kit's and the model's (`src/esquadro/icons.gen.ts`, generated); those you draw instead | `src/icons.tsx` | yours |

So that a new version of esquadro reaches a project, what is yours starts
small: the words, the menu and the sign-in and account pages take the kit's,
which esquadro keeps up to date, and say only what the project changes. A
new word, menu entry or page section of a new version shows up without
touching them; and what you did change in a file of yours gets esquadro's
changes merged in (see [Regenerating safely](/esquadro/regenerating/)).

```ts
// src/messages.ts: change the kit's words, and add your own
export const messages = {
  en: { signIn: "Log in", promoQueue: "Offers queue" },
  "pt-BR": { signIn: "Acessar", promoQueue: "Fila de ofertas" },
} satisfies Record<string, Words>
```

## Sites rendered on the server

A site for people and search engines (a store's shop window, a site of
offers) is a frontend too, of another architecture: `ssr`. Its pages come
rendered from the server, with what search engines and shared links read;
it is no single-page app.

```esquadro
app shop {
  blueprint workspace
  blueprint ui
  blueprint ssr site {
    engine go                // what renders it: go (the app's binary)
    domain "shop.com"        // or, without one, under path (/site)
    auth   "customer"        // the realm whose people sign in to it
    locale pt_br
    name   "Shop"            // in every title and in shared links
  }
}

entity Product {
  name  string required @store(search, sort)
  price money  required @store(sort)
  photo image
  category > Category required @store(filter)
  @ui(label "Products", singular "Product")
  @site(page, path "products", description name, schema product, menu)
}

entity Category {
  name string required
  products < Product
  @site(page, path "categories", relations [products], menu)
}

entity Customer {
  name string required
  @security(principal, signup open, providers [password, google])
}
```

Each entity marked `@site(page)` gets:

- **its list**, at `/products`: the search, filters (its enums, yes or no
  fields and related records marked `@store(filter)`) and orders the store
  allows, in the address (`?status=ACTIVE&sort=-price&page=2`); with htmx,
  a choice changes only the list;
- **a page per record**, at `/products/<id>/<slug>`: the slug comes from
  its title, and an address with another slug (an old title) moves to the
  right one; with its related records (`relations`) and a button to share
  it;
- what search engines and shared links read: the title, a description
  (`description`), the canonical address, Open Graph and its picture
  (`image`, or its first image field), structured data (`schema`:
  `product`, with its price, `article` or `organization`);
- a place in `/sitemap.xml`, and `robots.txt` keeps the account's pages
  out.

| `@site` | On | What | Default |
|---|---|---|---|
| `page` | entity | Its list and its records' pages | none |
| `path` | entity | The part of the address | its plural, in kebab case |
| `title`, `description`, `image` | entity | What a record is called, described and pictured by | the first text field; none; the first image field |
| `schema` | entity | `product`, `article` or `organization` | none |
| `sort` | entity | Its list's first order: `-price` | the newest first |
| `relations` | entity | Related records its page lists: `[products]` | none |
| `menu` | entity | An entry in the site's menu | every page, when none is marked |
| `label`, `singular` | entity, field, relation, enum value | What this site calls it, over `@ui`'s | `@ui`'s |
| `hidden` | field | Left out of the site | shown |

**Who reads what** is the model's: a page reads through the module's use
cases, with `@security(read ...)`. What everyone may read (`read public`)
everyone sees; what only the site's realm may read, those who signed in.
What the reader may not see is not found, and stays out of the sitemap.

**The visitor's account** comes with `auth`: signing in, the providers the
realm has, signing up when it is `signup open`, the forgotten password,
the email confirmed and an account page (signing out, deleting the
account). Their forms call the app's identity routes. With sign-in by
Google or GitHub and the site on a domain of its own, set the security
blueprint's `public_url` to the site's address.

**The site's own pages** ("Join our groups", "About") go in `routes.go`,
each a templ component of yours inside `Layout`.

**The site's icons** are drawn by name, `@Icon("share-2")`, as an inline
`<svg>` of the text's size and color: the kit's (the search, the pages'
chevrons, sharing) are [Lucide](https://lucide.dev/icons)'s, in
`icons_gen.go`, under their ISC license. `icons.go` is yours: an icon there
is drawn instead wherever that name is, in the kit and in your pages, and
a name of your own adds one.

```go
// icons.go
var ownIcons = map[string]string{
	// What goes inside the <svg>: a 24 by 24 grid, a 2px stroke of the text's color.
	"share-2": `<path d="M4 12v7a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-7"/><path d="m16 6-4-4-4 4"/><path d="M12 2v13"/>`,
	// A whole <svg> of your own, drawn as it is.
	"whatsapp": `<svg viewBox="0 0 24 24" width="1em" height="1em" fill="currentColor">…</svg>`,
	// None: the kit draws no chevrons.
	"chevron-left": "", "chevron-right": "",
}
```

| | Where | Whose |
|---|---|---|
| Routes, reading, SEO, the sitemap, the account, the words, the formats, the kit's icons | `web/<name>/*_gen.go`, `kit.templ`, `kit.js` | generated |
| The frame (header, menu, footer), the first page, the search, each entity's list, card and page | `layout.templ`, `home.templ`, `search.templ`, `<entity>.templ` | yours |
| Your pages; the words you change; the styles; your icons | `routes.go`, `messages.go`, `site.css`, `icons.go` | yours |

With `engine go`, the app's binary renders the site: [templ](https://templ.guide)
for the pages (`esquadro gen` writes their Go, and `task dev` writes it
again as you change them), [htmx](https://htmx.org) for lists changed in
place, Tailwind for the styles (`npm run build`, or `task web:build`,
writes them into `dist/assets`). The site uses the theme's options and
the shared words of every frontend.

Pages link the assets with a hash of their content (`Asset("site.css")`
writes `/assets/site.css?v=5e3091a8c7`): browsers keep an asset at its
version for good, and a new build is a new address, seen at once after a
deploy.

### A site with a brand

A site whose look is a brand of its own, not the frontends' default,
names the project's folder of its brand:

```esquadro
blueprint ssr site {
  brand "brand"
}
```

The folder has a `theme.json` and the files it names; the app embeds it
(`brand/embed.go`, generated) and the site serves it at `/brand/`, kept by
browsers for a day:

```json
{
  "colors": {
    "accent": "#2F6FEB", "on_accent": "#FFFFFF", "accent_edge": "#1E4FB8",
    "accent_text": "#1E4FB8", "accent_soft": "#EAF1FE",
    "ink": "#111827", "muted": "#6B7280", "page": "#FAFAF9", "surface": "#FFFFFF",
    "line": "#E5E7EB", "neutral": "#F3F4F6", "neutral_edge": "#D1D5DB",
    "sale": "#E5484D"
  },
  "fonts": {
    "display": { "family": "Display Sans", "files": { "700": "fonts/display-bold.woff2" } },
    "body": { "family": "Body Sans", "files": { "100 900": "fonts/body-variable.woff2" } }
  },
  "text": {
    "section": { "font": "display", "weight": 800, "size": 28, "line": 1.1, "tracking": -0.02 },
    "caption": { "font": "body", "weight": 500, "size": 12, "color": "muted" }
  },
  "radius": { "sm": 10, "md": 14, "lg": 20 },
  "buttons": "raised",
  "control": 44,
  "motion": { "enter": { "duration": 350, "curve": "cubic-bezier(.22,1.2,.36,1)" } },
  "states": { "empty": "states/empty.svg", "not_found": "states/lost.svg", "gone": "states/gone.svg" },
  "identity": { "logo": "logo/line.svg", "icons": "icons", "og_image": "og.png", "theme_color": "#2F6FEB" }
}
```

- **colors**: by role (`accent`, `on_accent`, `accent_edge`, `accent_text`,
  `accent_soft`, `ink`, `muted`, `page`, `surface`, `line`, `neutral`,
  `neutral_edge`, `ok`, `error`, `warning`, `info` and
  their `_soft`), each `--brand-<role>` and a Tailwind color
  (`bg-accent-soft`, `text-accent-text`); a role left out keeps the
  default look's. Any other name is a color of its own (`bg-sale`). The
  kit's tokens follow: `bg-primary` is the accent, `text-muted-foreground`
  the muted, `border-border` the line.
- **fonts**: `display` (titles, prices) and `body`, by weight, a range
  for a variable font; `font-display` and `font-body` in Tailwind,
  preloaded by every page.
- **text**: text roles of the names you choose, each a class
  `.t-<name>` with its font, weight, size, line height and letter
  spacing; the kit's titles and texts carry `.t-section`, `.t-block`,
  `.t-item` and `.t-text`, so roles of those names style them.
- **radius** (`rounded-sm`, `rounded-md`, `rounded-lg`), **control**
  (the height of buttons and fields), **buttons** (`flat`, or `raised`: an
  edge at the bottom that sinks when pressed), **motion**
  (`--motion-<name>` and `--ease-<name>`).
- **states**: the pictures of `empty`, `loading`, `success`, `error`,
  `not_found`, `forbidden` and `gone`; the error pages and the empty lists
  show them (`@StatePicture("empty")` in your pages).
- **identity**: the `logo` of the header (`@Logo()`), the folder of
  `icons` whose files are served at the root too (`favicon.ico`,
  `favicon.svg`, `apple-touch-icon.png`, `site.webmanifest`), linked by
  every page; the `og_image` of a page that sets none; the browser's
  `theme_color`.

`theme.gen.css` then writes the brand: the tokens, the fonts, the roles,
and the kit's pieces, `.btn` (`.btn-primary`, `.btn-secondary`,
`.btn-icon`), `.field`, `.chip`, `.card`, `.notice-ok|error|warning|info`,
and `.progress`; what is your product's own (a price, a product's photo)
goes in `site.css`, on the same tokens. `esquadro check` says what is wrong with the
brand: a color that is none, a role or state that does not exist, a file
that is not in the folder.

## Where it runs

The app's binary serves the frontend's build (`serve app`, the only way so
far): on its domain, where the app's own paths (`/api`, `/identity`) still
reach the app, so the frontend calls the API without CORS; or under its path,
without one. The image builds the frontend with Node first. On a
[platform](/esquadro/production/), add the frontend's domain to the app's
`domains`.
