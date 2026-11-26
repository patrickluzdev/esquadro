---
title: Security
description: Who signs in, what each one may do, and the defenses at the edge.
---

The [security](/esquadro/blueprints/reference/security/) blueprint keeps
security in one place. Without anyone who signs in, it answers CORS and sends
the security headers. Mark an entity `@security(principal)` and it also
generates sign-up, sign-in, sessions, tokens, password reset, social sign-in
and the checks of who may call each operation of the API. The `api` starter
has it, with a `User` and a `Customer`.

## Realms

Each entity marked `@security(principal)` is a **realm**: a kind of account,
with its own sign-up, sign-in, sessions and routes. A `User` and a `Customer`
never share an identity, even with the same email.

```esquadro
entity User {
  name string required
  @security(principal, roles [admin, staff], permissions [orders.refund],
            grants [admin.orders.refund], read user, write user.admin)
}

entity Customer {
  name string required
  @security(principal, signup open, providers [password, google])
}
```

| Setting | Values | Default |
|---|---|---|
| `signup` | `open` (anyone), `invite`, `closed` | `closed` |
| `providers` | `password`, `google`, `github` | `[password]` |
| `roles` | the realm's roles | none |
| `permissions` | named after what they are about and the action: `orders.refund` | none |
| `grants` | which role has which permission: `admin.orders.refund` | none |
| `verify_email` | `required` (no sign-in before), `optional`, `off` | `optional` |
| `password_reset` | `true`, `false` | `true` |
| `password_min`, `password_max`, `password_blocklist` | the realm's own password rules | the blueprint's |

An identity has the id of its principal: the `users` row of an account has the
id of its identity. The identities, credentials, sessions and the rest live in
`security_` tables (or the `security` schema, with the postgres blueprint's
`schemas ["security"]`), created by migrations of the blueprint.

## Who may do what

Every entity says who may `list`, `read`, `create`, `update` or `delete` it;
`write` is the last three. Each is `public` (anyone, signed in or not), a realm
(`user`: any of its accounts), a role (`user.admin`), or `owner`: the
principal the record belongs to.

```esquadro
entity Product {
  ...
  @security(read public, write [user.admin, user.staff])
}

entity Review {
  text     string required
  customer > Customer required
  @security(read [owner, user], create owner, update owner, delete [owner, user.admin])
}
```

- `list` follows `read`, and `create`, `update` and `delete` follow `write`.
- An entity without `@security` may be changed by whoever signed in, in any
  realm; `esquadro check` warns of it.
- An entity with rules, but none for an operation, lets no one do it.
- Linking and unlinking a many-to-many relation is an `update` of the record;
  listing it, a `read`.

### Owners

`owner` is the relation of the entity to a principal: `customer > Customer`
above. With more than one, `@security(owner buyer, ...)` says which; a
principal (`Customer` itself) owns its own record. With `owner`:

- a list holds only the records of whoever asks, unless another rule lets
  them see all (an admin sees every review); the query filters them, so pages
  stay whole;
- a record someone may not read answers `404`, as if it did not exist, and
  `401` to a request without a session or token: no one learns which ids
  exist;
- a record created by its owner is theirs, whatever the body says
  (`customer_id` is then optional in the API); someone let in by another rule,
  such as an admin, chooses the owner, creating on someone's behalf;
- an owner cannot hand a record over to someone else on update.

### Fields only some may read

A record everyone may read can hold a field that only some may: what the
store pays, a cost, a note of the team. Say who, on the field:

```esquadro
entity Offer {
  title      string  required
  commission decimal @security(read [user.admin, user.operator])
  @security(read public, write user.admin)
}
```

The API answers the others the record without the field, as if `fields`
had left it out, and refuses them, with 403 (401 when nobody signed in),
what would tell its value: a filter by it, a sort by it, its history. The
contract says it, and the project's access tests check it, as each one who
may and may not. A field only some may read cannot be in the search
(`@store(search)`), which everyone who lists uses.

Your own code reads the record whole: it is the API that leaves the field
out. A page of a site, or a route of your own, that shows records to
everyone decides by itself what it shows.

### Limits of creates

