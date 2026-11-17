---
title: Jobs
description: Work done in the background, declared in the model and run by the worker.
sidebar:
  order: 5
---

A **job** is work done in the background: an email after a sign-up, an
invoice to send, a report every night. It belongs to a module, like an
entity, and its fields are its **arguments**:

```esquadro
package accounts

/// Sends the welcome email to a new user.
job SendWelcomeEmail {
  user_id uuid   required
  locale  string default("en")
}

/// Closes the day's report, every day at 3:00.
job CloseDailyReport {
  @jobs(every "0 3 * * *")
}
```

Arguments have the types and rules of fields (`required`, `min`, `max`,
`pattern`, `default`) and may be enums of the module. A job has no
relations, because its arguments are stored in the queue as values: pass an
id instead (`user_id uuid`). `unique` is for entities. A job without
arguments may be written without its block: `job CloseDailyReport`.

Jobs need the [jobs](/esquadro/blueprints/reference/jobs/) blueprint, which
runs them on a queue in PostgreSQL ([River](https://riverqueue.com));
declaring one without it is an error that says `esquadro add jobs`. The
`worker` starter has it.

## What is generated

For `SendWelcomeEmail` in module `accounts`:

- `SendWelcomeEmailArgs`, the arguments, with `WithDefaults` and `Validate`
  as for an entity;
- `accounts.EnqueueSendWelcomeEmail(ctx, args)`, a method of the module, and
  `EnqueueSendWelcomeEmailTx(ctx, tx, args)`, which enqueues the job in a
  transaction: only if it commits. Invalid arguments are refused with
  `*InvalidFields`, as for entities;
- `EnqueueSendWelcomeEmailAt(ctx, at, args)` and `...AtTx`, which run the
  job at a time (right away if it has passed) and answer its id, and
  `accounts.CancelJob(ctx, id)`, which cancels one that has not run yet;
- `internal/accounts/send_welcome_email_work.go`, **a file of yours**, written
  once, where the work goes:

```go
func (m *Accounts) workSendWelcomeEmail(ctx context.Context, args SendWelcomeEmailArgs) error {
	// send the email; an error makes the queue try again later
	return nil
}
```

- with `testcontainers`, tests that enqueue each job, refuse invalid
  arguments, see that a transaction rolled back enqueues nothing, work a
  job through the queue, and run one at its time but not a cancelled one.

A use case enqueues a job with its module's method:

```go
if err := m.EnqueueSendWelcomeEmail(ctx, accounts.SendWelcomeEmailArgs{UserID: user.ID}); err != nil {
	return err
}
```

## At a time

A job can wait for a time: "post this offer at 8 PM". Keep its id to cancel
it; rescheduling is cancelling and enqueueing again.

```go
id, err := m.EnqueuePostOfferAt(ctx, offer.PostAt, catalog.PostOfferArgs{OfferID: offer.ID})
// later, if the operator changes their mind
err = m.CancelJob(ctx, id)
```

A cancellation can lose a race with the job starting, and two people can
reschedule at once: make the work check, when it runs, that what it was
enqueued for still holds (the offer is still approved, its time is still
this one), so that nothing happens twice or too late.

## Running them

The worker's `run` command works the queue, side by side with its other
processes; the app's `migrate` creates the queue's tables. An API and its
worker are the same binary: `serve` answers requests and enqueues, `run`
works the jobs, in another process or container.

```sh
task migrate
go run ./cmd/shop run     # or the image: docker run shop:dev run
```

A job that fails is tried again later, with a growing wait, up to its
attempts (25 by default); a job is worked once even with several workers;
a job enqueued survives the worker stopping.

## Options

`@jobs(...)` on a job:

| Key | |
|---|---|
| `every "..."` | enqueue it on a schedule, with its arguments' defaults: cron (`"0 3 * * *"`) or `"@every 10m"`, `"@hourly"`, `"@daily"` |
| `max_attempts 5` | attempts before it is given up |
| `queue "slow"` | the queue it goes to |

The blueprint's options set the queues the worker works and how many jobs
each works at once:

```esquadro
blueprint jobs {
  workers 10
  queues  ["default", "slow"]
}
```
