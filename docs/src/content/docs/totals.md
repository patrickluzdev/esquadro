---
title: Totals and orders
description: "Fields the store keeps from other records (a sum of votes, a count of comments) and orders of a list worked out by an SQL expression, such as a hot list."
---

An offer's **temperature** is the sum of its votes; its number of
comments, a count. Neither is written by anyone: the store keeps them, from
the records themselves. A **hot** list orders offers by that temperature
weighed by time, an expression no field holds. Both are declared in the
model, with the [store](/esquadro/blueprints/reference/store/) blueprint:

```esquadro
entity Offer {
  title       string required
  temperature int    @store(sum votes.value, sort)
  comments_n  int    @store(count comments)
  votes       < Vote
  comments    < Comment
  @store(order [hot, "temperature / power(extract(epoch from now() - created_at) / 3600 + 2, 1.5)"])
}

entity Vote {
  value int required min(-1) max(1)
  offer > Offer required on_delete(cascade)
}

entity Comment {
  body  text required
  offer > Offer required on_delete(cascade)
}
```

## Totals

`@store(sum <relation>.<field>)` adds up a field of the records of a
one-to-many relation; `@store(count <relation>)` counts them. The store
works the total out again **in the transaction of every change of one of
them**: a create, an update (and a patch), a delete, the trash and a
restore, an import. It counts again with a `SELECT` of the records, never
by adding or taking away, so that it never drifts; a record moved from one
offer to another counts again in both. Records in the trash do not count.

- A total is read only: `NewOffer` and `OfferChanges` leave it out, and so
  do the API's requests, the TS client's `NewOffer` and the panel's form.
  The record, its answer and its list carry it.
- Its column is `NOT NULL DEFAULT 0`. Added to a table with rows, the
  migration fills it with what the records add up to.
- The parent is locked (`FOR NO KEY UPDATE`) before counting, so that two
  votes at once both count. A total does not change the record's
  `version` or `updated_at`: a vote does not refuse someone's edit of the
  offer with `412`.
- A count is `int` or `int64`; a sum is of an `int` (into `int` or
  `int64`), an `int64` or a `decimal`, of the same type.
- A total may be `@store(sort)` and `@store(filter)`, as other fields.

Records the database deletes with another record of the module (votes
whose `poll > Poll on_delete(cascade)` go with their poll) count out too:
deleting the poll works out again the offers its votes were on. A cascade
two relations away (votes of the polls of a deleted campaign) is not
followed; delete those records through the module instead.

## Orders

`@store(order [hot, "<SQL expression>"])` gives the list a sort worked out
by an expression over the entity's columns: `sort=hot` lists the largest
first, and `-hot` the smallest. More orders go on in pairs:
`order [hot, "...", rising, "..."]`. An order cannot have the name of a
field, `created_at` or `relevance`.

An order's value changes with time (the hot list above cools as hours
go by), so a cursor would lose its place: a list in an order **pages by
number** only (`page`, `per_page`), and an entity with orders needs `page`
among its paginations. The API pages by number when asked for an order
without `page`; with `after`, it answers `400`. In Go:

```go
page, err := m.ListOffers(ctx, deals.OfferQuery{Sort: "hot", Page: 1, PerPage: 20})
```

The orders reach every list: the API's `sort` (and its description in the
contract), the TS client's `sort` type, a select beside the search in the
admin panel, and the site's list (`@site(page, sort "hot")` makes it its
first order).

The expression goes into the SQL as written: it is the model's, never a
request's. An expression without an index reads the whole table to order
it; for a large one, keep the list filtered, or write an index by hand in
a migration of your own.

## Votes

The votes themselves are an entity like any other. To vote once per
person, give it a unique index and an owner, and voting again is changing
one's vote:

```esquadro
entity Vote {
  value int required min(-1) max(1)
  offer > Offer required on_delete(cascade)
  voter > Customer required
  @postgres(unique_index [offer, voter])
  @security(read public, write owner)
}
```
