---
title: Settings
description: "An entity with one record only, always there with its defaults: what the team tunes without a deploy."
---

Some values are no list of records: a daily limit, the hours to send in,
the text of a page, an affiliate id. There is one of each, the team
changes them without a deploy, and the code reads them. Mark an entity
`@store(single)` and it has one record only, which always exists:

```esquadro
/// What the team tunes without a deploy.
entity SiteSettings {
  daily_limit   int    required default(20) min(1) max(100)
  sending_hours string required default("08:00-22:00")
  invite_url    string
  groups_text   text   default("Join our groups for the best deals.") @rest(html)
  @store(single)
  @security(read public, write user.admin)
  @ui(label "Settings")
  @admin(resource)
}
```

It is an entity like the others: its fields, rules, `@security`, business
rules (`BeforeUpdate`, `AfterUpdate`), audit and notifications all apply.
What changes is that it is never listed, created or deleted.

## Reading and changing it

The module reads and changes it without an id:

```go
s, err := m.ReadSiteSettings(ctx)               // with its defaults until someone saves
s, err = m.UpdateSiteSettings(ctx, s.Version, n) // a version, or 0 for none
s, err = m.PatchSiteSettings(ctx, 0, deals.SiteSettingsChanges{DailyLimit: &limit}, nil)
```

The first read stores the record with the defaults of its columns, so the
code never meets an empty one: from then on it has a version, an ETag and a
history like any record. That is why a required field needs a `default`.

The API has it at one path, with `If-Match` as the other records:

| Request | |
|---|---|
| `GET /api/v1/site-settings` | the record |
| `PUT /api/v1/site-settings` | replaces it |
| `PATCH /api/v1/site-settings` | changes the fields sent |
| `GET /api/v1/site-settings/history` | its changes, when audited |

`POST` and `DELETE` answer `405`. The TypeScript client has
`api.siteSettings.get()`, `.update(record)`, `.patch(changes)` and
`.history()`.

In the admin panel, the resource opens on its form, with its history under
it when the entity is audited.

## What it may not have

`esquadro check` refuses, on a single entity:

- a required field without a `default`: it would have no value before the
  first save;
- `soft_delete`: its one record is never deleted;
- relations, either way: keep the id of a record in a `uuid` field, and
  read the single entity with `Read<Entity>` instead of pointing at it;
- a page of its own (`@site(page)`), or being a principal.

Seeds leave it out: its record has its defaults.

A resource of the admin panel may not be named as one of the panel's own
pages (`settings`, `audit`, `notifications`, `accounts`...): call the
entity `SiteSettings` rather than `Settings`.

## Generated tests

The module's tests read it before anyone saved it (its defaults), save it,
patch one field and refuse an old version; the API's, the same through
the contract, with `POST` and `DELETE` refused.
