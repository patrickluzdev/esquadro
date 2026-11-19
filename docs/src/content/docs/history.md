---
title: Value history
description: "Every value a field takes, with when, kept by the store in the transaction of each change: read by the module as points or a low and a high, by the API and in the admin panel."
---

An offer's price goes up and down. To say it is "the lowest in 90 days",
or to draw how it moved, you need every price it had, not only the last.
Mark the field with `@store(history)`, with the
[store](/esquadro/blueprints/reference/store/) blueprint:

```esquadro
entity Offer {
  title string required
  price money required @store(history)
  @security(read public, write [user])
}
```

A history is of an `int`, `int64`, `decimal` or `money` field, declared
(not one esquadro keeps, nor a total), of an entity with many records (not
`@store(single)`).

## What is kept

Each field gets a table of its own, `offer_price_history`: the record's
key (gone with the record, `ON DELETE CASCADE`), the value (a money's
amount and currency, as the entity's own columns), and `at`, when it took
it. The migration that creates it starts it with the value each record has.

A point is added in the transaction of the change itself:

- when a record is created with a value;
- when an update, a patch or an import changes it; not when it stays the
  same;
- not when it becomes empty (an optional money left out).

Putting a record in the trash and restoring it change nothing; deleting it
for good deletes its history.

## In the module

For each field, the module reads its points and how low and how high it
went:

```go
since := time.Now().AddDate(0, 0, -90)
points, err := deals.OfferPriceHistory(ctx, id, since) // []OfferPricePoint{Value, At}, the oldest first
took, err := deals.OfferPriceRange(ctx, id, since)     // OfferPriceRange{Low, High, Points, First}
if took.Points > 0 && offer.Price.Equal(took.Low) {
	// the lowest in 90 days
}
```

A history answers the newest 1000 points when there are more. A range
reads all of them in the database: `Points` is how many, 0 when none;
`First`, when the first in the window was taken (a price kept for less
than 90 days has no "lowest in 90 days" yet).

## In the API

`GET /api/v1/offers/{id}/price-history?since=2026-08-01T00:00:00Z` answers
`{"items": [{"value": ..., "at": ...}]}`, the oldest first, since 90 days
ago when `since` is left out; whoever may read the offer may read it. A
money's value is written as the API writes money. The TS client has it as
`api.offers.priceHistory(id, { since })`.

## In the admin panel

A record's page shows a card for each field with a history: a line of its
values in the last 90 days, with the lowest and the highest.
