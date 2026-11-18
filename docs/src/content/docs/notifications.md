---
title: Notifications
description: "Notices to the people who sign in, sent by the model's rules or by your code, under a bell in the frontends."
---

With the `notifications` blueprint, the people who sign in get notices: a
new order for the admins, a task's status changed for everyone. Each person
lists, counts and reads their own; a [frontend](/esquadro/frontends/) shows
them under a bell in its header.

## From the model

An entity's rule sends a notice when its records change:

```esquadro
app shop {
  blueprint notifications
}

entity Order {
  number   string required
  customer string required
  @notifications(on create, to user.admin, title "New order {number}", text "From {customer}")
}

entity Task {
  title  string required
  status Status
  owner  > User required
  @notifications(on update, when status, to owner, title "{title} is now {status}")
}
```

- `on`: `create`, `update`, `delete`; with `when`, an update notifies only
  when one of those fields changed.
- `to`: a realm's people (`user`), a role's (`user.admin`), or the record's
  owner (`owner`). Who made the change is never notified, and neither is a
  disabled account.
- `title` and `text`: `{field}` is the record's field.

The notice is written in the same transaction as the change: if the change
does not stay, neither does the notice. It leads to the record's page.

## From your code

```go
err := notifications.Send(ctx, db, "user.admin", notifications.Notice{
    Title: "Import finished", Text: "1,200 products", Link: "/products",
})
err = notifications.ToPerson(ctx, db, "user", userID, notice)
```

`db` is the transaction of the change (`postgres.Querier(ctx, pool)`), or
the pool.

## The API

| Route | What |
|---|---|
| `GET /notifications?unread=true&page=` | the notices of who signs in, the newest first, with how many are unread |
| `GET /notifications/count` | how many are unread (what the bell asks) |
| `POST /notifications/{id}/read`, `POST /notifications/read` | mark one, or all, as read |
| `DELETE /notifications/{id}` | delete one |

The [TypeScript client](/esquadro/typescript-client/) has them as
`api.notifications`.

## Options

| Option | Default | |
|---|---|---|
| `poll_seconds` | `30` | how often a frontend's bell asks; `0`, only when a page opens or its window comes back to the front |
| `retention_days` | `90` | notices read longer ago are deleted, by a daily job with [jobs](/esquadro/blueprints/reference/jobs/), and by `<app> notifications purge [--days N]`; `0` keeps them |
| `page_size` | `10` | how many the bell shows |

The spa's `notifications false` leaves the bell out.
