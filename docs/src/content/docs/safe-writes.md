---
title: Safe writes
description: Changes that do not overwrite someone else's, and retries that do not create twice.
---

Two things go wrong with an API in production: two people edit the same
record and the second save silently undoes the first; or the network drops
after the server stored a `POST`, the client retries, and the record is
created twice. The generated API prevents both.

## Versions and If-Match

Every record has a `version`, starting at 1 and going up with each change.
Its answers carry it as the `ETag` header too:

```sh
GET /api/v1/products/0198…        → 200, ETag: "3"
PUT /api/v1/products/0198…        If-Match: "3"   → 200, ETag: "4"
PUT /api/v1/products/0198…        If-Match: "3"   → 412 precondition_failed
```

Send `If-Match` with the `ETag` you read on `PUT`, `DELETE` and on linking
or unlinking a many-to-many relation (which changes the parent record): if
the record changed since, the request answers `412 precondition_failed` and
changes nothing. Read it again, and decide. `*` matches any version.

The check is part of the `UPDATE` itself: no other change can get in between.

Without `If-Match`, a change applies to whatever version there is. To make it
required (`428 precondition_required` without it), set the
[rest](/esquadro/blueprints/reference/rest/) blueprint's `if_match` option to
`required`, or per entity:

```esquadro
entity Product {
  ...
  @rest(if_match required)
}
```

In your code, the use cases take the version too; `0` is any:

```go
updated, err := c.UpdateProduct(ctx, id, read.Version, changes)
if errors.Is(err, catalog.ErrProductChanged) { ... }
```

## Idempotency-Key

Send a new random UUID as `Idempotency-Key` with a `POST`, and the same one
when you retry it:

- the first request runs, in one database transaction with the key: the
  record, the key and any job it enqueued are stored together or not at all;
- a retry with the key answers what the first answered, with
  `Idempotent-Replayed: true`, without running again;
- a retry that comes while the first still runs waits for it (and answers
  `409 idempotency_in_progress` if it waits more than a few seconds);
- the key with another body answers `422 idempotency_key_reused`;
- a server error (5xx) keeps nothing: the retry runs for real.

Keys belong to who sent them (the signed-in principal), and answer retries
for 24 hours (the `idempotency_hours` option); expired ones are removed as
new keys come. Use a random UUID (v4): without sign-in, whoever guesses a key
could read its answer.

To require a key on every create (`428 idempotency_key_required` without
it), set the `idempotency` option to `required`, or per entity with
`@rest(idempotency required)`.

Your code can run a use case in a transaction the same way:
`postgres.WithTx(ctx, tx)` makes the `store` and `jobs` use `tx`.
