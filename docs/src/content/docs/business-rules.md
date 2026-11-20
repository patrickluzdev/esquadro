---
title: Business rules
description: "The rules of your product that the model cannot say, in a file of yours per entity: run by every change, in its transaction."
---

The model says what each field takes and who may do what. A product has
rules besides: no more than 20 offers a day in each group, only within
its hours, never approve an offer that expired. They go in Go, in a file of
yours for each entity, `internal/<module>/<entity>_rules.go`:

```go
package deals

import "context"

type SendRules struct {
	sendHooks
}

// BeforeCreate: no more than 20 sends a day to each group.
func (SendRules) BeforeCreate(ctx context.Context, m *Deals, n *NewSend) error {
	if err := m.Lock(ctx, "sends:"+n.DestinationID.String()); err != nil {
		return err
	}
	today, err := m.CountSendsToday(ctx, n.DestinationID) // a use case of yours
	if err != nil {
		return err
	}
	if today >= 20 {
		return Broken("daily_limit", "This group got 20 offers today.", "destination_id")
	}
	return nil
}
```

esquadro writes the file once, with no rule; a method you write there
replaces the one of `<entity>_rules_gen.go`, which does nothing:

| Method | When | Gets |
|---|---|---|
| `BeforeCreate` | before a new record is stored | the new one (`*New…`: it may change it) |
| `BeforeUpdate` | before a change, replaced or patched | the record as stored, and the new one (`*New…`) |
| `BeforeDelete` | before it is deleted, or put in the [trash](/esquadro/trash/) | the record |
| `AfterCreate`, `AfterUpdate`, `AfterDelete` | once it is written | the record (before and after, for a change) |

- **Every change runs them**: the API, the frontends, the jobs and your
  own code all write through the module's use cases, which call them.
- **In the change's transaction**: what they read is what is about to
  change, and what they do is kept with it (an `After` method that
  enqueues a job with `Enqueue…Tx`, or changes another record). An error
  undoes it all.
- **A changed record is checked again**: what a `Before` method sets passes
  the model's rules too.

## Refusing

`Broken(code, text, fields...)` refuses with your code and what people are
told. The API answers it as 422, with `code` and `title` yours and the
fields in `errors`, which the frontends show on them:

```json
{ "status": 422, "code": "daily_limit", "title": "This group got 20 offers today.",
  "errors": [{ "field": "destination_id", "code": "daily_limit" }] }
```

Write the text in your users' language: it is shown as it is.

## Counting right

A rule that counts before it writes ("20 a day") would let two changes at
once through, both counting 19. `m.Lock(ctx, key)` takes a lock until the
transaction ends: a second change of the same key waits for the first, and
counts it. Lock what the rule counts: the group, the person, the day.

## Transitions

`BeforeUpdate` sees the record before and after, so a rule may be about
what changes:

```go
// An offer that expired is not approved.
func (OfferRules) BeforeUpdate(ctx context.Context, m *Deals, stored Offer, n *NewOffer) error {
	if stored.Status == OfferStatusExpired && n.Status == OfferStatusApproved {
		return Broken("offer_expired", "This offer expired: it cannot be approved.", "status")
	}
	return nil
}
```

Who may change it at all is the [policy](/esquadro/security/)'s; the rules
come after it, for everyone it lets in.

## The generated tests

The API's generated tests (`cmd/<app>/api_test.go`) create records with
their required fields only. A rule that wants more (an affiliate link, a
description) refuses them: give them what it wants in a test file of yours,
by entity, and every generated test that makes one sends it.

```go
// cmd/shop/given_test.go
package main

import "example.com/shop/internal/apigen"

func init() {
	apiGiven["Offer"] = func(req any, i int) {
		req.(*apigen.NewOffer).AffiliateURL = apigen.NewOptString("https://shop.example/a")
	}
}
```
