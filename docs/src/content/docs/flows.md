---
title: State flows
description: "The changes an enum field may go through, and who makes each: refused off the flow by the module, refused to others by the API, and told to clients in each record's can."
---

An offer posted by a visitor goes through states: a draft, in review, then
published or refused, and at last expired. Who takes it from one to the
next varies: its author sends it to review, the moderators publish or
refuse it, a job expires it. That is **a flow**: the changes an enum field
may go through, and who makes them. It is declared on the field, with the
[store](/esquadro/blueprints/reference/store/) blueprint:

```esquadro
enum OfferStatus { DRAFT REVIEW PUBLISHED REFUSED EXPIRED }

entity Offer {
  title  string required
  status OfferStatus default(DRAFT) @store(flow [
    "DRAFT -> REVIEW",
    "REVIEW -> PUBLISHED by user.admin, user.moderator",
    "REVIEW -> REFUSED by user.admin, user.moderator",
    "PUBLISHED -> EXPIRED",
  ])
  @security(read public, write [user])
}
```

Each move is `FROM -> TO`, and may say `by` whom, as `@security`'s rules
name them: a realm (`user`, whoever signed in to it) or `realm.role`
(`user.admin`), or a service key's role (`service.publisher`).

## In the module: every change

- A new record starts in the field's **default**, the flow's first state:
  creating one in another state is refused with `invalid_fields`, the
  field's code `not_allowed`.
- A change (an update, a patch, an import's) goes only where the stored
  state may go; another is refused the same way. Staying in the same state
  is no move, and always passes.
- This holds for every change, whoever makes it: a request, a job, your
  code, an entity's rules (checked after `BeforeCreate` and
  `BeforeUpdate`). `by` is not the module's: a job that expires offers
  makes `PUBLISHED -> EXPIRED` as the system, which the flow allows.

The module has, for each field with a flow, `Next<Field>()`, the states a
record may go to (`offer.NextStatus()`), and `NewOffer.Follows(stored)`,
the check itself, for your own code.

## In the API: who makes each move

- A move with `by` is asked of the entity's policy,
  `OfferPolicy.StatusTo(ctx, offer, to)`: someone who may update the offer
  but is not among those `by` names gets `403`. A move without `by` is
  anyone's who may update it. As any of the policy's actions, write
  `StatusTo` in `offer_policy.go` to decide otherwise.
- A record's `can` says, besides `update` and `delete`, where the person
  asking may take it: `"status:PUBLISHED"`, `"status:REFUSED"`. The TS
  client types them; the contract describes the flow with the entity.

## In the admin panel

An action that moves a field with a flow only shows on a record whose
`can` lets it, so that "Publish" and "Refuse" appear to the moderators on
the offers in review, and to no one else:

```ts
actions: [
  { label: "Send to review", patch: { status: "REVIEW" } },
  { label: "Publish", patch: { status: "PUBLISHED" }, key: "p" },
  { label: "Refuse", patch: { status: "REFUSED" }, key: "r", destructive: true },
],
queue: { filter: { status: "REVIEW" } },
```

A moderation queue is the resource's `queue` of the offers in `REVIEW`,
with those actions on their keys: see
[queues and actions](/esquadro/frontends/#queues-and-actions). The form
still offers every state; the API refuses the moves the flow or the person
may not make.

## Seeds and imports

Seeds and imports go through the module, and so through the flow: a seed's
record starts in the first state, and an import changes a record only
along its flow. To load records in later states, create them and move them
along, in your own code.