With the [rate_limit](/esquadro/blueprints/reference/rate_limit/)
blueprint, an entity with an owner may say how many records each person
creates in a period: `N/min`, `N/h` or `N/d`.

```esquadro
entity Review {
  body     text required
  customer > Customer required
  @security(read public, write owner)
  @rate_limit(create "10/h")   // each customer creates at most 10 an hour
}
```

- A person is a principal of the owner's realm, and what they created is
  what they own, created in the period. The count is in the database, so
  it holds with more than one instance of the service, unlike the limit of
  requests a minute.
- Past it, the create answers `429 rate_limited`, with `Retry-After`: when
  the oldest of those records leaves the period.
- The limit is in the module's `Create<Entity>`, not in the API: the admin
  panel, the portal and your own code that creates with the person's
  context are limited too. An import counts each record it creates, and is
  refused whole past the limit.
- Anyone else is not limited: another realm (an admin creating on someone's
  behalf), a service key, or code without a person, such as a job or the
  seeds.
- `esquadro check` asks for an owner rule, and refuses a create that lets in
  anyone (`public`): a visitor without an account has no records to count.
- A record deleted for good no longer counts; with
  `@store(soft_delete)`, one in the trash still does.

### Policies

Each entity has a policy, in `internal/<module>/<entity>_policy.go`, that the
API asks before each operation: `List` (a scope: all, the owner's, or none),
`Read`, `Create`, `Update` and `Delete`. Its rules come from the model, in
`<entity>_policy_gen.go`, generated again with it; the file without `_gen` is
yours. Write an action there and it replaces the model's:

```go
// Only open orders change, and only by their customer.
func (p OrderPolicy) Update(ctx context.Context, o Order) bool {
	return p.orderRules.Update(ctx, o) && o.Status == StatusOpen
}
```

`List` says whether a request lists an entity's records (all of them, or
its owner's); `Narrow`, which of those it sees. The model's narrows none;
write it in your policy and the API's list asks it before reading, in its
query, so that pages, totals and cursors hold only those. Offers everyone
may read, but only the approved ones unless a user (the staff) signed in:

```go
func (OfferPolicy) Narrow(ctx context.Context, q *OfferQuery) {
	if !security.Allows(ctx, "user") {
		q.Filter.Status = []OfferStatus{OfferStatusApproved}
	}
}

func (p OfferPolicy) Read(ctx context.Context, x Offer) bool {
	return p.offerRules.Read(ctx, x) && (x.Status == OfferStatusApproved || security.Allows(ctx, "user"))
}
```

- `Narrow` changes the query after the request's own filters, so it has the
  last word: `?status=PENDING` from no one signed in lists the approved
  ones. The field it sets must be one the store filters by
  (`@store(filter)`).
- Write `Read` to agree, for a record asked by its id: one it refuses
  answers `404`, as if it did not exist.
- A list also leaves out each record `Read` refuses, though the query let it
  in. The page then holds fewer than asked, and its `total` still counts
  them; the next cursor goes on after them. Narrow in the query what you can,
  for pages and totals to be right.
- The records a list includes (`include=`) and a many-to-many relation's
  list (`GET /products/{id}/tags`) are the other entity's: only those its
  `Read` lets in.
- The sites (blueprint `ssr`) do not ask it: what a site shows is its
  `shown.go`'s, which narrows its lists of the records everyone may read
  (see [Frontends](/esquadro/frontends/#sites-rendered-on-the-server)).

Ask the policy in your own code too: `sales.OrderPolicy{}.Update(ctx, order)`.
The generated tests check the model's rules; `TestAPIAccess` checks that the
API answers as the policy that runs says, yours included.

The API also tells clients what the person may do, so that a screen offers
only that: each record comes with `"can": ["update", "delete"]` (its policy's
`Update` and `Delete`), and each list with `"can": ["create"]` (its
`CanCreate`: may this person create some?). Writing `Create` in your policy,
write `CanCreate` too, so that the two agree.

Each operation of the API answers `401 unauthorized` to a request without a
session or token that may not, and `403 forbidden` to one with a session;
its description in `openapi.yaml` says who may. In your own code,
`security.Require(ctx, "orders.refund")` answers the same problems, by the
roles' permissions; `security.Allows(ctx, "user.admin")` checks a rule. An
operation of your own, which is not an entity's, needs someone signed in.

## Signing in

The routes of each realm are under `/identity/<realm>/` (the `path` option).
With the [rest](/esquadro/blueprints/reference/rest/) blueprint they are part
of the API's contract, `/api/openapi.yaml` (and of `/docs`, with `api_docs`),
sign-up with the password's and the entity's rules; without it,
`/identity/openapi.yaml` describes them. Every JSON name is snake_case.

| Route | |
|---|---|
| `POST sign-up` | `{"email", "password", ...fields of the entity}`; answers `202` whether the email had an account or not |
| `POST sign-in` | `{"email", "password"}`: a session cookie and its `csrf_token`; with `"cookie": false`, a `session_token` for an `Authorization: Bearer` header |
| `POST sign-out` | ends the session |
| `GET session` | who signed in: `id`, `realm`, `roles`, `permissions`, `csrf_token` |
| `POST token` | a JWT of 15 minutes (`token_minutes`), for other services; they check it with `/.well-known/jwks.json` |
| `GET sessions`, `DELETE sessions`, `DELETE sessions/{id}` | the account's devices; ending the others needs a recent sign-in |
| `POST password` | `{"current_password", "new_password"}`; ends the other sessions |
| `POST verify-email/send`, `POST verify-email` | the verification link, and its `{"token"}` |
| `POST password/forgot`, `POST password/reset` | the reset link, and `{"token", "new_password"}` |
| `POST email`, `POST email/confirm` | a new email, confirmed by a link sent to it |
| `GET oauth/<provider>?redirect=` | sign in with Google or GitHub |
| `GET providers`, `POST providers/<provider>`, `DELETE providers/<provider>` | the ways the account signs in |
| `DELETE` (the realm's path) | deletes the account, after a recent sign-in |

**Browsers** use the session cookie (`HttpOnly`, `SameSite=Lax`, named
`__Host-<app>_<realm>_session` outside development). A request that changes
something must come from the app's origin (or one of `origins`) and send the
session's `csrf_token` in `X-CSRF-Token`. **Other clients** sign in with
`"cookie": false` and send `Authorization: Bearer <session_token>`, or a JWT.

Sessions end after 24 hours without use (`session_idle_hours`) and 30 days in
all (`session_max_days`). Failed sign-ins lock an account for 15 minutes after
5 (`lockout_attempts`, `lockout_minutes`), except in development.

## Passwords

Passwords follow NIST SP 800-63B and the OWASP Password Storage Cheat Sheet:

- **Argon2id** (19 MiB, 2 iterations), stored in the PHC format and hashed
  again when the costs go up; an optional pepper (`pepper true`,
  `SECURITY_PEPPER`), kept out of the database.
- **At least 12 characters** (`password_min`), up to 128; any character,
  spaces and emoji included; no rules of composition.
- **Blocklists** (`password_blocklist`): `none` by default; `lists` refuses the
  passwords of your files (`password_lists ["config/passwords-br.txt"]`, for
  the language and country of your users), sequences, repetitions, the app's
  name and the person's email; `pwned` also asks Pwned Passwords, which only
  sees the first five characters of the password's SHA-1.

A password refused answers `invalid_fields` (422) on the field `password`
(also when the request sends it as `new_password`), with why: `too_short` or
`too_long`, with `limit` the realm's minimum or maximum, `common` or
`breached`:

```json
{"code": "invalid_fields", "errors": [{"field": "password", "code": "too_short", "limit": 12}]}
```

The frontends say it in their language ("The password needs at least 12
characters.", "This password showed up in a data breach. Choose another.")
and show the minimum under a new password's field before it is sent.

## Google and GitHub

Add `google` or `github` to a realm's `providers`, and register the app at the
provider with the redirect URI `<PUBLIC_URL>/identity/<realm>/oauth/<provider>/callback`.
Its client id and secret go in `SECURITY_GOOGLE_CLIENT_ID` and
`SECURITY_GOOGLE_CLIENT_SECRET` (or `SECURITY_GITHUB_...`), which exist only
when a realm uses the provider.

The app sends the browser to `GET /identity/<realm>/oauth/google?redirect=/home`
and gets it back at `/home`, signed in; or at `/home?error=<code>`. The flow
uses PKCE and a state bound to the browser. An account is linked to an
existing one by email only when both sides verified it; otherwise the person
signs in and links the provider. `redirect` is a path of the app or an address
of `redirect_urls`.

## Emails

Verification, password reset and email change send links to a page of your
app (`redirect`), which posts the token back. They go by the
[mail](/esquadro/email/) blueprint, which an app with realms needs: from the
templates `verify_email`, `reset_password`, `confirm_email_change` and
`email_changed`, which are yours to change; in development, to Mailpit.

With the [jobs](/esquadro/blueprints/reference/jobs/) blueprint, an hourly job
deletes expired sessions and links, and rotates the JWT signing key every
`rotate_keys_days`. Without jobs, the
[worker](/esquadro/blueprints/reference/worker/) does it; without either, run
`<app> security cleanup` on a schedule.

## Service keys

Not everything that calls the API is a person: a system that reads your
data, a script, a partner. Give it a **service key**: an identity of the
app, with roles the model declares, which the rules name as
`service.<role>`.

```esquadro
app shop {
  blueprint security {
    services [reader, importer]
  }
}

entity Product {
  @security(read [public], write [user.admin, service.importer])
}

report SalesByDay {
  @security(read [user.admin, service.reader])
}
```

An admin creates a key with the app's command; it is shown once, and only
its hash is kept:

```sh
shop security key create studio -role reader [-days 365]
shop security key list
shop security key revoke studio
```

Those who administer them do the same through the API, and in a
[frontend](/esquadro/frontends/)'s Service keys page:

| Route | What |
|---|---|
| `GET <prefix>/service-keys` | the keys, newest first: name, roles, prefix, status (`active`, `expired`, `revoked`), created, expires, last used; never the key |
| `POST <prefix>/service-keys` | create one: `{name, roles, days}`; the answer's `key` is the only time it is shown |
| `DELETE <prefix>/service-keys/{id}` | revoke it, at once |
| `GET <prefix>/service-roles` | the service roles, what each may do as the rules say, how many active keys have each, and the lifetimes offered |

All of it is the `security` blueprint's options:

| Option | Default | |
|---|---|---|
| `service_admins` | `[]` | who administers the keys (`[user.admin]`); empty, those who administer the first realm's accounts |
| `service_key_days` | `365` | how long a new key lasts unless told otherwise; `0` is until revoked |
| `service_key_max_days` | `0` | the most a key may last (more is refused, and so is until revoked); `0` is no limit |
| `service_key_choices` | `[30, 90, 365]` | the lifetimes a frontend offers, those within the limit |

The service sends it as `Authorization: Bearer sk_...`, and goes through the
same policies as people: what `service.reader` may do, the key may; the rest
answers `403`. An unknown, expired or revoked key answers `401`. A key is
not the owner of records, so `owner` never lets one in. The TypeScript
client takes it as its `token`, on a server.

## Administering accounts

Those an account's entity lets write it (`@security(write user.admin)`)
administer its realm's accounts through the API, and in a
[frontend](/esquadro/frontends/)'s Users page:

| Route | What |
|---|---|
| `GET <prefix>/<realm>/accounts?q=&role=&status=&page=` | the accounts: email, name, roles, status (`active`, `invited`, `unverified`, `disabled`), last seen, and `resend_at`: when an invited or unverified one's email may be sent again (`null`, now) |
| `POST <prefix>/<realm>/accounts` | invite: `{email, roles, fields, redirect}` makes the account without a password, and emails it a link to choose one, which works for `invite_days` (7) |
| `POST .../accounts/{id}/invite` | the invitation again, while it has no password |
| `POST .../accounts/{id}/verify-email` | the link that confirms its email again, while it is `unverified`: `{redirect}`, the page of the app it opens (`409 already_verified` otherwise) |
| `PUT .../accounts/{id}/roles` | its roles: `{roles}` |
| `POST .../accounts/{id}/disable`, `.../enable` | disable it (its sessions end; it cannot sign in), enable it again |
| `DELETE .../accounts/{id}/sessions` | sign it out everywhere |
| `DELETE .../accounts/{id}` | delete it, and its record |
| `GET <prefix>/<realm>/roles` | the realm's roles as the model declares them, their permissions and how many accounts have each |

No one disables, deletes or takes the administering role from their own
account (`own_account`), and the realm always keeps someone enabled who
administers it (`last_administrator`). The invitation is the mail
blueprint's `invitation` template, saying who invited; the confirmation, its
`verify_email`.

An account's invitation, or the confirmation of its email, is sent again
no sooner than 5 minutes after the last one (`resend_minutes`), nor more
than 5 times in a day (`resend_per_day`); the first one, of the invitation
or the sign-up, counts. Sooner, the routes answer `429 too_soon`, with
`Retry-After`, and the account's `resend_at` says when. The server keeps the
count, a day long, in `security_email_sends`, so it holds across requests,
processes and restarts; `POST verify-email/send` keeps it too, without
saying so, as it answers the same to anyone. `0` is no limit.

```esquadro
blueprint security {
  resend_minutes 10
  resend_per_day 3
}
```

## Commands

```sh
go run ./cmd/shop security create user ana@example.com name=Ana -role admin
go run ./cmd/shop security invite user bia@example.com name=Bia -role staff
go run ./cmd/shop security reset user ana@example.com
go run ./cmd/shop security grant user ana@example.com staff
go run ./cmd/shop security revoke user ana@example.com admin
go run ./cmd/shop security sessions user ana@example.com
go run ./cmd/shop security delete customer bia@example.com
go run ./cmd/shop security cleanup
go run ./cmd/shop security rotate-keys
```

The links of emails and of these commands begin with `PUBLIC_URL`, the
app's address in each environment (`https://shop.com`; required outside
development, where it is the `public_url` option, `http://localhost:8080`).
In development, a link to a page of a panel opens where `task dev` serves
it (`http://localhost:5173/admin/...`).

A frontend on a [domain of its own](/esquadro/frontends/#where-it-runs)
has its links there instead: `https://admin.shop.com/reset-password`,
whatever `PUBLIC_URL` is. Its pages send that address as the `redirect`
of their requests, and the app allows it without `redirect_urls`, which
holds the frontends' domains (over HTTPS) besides the ones it lists. In
development such a link opens where the frontend is: a panel's where
`task dev` serves it (`http://localhost:5173/reset-password`), a site's on
its alias on the app's port (`http://site.localhost:8080/nova-senha`, from
the development `PUBLIC_URL`). The same holds where a provider sends the
browser back to.

`create` prints a link that sets the password; with `-password-stdin` it reads
the password instead, never from the arguments. `reset` prints a new link
for an account that has one already, or lost it. The link, and `invite`'s,
opens the page that sets a password of the realm's frontend
(`/admin/reset-password` of a panel, `/nova-senha` of a site in pt_br,
`https://admin.shop.com/reset-password` of a panel on a domain of its own),
or the page `-redirect` names.

## Tests

- The generated API tests run as everyone: an account of each realm with every
  role. `TestAPIAccess` runs every operation as no one, each realm and each
  role, against the rules of the model.
- A request with `X-Test-Principal: user.staff` (or `nobody`) is signed in as
  that one alone, in the API tests.
- `securitytest.As(ctx, "user", "admin")` signs a context in, for code that
  calls `security.Require`; `securitytest.SignIn(t, s, "user", "admin")` makes
  an account and a session, and answers its `Authorization` header.

## At the edge

- **CORS**: the [http](/esquadro/blueprints/reference/http/) blueprint's
  `cors_origins` option (`<APP>_CORS_ORIGINS`), in every project with an
  HTTP server. A listed origin may send cookies and sign in with them;
  `"*"` lets any in, without them.
- **Headers**: `nosniff`, `DENY` framing, a `default-src 'none'` CSP, no
  referrer, `no-store`, and HSTS with `hsts true` (`<APP>_HSTS`), behind HTTPS
  only.
